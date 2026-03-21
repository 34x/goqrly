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
	mux.HandleFunc("/qr/{key}", handleQR)
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
	
	key, _ := GenerateShortcode("Test123", "")

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
	if !strings.Contains(w.Body.String(), "Unlock") {
		t.Error("Expected 'Unlock' in locked response")
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
	
	key, _ := GenerateShortcode("Secret", "secret123")

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
	if strings.Contains(w.Body.String(), "Unlock") {
		t.Error("Should NOT contain 'Unlock' after correct password")
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

func TestHandleQRUnprotected(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	key, _ := GenerateShortcode("Test", "")

	req := httptest.NewRequest(http.MethodGet, "/qr/"+key, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("Expected Content-Type image/png, got %s", w.Header().Get("Content-Type"))
	}
}

func TestHandleQRProtected(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	key, _ := GenerateShortcode("Secret", "pass")

	req := httptest.NewRequest(http.MethodGet, "/qr/"+key, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302 for protected QR, got %d", w.Code)
	}
}

func TestHandleQRNotFound(t *testing.T) {
	store = NewStore()
	mux := testMux()
	
	req := httptest.NewRequest(http.MethodGet, "/qr/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHashPassword(t *testing.T) {
	hash1 := hashPassword("test")
	hash2 := hashPassword("test")
	hash3 := hashPassword("different")

	if hash1 != hash2 {
		t.Error("Same password should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("Different passwords should produce different hashes")
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
	if len(entry.QR) == 0 {
		t.Error("Expected non-empty QR")
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
