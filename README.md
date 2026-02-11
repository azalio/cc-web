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
| GET | `/healthz` | Health check (no auth) |
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
- Health endpoint `/healthz` (no auth) for tunnel/LB monitoring

## Network Access (Cloudflare Tunnel — recommended)

Cloudflare Tunnel gives you HTTPS access from your phone without opening ports, without conflicting with VPNs, and with Zero Trust authentication.

```
Phone → HTTPS → Cloudflare Edge → Encrypted Tunnel → Mac (localhost:8787)
```

### Automated setup

```bash
./scripts/setup-cloudflare-tunnel.sh your-domain.com
```

This will:
1. Install `cloudflared` if needed
2. Authenticate with Cloudflare (opens browser)
3. Create a tunnel named `claude-gateway`
4. Write `~/.cloudflared/config.yml`
5. Create DNS CNAME `claude.your-domain.com`
6. Validate the config

See `configs/cloudflared-config.example.yml` for a manual config template.

### Start the tunnel

```bash
# Manual
cloudflared tunnel run claude-gateway

# Install as system service (auto-start on boot)
sudo cloudflared service install
```

### Protect with Cloudflare Access

Go to [Cloudflare Zero Trust](https://one.dash.cloudflare.com) → Access → Applications:
1. Add Application → Self-hosted
2. Application domain: `claude.your-domain.com`
3. Policy: Allow → your email (or One-time PIN / GitHub login)

### Auto-start the gateway (macOS)

```bash
# Edit paths in the plist, then:
cp configs/com.claude-gateway.plist.example ~/Library/LaunchAgents/com.claude-gateway.plist
launchctl load ~/Library/LaunchAgents/com.claude-gateway.plist
```

### Other access methods

- **Tailscale**: `tailscale serve 8787`
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
