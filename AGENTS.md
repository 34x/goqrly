# goqrly — Specification

## Overview
Single-binary QR code generator with web UI. Zero runtime dependencies. Supports password and TOTP protection with AES-256-GCM encryption for sensitive content.

---

## Functionality

### CLI
- `goqrly` — Run server (default port 8080, in-memory storage)
- `goqrly --port 9000` — Custom port
- `goqrly --recent 20` — Number of recent codes on homepage
- `goqrly --list-recent-public` — Show recent public entries (hidden by default)
- `goqrly --data-dir ./data` — Persistent file-based storage
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
- Content stored as plaintext in file storage

### Password
- Viewer enters password to unlock
- Password hashed with **bcrypt** (not SHA256)
- Content **encrypted with AES-256-GCM** using key derived from password + **per-entry random salt**
- Key derivation uses **Argon2id** (3 iterations, 64MB memory, 4 parallelism)
- Each password-protected entry has a unique 16-byte random salt (128 bits)
- **Version 2** entries store salt in `salt` field; **Version 1** (legacy) used shortcode as salt

### TOTP
- Creator shares secret with group
- Group members use authenticator apps
- Current 6-digit code unlocks entry
- Standard otpauth URI format (FreeOTP compatible)

---

## Storage

### In-Memory (Default)
- Entries stored in memory map
- Lost on restart
- Thread-safe with sync.RWMutex

### File-Based (`--data-dir`)
- All entries encrypted at rest with server key before writing to disk
- Entries stored as individual encrypted files: `<key>.json`
- Server key (32 bytes) stored in `.server_key` (base64-encoded, 0600 permissions)
- Atomic writes via temp file + rename
- On startup: fails if directory has entries but no server key (inconsistent state)
- On startup: generates new server key if directory is empty

### File Format
Files contain base64-encoded encrypted JSON:
```
base64(nonce + ciphertext)
```

Decrypted JSON structure:
```json
{
  "key": "abc123",
  "text": "public content or empty for protected",
  "encrypted_data": "base64(nonce + ciphertext) or empty",
  "salt": "base64(random 16-byte salt) or empty",
  "protected": true,
  "password": "bcrypt hash or empty",
  "totp_secret": "base32 secret or empty",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

---

## Security

### Encryption (Password-Protected Entries)
- **Algorithm**: AES-256-GCM
- **Key Derivation**: Argon2id
  - Time: 3 iterations
  - Memory: 64 MB
  - Parallelism: 4 threads
  - Salt: 16-byte random value per entry (stored in `salt` field)
- **Nonce**: 12 bytes, randomly generated per encryption
- **Security Properties**:
  - Each entry uses a unique random salt, preventing rainbow table attacks
  - Same password + same text = different ciphertext (due to random salt and nonce)
  - Salt stored with entry, required for decryption

### Server-Key Encryption (File Storage)
- **Algorithm**: AES-256-GCM
- **Key**: 32-byte random key generated on first run, stored in `.server_key`
- **Nonce**: 12 bytes, randomly generated per write
- **Scope**: All entry data encrypted before writing to disk
- **Security Properties**:
  - Files on disk contain only encrypted data
  - Provides defense-in-depth for public entries and TOTP secrets
  - Obfuscation against casual inspection
  - **Limitation**: Server key stored in same directory as data
    - Does NOT protect against attackers with filesystem access
    - Does NOT protect against disk theft or backup exposure
    - Key and data stored together by design (simpler deployment)
  - For stronger security: use in-memory mode or separate key storage

### Password Hashing
- **Algorithm**: bcrypt (DefaultCost)
- Used for verification, not encryption

### CSRF Protection
- Token-based CSRF protection for all POST requests
- Tokens scoped to client IP + User-Agent

### Rate Limiting
- HTTP endpoints: 60 requests/minute per IP
- Password attempts: 5/minute per IP+key
- TOTP attempts: 10/minute per IP+key

---

## Technical

### Stack
- **Language:** Go 1.21+
- **Web:** stdlib `net/http`
- **QR:** `github.com/skip2/go-qrcode`
- **TOTP:** `github.com/pquerna/otp`
- **Password Hashing:** `golang.org/x/crypto/bcrypt`
- **Key Derivation:** `golang.org/x/crypto/argon2`
- **Embedding:** `//go:embed`
- **Binary:** CGO disabled, static build

### Store Interface
```go
type Store interface {
    Get(key string) *Entry
    Put(key string, e *Entry)
}
```

Implementations:
- `MemoryStore` — In-memory map storage (no encryption)
- `FileStore` — File-based persistence with server-key encryption

### Entry Lifecycle

**Create (GenerateShortcode/GenerateRandomShortcode)**:
```
1. Generate key (deterministic from text+password OR random)
2. Check for existing entry with same key
3. If exists and same content → return existing
4. If new → create Entry:
   - Public: Text = content
   - Password-protected: generate salt → encrypt → EncryptedData
   - TOTP: Text = content, TOTPSecret = generated secret
5. Store.Put(key, entry)
   - MemoryStore: map[key] = entry
   - FileStore: marshal → encrypt with server key → write file
```

**Retrieve (Store.Get)**:
```
1. Store.Get(key)
   - MemoryStore: return map[key]
   - FileStore: read file → decrypt with server key → unmarshal
2. If password-protected AND has password:
   - Entry.DecryptWithPassword(password) → plaintext
3. If TOTP-protected AND valid code:
   - Return stored Text directly
```

**Update (updateEntry)**:
```
1. Store.Get(key) → existing entry
2. For password-protected: generate NEW salt → re-encrypt
3. For public/TOTP: update Text directly
4. Store.Put(key, entry)
   - FileStore re-encrypts with new nonce (same server key)
```

**Restore (startup with --data-dir)**:
```
1. Check for .server_key file
2. If missing AND has entry files → ERROR (inconsistent state)
3. If missing AND empty → generate new server key
4. If present → load server key
5. All subsequent Get() calls decrypt with loaded key
```

### Entry Struct
```go
type Entry struct {
    Text          string    // Decrypted text (public entries only)
    EncryptedData string    // Base64(nonce + ciphertext), empty if not protected
    Salt          string    // Base64(random 16-byte salt), required for password-protected
    Protected     bool      // true if password or TOTP protected
    Password      string    // bcrypt hash, empty if TOTP or none
    TOTPSecret    string    // Base32 secret, empty if password or none
    UpdatedAt     time.Time
}
```

### File Structure
```
.
├── main.go             # CLI, install, server setup
├── handlers.go         # HTTP handlers
├── store.go            # Shortcode generation, store interface
├── filestore.go        # File-based persistence with server-key encryption
├── crypto.go           # AES-256-GCM encryption, Argon2id key derivation
├── config.go           # CLI configuration parsing
├── static/
│   ├── index.html      # Main page
│   ├── view.html       # QR display page
│   ├── lock.html       # Password/TOTP unlock form
│   └── setup-totp.html # TOTP setup page
├── handlers_test.go    # Unit tests
├── integration_test.go # Integration tests
├── test.sh             # Shell-based curl tests
├── go.mod
├── go.go               # Go package metadata
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

Result: ~8MB static binary, no runtime dependencies.

---

## Constraints
- No external services (no DB, no Redis)
- No runtime dependencies
- Works offline after build
- QR codes generated on-demand
- Binary path fixed at `/usr/local/bin/goqrly` (for install)
- Recent list hidden by default (privacy first)
- File-based storage encrypts all entries with server key at rest

---

## Tests
```bash
make test          # Build + run all tests (recommended)
make unit-test     # Unit tests only
./test.sh          # Run tests directly (builds first)
```

## Development
```bash
make build         # Build binary
make test          # Run tests
make lint          # Format + vet
make run           # Build and run locally
make clean         # Remove artifacts
make help          # Show all targets
```
