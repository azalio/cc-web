#!/bin/bash
# Install ttyd on macOS or Linux
set -e

if command -v ttyd &>/dev/null; then
  echo "ttyd is already installed: $(which ttyd)"
  ttyd --version
  exit 0
fi

if [[ "$OSTYPE" == "darwin"* ]]; then
  echo "Installing ttyd via Homebrew..."
  brew install ttyd
elif [[ -f /etc/debian_version ]]; then
  echo "Installing ttyd on Debian/Ubuntu..."
  sudo apt-get update && sudo apt-get install -y ttyd
elif [[ -f /etc/redhat-release ]]; then
  echo "Installing ttyd from GitHub releases..."
  TTYD_VERSION=$(curl -s https://api.github.com/repos/tsl0922/ttyd/releases/latest | grep tag_name | cut -d '"' -f 4)
  curl -sL "https://github.com/tsl0922/ttyd/releases/download/${TTYD_VERSION}/ttyd.$(uname -m)" -o /usr/local/bin/ttyd
  chmod +x /usr/local/bin/ttyd
else
  echo "Unsupported OS. Please install ttyd manually: https://github.com/tsl0922/ttyd"
  exit 1
fi

echo "ttyd installed successfully."
