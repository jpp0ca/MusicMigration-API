package youtube

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
	baseURL    = "https://www.googleapis.com/youtube/v3"
	maxResults = 50
)

// Provider implements ports.MusicProvider for YouTube using the Data API v3.
type Provider struct {
	client *http.Client
}

// NewProvider creates a new YouTube provider with the given HTTP client.
// If client is nil, http.DefaultClient is used.
func NewProvider(client *http.Client) *Provider {
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{client: client}
}

func (p *Provider) Name() string {
	return "youtube"
}

// -- API response types (internal) ------------------------------------------

type playlistListResponse struct {
	Items         []playlistResource `json:"items"`
	NextPageToken string             `json:"nextPageToken"`
}

type playlistResource struct {
	ID             string          `json:"id"`
	Snippet        playlistSnippet `json:"snippet"`
	ContentDetails struct {
		ItemCount int `json:"itemCount"`
	} `json:"contentDetails"`
}

type playlistSnippet struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	ChannelTitle string `json:"channelTitle"`
}

type playlistItemsResponse struct {
	Items         []playlistItemResource `json:"items"`
	NextPageToken string                 `json:"nextPageToken"`
}

type playlistItemResource struct {
	Snippet playlistItemSnippet `json:"snippet"`
}

type playlistItemSnippet struct {
	Title                  string     `json:"title"`
	VideoOwnerChannelTitle string     `json:"videoOwnerChannelTitle"`
	ResourceID             resourceID `json:"resourceId"`
}

type resourceID struct {
	VideoID string `json:"videoId"`
}

type searchListResponse struct {
	Items []searchResult `json:"items"`
}

type searchResult struct {
	ID      searchResultID `json:"id"`
	Snippet searchSnippet  `json:"snippet"`
}

type searchResultID struct {
	VideoID string `json:"videoId"`
}

type searchSnippet struct {
	Title        string `json:"title"`
	ChannelTitle string `json:"channelTitle"`
}

// -- MusicProvider implementation --------------------------------------------

func (p *Provider) GetPlaylists(ctx context.Context, token string) ([]domain.Playlist, error) {
	var playlists []domain.Playlist
	pageToken := ""

	for {
		endpoint := fmt.Sprintf(
			"%s/playlists?part=snippet,contentDetails&mine=true&maxResults=%d",
			baseURL, maxResults,
		)
		if pageToken != "" {
			endpoint += "&pageToken=" + pageToken
		}

		body, err := p.doGet(ctx, token, endpoint)
		if err != nil {
			return nil, fmt.Errorf("youtube: failed to get playlists: %w", err)
		}

		var resp playlistListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("youtube: failed to parse playlists response: %w", err)
		}

		for _, item := range resp.Items {
			playlists = append(playlists, domain.Playlist{
				ID:          item.ID,
				Name:        item.Snippet.Title,
				Description: item.Snippet.Description,
				OwnerName:   item.Snippet.ChannelTitle,
				TrackCount:  item.ContentDetails.ItemCount,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return playlists, nil
}

func (p *Provider) GetPlaylistTracks(ctx context.Context, token string, playlistID string) ([]domain.Track, error) {
	var tracks []domain.Track
	pageToken := ""

	for {
		endpoint := fmt.Sprintf(
			"%s/playlistItems?part=snippet&playlistId=%s&maxResults=%d",
			baseURL, playlistID, maxResults,
		)
		if pageToken != "" {
			endpoint += "&pageToken=" + pageToken
		}

		body, err := p.doGet(ctx, token, endpoint)
		if err != nil {
			return nil, fmt.Errorf("youtube: failed to get playlist items: %w", err)
		}

		var resp playlistItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("youtube: failed to parse playlist items response: %w", err)
		}

		for _, item := range resp.Items {
			if item.Snippet.ResourceID.VideoID == "" {
				continue
			}

			// YouTube playlist items only give us title and channel; we parse
			// the track name and artist from the video title heuristically.
			name, artist := parseVideoTitle(item.Snippet.Title)
			if name == "" {
				name = item.Snippet.Title
			}
			if artist == "" {
				artist = item.Snippet.VideoOwnerChannelTitle
			}

			tracks = append(tracks, domain.Track{
				Name:       name,
				Artist:     artist,
				ExternalID: item.Snippet.ResourceID.VideoID,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return tracks, nil
}

func (p *Provider) SearchTrack(ctx context.Context, token string, track domain.Track) (*domain.Track, float64, error) {
	query := fmt.Sprintf("%s %s", track.Name, track.Artist)
	endpoint := fmt.Sprintf(
		"%s/search?part=snippet&type=video&videoCategoryId=10&maxResults=5&q=%s",
		baseURL, url.QueryEscape(query),
	)

	body, err := p.doGet(ctx, token, endpoint)
	if err != nil {
		return nil, 0, fmt.Errorf("youtube: search failed: %w", err)
	}

	var resp searchListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("youtube: failed to parse search response: %w", err)
	}

	if len(resp.Items) == 0 {
		return nil, 0, nil
	}

	// Pick the best result based on title similarity
	best := resp.Items[0]
	matched := domain.Track{
		Name:       best.Snippet.Title,
		Artist:     best.Snippet.ChannelTitle,
		ExternalID: best.ID.VideoID,
	}

	score := calculateConfidence(track, matched)
	return &matched, score, nil
}

func (p *Provider) CreatePlaylist(ctx context.Context, token string, name string, description string) (string, error) {
	payload := map[string]interface{}{
		"snippet": map[string]string{
			"title":       name,
			"description": description,
		},
		"status": map[string]string{
			"privacyStatus": "private",
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	endpoint := fmt.Sprintf("%s/playlists?part=snippet,status", baseURL)
	body, err := p.doPost(ctx, token, endpoint, payloadBytes)
	if err != nil {
		return "", fmt.Errorf("youtube: failed to create playlist: %w", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("youtube: failed to parse create playlist response: %w", err)
	}

	return resp.ID, nil
}

func (p *Provider) AddTracksToPlaylist(ctx context.Context, token string, playlistID string, trackIDs []string) error {
	// YouTube requires adding one video at a time via playlistItems.insert
	for _, videoID := range trackIDs {
		payload := map[string]interface{}{
			"snippet": map[string]interface{}{
				"playlistId": playlistID,
				"resourceId": map[string]string{
					"kind":    "youtube#video",
					"videoId": videoID,
				},
			},
		}
		payloadBytes, _ := json.Marshal(payload)

		endpoint := fmt.Sprintf("%s/playlistItems?part=snippet", baseURL)
		if _, err := p.doPost(ctx, token, endpoint, payloadBytes); err != nil {
			return fmt.Errorf("youtube: failed to add video %s to playlist: %w", videoID, err)
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
		return nil, fmt.Errorf("youtube API returned status %d: %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("youtube API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// -- Helpers -----------------------------------------------------------------

// parseVideoTitle attempts to split a YouTube video title into track name and
// artist. Common formats: "Artist - Track", "Artist - Track (Official Video)".
func parseVideoTitle(title string) (name, artist string) {
	// Remove common suffixes
	suffixes := []string{
		"(Official Video)", "(Official Music Video)", "(Official Audio)",
		"(Lyric Video)", "(Lyrics)", "(Audio)", "[Official Video]",
		"[Official Music Video]", "[Official Audio]", "(HD)", "(HQ)",
	}
	cleaned := title
	for _, suffix := range suffixes {
		cleaned = strings.TrimSpace(strings.Replace(cleaned, suffix, "", 1))
	}

	// Split on " - " separator
	parts := strings.SplitN(cleaned, " - ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0])
	}

	return cleaned, ""
}

func calculateConfidence(source domain.Track, matched domain.Track) float64 {
	score := 0.0

	sourceName := strings.ToLower(source.Name)
	matchedTitle := strings.ToLower(matched.Name)

	// Check if the track name appears in the YouTube title
	if strings.Contains(matchedTitle, sourceName) {
		score += 0.5
	} else {
		// Check individual words overlap
		sourceWords := strings.Fields(sourceName)
		matchCount := 0
		for _, word := range sourceWords {
			if len(word) > 2 && strings.Contains(matchedTitle, word) {
				matchCount++
			}
		}
		if len(sourceWords) > 0 {
			score += 0.5 * float64(matchCount) / float64(len(sourceWords))
		}
	}

	// Check if the artist appears in the title or channel
	sourceArtist := strings.ToLower(source.Artist)
	matchedArtist := strings.ToLower(matched.Artist)
	if strings.Contains(matchedArtist, sourceArtist) || strings.Contains(matchedTitle, sourceArtist) {
		score += 0.4
	}

	// Bonus for exact title match
	if sourceName == matchedTitle {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}
