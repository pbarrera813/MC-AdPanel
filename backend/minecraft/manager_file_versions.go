package minecraft

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	fileVersionKeep     = 5
	fileVersionMaxBytes = 1 << 20
	fileVersionExt      = ".bak"
)

type FileVersion struct {
	ID      string `json:"id"`
	SavedAt string `json:"savedAt"`
	Size    int64  `json:"size"`
}

func (m *Manager) fileVersionsRoot(serverID string) string {
	return filepath.Join(m.baseDir, "data", "file-versions", serverID)
}

func (m *Manager) fileVersionsDir(serverID, subPath string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(filepath.Clean(subPath))))
	return filepath.Join(m.fileVersionsRoot(serverID), hex.EncodeToString(sum[:])[:16])
}

func writeFileVersion(dir string, content []byte, keep int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// The name is a timestamp; on coarse clocks (Windows) two saves can share
	// one, so step forward until the file is created fresh.
	id := time.Now().UnixNano()
	for {
		f, err := os.OpenFile(filepath.Join(dir, strconv.FormatInt(id, 10)+fileVersionExt), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			_, err = f.Write(content)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			break
		}
		if !os.IsExist(err) {
			return err
		}
		id++
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), fileVersionExt) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := keep; i < len(names); i++ {
		_ = os.Remove(filepath.Join(dir, names[i]))
	}
	return nil
}

func listFileVersions(dir string) []FileVersion {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []FileVersion{}
	}
	versions := make([]FileVersion, 0, len(entries))
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), fileVersionExt)
		nanos, convErr := strconv.ParseInt(id, 10, 64)
		if convErr != nil || id == e.Name() {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
		versions = append(versions, FileVersion{ID: id, SavedAt: time.Unix(0, nanos).UTC().Format(time.RFC3339), Size: info.Size()})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].ID > versions[j].ID })
	return versions
}

func (m *Manager) saveFileVersionBeforeWrite(serverID, subPath, filePath string, newContent []byte) {
	old, err := os.ReadFile(filePath)
	if err != nil || len(old) > fileVersionMaxBytes || string(old) == string(newContent) {
		return
	}
	if err := writeFileVersion(m.fileVersionsDir(serverID, subPath), old, fileVersionKeep); err != nil {
		log.Printf("[%s] could not keep previous version of %s: %v", serverID, subPath, err)
	}
}

func (m *Manager) ListFileVersions(id, subPath string) ([]FileVersion, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if _, err := SafePath(cfg.Dir, subPath); err != nil {
		return nil, err
	}
	return listFileVersions(m.fileVersionsDir(id, subPath)), nil
}

func (m *Manager) ReadFileVersion(id, subPath, versionID string) ([]byte, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if _, err := SafePath(cfg.Dir, subPath); err != nil {
		return nil, err
	}
	if _, convErr := strconv.ParseInt(versionID, 10, 64); convErr != nil {
		return nil, fmt.Errorf("invalid version id")
	}
	return os.ReadFile(filepath.Join(m.fileVersionsDir(id, subPath), versionID+fileVersionExt))
}
