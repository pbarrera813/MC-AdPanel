<h1><img src="public/icon.png" alt="Orexa Panel icon" width="34" /> Orexa Panel v1.0.1</h1>

Orexa Panel is a self-hosted Minecraft server panel built around one idea: plug and play.
Download it, open it, and start creating and managing servers from one place.
Core administration features are available without paywalls, hidden tiers, or lock-in.

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)

## What Orexa Is

Orexa is designed for day-to-day Minecraft administration with a simple workflow:

- Create and run multiple servers from the browser.
- Manage files, plugins/mods, logs, backups, and players without extra tools.
- Use safe defaults out of the box, with optional advanced tuning when needed.
- Keep core server management features open and accessible, with no paid feature gates.

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

### Manual Build (optional)

Requirements:

- Linux
- Go 1.22+
- Node.js 20+
- Java 17+

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
- Clone servers with per-section options (worlds, plugins/mods, configs).
- Scheduled restart and scheduled stop.
- Auto-start toggle and retry install support.
- Velocity-aware settings behavior for proxy instances.

### Monitoring and Console

- Live console stream over WebSocket.
- Console clears on new start after a prior stop, so each new run begins cleanly.
- Live server metrics with corrected host-share CPU and RAM percentages.
- System-wide usage endpoint and UI panel for panel + running managed servers.

### Players

- Live player list with name, world, session time, and actions.
- Per-player ping support when compatible plugin/mod support is present.
- Kick, ban, and kill actions directly from the panel.

### File Browser

- Folder browsing, search, upload, rename, edit, download, and delete.
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
| `ADPANEL_PLUGIN_UPDATE_ALLOWED_HOSTS` | unset | Extra allowed hosts/domains for plugin/mod update downloads. |
| `ADPANEL_MAX_PLUGIN_UPDATE_BYTES` | `268435456` | Max download size for plugin/mod update fetches (256 MB). |
| `ADPANEL_USER_AGENT` | unset | Optional global User-Agent override for upstream fetches. |
| `ADPANEL_DEBUG_PLUGIN_UPDATES` | `0` | Set to `1` for verbose plugin/mod update diagnostics. |
| `ADPANEL_AUTO_FIX_HOSTS` | enabled | Set to `false` to disable startup hostname `/etc/hosts` auto-fix attempts on Linux. |

## Security Posture (Current)

- Passwords are stored hashed with Argon2id.
- Legacy SHA-256 hashes are verified for backward compatibility and transparently upgraded on successful login.
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
| `POST` | `/api/servers/{id}/schedule-restart` |
| `DELETE` | `/api/servers/{id}/schedule-restart` |
| `POST` | `/api/servers/{id}/schedule-stop` |
| `POST` | `/api/servers/{id}/retry-install` |
| `PUT` | `/api/servers/{id}/version` |
| `PUT` | `/api/servers/{id}/settings` |
| `PUT` | `/api/servers/{id}/auto-start` |
| `PUT` | `/api/servers/{id}/flags` |
| `GET` | `/api/servers/{id}/status` |
| `POST` | `/api/servers/clone` |

### Versions

| Method | Endpoint |
|---|---|
| `GET` | `/api/versions/{type}` |

### Files

| Method | Endpoint |
|---|---|
| `GET` | `/api/servers/{id}/files?path=` |
| `GET` | `/api/servers/{id}/files/exists?path=` |
| `GET` | `/api/servers/{id}/files/content?path=` |
| `PUT` | `/api/servers/{id}/files/content` |
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
|   `-- extension-sources/
|-- Servers/
`-- Backups/
```

## License

MIT License.

Copyright 2026 Pablo Barrera
