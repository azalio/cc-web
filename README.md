# Claude Code Mobile Web Terminal

Mobile-friendly web interface for managing Claude Code sessions from your phone. View live terminal output, create/manage multiple sessions, and intervene with quick actions — all from Safari or any mobile browser.

## Architecture

- **Go backend** — REST API for session CRUD, tmux management, ttyd lifecycle, reverse proxy
- **tmux** — Session persistence; processes survive browser disconnects
- **ttyd** — Web terminal (xterm.js) attached to tmux sessions, proxied through the backend
- **PWA frontend** — Mobile-first UI with sessions list, embedded terminal, intervention panel

## Prerequisites

- Go 1.24+
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

1. **Configure** — Copy the example config and set a real auth token:
   ```bash
   cp configs/config.yaml configs/config.local.yaml
   ```
   Edit `configs/config.local.yaml`:
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
   go build -o cc-web ./cmd/gateway && ./cc-web -config configs/config.local.yaml
   ```

3. **Open on your phone**: Navigate to `http://<your-ip>:8787` and enter your token.

## API

API endpoints (`/api/...`) require `Authorization: Bearer <token>` header or `auth_token` cookie.
Terminal proxy (`/t/...`) also accepts the `auth_token` cookie set by the PWA on login.

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

## Remote Access via Cloudflare Tunnel

Cloudflare Tunnel gives you HTTPS access from your phone without opening ports, without conflicting with VPNs, and with Zero Trust authentication on top.

```
Phone (Safari/Chrome)
  │
  ▼  HTTPS
Cloudflare Edge  ──  Zero Trust Access (email OTP / GitHub / SSO)
  │
  ▼  Encrypted tunnel (QUIC)
Your machine (localhost:8787)  ──  cc-web gateway
```

### Prerequisites

- A domain with DNS managed by Cloudflare (free plan works)
- `python3` (used by the setup script to parse JSON)

### Step 1 — Run the setup script

```bash
./scripts/setup-cloudflare-tunnel.sh your-domain.com
```

The script will:

| Step | What happens |
|------|-------------|
| 1 | Installs `cloudflared` via Homebrew (macOS) or apt (Debian/Ubuntu) |
| 2 | Opens a browser to authenticate with your Cloudflare account |
| 3 | Creates a tunnel named **claude-gateway** |
| 4 | Locates the credentials file (`~/.cloudflared/<tunnel-id>.json`) |
| 5 | Writes `~/.cloudflared/config.yml` with ingress rules + WebSocket support |
| 6 | Creates a DNS CNAME: `claude.your-domain.com` → tunnel |
| 7 | Validates the config |

> **Custom port:** `GATEWAY_PORT=9000 ./scripts/setup-cloudflare-tunnel.sh your-domain.com`

### Step 2 — Start the tunnel

```bash
cloudflared tunnel run claude-gateway
```

You should see `Connection … registered` lines — the tunnel is live.

### Step 3 — Start the gateway

In a separate terminal (or tmux/screen):

```bash
make run
# or: go build -o cc-web ./cmd/gateway && ./cc-web -config configs/config.local.yaml
```

### Step 4 — Protect with Cloudflare Access (recommended)

Without this step, anyone with the URL can reach your login page. Cloudflare Access adds a second auth layer before traffic even hits your machine.

1. Open [Cloudflare Zero Trust dashboard](https://one.dash.cloudflare.com)
2. Go to **Access → Applications → Add Application → Self-hosted**
3. Set **Application domain** to `claude.your-domain.com`
4. Create a policy:
   - **Action:** Allow
   - **Include:** Emails — `your-email@example.com`
   - (or use GitHub / Google / One-time PIN)
5. Save

Now visiting `claude.your-domain.com` will first prompt for Cloudflare Access auth, then show the cc-web login page.

### Step 5 — Open on your phone

Navigate to:

```
https://claude.your-domain.com
```

Enter the `auth_token` from your `configs/config.local.yaml`. The PWA will offer "Add to Home Screen" for an app-like experience.

### Auto-start as a system service

So the tunnel starts on boot without manual intervention:

```bash
# macOS
sudo cloudflared service install
sudo launchctl start com.cloudflare.cloudflared

# Linux (systemd)
sudo cloudflared service install
sudo systemctl enable --now cloudflared
```

For the gateway itself on macOS:

```bash
cp configs/com.claude-gateway.plist.example ~/Library/LaunchAgents/com.claude-gateway.plist
# Edit paths inside the plist to match your setup
launchctl load ~/Library/LaunchAgents/com.claude-gateway.plist
```

### Manual tunnel setup (without the script)

If you prefer to configure everything yourself:

```bash
# Install
brew install cloudflared        # macOS
# sudo apt-get install cloudflared  # Linux

# Authenticate
cloudflared tunnel login

# Create tunnel
cloudflared tunnel create claude-gateway

# Route DNS
cloudflared tunnel route dns claude-gateway claude.your-domain.com
```

Then create `~/.cloudflared/config.yml` (see `configs/cloudflared-config.example.yml` for a template):

```yaml
tunnel: <tunnel-id>
credentials-file: /home/you/.cloudflared/<tunnel-id>.json

ingress:
  - hostname: claude.your-domain.com
    service: http://localhost:8787
    originRequest:
      connectTimeout: 10s
  - service: http_status:404
```

Validate and run:

```bash
cloudflared tunnel ingress validate
cloudflared tunnel run claude-gateway
```

### Alternative access methods

| Method | Command / Config | Notes |
|--------|-----------------|-------|
| **Tailscale** | `tailscale serve 8787` | Zero config, works behind NAT |
| **Local network** | Set `listen_addr: "0.0.0.0:8787"` in config | LAN only, no encryption |
| **SSH tunnel** | `ssh -L 8787:localhost:8787 your-server` | Requires SSH access |

### Troubleshooting

| Problem | Fix |
|---------|-----|
| `cloudflared` not found | Install: `brew install cloudflared` or [download](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) |
| "failed to connect" in cloudflared logs | Make sure cc-web is running on the correct port (`localhost:8787`) |
| WebSocket errors in terminal | Check that `config.yml` ingress has no `noTLSVerify: true` for localhost |
| Tunnel works but phone shows 403 | Check Cloudflare Access policy — your email must be in the Allow list |
| DNS not resolving | Wait 1-2 min for propagation, or check CNAME in Cloudflare DNS dashboard |

## Project Structure

```
cmd/gateway/          # Main server binary
internal/
  config/             # YAML config loader + path allowlist
  http/               # HTTP handlers, auth middleware, reverse proxy
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
