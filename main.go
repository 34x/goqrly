package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"fmt"
	"html/template"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

var store Store
var recentCodes []string
var recentMax = 12
var listRecentPublic = false

// Legacy global flags (for backward compatibility during transition)
var forceOverwrite = false
var removeBinary = false

var indexTmpl = template.Must(template.ParseFS(staticFS, "static/index.html"))
var viewTmpl = template.Must(template.ParseFS(staticFS, "static/view.html"))
var lockTmpl = template.Must(template.ParseFS(staticFS, "static/lock.html"))
var setupTOTPTmpl = template.Must(template.ParseFS(staticFS, "static/setup-totp.html"))

const certDir = "/etc/goqrly"
const certFileName = "goqrly.crt"
const keyFileName = "goqrly.key"
const autoGenMarker = "auto-generated"
const serviceFile = "/etc/systemd/system/goqrly.service"
const binaryPath = "/usr/local/bin/goqrly"

const firstTestPort = 8080
const lastTestPort = 8090

// availablePorts returns a list of ports that are currently free
func availablePorts() []int {
	ports := []int{}
	for port := firstTestPort; port <= lastTestPort; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			ports = append(ports, port)
		}
	}
	return ports
}

// initStore creates the appropriate store based on config
func initStore(cfg Config) (Store, error) {
	if cfg.DataDir != "" {
		fs, err := NewFileStore(cfg.DataDir)
		if err != nil {
			return nil, err
		}
		fmt.Printf("Using persistent storage: %s\n", cfg.DataDir)
		return fs, nil
	}
	return NewMemoryStore(), nil
}

func main() {
	args := os.Args[1:]

	// Check for help flags before parsing
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			printHelp()
			return
		}
	}

	// For install/uninstall commands, parse without port selection
	if len(args) > 0 && (args[0] == "install" || args[0] == "uninstall") {
		cfg := ParseArgs(args, nil)
		switch cfg.Command {
		case "install":
			installService(cfg)
			return
		case "uninstall":
			uninstallService()
			return
		}
	}

	cfg := ParseArgs(args, availablePorts())

	// Initialize store (memory or file-based)
	var err error
	store, err = initStore(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing store: %v\n", err)
		os.Exit(1)
	}

	// Update globals from config
	recentMax = cfg.RecentMax
	listRecentPublic = cfg.ListRecentPublic

	port := cfg.Port

	// Setup firewall (only if running as root)
	setupFirewall(port)

	addr := fmt.Sprintf(":%d", port)
	mux := setupMux()

	// Wrap with middleware (CSRF + rate limiting)
	handler := handlerWithMiddleware(mux)

	// Determine TLS mode
	useTLS := cfg.TLSEnabled || (cfg.CertFile != "" && cfg.KeyFile != "")

	if useTLS && cfg.CertFile != "" && cfg.KeyFile != "" {
		// Use provided certificates
		fmt.Printf("goqrly running on https://0.0.0.0:%d\n", port)
		if err := http.ListenAndServeTLS(addr, cfg.CertFile, cfg.KeyFile, handler); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else if useTLS {
		// Generate in-memory self-signed cert
		certPEM, keyPEM, err := generateSelfSignedCert()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating certificate: %v\n", err)
			os.Exit(1)
		}
		cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading certificate: %v\n", err)
			os.Exit(1)
		}
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		}
		fmt.Printf("goqrly running on https://0.0.0.0:%d (self-signed cert)\n", port)
		server := &http.Server{Addr: addr, TLSConfig: tlsConfig, Handler: handler}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("goqrly running on http://0.0.0.0:%d\n", port)
		if err := http.ListenAndServe(addr, handler); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func printHelp() {
	fmt.Println(`goqrly - QR Code Generator

Usage:
  goqrly [options]          Run server
  sudo goqrly install       Install as service (TLS on port 443)
  sudo goqrly uninstall    Uninstall service

Options:
  -h, --help               Show this help message
  --port <n>               Port to listen on (default: 8080)
  --recent <n>             Number of recent codes on index page (default: 12)
  --list-recent-public     Show recent public entries on index page
  --data-dir <path>        Data directory for persistent storage (default: in-memory)
  --tls                    Enable TLS with self-signed certificate
  --tls-disable            Disable auto-TLS for ports 443/8443
  --cert <path>            Path to TLS certificate
  --key <path>             Path to TLS private key
  --remove-binary          Remove binary when uninstalling

Examples:
  goqrly                              # Run on port 8080 (in-memory)
  goqrly --data-dir ./data            # Run with persistent storage
  goqrly --tls                        # Run on port 8080 with self-signed TLS
  goqrly --port 443                   # Run on port 443 with auto-TLS
  goqrly --list-recent-public         # Show recent public entries on homepage
  sudo goqrly install                 # Install with TLS on port 443 (default)
  sudo goqrly install --data-dir /var/lib/goqrly  # Install with persistent storage
  sudo goqrly install --port 8080     # Install without TLS on port 8080
  sudo goqrly uninstall               # Remove service, keep certs and binary
  sudo goqrly uninstall --remove-binary  # Remove everything`)
}

func setupFirewall(port int) {
	if os.Getuid() != 0 {
		return
	}
	cmd := exec.Command("ufw", "allow", fmt.Sprintf("%d/tcp", port))
	cmd.Run()
}

func installService(cfg Config) {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: install must be run as root")
		os.Exit(1)
	}

	// Get install-specific configuration
	installCfg := cfg.InstallConfig()

	// Determine TLS mode
	useTLS := installCfg.TLSEnabled || (installCfg.CertFile != "" && installCfg.KeyFile != "")
	autoGenCerts := useTLS && installCfg.CertFile == "" && installCfg.KeyFile == ""
	useCertFile := installCfg.CertFile
	useKeyFile := installCfg.KeyFile

	// Generate certs if needed
	if autoGenCerts {
		certPath := certDir + "/" + certFileName
		keyPath := certDir + "/" + keyFileName

		// Check if certs already exist
		if _, err := os.Stat(certPath); err == nil {
			if !forceOverwrite {
				fmt.Fprintf(os.Stderr, "Error: certificates already exist at %s\n", certPath)
				fmt.Fprintln(os.Stderr, "Use --force to overwrite, or --cert and --key to specify different certificates")
				os.Exit(1)
			}
		}

		// Generate new certificates
		certPEM, keyPEM, err := generateSelfSignedCert()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating certificates: %v\n", err)
			os.Exit(1)
		}

		// Create directory
		if err := os.MkdirAll(certDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating certificate directory: %v\n", err)
			os.Exit(1)
		}

		// Write certificates
		if err := os.WriteFile(certPath, []byte(certPEM), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing certificate: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(keyPath, []byte(keyPEM), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing key: %v\n", err)
			os.Exit(1)
		}

		// Write marker file to indicate auto-generated certs
		if err := os.WriteFile(certDir+"/"+autoGenMarker, []byte(""), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing marker file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Generated TLS certificates at %s\n", certDir)
		useCertFile = certPath
		useKeyFile = keyPath
	}

	// Copy binary to path
	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	dstPath := "/usr/local/bin/goqrly"
	if binPath != dstPath {
		src, err := os.Open(binPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		dst, err := os.Create(dstPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		io.Copy(dst, src)
		src.Close()
		dst.Close()
		os.Chmod(dstPath, 0755)
	}

	// Create data directory if specified
	if installCfg.DataDir != "" {
		if err := os.MkdirAll(installCfg.DataDir, 0750); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Data directory: %s\n", installCfg.DataDir)
	}

	// Detect public IP (1 second timeout)
	ip := detectPublicIP()

	// Build ExecStart line
	portStr := strconv.Itoa(installCfg.Port)
	execStart := "/usr/local/bin/goqrly --port " + portStr
	if useTLS {
		execStart += " --cert " + useCertFile + " --key " + useKeyFile
	}
	if installCfg.DataDir != "" {
		execStart += " --data-dir " + installCfg.DataDir
	}

	// Write systemd service
	service := `[Unit]
Description=goqrly QR Code Generator
After=network.target

[Service]
Type=simple
ExecStart=` + execStart + `
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

	if err := os.WriteFile("/etc/systemd/system/goqrly.service", []byte(service), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing service file: %v\n", err)
		os.Exit(1)
	}

	// Reload systemd, enable and start
	cmd := exec.Command("systemctl", "daemon-reload")
	cmd.Run()
	cmd = exec.Command("systemctl", "enable", "--now", "goqrly")
	cmd.Run()

	// Try to open firewall
	openFirewall(installCfg.Port)

	// Print result
	proto := "http"
	if useTLS {
		proto = "https"
	}
	fmt.Println("goqrly installed successfully!")
	fmt.Println()
	fmt.Println("Access locally: " + proto + "://localhost:" + portStr)
	if ip != "" {
		fmt.Println("Access remote:  " + proto + "://" + ip + ":" + portStr)
	}
	fmt.Println()
	fmt.Println("Manage service: sudo systemctl status goqrly")
}

func generateSelfSignedCert() (certPEM, keyPEM string, err error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: newSerialNumber(),
		Subject: pkix.Name{
			CommonName:   "goqrly",
			Organization: []string{"goqrly"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("0.0.0.0"), net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode to PEM
	certBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}
	certPEM = string(pem.EncodeToMemory(certBlock))

	// Encode private key to PEM
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyBlock := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}
	keyPEM = string(pem.EncodeToMemory(keyBlock))

	return certPEM, keyPEM, nil
}

func newSerialNumber() *big.Int {
	serial, _ := rand.Prime(rand.Reader, 16)
	return serial
}

func detectPublicIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan string, 1)

	go func() {
		// Try external service
		cmd := exec.Command("curl", "-s", "ifconfig.me")
		out, _ := cmd.Output()
		ip := strings.TrimSpace(string(out))
		if isPublicIP(ip) {
			done <- ip
			return
		}

		// Fallback: hostname -I
		cmd = exec.Command("hostname", "-I")
		out, _ = cmd.Output()
		for _, ip := range strings.Fields(string(out)) {
			if isPublicIP(ip) {
				done <- ip
				return
			}
		}

		done <- ""
	}()

	select {
	case ip := <-done:
		return ip
	case <-ctx.Done():
		return ""
	}
}

func isPublicIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	first, _ := strconv.Atoi(parts[0])
	second, _ := strconv.Atoi(parts[1])

	// Private ranges: 10.x.x.x, 172.16-31.x.x, 192.168.x.x, 127.x.x.x, ::1, fe80::
	if first == 10 {
		return false
	}
	if first == 172 && (second >= 16 && second <= 31) {
		return false
	}
	if first == 192 && second == 168 {
		return false
	}
	if first == 127 {
		return false
	}
	if strings.HasPrefix(ip, "fe80:") || ip == "::1" {
		return false
	}
	return true
}

func openFirewall(port int) {
	portStr := strconv.Itoa(port)

	// Try ufw
	cmd := exec.Command("ufw", "status")
	if err := cmd.Run(); err == nil {
		cmd = exec.Command("ufw", "allow", portStr+"/tcp")
		if err := cmd.Run(); err == nil {
			fmt.Println("Firewall: port " + portStr + " opened")
			return
		}
	}

	// Try firewalld
	cmd = exec.Command("firewall-cmd", "--state")
	if err := cmd.Run(); err == nil {
		cmd = exec.Command("firewall-cmd", "--permanent", "--add-port="+portStr+"/tcp")
		cmd.Run()
		cmd = exec.Command("firewall-cmd", "--reload")
		if err := cmd.Run(); err == nil {
			fmt.Println("Firewall: port " + portStr + " opened (firewalld)")
			return
		}
	}

	// No firewall tool available
	fmt.Println()
	fmt.Println("⚠ Firewall: please open port manually:")
	fmt.Println("   sudo ufw allow " + portStr + "/tcp")
	fmt.Println("   # or")
	fmt.Println("   sudo firewall-cmd --permanent --add-port=" + portStr + "/tcp && sudo firewall-cmd --reload")
}

func uninstallService() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: uninstall must be run as root")
		os.Exit(1)
	}

	fmt.Println("Uninstalling goqrly...")

	// Stop and disable service
	cmd := exec.Command("systemctl", "stop", "goqrly")
	cmd.Run()
	cmd = exec.Command("systemctl", "disable", "goqrly")
	cmd.Run()

	// Remove service file
	if _, err := os.Stat(serviceFile); err == nil {
		os.Remove(serviceFile)
		fmt.Println("Removed service file")
	}

	// Reload systemd
	cmd = exec.Command("systemctl", "daemon-reload")
	cmd.Run()

	// Remove auto-generated certificates
	markerPath := certDir + "/" + autoGenMarker
	if _, err := os.Stat(markerPath); err == nil {
		os.RemoveAll(certDir)
		fmt.Println("Removed auto-generated certificates from " + certDir)
	}

	// Optionally remove binary
	if removeBinary {
		if _, err := os.Stat(binaryPath); err == nil {
			os.Remove(binaryPath)
			fmt.Println("Removed binary from " + binaryPath)
		}
	} else {
		fmt.Println()
		fmt.Println("To remove the binary, run:")
		fmt.Println("   sudo rm " + binaryPath)
	}

	fmt.Println()
	fmt.Println("goqrly uninstalled successfully!")
}
