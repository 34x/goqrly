# goqrly

Single-binary QR code generator with web UI. Zero dependencies.

## Quick Setup (one-liner)

```bash
curl -sSL https://raw.githubusercontent.com/34x/goqrly/main/setup.sh | sudo bash
```

This downloads the latest binary, installs it as a systemd service with TLS on port 443, and opens the firewall.

## Install Command

Running `sudo ./goqrly install` will:

1. Generate self-signed TLS certificates
2. Copy binary to `/usr/local/bin/goqrly`
3. Create systemd service at `/etc/systemd/system/goqrly.service`
4. Enable and start the service
5. Open firewall port (ufw or firewalld)
6. Detect public IP and print access URLs

**Default: port 443 with TLS.** Use `--port 8080` (without `--tls`) to install without TLS.

## Uninstall

```bash
sudo ./goqrly uninstall                # Remove service, keep certs and binary
sudo ./goqrly uninstall --remove-binary  # Remove everything
```

Uninstall removes the systemd service and, if certificates were auto-generated, the `/etc/goqrly/` directory. Custom certificates (passed via `--cert`/`--key`) are never removed. Use `--remove-binary` to also remove the binary from `/usr/local/bin/goqrly`.

## Quick Start

```bash
# Download binary
curl -L https://github.com/34x/goqrly/releases/latest/download/goqrly_linux_amd64.tar.gz -o goqrly.tar.gz && tar -xzvf goqrly.tar.gz && mv goqrly_linux_amd64 goqrly && chmod +x ./goqrly

# Run directly
./goqrly                      # http on port 8080
./goqrly --port 9000          # http on custom port
./goqrly --tls                # https on port 443 (self-signed)

# Install as service
sudo ./goqrly install         # TLS on port 443 (default)
sudo ./goqrly install --port 8080  # No TLS on port 8080
```

## Features

- **Single binary** — No dependencies, no runtime needed
- **Web UI** — Simple form to generate QR codes
- **Short codes** — 3-6 character deterministic keys
- **Recent list** — Last 12 codes shown on homepage
- **Auto-scale** — Collision-safe, shortest available code
- **Case-insensitive** — `/abc` = `/ABC`
- **Systemd service** — Auto-restarts on failure
- **Firewall** — Auto-opens port (ufw/firewalld)
- **TLS support** — Self-signed certificates (auto-generated on install)

## TLS Certificates

Certificates are auto-generated on install and stored at:
- `/etc/goqrly/goqrly.crt`
- `/etc/goqrly/goqrly.key`

**Runtime with self-signed cert:**
```bash
./goqrly --tls  # Uses in-memory cert, port 443
```

**Provide your own certificates:**
```bash
./goqrly --cert /path/to/cert.pem --key /path/to/key.pem
sudo ./goqrly install --cert /path/to/cert.pem --key /path/to/key.pem
```

Note: Browsers will show a "not secure" warning for self-signed certificates. For production use, consider Let's Encrypt.

## Manual Installation

Without `install` command:

```bash
# Copy binary
sudo cp goqrly /usr/local/bin/

# Create service (edit ExecStart as needed)
sudo tee /etc/systemd/system/goqrly.service <<EOF
[Unit]
Description=goqrly QR Code Generator
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goqrly --port 8080
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable --now goqrly

# Open firewall
sudo ufw allow 8080/tcp
```

## Build from Source

```bash
git clone https://github.com/34x/goqrly.git
cd goqrly
go build -ldflags="-s -w" -o goqrly .
```

## License

MIT
