package main

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Integration tests use httptest.NewServer to make real HTTP requests
// These test the actual HTTP round-trip behavior, not just handler logic

func setupTestServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/setup-totp/{key}", handleSetupTOTP)
	mux.HandleFunc("/{key}", handleView)

	return httptest.NewServer(mux)
}

func setupTestServerTLS(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/setup-totp/{key}", handleSetupTOTP)
	mux.HandleFunc("/{key}", handleView)

	return httptest.NewTLSServer(mux)
}

// TestIntegrationHomepage verifies the homepage loads correctly
func TestIntegrationHomepage(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to get homepage: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "goqrly") {
		t.Error("Expected 'goqrly' in response")
	}
	if !strings.Contains(string(body), "Generate") {
		t.Error("Expected 'Generate' in response")
	}
}

// TestIntegrationGenerateUnprotected creates unprotected entry
func TestIntegrationGenerateUnprotected(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // Don't follow redirects
	}}

	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text": []string{"https://example.com"},
	})
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected 302 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		t.Error("Expected Location header")
	}
	if !strings.HasPrefix(location, "/") {
		t.Errorf("Expected shortcode path, got %s", location)
	}

	// Verify the shortcode works
	resp, err = http.Get(server.URL + location)
	if err != nil {
		t.Fatalf("Failed to get shortcode page: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "example.com") {
		t.Error("Expected example.com in response")
	}
	if !strings.Contains(string(body), "data:image/png;base64") {
		t.Error("Expected QR code in response")
	}
}

// TestIntegrationGeneratePasswordProtected creates password-protected entry
func TestIntegrationGeneratePasswordProtected(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text":     []string{"https://secret.com"},
		"password": []string{"secret123"},
	})
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected 302 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")

	// GET should show lock form without content
	resp, err = http.Get(server.URL + location)
	if err != nil {
		t.Fatalf("Failed to getshortcode page: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "type=\"password\"") {
		t.Error("Expected password input in lock form")
	}
	if !strings.Contains(string(body), "Confirm") {
		t.Error("Expected Confirm button")
	}
	if strings.Contains(string(body), "secret.com") {
		t.Error("Entry text leaked in lock form")
	}
	if strings.Contains(string(body), "data:image/png;base64") {
		t.Error("QR code leaked in lock form")
	}

	// Wrong password should show error
	resp, err = http.PostForm(server.URL+location, url.Values{
		"password": []string{"wrong"},
	})
	if err != nil {
		t.Fatalf("Failed to submit password: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Wrong password") {
		t.Error("Expected 'Wrong password' error")
	}

	// Correct password should reveal content
	resp, err = http.PostForm(server.URL+location, url.Values{
		"password": []string{"secret123"},
	})
	if err != nil {
		t.Fatalf("Failed to submit password: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "secret.com") {
		t.Error("Expected secret.com after correct password")
	}
	if !strings.Contains(string(body), "data:image/png;base64") {
		t.Error("Expected QR code after correct password")
	}
}

// TestIntegrationUpdateWithPassword updates an entry with password auth
func TestIntegrationUpdateWithPassword(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create protected entry
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text":     []string{"https://original.com"},
		"password": []string{"pass123"},
	})
	location := resp.Header.Get("Location")

	// Update with correct password
	resp, err = http.PostForm(server.URL+location, url.Values{
		"text":     []string{"https://updated.com"},
		"password": []string{"pass123"},
	})
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "updated.com") {
		t.Error("Expected updated content")
	}
	if !strings.Contains(string(body), "data:image/png;base64") {
		t.Error("Expected QR code for updated content")
	}

	// Verify the update persisted - same key shows new content
	resp, err = http.PostForm(server.URL+location, url.Values{
		"password": []string{"pass123"},
	})
	if err != nil {
		t.Fatalf("Failed to verify update: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "updated.com") {
		t.Error("Update did not persist")
	}
	if strings.Contains(string(body), "original.com") {
		t.Error("Old content still present")
	}
}

// TestIntegrationNotFound returns redirect for non-existent shortcode
func TestIntegrationNotFound(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(server.URL + "/nonexistent123")
	if err != nil {
		t.Fatalf("Failed to get shortcode: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected 302 for non-existent key, got %d", resp.StatusCode)
	}
}

// TestIntegrationHTTPSEndpoint verifies HTTPS works (with self-signed cert)
func TestIntegrationHTTPSEndpoint(t *testing.T) {
	server := setupTestServerTLS(t)
	defer server.Close()

	// Skip TLS verification for self-signed cert
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to get homepage via HTTPS: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "goqrly") {
		t.Error("Expected 'goqrly' in HTTPS response")
	}
}

// TestIntegrationMarkdownRendering tests markdown content rendering
func TestIntegrationMarkdownRendering(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	markdownText := "# Heading\n\nThis is **bold** text.\n\n- Item 1\n- Item 2\n\nA link: https://example.com"

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text": []string{markdownText},
	})
	location := resp.Header.Get("Location")

	resp, err = http.Get(server.URL + location)
	if err != nil {
		t.Fatalf("Failed to get shortcode page: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should have HTML elements from markdown
	if !strings.Contains(bodyStr, "<h1>") {
		t.Error("Expected <h1> tag")
	}
	if !strings.Contains(bodyStr, "<strong>") {
		t.Error("Expected <strong> tag")
	}
	if !strings.Contains(bodyStr, "<ul>") {
		t.Error("Expected <ul> tag")
	}
	// Should have QR code for the link
	if !strings.Contains(bodyStr, "data:image/png;base64") {
		t.Error("Expected QR code for link in markdown")
	}
}

// TestIntegrationMultipleQRsInContent tests multiple QR codes for multiple links
func TestIntegrationMultipleQRsInContent(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	text := "Visit https://first.com and https://second.com for more info."

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text": []string{text},
	})
	location := resp.Header.Get("Location")

	resp, err = http.Get(server.URL + location)
	if err != nil {
		t.Fatalf("Failed to get shortcode page: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should have both URLs visible
	if !strings.Contains(bodyStr, "first.com") {
		t.Error("Expected first.com in response")
	}
	if !strings.Contains(bodyStr, "second.com") {
		t.Error("Expected second.com in response")
	}
	// Should have QR codes (count occurrences)
	qrCount := strings.Count(bodyStr, "data:image/png;base64")
	if qrCount < 2 {
		t.Errorf("Expected at least 2 QR codes, got %d", qrCount)
	}
}

// TestIntegrationCaseInsensitiveShortcode ensuresshortcode lookup is case-insensitive
func TestIntegrationCaseInsensitiveShortcode(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Reset store for this test
	store = NewMemoryStore()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text": []string{"https://example.com"},
	})
	location := resp.Header.Get("Location")
	if location == "" {
		t.Fatal("Expected Location header")
	}

	// Extract the key without the leading /
	key := strings.TrimPrefix(location, "/")

	// Try accessing with different case variations
	testKeys := []string{
		key,                  // Original case
		strings.ToUpper(key), // Uppercase
		strings.ToLower(key), // Lowercase
	}

	for _, testKey := range testKeys {
		resp, err = http.Get(server.URL + "/" + testKey)
		if err != nil {
			t.Fatalf("Failed to get shortcode /%s: %v", testKey, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 for key /%s, got %d", testKey, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "example.com") {
			t.Errorf("Expected example.com for key /%s", testKey)
		}
	}
}

// TestIntegrationContentTypes tests different content types
func TestIntegrationContentTypes(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	testCases := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "Plain text",
			text:     "Hello World",
			expected: []string{"Hello World"},
		},
		{
			name:     "Single URL",
			text:     "https://example.com",
			expected: []string{"example.com", "data:image/png"},
		},
		{
			name:     "Multiple lines",
			text:     "Line 1\nLine 2\nLine 3",
			expected: []string{"Line 1", "Line 2", "Line 3"},
		},
		{
			name:     "Special characters",
			text:     "Special: @#$%^&*()<>",
			expected: []string{"Special:", "@"},
		},
	}

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store = NewMemoryStore() // Fresh store for each test

			resp, err := client.PostForm(server.URL+"/generate", url.Values{
				"text": []string{tc.text},
			})
			if err != nil {
				t.Fatalf("Failed to generate: %v", err)
			}
			location := resp.Header.Get("Location")

			resp, err = http.Get(server.URL + location)
			if err != nil {
				t.Fatalf("Failed to get shortcode: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			for _, exp := range tc.expected {
				if !strings.Contains(bodyStr, exp) {
					t.Errorf("Expected '%s' in response", exp)
				}
			}
		})
	}
}

// TestIntegrationTimestamps verifies UpdatedAt is set correctly
func TestIntegrationTimestamps(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	store = NewMemoryStore()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Create entry
	beforeTime := time.Now()
	resp, err := client.PostForm(server.URL+"/generate", url.Values{
		"text":     []string{"https://example.com"},
		"password": []string{"pass"},
	})
	location := resp.Header.Get("Location")
	afterTime := time.Now()

	// Check initial timestamp
	key := strings.TrimPrefix(location, "/")
	entry := store.Get(key)
	if entry == nil {
		t.Fatal("Entry not found")
	}
	if entry.UpdatedAt.Before(beforeTime) || entry.UpdatedAt.After(afterTime) {
		t.Error("UpdatedAt not within expected time range")
	}

	// Sleep a bit to ensure timestamp would be different
	time.Sleep(10 * time.Millisecond)

	// Update entry
	updateTime := time.Now()
	resp, err = client.PostForm(server.URL+location, url.Values{
		"text":     []string{"https://updated.com"},
		"password": []string{"pass"},
	})
	afterUpdate := time.Now()

	// Verify timestamp was updated
	entry = store.Get(key)
	if entry == nil {
		t.Fatal("Entry not found after update")
	}
	if entry.UpdatedAt.Before(updateTime) || entry.UpdatedAt.After(afterUpdate) {
		t.Error("UpdatedAt not updated correctly")
	}

	// Verify content was updated
	resp, err = http.PostForm(server.URL+location, url.Values{
		"password": []string{"pass"},
	})
	if err != nil {
		t.Fatalf("Failed to get updated entry: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "updated.com") {
		t.Error("Content not updated")
	}
}