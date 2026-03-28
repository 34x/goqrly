package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	// Argon2id parameters for key derivation
	// These parameters are chosen based on OWASP recommendations:
	// - Time: 3 iterations (balances security and performance)
	// - Memory: 64 MB (resistant to GPU attacks)
	// - Parallelism: 4 threads (reasonable for modern CPUs)
	// - Key length: 32 bytes (AES-256)
	argonTime     = 3
	argonMemory   = 64 * 1024 // 64 MB
	argonParallel = 4
	argonKeyLen   = 32 // AES-256

	// Nonce size for GCM (12 bytes is standard/recommended)
	nonceSize = 12

	// Salt size for key derivation (16 bytes = 128 bits, per OWASP recommendation)
	saltSize = 16

	// Server key size (32 bytes for AES-256)
	serverKeySize = 32
)

// generateSalt creates a cryptographically secure random salt for key derivation.
// Returns a 16-byte (128-bit) salt, which is sufficient per OWASP guidelines.
func generateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// deriveKey generates an encryption key from a password and salt using Argon2id.
// The salt must be a cryptographically random value (use generateSalt()).
// Returns a 32-byte key suitable for AES-256.
func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		argonTime, argonMemory, argonParallel, argonKeyLen,
	)
}

// encrypt encrypts plaintext using AES-256-GCM with the given key.
// Returns base64-encoded (nonce + ciphertext) or error.
func encrypt(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Combine nonce + ciphertext and encode as base64
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// decrypt decrypts base64-encoded (nonce + ciphertext) using AES-256-GCM with the given key.
// Returns plaintext string or error.
func decrypt(encrypted string, key []byte) (string, error) {
	combined, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	if len(combined) < nonceSize {
		return "", errors.New("encrypted data too short")
	}

	nonce := combined[:nonceSize]
	ciphertext := combined[nonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// encryptBytes encrypts a byte slice using AES-256-GCM with the given key.
// Returns base64-encoded (nonce + ciphertext) or error.
// Used for server-key encryption of persisted entries.
func encryptBytes(plaintext []byte, key []byte) (string, error) {
	if len(key) != serverKeySize {
		return "", errors.New("key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Combine nonce + ciphertext and encode as base64
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// decryptBytes decrypts base64-encoded (nonce + ciphertext) using AES-256-GCM.
// Returns plaintext bytes or error.
// Used for server-key decryption of persisted entries.
func decryptBytes(encrypted string, key []byte) ([]byte, error) {
	if len(key) != serverKeySize {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}

	combined, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}

	if len(combined) < nonceSize {
		return nil, errors.New("encrypted data too short")
	}

	nonce := combined[:nonceSize]
	ciphertext := combined[nonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}