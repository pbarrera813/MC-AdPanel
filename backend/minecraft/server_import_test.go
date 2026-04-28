package minecraft

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubJarProvider struct {
	versions []VersionInfo
}

func (s *stubJarProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	return append([]VersionInfo(nil), s.versions...), nil
}

func (s *stubJarProvider) DownloadJar(ctx context.Context, version string, destDir string, javaExec string, progressFn func(string)) error {
	return nil
}

func withStubProvider(t *testing.T, serverType string, versions []VersionInfo, fn func()) {
	t.Helper()
	key := strings.ToLower(strings.TrimSpace(serverType))
	previous := providers[key]
	providers[key] = &stubJarProvider{versions: versions}
	t.Cleanup(func() {
		if previous == nil {
			delete(providers, key)
		} else {
			providers[key] = previous
		}
	})
	fn()
}

func buildZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	return buf.Bytes()
}

func buildTarGzArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		body := []byte(content)
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header %s: %v", name, err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("failed to write tar entry %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestServerImportAnalyzeAndCommitZip(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"MyImported/server.properties":             "server-port=25565\nmax-players=12\nmotd=Hello Orexa\nwhite-list=true\nonline-mode=false\n",
		"MyImported/world/level.dat":               "world-data",
		"MyImported/plugins/TestPlugin.jar":        "jar-bytes",
		"MyImported/server.jar":                    "server-jar",
		"MyImported/logs/latest.log":               "[bootstrap] Loading Paper 1.21.10-130-ver/1.21.10@8043efd for Minecraft 1.21.10\n",
		"MyImported/eula.txt":                      "eula=true\n",
		"MyImported/bukkit.yml":                    "settings: {}\n",
		"MyImported/plugins/Another.jar":           "jar",
		"MyImported/plugins/Disabled.jar.disabled": "jar",
	})

	result, err := mgr.AnalyzeServerImportArchive("myimport.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if result.AnalysisID == "" {
		t.Fatalf("expected analysis id")
	}
	if !result.TypeDetected {
		t.Fatalf("expected type to be detected")
	}
	if result.ServerType == "" {
		t.Fatalf("expected detected server type")
	}
	if len(result.Worlds) == 0 || result.Worlds[0] != "world" {
		t.Fatalf("expected world detection, got %+v", result.Worlds)
	}
	if result.Properties.MaxPlayers == nil || *result.Properties.MaxPlayers != 12 {
		t.Fatalf("expected max players from server.properties, got %+v", result.Properties.MaxPlayers)
	}
	if result.Properties.OnlineMode == nil || *result.Properties.OnlineMode {
		t.Fatalf("expected online-mode false, got %+v", result.Properties.OnlineMode)
	}
	if result.ResolvedPort != 25565 {
		t.Fatalf("expected resolved port 25565, got %d", result.ResolvedPort)
	}

	info, err := mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{})
	if err != nil {
		t.Fatalf("CommitServerImport failed: %v", err)
	}
	if info.Status != "Stopped" {
		t.Fatalf("expected imported server status Stopped, got %s", info.Status)
	}
	serverDir, err := mgr.GetServerDir(info.ID)
	if err != nil {
		t.Fatalf("GetServerDir failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serverDir, "server.properties")); err != nil {
		t.Fatalf("expected imported server.properties to exist: %v", err)
	}
}

func TestServerImportRejectsUnsupportedArchive(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	_, err = mgr.AnalyzeServerImportArchive("invalid.rar", strings.NewReader("data"))
	if err == nil {
		t.Fatalf("expected unsupported archive format error")
	}
}

func TestServerImportRejectsNonServerPayload(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"NotServer/server.properties": "server-port=25565\nmax-players=20\n",
		"NotServer/world/level.dat":   "world-data",
		"NotServer/readme.txt":        "not a server package",
	})
	_, err = mgr.AnalyzeServerImportArchive("notserver.zip", bytes.NewReader(zipBytes))
	if err == nil {
		t.Fatalf("expected non-server payload rejection")
	}
	want := "The uploaded file couldn't be confirmed to be a server, are you sure you uploaded the right file?"
	if err.Error() != want {
		t.Fatalf("expected exact message %q, got %q", want, err.Error())
	}
}

func TestServerImportAcceptsCustomRootJarName(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"CustomJar/paperclip-1.21.10.jar": "jar-bytes",
		"CustomJar/spigot.yml":            "settings: {}\n",
		"CustomJar/logs/latest.log":       "[bootstrap] Loading Paper 1.21.10-130-ver/1.21.10@8043efd for Minecraft 1.21.10\n",
	})
	result, err := mgr.AnalyzeServerImportArchive("customjar.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	info, err := mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{})
	if err != nil {
		t.Fatalf("CommitServerImport failed: %v", err)
	}
	mgr.mu.Lock()
	cfg := mgr.configs[info.ID]
	mgr.mu.Unlock()
	if cfg == nil {
		t.Fatalf("expected imported config to exist")
	}
	if cfg.JarFile != "paperclip-1.21.10.jar" {
		t.Fatalf("expected custom jar to be selected, got %q", cfg.JarFile)
	}
}

func TestServerImportUnknownTypeRequiresOverride(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"Mystery/launcher-custom.jar": "jar",
		"Mystery/readme.txt":          "hello",
	})
	result, err := mgr.AnalyzeServerImportArchive("mystery.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if result.TypeDetected {
		t.Fatalf("expected unknown type for mystery archive")
	}
	if _, err := mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{}); err == nil {
		t.Fatalf("expected commit to require type override")
	}
	withStubProvider(t, "paper", []VersionInfo{{Version: "1.21.10", Latest: true}}, func() {
		version := "1.21.10"
		info, err := mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{
			TypeOverride: "Paper",
			Version:      &version,
		})
		if err != nil {
			t.Fatalf("CommitServerImport with override failed: %v", err)
		}
		if info.Type != "Paper" {
			t.Fatalf("expected override type Paper, got %s", info.Type)
		}
	})
}

func TestServerImportAnalyzeTarGz(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	archive := buildTarGzArchive(t, map[string]string{
		"Proxy/velocity.toml":      "bind = \"0.0.0.0:25570\"\nshow-max-players = 77\n",
		"Proxy/velocity-3.3.0.jar": "jar",
		"Proxy/forwarding.secret":  "abc",
	})
	result, err := mgr.AnalyzeServerImportArchive("proxy.tar.gz", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if !result.TypeDetected || result.ServerType != "Velocity" {
		t.Fatalf("expected Velocity detection, got type=%q detected=%v", result.ServerType, result.TypeDetected)
	}
	if result.Properties.MaxPlayers == nil || *result.Properties.MaxPlayers != 77 {
		t.Fatalf("expected max players 77 from velocity.toml, got %+v", result.Properties.MaxPlayers)
	}
	if result.ResolvedPort != 25570 {
		t.Fatalf("expected resolved port 25570, got %d", result.ResolvedPort)
	}
}

func TestServerImportPrefersPaperWhenPaperConfigExists(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"PaperLike/plugins/TestPlugin.jar":          "jar",
		"PaperLike/spigot.yml":                      "settings: {}\n",
		"PaperLike/config/paper-global.yml":         "verbose: false\n",
		"PaperLike/config/paper-world-defaults.yml": "world-settings: {}\n",
		"PaperLike/paper-1.21.10.jar":               "jar",
	})

	result, err := mgr.AnalyzeServerImportArchive("paperlike.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if !result.TypeDetected || result.ServerType != "Paper" {
		t.Fatalf("expected Paper detection, got type=%q detected=%v", result.ServerType, result.TypeDetected)
	}
}

func TestServerImportAcceptsForgeRunScriptEvidence(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"ForgePack/run.sh":                             "#!/bin/bash\njava @user_jvm_args.txt @libraries/net/minecraftforge/forge/args.txt nogui\n",
		"ForgePack/user_jvm_args.txt":                  "-Xmx2G\n",
		"ForgePack/libraries/net/minecraftforge/.keep": "x",
	})
	result, err := mgr.AnalyzeServerImportArchive("forgepack.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if result.TypeDetected {
		t.Fatalf("expected unknown type without explicit forge metadata, got %q", result.ServerType)
	}

	withStubProvider(t, "forge", []VersionInfo{{Version: "1.21.10", Latest: true}}, func() {
		version := "1.21.10"
		info, commitErr := mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{
			TypeOverride: "Forge",
			Version:      &version,
		})
		if commitErr != nil {
			t.Fatalf("CommitServerImport failed: %v", commitErr)
		}
		mgr.mu.Lock()
		cfg := mgr.configs[info.ID]
		mgr.mu.Unlock()
		if cfg == nil {
			t.Fatalf("expected imported config")
		}
		if len(cfg.StartCommand) != 3 || cfg.StartCommand[0] != "bash" || cfg.StartCommand[1] != "run.sh" || cfg.StartCommand[2] != "nogui" {
			t.Fatalf("expected run.sh start command for forge import, got %#v", cfg.StartCommand)
		}
	})
}

func TestServerImportDetectsVersionFromLatestLog(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"FromLogs/server.jar":      "jar",
		"FromLogs/logs/latest.log": "[bootstrap] Loading Paper 1.21.10-130-ver/1.21.10@8043efd for Minecraft 1.21.10\n",
	})

	result, err := mgr.AnalyzeServerImportArchive("fromlogs.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if result.Version != "1.21.10" {
		t.Fatalf("expected version 1.21.10 from logs, got %q", result.Version)
	}
}

func TestServerImportCommitAppliesOverrides(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"Editable/server.properties": "server-port=25565\nmax-players=20\nmotd=Original\nwhite-list=false\nonline-mode=true\n",
		"Editable/server.jar":        "jar",
	})

	result, err := mgr.AnalyzeServerImportArchive("editable.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}

	name := "Edited Server"
	port := 25590
	version := "1.21.10"
	maxPlayers := 35
	motd := "Edited MOTD"
	whiteList := true
	onlineMode := false
	var info *ServerInfo
	withStubProvider(t, "paper", []VersionInfo{{Version: "1.21.10", Latest: true}}, func() {
		info, err = mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{
			Name:         &name,
			Port:         &port,
			TypeOverride: "Paper",
			Version:      &version,
			MaxPlayers:   &maxPlayers,
			Motd:         &motd,
			WhiteList:    &whiteList,
			OnlineMode:   &onlineMode,
		})
	})
	if err != nil {
		t.Fatalf("CommitServerImport failed: %v", err)
	}

	if info.Name != name {
		t.Fatalf("expected overridden name %q, got %q", name, info.Name)
	}
	if info.Port != port {
		t.Fatalf("expected overridden port %d, got %d", port, info.Port)
	}
	if info.Type != "Paper" {
		t.Fatalf("expected overridden type Paper, got %q", info.Type)
	}
	if info.Version != version {
		t.Fatalf("expected overridden version %q, got %q", version, info.Version)
	}

	serverDir, err := mgr.GetServerDir(info.ID)
	if err != nil {
		t.Fatalf("GetServerDir failed: %v", err)
	}
	propsPath := filepath.Join(serverDir, "server.properties")
	propsBytes, err := os.ReadFile(propsPath)
	if err != nil {
		t.Fatalf("failed to read server.properties: %v", err)
	}
	props := string(propsBytes)
	if !strings.Contains(props, "server-port=25590") {
		t.Fatalf("expected overridden port in server.properties, got:\n%s", props)
	}
	if !strings.Contains(props, "max-players=35") {
		t.Fatalf("expected overridden max-players in server.properties, got:\n%s", props)
	}
	if !strings.Contains(props, "motd=Edited MOTD") {
		t.Fatalf("expected overridden motd in server.properties, got:\n%s", props)
	}
	if !strings.Contains(props, "white-list=true") {
		t.Fatalf("expected overridden white-list in server.properties, got:\n%s", props)
	}
	if !strings.Contains(props, "online-mode=false") {
		t.Fatalf("expected overridden online-mode in server.properties, got:\n%s", props)
	}
}

func TestServerImportDetectedVersionCannotBeOverridden(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"Locked/server.jar":      "jar",
		"Locked/logs/latest.log": "[bootstrap] Loading Paper 1.21.10-130-ver/1.21.10@8043efd for Minecraft 1.21.10\n",
		"Locked/spigot.yml":      "settings: {}\n",
	})
	result, err := mgr.AnalyzeServerImportArchive("locked.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	if result.Version == "" {
		t.Fatalf("expected detected version")
	}

	otherVersion := "1.20.6"
	_, err = mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{
		TypeOverride: "Paper",
		Version:      &otherVersion,
	})
	if err == nil {
		t.Fatalf("expected invalid version error")
	}
	var versionErr *ImportInvalidVersionError
	if !errors.As(err, &versionErr) {
		t.Fatalf("expected ImportInvalidVersionError, got %T (%v)", err, err)
	}
}

func TestServerImportManualVersionMustExistInProviderList(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	zipBytes := buildZipArchive(t, map[string]string{
		"Manual/server.properties": "server-port=25565\nmax-players=20\n",
		"Manual/server.jar":        "jar",
	})
	result, err := mgr.AnalyzeServerImportArchive("manual.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}

	withStubProvider(t, "paper", []VersionInfo{{Version: "1.21.10", Latest: true}}, func() {
		invalid := "9.99.9"
		_, err = mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{
			TypeOverride: "Paper",
			Version:      &invalid,
		})
	})
	if err == nil {
		t.Fatalf("expected invalid version error")
	}
	var versionErr *ImportInvalidVersionError
	if !errors.As(err, &versionErr) {
		t.Fatalf("expected ImportInvalidVersionError, got %T (%v)", err, err)
	}
}

func TestServerImportPortConflictReturnsSuggestion(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	_, err = mgr.CreateServer("BusyPort", "Vanilla", "1.21.10", 25565, "512M", "1024M", 20, "none", false)
	if err != nil {
		t.Fatalf("CreateServer failed: %v", err)
	}

	zipBytes := buildZipArchive(t, map[string]string{
		"Conflict/server.properties": "server-port=25565\nmax-players=20\n",
		"Conflict/server.jar":        "jar",
		"Conflict/logs/latest.log":   "[bootstrap] Loading Paper 1.21.10-130-ver/1.21.10@8043efd for Minecraft 1.21.10\n",
	})

	result, err := mgr.AnalyzeServerImportArchive("conflict.zip", bytes.NewReader(zipBytes))
	if err != nil {
		t.Fatalf("AnalyzeServerImportArchive failed: %v", err)
	}
	port := 25565
	_, err = mgr.CommitServerImport(result.AnalysisID, ServerImportCommitOptions{
		TypeOverride: "Vanilla",
		Port:         &port,
	})
	if err == nil {
		t.Fatalf("expected port conflict error")
	}
	var conflictErr *ImportPortConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ImportPortConflictError, got %T (%v)", err, err)
	}
	if conflictErr.SuggestedPort == 0 || conflictErr.SuggestedPort == port {
		t.Fatalf("expected suggested port different from requested, got %d", conflictErr.SuggestedPort)
	}
}
