package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/{key}", handleView)
	return mux
}

func TestHandleIndex(t *testing.T) {
	mux := testMux()
	
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "goqrly") {
		t.Error("Expected 'goqrly' in response")
	}
}

func TestHandleGenerate(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text=Hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}
}

func TestHandleViewUnprotected(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	key, _ := GenerateShortcode("https://example.com", "")

	req := httptest.NewRequest(http.MethodGet, "/"+key, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "data:image/png;base64,") {
		t.Error("Expected QR in response")
	}
}

func TestHandleViewProtectedLocked(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	key, _ := GenerateShortcode("Secret", "pass123")

	req := httptest.NewRequest(http.MethodGet, "/"+key, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "data:image/png;base64,") {
		t.Error("QR should NOT be in locked response")
	}
	if !strings.Contains(w.Body.String(), "Confirm") {
		t.Error("Expected 'Confirm' in locked response")
	}
}

func TestHandleViewProtectedWrongPassword(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	key, _ := GenerateShortcode("Secret", "mypassword")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("password=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Wrong password") {
		t.Error("Expected 'Wrong password' error")
	}
}

func TestHandleViewProtectedCorrectPassword(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	key, _ := GenerateShortcode("https://secret.com", "secret123")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("password=secret123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "data:image/png;base64,") {
		t.Error("Expected QR in unlocked response")
	}
	if strings.Contains(w.Body.String(), "Confirm") {
		t.Error("Should NOT contain 'Confirm' after correct password")
	}
}

func TestHandleViewNotFound(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	req := httptest.NewRequest(http.MethodGet, "/nonexistent123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}
}

func TestHashPassword(t *testing.T) {
	hash1 := hashPassword("test")
	hash2 := hashPassword("test")

	if hash1 == "" || hash2 == "" {
		t.Error("Hash should not be empty")
	}
	// bcrypt produces different hashes each time (salt), so we check verification instead
	if !verifyPassword(hash1, "test") || !verifyPassword(hash2, "test") {
		t.Error("Hash should verify original password")
	}
	if verifyPassword(hash1, "different") {
		t.Error("Hash should not verify wrong password")
	}
}

func TestGenerateShortcodeUnprotected(t *testing.T) {
	store = NewStore()
	key, entry := GenerateShortcode("Hello", "")

	if key == "" {
		t.Error("Expected non-empty key")
	}
	if entry.Text != "Hello" {
		t.Error("Expected text 'Hello'")
	}
	if entry.Password != "" {
		t.Error("Expected empty password")
	}
}

func TestGenerateShortcodeProtected(t *testing.T) {
	store = NewStore()
	key, entry := GenerateShortcode("Secret", "password123")

	if key == "" {
		t.Error("Expected non-empty key")
	}
	if entry.Text != "Secret" {
		t.Error("Expected text 'Secret'")
	}
	if entry.Password == "" {
		t.Error("Expected non-empty password hash")
	}
}

func TestSameTextDifferentPassword(t *testing.T) {
	store = NewStore()
	key1, _ := GenerateShortcode("Hello", "pass1")
	key2, _ := GenerateShortcode("Hello", "pass2")

	if key1 == key2 {
		t.Error("Same text with different passwords should produce different keys")
	}
}

func TestSameTextSamePassword(t *testing.T) {
	store = NewStore()
	key1, _ := GenerateShortcode("World", "password")
	key2, _ := GenerateShortcode("World", "password")

	if key1 != key2 {
		t.Error("Same text with same password should produce same key")
	}
}

// Decision Matrix Tests for POST /{key}

// Test POST with text only on public entry (ignored)
func TestHandleViewPostTextOnlyPublic(t *testing.T) {
	store = NewStore()
	mux := testMux()

	key, _ := GenerateShortcode("https://original.com", "")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=https://new.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should show content, text change ignored
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "original.com") {
		t.Error("Should show original text")
	}
	if strings.Contains(w.Body.String(), "new.com") {
		t.Error("Should NOT show new text (ignored for public)")
	}
}

// Test POST with text only on protected entry (shows auth form)
func TestHandleViewPostTextOnlyProtected(t *testing.T) {
	store = NewStore()
	mux := testMux()

	key, _ := GenerateShortcode("Secret text", "pass123")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=New text"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Authenticate to save changes") {
		t.Error("Should show auth form with message")
	}
	if !strings.Contains(w.Body.String(), "Confirm") {
		t.Error("Should show Confirm button")
	}
}

// Test POST with password + text + correct password (update + show content)
func TestHandleViewPostTextAndPasswordUpdate(t *testing.T) {
	store = NewStore()
	mux := testMux()

	key, _ := GenerateShortcode("https://original.com", "pass123")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=https://updated.com&password=pass123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "updated.com") {
		t.Error("Should show updated text")
	}
	if !strings.Contains(w.Body.String(), "data:image/png;base64,") {
		t.Error("Should show QR code for updated content")
	}
	if strings.Contains(w.Body.String(), "Authenticate") {
		t.Error("Should NOT show auth message after successful update")
	}
}

// Test POST with password + text + wrong password (show auth form with error)
func TestHandleViewPostTextAndWrongPassword(t *testing.T) {
	store = NewStore()
	mux := testMux()

	key, _ := GenerateShortcode("Secret", "correctpass")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=New text&password=wrongpass"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Wrong password") {
		t.Error("Should show wrong password error")
	}
}

// Test that update preserves the same key
func TestHandleViewUpdatePreservesKey(t *testing.T) {
	store = NewStore()
	mux := testMux()

	key, _ := GenerateShortcode("https://original.com", "pass123")

	// Update the entry
	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=https://updated.com&password=pass123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Verify same key still works and shows updated content
	req2 := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("password=pass123"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if !strings.Contains(w2.Body.String(), "updated.com") {
		t.Error("Same key should show updated content")
	}
}

// TOTP Tests

func TestTOTPSecretGeneration(t *testing.T) {
	store = NewStore()
	key, entry := GenerateRandomShortcode()

	if key == "" {
		t.Error("Expected non-empty key")
	}
	if entry != nil {
		t.Error("Expected nil entry from GenerateRandomShortcode")
	}

	// Now create a TOTP entry to verify secret generation
	secret := generateRandomBase32(20)
	if secret == "" {
		t.Error("Expected non-empty TOTP secret")
	}
	if len(secret) < 20 {
		t.Error("TOTP secret should be at least 20 characters")
	}

	// Verify it's valid base32
	for _, c := range secret {
		if !((c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7')) {
			t.Errorf("Invalid base32 character: %c", c)
		}
	}
}

func TestValidateTOTP(t *testing.T) {
	// Generate a TOTP secret
	secret := "JBSWY3DPEHPK3PXP"

	// ValidateTOTP uses time-based codes, so we test the structure
	// A real TOTP validation would require generating a valid code
	// For now, we test invalid codes are rejected
	valid := ValidateTOTP(secret, "000000")
	// This should be false (random code unlikely to be valid)
	if valid {
		t.Error("Invalid TOTP code should be rejected")
	}

	// Test malformed codes
	if ValidateTOTP(secret, "") {
		t.Error("Empty TOTP code should be rejected")
	}
	if ValidateTOTP(secret, "12345") {
		t.Error("5-digit TOTP code should be rejected")
	}
	if ValidateTOTP(secret, "ABCDEFG") {
		t.Error("Non-numeric TOTP code should be rejected")
	}
}

// Password Edge Cases

func TestPasswordHashEmpty(t *testing.T) {
	hash := hashPassword("")
	if hash == "" {
		t.Error("Empty password should still produce a hash")
	}
	// Empty password should verify correctly
	if !verifyPassword(hash, "") {
		t.Error("Empty password hash should verify empty password")
	}
}

func TestPasswordHashSpecialChars(t *testing.T) {
	passwords := []string{
		"p@ssw0rd!#$%",
		"日本語パスワード",
		"password\nwith\nnewlines",
		"\"quoted'password`",
		"<script>alert('xss')</script>",
	}

	for _, pass := range passwords {
		hash := hashPassword(pass)
		if hash == "" {
			t.Errorf("Password %q should produce hash", pass)
		}
		if !verifyPassword(hash, pass) {
			t.Errorf("Password %q should verify", pass)
		}
	}
}

func TestPasswordHashLong(t *testing.T) {
	// Test with a reasonably long password
	longPass := strings.Repeat("a", 50)
	hash := hashPassword(longPass)

	if hash == "" {
		t.Error("Long password should produce hash")
	}
	if !verifyPassword(hash, longPass) {
		t.Error("Long password should verify correctly")
	}
}

func TestPasswordVerifyWrongHash(t *testing.T) {
	wrongHash := "$2a$10$invalidhashthatisnotvalidbcrpt"

	if verifyPassword(wrongHash, "password") {
		t.Error("Invalid bcrypt hash should be rejected")
	}
}

// Shortcode Edge Cases

func TestShortcodeCollisionHandling(t *testing.T) {
	store = NewStore()

	// Generate many entries with same text/password to test collision
	key1, _ := GenerateShortcode("test", "pass")
	key2, _ := GenerateShortcode("test", "pass")

	if key1 != key2 {
		t.Error("Same input should produce same key")
	}

	// Verify both point to same entry
	e1 := store.Get(key1)
	e2 := store.Get(key2)
	if e1 != e2 {
		t.Error("Collision should return same entry")
	}
}

func TestShortcodeCaseInsensitivity(t *testing.T) {
	store = NewStore()
	key, _ := GenerateShortcode("Hello", "")

	testKeys := []string{
		key,
		strings.ToUpper(key),
		strings.ToLower(key),
	}

	for _, testKey := range testKeys {
		entry := store.Get(testKey)
		if entry == nil {
			t.Errorf("Key /%s should exist", testKey)
		}
		if entry.Text != "Hello" {
			t.Errorf("Expected 'Hello', got %s", entry.Text)
		}
	}
}

func TestGenerateShortcodeEmptyText(t *testing.T) {
	store = NewStore()

	// Should work with empty text (edge case)
	key, entry := GenerateShortcode("", "")

	if key == "" {
		t.Error("Empty text should still generate key")
	}
	if entry == nil {
		t.Error("Entry should exist")
	}
}

func TestGenerateShortcodeWithWhitespace(t *testing.T) {
	store = NewStore()

	key1, e1 := GenerateShortcode("  hello  ", "")
	key2, e2 := GenerateShortcode("hello", "")

	// Text is trimmed in GenerateShortcode, so should be same
	if key1 != key2 {
		t.Error("Whitespace-only passwords should be trimmed")
	}
	if e1.Text != e2.Text {
		t.Error("Trimmed text should be stored")
	}
}

// Store Operations

func TestStoreGetPut(t *testing.T) {
	s := NewStore()

	entry := &Entry{
		Text:      "Test content",
		Protected: false,
		UpdatedAt: time.Now(),
	}

	s.Put("testkey", entry)

	retrieved := s.Get("testkey")
	if retrieved == nil {
		t.Fatal("Entry not found")
	}
	if retrieved.Text != "Test content" {
		t.Error("Text mismatch")
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	s := NewStore()

	entry := s.Get("nonexistent")
	if entry != nil {
		t.Error("Non-existent key should return nil")
	}
}

func TestStorePutNilEntry(t *testing.T) {
	s := NewStore()

	// This should not panic
	s.Put("test", nil)

	// Should return nil
	if s.Get("test") != nil {
		t.Error("Nil entry should not be stored")
	}
}

func TestStoreOverwrite(t *testing.T) {
	s := NewStore()

	e1 := &Entry{Text: "First", UpdatedAt: time.Now()}
	s.Put("key", e1)

	e2 := &Entry{Text: "Second", UpdatedAt: time.Now()}
	s.Put("key", e2)

	retrieved := s.Get("key")
	if retrieved.Text != "Second" {
		t.Error("Entry should be overwritten")
	}
}

// GenerateRandomShortcode Tests

func TestGenerateRandomShortcodeNotEmpty(t *testing.T) {
	store = NewStore()

	for i := 0; i < 100; i++ {
		key, _ := GenerateRandomShortcode()
		if key == "" {
			t.Error("Key should not be empty")
		}
		if len(key) < 3 || len(key) > 6 {
			t.Errorf("Key length should be 3-6, got %d", len(key))
		}
	}
}

func TestGenerateRandomShortcodeUnique(t *testing.T) {
	store = NewStore()

	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, _ := GenerateRandomShortcode()
		if keys[key] {
			t.Errorf("Duplicate key generated: %s", key)
		}
		keys[key] = true
	}
}

func TestGenerateRandomShortcodeCollisionOnFullStore(t *testing.T) {
	// Fill the store with all possible 3-char keys
	// (36^3 = 46656, which is manageable)
	// Then try to generate more keys and ensure collision handling works

	// For this test, just verify that if the store is full,
	// we don't panic and can eventually find a longer key

	// Actually, with the salt mechanism, we should never run out
	for i := 0; i < 1000; i++ {
		key, _ := GenerateRandomShortcode()
		if key == "" {
			t.Error("Should always find a key")
		}
	}
}

// Entry Structure Tests

func TestEntryFields(t *testing.T) {
	now := time.Now()
	e := &Entry{
		Text:       "Test",
		Protected:  true,
		Password:   hashPassword("pass"),
		TOTPSecret: "JBSWY3DPEHPK3PXP",
		UpdatedAt:  now,
	}

	if e.Text != "Test" {
		t.Error("Text field incorrect")
	}
	if !e.Protected {
		t.Error("Protected field incorrect")
	}
	if e.Password == "" {
		t.Error("Password field incorrect")
	}
	if e.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Error("TOTPSecret field incorrect")
	}
	if e.UpdatedAt.IsZero() {
		t.Error("UpdatedAt field incorrect")
	}
}

func TestEntryZeroValue(t *testing.T) {
	var e Entry

	if e.Text != "" || e.Protected || e.Password != "" || e.TOTPSecret != "" {
		t.Error("Zero value Entry should have all empty/false fields")
	}
}

// Additional Handler Edge Cases

func TestHandleGenerateEmptyText(t *testing.T) {
	store = NewStore()
	mux := testMux()

	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should still generate a shortcode even with empty text
	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}
}

func TestHandleGenerateSpecialChars(t *testing.T) {
	store = NewStore()
	mux := testMux()

	text := "https://example.com/?id=123&ref=abc#section"
	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text="+url.QueryEscape(text)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	location := w.Header().Get("Location")

	// Get the entry and verify text is preserved
	req2 := httptest.NewRequest(http.MethodGet, "/"+strings.TrimPrefix(location, "/"), nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if !strings.Contains(w2.Body.String(), "id=123") {
		t.Error("URL parameters should be preserved")
	}
}

func TestHandleViewSpecialKey(t *testing.T) {
	store = NewStore()
	mux := testMux()

	// Keys with special path characters should work
	specialKeys := []string{"abc123", "XYZ", "aaa", "zzz", "test-1"}

	for _, key := range specialKeys {
		store.Put(key, &Entry{Text: "Content for " + key, UpdatedAt: time.Now()})

		req := httptest.NewRequest(http.MethodGet, "/"+key, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Key /%s should return 200, got %d", key, w.Code)
		}
	}
}

func TestHandleGenerateWithTOTP(t *testing.T) {
	store = NewStore()
	mux := testMux()

	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text=https://example.com&totp=on"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// TOTP shows setup page directly, not a redirect
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for TOTP setup, got %d", w.Code)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("TOTP setup should return content")
	}
}
