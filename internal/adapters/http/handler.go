package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jpp0ca/MusicMigration-API/internal/domain"
	"github.com/jpp0ca/MusicMigration-API/internal/ports"
)

// Handler holds the HTTP handlers for the migration API.
type Handler struct {
	service ports.MigrationService
}

// NewHandler creates a new HTTP handler with the given migration service.
func NewHandler(service ports.MigrationService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes sets up all API routes on the given Gin engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Health)

	api := r.Group("/api/v1")
	{
		api.GET("/playlists", h.ListPlaylists)
		api.POST("/migrate", h.MigratePlaylist)
	}
}

// Health returns a simple health check response.
//
//	@Summary		Health check
//	@Description	Returns the health status of the API
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/health [get]
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// ListPlaylists returns playlists for the given provider and authenticated user.
//
//	@Summary		List user playlists
//	@Description	Returns all playlists for the authenticated user on the specified streaming provider.
//	@Description	Supported providers: spotify, youtube.
//	@Tags			playlists
//	@Produce		json
//	@Param			provider	query		string	true	"Streaming provider"	Enums(spotify, youtube)
//	@Param			Authorization	header	string	true	"Bearer token for the streaming provider"
//	@Success		200	{array}		domain.Playlist
//	@Failure		400	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/v1/playlists [get]
func (h *Handler) ListPlaylists(c *gin.Context) {
	provider := c.Query("provider")
	if provider == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "bad_request",
			Message: "query parameter 'provider' is required",
		})
		return
	}

	token := extractToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Authorization header with Bearer token is required",
		})
		return
	}

	playlists, err := h.service.ListPlaylists(c.Request.Context(), provider, token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, playlists)
}

// MigratePlaylist initiates a playlist migration between two streaming providers.
//
//	@Summary		Migrate playlist
//	@Description	Transfers a playlist from one streaming provider to another using concurrent workers.
//	@Description	Fetches tracks from the source, matches them on the destination via ISRC or name+artist,
//	@Description	and creates a new playlist with the matched tracks. Returns detailed results with confidence scores.
//	@Tags			migration
//	@Accept			json
//	@Produce		json
//	@Param			request	body		domain.MigrationRequest	true	"Migration request with source/dest providers, tokens, and playlist ID"
//	@Success		200		{object}	domain.MigrationResult
//	@Failure		400		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/v1/migrate [post]
func (h *Handler) MigratePlaylist(c *gin.Context) {
	var req domain.MigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "bad_request",
			Message: "invalid request body: " + err.Error(),
		})
		return
	}

	result, err := h.service.MigratePlaylist(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "migration_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// extractToken retrieves the Bearer token from the Authorization header.
func extractToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return auth
}
