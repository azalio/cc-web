#!/bin/bash
# Build and run the Claude Code Mobile Terminal
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "Building cc-web..."
go build -o cc-web ./cmd/gateway

CONFIG="${1:-configs/config.local.yaml}"

if [ ! -f "$CONFIG" ]; then
  echo "Error: $CONFIG not found."
  echo "Copy configs/config.yaml and set a real auth_token:"
  echo "  cp configs/config.yaml configs/config.local.yaml"
  exit 1
fi

echo "Starting Claude Code Mobile Terminal..."
echo "Config: $CONFIG"
echo ""
exec ./cc-web -config "$CONFIG"
