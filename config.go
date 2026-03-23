package main

import (
	"strconv"
	"strings"
)

// Config holds the resolved CLI configuration
type Config struct {
	Port              int
	TLSEnabled        bool
	CertFile          string
	KeyFile           string
	RecentMax         int
	ListRecentPublic  bool
	Command           string // "run", "install", "uninstall"
	ExplicitPort      bool   // true if --port was explicitly provided
	TLSDExplicitlyDisabled bool // true if --tls-disable was explicitly provided
}

// Default configuration values
const (
	DefaultPort    = 8080
	MinPort        = 1
	MaxPort        = 65535
	PortHTTPS      = 443
	PortAltHTTPS   = 8443
	DefaultRecent  = 12
	FirstTestPort  = 8080
	LastTestPort   = 8090 // Range for auto-port selection in dev mode
)

// ParseArgs parses command-line arguments and returns fully resolved Config
func ParseArgs(args []string, availablePorts []int) Config {
	cfg := Config{
		Port:       DefaultPort,
		RecentMax:  DefaultRecent,
	}

	// Handle commands first (install, uninstall)
	if len(args) > 0 {
		switch args[0] {
		case "install", "uninstall":
			cfg.Command = args[0]
			args = args[1:] // parse remaining args after command
		}
	}

	// Parse flags
	for i, arg := range args {
		switch arg {
		case "--port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil && p >= MinPort && p <= MaxPort {
					cfg.Port = p
					cfg.ExplicitPort = true
				}
			}
		case "--recent":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					cfg.RecentMax = n
				}
			}
		case "--tls":
			cfg.TLSEnabled = true
		case "--tls-disable":
			cfg.TLSDExplicitlyDisabled = true
		case "--cert":
			if i+1 < len(args) {
				cfg.CertFile = args[i+1]
				cfg.TLSEnabled = true
			}
		case "--key":
			if i+1 < len(args) {
				cfg.KeyFile = args[i+1]
				cfg.TLSEnabled = true
			}
		case "--list-recent-public":
			cfg.ListRecentPublic = true
		}
	}

	// Apply auto-TLS logic for standard HTTPS ports
	if !cfg.TLSDExplicitlyDisabled && (cfg.Port == PortHTTPS || cfg.Port == PortAltHTTPS) {
		cfg.TLSEnabled = true
	}

	// Select port: explicit port, or first available
	if !cfg.ExplicitPort && len(availablePorts) > 0 {
		cfg.Port = availablePorts[0]
	}

	return cfg
}

// InstallConfig returns the configuration resolved for installation
// This applies install-specific defaults (port 443)
func (c *Config) InstallConfig() Config {
	installCfg := *c

	// Install defaults to port 443 if no explicit port was given
	if !c.ExplicitPort {
		installCfg.Port = PortHTTPS
	}

	// Auto-enable TLS for standard HTTPS ports in install mode
	if (installCfg.Port == PortHTTPS || installCfg.Port == PortAltHTTPS) && !c.TLSDExplicitlyDisabled {
		installCfg.TLSEnabled = true
	}

	return installCfg
}

// String returns a human-readable description
func (c *Config) String() string {
	var parts []string
	parts = append(parts, "port:"+strconv.Itoa(c.Port))
	if c.TLSEnabled {
		parts = append(parts, "TLS")
	}
	if c.CertFile != "" {
		parts = append(parts, "cert:"+c.CertFile)
	}
	if c.KeyFile != "" {
		parts = append(parts, "key:"+c.KeyFile)
	}
	if c.Command != "" {
		parts = append(parts, "cmd:"+c.Command)
	}
	return strings.Join(parts, ",")
}
