package minecraft

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

type AppSettings struct {
	UserAgent          string `json:"userAgent"`
	DefaultMinRAM      string `json:"defaultMinRam,omitempty"`
	DefaultMaxRAM      string `json:"defaultMaxRam,omitempty"`
	DefaultFlags       string `json:"defaultFlags,omitempty"`
	StatusPollInterval int    `json:"statusPollInterval,omitempty"`
	TpsPollInterval    int    `json:"tpsPollInterval,omitempty"`
	PlayerSyncInterval int    `json:"playerSyncInterval,omitempty"`
	PingPollInterval   int    `json:"pingPollInterval,omitempty"`
	LoginUser          string `json:"loginUser,omitempty"`
	LoginPasswordHash  string `json:"loginPasswordHash,omitempty"`
}

var (
	userAgentMu       sync.RWMutex
	userAgentOverride string
)

const (
	passwordHashSchemeArgon2id  = "argon2id"
	passwordHashSchemeLegacySHA = "sha256"
	LoginPasswordMinLength      = 10
	argon2MemoryKiB             = 64 * 1024
	argon2Iterations            = 3
	argon2Parallelism           = 2
	argon2SaltLength            = 16
	argon2KeyLength             = 32
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
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2MemoryKiB, argon2Parallelism, argon2KeyLength)
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(key)
	return fmt.Sprintf(
		"%s$v=19$m=%d,t=%d,p=%d$%s$%s",
		passwordHashSchemeArgon2id,
		argon2MemoryKiB,
		argon2Iterations,
		argon2Parallelism,
		saltB64,
		hashB64,
	), nil
}

func hashPasswordLegacySHA256(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	sum := sha256.Sum256(append(salt, []byte(password)...))
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(sum[:])
	return passwordHashSchemeLegacySHA + "$" + saltB64 + "$" + hashB64, nil
}

func verifyPasswordLegacySHA256(storedHash, password string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 3 || parts[0] != passwordHashSchemeLegacySHA {
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

func verifyPasswordArgon2id(storedHash, password string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 5 || parts[0] != passwordHashSchemeArgon2id {
		return false
	}
	if parts[1] != "v=19" {
		return false
	}

	var memory uint64
	var iterations uint64
	var parallelism uint64
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	if memory == 0 || iterations == 0 || parallelism == 0 {
		return false
	}
	if memory > uint64(^uint32(0)) || iterations > uint64(^uint32(0)) || parallelism > uint64(^uint8(0)) {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	if len(want) == 0 || len(want) > 1024 {
		return false
	}

	got := argon2.IDKey(
		[]byte(password),
		salt,
		uint32(iterations),
		uint32(memory),
		uint8(parallelism),
		uint32(len(want)),
	)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func verifyPasswordWithUpgrade(storedHash, password string) (valid bool, needsUpgrade bool) {
	storedHash = strings.TrimSpace(storedHash)
	switch {
	case strings.HasPrefix(storedHash, passwordHashSchemeArgon2id+"$"):
		return verifyPasswordArgon2id(storedHash, password), false
	case strings.HasPrefix(storedHash, passwordHashSchemeLegacySHA+"$"):
		return verifyPasswordLegacySHA256(storedHash, password), true
	default:
		return false, false
	}
}

func verifyPassword(storedHash, password string) bool {
	valid, _ := verifyPasswordWithUpgrade(storedHash, password)
	return valid
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
	if cfg.StatusPollInterval > 30 {
		cfg.StatusPollInterval = 30
	}
	if cfg.TpsPollInterval <= 0 {
		cfg.TpsPollInterval = 30
	}
	if cfg.TpsPollInterval < 5 {
		cfg.TpsPollInterval = 5
	}
	if cfg.TpsPollInterval > 300 {
		cfg.TpsPollInterval = 300
	}
	if cfg.PlayerSyncInterval <= 0 {
		cfg.PlayerSyncInterval = 15
	}
	if cfg.PlayerSyncInterval < 2 {
		cfg.PlayerSyncInterval = 2
	}
	if cfg.PlayerSyncInterval > 300 {
		cfg.PlayerSyncInterval = 300
	}
	if cfg.PingPollInterval <= 0 {
		cfg.PingPollInterval = 20
	}
	if cfg.PingPollInterval < 5 {
		cfg.PingPollInterval = 5
	}
	if cfg.PingPollInterval > 300 {
		cfg.PingPollInterval = 300
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
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp settings: %w", err)
	}
	if err := os.Rename(tmpFile, m.settingsFile); err != nil {
		return fmt.Errorf("failed to rename settings file: %w", err)
	}
	if err := os.Chmod(m.settingsFile, 0600); err != nil {
		return fmt.Errorf("failed to secure settings file permissions: %w", err)
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

func (m *Manager) UpdateAppSettings(
	userAgent,
	defaultMinRAM,
	defaultMaxRAM,
	defaultFlags string,
	statusPollInterval,
	tpsPollInterval,
	playerSyncInterval,
	pingPollInterval int,
	loginUser,
	loginPassword string,
) (AppSettings, error) {
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
	if tpsPollInterval <= 0 {
		tpsPollInterval = 30
	}
	if tpsPollInterval < 5 {
		tpsPollInterval = 5
	}
	if tpsPollInterval > 300 {
		tpsPollInterval = 300
	}
	if playerSyncInterval <= 0 {
		playerSyncInterval = 15
	}
	if playerSyncInterval < 2 {
		playerSyncInterval = 2
	}
	if playerSyncInterval > 300 {
		playerSyncInterval = 300
	}
	if pingPollInterval <= 0 {
		pingPollInterval = 20
	}
	if pingPollInterval < 5 {
		pingPollInterval = 5
	}
	if pingPollInterval > 300 {
		pingPollInterval = 300
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
		if len(loginPassword) < LoginPasswordMinLength {
			return AppSettings{}, fmt.Errorf("password must be at least %d characters", LoginPasswordMinLength)
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
		TpsPollInterval:    tpsPollInterval,
		PlayerSyncInterval: playerSyncInterval,
		PingPollInterval:   pingPollInterval,
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
	trimmedUsername := strings.TrimSpace(username)

	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	if trimmedUsername != m.settings.LoginUser {
		return false
	}
	valid, needsUpgrade := verifyPasswordWithUpgrade(m.settings.LoginPasswordHash, password)
	if !valid {
		return false
	}
	if needsUpgrade {
		hashed, err := hashPassword(password)
		if err != nil {
			log.Printf("Failed to upgrade password hash format: %v", err)
			return true
		}
		m.settings.LoginPasswordHash = hashed
		if err := m.persistSettings(); err != nil {
			log.Printf("Failed to persist upgraded password hash: %v", err)
		}
	}
	return true
}

func (m *Manager) IsUsingDefaultLogin() bool {
	m.settingsMu.RLock()
	defer m.settingsMu.RUnlock()

	return m.settings.LoginUser == defaultLoginUser() && verifyPassword(m.settings.LoginPasswordHash, defaultLoginPassword())
}
