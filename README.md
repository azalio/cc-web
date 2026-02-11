# Claude Code Mobile Web Terminal

Mobile-friendly web interface for managing Claude Code sessions from your phone. View live terminal output, create/manage multiple sessions, and intervene with quick actions — all from Safari or any mobile browser.

## Architecture

- **Go backend** — REST API for session CRUD, tmux management, ttyd lifecycle, reverse proxy
- **tmux** — Session persistence; processes survive browser disconnects
- **ttyd** — Web terminal (xterm.js) attached to tmux sessions, proxied through the backend
- **PWA frontend** — Mobile-first UI with sessions list, embedded terminal, intervention panel

## Prerequisites

- Go 1.21+
- tmux
- ttyd (optional but recommended for embedded terminal)

### Install ttyd

```bash
# macOS
brew install ttyd

# Debian/Ubuntu
sudo apt-get install ttyd

# Or use the helper script
./scripts/install-ttyd.sh
```

## Quick Start

1. **Configure** — Edit `configs/config.yaml`:
   ```yaml
   auth_token: "your-secure-token-here"
   projects_allowed:
     - "/Users/you/src"
     - "/Users/you/work"
   ```

2. **Build and run**:
   ```bash
   make run
   # or
   go build -o cc-web ./cmd/gateway && ./cc-web -config configs/config.yaml
   ```

3. **Open on your phone**: Navigate to `http://<your-ip>:8787` and enter your token.

## API

All API endpoints require `Authorization: Bearer <token>` header (or `?token=<token>` query param).

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/sessions` | List all sessions |
| POST | `/api/sessions` | Create session `{name, cwd, start_cmd}` |
| GET | `/api/sessions/{id}` | Get session details |
| POST | `/api/sessions/{id}/send` | Send text `{text}` + Enter |
| POST | `/api/sessions/{id}/interrupt` | Send Ctrl+C |
| POST | `/api/sessions/{id}/keys` | Send key tokens `{keys: ["ESC","UP"]}` |
| POST | `/api/sessions/{id}/kill` | Kill session |
| GET | `/t/{id}/` | Terminal proxy (ttyd WebSocket) |

## Security

- Bearer token authentication on all endpoints
- Working directory allowlist prevents arbitrary path access
- ttyd binds to 127.0.0.1 only (not exposed directly)
- Recommended: use with Tailscale for secure remote access

## Network Access

For phone access, either:

- **Tailscale**: `tailscale serve 8787` (recommended)
- **Local network**: Change `listen_addr` to `0.0.0.0:8787` (LAN only)
- **SSH tunnel**: `ssh -L 8787:localhost:8787 your-mac`

## Project Structure

```
cmd/gateway/          # Main server binary
internal/
  config/             # YAML config loader + path allowlist
  http/               # HTTP handlers + auth middleware
  proxy/              # Reverse proxy helpers
  sessions/           # Session manager, tmux runner, ttyd manager
web/static/           # PWA frontend (HTML/CSS/JS)
scripts/              # Install and run helpers
configs/              # Example configuration
```

## Development

```bash
make build    # Build binary
make test     # Run tests
make run      # Build + run
make clean    # Remove artifacts
```
