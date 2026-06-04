// Command kiromon-dashboard serves a localhost HTML dashboard for daily Kiro usage stats.
//
// Default port is 17891. Override via CLI flag (--port N), config file
// (~/.kiromon/dashboard.json with {"port": N}), or use --port 0 for an
// OS-assigned random port. CLI flag takes precedence over the config file.
//
// On startup the actual bound port is written to ~/.kiromon/dashboard.port so
// the kiromon menubar app can discover and reuse the running instance.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

//go:embed all:static
var staticFS embed.FS

const (
	defaultPort   = 17891
	portUnset     = -1
	portFileName  = "dashboard.port"
	configName    = "dashboard.json"
	dataDir       = "~/.kiromon"
	openBrowserDelay = 250 * time.Millisecond
)

type config struct {
	Port int `json:"port"`
}

func main() {
	flagPort := flag.Int("port", portUnset, "TCP port (0 = OS-assigned random; -1 or omitted = use config/default)")
	flagHost := flag.String("host", "127.0.0.1", "host to bind to (keep on loopback for security)")
	flagNoBrowser := flag.Bool("no-browser", false, "do not auto-open the browser on startup")
	flagConfig := flag.String("config", "", "path to config file (default: ~/.kiromon/dashboard.json)")
	flag.Parse()

	initLogger()

	port := resolvePort(*flagPort, *flagConfig)
	addr := fmt.Sprintf("%s:%d", *flagHost, port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to bind", "addr", addr, "error", err)
		fmt.Fprintf(os.Stderr, "kiromon-dashboard: failed to bind to %s: %v\n", addr, err)
		os.Exit(1)
	}

	tcpAddr, _ := listener.Addr().(*net.TCPAddr)
	boundPort := tcpAddr.Port
	url := fmt.Sprintf("http://127.0.0.1:%d/", boundPort)

	// Write port-discovery file for the menubar app.
	portPath := writePortFile(boundPort)
	defer removePortFile(portPath)

	fmt.Println("Kiromon Dashboard listening on", url)
	slog.Info("dashboard up", "url", url, "port", boundPort)

	server := &http.Server{
		Handler:           routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Auto-open browser (unless disabled).
	if !*flagNoBrowser {
		go func() {
			time.Sleep(openBrowserDelay)
			openBrowser(url)
		}()
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(listener)
	}()

	select {
	case sig := <-sigCh:
		slog.Info("signal received, shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Warn("graceful shutdown error", "error", err)
		}
	case err := <-serveErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}
}

// resolvePort returns the port to bind: CLI flag > config file > default.
func resolvePort(flagPort int, configPath string) int {
	if flagPort >= 0 {
		return flagPort
	}
	cfg := readConfig(configPath)
	if cfg.Port >= 0 {
		return cfg.Port
	}
	return defaultPort
}

func readConfig(customPath string) config {
	path := customPath
	if path == "" {
		path = filepath.Join(expandPath(dataDir), configName)
	}
	cfg := config{Port: portUnset}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("config read failed; falling back to default", "path", path, "error", err)
		}
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("config parse failed; falling back to default", "path", path, "error", err)
		return config{Port: portUnset}
	}
	return cfg
}

func writePortFile(port int) string {
	dir := expandPath(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("failed to create data dir", "dir", dir, "error", err)
		return ""
	}
	path := filepath.Join(dir, portFileName)
	contents := fmt.Sprintf("%d\n", port)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		slog.Warn("failed to write port file", "path", path, "error", err)
		return ""
	}
	return path
}

func removePortFile(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("failed to remove port file", "path", path, "error", err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		slog.Info("auto-open not supported on this OS", "os", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("failed to open browser", "error", err)
	}
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[1:])
	}
	return p
}

func initLogger() {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
}
