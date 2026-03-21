package main

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/skip2/go-qrcode"
)

const minLen = 3
const maxLen = 6

type Entry struct {
	Text string
	QR   []byte
}

type Store struct {
	entries map[string]*Entry
}

func NewStore() *Store {
	return &Store{entries: make(map[string]*Entry)}
}

func (s *Store) Get(key string) *Entry {
	return s.entries[strings.ToLower(key)]
}

func (s *Store) Put(key string, e *Entry) {
	s.entries[strings.ToLower(key)] = e
}

func GenerateShortcode(text string) (string, *Entry) {
	text = strings.TrimSpace(text)
	entry := &Entry{Text: text}

	qr, err := qrcode.Encode(text, qrcode.Medium, 512)
	if err != nil {
		qr, _ = qrcode.Encode("error", qrcode.Medium, 512)
	}
	entry.QR = qr

	for length := minLen; length <= maxLen; length++ {
		key := generateKey(text, length, 0)
		existing := store.Get(key)
		if existing == nil {
			store.Put(key, entry)
			return key, entry
		}
		if existing.Text == text {
			return key, existing
		}
	}

	for salt := 1; ; salt++ {
		for length := minLen; length <= maxLen; length++ {
			key := generateKey(text, length, salt)
			existing := store.Get(key)
			if existing == nil {
				store.Put(key, entry)
				return key, entry
			}
			if existing.Text == text {
				return key, existing
			}
		}
	}
}

func generateKey(text string, length, salt int) string {
	data := text
	if salt > 0 {
		data = text + string(rune('0'+salt))
	}
	hash := sha256.Sum256([]byte(data))
	encoded := base64.RawURLEncoding.EncodeToString(hash[:])
	return strings.ToLower(encoded[:length])
}
