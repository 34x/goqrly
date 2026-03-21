package main

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/skip2/go-qrcode"
)

type ViewData struct {
	Key           string
	Text          string
	QRBase64      string
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
	QRBase64 string
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
				qrBase64 := base64.StdEncoding.EncodeToString(entry.QR)
				recent = append(recent, RecentItem{Key: key, Text: text, QRBase64: qrBase64})
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
		qrPNG, err := qrcode.Encode(totpURI, qrcode.Medium, 256)
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
		qrPNG, err := qrcode.Encode(totpURI, qrcode.Medium, 256)
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
			qrPNG, _ := qrcode.Encode(uri, qrcode.Medium, 256)
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
		qr, _ := qrcode.Encode(text, qrcode.Medium, 512)
		entry.QR = qr

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

func handleQR(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	entry := store.Get(key)
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	// If protected, redirect to view page for unlock
	if entry.Protected {
		http.Redirect(w, r, "/"+key, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(entry.QR)
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
				qrBase64 := base64.StdEncoding.EncodeToString(entry.QR)
				viewTmpl.Execute(w, ViewData{Key: key, Text: entry.Text, QRBase64: qrBase64, IsTOTP: true})
				return
			}
		} else if entry.Password != "" {
			// Password unlock
			password := r.FormValue("password")
			if hashPassword(password) == entry.Password {
				qrBase64 := base64.StdEncoding.EncodeToString(entry.QR)
				viewTmpl.Execute(w, ViewData{Key: key, Text: entry.Text, QRBase64: qrBase64})
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

	// No password - show QR directly
	qrBase64 := base64.StdEncoding.EncodeToString(entry.QR)
	viewTmpl.Execute(w, ViewData{Key: key, Text: entry.Text, QRBase64: qrBase64})
}

// buildTOTPURI generates the otpauth URI for a TOTP secret
func buildTOTPURI(secret string) string {
	// Label format required by FreeOTP: otpauth://totp/LABEL?secret=SECRET
	return fmt.Sprintf("otpauth://totp/goqrly?secret=%s", secret)
}
