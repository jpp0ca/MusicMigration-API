package app

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/jpp0ca/MusicMigration-API/internal/adapters"
	"github.com/jpp0ca/MusicMigration-API/internal/domain"
)

// Service implements ports.MigrationService using a worker pool pattern for
// concurrent track matching across streaming providers.
type Service struct {
	registry *adapters.ProviderRegistry
	workers  int
}

// NewService creates a new migration service with the given provider registry
// and number of concurrent workers for track matching.
func NewService(registry *adapters.ProviderRegistry, workers int) *Service {
	if workers < 1 {
		workers = 1
	}
	return &Service{
		registry: registry,
		workers:  workers,
	}
}

func (s *Service) ListPlaylists(ctx context.Context, provider string, token string) ([]domain.Playlist, error) {
	p, err := s.registry.Get(provider)
	if err != nil {
		return nil, err
	}
	return p.GetPlaylists(ctx, token)
}

func (s *Service) MigratePlaylist(ctx context.Context, req domain.MigrationRequest) (*domain.MigrationResult, error) {
	source, err := s.registry.Get(req.SourceProvider)
	if err != nil {
		return nil, fmt.Errorf("source provider error: %w", err)
	}

	dest, err := s.registry.Get(req.DestProvider)
	if err != nil {
		return nil, fmt.Errorf("destination provider error: %w", err)
	}

	// Step 1: Fetch tracks from source playlist
	log.Printf("[migration] fetching tracks from %s playlist %s", req.SourceProvider, req.PlaylistID)
	tracks, err := source.GetPlaylistTracks(ctx, req.SourceToken, req.PlaylistID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch source tracks: %w", err)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("source playlist is empty")
	}

	log.Printf("[migration] found %d tracks, starting migration to %s", len(tracks), req.DestProvider)

	// Step 2: Search for each track on destination using worker pool
	results := s.searchTracksParallel(ctx, dest, req.DestToken, tracks)

	// Step 3: Collect matched track IDs for batch insertion
	var matchedIDs []string
	matched := 0
	failed := 0

	for i := range results {
		if results[i].Status == domain.TrackStatusMatched && results[i].MatchedTrack != nil {
			matchedIDs = append(matchedIDs, results[i].MatchedTrack.ExternalID)
			matched++
		} else {
			failed++
		}
	}

	log.Printf("[migration] search complete: %d matched, %d failed", matched, failed)

	// Step 4: Create destination playlist
	playlistName := fmt.Sprintf("Migrated from %s", req.SourceProvider)
	destPlaylistID, err := dest.CreatePlaylist(
		ctx, req.DestToken, playlistName,
		fmt.Sprintf("Migrated %d/%d tracks", matched, len(tracks)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination playlist: %w", err)
	}

	log.Printf("[migration] created destination playlist: %s", destPlaylistID)

	// Step 5: Add matched tracks to the destination playlist
	if len(matchedIDs) > 0 {
		if err := dest.AddTracksToPlaylist(ctx, req.DestToken, destPlaylistID, matchedIDs); err != nil {
			return nil, fmt.Errorf("failed to add tracks to destination playlist: %w", err)
		}
	}

	log.Printf("[migration] migration complete")

	return &domain.MigrationResult{
		SourcePlaylist: req.PlaylistID,
		DestPlaylistID: destPlaylistID,
		TotalTracks:    len(tracks),
		MatchedTracks:  matched,
		FailedTracks:   failed,
		TrackResults:   results,
	}, nil
}

// searchTracksParallel uses a worker pool to search for tracks concurrently
// on the destination provider. The number of concurrent workers is controlled
// by s.workers to respect API rate limits.
func (s *Service) searchTracksParallel(
	ctx context.Context,
	dest interface {
		SearchTrack(ctx context.Context, token string, track domain.Track) (*domain.Track, float64, error)
	},
	token string,
	tracks []domain.Track,
) []domain.TrackResult {

	type indexedResult struct {
		index  int
		result domain.TrackResult
	}

	trackCh := make(chan struct {
		index int
		track domain.Track
	}, len(tracks))

	resultCh := make(chan indexedResult, len(tracks))

	// Launch worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for item := range trackCh {
				select {
				case <-ctx.Done():
					resultCh <- indexedResult{
						index: item.index,
						result: domain.TrackResult{
							SourceTrack: item.track,
							Status:      domain.TrackStatusError,
							Error:       "context cancelled",
						},
					}
					continue
				default:
				}

				matched, score, err := dest.SearchTrack(ctx, token, item.track)
				tr := domain.TrackResult{
					SourceTrack: item.track,
				}

				if err != nil {
					tr.Status = domain.TrackStatusError
					tr.Error = err.Error()
					log.Printf("[worker-%d] error searching '%s - %s': %v",
						workerID, item.track.Artist, item.track.Name, err)
				} else if matched == nil {
					tr.Status = domain.TrackStatusNotFound
					log.Printf("[worker-%d] not found: '%s - %s'",
						workerID, item.track.Artist, item.track.Name)
				} else {
					tr.Status = domain.TrackStatusMatched
					tr.MatchedTrack = matched
					tr.ConfidenceScore = score
					log.Printf("[worker-%d] matched: '%s - %s' -> '%s' (score: %.2f)",
						workerID, item.track.Artist, item.track.Name, matched.ExternalID, score)
				}

				resultCh <- indexedResult{index: item.index, result: tr}
			}
		}(i)
	}

	// Send tracks to worker pool
	for i, track := range tracks {
		trackCh <- struct {
			index int
			track domain.Track
		}{index: i, track: track}
	}
	close(trackCh)

	// Wait for all workers to finish, then close results channel
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results preserving original order
	results := make([]domain.TrackResult, len(tracks))
	for ir := range resultCh {
		results[ir.index] = ir.result
	}

	return results
}
