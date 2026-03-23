package main

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"sync"
)

// CSRFTokenSize is the number of random bytes in a CSRF token
const CSRFTokenSize = 32

// CSRFStore manages CSRF tokens with expiration
type CSRFStore struct {
	mu     sync.RWMutex
	tokens map[string]string // token -> client key (IP+UA)
}

// NewCSRFStore creates a new CSRF store
func NewCSRFStore() *CSRFStore {
	return &CSRFStore{
		tokens: make(map[string]string),
	}
}

// GenerateToken creates a new token for the given client key (IP+User-Agent)
func (s *CSRFStore) GenerateToken(clientKey string) string {
	token := generateSecureToken(CSRFTokenSize)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = clientKey

	return token
}

// ValidateToken checks if the token is valid for the given client key
func (s *CSRFStore) ValidateToken(token, clientKey string) bool {
	if token == "" || clientKey == "" {
		return false
	}

	s.mu.RLock()
	expectedKey, exists := s.tokens[token]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(expectedKey), []byte(clientKey)) != 1 {
		return false
	}

	// Token can only be used once
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()

	return true
}

// Cleanup removes all tokens (should be called periodically)
func (s *CSRFStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = make(map[string]string)
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
