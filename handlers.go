package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// CSRF store instance
var csrfStore = NewCSRFStore()

// HTTP rate limiter middleware (global instance)
var authHTTPLimiter = NewHTTPLimiter(60, time.Minute, LimitByIP)

// Rate limiters for auth attempts (per IP + key)
var (
	totpRateLimiter     = NewRateLimiter(10, time.Minute) // 10 TOTP attempts per minute per IP
	passwordRateLimiter = NewRateLimiter(5, time.Minute)  // 5 password attempts per minute per IP
)

type ViewData struct {
	Key           string
	Text          string
	TextHTML      template.HTML
	WrongPassword bool
	IsTOTP        bool
	PendingText   string // For edit confirmation
	Password      bool   // True if password protected (for showing edit form)
	UpdatedAt     time.Time
	Updated       bool   // True if this response is from a successful edit
	CSRFToken     string // CSRF token for form submission
}

type LockData struct {
	Key           string
	WrongPassword bool
	IsTOTP        bool
	PendingText   string // For edit confirmation
	CSRFToken     string // CSRF token for form submission
	RateLimited   bool   // True if rate limit was exceeded
}

type SetupTOTPData struct {
	Secret      string
	SecretQR    string // base64 QR code of the otpauth URI
	Text        string
	WrongCode   bool
	CSRFToken   string // CSRF token for form submission
	RateLimited bool   // True if rate limit was exceeded
}

type IndexData struct {
	Recent         []RecentItem
	ShowRecentList bool
	CSRFToken      string // CSRF token for form submission
}

type RecentItem struct {
	Key      string
	Text     string
	TextHTML template.HTML
}

// URL regex pattern for detecting links
var urlPattern = regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)

// markdownRenderer is a shared goldmark instance
var markdownRenderer goldmark.Markdown

func init() {
	markdownRenderer = goldmark.New(
		goldmark.WithExtensions(extension.Linkify),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}

// clientKey extracts a unique identifier from the request (IP + User-Agent)
func clientKey(r *http.Request) string {
	ip := LimitByIP(r)
	ua := r.Header.Get("User-Agent")
	return fmt.Sprintf("%s:%s", ip, ua)
}

// csrfMiddleware ensures CSRF validation for POST requests
func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For POST requests, validate CSRF token
		if r.Method == http.MethodPost {
			client := clientKey(r)
			token := r.FormValue("csrf_token")
			if !csrfStore.ValidateToken(token, client) {
				http.Error(w, "Invalid or missing CSRF token. Please refresh the page and try again.", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// setupMux configures all HTTP routes - single source of truth for routing
func setupMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/setup-totp", handleSetupTOTP)
	mux.HandleFunc("/{key}", handleView)

	return mux
}

// handlerWithMiddleware wraps a handler with CSRF and rate limiting middleware
func handlerWithMiddleware(h http.Handler) http.Handler {
	// First apply CSRF middleware (session management + CSRF validation)
	h = csrfMiddleware(h)

	// Then apply HTTP rate limiting middleware
	h = authHTTPLimiter.Middleware(h)

	return h
}

// renderMarkdown converts markdown text to HTML
func renderMarkdown(text string) template.HTML {
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(text), &buf); err != nil {
		return template.HTML(text) // Fallback to plain text on error
	}
	return template.HTML(buf.String())
}

// isURL checks if the text is a valid URL
func isURL(text string) bool {
	matched, _ := regexp.MatchString(`^https?://`, text)
	return matched
}

// extractURL extracts a URL from text if present
func extractURL(text string) string {
	match := urlPattern.FindString(text)
	return match
}

// extractURLsFromHTML extracts all unique URLs from rendered HTML
func extractURLsFromHTML(html string) []string {
	var urls []string
	re := regexp.MustCompile(`href="([^"]+)"`)
	matches := re.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool)
	for _, m := range matches {
		url := m[1]
		if !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	return urls
}

// injectQRCodesIntoHTML adds QR code images after each link in the HTML
func injectQRCodesIntoHTML(html string) template.HTML {
	// Pattern to match <a href="...">...</a>
	linkRe := regexp.MustCompile(`(<a\s+href="([^"]+)"[^>]*>)(.*?)(</a>)`)

	// Generate QR for each URL once
	urlQRs := make(map[string]string)
	linkRe2 := regexp.MustCompile(`href="([^"]+)"`)
	for _, matches := range linkRe2.FindAllStringSubmatch(html, -1) {
		url := matches[1]
		if _, ok := urlQRs[url]; !ok {
			qrPNG, err := qrcode.Encode(url, qrcode.Medium, 512)
			if err == nil {
				qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)
				urlQRs[url] = qrBase64
			}
		}
	}

	// Replace each <a href="URL">...</a> with <a href="URL">...</a><br><img src="QR">
	result := linkRe.ReplaceAllStringFunc(html, func(match string) string {
		// Extract URL from match
		urlMatch := regexp.MustCompile(`href="([^"]+)"`)
		urlMatches := urlMatch.FindStringSubmatch(match)
		if len(urlMatches) < 2 {
			return match
		}
		url := urlMatches[1]
		qrBase64, ok := urlQRs[url]
		if !ok {
			return match
		}
		// Insert QR code image after the link on a new line
		return match + `<img src="data:image/png;base64,` + qrBase64 + `" style="display:block;width:100%;" alt="QR">`
	})

	return template.HTML(result)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	client := clientKey(r)
	csrfToken := csrfStore.GenerateToken(client)

	recent := make([]RecentItem, 0, recentMax)
	if listRecentPublic {
		for i := len(recentCodes) - 1; i >= 0 && len(recent) < recentMax; i-- {
			key := recentCodes[i]
			entry := store.Get(key)
			if entry != nil && !entry.Protected {
				text := entry.Text
				if len(text) > 50 {
					text = text[:47] + "..."
				}
				textHTML := renderMarkdown(entry.Text)
				recent = append(recent, RecentItem{Key: key, Text: text, TextHTML: textHTML})
			}
		}
	}

	indexTmpl.Execute(w, IndexData{Recent: recent, ShowRecentList: listRecentPublic, CSRFToken: csrfToken})
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	text := r.FormValue("text")
	if text == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	password := r.FormValue("password")
	totp := r.FormValue("totp")

	// TOTP setup - generate secret and show QR
	if totp == "on" {
		secret, err := GenerateTOTPSecret()
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Generate QR code for the TOTP secret
		totpURI := buildTOTPURI(secret)
		qrPNG, err := qrcode.Encode(totpURI, qrcode.Medium, 512)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

		sessionID := clientKey(r)
		csrfToken := csrfStore.GenerateToken(sessionID)

		setupTOTPTmpl.Execute(w, SetupTOTPData{
			Secret:    secret,
			SecretQR:  qrBase64,
			Text:      text,
			CSRFToken: csrfToken,
		})
		return
	}

	// Password or no protection
	key, _ := GenerateShortcode(text, password)

	recentCodes = append(recentCodes, key)
	if len(recentCodes) > 100 {
		recentCodes = recentCodes[len(recentCodes)-100:]
	}

	http.Redirect(w, r, "/"+key, http.StatusFound)
}

func handleSetupTOTP(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	entry := store.Get(key)

	// GET - show setup page if entry needs TOTP setup
	if r.Method == http.MethodGet {
		if entry == nil || entry.TOTPSecret == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Generate QR for the secret
		totpURI := buildTOTPURI(entry.TOTPSecret)
		qrPNG, err := qrcode.Encode(totpURI, qrcode.Medium, 512)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

		client := clientKey(r)
		csrfToken := csrfStore.GenerateToken(client)

		setupTOTPTmpl.Execute(w, SetupTOTPData{
			Secret:    entry.TOTPSecret,
			SecretQR:  qrBase64,
			Text:      entry.Text,
			CSRFToken: csrfToken,
		})
		return
	}

	// POST - verify code and complete setup
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		secret := r.FormValue("secret")
		code := r.FormValue("code")
		text := r.FormValue("text")

		// Rate limit TOTP attempts
		clientIP := LimitByIP(r)
		if !totpRateLimiter.Allow(clientIP) {
			client := clientKey(r)
			csrfToken := csrfStore.GenerateToken(client)
			uri := buildTOTPURI(secret)
			qrPNG, _ := qrcode.Encode(uri, qrcode.Medium, 512)
			qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

			setupTOTPTmpl.Execute(w, SetupTOTPData{
				Secret:      secret,
				SecretQR:    qrBase64,
				Text:        text,
				WrongCode:   true,
				CSRFToken:   csrfToken,
				RateLimited: true,
			})
			return
		}

		// Validate the code
		if !ValidateTOTP(secret, code) {
			// Show error page
			uri := buildTOTPURI(secret)
			qrPNG, _ := qrcode.Encode(uri, qrcode.Medium, 512)
			qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

			client := clientKey(r)
			csrfToken := csrfStore.GenerateToken(client)

			setupTOTPTmpl.Execute(w, SetupTOTPData{
				Secret:    secret,
				SecretQR:  qrBase64,
				Text:      text,
				WrongCode: true,
				CSRFToken: csrfToken,
			})
			return
		}

		// Create entry with the verified secret
		entry := &Entry{
			Text:       text,
			Protected:  true,
			TOTPSecret: secret,
			UpdatedAt:  time.Now(),
		}

		// Generate random shortcode
		key, _ := GenerateRandomShortcode()
		store.Put(key, entry)

		recentCodes = append(recentCodes, key)
		if len(recentCodes) > 100 {
			recentCodes = recentCodes[len(recentCodes)-100:]
		}

		http.Redirect(w, r, "/"+key, http.StatusFound)
		return
	}

	http.NotFound(w, r)
}

func handleView(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	entry := store.Get(key)
	if entry == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	client := clientKey(r)
	csrfToken := csrfStore.GenerateToken(client)

	// GET: show content or lock form
	if r.Method == http.MethodGet {
		if entry.Protected {
			if entry.TOTPSecret != "" {
				lockTmpl.Execute(w, LockData{Key: key, IsTOTP: true, CSRFToken: csrfToken})
			} else {
				lockTmpl.Execute(w, LockData{Key: key, CSRFToken: csrfToken})
			}
			return
		}
		// Public entry
		showContent(w, key, entry)
		return
	}

	// POST: parse form
	r.ParseForm()
	text := r.FormValue("text")
	password := r.FormValue("password")
	code := r.FormValue("code") // TOTP

	// POST with text only → show auth form (protected) or content (public)
	if text != "" && password == "" && code == "" {
		if entry.Protected {
			if entry.TOTPSecret != "" {
				lockTmpl.Execute(w, LockData{Key: key, IsTOTP: true, PendingText: text, CSRFToken: csrfToken})
			} else {
				lockTmpl.Execute(w, LockData{Key: key, PendingText: text, CSRFToken: csrfToken})
			}
			return
		}
		// Public entry: ignore text, show content
		showContent(w, key, entry)
		return
	}

	clientIP := LimitByIP(r)

	// POST with password (and optionally text) → verify
	if entry.TOTPSecret != "" {
		// TOTP verification
		if code != "" && ValidateTOTP(entry.TOTPSecret, code) {
			// Reset rate limit on success
			totpRateLimiter.Reset(clientIP + ":" + key)
			updated := false
			if text != "" {
				updateEntry(key, text, "") // TOTP entries not encrypted in this stage
				entry = store.Get(key)
				updated = true
			}
			showContentUpdated(w, key, entry, updated, csrfToken)
			return
		}

		// Rate limit failed TOTP attempts
		if !totpRateLimiter.Allow(clientIP + ":" + key) {
			lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true, IsTOTP: true, PendingText: text, CSRFToken: csrfToken, RateLimited: true})
			return
		}
	} else if entry.Password != "" {
		// Password verification
		if password != "" && verifyPassword(entry.Password, password) {
			// Reset rate limit on success
			passwordRateLimiter.Reset(clientIP + ":" + key)
			
			// Decrypt the entry text
			var decryptedText string
			if entry.EncryptedData != "" {
				decrypted, err := entry.DecryptWithKey(password, key)
				if err != nil {
					// Decryption failed - show error
					lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true, PendingText: text, CSRFToken: csrfToken})
					return
				}
				decryptedText = decrypted
			} else {
				decryptedText = entry.Text
			}
			
			updated := false
			if text != "" {
				updateEntry(key, text, password)
				decryptedText = text
				updated = true
			}
			showContentDecrypted(w, key, entry, decryptedText, updated, csrfToken)
			return
		}

		// Rate limit failed password attempts
		if !passwordRateLimiter.Allow(clientIP + ":" + key) {
			lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true, PendingText: text, CSRFToken: csrfToken, RateLimited: true})
			return
		}
	}

	// Verification failed → show auth form
	if entry.TOTPSecret != "" {
		lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true, IsTOTP: true, PendingText: text, CSRFToken: csrfToken})
	} else {
		lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true, PendingText: text, CSRFToken: csrfToken})
	}
}

func showContentUpdated(w http.ResponseWriter, key string, entry *Entry, updated bool, csrfToken string) {
	textHTML := renderMarkdown(entry.Text)
	textHTML = injectQRCodesIntoHTML(string(textHTML))
	viewTmpl.Execute(w, ViewData{
		Key: key, Text: entry.Text, TextHTML: textHTML,
		IsTOTP:    entry.TOTPSecret != "",
		Password:  entry.Password != "",
		UpdatedAt: entry.UpdatedAt,
		Updated:   updated,
		CSRFToken: csrfToken,
	})
}

func showContent(w http.ResponseWriter, key string, entry *Entry) {
	showContentUpdated(w, key, entry, false, "")
}

// showContentDecrypted displays content with pre-decrypted text (for password-protected entries)
func showContentDecrypted(w http.ResponseWriter, key string, entry *Entry, text string, updated bool, csrfToken string) {
	textHTML := renderMarkdown(text)
	textHTML = injectQRCodesIntoHTML(string(textHTML))
	viewTmpl.Execute(w, ViewData{
		Key: key, Text: text, TextHTML: textHTML,
		IsTOTP:    entry.TOTPSecret != "",
		Password:  entry.Password != "",
		UpdatedAt: entry.UpdatedAt,
		Updated:   updated,
		CSRFToken: csrfToken,
	})
}

// buildTOTPURI generates the otpauth URI for a TOTP secret
func buildTOTPURI(secret string) string {
	// Label format required by FreeOTP: otpauth://totp/LABEL?secret=SECRET
	return fmt.Sprintf("otpauth://totp/goqrly?secret=%s", secret)
}

// updateEntry updates the text of an existing entry
// For password-protected entries, encrypts the text with the provided password
func updateEntry(key, text, password string) *Entry {
	entry := store.Get(key)
	if entry == nil {
		return nil
	}

	entry.UpdatedAt = time.Now()

	if entry.Password != "" && password != "" {
		// Password-protected entry: encrypt the new text
		encKey := deriveKey(password, key)
		encrypted, err := encrypt(text, encKey)
		if err != nil {
			// Fallback to unencrypted on error
			entry.Text = text
			entry.EncryptedData = ""
		} else {
			entry.Text = ""
			entry.EncryptedData = encrypted
		}
	} else {
		// Public entry: store text directly
		entry.Text = text
	}

	return entry
}
