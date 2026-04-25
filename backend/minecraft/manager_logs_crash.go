package minecraft

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ListCrashReports scans the crash-reports/ directory.
func (m *Manager) ListCrashReports(id string) ([]CrashReport, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	crashDir := filepath.Join(cfg.Dir, "crash-reports")
	entries, err := os.ReadDir(crashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []CrashReport{}, nil
		}
		return nil, err
	}

	reports := make([]CrashReport, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		cause := extractCrashCause(filepath.Join(crashDir, entry.Name()))

		reports = append(reports, CrashReport{
			Name:  entry.Name(),
			Date:  info.ModTime().UTC().Format(time.RFC3339),
			Size:  formatFileSize(info.Size()),
			Cause: cause,
		})
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Date > reports[j].Date
	})

	return reports, nil
}

// ReadCrashReport returns the content of a crash report file.
func (m *Manager) ReadCrashReport(id, fileName string) ([]byte, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	filePath, err := SafePath(filepath.Join(cfg.Dir, "crash-reports"), fileName)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(filePath)
}

// ListLogFiles returns files under the server's logs/ directory.
func (m *Manager) ListLogFiles(id string) ([]FileEntry, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	logsDir := filepath.Join(cfg.Dir, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileEntry{}, nil
		}
		return nil, err
	}

	files := make([]FileEntry, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !(strings.HasSuffix(strings.ToLower(name), ".log") || strings.HasSuffix(strings.ToLower(name), ".txt") || strings.HasSuffix(strings.ToLower(name), ".gz")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileEntry{
			Name:    name,
			Type:    "file",
			Size:    formatFileSize(info.Size()),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime > files[j].ModTime
	})

	return files, nil
}

// ReadLogFile returns the (possibly decompressed) content of a log file.
func (m *Manager) ReadLogFile(id, fileName string) ([]byte, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	filePath, err := SafePath(filepath.Join(cfg.Dir, "logs"), fileName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(strings.ToLower(fileName), ".gz") {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	return data, nil
}

// CopyCrashReport duplicates a crash report file with a "-copy" suffix.
func (m *Manager) CopyCrashReport(id, fileName string) (string, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return "", err
	}

	crashDir := filepath.Join(cfg.Dir, "crash-reports")
	srcPath, err := SafePath(crashDir, fileName)
	if err != nil {
		return "", err
	}

	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	copyName := base + "-copy" + ext

	dstPath, err := SafePath(crashDir, copyName)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(dstPath, content, 0644); err != nil {
		return "", err
	}

	return copyName, nil
}

// DeleteCrashReport deletes a crash report file.
func (m *Manager) DeleteCrashReport(id, fileName string) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return err
	}

	filePath, err := SafePath(filepath.Join(cfg.Dir, "crash-reports"), fileName)
	if err != nil {
		return err
	}

	return os.Remove(filePath)
}

// extractCrashCause reads the first lines of a crash report to find the cause.
func extractCrashCause(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return "Unknown"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 30 && scanner.Scan(); i++ {
		line := scanner.Text()
		if strings.HasPrefix(line, "Description: ") {
			return strings.TrimPrefix(line, "Description: ")
		}
	}
	return "Unknown"
}
