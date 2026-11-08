package minecraft

import (
	"archive/zip"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func readJarEntry(zr *zip.Reader, name string, limit int64) string {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		defer rc.Close()
		data, _ := io.ReadAll(io.LimitReader(rc, limit))
		return string(data)
	}
	return ""
}

// Velocity states its version in the manifest; Minecraft jars carry version.json or META-INF/versions.list.
// "" means no signal (Fabric, Forge, snapshots, unreadable), so callers keep the current version.
func detectJarVersion(jarPath, serverType string) string {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return ""
	}
	defer zr.Close()

	if strings.EqualFold(serverType, "velocity") {
		for _, line := range strings.Split(readJarEntry(&zr.Reader, "META-INF/MANIFEST.MF", 64<<10), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Implementation-Version:"); ok {
				return extractVersionFromText(v)
			}
		}
		return ""
	}

	var meta struct{ ID, Name string }
	if json.Unmarshal([]byte(readJarEntry(&zr.Reader, "version.json", 64<<10)), &meta) == nil {
		for _, v := range []string{meta.ID, meta.Name} {
			if found := extractMCVersionFromText(v); found != "" {
				return found
			}
		}
	}
	return extractMCVersionFromText(readJarEntry(&zr.Reader, "META-INF/versions.list", 64<<10))
}

func newestRootJar(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	var bestTime int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); best == "" || mod > bestTime {
			best, bestTime = entry.Name(), mod
		}
	}
	return best
}

func (m *Manager) SyncServerJar(id, touched string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, rs := m.configs[id], m.running[id]
	if cfg == nil || rs == nil || len(cfg.StartCommand) > 0 {
		return ""
	}
	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()
	if status == "Installing" {
		return ""
	}

	isRootJar := func(name string) bool {
		if name == "" || name != filepath.Base(name) || !strings.HasSuffix(strings.ToLower(name), ".jar") {
			return false
		}
		info, err := os.Stat(filepath.Join(cfg.Dir, name))
		return err == nil && !info.IsDir()
	}
	jar := cfg.JarFile
	switch {
	case isRootJar(touched):
		jar = touched
	case !isRootJar(jar):
		if newest := newestRootJar(cfg.Dir); newest != "" {
			jar = newest
		}
	}

	changedJar := ""
	dirty := false
	if jar != cfg.JarFile {
		log.Printf("[%s] Using %s as the server jar (was %s)", cfg.Name, jar, cfg.JarFile)
		cfg.JarFile, changedJar, dirty = jar, jar, true
	}
	if status != "Running" && status != "Booting" {
		if v := detectJarVersion(filepath.Join(cfg.Dir, cfg.JarFile), cfg.Type); v != "" && v != cfg.Version {
			log.Printf("[%s] Server jar is version %s (card said %s)", cfg.Name, v, cfg.Version)
			cfg.Version, dirty = v, true
		}
	}
	if dirty {
		if err := m.persist(); err != nil {
			log.Printf("[%s] Failed to save server jar change: %v", cfg.Name, err)
		}
	}
	return changedJar
}
