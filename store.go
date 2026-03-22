package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"strings"

	"github.com/pquerna/otp/totp"
)

const minLen = 3
const maxLen = 6

type Entry struct {
	Text       string
	Protected  bool
	Password   string // bcrypt hash, empty if no password
	TOTPSecret string // base32 secret, empty if not TOTP
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

// GenerateTOTPSecret creates a new TOTP secret for setup
func GenerateTOTPSecret() (string, error) {
	// Generate random bytes for secret
	secretBytes := make([]byte, 20)
	_, err := rand.Read(secretBytes)
	if err != nil {
		return "", err
	}
	
	// Encode as base32
	secret := base32.StdEncoding.EncodeToString(secretBytes)
	return secret, nil
}

// ValidateTOTP checks if the provided code is valid for the secret
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateShortcode creates a shortcode for text with optional password protection
func GenerateShortcode(text, password string) (string, *Entry) {
	text = strings.TrimSpace(text)
	entry := &Entry{Text: text, Protected: password != ""}

	if password != "" {
		entry.Password = hashPassword(password)
	}

	// Include password in key generation
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

// GenerateRandomShortcode creates a random shortcode for TOTP entries
func GenerateRandomShortcode() (string, *Entry) {
	for length := minLen; length <= maxLen; length++ {
		key := generateRandomKey(length)
		if store.Get(key) == nil {
			return key, nil
		}
	}
	// Fallback to longer keys
	key := generateRandomKey(maxLen + 2)
	return key, nil
}

func generateKey(data string, length, salt int) string {
	if salt > 0 {
		data = data + string(rune('0'+salt))
	}
	hash := sha256.Sum256([]byte(data))
	encoded := base64.RawURLEncoding.EncodeToString(hash[:])
	return strings.ToLower(encoded[:length])
}

func generateRandomKey(length int) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	rand.Read(result)
	for i := range result {
		result[i] = chars[int(result[i])%len(chars)]
	}
	return string(result)
}

func generateRandomBase32(length int) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	result := make([]byte, length)
	rand.Read(result)
	for i := range result {
		result[i] = chars[int(result[i])%len(chars)]
	}
	return string(result)
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
