#!/bin/bash
set -e

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

curl -sSL "https://github.com/34x/goqrly/releases/latest/download/goqrly_linux_${ARCH}.tar.gz" | tar -xz
mv goqrly_linux_${ARCH} goqrly
chmod +x goqrly
sudo ./goqrly install --port 80
