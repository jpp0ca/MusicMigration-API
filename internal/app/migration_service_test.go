package app

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jpp0ca/MusicMigration-API/internal/adapters"
	"github.com/jpp0ca/MusicMigration-API/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- Mock provider -----------------------------------------------------------

type mockProvider struct {
	name            string
	playlists       []domain.Playlist
	tracks          []domain.Track
	searchResults   map[string]*searchResult
	createdID       string
	addedTracks     []string
	mu              sync.Mutex
	searchCallCount int
}

type searchResult struct {
	track *domain.Track
	score float64
	err   error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) GetPlaylists(_ context.Context, _ string) ([]domain.Playlist, error) {
	return m.playlists, nil
}

func (m *mockProvider) GetPlaylistTracks(_ context.Context, _ string, _ string) ([]domain.Track, error) {
	return m.tracks, nil
}

func (m *mockProvider) SearchTrack(_ context.Context, _ string, track domain.Track) (*domain.Track, float64, error) {
	m.mu.Lock()
	m.searchCallCount++
	m.mu.Unlock()

	key := track.Name + "|" + track.Artist
	if result, ok := m.searchResults[key]; ok {
		return result.track, result.score, result.err
	}
	return nil, 0, nil
}

func (m *mockProvider) CreatePlaylist(_ context.Context, _ string, _ string, _ string) (string, error) {
	return m.createdID, nil
}

func (m *mockProvider) AddTracksToPlaylist(_ context.Context, _ string, _ string, trackIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addedTracks = append(m.addedTracks, trackIDs...)
	return nil
}

// -- Tests -------------------------------------------------------------------

func TestMigratePlaylist_AllMatched(t *testing.T) {
	sourceTracks := []domain.Track{
		{Name: "Bohemian Rhapsody", Artist: "Queen", ISRC: "GBUM71029604"},
		{Name: "Stairway to Heaven", Artist: "Led Zeppelin", ISRC: "USAT20700634"},
		{Name: "Hotel California", Artist: "Eagles", ISRC: "USEE10400237"},
	}

	source := &mockProvider{
		name:   "source",
		tracks: sourceTracks,
	}

	dest := &mockProvider{
		name:      "dest",
		createdID: "new-playlist-123",
		searchResults: map[string]*searchResult{
			"Bohemian Rhapsody|Queen": {
				track: &domain.Track{Name: "Bohemian Rhapsody", Artist: "Queen", ExternalID: "vid-1"},
				score: 0.95,
			},
			"Stairway to Heaven|Led Zeppelin": {
				track: &domain.Track{Name: "Stairway to Heaven", Artist: "Led Zeppelin", ExternalID: "vid-2"},
				score: 0.90,
			},
			"Hotel California|Eagles": {
				track: &domain.Track{Name: "Hotel California", Artist: "Eagles", ExternalID: "vid-3"},
				score: 0.88,
			},
		},
	}

	registry := adapters.NewProviderRegistry()
	registry.Register(source)
	registry.Register(dest)

	svc := NewService(registry, 3)
	result, err := svc.MigratePlaylist(context.Background(), domain.MigrationRequest{
		SourceProvider: "source",
		SourceToken:    "token-source",
		DestProvider:   "dest",
		DestToken:      "token-dest",
		PlaylistID:     "playlist-1",
	})

	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalTracks)
	assert.Equal(t, 3, result.MatchedTracks)
	assert.Equal(t, 0, result.FailedTracks)
	assert.Equal(t, "new-playlist-123", result.DestPlaylistID)
	assert.Len(t, dest.addedTracks, 3)
}

func TestMigratePlaylist_PartialMatch(t *testing.T) {
	sourceTracks := []domain.Track{
		{Name: "Track A", Artist: "Artist A"},
		{Name: "Track B", Artist: "Artist B"},
		{Name: "Track C", Artist: "Artist C"},
	}

	source := &mockProvider{
		name:   "source",
		tracks: sourceTracks,
	}

	dest := &mockProvider{
		name:      "dest",
		createdID: "new-playlist-456",
		searchResults: map[string]*searchResult{
			"Track A|Artist A": {
				track: &domain.Track{Name: "Track A", Artist: "Artist A", ExternalID: "vid-a"},
				score: 0.85,
			},
			// Track B not found (not in map)
			"Track C|Artist C": {
				track: nil,
				score: 0,
				err:   fmt.Errorf("search quota exceeded"),
			},
		},
	}

	registry := adapters.NewProviderRegistry()
	registry.Register(source)
	registry.Register(dest)

	svc := NewService(registry, 2)
	result, err := svc.MigratePlaylist(context.Background(), domain.MigrationRequest{
		SourceProvider: "source",
		SourceToken:    "t1",
		DestProvider:   "dest",
		DestToken:      "t2",
		PlaylistID:     "pl-1",
	})

	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalTracks)
	assert.Equal(t, 1, result.MatchedTracks)
	assert.Equal(t, 2, result.FailedTracks)
	assert.Len(t, dest.addedTracks, 1)
}

func TestMigratePlaylist_EmptyPlaylist(t *testing.T) {
	source := &mockProvider{
		name:   "source",
		tracks: []domain.Track{},
	}

	dest := &mockProvider{name: "dest"}

	registry := adapters.NewProviderRegistry()
	registry.Register(source)
	registry.Register(dest)

	svc := NewService(registry, 2)
	_, err := svc.MigratePlaylist(context.Background(), domain.MigrationRequest{
		SourceProvider: "source",
		SourceToken:    "t1",
		DestProvider:   "dest",
		DestToken:      "t2",
		PlaylistID:     "pl-empty",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestMigratePlaylist_UnknownProvider(t *testing.T) {
	registry := adapters.NewProviderRegistry()
	svc := NewService(registry, 2)

	_, err := svc.MigratePlaylist(context.Background(), domain.MigrationRequest{
		SourceProvider: "unknown",
		SourceToken:    "t",
		DestProvider:   "dest",
		DestToken:      "t",
		PlaylistID:     "pl",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestMigratePlaylist_ConcurrencyWorkerCount(t *testing.T) {
	tracks := make([]domain.Track, 20)
	for i := range tracks {
		tracks[i] = domain.Track{
			Name:   fmt.Sprintf("Track %d", i),
			Artist: fmt.Sprintf("Artist %d", i),
		}
	}

	source := &mockProvider{name: "source", tracks: tracks}

	searchResults := make(map[string]*searchResult)
	for i := range tracks {
		key := fmt.Sprintf("Track %d|Artist %d", i, i)
		searchResults[key] = &searchResult{
			track: &domain.Track{
				Name:       fmt.Sprintf("Track %d", i),
				Artist:     fmt.Sprintf("Artist %d", i),
				ExternalID: fmt.Sprintf("vid-%d", i),
			},
			score: 0.9,
		}
	}

	dest := &mockProvider{
		name:          "dest",
		createdID:     "big-playlist",
		searchResults: searchResults,
	}

	registry := adapters.NewProviderRegistry()
	registry.Register(source)
	registry.Register(dest)

	svc := NewService(registry, 5)
	result, err := svc.MigratePlaylist(context.Background(), domain.MigrationRequest{
		SourceProvider: "source",
		SourceToken:    "t1",
		DestProvider:   "dest",
		DestToken:      "t2",
		PlaylistID:     "pl-big",
	})

	require.NoError(t, err)
	assert.Equal(t, 20, result.TotalTracks)
	assert.Equal(t, 20, result.MatchedTracks)
	assert.Equal(t, 0, result.FailedTracks)
	assert.Equal(t, 20, dest.searchCallCount)

	// Verify all tracks are present in the result (order may vary due to concurrency)
	statusCounts := map[domain.TrackStatus]int{}
	for _, tr := range result.TrackResults {
		statusCounts[tr.Status]++
	}
	assert.Equal(t, 20, statusCounts[domain.TrackStatusMatched])
}

func TestListPlaylists(t *testing.T) {
	provider := &mockProvider{
		name: "test",
		playlists: []domain.Playlist{
			{ID: "1", Name: "Playlist A", TrackCount: 10},
			{ID: "2", Name: "Playlist B", TrackCount: 5},
		},
	}

	registry := adapters.NewProviderRegistry()
	registry.Register(provider)

	svc := NewService(registry, 2)
	playlists, err := svc.ListPlaylists(context.Background(), "test", "token")

	require.NoError(t, err)
	assert.Len(t, playlists, 2)
	assert.Equal(t, "Playlist A", playlists[0].Name)
}
