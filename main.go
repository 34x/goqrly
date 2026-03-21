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

var store = NewStore()
var recentCodes []string
var recentMax = 12
var serverPort = 8080
var defaultPort = 8080

// TLS settings
var tlsEnabled = false
var certFile = ""
var keyFile = ""
var forceOverwrite = false
var removeBinary = false

var indexTmpl = template.Must(template.ParseFS(staticFS, "static/index.html"))
var viewTmpl = template.Must(template.ParseFS(staticFS, "static/view.html"))

const certDir = "/etc/goqrly"
const certFileName = "goqrly.crt"
const keyFileName = "goqrly.key"
const autoGenMarker = "auto-generated"
const serviceFile = "/etc/systemd/system/goqrly.service"
const binaryPath = "/usr/local/bin/goqrly"

func main() {
	args := os.Args[1:]

	// Parse global flags
	for i, arg := range args {
		switch arg {
		case "--port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					serverPort = p
				}
			}
		case "--recent":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					recentMax = n
				}
			}
		case "--tls":
			tlsEnabled = true
			if serverPort == 8080 && !hasExplicitPort(args) {
				serverPort = 443
			} else if serverPort == 8443 && !hasExplicitPort(args) {
				serverPort = 443
			}
		case "--cert":
			if i+1 < len(args) {
				certFile = args[i+1]
			}
		case "--key":
			if i+1 < len(args) {
				keyFile = args[i+1]
			}
		case "--force":
			forceOverwrite = true
		case "--remove-binary":
			removeBinary = true
		}
	}

	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help":
			printHelp()
			return
		case "install":
			// Install defaults to port 443 with TLS
			if serverPort == 8080 {
				serverPort = 443
				tlsEnabled = true
			}
			installService()
			return
		case "uninstall":
			uninstallService()
			return
		}
	}

	// Determine if we need TLS
	useTLS := tlsEnabled || (certFile != "" && keyFile != "")

	setupFirewall(serverPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/qr/{key}", handleQR)
	mux.HandleFunc("/{key}", handleView)

	addr := fmt.Sprintf(":%d", serverPort)

	if useTLS && certFile != "" && keyFile != "" {
		fmt.Printf("goqrly running on https://0.0.0.0:%d\n", serverPort)
		if err := http.ListenAndServeTLS(addr, certFile, keyFile, mux); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else if useTLS {
		// Generate in-memory cert
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
		fmt.Printf("goqrly running on https://0.0.0.0:%d (self-signed cert)\n", serverPort)
		server := &http.Server{Addr: addr, TLSConfig: tlsConfig, Handler: mux}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("goqrly running on http://0.0.0.0:%d\n", serverPort)
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func hasExplicitPort(args []string) bool {
	for i, arg := range args {
		if arg == "--port" && i+1 < len(args) {
			return true
		}
	}
	return false
}

func printHelp() {
	fmt.Println(`goqrly - QR Code Generator

Usage:
  goqrly [options]          Run server
  sudo goqrly install       Install as service (TLS on port 443)
  sudo goqrly uninstall    Uninstall service

Options:
  -h, --help          Show this help message
  --port <n>          Port to listen on (default: 8080)
  --recent <n>        Number of recent codes on index page (default: 12)
  --tls               Enable TLS with self-signed certificate
  --cert <path>       Path to TLS certificate
  --key <path>        Path to TLS private key
  --remove-binary     Remove binary when uninstalling

Examples:
  goqrly                              # Run on port 8080
  goqrly --tls                        # Run on port 443 with self-signed cert
  sudo goqrly install                 # Install with TLS on port 443 (default)
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

func installService() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: install must be run as root")
		os.Exit(1)
	}

	// Determine TLS mode
	useTLS := tlsEnabled || (certFile != "" && keyFile != "")
	autoGenCerts := tlsEnabled && certFile == "" && keyFile == ""
	useCertFile := certFile
	useKeyFile := keyFile

	// Auto-enable TLS for ports 443 and 8443
	if !useTLS && (serverPort == 443 || serverPort == 8443) {
		useTLS = true
		autoGenCerts = true
	}

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

	// Detect public IP (1 second timeout)
	ip := detectPublicIP()

	// Build ExecStart line
	portStr := strconv.Itoa(serverPort)
	execStart := "/usr/local/bin/goqrly --port " + portStr
	if useTLS {
		execStart += " --cert " + useCertFile + " --key " + useKeyFile
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
	openFirewall(serverPort)

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
