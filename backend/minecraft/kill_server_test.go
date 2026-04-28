//go:build linux

package minecraft

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func buildTestManagerForKill(t *testing.T, id string, rs *runningServer) *Manager {
	t.Helper()
	base := t.TempDir()
	serversRoot := filepath.Join(base, "Servers")
	if err := os.MkdirAll(serversRoot, 0o755); err != nil {
		t.Fatalf("failed to create servers root: %v", err)
	}
	serverDir := filepath.Join(serversRoot, "srv")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatalf("failed to create server dir: %v", err)
	}
	cfg := &ServerConfig{
		ID:   id,
		Name: "TestServer",
		Dir:  serverDir,
		Type: "Paper",
	}
	return &Manager{
		configs:            map[string]*ServerConfig{id: cfg},
		running:            map[string]*runningServer{id: rs},
		quarantinedServers: map[string]string{},
		serversRoot:        serversRoot,
		serversRootReal:    serversRoot,
	}
}

func TestKillServerRejectsNonRunningState(t *testing.T) {
	const id = "srv1"
	mgr := buildTestManagerForKill(t, id, &runningServer{
		status: "Stopped",
	})
	if err := mgr.KillServer(id); err == nil {
		t.Fatal("expected error when killing a non-running server")
	}
}

func TestKillServerTerminatesProcessAndResetsState(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 120")
	prepareServerProcessCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start test process: %v", err)
	}

	const id = "srv1"
	rs := &runningServer{
		cmd:         cmd,
		status:      "Running",
		pid:         cmd.Process.Pid,
		stopMetrics: make(chan struct{}),
		players:     map[string]*onlinePlayer{"Alice": {Name: "Alice"}},
	}
	mgr := buildTestManagerForKill(t, id, rs)

	if err := mgr.KillServer(id); err != nil {
		t.Fatalf("KillServer returned error: %v", err)
	}

	rs.mu.RLock()
	status := rs.status
	pid := rs.pid
	players := len(rs.players)
	rs.mu.RUnlock()
	if status != "Stopped" {
		t.Fatalf("expected status Stopped, got %s", status)
	}
	if pid != 0 {
		t.Fatalf("expected pid reset to 0, got %d", pid)
	}
	if players != 0 {
		t.Fatalf("expected players map cleared, got %d players", players)
	}

	select {
	case <-rs.stopMetrics:
	default:
		t.Fatal("expected stopMetrics channel to be closed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(cmd.Process.Pid, 0)
		if err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("process still alive after KillServer")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
