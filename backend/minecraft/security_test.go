package minecraft

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePathBlocksSiblingPrefixTraversal(t *testing.T) {
	base := t.TempDir()
	serverDir := filepath.Join(base, "server1")
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		t.Fatalf("failed to create server dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "server10"), 0755); err != nil {
		t.Fatalf("failed to create sibling dir: %v", err)
	}

	if _, err := SafePath(serverDir, filepath.Join("..", "server10", "world")); err == nil {
		t.Fatalf("expected traversal attempt to be rejected")
	}
}

func TestSafePathReturnsAbsoluteContainedPath(t *testing.T) {
	serverDir := t.TempDir()
	rootPath, err := SafePath(serverDir, ".")
	if err != nil {
		t.Fatalf("SafePath root failed: %v", err)
	}

	got, err := SafePath(serverDir, filepath.Join("logs", "latest.log"))
	if err != nil {
		t.Fatalf("SafePath failed: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
	rel, err := filepath.Rel(rootPath, got)
	if err != nil {
		t.Fatalf("filepath.Rel failed: %v", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("path escaped base dir: %q", got)
	}
}

func TestSafePathBlocksSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	serverDir := filepath.Join(base, "server")
	outsideDir := filepath.Join(base, "outside")
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		t.Fatalf("failed to create server dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	linkPath := filepath.Join(serverDir, "escape")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("skipping symlink test on this environment: %v", err)
	}

	if _, err := SafePath(serverDir, filepath.Join("escape", "secret.txt")); err == nil {
		t.Fatalf("expected symlink escape to be rejected")
	}
}

func TestValidateLoginUpgradesLegacyHash(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	legacyHash, err := hashPasswordLegacySHA256(defaultLoginPassword())
	if err != nil {
		t.Fatalf("failed to create legacy hash: %v", err)
	}

	mgr.settingsMu.Lock()
	mgr.settings.LoginUser = defaultLoginUser()
	mgr.settings.LoginPasswordHash = legacyHash
	if err := mgr.persistSettings(); err != nil {
		mgr.settingsMu.Unlock()
		t.Fatalf("persistSettings failed: %v", err)
	}
	mgr.settingsMu.Unlock()

	if !mgr.ValidateLogin(defaultLoginUser(), defaultLoginPassword()) {
		t.Fatalf("expected legacy credentials to validate")
	}

	mgr.settingsMu.RLock()
	upgraded := mgr.settings.LoginPasswordHash
	mgr.settingsMu.RUnlock()
	if !strings.HasPrefix(upgraded, passwordHashSchemeArgon2id+"$") {
		t.Fatalf("expected hash to upgrade to argon2id, got %q", upgraded)
	}
}

func TestUpdateAppSettingsEnforcesPasswordLength(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	_, err = mgr.UpdateAppSettings("", "0.5", "1", "none", 3, 30, 15, 20, "adminuser", "short")
	if err == nil {
		t.Fatalf("expected short password to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "at least 10") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSanitizeNameRejectsDotSegments(t *testing.T) {
	if got := sanitizeName("."); got != "server" {
		t.Fatalf("expected '.' to sanitize to server, got %q", got)
	}
	if got := sanitizeName(".."); got != "server" {
		t.Fatalf("expected '..' to sanitize to server, got %q", got)
	}
	if got := sanitizeName("..."); got != "server" {
		t.Fatalf("expected '...' to sanitize to server, got %q", got)
	}
}

func TestQuarantineUnsafeServerConfigPath(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "data"), 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}
	cfgs := []*ServerConfig{
		{
			ID:         "unsafe001",
			Name:       "Unsafe Server",
			Type:       "paper",
			Version:    "1.21.1",
			Port:       25565,
			JarFile:    "server.jar",
			MaxRAM:     "1G",
			MinRAM:     "512M",
			MaxPlayers: 20,
			Dir:        filepath.Join(base, "..", "outside-server"),
		},
	}
	raw, err := json.Marshal(cfgs)
	if err != nil {
		t.Fatalf("failed to marshal configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "data", "servers.json"), raw, 0644); err != nil {
		t.Fatalf("failed to write servers.json: %v", err)
	}

	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	if err := mgr.EnsureServerOperational("unsafe001"); err == nil {
		t.Fatalf("expected unsafe server path to be quarantined")
	} else if !IsConfigPathSafetyError(err) {
		t.Fatalf("expected ConfigPathSafetyError, got %v", err)
	}
}

func TestBackupMigrationUsesStableServerID(t *testing.T) {
	base := t.TempDir()
	serversDir := filepath.Join(base, "Servers")
	serverRoot := filepath.Join(serversDir, "LegacySrv")
	if err := os.MkdirAll(serverRoot, 0755); err != nil {
		t.Fatalf("failed to create server root: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(base, "data"), 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}
	cfgs := []*ServerConfig{
		{
			ID:         "legacy01",
			Name:       "LegacySrv",
			Type:       "paper",
			Version:    "1.21.1",
			Port:       25565,
			JarFile:    "server.jar",
			MaxRAM:     "1G",
			MinRAM:     "512M",
			MaxPlayers: 20,
			Dir:        serverRoot,
		},
	}
	raw, err := json.Marshal(cfgs)
	if err != nil {
		t.Fatalf("failed to marshal configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "data", "servers.json"), raw, 0644); err != nil {
		t.Fatalf("failed to write servers.json: %v", err)
	}

	oldBackupDir := filepath.Join(base, "Backups", sanitizeName("LegacySrv"))
	if err := os.MkdirAll(oldBackupDir, 0755); err != nil {
		t.Fatalf("failed to create legacy backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldBackupDir, "backup_old.tar.gz"), []byte("legacy"), 0644); err != nil {
		t.Fatalf("failed to write backup: %v", err)
	}

	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	newBackupPath := filepath.Join(base, "Backups", sanitizeName("legacy01"), "backup_old.tar.gz")
	if _, err := os.Stat(newBackupPath); err != nil {
		t.Fatalf("expected backup to be migrated to stable server-id directory: %v", err)
	}
}
