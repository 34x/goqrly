package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"regexp"

	"github.com/skip2/go-qrcode"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type ViewData struct {
	Key           string
	Text          string
	TextHTML      template.HTML
	WrongPassword bool
	IsTOTP        bool
}

type LockData struct {
	Key           string
	WrongPassword bool
	IsTOTP        bool
}

type SetupTOTPData struct {
	Secret      string
	SecretQR    string // base64 QR code of the otpauth URI
	Text        string
	WrongCode   bool
}

type IndexData struct {
	Recent         []RecentItem
	ShowRecentList bool
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

	indexTmpl.Execute(w, IndexData{Recent: recent, ShowRecentList: listRecentPublic})
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

		setupTOTPTmpl.Execute(w, SetupTOTPData{
			Secret:   secret,
			SecretQR: qrBase64,
			Text:     text,
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

		setupTOTPTmpl.Execute(w, SetupTOTPData{
			Secret:   entry.TOTPSecret,
			SecretQR: qrBase64,
			Text:     entry.Text,
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

		// Validate the code
		if !ValidateTOTP(secret, code) {
			// Show error page
			uri := buildTOTPURI(secret)
			qrPNG, _ := qrcode.Encode(uri, qrcode.Medium, 512)
			qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

			setupTOTPTmpl.Execute(w, SetupTOTPData{
				Secret:    secret,
				SecretQR:  qrBase64,
				Text:      text,
				WrongCode: true,
			})
			return
		}

		// Create entry with the verified secret
		entry := &Entry{
			Text:       text,
			Protected:  true,
			TOTPSecret: secret,
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

	// Handle unlock POST
	if r.Method == http.MethodPost {
		r.ParseForm()

		if entry.TOTPSecret != "" {
			// TOTP unlock
			code := r.FormValue("code")
			if ValidateTOTP(entry.TOTPSecret, code) {
				textHTML := renderMarkdown(entry.Text)
				textHTML = injectQRCodesIntoHTML(string(textHTML))
				viewTmpl.Execute(w, ViewData{
					Key: key, Text: entry.Text, TextHTML: textHTML,
					IsTOTP: true,
				})
				return
			}
		} else if entry.Password != "" {
			// Password unlock
			password := r.FormValue("password")
			if hashPassword(password) == entry.Password {
				textHTML := renderMarkdown(entry.Text)
				textHTML = injectQRCodesIntoHTML(string(textHTML))
				viewTmpl.Execute(w, ViewData{
					Key: key, Text: entry.Text, TextHTML: textHTML,
				})
				return
			}
		}

		// Wrong credentials - show appropriate lock form
		if entry.TOTPSecret != "" {
			lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true, IsTOTP: true})
		} else {
			lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true})
		}
		return
	}

	// If protected, show lock form
	if entry.Protected {
		if entry.TOTPSecret != "" {
			lockTmpl.Execute(w, LockData{Key: key, IsTOTP: true})
		} else {
			lockTmpl.Execute(w, LockData{Key: key})
		}
		return
	}

	// No password - show QR directly with markdown rendered and QR codes injected
	textHTML := renderMarkdown(entry.Text)
	textHTML = injectQRCodesIntoHTML(string(textHTML))
	viewTmpl.Execute(w, ViewData{
		Key: key, Text: entry.Text, TextHTML: textHTML,
	})
}

// buildTOTPURI generates the otpauth URI for a TOTP secret
func buildTOTPURI(secret string) string {
	// Label format required by FreeOTP: otpauth://totp/LABEL?secret=SECRET
	return fmt.Sprintf("otpauth://totp/goqrly?secret=%s", secret)
}
