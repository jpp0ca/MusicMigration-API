package adapters

import (
	"context"
	"testing"

	"github.com/jpp0ca/MusicMigration-API/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- Minimal mock for registry tests -----------------------------------------

type stubProvider struct {
	name string
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) GetPlaylists(_ context.Context, _ string) ([]domain.Playlist, error) {
	return nil, nil
}
func (s *stubProvider) GetPlaylistTracks(_ context.Context, _ string, _ string) ([]domain.Track, error) {
	return nil, nil
}
func (s *stubProvider) SearchTrack(_ context.Context, _ string, _ domain.Track) (*domain.Track, float64, error) {
	return nil, 0, nil
}
func (s *stubProvider) CreatePlaylist(_ context.Context, _ string, _ string, _ string) (string, error) {
	return "", nil
}
func (s *stubProvider) AddTracksToPlaylist(_ context.Context, _ string, _ string, _ []string) error {
	return nil
}

// -- Tests -------------------------------------------------------------------

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	registry := NewProviderRegistry()
	registry.Register(&stubProvider{name: "spotify"})
	registry.Register(&stubProvider{name: "youtube"})

	p, err := registry.Get("spotify")
	require.NoError(t, err)
	assert.Equal(t, "spotify", p.Name())

	p, err = registry.Get("youtube")
	require.NoError(t, err)
	assert.Equal(t, "youtube", p.Name())
}

func TestProviderRegistry_GetUnknown(t *testing.T) {
	registry := NewProviderRegistry()

	_, err := registry.Get("deezer")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestProviderRegistry_Available(t *testing.T) {
	registry := NewProviderRegistry()
	registry.Register(&stubProvider{name: "spotify"})
	registry.Register(&stubProvider{name: "youtube"})

	available := registry.Available()
	assert.Len(t, available, 2)
	assert.Contains(t, available, "spotify")
	assert.Contains(t, available, "youtube")
}

func TestProviderRegistry_OverwriteExisting(t *testing.T) {
	registry := NewProviderRegistry()
	registry.Register(&stubProvider{name: "spotify"})
	registry.Register(&stubProvider{name: "spotify"}) // re-register

	available := registry.Available()
	assert.Len(t, available, 1)
}
