package minecraft

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// backupDir returns the centralized backup directory for a server
func (m *Manager) backupDir(cfg *ServerConfig) string {
	return filepath.Join(m.backupsRoot, m.backupFolderKey(cfg))
}

// ListBackups returns all backup archives for a server
func (m *Manager) ListBackups(id string) ([]BackupInfo, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	backupsDir := m.backupDir(cfg)
	if err := m.validateManagedBackupDir(backupsDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}

	backups := make([]BackupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			Name: entry.Name(),
			Date: info.ModTime().UTC().Format(time.RFC3339),
			Size: formatFileSize(info.Size()),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Date > backups[j].Date
	})

	return backups, nil
}

// CreateBackup creates a tar.gz archive of the server directory
func (m *Manager) CreateBackup(id string) (*BackupInfo, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if err := m.validateManagedServerDir(cfg.Dir); err != nil {
		return nil, m.configPathErrorLocked(id, err.Error())
	}

	backupsDir := m.backupDir(cfg)
	if err := m.validateManagedBackupDir(backupsDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		return nil, err
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	fileName := fmt.Sprintf("backup_%s.tar.gz", timestamp)
	backupPath := filepath.Join(backupsDir, fileName)

	cmd := exec.Command("tar", "-czf", backupPath, "--exclude=backups", "-C", cfg.Dir, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("backup failed: %s: %w", string(output), err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, err
	}

	return &BackupInfo{
		Name: fileName,
		Date: time.Now().UTC().Format(time.RFC3339),
		Size: formatFileSize(info.Size()),
	}, nil
}

// DeleteBackup removes a backup archive
func (m *Manager) DeleteBackup(id, fileName string) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	backupsDir := m.backupDir(cfg)
	if err := m.validateManagedBackupDir(backupsDir); err != nil {
		return err
	}

	backupPath, err := SafePath(backupsDir, fileName)
	if err != nil {
		return err
	}

	return os.Remove(backupPath)
}

// GetBackupPath returns the full filesystem path for downloading a backup
func (m *Manager) GetBackupPath(id, fileName string) (string, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	m.mu.RUnlock()
	if err != nil {
		return "", err
	}
	backupsDir := m.backupDir(cfg)
	if err := m.validateManagedBackupDir(backupsDir); err != nil {
		return "", err
	}

	backupPath, err := SafePath(backupsDir, fileName)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup %s not found", fileName)
	}

	return backupPath, nil
}

// RestoreBackup extracts a backup archive into the server directory (server must be stopped)
func (m *Manager) RestoreBackup(id, fileName string) error {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	rs, rsOk := m.running[id]
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	if !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()
	if status != "Stopped" && status != "Crashed" && status != "Error" {
		return fmt.Errorf("server must be stopped before restoring a backup")
	}
	if err := m.validateManagedServerDir(cfg.Dir); err != nil {
		return m.configPathErrorLocked(id, err.Error())
	}
	backupsDir := m.backupDir(cfg)
	if err := m.validateManagedBackupDir(backupsDir); err != nil {
		return err
	}

	backupPath, err := SafePath(backupsDir, fileName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup %s not found", fileName)
	}

	// Clear server directory contents
	serverRoot, err := SafePath(cfg.Dir, ".")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(serverRoot)
	if err != nil {
		return fmt.Errorf("failed to read server directory: %w", err)
	}
	for _, entry := range entries {
		target := filepath.Join(serverRoot, entry.Name())
		if err := ensurePathWithinBase(serverRoot, filepath.Clean(target)); err != nil {
			return fmt.Errorf("failed to clear server directory entry %q: path safety check failed", entry.Name())
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("failed to clear server directory entry %q: %w", entry.Name(), err)
		}
	}

	// Extract backup
	cmd := exec.Command("tar", "-xzf", backupPath, "-C", cfg.Dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore failed: %s: %w", string(output), err)
	}

	log.Printf("Restored backup %s for server %s", fileName, cfg.Name)
	return nil
}

// SetBackupSchedule sets or clears the automatic backup schedule for a server
func (m *Manager) SetBackupSchedule(id, schedule string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.serverConfigForOperationLocked(id)
	if err != nil {
		return err
	}

	valid := map[string]bool{"": true, "daily": true, "weekly": true, "monthly": true, "sixmonths": true, "yearly": true}
	if !valid[schedule] {
		return fmt.Errorf("invalid schedule: %s", schedule)
	}

	cfg.BackupSchedule = schedule
	if schedule != "" && cfg.LastScheduledBackup == "" {
		cfg.LastScheduledBackup = time.Now().UTC().Format(time.RFC3339)
	}
	if schedule == "" {
		cfg.LastScheduledBackup = ""
	}

	return m.persist()
}

// GetBackupSchedule returns the backup schedule info for a server
func (m *Manager) GetBackupSchedule(id string) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, err := m.serverConfigForOperationLocked(id)
	if err != nil {
		return nil, err
	}

	result := map[string]string{
		"schedule": cfg.BackupSchedule,
	}
	if cfg.BackupSchedule != "" && cfg.LastScheduledBackup != "" {
		lastTime, err := time.Parse(time.RFC3339, cfg.LastScheduledBackup)
		if err == nil {
			next := nextScheduledBackupTime(lastTime, cfg.BackupSchedule)
			result["nextBackup"] = next.UTC().Format(time.RFC3339)
		}
	}
	return result, nil
}

// nextScheduledBackupTime calculates the next backup time from the last backup and schedule
func nextScheduledBackupTime(last time.Time, schedule string) time.Time {
	switch schedule {
	case "daily":
		return last.Add(24 * time.Hour)
	case "weekly":
		return last.Add(7 * 24 * time.Hour)
	case "monthly":
		return last.AddDate(0, 1, 0)
	case "sixmonths":
		return last.AddDate(0, 6, 0)
	case "yearly":
		return last.AddDate(1, 0, 0)
	default:
		return time.Time{}
	}
}

// runBackupScheduler periodically checks if any scheduled backups are due
func (m *Manager) runBackupScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopScheduler:
			return
		case <-ticker.C:
			m.checkScheduledBackups()
		}
	}
}

// checkScheduledBackups runs pending scheduled backups
func (m *Manager) checkScheduledBackups() {
	m.mu.RLock()
	type pending struct {
		id   string
		name string
	}
	var due []pending
	now := time.Now().UTC()

	for id, cfg := range m.configs {
		if cfg.BackupSchedule == "" || cfg.LastScheduledBackup == "" {
			continue
		}
		lastTime, err := time.Parse(time.RFC3339, cfg.LastScheduledBackup)
		if err != nil {
			continue
		}
		next := nextScheduledBackupTime(lastTime, cfg.BackupSchedule)
		if now.After(next) {
			due = append(due, pending{id: id, name: cfg.Name})
		}
	}
	m.mu.RUnlock()

	for _, p := range due {
		log.Printf("Running scheduled backup for server: %s", p.name)
		backup, err := m.CreateBackup(p.id)
		if err != nil {
			log.Printf("Scheduled backup failed for %s: %v", p.name, err)
			continue
		}
		log.Printf("Scheduled backup completed for %s: %s", p.name, backup.Name)

		// Update last scheduled backup time
		m.mu.Lock()
		if cfg, ok := m.configs[p.id]; ok {
			cfg.LastScheduledBackup = time.Now().UTC().Format(time.RFC3339)
			m.persist()
		}
		m.mu.Unlock()
	}
}
