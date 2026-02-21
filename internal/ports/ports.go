package ports

import (
	"context"

	"github.com/jpp0ca/MusicMigration-API/internal/domain"
)

// MusicProvider defines the contract that every streaming service adapter must
// implement. This is the primary driven port of the hexagonal architecture.
type MusicProvider interface {
	// GetPlaylists returns all playlists accessible by the authenticated user.
	GetPlaylists(ctx context.Context, token string) ([]domain.Playlist, error)

	// GetPlaylistTracks returns all tracks in a specific playlist, handling
	// pagination internally.
	GetPlaylistTracks(ctx context.Context, token string, playlistID string) ([]domain.Track, error)

	// SearchTrack attempts to find a matching track on this provider.
	// Returns the matched track, a confidence score (0.0-1.0), and any error.
	SearchTrack(ctx context.Context, token string, track domain.Track) (*domain.Track, float64, error)

	// CreatePlaylist creates a new playlist and returns its ID.
	CreatePlaylist(ctx context.Context, token string, name string, description string) (string, error)

	// AddTracksToPlaylist adds the given tracks (by their external IDs) to a playlist.
	AddTracksToPlaylist(ctx context.Context, token string, playlistID string, trackIDs []string) error

	// Name returns the provider identifier (e.g., "spotify", "youtube").
	Name() string
}

// MigrationService defines the driving port for the core migration use case.
type MigrationService interface {
	// MigratePlaylist orchestrates the full migration of a playlist from one
	// provider to another, using concurrent workers for track matching.
	MigratePlaylist(ctx context.Context, req domain.MigrationRequest) (*domain.MigrationResult, error)

	// ListPlaylists returns playlists from a given provider for the authenticated user.
	ListPlaylists(ctx context.Context, provider string, token string) ([]domain.Playlist, error)
}
