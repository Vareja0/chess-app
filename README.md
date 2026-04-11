# Chess App

A real-time multiplayer chess application with JWT authentication, built with Go.

## Features

- User registration and login with JWT tokens
- Automatic matchmaking queue
- Real-time chess gameplay over WebSocket
- 5-minute game timer per player
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
| GET | `/create` | ✓ | Create a private game room |

## WebSocket Message Protocol

```jsonc
// Client → Server
{ "type": "start" }
{ "type": "move", "from": "e2", "to": "e4", "promotion": "" }
{ "type": "resign" }

// Server → Client
{ "type": "joined", "color": "White", "fen": "..." }
{ "type": "move", "from": "e2", "to": "e4", "fen": "...", "turn": "Black" }
{ "type": "timer", "white_time": 300, "black_time": 298 }
{ "type": "game_over", "outcome": "checkmate", "method": "checkmate" }
```
