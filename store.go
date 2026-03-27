package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/pquerna/otp/totp"
)

const minLen = 3
const maxLen = 6

// Entry represents a QR code entry
type Entry struct {
	Text          string    // Decrypted text (populated only for public entries or after decryption)
	EncryptedData string    // Base64-encoded (nonce + ciphertext), empty if not protected
	Protected     bool      // True if password or TOTP protected
	Password      string    // bcrypt hash, empty if no password
	TOTPSecret    string    // base32 secret, empty if not TOTP
	UpdatedAt     time.Time
}

// Decrypt attempts to decrypt the entry text using the provided password.
// Returns the decrypted text and true if successful, or empty string and false on failure.
// For non-password-protected entries, returns Text and true.
func (e *Entry) Decrypt(password string) (string, bool) {
	if !e.Protected || e.Password == "" {
		// Not password-protected, return text directly
		return e.Text, true
	}

	if e.EncryptedData == "" {
		// Legacy entry without encryption (shouldn't happen)
		return e.Text, true
	}

	if password == "" {
		return "", false
	}

	// Need to derive key, but we need the shortcode as salt
	// This is called from handlers where we have access to the key
	return "", false
}

// DecryptWithKey decrypts the entry text using password and shortcode (used as salt).
// Returns decrypted text or error.
func (e *Entry) DecryptWithKey(password, shortcode string) (string, error) {
	if e.EncryptedData == "" {
		return e.Text, nil
	}

	key := deriveKey(password, shortcode)
	return decrypt(e.EncryptedData, key)
}

// Store defines the interface for entry storage
type Store interface {
	Get(key string) *Entry
	Put(key string, e *Entry)
}

// MemoryStore implements Store with in-memory storage
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]*Entry
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]*Entry)}
}

func (s *MemoryStore) Get(key string) *Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries[strings.ToLower(key)]
}

func (s *MemoryStore) Put(key string, e *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

	// Include password in key generation
	data := text
	if password != "" {
		data = text + "\x00" + password
	}

	// Find an available key
	for length := minLen; length <= maxLen; length++ {
		key := generateKey(data, length, 0)
		existing := store.Get(key)
		if existing == nil {
			// Found empty slot - create new entry
			return createEntry(key, text, password)
		}
		// Check if this is the same entry
		if isSameEntry(existing, key, text, password) {
			return key, existing
		}
	}

	// Try with salt for collision resolution
	for salt := 1; ; salt++ {
		for length := minLen; length <= maxLen; length++ {
			key := generateKey(data, length, salt)
			existing := store.Get(key)
			if existing == nil {
				// Found empty slot - create new entry
				return createEntry(key, text, password)
			}
			// Check if this is the same entry
			if isSameEntry(existing, key, text, password) {
				return key, existing
			}
		}
	}
}

// createEntry creates and stores a new entry
func createEntry(key, text, password string) (string, *Entry) {
	entry := &Entry{Protected: password != "", UpdatedAt: time.Now()}

	if password != "" {
		entry.Password = hashPassword(password)
		// Encrypt text with key derived from password + shortcode
		encKey := deriveKey(password, key)
		encrypted, err := encrypt(text, encKey)
		if err != nil {
			// Fallback to unencrypted on error (should not happen)
			entry.Text = text
		} else {
			entry.EncryptedData = encrypted
		}
	} else {
		entry.Text = text
	}

	store.Put(key, entry)
	return key, entry
}

// isSameEntry checks if an existing entry matches the new text and password
func isSameEntry(existing *Entry, key, text, password string) bool {
	if existing == nil {
		return false
	}

	// For password-protected entries, verify password matches
	if password != "" && existing.Password != "" {
		if !verifyPassword(existing.Password, password) {
			return false
		}
		// Password matches - decrypt and compare text
		if existing.EncryptedData != "" {
			encKey := deriveKey(password, key)
			decrypted, err := decrypt(existing.EncryptedData, encKey)
			if err != nil {
				return false
			}
			return decrypted == text
		}
		// Legacy unencrypted entry
		return existing.Text == text
	}

	// For unprotected entries, compare text directly
	if password == "" && existing.Password == "" {
		return existing.Text == text
	}

	return false
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
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// Fallback to empty password on error (should not happen with valid input)
		return ""
	}
	return string(bytes)
}

func verifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
