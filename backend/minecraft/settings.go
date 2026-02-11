package minecraft

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type AppSettings struct {
	UserAgent          string `json:"userAgent"`
	DefaultMinRAM      string `json:"defaultMinRam,omitempty"`
	DefaultMaxRAM      string `json:"defaultMaxRam,omitempty"`
	DefaultFlags       string `json:"defaultFlags,omitempty"`
	StatusPollInterval int    `json:"statusPollInterval,omitempty"`
}

var (
	userAgentMu       sync.RWMutex
	userAgentOverride string
)

func defaultUserAgent() string {
	return "MC-AdPanel/1.0 (+https://github.com/pbarrera813/MC-AdPanel)"
}

func setUserAgentOverride(ua string) {
	userAgentMu.Lock()
	userAgentOverride = strings.TrimSpace(ua)
	userAgentMu.Unlock()
}

func getUserAgentOverride() string {
	userAgentMu.RLock()
	defer userAgentMu.RUnlock()
	return userAgentOverride
}

func effectiveUserAgent() string {
	if ua := getUserAgentOverride(); ua != "" {
		return ua
	}
	if ua := strings.TrimSpace(os.Getenv("ADPANEL_USER_AGENT")); ua != "" {
		return ua
	}
	return defaultUserAgent()
}

func applySettingsDefaults(cfg *AppSettings) {
	if cfg.DefaultMinRAM == "" {
		cfg.DefaultMinRAM = "0.5"
	}
	if cfg.DefaultMaxRAM == "" {
		cfg.DefaultMaxRAM = "1"
	}
	if cfg.DefaultFlags == "" {
		cfg.DefaultFlags = "none"
	}
	if cfg.StatusPollInterval <= 0 {
		cfg.StatusPollInterval = 3
	}
}

func (m *Manager) loadSettings() error {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	if m.settingsFile == "" {
		return fmt.Errorf("settings file path is not configured")
	}

	data, err := os.ReadFile(m.settingsFile)
	if err != nil {
		if os.IsNotExist(err) {
			m.settings = AppSettings{UserAgent: effectiveUserAgent()}
			applySettingsDefaults(&m.settings)
			setUserAgentOverride(m.settings.UserAgent)
			return nil
		}
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	var cfg AppSettings
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse settings file: %w", err)
	}
	cfg.UserAgent = strings.TrimSpace(cfg.UserAgent)
	if cfg.UserAgent == "" {
		cfg.UserAgent = effectiveUserAgent()
	}
	applySettingsDefaults(&cfg)
	m.settings = cfg
	setUserAgentOverride(cfg.UserAgent)
	return nil
}

func (m *Manager) persistSettings() error {
	if m.settingsFile == "" {
		return fmt.Errorf("settings file path is not configured")
	}
	data, err := json.MarshalIndent(m.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}
	tmpFile := m.settingsFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp settings: %w", err)
	}
	if err := os.Rename(tmpFile, m.settingsFile); err != nil {
		return fmt.Errorf("failed to rename settings file: %w", err)
	}
	return nil
}

func (m *Manager) GetSettings() AppSettings {
	m.settingsMu.RLock()
	defer m.settingsMu.RUnlock()
	s := m.settings
	if s.UserAgent == "" {
		s.UserAgent = effectiveUserAgent()
	}
	applySettingsDefaults(&s)
	return s
}

func (m *Manager) UpdateAppSettings(userAgent, defaultMinRAM, defaultMaxRAM, defaultFlags string, statusPollInterval int) (AppSettings, error) {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		ua = defaultUserAgent()
	}

	if statusPollInterval <= 0 {
		statusPollInterval = 3
	}
	if statusPollInterval > 30 {
		statusPollInterval = 30
	}

	m.settings = AppSettings{
		UserAgent:          ua,
		DefaultMinRAM:      defaultMinRAM,
		DefaultMaxRAM:      defaultMaxRAM,
		DefaultFlags:       defaultFlags,
		StatusPollInterval: statusPollInterval,
	}
	applySettingsDefaults(&m.settings)
	setUserAgentOverride(ua)

	if err := os.MkdirAll(filepath.Dir(m.settingsFile), 0755); err != nil {
		return AppSettings{}, fmt.Errorf("failed to create settings directory: %w", err)
	}
	if err := m.persistSettings(); err != nil {
		return AppSettings{}, err
	}
	return m.settings, nil
}
