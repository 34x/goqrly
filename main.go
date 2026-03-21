package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
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

var indexTmpl = template.Must(template.ParseFS(staticFS, "static/index.html"))
var viewTmpl = template.Must(template.ParseFS(staticFS, "static/view.html"))

func main() {
	args := os.Args[1:]

	// Parse global flags
	if len(args) > 0 {
		for i, arg := range args {
			if arg == "--port" && i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					serverPort = p
				}
			}
			if arg == "--recent" && i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					recentMax = n
				}
			}
		}
	}

	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help":
			printHelp()
			return
		case "install":
			if serverPort == 8080 && !hasExplicitPort(args) {
				fmt.Fprintln(os.Stderr, "Error: --port is required for install")
				fmt.Fprintln(os.Stderr, "Usage: sudo goqrly install --port 8080")
				os.Exit(1)
			}
			installService()
			return
		}
	}

	setupFirewall(serverPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/qr/{key}", handleQR)
	mux.HandleFunc("/{key}", handleView)

	addr := fmt.Sprintf(":%d", serverPort)
	fmt.Printf("goqrly running on http://0.0.0.0:%d\n", serverPort)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
  sudo goqrly install --port <n>   Install as service

Options:
  -h, --help     Show this help message
  --port <n>     Port to listen on (default: 8080)
  --recent <n>   Number of recent codes on index page (default: 12)

Examples:
  goqrly                              # Run on port 8080
  goqrly --port 9000                 # Run on port 9000
  sudo goqrly install --port 8080    # Install as service`)
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

	// Write systemd service
	portStr := strconv.Itoa(serverPort)
	service := `[Unit]
Description=goqrly QR Code Generator
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goqrly --port ` + portStr + `
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
	fmt.Println("goqrly installed successfully!")
	fmt.Println()
	fmt.Println("Access locally: http://localhost:" + strconv.Itoa(serverPort))
	if ip != "" {
		fmt.Println("Access remote:  http://" + ip + ":" + strconv.Itoa(serverPort))
	}
	fmt.Println()
	fmt.Println("Manage service: sudo systemctl status goqrly")
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
