package main

import (
	"net/http"
)

type ViewData struct {
	Key  string
	Text string
}

type IndexData struct {
	Recent []RecentItem
}

type RecentItem struct {
	Key  string
	Text string
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
		if entry != nil {
			text := entry.Text
			if len(text) > 50 {
				text = text[:47] + "..."
			}
			recent = append(recent, RecentItem{Key: key, Text: text})
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

	key, _ := GenerateShortcode(text)

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
	viewTmpl.Execute(w, ViewData{Key: key, Text: entry.Text})
}
