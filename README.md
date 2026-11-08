<h1><img src="public/icon.png" alt="Orexa Panel icon" width="34" /> Orexa Panel v1.1.0</h1>

Orexa Panel is a self-hosted Minecraft server panel built around one simple idea: plug and play
Just download it, open it, and start creating and managing servers from one place, without putting features behind a lot of diferent options or bloating you with unnecessary options, just download, run the docker command and start going crazy.

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)

## What Orexa Is

Orexa is designed for ease of use Minecraft administration with a simple workflow:

- Create and run multiple servers from the browser.
- Manage files, plugins/mods, logs, backups, and players without extra tools.
- Use safe defaults out of the box, with optional advanced tuning when needed.
- Keep core server management features open and accessible, with no paid feature gates.

## What's New in 1.1.0

### Java selection

Orexa chooses the Java runtime for each server: Java 25 for 26.x, Java 21 from Minecraft 1.20.5 onward, and Java 17 before that. When you create, update or import a server, the version picker shows which Java the selected version will run on, and warns you if that Java is not available in your install.

### Clearer errors

Failed installs and crashes now show a short title and a suggested fix on the server card, such as "Port 25565 is already in use" or "The server ran out of memory". The original error text is still available through the "Show details" button.

### Restart on crash

Each server card has a "Restart on crash" switch, off by default. When it is on, a crashed server is started again after a few seconds. After 3 crashes within 10 minutes Orexa stops retrying and shows a notice for you to review. Stopping or killing a server from the panel is never counted as a crash.

### Server list

With more than one server, the list gets a search box, a status filter (All, Running, Stopped, Problems) and a sort menu. Cards show the number of online players and the uptime. Drag-to-reorder works in the default view and is paused while a search or filter is active.

### Manage version

"Update version" is now "Manage version", and it lets you move a server to an older version as well as a newer one. Downgrading a regular server shows a warning that worlds saved on a newer version may not load on an older one, and you have to tick "I understand the risk" before it proceeds. Proxies have no world, so they skip the warning.

### Server jar detection

Upload a server `.jar` to the server folder under any name and Orexa will use it. The version on the card is read from the jar, and the Java runtime is chosen for that version. If the configured jar is deleted, Orexa falls back to the newest `.jar` in that server root folder.

### Velocity

Velocity is now correctly detected. A proxy's bind port and shown max players are read from `velocity.toml`. Changing them in the File Browser updates the card and the settings right away. A new proxy created on a custom port gets that port in its first `velocity.toml`.

### File editor

`Ctrl+S` saves without closing the editor, and closing it with unsaved changes asks for confirmation. Orexa keeps the previous five versions of each edited file, and the "Versions" menu loads one back into the editor.

### Console

The up and down arrow keys step through previously entered commands. History is stored per server.

### Other changes

- All pop-ups close with `Esc` for a better UX.
- The upload window highlights its drop area while you drag files over it.
- CPU and RAM usage are read directly from `/proc`, so the system-monitoring library is no longer a dependency.
- Unused code and old migration paths were removed.

## Quick Start

### Docker (recommended)

```bash
git clone https://github.com/pbarrera813/MC-AdPanel.git
cd MC-AdPanel
docker compose up -d --build
```

Open the panel at:

```text
http://<your-server-ip>:4010
```

Default first login:

- Username: `mcpanel`
- Password: `mcpanel`

If default credentials are still active, login is allowed but protected actions are gated until the password is changed.

### Manual Build (optional, really just run the docker command)

Requirements:

- Linux
- Go 1.22+
- Node.js 20+
- Docker image includes bundled JDK runtimes (17/21/25) with automatic selection per server type/version (Java 25 for Minecraft 26.x, Java 21 from 1.20.5, Java 17 before that) to avoid problems when creating servers.

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

## Main Features

### Server Management

- Multi-server lifecycle control: start, stop, kill, safe start, and delete.
- Supported server types: Vanilla, Paper, Spigot, Purpur, Folia, Fabric, Forge, NeoForge, and Velocity.
- Import existing servers from `.zip` or `.tar.gz` files with analyze/confirm flow and editable pre-import metadata.
- Clone servers with per-section options (worlds, plugins/mods, configs).
- Scheduled restart and scheduled stop.
- Auto-start toggle and retry install support.
- Optional "Restart on crash" per server (off by default, gives up after 3 crashes in 10 minutes).
- Manage version: update a server to a newer version or downgrade to an older one, with a world-safety warning on downgrade.
- Pre-flight Java check shown next to the version picker when creating, updating, or importing.
- Plain-language explanations for failed installs and crashes, with technical details one click away.
- Follows the server jar in the folder: upload a jar under any name and the version and Java follow it.
- Velocity-aware settings: bind port and shown max players stay in sync with `velocity.toml`.
- Search, status filter, and sort for the server list, plus live players and uptime on each card.
- Persistent custom server card ordering.
- Desktop right-click context menu for quick server actions.
- Mobile long-press drag reorder for server cards.

### Monitoring and Console

- Live console stream over WebSocket, which reconnects cleanly when you switch between servers.
- Command history with the up and down arrow keys, remembered per server.
- Console clears on new start after a prior stop, so each new run begins cleanly.
- Live server metrics with corrected host-share CPU and RAM percentages.
- System-wide usage endpoint and UI panel for panel + running managed servers.

### Players

- Live player list with name, world, session time, and actions.
- Per-player ping support when compatible plugin/mod support is present.
- Kick, ban, and kill actions directly from the panel.

### File Browser

- Folder browsing, search, upload, rename, edit, download, and delete.
- Editor safety: `Ctrl+S` saves without closing, unsaved changes are never discarded silently, and the last five saved versions of a file can be restored from the Versions menu.
- Uploading a `.jar` to the server root makes it the server jar; the card version updates to match.
- Route resets to server root when switching between servers.
- Rename input preselects file name portion before extension.
- Extension-change warning during rename to prevent accidental breakage.
- Conflict handling for uploads and duplicate destination names.
- Delete safeguard: confirmation plus 3-second undo notification.

### Plugins / Mods

- Auto-targets `plugins/` or `mods/` by server type.
- Upload, delete, enable/disable, source URL assignment, update checks, and updates.
- Duplicate install validation uses metadata and blocks true duplicates.
- User-facing duplicate message adapts to server type (plugin vs mod).
- Maximum upload size surfaced in the page UI.

### Backups and Logs

- Backup create, list, download, restore, and delete.
- Scheduled backups.
- Logs page behavior:
- Running server: live logs view.
- Stopped server: filesystem log files list.
- Crash report list/read/copy/download/delete.
- Delete safeguard with 3-second undo applies to logs, crash reports, and backups.

### System Settings UX Safeguards

- Unsaved changes warning in accent color with inline Save action.
- Overall Usage section with live totals and per-process details.
- Detailed View state persists when navigating away and back.
- Manage and Stop actions available from Overall Usage process list.

## Optional Advanced Configuration

Orexa works with defaults. If you need extra control, use environment variables:

| Environment Variable | Default | Description |
|---|---|---|
| `ADPANEL_DIR` | `/AdPanel` | Base path for panel data, servers, backups, and built assets. |
| `ADPANEL_ALLOWED_ORIGINS` | unset | Comma-separated allowed origins for CORS and WebSocket origin checks. |
| `ADPANEL_TRUSTED_PROXIES` | unset | Comma-separated trusted CIDRs/IPs for forwarded header handling. |
| `ADPANEL_CSRF_MODE` | `enforce` | CSRF policy for unsafe authenticated API methods (`enforce`, `report`, `off`). |
| `ADPANEL_MAX_UPLOAD_BYTES` | `268435456` | Max request size for file browser and plugin/mod uploads (256 MB). |
| `ADPANEL_MAX_SERVER_IMPORT_BYTES` | `8589934592` | Max request size for server import file uploads (8 GB). |
| `ADPANEL_PLUGIN_UPDATE_ALLOWED_HOSTS` | unset | Extra allowed hosts/domains for plugin/mod update downloads. |
| `ADPANEL_MAX_PLUGIN_UPDATE_BYTES` | `268435456` | Max download size for plugin/mod update fetches (256 MB). |
| `ADPANEL_USER_AGENT` | unset | Optional global User-Agent override for upstream fetches. |
| `ADPANEL_AUTO_FIX_HOSTS` | enabled | Set to `false` to disable startup hostname `/etc/hosts` auto-fix attempts on Linux. |

## Security Posture (Current)

- Passwords are stored hashed with Argon2id.
- Default credentials are detected and gated: session is marked `mustChangePassword`.
- Unsafe API calls are blocked until password change when the default credential state is active.
- CSRF protection validates same-origin requests for unsafe authenticated API methods.
- Forwarded headers are trusted only when the request comes from configured trusted proxies.
- Upload endpoints are size-capped and stream handling avoids unbounded memory reads.
- Plugin/mod update URLs are validated against host policy with private-address protections.
- Path containment checks and quarantine protections prevent unsafe server directory operations.

## API Reference

All endpoints are served under `/api`.

### Health

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/health` | Liveness check. |
| `GET` | `/api/ready` | Readiness check. |

### Auth

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/auth/login` | Login. Returns `mustChangePassword` when defaults are active. |
| `POST` | `/api/auth/logout` | Logout current session. |
| `GET` | `/api/auth/session` | Session status, including `mustChangePassword` when applicable. |

Auth gate and security error codes used by protected routes include:

- `password_change_required`
- `csrf_origin_mismatch`

### System

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/settings` | Read panel settings. |
| `PUT` | `/api/settings` | Update panel settings. |
| `GET` | `/api/system/usage` | Live usage snapshot: host, panel, running servers, totals. |

`/api/system/usage` response includes:

- `timestamp`
- `host` (`logicalCpuCount`, `totalRamBytes`)
- `panel` (`cpuPercent`, `ramBytes`, `ramPercent`, `pid`)
- `servers[]` (`id`, `name`, `type`, `status`, `pid`, `cpuPercent`, `ramBytes`, `ramPercent`)
- `total` (`cpuPercent`, `ramBytes`, `ramPercent`)

### Servers

| Method | Endpoint |
|---|---|
| `GET` | `/api/servers` |
| `POST` | `/api/servers` |
| `DELETE` | `/api/servers/{id}` |
| `PUT` | `/api/servers/{id}/name` |
| `POST` | `/api/servers/{id}/start` |
| `POST` | `/api/servers/{id}/start-safe` |
| `POST` | `/api/servers/{id}/stop` |
| `POST` | `/api/servers/{id}/kill` |
| `POST` | `/api/servers/{id}/schedule-restart` |
| `DELETE` | `/api/servers/{id}/schedule-restart` |
| `POST` | `/api/servers/{id}/schedule-stop` |
| `POST` | `/api/servers/{id}/retry-install` |
| `PUT` | `/api/servers/{id}/version` |
| `PUT` | `/api/servers/{id}/settings` |
| `PUT` | `/api/servers/{id}/auto-start` |
| `PUT` | `/api/servers/{id}/restart-on-crash` |
| `PUT` | `/api/servers/{id}/flags` |
| `GET` | `/api/servers/{id}/status` |
| `PUT` | `/api/servers/order` |
| `POST` | `/api/servers/clone` |
| `POST` | `/api/servers/import/analyze` |
| `POST` | `/api/servers/import/commit` |
| `DELETE` | `/api/servers/import/analyze/{id}` |

### Versions

| Method | Endpoint |
|---|---|
| `GET` | `/api/versions/{type}` |
| `GET` | `/api/versions/{type}/{version}/java` |

The second endpoint is the Java pre-flight check: it reports which Java a version needs and whether this install has it.

### Files

| Method | Endpoint |
|---|---|
| `GET` | `/api/servers/{id}/files?path=` |
| `GET` | `/api/servers/{id}/files/exists?path=` |
| `GET` | `/api/servers/{id}/files/content?path=` |
| `PUT` | `/api/servers/{id}/files/content` |
| `GET` | `/api/servers/{id}/files/versions?path=` |
| `GET` | `/api/servers/{id}/files/versions/content?path=&id=` |
| `POST` | `/api/servers/{id}/files/upload` |
| `DELETE` | `/api/servers/{id}/files?path=` |
| `POST` | `/api/servers/{id}/files/mkdir` |
| `PUT` | `/api/servers/{id}/files/rename` |
| `POST` | `/api/servers/{id}/files/download` |

### Plugins / Mods

| Method | Endpoint |
|---|---|
| `GET` | `/api/servers/{id}/plugins` |
| `POST` | `/api/servers/{id}/plugins` |
| `DELETE` | `/api/servers/{id}/plugins/{name}` |
| `PUT` | `/api/servers/{id}/plugins/{name}/toggle` |
| `PUT` | `/api/servers/{id}/plugins/{name}/source` |
| `GET` | `/api/servers/{id}/plugins/check-updates` |
| `POST` | `/api/servers/{id}/plugins/{name}/update` |

### Backups

| Method | Endpoint |
|---|---|
| `GET` | `/api/servers/{id}/backups` |
| `POST` | `/api/servers/{id}/backups` |
| `DELETE` | `/api/servers/{id}/backups/{name}` |
| `GET` | `/api/servers/{id}/backups/{name}/download` |
| `POST` | `/api/servers/{id}/backups/{name}/restore` |
| `GET` | `/api/servers/{id}/backup-schedule` |
| `PUT` | `/api/servers/{id}/backup-schedule` |

### Logs and Crash Reports

| Method | Endpoint |
|---|---|
| `WS` | `/api/logs/{id}` |
| `GET` | `/api/servers/{id}/logs` |
| `GET` | `/api/servers/{id}/logs/{name}` |
| `GET` | `/api/servers/{id}/crash-reports` |
| `GET` | `/api/servers/{id}/crash-reports/{name}` |
| `POST` | `/api/servers/{id}/crash-reports/{name}/copy` |
| `DELETE` | `/api/servers/{id}/crash-reports/{name}` |

### Players

| Method | Endpoint |
|---|---|
| `GET` | `/api/servers/{id}/players` |
| `POST` | `/api/servers/{id}/players/{name}/kick` |
| `POST` | `/api/servers/{id}/players/{name}/ban` |
| `POST` | `/api/servers/{id}/players/{name}/kill` |

## Data Layout

Default runtime paths under `ADPANEL_DIR` (default `/AdPanel`):

```text
/AdPanel/
|-- orexa-panel
|-- dist/
|-- data/
|   |-- servers.json
|   |-- settings.json
|   |-- file-versions/        (previous versions of edited files, 5 per file)
|   `-- extension-sources/
|-- Servers/
`-- Backups/
```

## License

MIT License.

Copyright 2026 Pbarrera813
