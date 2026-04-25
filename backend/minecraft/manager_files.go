package minecraft

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ListFiles returns directory contents at the given subpath
func (m *Manager) ListFiles(id, subPath string) ([]FileEntry, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	dirPath, err := SafePath(cfg.Dir, subPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	files := make([]FileEntry, 0)
	for _, entry := range entries {
		if shouldHideServerRootArtifact(subPath, entry.Name()) {
			continue
		}
		info, err := resolveDirEntryInfo(dirPath, entry)
		if err != nil {
			entryType := "file"
			if entry.IsDir() {
				entryType = "folder"
			}
			files = append(files, FileEntry{
				Name:    entry.Name(),
				Type:    entryType,
				Size:    "-",
				ModTime: time.Time{}.UTC().Format(time.RFC3339),
			})
			continue
		}
		entryType := "file"
		if info.IsDir() || entry.IsDir() {
			entryType = "folder"
		}
		files = append(files, FileEntry{
			Name:    entry.Name(),
			Type:    entryType,
			Size:    formatFileSize(info.Size()),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Type != files[j].Type {
			return files[i].Type == "folder"
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

// ReadFileContent reads a file's content within a server directory
func (m *Manager) ReadFileContent(id, subPath string) ([]byte, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	filePath, err := SafePath(cfg.Dir, subPath)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(filePath)
}

// WriteFileContent writes content to a file within a server directory
func (m *Manager) WriteFileContent(id, subPath string, content []byte) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	filePath, err := SafePath(cfg.Dir, subPath)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, content, 0644)
}

// DeletePath removes a file or directory within a server directory
func (m *Manager) DeletePath(id, subPath string) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	targetPath, err := SafePath(cfg.Dir, subPath)
	if err != nil {
		return err
	}

	serverRoot, err := SafePath(cfg.Dir, ".")
	if err != nil {
		return err
	}
	if samePath(serverRoot, targetPath) {
		return fmt.Errorf("cannot delete server root directory")
	}

	return os.RemoveAll(targetPath)
}

// CreateDirectory creates a directory within a server directory
func (m *Manager) CreateDirectory(id, subPath string) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	dirPath, err := SafePath(cfg.Dir, subPath)
	if err != nil {
		return err
	}

	return os.MkdirAll(dirPath, 0755)
}

// RenamePath renames a file or directory within a server directory
func (m *Manager) RenamePath(id, oldSubPath, newName string) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	oldPath, err := SafePath(cfg.Dir, oldSubPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(oldPath); err != nil {
		return fmt.Errorf("path does not exist: %s", oldSubPath)
	}

	// Build new path in the same parent directory
	newPath := filepath.Join(filepath.Dir(oldPath), newName)

	// Validate the new path is still within the server directory
	if _, err := SafePath(cfg.Dir, filepath.Join(filepath.Dir(oldSubPath), newName)); err != nil {
		return err
	}

	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("a file or folder named %q already exists", newName)
	}

	return os.Rename(oldPath, newPath)
}
