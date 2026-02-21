# MusicMigration-API

API em Go para transferir playlists entre servicos de streaming (Spotify, YouTube Music), usando arquitetura hexagonal e concorrencia nativa do Go.

## Arquitetura

```
cmd/api/                          -- Entrypoint
internal/
  domain/                         -- Modelos puros (Track, Playlist, etc)
  ports/                          -- Interfaces (MusicProvider, MigrationService)
  app/                            -- Logica de aplicacao (worker pool)
  adapters/
    spotify/                      -- Adapter Spotify Web API
    youtube/                      -- Adapter YouTube Data API v3
    http/                         -- Handler HTTP (Gin)
  config/                         -- Configuracao via .env
```

## Features

- **ISRC matching** -- usa codigo ISRC para matching preciso entre plataformas
- **Confidence score** -- cada track recebe score de 0 a 1 indicando qualidade do match
- **Worker pool** -- goroutines configuraveis para busca paralela (respeita rate limits)
- **Extensivel** -- adicionar novo streaming = implementar interface `MusicProvider`

## Setup

```bash
# Clonar e instalar dependencias
git clone https://github.com/jpp0ca/MusicMigration-API.git
cd MusicMigration-API
go mod tidy

# Configurar variaveis de ambiente
cp .env.example .env

# Rodar
go run ./cmd/api

# Testes
go test ./... -v
```

## Endpoints

| Metodo | Rota | Descricao |
|--------|------|-----------|
| `GET` | `/health` | Health check |
| `GET` | `/api/v1/playlists?provider=spotify` | Lista playlists (requer header `Authorization: Bearer <token>`) |
| `POST` | `/api/v1/migrate` | Migra playlist entre providers |
| `GET` | `/swagger/index.html` | Documentacao Swagger UI |

### Exemplo de migracao

```bash
curl -X POST http://localhost:8080/api/v1/migrate \
  -H "Content-Type: application/json" \
  -d '{
    "source_provider": "spotify",
    "source_token": "seu_token_spotify",
    "dest_provider": "youtube",
    "dest_token": "seu_token_youtube",
    "playlist_id": "37i9dQZF1DXcBWIGoYBM5M"
  }'
```

## Configuracao (.env)

| Variavel | Padrao | Descricao |
|----------|--------|-----------|
| `PORT` | `8080` | Porta do servidor |
| `MIGRATION_WORKERS` | `5` | Goroutines no worker pool |
| `LOG_LEVEL` | `info` | Nivel de log |

---

## Obtendo os tokens de acesso

### Spotify

1. Acesse o [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) e faca login com sua conta Spotify
2. Clique em **Create App**, preencha os campos e adicione `http://localhost:8888/callback` como Redirect URI
3. Em **Settings**, copie o **Client ID** e o **Client Secret**
4. Gere o token via [Authorization Code Flow](https://developer.spotify.com/documentation/web-api/tutorials/code-flow) com os seguintes scopes:

```
playlist-read-private
playlist-read-collaborative
playlist-modify-private
playlist-modify-public
```

> Em **Development Mode**, o app acessa ate **25 usuarios de teste** cadastrados no Dashboard.

---

### YouTube (Google)

1. Acesse o [Google Cloud Console](https://console.cloud.google.com/) e crie um novo projeto
2. Va em **APIs & Services > Library** e habilite a **YouTube Data API v3**
3. Va em **APIs & Services > Credentials > Create Credentials > OAuth 2.0 Client IDs**
   - Tipo: **Web application**
   - Redirect URI autorizado: `http://localhost:8888/callback`
4. Copie o **Client ID** e o **Client Secret**
5. Use o [Google OAuth 2.0 Playground](https://developers.google.com/oauthplayground/) para gerar o token:
   - Em **OAuth 2.0 Configuration**, informe seu Client ID e Secret
   - Selecione o escopo `https://www.googleapis.com/auth/youtube`
   - Clique em **Authorize APIs** e depois em **Exchange authorization code for tokens**
   - Copie o **Access token**

> A YouTube Data API v3 tem cota de **10.000 unidades/dia** no nivel gratuito. Cada busca de musica custa 100 unidades -- para playlists grandes, ajuste `MIGRATION_WORKERS` com cuidado para nao estourar a cota.
