#!/bin/bash
# Build and run the Claude Code Mobile Terminal
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "Building cc-web..."
go build -o cc-web ./cmd/gateway

echo "Starting Claude Code Mobile Terminal..."
echo "Config: ${1:-configs/config.yaml}"
echo ""
exec ./cc-web -config "${1:-configs/config.yaml}"
