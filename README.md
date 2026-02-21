# MusicMigration-API

Go API to transfer playlists between streaming services (Spotify, YouTube Music) using hexagonal architecture and Go native concurrency.

## Architecture

```
cmd/api/                          -- Entrypoint
internal/
  domain/                         -- Pure models (Track, Playlist, etc.)
  ports/                          -- Interfaces (MusicProvider, MigrationService)
  app/                            -- Application logic (worker pool)
  adapters/
    spotify/                      -- Spotify Web API Adapter
    youtube/                      -- YouTube Data API v3 Adapter
    http/                         -- HTTP Handler (Gin)
  config/                         -- Configuration via .env
```

## Features

- **ISRC matching** -- uses ISRC code for precise matching between platforms
- **Confidence score** -- each track receives a score from 0 to 1 indicating match quality
- **Worker pool** -- configurable goroutines for parallel search (respects rate limits)
- **Extensible** -- add new streaming service = implement `MusicProvider` interface

## Setup

```bash
# Clone and install dependencies
git clone https://github.com/jpp0ca/MusicMigration-API.git
cd MusicMigration-API
go mod tidy

# Configure environment variables
cp .env.example .env

# Run
go run ./cmd/api

# Tests
go test ./... -v
```

## Endpoints

| Method | Route | Description |
|--------|------|-----------|
| `GET` | `/health` | Health check |
| `GET` | `/api/v1/playlists?provider=spotify` | List playlists (requires `Authorization: Bearer <token>` header) |
| `POST` | `/api/v1/migrate` | Migrate playlist between providers |
| `GET` | `/swagger/index.html` | Swagger UI documentation |

### Migration example

```bash
curl -X POST http://localhost:8080/api/v1/migrate \
  -H "Content-Type: application/json" \
  -d '{
    "source_provider": "spotify",
    "source_token": "your_spotify_token",
    "dest_provider": "youtube",
    "dest_token": "your_youtube_token",
    "playlist_id": "37i9dQZF1DXcBWIGoYBM5M"
  }'
```

## Configuration (.env)

| Variable | Default | Description |
|----------|--------|-----------|
| `PORT` | `8080` | Server port |
| `MIGRATION_WORKERS` | `5` | Goroutines in worker pool |
| `LOG_LEVEL` | `info` | Log level |

---

## Getting access tokens

### Spotify

1. Go to the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) and log in with your Spotify account
2. Click on **Create App**, fill in the fields and add `http://localhost:8888/callback` as a Redirect URI
3. In **Settings**, copy the **Client ID** and **Client Secret**
4. Generate the token via [Authorization Code Flow](https://developer.spotify.com/documentation/web-api/tutorials/code-flow) with the following scopes:

```
playlist-read-private
playlist-read-collaborative
playlist-modify-private
playlist-modify-public
```

> In **Development Mode**, the app accesses up to **25 test users** registered in the Dashboard.

---

### YouTube (Google)

1. Go to the [Google Cloud Console](https://console.cloud.google.com/) and create a new project
2. Go to **APIs & Services > Library** and enable **YouTube Data API v3**
3. Go to **APIs & Services > Credentials > Create Credentials > OAuth 2.0 Client IDs**
   - Type: **Web application**
   - Authorized redirect URI: `http://localhost:8888/callback`
4. Copy the **Client ID** and **Client Secret**
5. Use the [Google OAuth 2.0 Playground](https://developers.google.com/oauthplayground/) to generate the token:
   - In **OAuth 2.0 Configuration**, fill in your Client ID and Secret
   - Select the scope `https://www.googleapis.com/auth/youtube`
   - Click on **Authorize APIs** and then on **Exchange authorization code for tokens**
   - Copy the **Access token**

> The YouTube Data API v3 has a quota of **10,000 units/day** on the free tier. Each song search costs 100 units -- for large playlists, adjust `MIGRATION_WORKERS` carefully so you don't exceed the quota.
