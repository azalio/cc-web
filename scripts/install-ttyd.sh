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

  # Map architecture names
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  ARCH="x86_64" ;;
    aarch64) ARCH="aarch64" ;;
    armv7l)  ARCH="armhf" ;;
    *)
      echo "Unsupported architecture: $ARCH"
      exit 1
      ;;
  esac

  sudo curl -sL "https://github.com/tsl0922/ttyd/releases/download/${TTYD_VERSION}/ttyd.${ARCH}" -o /usr/local/bin/ttyd
  sudo chmod +x /usr/local/bin/ttyd
else
  echo "Unsupported OS. Please install ttyd manually: https://github.com/tsl0922/ttyd"
  exit 1
fi

echo "ttyd installed successfully."
