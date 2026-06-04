package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	dashboardBinName = "kiromon-dashboard"
	portFileRel      = ".kiromon/dashboard.port"
	spawnReadyMaxWait = 5 * time.Second
	spawnPollInterval = 150 * time.Millisecond
	dashboardKillGrace = 3 * time.Second
)

var (
	dashboardMu  sync.Mutex
	dashboardCmd *exec.Cmd

	// 🌟 Double-spawn guard. Set while a goroutine is in the middle of
	// spawn-and-wait so a rapid second click is ignored. Without this, two
	// concurrent invocations could both miss the port file and both spawn a
	// child — harmless on a fixed port (second EADDRINUSE-fails) but on a
	// random port both would bind successfully and one becomes orphaned.
	launching atomic.Bool
)

// openDashboard implements the menubar "Open Dashboard" click:
//   1. If the port file exists and the server responds, just open the browser.
//   2. Otherwise spawn kiromon-dashboard (--no-browser), poll for readiness,
//      then open the browser.
//
// Concurrent invocations (rapid double-click) are coalesced via the
// `launching` atomic flag — only the first goroutine spawns; subsequent ones
// return immediately.
func openDashboard() {
	if !launching.CompareAndSwap(false, true) {
		slog.Debug("Open Dashboard already in progress; ignoring click")
		return
	}
	defer launching.Store(false)

	if port, ok := readDashboardPort(); ok && pingDashboard(port) {
		openURLInBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port))
		slog.Info("Dashboard already running; opened browser", "port", port)
		return
	}

	binPath, err := findDashboardBinary()
	if err != nil {
		slog.Error("Cannot locate dashboard binary", "error", err)
		return
	}

	cmd := exec.Command(binPath, "--no-browser")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Detach from controlling terminal (the menubar app doesn't have one anyway).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		slog.Error("Failed to spawn dashboard", "binary", binPath, "error", err)
		return
	}
	slog.Info("Dashboard spawned", "binary", binPath, "pid", cmd.Process.Pid)
	trackDashboard(cmd)

	deadline := time.Now().Add(spawnReadyMaxWait)
	for time.Now().Before(deadline) {
		if port, ok := readDashboardPort(); ok && pingDashboard(port) {
			openURLInBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port))
			return
		}
		time.Sleep(spawnPollInterval)
	}
	slog.Warn("Dashboard did not become ready within timeout; not opening browser")
}

func readDashboardPort() (int, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join(home, portFileRel))
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func pingDashboard(port int) bool {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// findDashboardBinary locates kiromon-dashboard adjacent to the running executable
// (the typical .app bundle layout) and falls back to $PATH for dev/standalone usage.
func findDashboardBinary() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(exe), dashboardBinName)
		if info, err := os.Stat(sibling); err == nil && !info.IsDir() {
			return sibling, nil
		}
	}
	if p, err := exec.LookPath(dashboardBinName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found next to %q or in $PATH", dashboardBinName, exe)
}

func openURLInBrowser(url string) {
	if err := exec.Command("open", url).Start(); err != nil {
		slog.Warn("Failed to open browser", "url", url, "error", err)
	}
}

func trackDashboard(cmd *exec.Cmd) {
	dashboardMu.Lock()
	dashboardCmd = cmd
	dashboardMu.Unlock()

	// Reap on exit so we don't carry a stale handle.
	go func() {
		_ = cmd.Wait()
		dashboardMu.Lock()
		if dashboardCmd == cmd {
			dashboardCmd = nil
		}
		dashboardMu.Unlock()
	}()
}

// killDashboard sends SIGTERM to a tracked dashboard subprocess and waits up to
// dashboardKillGrace before SIGKILL. Safe to call when nothing is tracked.
func killDashboard() {
	dashboardMu.Lock()
	cmd := dashboardCmd
	dashboardCmd = nil
	dashboardMu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		slog.Warn("Failed to SIGTERM dashboard", "error", err)
	}

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(dashboardKillGrace):
		_ = cmd.Process.Kill()
	}
}
