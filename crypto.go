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
	argonTime     = 3
	argonMemory   = 64 * 1024 // 64 MB
	argonParallel = 4
	argonKeyLen   = 32        // AES-256

	// Nonce size for GCM (12 bytes is standard)
	nonceSize = 12
)

// deriveKey generates an encryption key from a password and salt using Argon2id
func deriveKey(password, salt string) []byte {
	return argon2.IDKey(
		[]byte(password),
		[]byte(salt),
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