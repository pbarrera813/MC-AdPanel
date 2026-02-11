# Minecraft Admin Panel v1.0

A self-hosted web panel for managing multiple Minecraft Java servers from a single interface. Create, start, stop, monitor, and configure servers — all from your browser.

Built with a **Go** backend and a **React** frontend, the panel runs on any Linux machine with Java and manages real Minecraft server processes directly. Designed for deployment via **Docker** on CasaOS or any Docker-capable host.

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)

## Motivation

MC AdPanel was born out of a simple need: managing Minecraft servers shouldn't be complicated. Existing solutions often require extensive configuration, CLI knowledge, or come with bloated feature sets that get in the way of what you actually want to do — run a few Minecraft servers for yourself and your friends.

The goal was to build a **plug and play** experience. Install the Docker container, open your browser, and start creating servers. No config files to edit by hand, no terminal commands to memorize, no reverse proxies to set up. Just a clean web interface where you can:

- Pick a server type and version from a dropdown
- Click "Create" and the panel downloads everything for you
- Hit "Start" and you're playing (Remember to open the appropiate ports on your firewall, some people forget to do that!).

Everything else — backups, plugins, file editing, monitoring — is right there in the same interface, designed to stay out of your way until you need it. One container, one port, zero friction.

## Features

### Server Management
- **Multi-server management** — Run multiple Minecraft servers simultaneously, each with its own port, type, and version
- **9 server types supported** — Paper, Spigot, Purpur, Folia, Fabric, Forge, NeoForge, Velocity, Waterfall
- **Automatic jar download** — Fetches the correct server jar from upstream APIs when you create a server (no manual downloads)
- **Dynamic version fetching** — Versions are fetched live from upstream APIs with a 15-minute in-memory cache
- **Multi-select and batch operations** — Select multiple servers for batch deletion; running servers are blocked from deletion with a clear warning
- **Inline rename** — Click on a selected server's name to rename it directly
- **Delete server** — Permanently remove a server including all its files and backups, with confirmation safeguard
- **Server cloning** — Clone an existing server with options to copy worlds, plugins, and configs; multi-select sources to batch clone with auto-naming and auto-incrementing ports
- **Scheduled restarts** — Schedule a server restart after a configurable delay (5m, 30m, 1h, 3h, 6h, or custom time)
- **Auto-start** — Per-server toggle to automatically start servers when the panel boots
- **Safe mode** — Start a server with plugins/mods disabled (renames directories, restores them on stop)

### JVM Flags
- **Preset selection** — Choose from Aikar's Flags (optimized GC for game servers), Velocity & Waterfall (optimized for proxies), or no flags if you are just built like that you know.
- **AlwaysPreTouch toggle** — Optionally enable `-XX:+AlwaysPreTouch` (pre-allocates memory at startup, useful if you want to remember how much ram the servers are using.)
- **Configurable per-server** — Set flags at creation time or change them later from the server card
- **Forge/NeoForge support** — Flags are written to `user_jvm_args.txt` for servers using `run.sh`

### Monitoring & Console
- **Real-time console** — WebSocket-based live console with ANSI color rendering and command input.
- **TPS monitoring** — Live TPS (Ticks Per Second) display with color-coded indicator and progress bar (green >=18, yellow >=15, red <15).
- **CPU usage graph** — Live area chart normalized to total system CPU (0-100%).
- **RAM usage graph** — Real-time memory consumption tracking.
- **Player tracking** — Online player list parsed from server logs with periodic `list` command verification to clean stale entries.
- **Crash reports** — View, copy, download, and delete crash report files with multi-select for batch deletion.

### Plugin & Mod Management
- **Smart directory detection** — The backend automatically reads from the correct directory: `plugins/` for plugin-based servers (Paper, Spigot, Purpur, Folia, Velocity, Waterfall) and `mods/` for modded servers (Forge, Fabric, NeoForge).
- **Dynamic labeling** — The interface automatically says "Plugins" for plugin-based servers and "Mods" for modded servers throughout the entire page.
- **Plugin/mod list** — View all installed plugins or mods with name, version, file size, and enabled/disabled status.
- **Multi-select** — Click rows to select plugins/mods; dynamic action button adapts: "Check for updates" when nothing is selected, "Update selected" when some are selected, "Update all" when all are selected.
- **Enable / Disable** — Toggle plugins/mods without deleting (`.jar` / `.jar.disabled` rename)
- **Upload** — Drag-and-drop `.jar` upload with multi-file support.
- **Delete** — Remove with confirmation.
- **Update checking** — Check for outdated plugins/mods via Modrinth and Spiget APIs; update individually or batch update.
- **Version status badges** — Each plugin/mod shows its version status: Latest (green), Outdated (yellow), Incompatible (red), or Unknown.

### Backup System
- **Create backups** — One-click full server backup (`tar.gz` archive).
- **Restore backups** — Restore any backup to replace current server files (server must be stopped).
- **Download backups** — Download backup archives to your local machine.
- **Delete backups** — Remove old backups with confirmation; multi-select for batch deletion.
- **Scheduled backups** — Automatic backups on a recurring schedule: daily, weekly, monthly, every 6 months, or yearly.

### File Browser
- **Directory navigation** — Browse server files with breadcrumb path.
- **File editor** — Edit text-based config files (`.properties`, `.yml`, `.json`, `.toml`, `.cfg`, `.xml`, etc.) directly in the browser.
- **Rename files and folders** — Select a single file or folder and click the pencil icon to rename it.
- **Upload files** — Drag-and-drop or click to upload; newly uploaded files are highlighted with a "New!" indicator until you navigate away.
- **Download files** — Download single files directly or multiple files as a zip archive.
- **Create folders** — New directory creation.
- **Multi-select** — Select multiple files for batch download or deletion.
- **Delete** — Remove files and folders.
- **Path traversal protection** — Sandboxed to the server directory.

### Player Management
- **Live player list** — Shows all online players with avatar, IP, ping, session time, and current world.
- **Ping indicator** — Color-coded latency display (green <100ms, yellow 100-300ms, red >300ms).
- **Current world** — Shows which dimension each player is in (Overworld, Nether, The End).
- **Player search** — Filter players by name.
- **Kick / Ban / Kill** — Player moderation actions with one click.

### Logs
- **Live log viewer** — Real-time log streaming with level filtering (INFO, WARN, ERROR).
- **Robust log parsing** — Strips ANSI and Minecraft color codes, detects all log levels (INFO, WARN/WARNING, ERROR/FATAL/SEVERE).
- **Search logs** — Full-text search across log output.
- **Pause / Resume** — Freeze the log stream for inspection.

### System Settings
- **User-Agent configuration** — Set a custom User-Agent string used for all upstream API requests and downloads (hover tooltip warns non-technical users to leave it alone).
- **Default RAM allocation** — Configure default min/max RAM for new servers (saves time when creating many servers).
- **Default JVM flags preset** — Pre-select a JVM flags preset (None, Aikar's Flags, Velocity & Waterfall) for new servers.
- **Status polling interval** — Configure how often the panel polls for server status updates (1-30 seconds).
- **Persistent settings** — All settings are stored in `/AdPanel/data/settings.json` and survive restarts.

### Notifications
- **Toast notifications** — Contextual success, error, warning, and info messages with circular close buttons.
- **Color-matched close buttons** — Close buttons inside each toast match the notification type color (green for success, red for error, yellow for warning, blue for info).

### Security
- **Non-root execution** — The panel and all Minecraft servers run as a dedicated `mcpanel` user, not root (this is really important).
- **Privilege drop** — Entrypoint uses `gosu` to fix volume ownership then drops to unprivileged user.
- **Path traversal protection** — File browser is sandboxed to each server's directory.
- **Filename sanitization** — Plugin and server names are sanitized to prevent directory escape.

### CasaOS Integration (or any docker based management app really)
- **App icon** — Displays in the CasaOS dashboard with custom icon.
- **Docker labels** — Configured for CasaOS app discovery (name, icon, port, scheme).
- **AppData volumes** — Follows the CasaOS convention at `/DATA/AppData/minecraft-adpanel/`
- **One-click access** — Open the panel directly from CasaOS UI.

## Architecture

```
+-----------------------------------------+
|          Browser (React SPA)            |
|   Vite + Tailwind CSS 4 + shadcn/ui    |
+----------------+------------------------+
                 | HTTP / WebSocket
+----------------v------------------------+
|         Go HTTP Server (:4010)          |
|   REST API + Static file serving        |
+----------------+------------------------+
                 | os/exec stdin/stdout
+----------------v------------------------+
|       Minecraft Server Processes        |
|   java -jar server.jar (per server)     |
+-----------------------------------------+
```

- The Go backend starts on port **4010** and serves both the API and the compiled React frontend
- Each Minecraft server runs as a child process managed via `os/exec` with stdin/stdout piping
- Console output is streamed to clients over WebSocket in real time with ANSI color support
- Server metadata is persisted in a JSON file (`/AdPanel/data/servers.json`)
- Backup scheduler runs as a background goroutine, checking every minute for due backups
- A player info poller periodically queries TPS, player list, ping, and dimension data via server commands
- Plugin/mod operations automatically target the correct directory (`plugins/` or `mods/`) based on server type

### Directory Structure

```
/AdPanel/
├── adpanel              # Go binary
├── dist/                # Compiled React frontend
│   └── icon.png         # Favicon
├── data/
│   ├── servers.json     # Server configurations & backup schedules
│   └── settings.json    # System settings (User-Agent, defaults, poll interval)
├── Servers/
│   ├── Survival/        # Server files (world, plugins/, server.jar, ...)
│   ├── Modded/          # Modded server files (world, mods/, server.jar, ...)
│   └── Creative/
└── Backups/
    ├── Survival/        # Backups for each server
    └── Creative/
```

## Getting Started

### Docker (recommended)

```bash
git clone <https://github.com/pbarrera813/MC-AdPanel>
cd MC-AdPanel
docker compose up -d
```

The panel will be available at `http://<your-server-ip>:4010`.

Docker uses **host networking** by default so Minecraft server ports (25565, etc.) are accessible directly. Persistent data is stored in three volumes:

| Volume | Container Path | Purpose |
|--------|---------------|---------|
| Servers | `/AdPanel/Servers` | Server files (worlds, plugins, mods, configs) |
| Data | `/AdPanel/data` | Panel configuration (`servers.json`, `settings.json`) |
| Backups | `/AdPanel/Backups` | Server backup archives |

All necessary directories are created automatically on first launch. If data already exists in the volumes, the panel detects it and preserves everything.

If you prefer bridge networking, edit `docker-compose.yml` — instructions are included as comments.

### CasaOS

1. Copy the repository to your CasaOS server
2. Build the image:
   ```bash
   cd MC-AdPanel
   docker compose build --no-cache
   ```
3. In CasaOS, go to **App Store > Custom Install**
4. Fill in:
   - **Docker Image:** `minecraft-adpanel:latest`
   - **Network:** Host
   - **Restart Policy:** `unless-stopped`
   - **Volumes:**
   - **Icon URL:** `https://i.imgur.com/PPQD6NN.png`

     | Host Path | Container Path |
     |-----------|---------------|
     | `/DATA/AppData/minecraft-adpanel/servers` | `/AdPanel/Servers` |
     | `/DATA/AppData/minecraft-adpanel/data` | `/AdPanel/data` |
     | `/DATA/AppData/minecraft-adpanel/backups` | `/AdPanel/Backups` |

5. Click **Install**. The panel will be accessible on port `4010`.

### Updating

To update to a new version (if i ever release one):

```
git clone <https://github.com/pbarrera813/MC-AdPanel>
cd MC-AdPanel
docker compose up -d
```

Your server data, configuration, and backups are preserved in the mounted volumes.

### Manual Setup

**Requirements:**
- Linux (the backend uses Linux-specific tools like `tar` for backups)
- Go 1.22+
- Node.js 20+
- Java 17+ (21 recommended)
- git (required for Spigot BuildTools)

**Build the frontend:**

```bash
npm install
npm run build
```

**Build the backend:**

```bash
cd backend
go build -o adpanel .
```

**Run:**

```bash
export ADPANEL_DIR=/AdPanel
mkdir -p $ADPANEL_DIR/Servers $ADPANEL_DIR/data $ADPANEL_DIR/Backups $ADPANEL_DIR/dist
cp -r dist/* $ADPANEL_DIR/dist/
cp backend/adpanel $ADPANEL_DIR/
cd $ADPANEL_DIR
./adpanel
```

Open `http://localhost:4010` in your browser.

## Supported Server Types

| Type | Source | Download Method |
|------|--------|----------------|
| **Paper** | [PaperMC API](https://api.papermc.io) | Direct jar download |
| **Folia** | [PaperMC API](https://api.papermc.io) | Direct jar download |
| **Velocity** | [PaperMC API](https://api.papermc.io) | Direct jar download (proxy) |
| **Waterfall** | [PaperMC API](https://api.papermc.io) | Direct jar download (proxy) |
| **Purpur** | [Purpur API](https://api.purpurmc.org) | Direct jar download |
| **Fabric** | [Fabric Meta](https://meta.fabricmc.net) | Server jar from loader + installer endpoint |
| **Forge** | [MinecraftForge](https://files.minecraftforge.net) | Installer jar + `--installServer` |
| **NeoForge** | [NeoForged Maven](https://maven.neoforged.net) | Installer jar + `--installServer` |
| **Spigot** | [SpigotMC BuildTools](https://hub.spigotmc.org) | BuildTools compilation (~10 min) |

Versions are fetched live from these APIs with a 15-minute in-memory cache.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `ADPANEL_DIR` | `/AdPanel` | Base directory for all panel data |
| `ADPANEL_USER_AGENT` | _(none)_ | Override User-Agent for upstream API requests (can also be set in System Settings UI) |

Each Minecraft server's port is configured at creation time. The default port is `25565` — increment for additional servers (25566, 25567, ...).

Additional settings (default RAM, JVM flags, polling interval) can be configured from the System Settings page in the UI and are persisted to `/AdPanel/data/settings.json`.

## API Reference

All endpoints are under `/api`. The panel web UI is served at `/`.

### Servers
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers` | List all servers |
| `POST` | `/api/servers` | Create a new server |
| `DELETE` | `/api/servers/{id}` | Delete a server permanently |
| `PUT` | `/api/servers/{id}/name` | Rename a server |
| `POST` | `/api/servers/{id}/start` | Start a server |
| `POST` | `/api/servers/{id}/start-safe` | Start in safe mode |
| `POST` | `/api/servers/{id}/stop` | Stop a server |
| `GET` | `/api/servers/{id}/status` | Get server status and metrics |
| `PUT` | `/api/servers/{id}/settings` | Update RAM and player settings |
| `PUT` | `/api/servers/{id}/auto-start` | Toggle auto-start |
| `PUT` | `/api/servers/{id}/flags` | Update JVM flags preset |
| `POST` | `/api/servers/{id}/schedule-restart` | Schedule a restart |
| `DELETE` | `/api/servers/{id}/schedule-restart` | Cancel scheduled restart |
| `POST` | `/api/servers/{id}/retry-install` | Retry failed installation |
| `POST` | `/api/servers/clone` | Clone a server |

### Versions
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/versions/{type}` | Get available versions for a server type |

### Console
| Method | Endpoint | Description |
|--------|----------|-------------|
| `WS` | `/api/logs/{id}` | WebSocket for real-time console logs |

### Plugins / Mods
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/plugins` | List plugins or mods (auto-detects `plugins/` or `mods/` directory) |
| `POST` | `/api/servers/{id}/plugins` | Upload a plugin/mod jar |
| `DELETE` | `/api/servers/{id}/plugins/{name}` | Delete a plugin/mod |
| `PUT` | `/api/servers/{id}/plugins/{name}/toggle` | Enable/disable a plugin/mod |
| `GET` | `/api/servers/{id}/plugins/check-updates` | Check for available updates (Modrinth + Spiget) |
| `POST` | `/api/servers/{id}/plugins/{name}/update` | Update a plugin/mod to latest version |

### Settings
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/settings` | Get system settings |
| `PUT` | `/api/settings` | Update system settings |

### Backups
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/backups` | List backups |
| `POST` | `/api/servers/{id}/backups` | Create a backup |
| `DELETE` | `/api/servers/{id}/backups/{name}` | Delete a backup |
| `GET` | `/api/servers/{id}/backups/{name}/download` | Download a backup |
| `POST` | `/api/servers/{id}/backups/{name}/restore` | Restore a backup |
| `GET` | `/api/servers/{id}/backup-schedule` | Get backup schedule |
| `PUT` | `/api/servers/{id}/backup-schedule` | Set backup schedule |

### Files
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/servers/{id}/files?path=` | List directory contents |
| `GET` | `/api/servers/{id}/files/content?path=` | Read file content |
| `PUT` | `/api/servers/{id}/files/content` | Write file content |
| `POST` | `/api/servers/{id}/files/upload` | Upload a file |
| `DELETE` | `/api/servers/{id}/files?path=` | Delete a file or directory |
| `POST` | `/api/servers/{id}/files/mkdir` | Create a directory |
| `PUT` | `/api/servers/{id}/files/rename` | Rename a file or directory |
| `POST` | `/api/servers/{id}/files/download` | Download file(s) (single or zip) |

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
| `DELETE` | `/api/servers/{id}/crash-reports/{name}` | Delete a crash report |

## Tech Stack

**Backend:**
- Go 1.22+ with `net/http` (ServeMux pattern matching)
- gorilla/websocket for real-time console
- gopsutil for system metrics
- google/uuid for server IDs

**Frontend:**
- React 18 + TypeScript
- Vite 6 (build tool)
- Tailwind CSS 4
- shadcn/ui (Radix primitives)
- Lucide React (icons)
- Framer Motion (animations)
- Recharts (charts)
- Sonner (toast notifications)

**Runtime:**
- Java 21 (Eclipse Temurin JRE)
- Docker multi-stage build (Node > Go > Java)

## License

Copyright 2026 Pablo Barrera

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the “Software”), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.