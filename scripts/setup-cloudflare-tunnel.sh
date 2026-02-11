#!/bin/bash
# Setup Cloudflare Tunnel for Claude Code Mobile Gateway
# This script guides you through creating a Cloudflare Tunnel
# to securely expose your local gateway to the internet (HTTPS, no open ports).
#
# Prerequisites:
#   - A domain managed by Cloudflare DNS
#   - macOS with Homebrew, or Debian/Ubuntu Linux with apt
#   - python3 (for parsing cloudflared JSON output)
#
# Usage: ./scripts/setup-cloudflare-tunnel.sh <your-domain>
# Example: ./scripts/setup-cloudflare-tunnel.sh example.com
#   -> creates tunnel at claude.example.com

set -euo pipefail

TUNNEL_NAME="claude-gateway"
SUBDOMAIN="claude"
GATEWAY_PORT="${GATEWAY_PORT:-8787}"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# --- Parse args ---
DOMAIN="${1:-}"
if [ -z "$DOMAIN" ]; then
    err "Usage: $0 <your-domain>"
    err "Example: $0 example.com"
    exit 1
fi

if ! command -v python3 &>/dev/null; then
    err "python3 is required to parse cloudflared JSON output."
    err "Install it: sudo apt-get install python3 (or brew install python3)"
    exit 1
fi

HOSTNAME="${SUBDOMAIN}.${DOMAIN}"

echo ""
echo "============================================="
echo " Cloudflare Tunnel Setup for Claude Gateway"
echo "============================================="
echo ""
echo " Tunnel name:  ${TUNNEL_NAME}"
echo " Hostname:     ${HOSTNAME}"
echo " Gateway:      http://localhost:${GATEWAY_PORT}"
echo ""

# --- Step 1: Install cloudflared ---
info "Step 1: Checking cloudflared installation..."

if command -v cloudflared &>/dev/null; then
    ok "cloudflared is installed: $(cloudflared --version 2>&1 | head -1)"
else
    info "Installing cloudflared..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        if command -v brew &>/dev/null; then
            brew install cloudflared
        else
            err "Homebrew not found. Install it first: https://brew.sh"
            exit 1
        fi
    elif [[ -f /etc/debian_version ]]; then
        # Determine codename; lsb_release may be absent on minimal installs
        local codename
        if command -v lsb_release &>/dev/null; then
            codename=$(lsb_release -cs)
        elif [[ -f /etc/os-release ]]; then
            codename=$(. /etc/os-release && echo "${VERSION_CODENAME:-$UBUNTU_CODENAME}")
        fi
        if [[ -z "$codename" ]]; then
            err "Cannot determine Debian/Ubuntu codename. Install lsb-release or check /etc/os-release."
            exit 1
        fi
        curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg \
            | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
        echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared ${codename} main" \
            | sudo tee /etc/apt/sources.list.d/cloudflared.list
        sudo apt-get update && sudo apt-get install -y cloudflared
    else
        err "Unsupported OS. Install cloudflared manually:"
        err "  https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
        exit 1
    fi
    ok "cloudflared installed."
fi

# --- Step 2: Authenticate ---
info "Step 2: Authenticating with Cloudflare..."

if [ -f "$HOME/.cloudflared/cert.pem" ]; then
    ok "Already authenticated (cert.pem found)."
else
    info "Opening browser for Cloudflare login..."
    info "Select the zone for: ${DOMAIN}"
    cloudflared tunnel login
    ok "Authentication complete."
fi

# --- Step 3: Create tunnel ---
info "Step 3: Creating tunnel '${TUNNEL_NAME}'..."

# Check if tunnel already exists
EXISTING_ID=$(cloudflared tunnel list --output json 2>/dev/null \
    | python3 -c "import sys,json; tunnels=json.load(sys.stdin); print(next((t['id'] for t in tunnels if t['name']=='${TUNNEL_NAME}'), ''))" 2>/dev/null || true)

if [ -n "$EXISTING_ID" ]; then
    ok "Tunnel '${TUNNEL_NAME}' already exists (ID: ${EXISTING_ID})."
    TUNNEL_ID="$EXISTING_ID"
else
    cloudflared tunnel create "$TUNNEL_NAME"
    TUNNEL_ID=$(cloudflared tunnel list --output json 2>/dev/null \
        | python3 -c "import sys,json; tunnels=json.load(sys.stdin); print(next((t['id'] for t in tunnels if t['name']=='${TUNNEL_NAME}'), ''))" 2>/dev/null)
    ok "Tunnel created (ID: ${TUNNEL_ID})."
fi

if [ -z "$TUNNEL_ID" ]; then
    err "Could not determine tunnel ID. Run: cloudflared tunnel list"
    exit 1
fi

# --- Step 4: Find credentials file ---
CREDS_FILE="$HOME/.cloudflared/${TUNNEL_ID}.json"
if [ ! -f "$CREDS_FILE" ]; then
    err "Credentials file not found: ${CREDS_FILE}"
    err "Try re-creating the tunnel: cloudflared tunnel delete ${TUNNEL_NAME} && cloudflared tunnel create ${TUNNEL_NAME}"
    exit 1
fi
ok "Credentials: ${CREDS_FILE}"

# --- Step 5: Write cloudflared config ---
info "Step 5: Writing cloudflared config..."

CLOUDFLARED_CONFIG="$HOME/.cloudflared/config.yml"

# Back up existing config
if [ -f "$CLOUDFLARED_CONFIG" ]; then
    BACKUP="${CLOUDFLARED_CONFIG}.backup.$(date +%s)"
    cp "$CLOUDFLARED_CONFIG" "$BACKUP"
    warn "Existing config backed up to: ${BACKUP}"
fi

cat > "$CLOUDFLARED_CONFIG" << CFGEOF
# Cloudflare Tunnel config for Claude Code Gateway
# Generated by setup-cloudflare-tunnel.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")

tunnel: ${TUNNEL_ID}
credentials-file: ${CREDS_FILE}

ingress:
  - hostname: ${HOSTNAME}
    service: http://localhost:${GATEWAY_PORT}
    originRequest:
      # Support WebSocket for ttyd terminal
      noTLSVerify: false
      connectTimeout: 10s
      noHappyEyeballs: false
  # Catch-all (required by cloudflared)
  - service: http_status:404
CFGEOF

ok "Config written: ${CLOUDFLARED_CONFIG}"

# --- Step 6: Route DNS ---
info "Step 6: Setting up DNS route..."

if cloudflared tunnel route dns "$TUNNEL_NAME" "$HOSTNAME" 2>&1; then
    ok "DNS CNAME: ${HOSTNAME} -> ${TUNNEL_NAME}.cfargotunnel.com"
else
    warn "DNS route command failed. The CNAME may already exist, or you may need to add it manually."
    warn "Expected: CNAME ${HOSTNAME} -> ${TUNNEL_ID}.cfargotunnel.com"
fi

# --- Step 7: Validate ---
info "Step 7: Validating tunnel config..."

cloudflared tunnel ingress validate 2>&1
ok "Config validation passed."

# --- Step 8: Test run ---
echo ""
echo "============================================="
echo " Setup complete!"
echo "============================================="
echo ""
echo " Tunnel ID:    ${TUNNEL_ID}"
echo " Hostname:     https://${HOSTNAME}"
echo " Config:       ${CLOUDFLARED_CONFIG}"
echo " Credentials:  ${CREDS_FILE}"
echo ""
echo " To start the tunnel manually:"
echo "   cloudflared tunnel run ${TUNNEL_NAME}"
echo ""
echo " To install as a system service (auto-start):"
if [[ "$OSTYPE" == "darwin"* ]]; then
echo "   sudo cloudflared service install"
echo "   sudo launchctl start com.cloudflare.cloudflared"
else
echo "   sudo cloudflared service install"
echo "   sudo systemctl enable --now cloudflared"
fi
echo ""
echo " IMPORTANT: Set up Cloudflare Access to protect your endpoint!"
echo " Go to: https://one.dash.cloudflare.com -> Access -> Applications"
echo "   1. Add Application -> Self-hosted"
echo "   2. Application domain: ${HOSTNAME}"
echo "   3. Create a policy: Allow -> your email"
echo ""
echo " Then open https://${HOSTNAME} on your phone."
echo ""
