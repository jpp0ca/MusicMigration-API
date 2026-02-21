package domain

// Track represents a music track with metadata used for cross-platform matching.
type Track struct {
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	ISRC       string `json:"isrc,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

// Playlist represents a collection of tracks from a streaming provider.
type Playlist struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	OwnerName   string  `json:"owner_name,omitempty"`
	TrackCount  int     `json:"track_count"`
	Tracks      []Track `json:"tracks,omitempty"`
}

// MigrationRequest contains all information needed to migrate a playlist
// from one streaming provider to another.
type MigrationRequest struct {
	SourceProvider string `json:"source_provider" binding:"required"`
	SourceToken    string `json:"source_token" binding:"required"`
	DestProvider   string `json:"dest_provider" binding:"required"`
	DestToken      string `json:"dest_token" binding:"required"`
	PlaylistID     string `json:"playlist_id" binding:"required"`
}

// TrackStatus describes the result of attempting to match a single track.
type TrackStatus string

const (
	TrackStatusMatched  TrackStatus = "matched"
	TrackStatusNotFound TrackStatus = "not_found"
	TrackStatusError    TrackStatus = "error"
)

// TrackResult holds the outcome of migrating a single track, including
// the confidence score of the match (0.0 to 1.0).
type TrackResult struct {
	SourceTrack     Track       `json:"source"`
	MatchedTrack    *Track      `json:"matched,omitempty"`
	Status          TrackStatus `json:"status"`
	ConfidenceScore float64     `json:"confidence_score"`
	Error           string      `json:"error,omitempty"`
}

// MigrationResult summarizes the outcome of a full playlist migration.
type MigrationResult struct {
	SourcePlaylist string        `json:"source_playlist"`
	DestPlaylistID string        `json:"dest_playlist_id"`
	TotalTracks    int           `json:"total_tracks"`
	MatchedTracks  int           `json:"matched_tracks"`
	FailedTracks   int           `json:"failed_tracks"`
	TrackResults   []TrackResult `json:"track_results"`
}
