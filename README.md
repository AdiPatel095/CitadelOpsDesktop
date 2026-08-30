# CitadelOps Desktop

CitadelOps Desktop is a local dashboard and automation client for Goodgame
Empire. It runs an HTTP dashboard on your computer, launches a separate
app-managed Chromium profile, and keeps account state in a data directory that
you control.

This repository contains the application source and local developer tooling.
Cloud deployment, container, release-publishing, and production infrastructure
configuration are intentionally not included.

## Requirements

- Go 1.25 or newer
- Node.js 20.19 or newer, or Node.js 22.12 or newer
- npm
- A Chromium-compatible browser such as Chrome, Chromium, Edge, Brave,
  Vivaldi, Opera, or Arc

## Build from source

Clone the repository and install the locked frontend dependencies:

```sh
git clone https://github.com/AdiPatel095/CitadelOpsDesktop.git
cd CitadelOpsDesktop
npm ci --prefix Client
```

Build the frontend, then create a desktop binary with the frontend embedded:

```sh
npm run client:build
```

On Windows PowerShell:

```powershell
go build -tags desktop -o CitadelOpsDesktop.exe .
```

On macOS or Linux:

```sh
go build -tags desktop -o CitadelOpsDesktop .
```

The generated binary is self-contained except for the compatible browser used
to establish the game session.

Public source does not embed the official artifact-storage origin. Automatic
binary replacement therefore remains unavailable in an ordinary source build;
official distribution builds supply that trusted origin outside this
repository. Update downloads still require both an allowlisted HTTPS location
and a matching SHA-256 digest.

## Run the application

CitadelOps writes runtime state, its isolated browser profile, settings, and
history below its data directory. Set `CITADEL_DATA_DIR` to a location your
user account can write to. This avoids permission failures when the executable
is stored under a protected or read-only directory.

Windows PowerShell:

```powershell
$env:CITADEL_DATA_DIR = Join-Path $env:LOCALAPPDATA "CitadelOps"
New-Item -ItemType Directory -Force $env:CITADEL_DATA_DIR | Out-Null
.\CitadelOpsDesktop.exe
```

macOS or Linux:

```sh
mkdir -p "$HOME/.local/share/CitadelOps"
CITADEL_DATA_DIR="$HOME/.local/share/CitadelOps" ./CitadelOpsDesktop
```

At startup, the terminal prints the local dashboard address. The default is
`http://127.0.0.1:8080`; if that port is busy, CitadelOps chooses another local
port. The app also opens its own browser profile. Log in to Goodgame Empire in
that browser window rather than attempting to attach CitadelOps to your normal
browser profile.

If browser discovery does not find the browser you want, pass its executable
path explicitly:

```powershell
.\CitadelOpsDesktop.exe --browser-path "C:\Path\To\chrome.exe"
```

Use `CitadelOpsDesktop --help` to see optional local flags such as `--addr`,
`--offline`, `--browser`, and `--no-auto-start`.

Some features can issue actions to the connected game account. Review their
settings and enable only the automation you intend to use. Treat the selected
data directory as private: it can contain account state, settings, operation
history, and the app-managed browser profile.

## Local development

For the simplest source run, build the frontend once and run the Go server:

```sh
npm ci --prefix Client
npm run client:build
go run .
```

For frontend hot reload, run these in separate terminals:

```sh
go run .
```

```sh
npm run client:dev
```

Then open `http://127.0.0.1:41731`. Vite proxies API and WebSocket requests to
the Go server on port 8080.

## Verify a change

```sh
go test ./...
npm run client:build
npm run client:test
```

Generated binaries, `Client/dist`, `Client/node_modules`, local data,
credentials, browser profiles, and logs are excluded by `.gitignore`. Never
commit or share those local files.

For the application boundaries and package ownership model, see
[`Architecture.md`](Architecture.md). Feature-specific notes are under
[`Docs/`](Docs/).
