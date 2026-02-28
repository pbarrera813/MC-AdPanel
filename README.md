# Orexa Panel v1.0.1

Orexa Panel is a self-hosted web panel for creating, running, and managing multiple Minecraft Java servers from a single interface. It combines a Go backend with a React frontend and is designed to run well on Docker-based hosts such as CasaOS, as well as on standard Linux machines.

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)

## Overview

Orexa Panel focuses on the workflows server admins use most often:

- Create Minecraft servers from supported upstream providers.
- Start, stop, monitor, clone, and delete servers from the browser.
- Manage files, plugins, mods, backups, crash reports, and players without leaving the panel.
- Keep the panel lightweight enough for self-hosted environments.

## Features

### Server Management
- Multi-server management with separate ports, types, versions, RAM limits, and JVM presets.
- Supported server types: Vanilla, Paper, Spigot, Purpur, Folia, Fabric, Forge, NeoForge, and Velocity.
- Automatic jar or installer download from upstream providers when creating servers.
- Live version fetching with an in-memory cache.
- In-place forward version upgrades for stopped servers.
- Inline rename and batch delete from the Servers page.
- Server cloning with options to copy worlds, plugins/mods, and configs.
- Clone port defaults use the closest free port.
- Scheduled restart support with preset delays and custom times.
- Auto-start toggle per server.
- Safe mode startup that temporarily disables plugins or mods and restores them after stop.

### Console and Monitoring
- Real-time console over WebSocket with ANSI color rendering.
- Sequence-based console, so output stays current at all times
- Correct console reset after server reboot, followed by live streaming for the new run.
- Live TPS, CPU, and RAM monitoring in the management view.
- Player tracking uses `list` as the source of truth, with join/leave detection as triggers plus a low-frequency safety resync to avoid unnecessary console spam.

### Player Management
- Live online player list with avatar, IP, ping, session time, and dimension when supported by the server type.
- Search players by name.
- Kick, ban, and kill actions from the panel.

### File Browser
- Breadcrumb-based directory navigation.
- Folder-scoped search with match highlighting in the current directory.
- Built-in text editor for common config and text formats.
- Editor search with highlighted matches and next/previous navigation.
- File and folder rename support.
- Single-file, multi-file, and folder uploads.
- Upload progress modal with percentage feedback.
- Upload conflict prompt with `Replace` and `Skip` actions.
- Newly uploaded files are visually marked until navigation changes.
- File timestamps shown next to file size using the latest filesystem modification time.
- Single-file download or multi-file zip download.
- Multi-select delete and download actions.
- File operations are sandboxed to each server directory.

### Plugins and Mods
- Automatic directory targeting: `plugins/` for plugin-based servers and `mods/` for modded servers.
- Dynamic UI wording that switches between Plugins and Mods based on server type.
- Vanilla disables the Plugins / Mods page because it is not supported.
- Upload accepts `.jar` and `.JAR` files.
- Duplicate plugin/mod uploads are preserved with an automatic numeric suffix instead of overwriting the existing file.
- Enable and disable by renaming `.jar` and `.jar.disabled`.
- Update checking through Modrinth and Spigot APIs, with source-aware matching when a source URL is configured.
- Update status badges: Latest, Outdated, Incompatible, and Unknown.
- Safer update flow that validates replacements before swapping files.
- Optional source URL metadata is stored outside the server root in `/AdPanel/data/extension-sources/`.

### Backups and Logs
- Create full server backups as `.tar.gz` archives.
- Restore, download, and delete backups from the panel.
- Scheduled backups with recurring intervals.
- Live log viewer with search, level filtering, and pause/resume controls.
- Crash report listing, reading, copying, downloading, and deletion.

### System Settings
- Change panel login username and password.
- Configure the User-Agent used for upstream version and jar downloads.
- Configure default min/max RAM for newly created servers.
- Configure the default JVM flags preset for new servers.
- Configure the status polling interval.

### Authentication and Security
- Dedicated login screen for panel access.
- Default first-run credentials: `mcpanel / mcpanel`.
- Authentication sessions are stored in memory on the running panel instance.
- A logged-in browser stays authenticated across tab or browser restarts while the panel is still running.
- Restarting the panel or host clears active sessions and requires logging in again.
- Passwords are stored hashed in settings, not in plaintext.
- API routes are protected by authentication middleware.
- Login attempts are rate-limited per client IP.
- The panel and managed server processes run as a non-root `mcpanel` user in the container.

### Docker Deployment
- Docker-first deployment model.
- Host networking support for direct Minecraft port exposure.
- Docker labels and icon metadata included in `docker-compose.yml`.
- Persistent data layout under `/DATA/AppData/orexa-panel/` by default in the sample compose file.

## Architecture

```text
+-----------------------------------------+
|          Browser (React SPA)            |
|   Vite + Tailwind CSS 4 + shadcn/ui     |
+----------------+------------------------+
                 | HTTP / WebSocket
+----------------v------------------------+
|         Go HTTP Server (:4010)          |
|      REST API + static file serving     |
+----------------+------------------------+
                 | os/exec stdin/stdout
+----------------v------------------------+
|       Minecraft Server Processes        |
|     java -jar / installer workflows     |
+-----------------------------------------+
```

Key runtime details:

- The Go backend serves both the API and the built frontend on port `4010`.
- Each Minecraft server is managed as a child process.
- Console streaming uses a resumable sequence-based protocol.
- Panel sessions are in memory and are cleared on panel restart.
- Server metadata is stored in `/AdPanel/data/servers.json`.
- System settings are stored in `/AdPanel/data/settings.json`.
- Extension source metadata is stored in `/AdPanel/data/extension-sources/`.
- Backups are stored under `/AdPanel/Backups/<server-name>/`.

## Directory Layout

```text
/AdPanel/
|-- orexa-panel
|-- dist/
|   `-- ...frontend build output...
|-- data/
|   |-- servers.json
|   |-- settings.json
|   `-- extension-sources/
|-- Servers/
|   |-- <ServerName>/
|   `-- ...
`-- Backups/
    |-- <ServerName>/
    `-- ...
```

## Getting Started

### Docker

```bash
git clone https://github.com/pbarrera813/Orexa-Panel.git
cd Orexa-Panel
docker compose up -d --build
```

Default login on first run:

- Username: `mcpanel`
- Password: `mcpanel`

The backend also prints the default credentials to the container logs on first startup while they are still unchanged.

The included `docker-compose.yml` uses host networking by default so Minecraft ports are exposed directly on the host without per-port mappings.

Persistent data volumes in the sample compose file:

| Host Path | Container Path | Purpose |
|-----------|----------------|---------|
| `/DATA/AppData/orexa-panel/servers` | `/AdPanel/Servers` | Server files |
| `/DATA/AppData/orexa-panel/data` | `/AdPanel/data` | Panel configuration and metadata |
| `/DATA/AppData/orexa-panel/backups` | `/AdPanel/Backups` | Backup archives |

If you prefer bridge networking, edit `docker-compose.yml` and use explicit port mappings instead of `network_mode: host`.

### CasaOS

1. Copy the repository to the host.
2. Build the image:

```bash
cd Orexa-Panel
docker compose build --no-cache
```

3. In CasaOS, open Custom Install.
4. Use image `orexa-panel:latest`.
5. Use host networking.
6. Mount the same three volumes used in the sample compose file.
7. Open the panel on port `4010`.

### Manual Setup

Requirements:

- Linux
- Go 1.22+
- Node.js 20+
- Java 17+ for supported server runtimes and installers
- Git for Spigot BuildTools

Build frontend:

```bash
npm install
npm run build
```

Build backend:

```bash
cd backend
go build -o orexa-panel .
```

Run:

```bash
export ADPANEL_DIR=/AdPanel
mkdir -p "$ADPANEL_DIR/Servers" "$ADPANEL_DIR/data" "$ADPANEL_DIR/Backups" "$ADPANEL_DIR/dist"
cp -r dist/* "$ADPANEL_DIR/dist/"
cp backend/orexa-panel "$ADPANEL_DIR/"
cd "$ADPANEL_DIR"
./orexa-panel
```

Open `http://localhost:4010`.

## Updating

To update the panel while preserving your mounted data:

```bash
git pull
docker compose up -d --build
```

If you suspect stale Docker build cache issues, use:

```bash
docker compose build --no-cache
docker compose up -d
```

Notes:

- Mounted data in `/AdPanel/Servers`, `/AdPanel/data`, and `/AdPanel/Backups` is preserved.
- Orexa supports forward server version upgrades from the UI.
- In-place Minecraft version downgrades are not supported from the UI; use a backup restore or cloned fallback if you need to roll a server back.

## Supported Server Types

| Type | Source | Download Method |
|------|--------|-----------------|
| Vanilla | Mojang version manifest | Direct official server jar download |
| Paper | PaperMC API | Direct jar download |
| Spigot | Spigot BuildTools | BuildTools compilation |
| Purpur | Purpur API | Direct jar download |
| Folia | PaperMC API | Direct jar download |
| Fabric | Fabric Meta | Loader and installer workflow |
| Forge | MinecraftForge | Installer workflow |
| NeoForge | NeoForged Maven | Installer workflow |
| Velocity | PaperMC API | Direct jar download |

## Configuration

| Environment Variable | Default | Description |
|----------------------|---------|-------------|
| `ADPANEL_DIR` | `/AdPanel` | Base directory for panel data |
| `ADPANEL_USER_AGENT` | unset | Override User-Agent for upstream downloads |
| `ADPANEL_ALLOWED_ORIGINS` | unset | Comma-separated list of origins allowed by the CORS middleware |
| `ADPANEL_DEBUG_PLUGIN_UPDATES` | `0` | Enable verbose plugin/mod update diagnostics when set to `1` |

Most day-to-day configuration is managed from the System Settings page and stored in `/AdPanel/data/settings.json`.

## API Reference

All API routes are served under `/api`.

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Basic health response |
| `GET` | `/api/ready` | Readiness check for runtime dependencies |

### Auth

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/auth/login` | Authenticate and create a cookie-backed in-memory panel session |
| `POST` | `/api/auth/logout` | Destroy the current session |
| `GET` | `/api/auth/session` | Check whether the current browser session is authenticated |

### Servers

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers` | List all servers |
| `POST` | `/api/servers` | Create a server |
| `DELETE` | `/api/servers/{id}` | Delete a server |
| `PUT` | `/api/servers/{id}/name` | Rename a server |
| `POST` | `/api/servers/{id}/start` | Start a server |
| `POST` | `/api/servers/{id}/start-safe` | Start a server in safe mode |
| `POST` | `/api/servers/{id}/stop` | Stop a server |
| `GET` | `/api/servers/{id}/status` | Get runtime status and metrics |
| `PUT` | `/api/servers/{id}/version` | Update a stopped server to a newer version |
| `PUT` | `/api/servers/{id}/settings` | Update RAM and player-related settings |
| `PUT` | `/api/servers/{id}/auto-start` | Toggle auto-start |
| `PUT` | `/api/servers/{id}/flags` | Update JVM flags preset |
| `POST` | `/api/servers/{id}/schedule-restart` | Schedule a restart |
| `DELETE` | `/api/servers/{id}/schedule-restart` | Cancel a scheduled restart |
| `POST` | `/api/servers/{id}/retry-install` | Retry a failed installation |
| `POST` | `/api/servers/clone` | Clone a server |

### Versions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/versions/{type}` | List available versions for a server type |

### Console and Logs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `WS` | `/api/logs/{id}` | Live console stream with resumable `lastSeq` support |
| `GET` | `/api/servers/{id}/logs` | List saved log files |
| `GET` | `/api/servers/{id}/logs/{name}` | Read a saved log file |

### Plugins and Mods

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/plugins` | List installed plugins or mods |
| `POST` | `/api/servers/{id}/plugins` | Upload a plugin or mod jar |
| `DELETE` | `/api/servers/{id}/plugins/{name}` | Delete a plugin or mod |
| `PUT` | `/api/servers/{id}/plugins/{name}/toggle` | Enable or disable a plugin or mod |
| `PUT` | `/api/servers/{id}/plugins/{name}/source` | Set or update the source URL for update matching |
| `GET` | `/api/servers/{id}/plugins/check-updates` | Check for plugin or mod updates |
| `POST` | `/api/servers/{id}/plugins/{name}/update` | Update a plugin or mod |

### Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/settings` | Read system settings |
| `PUT` | `/api/settings` | Update system settings |

### Backups

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/backups` | List backups |
| `POST` | `/api/servers/{id}/backups` | Create a backup |
| `DELETE` | `/api/servers/{id}/backups/{name}` | Delete a backup |
| `GET` | `/api/servers/{id}/backups/{name}/download` | Download a backup |
| `POST` | `/api/servers/{id}/backups/{name}/restore` | Restore a backup |
| `GET` | `/api/servers/{id}/backup-schedule` | Read backup schedule |
| `PUT` | `/api/servers/{id}/backup-schedule` | Update backup schedule |

### Files

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/files?path=` | List directory contents |
| `GET` | `/api/servers/{id}/files/exists?path=` | Check whether a file exists |
| `GET` | `/api/servers/{id}/files/content?path=` | Read file content |
| `PUT` | `/api/servers/{id}/files/content` | Save file content |
| `POST` | `/api/servers/{id}/files/upload` | Upload files or folders |
| `DELETE` | `/api/servers/{id}/files?path=` | Delete a file or directory |
| `POST` | `/api/servers/{id}/files/mkdir` | Create a directory |
| `PUT` | `/api/servers/{id}/files/rename` | Rename a file or directory |
| `POST` | `/api/servers/{id}/files/download` | Download one or more files |

### Players

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/players` | List online players |
| `POST` | `/api/servers/{id}/players/{name}/kick` | Kick a player |
| `POST` | `/api/servers/{id}/players/{name}/ban` | Ban a player |
| `POST` | `/api/servers/{id}/players/{name}/kill` | Kill a player |

### Crash Reports

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/crash-reports` | List crash reports |
| `GET` | `/api/servers/{id}/crash-reports/{name}` | Read a crash report |
| `POST` | `/api/servers/{id}/crash-reports/{name}/copy` | Copy a crash report |
| `DELETE` | `/api/servers/{id}/crash-reports/{name}` | Delete a crash report |

## Tech Stack

Backend:
- Go 1.22+
- `net/http` ServeMux routing
- `gorilla/websocket`
- `gopsutil`
- `google/uuid`

Frontend:
- React 18
- TypeScript
- Vite 6
- Tailwind CSS 4
- shadcn/ui
- Framer Motion
- Recharts
- Sonner

Runtime:
- Java runtime for managed servers
- Docker multi-stage build for the packaged image

## License

Copyright 2026 Pablo Barrera

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

