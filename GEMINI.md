# CitadelOpsDesktop Project Overview

CitadelOpsDesktop is a cross-platform desktop automation tool and dashboard designed for the game "Goodgame Empire". It features a Go backend for game interaction and a React-based frontend for a modern user interface.

## Core Technologies
- **Backend:** Go (1.25)
- **Frontend:** React (19), TypeScript, Vite, TailwindCSS (4)
- **Game Interaction:** `chromedp` for browser-based login interception and `gorilla/websocket` for game server communication.
- **Embedded Assets:** Frontend assets are built and embedded into the Go binary using `go:embed`.

## Architecture

### Backend (Go)
The backend is organized into several packages under the `Server/` directory:
- **Core:** Handles core login logic and browser interception using `chromedp`.
- **ResponseRegistry:** Manages the persistent WebSocket connection and browser interaction using `chromedp`.
- **GameParser:** Processes incoming binary/text messages from the game server and updates the internal game state.
- **FrontendWebsocket:** Manages the WebSocket hub for communicating with the React dashboard.
- **Models:** Defines the data structures for game state, equipment, and resources.
- **ReconfigureLoadout:** Contains optimization algorithms for commander and castellan equipment sets.
- **Version:** Handles background version checks and self-update functionality.

### Frontend (React)
Located in the `Client/` directory, the frontend is a modern SPA built with:
- **Vite:** For fast development and optimized builds.
- **TailwindCSS:** For utility-first styling.
- **Lucide React:** For consistent iconography.
- **WebSocket:** Communicates with the Go backend for real-time updates and command execution.

## Building and Running

### Development
1.  **Frontend:**
    ```bash
    cd Client
    npm install
    npm run dev
    ```
2.  **Backend:**
    ```bash
    go mod tidy
    go run main.go
    ```
    *Note: The backend expects `Client/dist` to exist for embedding. Run `npm run build` in the `Client` directory first.*

### Production Build
The project uses a multi-stage `Dockerfile` to build binaries for Windows (x64), macOS (Intel), and macOS (Apple Silicon).
```bash
docker build -t citadel-ops-desktop .
```
To build locally for your current platform:
1.  **Build Frontend:** `cd Client && npm run build`
2.  **Build Backend:** `go build -o CitadelDesktop.exe main.go`

## Key Workflows

### Login Process
1.  The app checks for `loginBytes.json`.
2.  If missing or expired, it launches a visible browser via `chromedp`.
3.  The user logs in manually on the game's website.
4.  The backend intercepts the WebSocket frames, extracts credentials (RCT, name, etc.), and saves them for reuse.

### Automation & Optimization
- **AutoBird:** Automated troop management and attack/defense monitoring.
- **Equipment Optimizer:** Calculates the best equipment combinations based on user-defined scoring and targets.
- **Asset Management:** Automated selling of non-relic or lower-tier relic equipment and gems.

## Development Conventions
- **Naming:** Follow standard Go idioms for backend and React/TS conventions for frontend.
- **WebSocket Protocol:** Communication between frontend and backend uses JSON objects with a `type` field for dispatching.
- **Error Handling:** Use the `Alerts` system (via WebSocket) to notify the user of errors or successes in the dashboard.
