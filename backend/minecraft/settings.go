package minecraft

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
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
	LoginUser          string `json:"loginUser,omitempty"`
	LoginPasswordHash  string `json:"loginPasswordHash,omitempty"`
}

var (
	userAgentMu       sync.RWMutex
	userAgentOverride string
)

func defaultUserAgent() string {
	return "Orexa-Panel/1.0 (+https://github.com/pbarrera813/Orexa-Panel)"
}

func defaultLoginUser() string {
	return "mcpanel"
}

func defaultLoginPassword() string {
	return "mcpanel"
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	sum := sha256.Sum256(append(salt, []byte(password)...))
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(sum[:])
	return "sha256$" + saltB64 + "$" + hashB64, nil
}

func verifyPassword(storedHash, password string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 3 || parts[0] != "sha256" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	sum := sha256.Sum256(append(salt, []byte(password)...))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
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
	if strings.TrimSpace(cfg.LoginUser) == "" {
		cfg.LoginUser = defaultLoginUser()
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
			defaultHash, hashErr := hashPassword(defaultLoginPassword())
			if hashErr != nil {
				return hashErr
			}
			m.settings = AppSettings{
				UserAgent:         effectiveUserAgent(),
				LoginUser:         defaultLoginUser(),
				LoginPasswordHash: defaultHash,
			}
			applySettingsDefaults(&m.settings)
			setUserAgentOverride(m.settings.UserAgent)
			if err := os.MkdirAll(filepath.Dir(m.settingsFile), 0755); err != nil {
				return fmt.Errorf("failed to create settings directory: %w", err)
			}
			if err := m.persistSettings(); err != nil {
				return err
			}
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
	needsPersist := false
	if strings.TrimSpace(cfg.LoginUser) == "" {
		cfg.LoginUser = defaultLoginUser()
		needsPersist = true
	}
	if strings.TrimSpace(cfg.LoginPasswordHash) == "" {
		defaultHash, hashErr := hashPassword(defaultLoginPassword())
		if hashErr != nil {
			return hashErr
		}
		cfg.LoginPasswordHash = defaultHash
		needsPersist = true
	}
	applySettingsDefaults(&cfg)
	m.settings = cfg
	setUserAgentOverride(cfg.UserAgent)
	if needsPersist {
		if err := m.persistSettings(); err != nil {
			return err
		}
	}
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
	s.LoginPasswordHash = ""
	return s
}

func (m *Manager) UpdateAppSettings(userAgent, defaultMinRAM, defaultMaxRAM, defaultFlags string, statusPollInterval int, loginUser, loginPassword string) (AppSettings, error) {
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
	loginUser = strings.TrimSpace(loginUser)
	if loginUser == "" {
		loginUser = m.settings.LoginUser
	}
	if loginUser == "" {
		loginUser = defaultLoginUser()
	}
	if len(loginUser) < 4 || len(loginUser) > 12 {
		return AppSettings{}, fmt.Errorf("username must be between 4 and 12 characters")
	}

	passwordHash := m.settings.LoginPasswordHash
	if strings.TrimSpace(loginPassword) != "" {
		if len(loginPassword) < 4 {
			return AppSettings{}, fmt.Errorf("password must be at least 4 characters")
		}
		hashed, err := hashPassword(loginPassword)
		if err != nil {
			return AppSettings{}, err
		}
		passwordHash = hashed
	}
	if strings.TrimSpace(passwordHash) == "" {
		hashed, err := hashPassword(defaultLoginPassword())
		if err != nil {
			return AppSettings{}, err
		}
		passwordHash = hashed
	}

	m.settings = AppSettings{
		UserAgent:          ua,
		DefaultMinRAM:      defaultMinRAM,
		DefaultMaxRAM:      defaultMaxRAM,
		DefaultFlags:       defaultFlags,
		StatusPollInterval: statusPollInterval,
		LoginUser:          loginUser,
		LoginPasswordHash:  passwordHash,
	}
	applySettingsDefaults(&m.settings)
	setUserAgentOverride(ua)

	if err := os.MkdirAll(filepath.Dir(m.settingsFile), 0755); err != nil {
		return AppSettings{}, fmt.Errorf("failed to create settings directory: %w", err)
	}
	if err := m.persistSettings(); err != nil {
		return AppSettings{}, err
	}
	result := m.settings
	result.LoginPasswordHash = ""
	return result, nil
}

func (m *Manager) ValidateLogin(username, password string) bool {
	m.settingsMu.RLock()
	defer m.settingsMu.RUnlock()

	if strings.TrimSpace(username) != m.settings.LoginUser {
		return false
	}
	return verifyPassword(m.settings.LoginPasswordHash, password)
}

func (m *Manager) IsUsingDefaultLogin() bool {
	m.settingsMu.RLock()
	defer m.settingsMu.RUnlock()

	return m.settings.LoginUser == defaultLoginUser() && verifyPassword(m.settings.LoginPasswordHash, defaultLoginPassword())
}

