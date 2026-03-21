package main

import (
	"encoding/base64"
	"net/http"
)

type ViewData struct {
	Key           string
	Text          string
	QRBase64      string
	WrongPassword bool
}

type LockData struct {
	Key           string
	WrongPassword bool
}

type IndexData struct {
	Recent []RecentItem
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

	indexTmpl.Execute(w, IndexData{Recent: recent})
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

	key, _ := GenerateShortcode(text, password)

	recentCodes = append(recentCodes, key)
	if len(recentCodes) > 100 {
		recentCodes = recentCodes[len(recentCodes)-100:]
	}

	http.Redirect(w, r, "/"+key, http.StatusFound)
}

func handleQR(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	entry := store.Get(key)
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	// If password protected, redirect to view page for unlock
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
		password := r.FormValue("password")
		if hashPassword(password) == entry.Password {
			// Password correct - show QR
			qrBase64 := base64.StdEncoding.EncodeToString(entry.QR)
			viewTmpl.Execute(w, ViewData{Key: key, Text: entry.Text, QRBase64: qrBase64})
			return
		}
		// Wrong password - show lock form
		lockTmpl.Execute(w, LockData{Key: key, WrongPassword: true})
		return
	}

	// If password protected, show lock form
	if entry.Protected {
		lockTmpl.Execute(w, LockData{Key: key})
		return
	}

	// No password - show QR directly
	qrBase64 := base64.StdEncoding.EncodeToString(entry.QR)
	viewTmpl.Execute(w, ViewData{Key: key, Text: entry.Text, QRBase64: qrBase64})
}
