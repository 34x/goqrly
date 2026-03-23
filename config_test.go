package main

import (
	"strconv"
	"testing"
)

// ============================================================
// RUN MODE TESTS
// ============================================================

func TestRun_DefaultPort(t *testing.T) {
	cfg := ParseArgs([]string{})
	if cfg.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Port)
	}
}

func TestRun_NoTLSByDefault(t *testing.T) {
	cfg := ParseArgs([]string{})
	if cfg.TLSEnabled {
		t.Error("TLS should not be enabled by default")
	}
}

func TestRun_ExplicitPort(t *testing.T) {
	tests := []struct {
		name  string
		port  int
		tls   bool
	}{
		{"port 3000", 3000, false},
		{"port 8080", 8080, false},
		{"port 9000", 9000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ParseArgs([]string{"--port", itoa(tt.port)})
			if cfg.Port != tt.port {
				t.Errorf("Expected port %d, got %d", tt.port, cfg.Port)
			}
			if cfg.TLSEnabled != tt.tls {
				t.Errorf("Expected TLS=%v, got %v", tt.tls, cfg.TLSEnabled)
			}
		})
	}
}

func TestRun_Port443AutoTLS(t *testing.T) {
	cfg := ParseArgs([]string{"--port", "443"})
	if cfg.Port != 443 {
		t.Errorf("Expected port 443, got %d", cfg.Port)
	}
	if !cfg.TLSEnabled {
		t.Error("TLS should be auto-enabled for port 443")
	}
}

func TestRun_Port8443AutoTLS(t *testing.T) {
	cfg := ParseArgs([]string{"--port", "8443"})
	if cfg.Port != 8443 {
		t.Errorf("Expected port 8443, got %d", cfg.Port)
	}
	if !cfg.TLSEnabled {
		t.Error("TLS should be auto-enabled for port 8443")
	}
}

func TestRun_ExplicitTLSKeepsPort(t *testing.T) {
	// --tls alone should NOT auto-change port to 443
	cfg := ParseArgs([]string{"--tls"})
	if cfg.Port != 8080 {
		t.Errorf("Port should remain 8080 with --tls only, got %d", cfg.Port)
	}
	if !cfg.TLSEnabled {
		t.Error("TLS should be enabled with --tls")
	}
}

func TestRun_TLSWithCustomPort(t *testing.T) {
	cfg := ParseArgs([]string{"--tls", "--port", "9000"})
	if cfg.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Port)
	}
	if !cfg.TLSEnabled {
		t.Error("TLS should be enabled")
	}
}

func TestRun_TLSPortOrderIndependent(t *testing.T) {
	cfg1 := ParseArgs([]string{"--tls", "--port", "9000"})
	cfg2 := ParseArgs([]string{"--port", "9000", "--tls"})

	if cfg1.Port != cfg2.Port {
		t.Errorf("Port should be same regardless of order: %d vs %d", cfg1.Port, cfg2.Port)
	}
	if cfg1.TLSEnabled != cfg2.TLSEnabled {
		t.Error("TLS should be enabled regardless of flag order")
	}
}

func TestRun_CustomCerts(t *testing.T) {
	cfg := ParseArgs([]string{"--cert", "/path/cert.pem", "--key", "/path/key.pem"})
	if !cfg.TLSEnabled {
		t.Error("TLS should be enabled with custom certs")
	}
	if cfg.CertFile != "/path/cert.pem" {
		t.Errorf("CertFile mismatch: got %s", cfg.CertFile)
	}
	if cfg.KeyFile != "/path/key.pem" {
		t.Errorf("KeyFile mismatch: got %s", cfg.KeyFile)
	}
}

func TestRun_TLSDisable(t *testing.T) {
	cfg := ParseArgs([]string{"--port", "443", "--tls-disable"})
	if cfg.TLSEnabled {
		t.Error("--tls-disable should prevent auto-TLS on port 443")
	}
}

func TestRun_OtherPortsNoTLS(t *testing.T) {
	for _, port := range []int{80, 3000, 5000, 8080, 10000} {
		cfg := ParseArgs([]string{"--port", itoa(port)})
		if cfg.TLSEnabled {
			t.Errorf("TLS should NOT be enabled for port %d", port)
		}
	}
}

func TestRun_RecentMax(t *testing.T) {
	cfg := ParseArgs([]string{"--recent", "25"})
	if cfg.RecentMax != 25 {
		t.Errorf("RecentMax should be 25, got %d", cfg.RecentMax)
	}
}

func TestRun_ListRecentPublic(t *testing.T) {
	cfg := ParseArgs([]string{"--list-recent-public"})
	if !cfg.ListRecentPublic {
		t.Error("ListRecentPublic should be true")
	}
}

func TestRun_Combined(t *testing.T) {
	cfg := ParseArgs([]string{
		"--port", "9000",
		"--recent", "20",
		"--list-recent-public",
	})
	if cfg.Port != 9000 || cfg.RecentMax != 20 || !cfg.ListRecentPublic {
		t.Error("Combined options not parsed correctly")
	}
}

// ============================================================
// INSTALL MODE TESTS
// ============================================================

func TestInstall_DefaultPort(t *testing.T) {
	cfg := ParseArgs([]string{"install"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 443 {
		t.Errorf("Install default port should be 443, got %d", installCfg.Port)
	}
}

func TestInstall_DefaultTLS(t *testing.T) {
	cfg := ParseArgs([]string{"install"})
	installCfg := cfg.InstallConfig()

	if !installCfg.TLSEnabled {
		t.Error("Install should enable TLS by default")
	}
}

func TestInstall_ExplicitPort443(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--port", "443"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 443 {
		t.Errorf("Install port should be 443, got %d", installCfg.Port)
	}
	if !installCfg.TLSEnabled {
		t.Error("Install on port 443 should enable TLS")
	}
}

func TestInstall_ExplicitPort8443(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--port", "8443"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 8443 {
		t.Errorf("Install port should be 8443, got %d", installCfg.Port)
	}
	if !installCfg.TLSEnabled {
		t.Error("Install on port 8443 should enable TLS")
	}
}

func TestInstall_ExplicitPort8080(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--port", "8080"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 8080 {
		t.Errorf("Install port should be 8080, got %d", installCfg.Port)
	}
	if installCfg.TLSEnabled {
		t.Error("Install on port 8080 should NOT auto-enable TLS")
	}
}

func TestInstall_ExplicitPort3000(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--port", "3000"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 3000 {
		t.Errorf("Install port should be 3000, got %d", installCfg.Port)
	}
	if installCfg.TLSEnabled {
		t.Error("Install on port 3000 should NOT auto-enable TLS")
	}
}

func TestInstall_ExplicitTLS(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--tls"})
	installCfg := cfg.InstallConfig()

	if !installCfg.TLSEnabled {
		t.Error("Install with --tls should enable TLS")
	}
}

func TestInstall_TLSDisable(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--port", "443", "--tls-disable"})
	installCfg := cfg.InstallConfig()

	if installCfg.TLSEnabled {
		t.Error("--tls-disable should prevent auto-TLS on port 443")
	}
}

func TestInstall_CustomCerts(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--cert", "/certs/cert.pem", "--key", "/certs/key.pem"})
	installCfg := cfg.InstallConfig()

	if !installCfg.TLSEnabled {
		t.Error("Install with custom certs should enable TLS")
	}
	if installCfg.CertFile != "/certs/cert.pem" || installCfg.KeyFile != "/certs/key.pem" {
		t.Error("Cert files should be preserved")
	}
}

func TestInstall_Port8080WithTLSCerts(t *testing.T) {
	// Even on port 8080, custom certs should enable TLS
	cfg := ParseArgs([]string{"install", "--port", "8080", "--cert", "/certs/cert.pem", "--key", "/certs/key.pem"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 8080 {
		t.Errorf("Port should be 8080, got %d", installCfg.Port)
	}
	if !installCfg.TLSEnabled {
		t.Error("Custom certs should enable TLS")
	}
}

func TestInstall_Combined(t *testing.T) {
	cfg := ParseArgs([]string{"install", "--port", "3000", "--recent", "5"})
	installCfg := cfg.InstallConfig()

	if installCfg.Port != 3000 {
		t.Errorf("Port should be 3000, got %d", installCfg.Port)
	}
	if installCfg.TLSEnabled {
		t.Error("Port 3000 should not auto-enable TLS")
	}
}

// ============================================================
// UNINSTALL MODE TESTS
// ============================================================

func TestUninstall_Command(t *testing.T) {
	cfg := ParseArgs([]string{"uninstall"})
	if cfg.Command != "uninstall" {
		t.Errorf("Command should be 'uninstall', got %s", cfg.Command)
	}
}

func TestUninstall_WithRemoveBinary(t *testing.T) {
	cfg := ParseArgs([]string{"uninstall", "--remove-binary"})
	if cfg.Command != "uninstall" {
		t.Error("Command should be 'uninstall'")
	}
}

// ============================================================
// EDGE CASES
// ============================================================

func TestRun_PortBelow1024(t *testing.T) {
	// Ports below 1024 should be accepted (user needs sudo)
	cfg := ParseArgs([]string{"--port", "80"})
	if cfg.Port != 80 {
		t.Errorf("Port 80 should be accepted, got %d", cfg.Port)
	}
}

func TestRun_DuplicatePorts(t *testing.T) {
	cfg := ParseArgs([]string{"--port", "3000", "--port", "9000"})
	if cfg.Port != 9000 {
		t.Errorf("Last port should win, got %d", cfg.Port)
	}
}

func TestRun_PortFlagNoValue(t *testing.T) {
	cfg := ParseArgs([]string{"--port"})
	if cfg.Port != 8080 {
		t.Errorf("Should fall back to default, got %d", cfg.Port)
	}
}

func TestRun_UnknownFlagsIgnored(t *testing.T) {
	cfg := ParseArgs([]string{"--unknown", "value", "--port", "9000"})
	if cfg.Port != 9000 {
		t.Errorf("Port should still be parsed, got %d", cfg.Port)
	}
}

// Test helper
func itoa(i int) string {
	return strconv.Itoa(i)
}
