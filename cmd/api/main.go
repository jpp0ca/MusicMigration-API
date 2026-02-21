package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/jpp0ca/MusicMigration-API/internal/adapters"
	handler "github.com/jpp0ca/MusicMigration-API/internal/adapters/http"
	"github.com/jpp0ca/MusicMigration-API/internal/adapters/spotify"
	"github.com/jpp0ca/MusicMigration-API/internal/adapters/youtube"
	"github.com/jpp0ca/MusicMigration-API/internal/app"
	"github.com/jpp0ca/MusicMigration-API/internal/config"

	_ "github.com/jpp0ca/MusicMigration-API/docs"
)

// @title			MusicMigration API
// @version		1.0
// @description	API for transferring playlists between streaming services (Spotify, YouTube Music).
// @description	Supports concurrent track matching with configurable worker pools.

// @contact.name	MusicMigration API Support
// @license.name	MIT

// @host		localhost:8080
// @BasePath	/

// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token for the streaming provider (e.g. "Bearer your_token_here")
func main() {
	cfg := config.Load()

	// Create provider adapters
	httpClient := &http.Client{}
	spotifyProvider := spotify.NewProvider(httpClient)
	youtubeProvider := youtube.NewProvider(httpClient)

	// Register providers
	registry := adapters.NewProviderRegistry()
	registry.Register(spotifyProvider)
	registry.Register(youtubeProvider)

	// Create application service
	migrationService := app.NewService(registry, cfg.MigrationWorkers)

	// Setup HTTP server
	r := gin.Default()
	h := handler.NewHandler(migrationService)
	h.RegisterRoutes(r)

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	addr := ":" + cfg.Port
	log.Printf("Starting MusicMigration API on %s", addr)
	log.Printf("Workers: %d", cfg.MigrationWorkers)
	log.Printf("Registered providers: %v", registry.Available())
	log.Printf("Swagger UI: http://localhost%s/swagger/index.html", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
