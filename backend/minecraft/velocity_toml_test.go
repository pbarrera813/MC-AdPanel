package minecraft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const defaultVelocityToml = `# Config version. Do not change this
config-version = "2.9"

# What port should the proxy be bound to? By default, we'll bind to all addresses on port 25565.
bind = "0.0.0.0:25565"

motd = "<#09add3>A Velocity Server"

# What should we display for the maximum number of players?
show-max-players = 500

online-mode = true

[servers]
lobby = "127.0.0.1:30066"
try = [
    "lobby"
]

[query]
enabled = false
port = 25565
`

func TestUpdateVelocityToml(t *testing.T) {
	path := filepath.Join(t.TempDir(), "velocity.toml")
	if err := os.WriteFile(path, []byte(defaultVelocityToml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := updateVelocityToml(path, 20, 25570); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{`bind = "0.0.0.0:25570"`, "show-max-players = 20", "online-mode = true", "port = 25565\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated file lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `bind = "0.0.0.0:25565"`) {
		t.Fatalf("old bind line still present:\n%s", got)
	}
	if err := updateVelocityToml(filepath.Join(t.TempDir(), "missing.toml"), 20, 25570); err == nil {
		t.Fatal("missing file should report an error")
	}
}

func TestUpdateVelocityTomlKeepsIPv6Host(t *testing.T) {
	path := filepath.Join(t.TempDir(), "velocity.toml")
	if err := os.WriteFile(path, []byte("bind = \"[::1]:25565\"\nshow-max-players = 5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := updateVelocityToml(path, 5, 25600); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `bind = "[::1]:25600"`) {
		t.Fatalf("ipv6 host lost:\n%s", data)
	}
}

func TestSyncVelocityConfig(t *testing.T) {
	t.Run("an edit in velocity.toml shows up in the panel config", func(t *testing.T) {
		cfg := &ServerConfig{ID: "v", Type: "Velocity", Port: 25570, MaxPlayers: 20, JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		toml := strings.Replace(defaultVelocityToml, `0.0.0.0:25565`, `0.0.0.0:25580`, 1)
		toml = strings.Replace(toml, "show-max-players = 500", "show-max-players = 40", 1)
		if err := os.WriteFile(filepath.Join(cfg.Dir, "velocity.toml"), []byte(toml), 0644); err != nil {
			t.Fatal(err)
		}
		m.SyncVelocityConfig("v")
		if cfg.Port != 25580 || cfg.MaxPlayers != 40 {
			t.Fatalf("cfg = port %d max %d; want 25580 and 40", cfg.Port, cfg.MaxPlayers)
		}
		if _, err := os.Stat(m.dataFile); err != nil {
			t.Fatalf("change was not persisted: %v", err)
		}
	})

	t.Run("a missing velocity.toml is created from the jar default with the panel port", func(t *testing.T) {
		cfg := &ServerConfig{ID: "v", Type: "Velocity", Port: 25570, MaxPlayers: 20, JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "server.jar"), map[string]string{"default-velocity.toml": defaultVelocityToml})
		m.SyncVelocityConfig("v")
		data, err := os.ReadFile(filepath.Join(cfg.Dir, "velocity.toml"))
		if err != nil {
			t.Fatalf("velocity.toml was not created: %v", err)
		}
		if !strings.Contains(string(data), `bind = "0.0.0.0:25570"`) || !strings.Contains(string(data), "show-max-players = 20") {
			t.Fatalf("seeded file does not carry the panel values:\n%s", data)
		}
		if cfg.Port != 25570 {
			t.Fatalf("panel port changed to %d", cfg.Port)
		}
	})

	t.Run("a jar without a bundled default creates nothing", func(t *testing.T) {
		cfg := &ServerConfig{ID: "v", Type: "Velocity", Port: 25570, MaxPlayers: 20, JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		writeTestJar(t, filepath.Join(cfg.Dir, "server.jar"), map[string]string{"x": "y"})
		m.SyncVelocityConfig("v")
		if _, err := os.Stat(filepath.Join(cfg.Dir, "velocity.toml")); err == nil {
			t.Fatal("velocity.toml should not exist")
		}
	})

	t.Run("other server types and installing proxies are ignored", func(t *testing.T) {
		cfg := &ServerConfig{ID: "p", Type: "Paper", Port: 25565, MaxPlayers: 20, JarFile: "server.jar"}
		m := newJarTestManager(t, cfg, "Stopped")
		if err := os.WriteFile(filepath.Join(cfg.Dir, "velocity.toml"), []byte(defaultVelocityToml), 0644); err != nil {
			t.Fatal(err)
		}
		m.SyncVelocityConfig("p")
		if cfg.Port != 25565 || cfg.MaxPlayers != 20 {
			t.Fatalf("paper config changed: %d %d", cfg.Port, cfg.MaxPlayers)
		}

		vcfg := &ServerConfig{ID: "v", Type: "Velocity", Port: 25570, MaxPlayers: 20, JarFile: "server.jar"}
		vm := newJarTestManager(t, vcfg, "Installing")
		if err := os.WriteFile(filepath.Join(vcfg.Dir, "velocity.toml"), []byte(defaultVelocityToml), 0644); err != nil {
			t.Fatal(err)
		}
		vm.SyncVelocityConfig("v")
		if vcfg.Port != 25570 {
			t.Fatalf("installing proxy changed port to %d", vcfg.Port)
		}
	})
}
