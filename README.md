# Chess App

A real-time multiplayer chess application with JWT authentication, built with Go.

## Features

- User registration and login with JWT tokens
- Automatic matchmaking queue
- Real-time chess gameplay over WebSocket
- Time controls: bullet (1 min), blitz (3 min), rapid (5 min), classical (10 min)
- Abort before both sides have moved; automatic draws by threefold repetition and the fifty-move rule
- Game result persistence

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go, Gin |
| Real-time | Gorilla WebSocket |
| Chess engine | notnil/chess |
| Auth | JWT (HS256) + bcrypt |
| Database | PostgreSQL (users/sessions) |
| Cache / Queue | Redis (player state, matchmaking) |
| Containers | Docker, Docker Compose |
| Orchestration | Kubernetes + Helm |

## Project Structure

```
.
├── main.go                  # Entry point and routing
├── controllers/             # Request handlers (auth, game, websocket)
├── middleware/              # JWT auth middleware
├── models/                  # User and Session structs
├── initializers/            # DB, Redis, and env setup
├── utils/                   # Helpers
├── views/                   # HTML templates
├── public/                  # Static assets (CSS, JS)
├── chess-app/               # Helm chart for Kubernetes
│   ├── values.yaml          # Default config (placeholders for secrets)
│   ├── values-local.yaml    # Local overrides with real secrets (gitignored)
│   └── templates/           # K8s manifests
├── Dockerfile
└── docker-compose.yaml
```

## Running Locally

### Docker Compose

```bash
docker-compose up
```

Starts the Go app (port 3000), PostgreSQL, and Redis. The app reloads automatically on code changes via CompileDaemon.

### Without Docker

```bash
go mod download
go run main.go
```

## Deploying to Kubernetes

```bash
# Add Bitnami repo (required for Redis chart)
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Install chart dependencies
helm dependency update ./chess-app

# Deploy
helm install chess-app ./chess-app -f chess-app/values-local.yaml
```

Copy `chess-app/values.yaml` to `chess-app/values-local.yaml`, fill in the real credentials, and pass `-f chess-app/values-local.yaml` on every install/upgrade. This file is gitignored.

## API Routes

| Method | Route | Auth | Description |
|--------|-------|------|-------------|
| GET | `/` | — | Home page |
| GET | `/login` | — | Login page |
| GET | `/signup` | — | Signup page |
| POST | `/login` | — | Authenticate and receive tokens |
| POST | `/signup` | — | Create account |
| POST | `/refresh` | — | Refresh access token |
| POST | `/matchmaking` | ✓ | Join matchmaking queue |
| POST | `/matchmaking/cancel` | ✓ | Leave queue |
| GET | `/ws/:room` | ✓ | WebSocket connection for a game |

## WebSocket Message Protocol

```jsonc
// Client → Server
{ "type": "move", "from": "e2", "to": "e4", "promotion": "" }
{ "type": "resign" }
{ "type": "abort" }   // only before both sides have made their first move

// Server → Client
// clocks: { "white_ms": 180000, "black_ms": 180000, "running": "white" | "black" | null }
// Clocks start after both first moves; the client counts down locally between snapshots.
{ "type": "joined", "color": "white", "fen": "...", "moves": ["e2e4"], "turn": "black",
  "clocks": { ... }, "opponent_connected": true }
{ "type": "opponent_joined", "message": "..." }
{ "type": "move", "from": "e2", "to": "e4", "promotion": "", "fen": "...", "turn": "black", "clocks": { ... } }
{ "type": "game_over", "winner": "white" | "black" | null, "method": "checkmate", "clocks": { ... } }
{ "type": "error", "code": "not_your_turn" | "illegal_move" | "cannot_abort" | "bad_message", "message": "..." }
```

`method` is one of `checkmate`, `stalemate`, `insufficient_material`, `threefold_repetition`, `fifty_moves`, `resign`, `timeout`, `disconnect`, `aborted`.
