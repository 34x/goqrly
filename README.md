# goqrly

Share QR codes with friends over the internet. No accounts, no cloud, no tracking — just self-hosted simplicity.

## Features

**Generate QR codes in seconds** — Paste a link, text, or anything. Get a QR code and short link instantly. Put it on a poster, in a presentation, wherever.

**Share sensitive stuff easily** — WiFi passwords, credentials, private links. Protect with password or authenticator app. Share link and access separately.

**No accounts, no tracking** — Self-hosted, no cookies, no cloud. Your data stays with you.

**TLS built-in** — HTTPS ready out of the box with self-signed certificates.

**Single binary** — Zero dependencies.

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
./goqrly --list-recent-public # Show recent public entries on homepage

# Install as service
sudo ./goqrly install         # TLS on port 443 (default)
sudo ./goqrly install --port 8080  # No TLS on port 8080
```

## Protection Options

When generating a QR code, choose protection level:

**No protection** — Anyone with the link can view.

**Password** — Viewers need a password to unlock.

**TOTP** — For group sharing:
1. Creator generates QR with TOTP option
2. Scan the setup QR with an authenticator app (FreeOTP, Google Auth, Authy, etc.)
3. Enter the 6-digit code to verify and create the entry
4. Share the short link with the group
5. Group members use their authenticator apps to get the current code and unlock

```
┌─────────────────────────────────────┐
│  Password    │  TOTP               │
│  [________]  │  [ ]                │
└─────────────────────────────────────┘
              [Generate]
```

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

## Command Line Options

```
-h, --help               Show this help message
--port <n>               Port to listen on (default: 8080)
--recent <n>             Number of recent codes on index page (default: 12)
--list-recent-public     Show recent public entries on index page
--tls                    Enable TLS with self-signed certificate
--cert <path>            Path to TLS certificate
--key <path>             Path to TLS private key
--remove-binary          Remove binary when uninstalling
```

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

## Tests

```bash
go test       # Unit tests
./test.sh     # Integration tests
```

## License

MIT
