package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jpp0ca/MusicMigration-API/internal/domain"
)

const (
	baseURL    = "https://api.spotify.com/v1"
	maxPerPage = 50
	maxBatch   = 100
)

// Provider implements ports.MusicProvider for Spotify using the Web API.
type Provider struct {
	client *http.Client
}

// NewProvider creates a new Spotify provider with the given HTTP client.
// If client is nil, http.DefaultClient is used.
func NewProvider(client *http.Client) *Provider {
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{client: client}
}

func (p *Provider) Name() string {
	return "spotify"
}

// -- API response types (internal) ------------------------------------------

type playlistsResponse struct {
	Items []playlistItem `json:"items"`
	Next  string         `json:"next"`
	Total int            `json:"total"`
}

type playlistItem struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Owner       playlistOwner `json:"owner"`
	Tracks      trackRef      `json:"tracks"`
}

type playlistOwner struct {
	DisplayName string `json:"display_name"`
}

type trackRef struct {
	Total int `json:"total"`
}

type tracksResponse struct {
	Items []trackItem `json:"items"`
	Next  string      `json:"next"`
}

type trackItem struct {
	Track trackData `json:"track"`
}

type trackData struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Artists     []artistData `json:"artists"`
	Album       albumData    `json:"album"`
	ExternalIDs externalIDs  `json:"external_ids"`
}

type artistData struct {
	Name string `json:"name"`
}

type albumData struct {
	Name string `json:"name"`
}

type externalIDs struct {
	ISRC string `json:"isrc"`
}

type searchResponse struct {
	Tracks searchTracks `json:"tracks"`
}

type searchTracks struct {
	Items []trackData `json:"items"`
}

type createPlaylistResponse struct {
	ID string `json:"id"`
}

// -- MusicProvider implementation --------------------------------------------

func (p *Provider) GetPlaylists(ctx context.Context, token string) ([]domain.Playlist, error) {
	var playlists []domain.Playlist
	endpoint := fmt.Sprintf("%s/me/playlists?limit=%d", baseURL, maxPerPage)

	for endpoint != "" {
		body, err := p.doGet(ctx, token, endpoint)
		if err != nil {
			return nil, fmt.Errorf("spotify: failed to get playlists: %w", err)
		}

		var resp playlistsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("spotify: failed to parse playlists response: %w", err)
		}

		for _, item := range resp.Items {
			playlists = append(playlists, domain.Playlist{
				ID:          item.ID,
				Name:        item.Name,
				Description: item.Description,
				OwnerName:   item.Owner.DisplayName,
				TrackCount:  item.Tracks.Total,
			})
		}

		endpoint = resp.Next
	}

	return playlists, nil
}

func (p *Provider) GetPlaylistTracks(ctx context.Context, token string, playlistID string) ([]domain.Track, error) {
	var tracks []domain.Track
	endpoint := fmt.Sprintf("%s/playlists/%s/tracks?limit=%d", baseURL, playlistID, maxPerPage)

	for endpoint != "" {
		body, err := p.doGet(ctx, token, endpoint)
		if err != nil {
			return nil, fmt.Errorf("spotify: failed to get playlist tracks: %w", err)
		}

		var resp tracksResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("spotify: failed to parse tracks response: %w", err)
		}

		for _, item := range resp.Items {
			if item.Track.ID == "" {
				continue // skip local or unavailable tracks
			}
			tracks = append(tracks, toTrack(item.Track))
		}

		endpoint = resp.Next
	}

	return tracks, nil
}

func (p *Provider) SearchTrack(ctx context.Context, token string, track domain.Track) (*domain.Track, float64, error) {
	// Try ISRC-based search first for higher accuracy
	if track.ISRC != "" {
		result, score, err := p.searchByISRC(ctx, token, track)
		if err == nil && result != nil {
			return result, score, nil
		}
	}

	// Fallback to name + artist search
	query := fmt.Sprintf("track:%s artist:%s", track.Name, track.Artist)
	endpoint := fmt.Sprintf("%s/search?type=track&limit=5&q=%s", baseURL, url.QueryEscape(query))

	body, err := p.doGet(ctx, token, endpoint)
	if err != nil {
		return nil, 0, fmt.Errorf("spotify: search failed: %w", err)
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("spotify: failed to parse search response: %w", err)
	}

	if len(resp.Tracks.Items) == 0 {
		return nil, 0, nil
	}

	best := resp.Tracks.Items[0]
	matched := toTrack(best)
	score := calculateConfidence(track, matched)

	return &matched, score, nil
}

func (p *Provider) searchByISRC(ctx context.Context, token string, track domain.Track) (*domain.Track, float64, error) {
	query := fmt.Sprintf("isrc:%s", track.ISRC)
	endpoint := fmt.Sprintf("%s/search?type=track&limit=1&q=%s", baseURL, url.QueryEscape(query))

	body, err := p.doGet(ctx, token, endpoint)
	if err != nil {
		return nil, 0, err
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}

	if len(resp.Tracks.Items) == 0 {
		return nil, 0, nil
	}

	matched := toTrack(resp.Tracks.Items[0])
	return &matched, 1.0, nil // ISRC match is exact
}

func (p *Provider) CreatePlaylist(ctx context.Context, token string, name string, description string) (string, error) {
	// First, get the current user ID
	userBody, err := p.doGet(ctx, token, baseURL+"/me")
	if err != nil {
		return "", fmt.Errorf("spotify: failed to get current user: %w", err)
	}

	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(userBody, &user); err != nil {
		return "", fmt.Errorf("spotify: failed to parse user response: %w", err)
	}

	payload := map[string]interface{}{
		"name":        name,
		"description": description,
		"public":      false,
	}
	payloadBytes, _ := json.Marshal(payload)

	endpoint := fmt.Sprintf("%s/users/%s/playlists", baseURL, user.ID)
	body, err := p.doPost(ctx, token, endpoint, payloadBytes)
	if err != nil {
		return "", fmt.Errorf("spotify: failed to create playlist: %w", err)
	}

	var resp createPlaylistResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("spotify: failed to parse create playlist response: %w", err)
	}

	return resp.ID, nil
}

func (p *Provider) AddTracksToPlaylist(ctx context.Context, token string, playlistID string, trackIDs []string) error {
	// Spotify accepts up to 100 URIs per request
	for i := 0; i < len(trackIDs); i += maxBatch {
		end := i + maxBatch
		if end > len(trackIDs) {
			end = len(trackIDs)
		}

		uris := make([]string, 0, end-i)
		for _, id := range trackIDs[i:end] {
			uris = append(uris, fmt.Sprintf("spotify:track:%s", id))
		}

		payload := map[string]interface{}{
			"uris": uris,
		}
		payloadBytes, _ := json.Marshal(payload)

		endpoint := fmt.Sprintf("%s/playlists/%s/tracks", baseURL, playlistID)
		if _, err := p.doPost(ctx, token, endpoint, payloadBytes); err != nil {
			return fmt.Errorf("spotify: failed to add tracks to playlist: %w", err)
		}
	}

	return nil
}

// -- HTTP helpers ------------------------------------------------------------

func (p *Provider) doGet(ctx context.Context, token string, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (p *Provider) doPost(ctx context.Context, token string, endpoint string, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// -- Helpers -----------------------------------------------------------------

func toTrack(t trackData) domain.Track {
	artists := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		artists = append(artists, a.Name)
	}

	return domain.Track{
		Name:       t.Name,
		Artist:     strings.Join(artists, ", "),
		Album:      t.Album.Name,
		ISRC:       t.ExternalIDs.ISRC,
		ExternalID: t.ID,
	}
}

func calculateConfidence(source, matched domain.Track) float64 {
	score := 0.0

	// ISRC match is the strongest signal
	if source.ISRC != "" && matched.ISRC != "" && strings.EqualFold(source.ISRC, matched.ISRC) {
		return 1.0
	}

	// Name comparison (case-insensitive)
	if strings.EqualFold(source.Name, matched.Name) {
		score += 0.5
	} else if strings.Contains(strings.ToLower(matched.Name), strings.ToLower(source.Name)) {
		score += 0.3
	}

	// Artist comparison
	if strings.EqualFold(source.Artist, matched.Artist) {
		score += 0.35
	} else if strings.Contains(strings.ToLower(matched.Artist), strings.ToLower(source.Artist)) {
		score += 0.2
	}

	// Album comparison
	if source.Album != "" && strings.EqualFold(source.Album, matched.Album) {
		score += 0.15
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}
