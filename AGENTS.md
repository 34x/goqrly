# goqrly — Specification

## Overview
Single-binary QR code generator with web UI. Zero runtime dependencies. Supports password and TOTP protection.

---

## Functionality

### CLI
- `goqrly` — Run server (default port 8080)
- `goqrly --port 9000` — Custom port
- `goqrly --recent 20` — Number of recent codes on homepage
- `goqrly --list-recent-public` — Show recent public entries (hidden by default)
- `goqrly --tls` — HTTPS on port 443 with self-signed cert
- `goqrly -h` — Show help
- `sudo goqrly install` — One command: binary + service + firewall + TLS + ready

### Install Command (`goqrly install`)
1. Generate self-signed TLS certificates in `/etc/goqrly/`
2. Copy binary to `/usr/local/bin/goqrly`
3. Create systemd service
4. Enable and start service
5. Try to open firewall (ufw or firewalld)
6. Detect public IP (1s timeout)
7. Print access URLs

### Web UI (`/`)
- Text input for content
- Password input (optional)
- TOTP checkbox (optional)
- Generate button
- Recent public entries list (only if `--list-recent-public` enabled)

### QR Generation (`POST /generate`)
- Input: `text`, `password` (optional), `totp` (optional) fields
- If TOTP: show setup page with QR code of secret + code verification
- If password or none: generate shortcode and redirect to `/<shortcode>`
- Generate: 512x512 PNG QR code

### TOTP Setup (`POST /setup-totp`)
- Hidden fields: `secret`, `text`
- Input: `code` (6-digit TOTP)
- Validate code against secret
- If valid: create entry with random shortcode, redirect to `/<key>`
- If invalid: show error, allow retry

### QR Display (`/<shortcode>`)
- Shortcode lookup is **case-insensitive**
- Unprotected: show QR directly
- Protected (password): show password form
- Protected (TOTP): show code input form
- POST with correct credentials: show QR

### Shortcode Strategy
- **Password-protected/unprotected**: Deterministic
  ```
  text → SHA256 → base64 (lowercase) → key[0:N]
  Same text + same password = same key
  ```
- **TOTP-protected**: Random (3-6 chars)
  ```
  Generate random key → check collision → store
  Different from deterministic keys
  ```

---

## Protection Types

### None (Public)
- Anyone with link can view
- Shown in recent list only if `--list-recent-public` enabled

### Password
- Viewer enters password to unlock
- Password hashed with SHA256

### TOTP
- Creator shares secret with group
- Group members use authenticator apps
- Current 6-digit code unlocks entry
- Standard otpauth URI format (FreeOTP compatible)

---

## Technical

### Stack
- **Language:** Go 1.21+
- **Web:** stdlib `net/http`
- **QR:** `github.com/skip2/go-qrcode`
- **TOTP:** `github.com/pquerna/otp`
- **Embedding:** `//go:embed`
- **Binary:** CGO disabled, static build

### Entry Struct
```go
type Entry struct {
    Text       string  // Original content
    Protected  bool    // true if password or TOTP
    Password   string  // SHA256 hash, empty if TOTP or none
    TOTPSecret string  // Base32 secret, empty if password or none
    QR         []byte  // PNG data
}
```

### File Structure
```
.
├── main.go             # CLI, install, server setup
├── handlers.go         # HTTP handlers
├── store.go            # QR generation, shortcode logic, TOTP
├── static/
│   ├── index.html      # Main page
│   ├── view.html       # QR display page
│   ├── lock.html       # Password/TOTP unlock form
│   └── setup-totp.html # TOTP setup page
├── handlers_test.go    # Unit tests
├── test.sh             # Integration tests
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
go build -ldflags="-s -w" -o goqrly .
```

Result: ~8MB static binary, no dependencies.

---

## Constraints
- No external services (no DB, no Redis)
- No runtime dependencies
- Works offline after build
- QR codes generated on-demand, stored in memory
- Binary path fixed at `/usr/local/bin/goqrly` (for install)
- Recent list hidden by default (privacy first)

---

## Tests
```bash
go test    # Unit tests (handlers, store, TOTP)
./test.sh  # Integration tests (curl-based)
```
