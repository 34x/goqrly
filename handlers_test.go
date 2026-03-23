package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// setupMux is defined in handlers.go - single source of truth for routing

// HTTP handler tests (need mux for routing)
func TestHandleIndex(t *testing.T) {
	mux := setupMux()

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
	mux := setupMux()

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
	mux := setupMux()

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
	mux := setupMux()

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
	mux := setupMux()

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
	mux := setupMux()

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
	mux := setupMux()

	req := httptest.NewRequest(http.MethodGet, "/nonexistent123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}
}

func TestHandleViewPostTextOnlyPublic(t *testing.T) {
	store = NewStore()
	mux := setupMux()

	key, _ := GenerateShortcode("https://original.com", "")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=https://new.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

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

func TestHandleViewPostTextOnlyProtected(t *testing.T) {
	store = NewStore()
	mux := setupMux()

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

func TestHandleViewPostTextAndPasswordUpdate(t *testing.T) {
	store = NewStore()
	mux := setupMux()

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

func TestHandleViewPostTextAndWrongPassword(t *testing.T) {
	store = NewStore()
	mux := setupMux()

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

func TestHandleViewUpdatePreservesKey(t *testing.T) {
	store = NewStore()
	mux := setupMux()

	key, _ := GenerateShortcode("https://original.com", "pass123")

	req := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("text=https://updated.com&password=pass123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req2 := httptest.NewRequest(http.MethodPost, "/"+key, strings.NewReader("password=pass123"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if !strings.Contains(w2.Body.String(), "updated.com") {
		t.Error("Same key should show updated content")
	}
}

func TestHandleGenerateEmptyText(t *testing.T) {
	store = NewStore()
	mux := setupMux()

	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}
}

func TestHandleGenerateSpecialChars(t *testing.T) {
	store = NewStore()
	mux := setupMux()

	text := "https://example.com/?id=123&ref=abc#section"
	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text="+url.QueryEscape(text)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	location := w.Header().Get("Location")
	key := strings.TrimPrefix(location, "/")

	req2 := httptest.NewRequest(http.MethodGet, "/"+key, nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if !strings.Contains(w2.Body.String(), "id=123") {
		t.Error("URL parameters should be preserved")
	}
}

func TestHandleViewSpecialKey(t *testing.T) {
	store = NewStore()
	mux := setupMux()

	keys := []string{"abc123", "XYZ", "aaa", "zzz", "test-1"}
	for _, key := range keys {
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
	mux := setupMux()

	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("text=https://example.com&totp=on"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for TOTP setup, got %d", w.Code)
	}
	if len(w.Body.String()) == 0 {
		t.Error("TOTP setup should return content")
	}
}

// Password hashing tests (no mux needed)
func TestHashPassword(t *testing.T) {
	hash1 := hashPassword("test")
	hash2 := hashPassword("test")

	if hash1 == "" || hash2 == "" {
		t.Error("Hash should not be empty")
	}
	if !verifyPassword(hash1, "test") || !verifyPassword(hash2, "test") {
		t.Error("Hash should verify original password")
	}
	if verifyPassword(hash1, "different") {
		t.Error("Hash should not verify wrong password")
	}
}

func TestPasswordHashEmpty(t *testing.T) {
	hash := hashPassword("")
	if hash == "" {
		t.Error("Empty password should still produce a hash")
	}
	if !verifyPassword(hash, "") {
		t.Error("Empty password hash should verify empty password")
	}
}

func TestPasswordHashSpecialChars(t *testing.T) {
	passwords := []string{
		"p@ssw0rd!#$%",
		"日本語パスワード",
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
	hash := hashPassword(strings.Repeat("a", 50))
	if hash == "" {
		t.Error("Long password should produce hash")
	}
	if !verifyPassword(hash, strings.Repeat("a", 50)) {
		t.Error("Long password should verify correctly")
	}
}

func TestPasswordVerifyWrongHash(t *testing.T) {
	if verifyPassword("$2a$10$invalidhashthatisnotvalidbcrpt", "password") {
		t.Error("Invalid bcrypt hash should be rejected")
	}
}

// TOTP tests (no mux needed)
func TestTOTPSecretGeneration(t *testing.T) {
	store = NewStore()
	key, entry := GenerateRandomShortcode()

	if key == "" {
		t.Error("Expected non-empty key")
	}
	if entry != nil {
		t.Error("Expected nil entry from GenerateRandomShortcode")
	}

	secret := generateRandomBase32(20)
	if secret == "" || len(secret) < 20 {
		t.Error("TOTP secret should be at least 20 characters")
	}

	for _, c := range secret {
		if !((c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7')) {
			t.Errorf("Invalid base32 character: %c", c)
		}
	}
}

func TestValidateTOTP(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"

	if ValidateTOTP(secret, "000000") {
		t.Error("Invalid TOTP code should be rejected")
	}
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

// Shortcode tests (no mux needed)
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

func TestShortcodeCollisionHandling(t *testing.T) {
	store = NewStore()

	key1, _ := GenerateShortcode("test", "pass")
	key2, _ := GenerateShortcode("test", "pass")

	if key1 != key2 {
		t.Error("Same input should produce same key")
	}

	e1 := store.Get(key1)
	e2 := store.Get(key2)
	if e1 != e2 {
		t.Error("Collision should return same entry")
	}
}

func TestShortcodeCaseInsensitivity(t *testing.T) {
	store = NewStore()
	key, _ := GenerateShortcode("Hello", "")

	for _, testKey := range []string{key, strings.ToUpper(key), strings.ToLower(key)} {
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

	key, entry := GenerateShortcode("", "")
	if key == "" {
		t.Error("Empty text should still generate key")
	}
	if entry == nil {
		t.Error("Entry should exist")
	}
}

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
	for i := 0; i < 1000; i++ {
		key, _ := GenerateRandomShortcode()
		if key == "" {
			t.Error("Should always find a key")
		}
	}
}

// Store tests (no mux needed)
func TestStoreGetPut(t *testing.T) {
	s := NewStore()

	e := &Entry{Text: "Test content", UpdatedAt: time.Now()}
	s.Put("testkey", e)

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

	if s.Get("nonexistent") != nil {
		t.Error("Non-existent key should return nil")
	}
}

func TestStorePutNilEntry(t *testing.T) {
	s := NewStore()

	s.Put("test", nil)

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

	if s.Get("key").Text != "Second" {
		t.Error("Entry should be overwritten")
	}
}

// Entry structure tests (no mux needed)
func TestEntryFields(t *testing.T) {
	now := time.Now()
	e := &Entry{
		Text:       "Test",
		Protected:  true,
		Password:   hashPassword("pass"),
		TOTPSecret: "JBSWY3DPEHPK3PXP",
		UpdatedAt:  now,
	}

	if e.Text != "Test" || !e.Protected || e.Password == "" || e.TOTPSecret != "JBSWY3DPEHPK3PXP" || e.UpdatedAt.IsZero() {
		t.Error("Entry fields incorrect")
	}
}

func TestEntryZeroValue(t *testing.T) {
	var e Entry

	if e.Text != "" || e.Protected || e.Password != "" || e.TOTPSecret != "" {
		t.Error("Zero value Entry should have all empty/false fields")
	}
}
