# goqrly

Single-binary QR code generator with web UI. Zero dependencies.

## Quick Setup (one-liner, uses port 80)

```bash
curl -sSL https://raw.githubusercontent.com/34x/goqrly/main/setup.sh | sudo bash
```

This downloads the latest binary, installs it as a systemd service on port 80, and opens the firewall.

## Install Command

Running `sudo ./goqrly install --port 8080` will:

1. Copy binary to `/usr/local/bin/goqrly`
2. Create systemd service at `/etc/systemd/system/goqrly.service`
3. Enable and start the service
4. Open firewall port (ufw or firewalld)
5. Detect public IP and print access URLs

## Quick Start

```bash
# Download binary (replace VERSION and ARCH as needed)
curl -L https://github.com/34x/goqrly/releases/latest/download/goqrly_linux_amd64.tar.gz -o goqrly.tar.gz && tar -xzvf goqrly.tar.gz && mv goqrly_linux_amd64 goqrly && chmod +x ./goqrly && ./goqrly install --port 80
```

That's it. Access at `http://YOUR_IP:80`

## Manual Usage without installation as a system service

```bash
# Run directly
./goqrly                      # default port 8080
./goqrly --port 9000          # custom port
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

## Manual Installation

Without `install` command:

```bash
# Copy binary
sudo cp goqrly /usr/local/bin/

# Create service
sudo tee /etc/systemd/system/goqrly.service <<EOF
[Unit]
Description=goqrly QR Code Generator
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goqrly
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
git clone https://github.com/YOUR_USER/goqrly.git
cd goqrly
go build -ldflags="-s -w" -o goqrly .
```

## License

MIT
