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
	Text      string
	Password  string // bcrypt hash, empty if no password
	Protected bool   // true if password protected
	QR        []byte
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

func GenerateShortcode(text, password string) (string, *Entry) {
	text = strings.TrimSpace(text)
	entry := &Entry{Text: text, Protected: password != ""}

	if password != "" {
		entry.Password = hashPassword(password)
	}

	qr, err := qrcode.Encode(text, qrcode.Medium, 512)
	if err != nil {
		qr, _ = qrcode.Encode("error", qrcode.Medium, 512)
	}
	entry.QR = qr

	// Include password in key generation to get different codes for same text with different passwords
	data := text
	if password != "" {
		data = text + "\x00" + password
	}

	for length := minLen; length <= maxLen; length++ {
		key := generateKey(data, length, 0)
		existing := store.Get(key)
		if existing == nil {
			store.Put(key, entry)
			return key, entry
		}
		if existing.Text == text && existing.Password == entry.Password {
			return key, existing
		}
	}

	for salt := 1; ; salt++ {
		for length := minLen; length <= maxLen; length++ {
			key := generateKey(data, length, salt)
			existing := store.Get(key)
			if existing == nil {
				store.Put(key, entry)
				return key, entry
			}
			if existing.Text == text && existing.Password == entry.Password {
				return key, existing
			}
		}
	}
}

func generateKey(data string, length, salt int) string {
	if salt > 0 {
		data = data + string(rune('0'+salt))
	}
	hash := sha256.Sum256([]byte(data))
	encoded := base64.RawURLEncoding.EncodeToString(hash[:])
	return strings.ToLower(encoded[:length])
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
