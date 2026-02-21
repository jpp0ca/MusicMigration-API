package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jpp0ca/MusicMigration-API/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- Mock service ------------------------------------------------------------

type mockMigrationService struct {
	playlists       []domain.Playlist
	migrationResult *domain.MigrationResult
	err             error
}

func (m *mockMigrationService) ListPlaylists(_ context.Context, _ string, _ string) ([]domain.Playlist, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.playlists, nil
}

func (m *mockMigrationService) MigratePlaylist(_ context.Context, _ domain.MigrationRequest) (*domain.MigrationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.migrationResult, nil
}

// -- Helpers -----------------------------------------------------------------

func setupRouter(svc *mockMigrationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(svc)
	h.RegisterRoutes(r)
	return r
}

// -- Tests -------------------------------------------------------------------

func TestHealth(t *testing.T) {
	r := setupRouter(&mockMigrationService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestListPlaylists_Success(t *testing.T) {
	svc := &mockMigrationService{
		playlists: []domain.Playlist{
			{ID: "1", Name: "Rock Classics", TrackCount: 25},
			{ID: "2", Name: "Jazz Vibes", TrackCount: 40},
		},
	}
	r := setupRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists?provider=spotify", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var playlists []domain.Playlist
	err := json.Unmarshal(w.Body.Bytes(), &playlists)
	require.NoError(t, err)
	assert.Len(t, playlists, 2)
}

func TestListPlaylists_MissingProvider(t *testing.T) {
	r := setupRouter(&mockMigrationService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListPlaylists_MissingToken(t *testing.T) {
	r := setupRouter(&mockMigrationService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists?provider=spotify", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMigratePlaylist_Success(t *testing.T) {
	svc := &mockMigrationService{
		migrationResult: &domain.MigrationResult{
			SourcePlaylist: "pl-1",
			DestPlaylistID: "new-pl",
			TotalTracks:    10,
			MatchedTracks:  8,
			FailedTracks:   2,
		},
	}
	r := setupRouter(svc)

	body := domain.MigrationRequest{
		SourceProvider: "spotify",
		SourceToken:    "token-s",
		DestProvider:   "youtube",
		DestToken:      "token-y",
		PlaylistID:     "pl-1",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result domain.MigrationResult
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, 8, result.MatchedTracks)
	assert.Equal(t, 2, result.FailedTracks)
}

func TestMigratePlaylist_InvalidBody(t *testing.T) {
	r := setupRouter(&mockMigrationService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
