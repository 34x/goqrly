package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const serverKeyFile = ".server_key"

// FileStore implements Store with file-based persistence.
// All entries are encrypted at rest using a server key before being written to disk.
type FileStore struct {
	mu        sync.RWMutex
	dataDir   string
	serverKey []byte // Key for encrypting all persisted entries
}

// storedEntry is the JSON representation for disk storage.
// This struct is encrypted with the server key before being written to disk.
type storedEntry struct {
	Key           string    `json:"key"`
	Text          string    `json:"text,omitempty"`
	EncryptedData string    `json:"encrypted_data,omitempty"`
	Salt          string    `json:"salt,omitempty"`
	Protected     bool      `json:"protected"`
	Password      string    `json:"password,omitempty"`
	TOTPSecret    string    `json:"totp_secret,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NewFileStore creates a new file-based store with server-key encryption.
// Returns error if data directory exists but has entries without server key (inconsistent state).
func NewFileStore(dataDir string) (*FileStore, error) {
	fs := &FileStore{dataDir: dataDir}

	// Check if directory exists
	info, err := os.Stat(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Create directory
			if err := os.MkdirAll(dataDir, 0750); err != nil {
				return nil, fmt.Errorf("failed to create data directory: %w", err)
			}
			// Generate new server key
			if err := fs.generateServerKey(); err != nil {
				return nil, fmt.Errorf("failed to generate server key: %w", err)
			}
			return fs, nil
		}
		return nil, fmt.Errorf("failed to access data directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dataDir)
	}

	// Directory exists - check for server key
	keyPath := filepath.Join(dataDir, serverKeyFile)
	_, keyErr := os.Stat(keyPath)

	if os.IsNotExist(keyErr) {
		// No server key - check if directory has any entry files
		hasEntries, err := fs.hasEntryFiles()
		if err != nil {
			return nil, fmt.Errorf("failed to scan data directory: %w", err)
		}
		if hasEntries {
			return nil, errors.New("data directory has entries but .server_key is missing (inconsistent state)")
		}
		// Empty directory - generate new server key
		if err := fs.generateServerKey(); err != nil {
			return nil, fmt.Errorf("failed to generate server key: %w", err)
		}
		return fs, nil
	} else if keyErr != nil {
		return nil, fmt.Errorf("failed to check server key: %w", keyErr)
	}

	// Load server key
	if err := fs.loadServerKey(); err != nil {
		return nil, fmt.Errorf("failed to load server key: %w", err)
	}

	return fs, nil
}

// hasEntryFiles checks if the data directory contains any .json entry files
func (s *FileStore) hasEntryFiles() (bool, error) {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			return true, nil
		}
	}
	return false, nil
}

// Get retrieves an entry by key from disk, decrypting with server key.
// Returns nil if entry doesn't exist or decryption fails.
func (s *FileStore) Get(key string) *Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key = strings.ToLower(key)
	filePath := filepath.Join(s.dataDir, key+".json")

	// Read encrypted file
	encrypted, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	// Decrypt with server key
	plaintext, err := decryptBytes(string(encrypted), s.serverKey)
	if err != nil {
		// Decryption failed - entry may be corrupted or key mismatch
		return nil
	}

	// Parse JSON
	var se storedEntry
	if err := json.Unmarshal(plaintext, &se); err != nil {
		return nil
	}

	return storedToEntry(&se)
}

// Put stores an entry to disk, encrypting with server key.
// Uses atomic write (temp file + rename) for durability.
func (s *FileStore) Put(key string, e *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = strings.ToLower(key)
	filePath := filepath.Join(s.dataDir, key+".json")

	// Convert to stored entry and serialize
	se := entryToStored(key, e)
	jsonData, err := json.MarshalIndent(se, "", "  ")
	if err != nil {
		return
	}

	// Encrypt with server key
	encrypted, err := encryptBytes(jsonData, s.serverKey)
	if err != nil {
		return
	}

	// Write to temp file first, then rename (atomic)
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(encrypted), 0640); err != nil {
		return
	}

	os.Rename(tmpPath, filePath)
}

// ServerKey returns the server encryption key.
// Used for encrypting entries before disk storage.
func (s *FileStore) ServerKey() []byte {
	return s.serverKey
}

func (s *FileStore) generateServerKey() error {
	key := make([]byte, serverKeySize)
	if _, err := rand.Read(key); err != nil {
		return err
	}

	keyPath := filepath.Join(s.dataDir, serverKeyFile)
	data := base64.StdEncoding.EncodeToString(key)

	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(keyPath, []byte(data), 0600); err != nil {
		return err
	}

	s.serverKey = key
	return nil
}

func (s *FileStore) loadServerKey() error {
	keyPath := filepath.Join(s.dataDir, serverKeyFile)
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	key, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return fmt.Errorf("invalid server key format: %w", err)
	}

	if len(key) != serverKeySize {
		return fmt.Errorf("server key must be %d bytes, got %d", serverKeySize, len(key))
	}

	s.serverKey = key
	return nil
}

// storedToEntry converts a stored entry to an in-memory entry.
func storedToEntry(se *storedEntry) *Entry {
	return &Entry{
		Text:          se.Text,
		EncryptedData: se.EncryptedData,
		Salt:          se.Salt,
		Protected:     se.Protected,
		Password:      se.Password,
		TOTPSecret:    se.TOTPSecret,
		UpdatedAt:     se.UpdatedAt,
	}
}

// entryToStored converts an in-memory entry to a stored entry.
func entryToStored(key string, e *Entry) *storedEntry {
	return &storedEntry{
		Key:           key,
		Text:          e.Text,
		EncryptedData: e.EncryptedData,
		Salt:          e.Salt,
		Protected:     e.Protected,
		Password:      e.Password,
		TOTPSecret:    e.TOTPSecret,
		UpdatedAt:     e.UpdatedAt,
	}
}