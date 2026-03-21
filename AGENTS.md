# goqrly — Specification

## Overview
Single-binary QR code generator with web UI. Zero runtime dependencies.

---

## Functionality

### CLI
- `goqrly` — Run server (default port 8080)
- `goqrly --port 9000` — Custom port
- `goqrly --recent 20` — Number of recent codes on homepage
- `goqrly -h` — Show help
- `sudo goqrly install` — One command: binary + service + firewall + ready

### Install Command (`goqrly install`)
1. Copy binary to `/usr/local/bin/goqrly`
2. Create systemd service
3. Enable and start service
4. Try to open firewall (ufw or firewalld)
5. Detect public IP (1s timeout)
6. Print access URLs

### Web UI (`/`)
- Single text input for any content
- "Generate" button
- Recent codes list (QR + truncated text + shortcode)
- POST to `/generate`

### QR Generation (`POST /generate`)
- Input: `text` field in form body (any text, not just URLs)
- Validate: non-empty, trimmed
- Generate: 512x512 PNG QR code
- Response: redirect to `/<shortcode>`

### QR Display (`/<shortcode>`)
- Shortcode lookup is **case-insensitive** (`/abc` = `/ABC` = `/AbC`)
- Serve embedded HTML page showing:
  - QR code image
  - Original text (in input field for easy regeneration)
  - "Generate another" link
- QR codes persist indefinitely (in-memory map)

### Shortcode Strategy
- Canonicalize input: trim whitespace
- Generate SHA256 hash of canonicalized input
- Encode hash as lowercase base64 (RawURLEncoding)
- Generate shortest unique key: try 3 chars, increment to 6 if collision
  ```
  text → SHA256 → base64 (lowercase) → key[0:N]
  if key exists AND text mismatch:
      increment salt → regenerate hash → try again
  ```
- Same text always returns same shortcode (deterministic)
- Store in memory: `map[key]{text, pngBytes}`
- No expiration, no cleanup

---

## Technical

### Stack
- **Language:** Go 1.21+
- **Web:** stdlib `net/http`
- **QR:** `github.com/skip2/go-qrcode`
- **Embedding:** `//go:embed`
- **Binary:** CGO disabled, static build

### File Structure
```
.
├── main.go          # CLI, install, server setup
├── handlers.go      # HTTP handlers
├── store.go         # QR generation, shortcode logic
├── static/
│   ├── index.html   # Main page with recent list
│   └── view.html    # QR display page
├── go.mod
├── go.sum
├── README.md
└── AGENTS.md
```

### Firewall
- Try `ufw allow <port>/tcp` first
- Fallback to `firewall-cmd` (firewalld)
- If neither available, print manual command

### IP Detection
- 1 second timeout
- Try `curl -s ifconfig.me` (external service)
- Fallback to `hostname -I` filtered for public IPs
- Gracefully skip if unavailable

---

## Build
```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o goqrly .
```

Result: ~8MB static binary, no dependencies.

---

## Constraints
- No external services (no DB, no Redis)
- No runtime dependencies
- Works offline after build
- QR codes generated on-demand, stored in memory
- Binary path fixed at `/usr/local/bin/goqrly` (for install)
