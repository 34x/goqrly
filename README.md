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

## Reverse Proxy with Caddy

For production with automatic Let's Encrypt SSL, run goqrly behind Caddy:

**1. Install goqrly:**
```bash
sudo curl -sSL https://github.com/34x/goqrly/releases/latest/download/goqrly_linux_amd64.tar.gz | tar -xz
sudo mv goqrly_linux_amd64 /usr/local/bin/goqrly
sudo chmod +x /usr/local/bin/goqrly
sudo mkdir -p /var/lib/goqrly
```

**2. Create systemd service:**
```bash
sudo tee /etc/systemd/system/goqrly.service <<EOF
[Unit]
Description=goqrly QR Code Generator
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goqrly --port 8080 --data-dir /var/lib/goqrly
Restart=always

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now goqrly
```

**3. Install Caddy:**
```bash
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update && sudo apt install -y caddy
```

**4. Configure Caddy:**
```bash
sudo tee /etc/caddy/Caddyfile <<EOF
example.com {
    reverse_proxy localhost:8080
}
EOF
```

Replace `example.com` with your domain. Ensure DNS points to your server.

**5. Open firewall and restart:**
```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo systemctl restart caddy
```

Caddy will automatically obtain and renew Let's Encrypt certificates.

**Result:**
- `http://example.com` → redirects to HTTPS
- `https://example.com` → goqrly with valid SSL certificate
- Data persisted in `/var/lib/goqrly/` (encrypted at rest)

## Build from Source

```bash
git clone https://github.com/34x/goqrly.git
cd goqrly
go build -ldflags="-s -w" -o goqrly .
```

## Security

### Password-Protected Entries

Passwords are hashed with bcrypt. Content is encrypted with AES-256-GCM using a key derived from the password via Argon2id (per-entry random salt).

### File Storage Encryption

When using `--data-dir`, all entries are encrypted at rest with AES-256-GCM using a server key stored in `.server_key`:

```
data/
├── .server_key    # 32-byte key (base64)
├── abc123.json    # Encrypted entry
└── xyz789.json    # Encrypted entry
```

**Important:** The server key is stored alongside the encrypted data. This provides:
- Obfuscation against casual inspection
- Protection against accidental log/backup inclusion
- Compliance with "encrypted at rest" requirements

**This does NOT protect against:**
- Attackers with filesystem access (key is in the same directory)
- Disk theft (key is on the same disk)
- Backup exposure (key is in the backup)

For stronger security, consider running in memory-only mode (default) or storing `.server_key` separately with restricted permissions.

### In-Memory Mode (Default)

Without `--data-dir`, all data exists only in RAM and is lost on restart. No persistent attack surface.

## Tests

```bash
make test     # Build + run all tests (recommended)
make unit-test    # Unit tests only
./test.sh         # Run tests directly (builds first)
```

## Development

```bash
make build    # Build binary
make test     # Run all tests
make lint     # Format + vet
make run      # Build and run locally
make clean    # Remove artifacts
make help     # Show all targets
```

## License

MIT
