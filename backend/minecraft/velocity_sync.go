package minecraft

import (
	"archive/zip"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Velocity would otherwise generate the file with its default port, so seed it from the jar's
// bundled default to make the very first start use the panel's port.
func seedVelocityToml(tomlPath, jarPath string, maxPlayers, port int) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return
	}
	defer zr.Close()
	def := readJarEntry(&zr.Reader, "default-velocity.toml", 256<<10)
	if def == "" || os.WriteFile(tomlPath, []byte(def), 0644) != nil {
		return
	}
	if err := updateVelocityToml(tomlPath, maxPlayers, port); err != nil {
		log.Printf("Could not apply port to the new velocity.toml: %v", err)
	}
}

func (m *Manager) SyncVelocityConfig(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, rs := m.configs[id], m.running[id]
	if cfg == nil || rs == nil || !strings.EqualFold(cfg.Type, "velocity") {
		return
	}
	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()
	if status == "Installing" {
		return
	}

	tomlPath := filepath.Join(cfg.Dir, "velocity.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		seedVelocityToml(tomlPath, filepath.Join(cfg.Dir, cfg.JarFile), cfg.MaxPlayers, cfg.Port)
		return
	}

	port, maxPlayers := parseVelocityToml(tomlPath)
	dirty := false
	if port != nil && *port >= 1 && *port <= 65535 && *port != cfg.Port {
		cfg.Port, dirty = *port, true
	}
	if maxPlayers != nil && *maxPlayers > 0 && *maxPlayers != cfg.MaxPlayers {
		cfg.MaxPlayers, dirty = *maxPlayers, true
	}
	if dirty {
		if err := m.persist(); err != nil {
			log.Printf("[%s] Failed to save velocity.toml values: %v", cfg.Name, err)
		}
	}
}
