package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
