package minecraft

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestJar(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDetectJarVersion(t *testing.T) {
	dir := t.TempDir()
	velocityManifest := "Manifest-Version: 1.0\r\nImplementation-Version: 4.2.0\r\nImplementation-Title: Velocity\r\n"
	tests := []struct {
		name    string
		kind    string
		entries map[string]string
		want    string
	}{
		{"vanilla 26.x", "Vanilla", map[string]string{"version.json": `{"id":"26.1.2","name":"26.1.2","world_version":4189}`}, "26.1.2"},
		{"vanilla 1.x", "Vanilla", map[string]string{"version.json": `{"id":"1.21.4","name":"1.21.4"}`}, "1.21.4"},
		{"paperclip list", "Paper", map[string]string{"META-INF/versions.list": "abc123\tpaper-1.21.4\t1.21.4/paper-1.21.4.jar\n"}, "1.21.4"},
		{"snapshot id gives no signal", "Vanilla", map[string]string{"version.json": `{"id":"24w44a","name":"24w44a"}`}, ""},
		{"no known file", "Fabric", map[string]string{"fabric-server-launch.properties": "x=1"}, ""},
		{"velocity release from manifest", "Velocity", map[string]string{"META-INF/MANIFEST.MF": velocityManifest}, "4.2.0"},
		{"velocity snapshot from manifest", "velocity", map[string]string{"META-INF/MANIFEST.MF": "Implementation-Version: 3.4.0-SNAPSHOT (git-abc-b535)\n"}, "3.4.0-SNAPSHOT"},
		{"manifest of a non-proxy jar is not trusted", "Fabric", map[string]string{"META-INF/MANIFEST.MF": "Implementation-Version: 1.0.1\n"}, ""},
	}
	for _, tt := range tests {
		path := filepath.Join(dir, tt.name+".jar")
		writeTestJar(t, path, tt.entries)
		if got := detectJarVersion(path, tt.kind); got != tt.want {
			t.Fatalf("%s: detectJarVersion = %q; want %q", tt.name, got, tt.want)
		}
	}
	if got := detectJarVersion(filepath.Join(dir, "missing.jar"), "Paper"); got != "" {
		t.Fatalf("missing jar = %q; want empty", got)
	}
	notZip := filepath.Join(dir, "broken.jar")
	if err := os.WriteFile(notZip, []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := detectJarVersion(notZip, "Paper"); got != "" {
		t.Fatalf("non-zip jar = %q; want empty", got)
	}
}

func newJarTestManager(t *testing.T, cfg *ServerConfig, status string) *Manager {
	t.Helper()
	base := t.TempDir()
	cfg.Dir = filepath.Join(base, "srv")
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	return &Manager{
		dataFile: filepath.Join(base, "servers.json"),
		configs:  map[string]*ServerConfig{cfg.ID: cfg},
		running:  map[string]*runningServer{cfg.ID: {status: status}},
	}
}

func vanillaJar(version string) map[string]string {
	return map[string]string{"version.json": `{"id":"` + version + `"}`}
}

func TestSyncServerJar(t *testing.T) {
	t.Run("uploaded jar with another name becomes the server jar and sets the version", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Vanilla", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "server.jar"), vanillaJar("1.20.1"))
		writeTestJar(t, filepath.Join(cfg.Dir, "my-new-server.jar"), vanillaJar("26.1.2"))

		if got := m.SyncServerJar("a", "my-new-server.jar"); got != "my-new-server.jar" {
			t.Fatalf("returned %q; want the new jar name", got)
		}
		if cfg.JarFile != "my-new-server.jar" || cfg.Version != "26.1.2" {
			t.Fatalf("cfg = jar %q version %q; want my-new-server.jar 26.1.2", cfg.JarFile, cfg.Version)
		}
		if _, err := os.Stat(m.dataFile); err != nil {
			t.Fatalf("change was not persisted: %v", err)
		}
	})

	t.Run("configured jar present and nothing touched is left alone", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Vanilla", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "server.jar"), vanillaJar("1.20.1"))
		writeTestJar(t, filepath.Join(cfg.Dir, "other.jar"), vanillaJar("26.1.2"))

		if got := m.SyncServerJar("a", ""); got != "" || cfg.JarFile != "server.jar" || cfg.Version != "1.20.1" {
			t.Fatalf("got %q, cfg = %q %q; want unchanged", got, cfg.JarFile, cfg.Version)
		}
	})

	t.Run("missing configured jar falls back to the newest root jar", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Paper", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		old := filepath.Join(cfg.Dir, "old.jar")
		fresh := filepath.Join(cfg.Dir, "paper-1.21.4.jar")
		writeTestJar(t, old, nil)
		writeTestJar(t, fresh, map[string]string{"META-INF/versions.list": "h\tpaper-1.21.4\t1.21.4/paper-1.21.4.jar"})
		past := time.Now().Add(-time.Hour)
		if err := os.Chtimes(old, past, past); err != nil {
			t.Fatal(err)
		}

		if got := m.SyncServerJar("a", ""); got != "paper-1.21.4.jar" {
			t.Fatalf("returned %q; want paper-1.21.4.jar", got)
		}
		if cfg.JarFile != "paper-1.21.4.jar" || cfg.Version != "1.21.4" {
			t.Fatalf("cfg = %q %q", cfg.JarFile, cfg.Version)
		}
	})

	t.Run("missing configured jar and no other jar keeps the name", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Paper", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		if got := m.SyncServerJar("a", ""); got != "" || cfg.JarFile != "server.jar" {
			t.Fatalf("got %q jar %q; want unchanged", got, cfg.JarFile)
		}
	})

	t.Run("start-command servers are skipped", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Forge", Version: "1.20.1", JarFile: "server.jar", StartCommand: []string{"bash", "run.sh"}}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "other.jar"), vanillaJar("26.1.2"))
		if got := m.SyncServerJar("a", "other.jar"); got != "" || cfg.JarFile != "server.jar" || cfg.Version != "1.20.1" {
			t.Fatalf("got %q cfg = %q %q; want unchanged", got, cfg.JarFile, cfg.Version)
		}
	})

	t.Run("touched values that are not a plain root jar are ignored", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Vanilla", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "server.jar"), vanillaJar("1.20.1"))
		if err := os.MkdirAll(filepath.Join(cfg.Dir, "plugins"), 0755); err != nil {
			t.Fatal(err)
		}
		writeTestJar(t, filepath.Join(cfg.Dir, "plugins", "x.jar"), vanillaJar("26.1.2"))
		if err := os.WriteFile(filepath.Join(cfg.Dir, "notes.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		for _, touched := range []string{"plugins/x.jar", "../x.jar", "notes.txt", "missing.jar", "."} {
			if got := m.SyncServerJar("a", touched); got != "" || cfg.JarFile != "server.jar" {
				t.Fatalf("touched %q: got %q jar %q; want ignored", touched, got, cfg.JarFile)
			}
		}
	})

	t.Run("a running server switches the jar but keeps the version until the next start", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Vanilla", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Running")
		writeTestJar(t, filepath.Join(cfg.Dir, "next.jar"), vanillaJar("26.1.2"))
		if got := m.SyncServerJar("a", "next.jar"); got != "next.jar" || cfg.Version != "1.20.1" {
			t.Fatalf("got %q version %q; want next.jar and 1.20.1", got, cfg.Version)
		}
	})

	t.Run("a replaced Velocity jar updates the card version from its manifest", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Velocity", Version: "4.3.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "server.jar"), map[string]string{"META-INF/MANIFEST.MF": "Implementation-Version: 4.2.0\n"})
		if got := m.SyncServerJar("a", ""); got != "" || cfg.Version != "4.2.0" {
			t.Fatalf("got %q version %q; want unchanged jar and 4.2.0", got, cfg.Version)
		}
	})

	t.Run("an installing server is never touched", func(t *testing.T) {
		cfg := &ServerConfig{ID: "a", Type: "Vanilla", Version: "1.20.1", JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Installing")
		writeTestJar(t, filepath.Join(cfg.Dir, "next.jar"), vanillaJar("26.1.2"))
		if got := m.SyncServerJar("a", "next.jar"); got != "" || cfg.JarFile != "server.jar" {
			t.Fatalf("got %q jar %q; want unchanged", got, cfg.JarFile)
		}
	})
}
