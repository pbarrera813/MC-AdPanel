package minecraft

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"
)

// ServerConfig is what gets persisted to servers.json
type ServerConfig struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	Version             string   `json:"version"`
	Port                int      `json:"port"`
	JarFile             string   `json:"jarFile"`
	MaxRAM              string   `json:"maxRam"`
	MinRAM              string   `json:"minRam"`
	MaxPlayers          int      `json:"maxPlayers"`
	Dir                 string   `json:"dir"`
	StartCommand        []string `json:"startCommand,omitempty"`
	AutoStart           bool     `json:"autoStart"`
	Flags               string   `json:"flags"`
	AlwaysPreTouch      bool     `json:"alwaysPreTouch"`
	BackupSchedule      string   `json:"backupSchedule,omitempty"`
	LastScheduledBackup string   `json:"lastScheduledBackup,omitempty"`
}

// ServerInfo is the API-facing struct with runtime state
type ServerInfo struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	Version            string  `json:"version"`
	Status             string  `json:"status"`
	CPU                float64 `json:"cpu"`
	RAM                float64 `json:"ram"`
	TPS                float64 `json:"tps"`
	Port               int     `json:"port"`
	MaxRAM             string  `json:"maxRam"`
	MinRAM             string  `json:"minRam"`
	MaxPlayers         int     `json:"maxPlayers"`
	AutoStart          bool    `json:"autoStart"`
	Flags              string  `json:"flags"`
	AlwaysPreTouch     bool    `json:"alwaysPreTouch"`
	InstallError       string  `json:"installError,omitempty"`
	FabricTpsAvailable bool    `json:"fabricTpsAvailable,omitempty"`
}

// PluginInfo represents a plugin jar file
type PluginInfo struct {
	Name          string `json:"name"`
	FileName      string `json:"fileName"`
	Size          string `json:"size"`
	Enabled       bool   `json:"enabled"`
	Version       string `json:"version"`
	LatestVersion string `json:"latestVersion,omitempty"`
	VersionStatus string `json:"versionStatus,omitempty"`
	UpdateURL     string `json:"updateUrl,omitempty"`
	SourceURL     string `json:"sourceUrl,omitempty"`
}

// BackupInfo represents a backup archive
type BackupInfo struct {
	Name string `json:"name"`
	Date string `json:"date"`
	Size string `json:"size"`
}

// FileEntry represents a file or directory in the server's filesystem
type FileEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Size    string `json:"size"`
	ModTime string `json:"modTime"`
}

// PlayerInfo represents an online player
type PlayerInfo struct {
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Ping       int    `json:"ping"`
	World      string `json:"world"`
	OnlineTime string `json:"onlineTime"`
}

// onlinePlayer tracks a connected player's session
type onlinePlayer struct {
	Name     string
	IP       string
	Ping     int
	World    string
	JoinedAt time.Time
}

// CrashReport represents a crash report file
type CrashReport struct {
	Name  string `json:"name"`
	Date  string `json:"date"`
	Size  string `json:"size"`
	Cause string `json:"cause"`
}

// ConsoleLogEntry represents one console line with a monotonic sequence ID.
type ConsoleLogEntry struct {
	Seq  uint64 `json:"seq"`
	Line string `json:"line"`
}

// runningServer holds runtime state for a managed server
type runningServer struct {
	cmd                *exec.Cmd
	stdin              io.WriteCloser
	status             string
	cpu                float64
	ram                float64
	tps                float64
	pid                int
	logBuffer          []ConsoleLogEntry
	subscribers        []chan ConsoleLogEntry
	nextLogSeq         uint64
	players            map[string]*onlinePlayer
	pingBlocked        map[string]bool
	lastPingPlayer     string
	restartTimer       *time.Timer
	restartAt          time.Time
	installError       string
	lastTpsCmd         time.Time
	lastPlayerInfoCmd  time.Time
	lastPingCmd        time.Time
	pendingListRefresh bool
	nextListRefreshAt  time.Time
	pingSupported      bool
	pingDisabledReason string
	safeModeDisabled   []string // dirs renamed for safe mode (original paths)
	mu                 sync.RWMutex
	stopMetrics        chan struct{}
}

const maxLogBuffer = 2000
const logTrimSize = 200

// Regex patterns for player tracking and server info
var (
	// Support Java names and Floodgate-prefixed Bedrock names (prefix is configurable).
	playerNamePattern   = `([^\s\[\]:]+)`
	joinPattern         = regexp.MustCompile(playerNamePattern + `\[/([0-9a-fA-F:.]+):\d+\] logged in`)
	leavePattern        = regexp.MustCompile(playerNamePattern + ` left the game`)
	ansiPattern         = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	mcColorPattern      = regexp.MustCompile(`§[0-9a-fk-or]`)
	nameSanitize        = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)
	tpsPattern          = regexp.MustCompile(`TPS from last 1m, 5m, 15m: \*?([0-9.]+)`)
	forgeTpsPattern     = regexp.MustCompile(`(?i)overall:\s*(?:tps[:=]\s*)?([0-9.]+)\s*tps\b|overall:.*\btps[:=]\s*([0-9.]+)`)
	simpleTpsPattern    = regexp.MustCompile(`(?i)\bTPS[:=]\s*([0-9.]+)`)
	dimensionPattern    = regexp.MustCompile(playerNamePattern + ` has the following entity data: "minecraft:(\w+)"`)
	listPattern         = regexp.MustCompile(`There are (\d+) of a max of (\d+) players online:\s*(.*)`)
	pingPattern1        = regexp.MustCompile(`(?i)ping of ` + playerNamePattern + ` (?:is|was) ([0-9]+)`)
	pingPattern2        = regexp.MustCompile(`(?i)` + playerNamePattern + `'?s ping(?: is|:)? ([0-9]+)`)
	pingPattern3        = regexp.MustCompile(`(?i)` + playerNamePattern + ` has (?:a )?ping(?: of)? ([0-9]+)`)
	pingPattern4        = regexp.MustCompile(`(?i)` + playerNamePattern + `'s latency is ([0-9]+)\s*ms`)
	pingNotFoundPattern = regexp.MustCompile(`(?i)player not found or offline`)
)

// Manager coordinates all Minecraft server processes
type Manager struct {
	configs       map[string]*ServerConfig
	running       map[string]*runningServer
	dataFile      string
	settingsFile  string
	settingsMu    sync.RWMutex
	settings      AppSettings
	baseDir       string
	stopScheduler chan struct{}
	mu            sync.RWMutex
}

var hiddenServerRootArtifacts = map[string]struct{}{
	".adpanel-extension-sources.json": {},
	".console_history":                {},
}

// sanitizeName converts a server name to a safe directory name
func sanitizeName(name string) string {
	result := strings.ReplaceAll(name, " ", "_")
	result = nameSanitize.ReplaceAllString(result, "")
	if result == "" {
		result = "server"
	}
	return result
}

// formatFileSize formats bytes into human-readable size
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func isModdedType(serverType string) bool {
	switch strings.ToLower(serverType) {
	case "forge", "fabric", "neoforge":
		return true
	default:
		return false
	}
}

func isProxyType(serverType string) bool {
	switch strings.ToLower(serverType) {
	case "velocity":
		return true
	default:
		return false
	}
}

func listCommandForType(serverType string) string {
	switch strings.ToLower(serverType) {
	case "paper", "spigot", "purpur", "folia":
		return "minecraft:list"
	default:
		return "list"
	}
}

func tpsCommandForType(serverType string) (string, bool) {
	switch strings.ToLower(serverType) {
	case "paper", "spigot", "purpur", "folia":
		return "tps", true
	case "forge":
		return "forge tps", true
	case "neoforge":
		return "neoforge tps", true
	case "fabric":
		return "fabric tps", true
	default:
		return "", false
	}
}

func scheduleListRefreshLocked(rs *runningServer, delay time.Duration) {
	when := time.Now().Add(delay)
	if !rs.pendingListRefresh || rs.nextListRefreshAt.IsZero() || when.Before(rs.nextListRefreshAt) {
		rs.pendingListRefresh = true
		rs.nextListRefreshAt = when
	}
}

func hasFabricTps(modsDir string) bool {
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".jar") {
			continue
		}
		if strings.Contains(name, "fabric-tps") || strings.Contains(name, "fabric_tps") || strings.Contains(name, "fabrictps") {
			return true
		}
	}
	return false
}

func hasPingPlayer(pluginsDir string) bool {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".jar") {
			continue
		}
		jarPath := filepath.Join(pluginsDir, entry.Name())
		pluginName, _ := extractPluginVersion(jarPath)
		if strings.EqualFold(pluginName, "PingPlayer") || strings.EqualFold(pluginName, "pingplayer") {
			return true
		}
		if strings.Contains(name, "pingplayer") {
			return true
		}
	}
	return false
}

func hasPingPlayerMod(modsDir string) bool {
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".jar") {
			continue
		}
		if strings.Contains(name, "player-ping") || strings.Contains(name, "player_ping") || strings.Contains(name, "playerping") || strings.Contains(name, "pingplayer") {
			return true
		}
	}
	return false
}

func (m *Manager) refreshPingSupport(id string) {
	m.mu.RLock()
	cfg := m.configs[id]
	rs := m.running[id]
	m.mu.RUnlock()
	if cfg == nil || rs == nil {
		return
	}

	if isModdedType(cfg.Type) {
		modsDir := filepath.Join(cfg.Dir, "mods")
		supported := hasPingPlayerMod(modsDir)
		rs.mu.Lock()
		rs.pingSupported = supported
		if supported {
			rs.pingDisabledReason = ""
		} else {
			rs.pingDisabledReason = "missing_pingplayer_mod"
		}
		rs.mu.Unlock()
		return
	}
	if strings.EqualFold(cfg.Type, "vanilla") {
		rs.mu.Lock()
		rs.pingSupported = false
		rs.pingDisabledReason = "unsupported_server_type"
		rs.mu.Unlock()
		return
	}

	pluginsDir := filepath.Join(cfg.Dir, "plugins")
	supported := hasPingPlayer(pluginsDir)
	rs.mu.Lock()
	rs.pingSupported = supported
	if supported {
		rs.pingDisabledReason = ""
	} else {
		rs.pingDisabledReason = "missing_pingplayer"
	}
	rs.mu.Unlock()
}

// NewManager creates a new Manager with the given base directory (e.g. /AdPanel)
func NewManager(baseDir string) (*Manager, error) {
	dataDir := filepath.Join(baseDir, "data")
	serversDir := filepath.Join(baseDir, "Servers")

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	if err := os.MkdirAll(serversDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create servers directory: %w", err)
	}
	backupsDir := filepath.Join(baseDir, "Backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backups directory: %w", err)
	}

	mgr := &Manager{
		configs:       make(map[string]*ServerConfig),
		running:       make(map[string]*runningServer),
		dataFile:      filepath.Join(dataDir, "servers.json"),
		settingsFile:  filepath.Join(dataDir, "settings.json"),
		baseDir:       baseDir,
		stopScheduler: make(chan struct{}),
	}

	if err := mgr.load(); err != nil {
		return nil, err
	}
	mgr.migrateLegacyServerArtifacts()
	if err := mgr.loadSettings(); err != nil {
		return nil, err
	}
	if mgr.IsUsingDefaultLogin() {
		log.Printf("Auth initialized with default credentials: username=%q password=%q", "mcpanel", "mcpanel")
		log.Printf("Change default credentials in System Settings after first login.")
	}

	for id := range mgr.configs {
		mgr.running[id] = &runningServer{
			status:      "Stopped",
			logBuffer:   make([]ConsoleLogEntry, 0),
			nextLogSeq:  1,
			players:     make(map[string]*onlinePlayer),
			pingBlocked: make(map[string]bool),
		}
	}

	// Auto-start servers that have AutoStart enabled
	for id, cfg := range mgr.configs {
		if cfg.AutoStart {
			go func(serverID, serverName string) {
				time.Sleep(2 * time.Second)
				log.Printf("Auto-starting server: %s", serverName)
				if err := mgr.StartServer(serverID); err != nil {
					log.Printf("Auto-start failed for %s: %v", serverName, err)
				} else {
					log.Printf("Auto-started server: %s", serverName)
				}
			}(id, cfg.Name)
		}
	}

	// Start the scheduled backup checker
	go mgr.runBackupScheduler()

	return mgr, nil
}

func isServerRootSubPath(subPath string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(subPath))
	return cleaned == "." || cleaned == "/"
}

func shouldHideServerRootArtifact(subPath, name string) bool {
	if !isServerRootSubPath(subPath) {
		return false
	}
	_, hidden := hiddenServerRootArtifacts[name]
	return hidden
}

func (m *Manager) migrateLegacyServerArtifacts() {
	for _, cfg := range m.configs {
		if cfg == nil {
			continue
		}

		legacySourcesPath := legacyExtensionSourcesPath(cfg)
		if legacyData, err := os.ReadFile(legacySourcesPath); err == nil {
			newPath := m.extensionSourcesPath(cfg)
			if _, err := os.Stat(newPath); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
					log.Printf("[%s] failed to create extension sources dir: %v", cfg.Name, err)
				} else if err := os.WriteFile(newPath, legacyData, 0644); err != nil {
					log.Printf("[%s] failed to migrate extension sources: %v", cfg.Name, err)
				}
			}
			if err := os.Remove(legacySourcesPath); err != nil && !os.IsNotExist(err) {
				log.Printf("[%s] failed to remove legacy extension sources file: %v", cfg.Name, err)
			}
		}

		legacyConsoleHistoryPath := filepath.Join(cfg.Dir, ".console_history")
		if err := os.Remove(legacyConsoleHistoryPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[%s] failed to remove legacy console history file: %v", cfg.Name, err)
		}
	}
}

// load reads servers.json into configs map
func (m *Manager) load() error {
	data, err := os.ReadFile(m.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read data file: %w", err)
	}

	var configs []*ServerConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("failed to parse data file: %w", err)
	}

	for _, cfg := range configs {
		m.configs[cfg.ID] = cfg
	}

	return nil
}

// persist writes all configs to servers.json atomically
func (m *Manager) persist() error {
	configs := make([]*ServerConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		configs = append(configs, cfg)
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configs: %w", err)
	}

	tmpFile := m.dataFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, m.dataFile); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// CreateServer creates a new server with the given config
func (m *Manager) CreateServer(name, serverType, version string, port int, minRAM, maxRAM string, maxPlayers int, flags string, alwaysPreTouch bool) (*ServerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cfg := range m.configs {
		if cfg.Port == port {
			return nil, fmt.Errorf("port %d is already in use by server %s", port, cfg.Name)
		}
	}

	id := uuid.New().String()[:8]
	dirName := sanitizeName(name)
	serverDir := filepath.Join(m.baseDir, "Servers", dirName)

	// If directory already exists, append short ID to avoid collision
	if _, err := os.Stat(serverDir); err == nil {
		serverDir = filepath.Join(m.baseDir, "Servers", dirName+"_"+id)
	}

	if err := os.MkdirAll(serverDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create server directory: %w", err)
	}

	// Create standard subdirectories (only for plugin-based servers)
	if !isModdedType(serverType) {
		os.MkdirAll(filepath.Join(serverDir, "plugins"), 0755)
	}

	// Write eula.txt
	eulaPath := filepath.Join(serverDir, "eula.txt")
	if err := os.WriteFile(eulaPath, []byte("eula=true\n"), 0644); err != nil {
		return nil, fmt.Errorf("failed to write eula.txt: %w", err)
	}

	// Write server.properties
	props := fmt.Sprintf(
		"server-port=%d\nmotd=A Minecraft Server\nmax-players=%d\nonline-mode=true\nview-distance=10\n",
		port, maxPlayers,
	)
	propsPath := filepath.Join(serverDir, "server.properties")
	if err := os.WriteFile(propsPath, []byte(props), 0644); err != nil {
		return nil, fmt.Errorf("failed to write server.properties: %w", err)
	}

	cfg := &ServerConfig{
		ID:             id,
		Name:           name,
		Type:           serverType,
		Version:        version,
		Port:           port,
		JarFile:        "server.jar",
		MaxRAM:         maxRAM,
		MinRAM:         minRAM,
		MaxPlayers:     maxPlayers,
		Dir:            serverDir,
		Flags:          flags,
		AlwaysPreTouch: alwaysPreTouch,
	}

	m.configs[id] = cfg
	m.running[id] = &runningServer{
		status:      "Installing",
		logBuffer:   make([]ConsoleLogEntry, 0),
		nextLogSeq:  1,
		players:     make(map[string]*onlinePlayer),
		pingBlocked: make(map[string]bool),
	}

	if err := m.persist(); err != nil {
		return nil, fmt.Errorf("failed to persist config: %w", err)
	}

	// Launch async jar download
	go m.installServerJar(id, serverType, version)

	return m.serverInfo(id), nil
}

// buildJVMFlags returns extra JVM arguments based on the flags preset
func buildJVMFlags(flags string, alwaysPreTouch bool) []string {
	var args []string
	switch flags {
	case "aikars":
		args = []string{
			"--add-modules=jdk.incubator.vector",
			"-XX:+UseG1GC",
			"-XX:+ParallelRefProcEnabled",
			"-XX:MaxGCPauseMillis=200",
			"-XX:+UnlockExperimentalVMOptions",
			"-XX:+DisableExplicitGC",
			"-XX:G1HeapWastePercent=5",
			"-XX:G1MixedGCCountTarget=4",
			"-XX:InitiatingHeapOccupancyPercent=15",
			"-XX:G1MixedGCLiveThresholdPercent=90",
			"-XX:G1RSetUpdatingPauseTimePercent=5",
			"-XX:SurvivorRatio=32",
			"-XX:+PerfDisableSharedMem",
			"-XX:MaxTenuringThreshold=1",
			"-Dusing.aikars.flags=https://mcflags.emc.gs",
			"-Daikars.new.flags=true",
			"-XX:G1NewSizePercent=30",
			"-XX:G1MaxNewSizePercent=40",
			"-XX:G1HeapRegionSize=8M",
			"-XX:G1ReservePercent=20",
		}
	case "velocity":
		args = []string{
			"-XX:+UseG1GC",
			"-XX:G1HeapRegionSize=4M",
			"-XX:+UnlockExperimentalVMOptions",
			"-XX:+ParallelRefProcEnabled",
			"-XX:MaxInlineLevel=15",
		}
	case "modded":
		args = []string{
			"-XX:+UseG1GC",
			"-XX:+UnlockExperimentalVMOptions",
			"-XX:MaxGCPauseMillis=50",
			"-XX:+DisableExplicitGC",
			"-XX:G1NewSizePercent=30",
			"-XX:G1MaxNewSizePercent=40",
			"-XX:G1HeapRegionSize=16M",
			"-XX:InitiatingHeapOccupancyPercent=15",
			"-XX:G1MixedGCLiveThresholdPercent=50",
			"-XX:+PerfDisableSharedMem",
		}
	case "none", "":
		args = []string{
			"--add-modules=jdk.incubator.vector",
		}
	default:
		// Unknown preset — apply baseline compatibility flag.
		args = []string{
			"--add-modules=jdk.incubator.vector",
		}
	}
	if alwaysPreTouch && len(args) > 0 {
		args = append(args, "-XX:+AlwaysPreTouch")
	}
	return args
}

func writeManagedUserJVMArgs(path string, extraFlags []string) error {
	content := "# JVM flags managed by Admin Panel\n"
	for _, f := range extraFlags {
		content += f + "\n"
	}
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// StartServer starts the Minecraft process for the given server
func (m *Manager) StartServer(id string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	rs, rsOk := m.running[id]
	m.mu.RUnlock()

	if !ok || !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.Lock()
	if rs.status == "Installing" {
		rs.mu.Unlock()
		return fmt.Errorf("server %s is still installing, please wait", id)
	}
	if rs.status == "Running" || rs.status == "Booting" {
		rs.mu.Unlock()
		return fmt.Errorf("server %s is already %s", id, rs.status)
	}

	// Determine start command
	var cmd *exec.Cmd
	if len(cfg.StartCommand) > 0 {
		// For StartCommand-based servers (e.g. Forge/NeoForge), keep user_jvm_args.txt
		// in sync with selected preset while avoiding unnecessary rewrites.
		extraFlags := buildJVMFlags(cfg.Flags, cfg.AlwaysPreTouch)
		jvmArgsPath := filepath.Join(cfg.Dir, "user_jvm_args.txt")
		if err := writeManagedUserJVMArgs(jvmArgsPath, extraFlags); err != nil {
			log.Printf("[%s] Failed to write user_jvm_args.txt: %v", cfg.Name, err)
		}
		cmd = exec.Command(cfg.StartCommand[0], cfg.StartCommand[1:]...)
	} else {
		jarPath := filepath.Join(cfg.Dir, cfg.JarFile)
		if _, err := os.Stat(jarPath); os.IsNotExist(err) {
			rs.mu.Unlock()
			return fmt.Errorf("server.jar not found at %s - please place the server jar file in the server directory", jarPath)
		}
		jvmArgs := []string{
			"-Xmx" + cfg.MaxRAM,
			"-Xms" + cfg.MinRAM,
		}
		jvmArgs = append(jvmArgs, buildJVMFlags(cfg.Flags, cfg.AlwaysPreTouch)...)
		jvmArgs = append(jvmArgs, "-jar", cfg.JarFile, "nogui")
		cmd = exec.Command("java", jvmArgs...)
	}
	cmd.Dir = cfg.Dir

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		rs.mu.Unlock()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		rs.mu.Unlock()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		rs.mu.Unlock()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		rs.mu.Unlock()
		return fmt.Errorf("failed to start server: %w", err)
	}

	rs.cmd = cmd
	rs.stdin = stdinPipe
	rs.status = "Booting"
	rs.pid = cmd.Process.Pid
	rs.logBuffer = make([]ConsoleLogEntry, 0)
	rs.nextLogSeq = 1
	rs.pendingListRefresh = false
	rs.nextListRefreshAt = time.Time{}
	rs.players = make(map[string]*onlinePlayer)
	rs.stopMetrics = make(chan struct{})
	rs.mu.Unlock()

	m.refreshPingSupport(id)

	log.Printf("[%s] Server starting (PID: %d) in %s", cfg.Name, rs.pid, cfg.Dir)

	go m.scanOutput(id, rs, stdoutPipe)
	go m.scanOutput(id, rs, stderrPipe)

	go func() {
		err := cmd.Wait()
		rs.mu.Lock()
		if rs.status == "Running" || rs.status == "Booting" {
			if err != nil {
				rs.status = "Crashed"
				log.Printf("[%s] Server crashed: %v", cfg.Name, err)
			} else {
				rs.status = "Stopped"
				log.Printf("[%s] Server stopped gracefully", cfg.Name)
			}
		}
		rs.cpu = 0
		rs.ram = 0
		rs.pid = 0
		rs.players = make(map[string]*onlinePlayer)

		// Restore safe mode disabled directories
		if len(rs.safeModeDisabled) > 0 {
			for _, origPath := range rs.safeModeDisabled {
				disabledPath := origPath + "_disabled"
				if err := os.Rename(disabledPath, origPath); err != nil {
					log.Printf("[%s] Failed to restore %s from safe mode: %v", cfg.Name, filepath.Base(origPath), err)
				} else {
					log.Printf("[%s] Restored %s from safe mode", cfg.Name, filepath.Base(origPath))
				}
			}
			rs.safeModeDisabled = nil
		}
		rs.mu.Unlock()

		select {
		case <-rs.stopMetrics:
		default:
			close(rs.stopMetrics)
		}
	}()

	go m.collectMetrics(id, rs)

	return nil
}

// StartServerSafeMode starts a server with plugins/mods disabled
// by temporarily renaming the plugins and mods directories.
// They are automatically restored when the server stops.
func (m *Manager) StartServerSafeMode(id string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	rs, rsOk := m.running[id]
	m.mu.RUnlock()

	if !ok || !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	// Rename plugins and mods dirs before starting
	var disabledDirs []string
	for _, dirName := range []string{"plugins", "mods"} {
		origPath := filepath.Join(cfg.Dir, dirName)
		disabledPath := origPath + "_disabled"
		if info, err := os.Stat(origPath); err == nil && info.IsDir() {
			if err := os.Rename(origPath, disabledPath); err != nil {
				// Restore any already-renamed dirs on failure
				for _, d := range disabledDirs {
					os.Rename(d+"_disabled", d)
				}
				return fmt.Errorf("failed to disable %s for safe mode: %w", dirName, err)
			}
			disabledDirs = append(disabledDirs, origPath)
			log.Printf("[%s] Safe mode: disabled %s", cfg.Name, dirName)
		}
	}

	// Start the server normally
	if err := m.StartServer(id); err != nil {
		// Restore dirs if start fails
		for _, d := range disabledDirs {
			os.Rename(d+"_disabled", d)
		}
		return err
	}

	// Record which dirs were disabled so they get restored on stop
	rs.mu.Lock()
	rs.safeModeDisabled = disabledDirs
	rs.mu.Unlock()

	return nil
}

// scanOutput reads from a pipe and broadcasts each line, tracking players
func (m *Manager) scanOutput(id string, rs *runningServer, pipe io.Reader) {
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		// Strip ANSI and Minecraft color codes for pattern matching
		clean := ansiPattern.ReplaceAllString(line, "")
		clean = mcColorPattern.ReplaceAllString(clean, "")
		clean = strings.TrimRight(clean, " \r")

		rs.mu.Lock()
		if strings.Contains(clean, "Done (") {
			isReadyLine := strings.Contains(clean, "! For help,") || strings.Contains(clean, ")!")
			if isReadyLine {
				rs.status = "Running"
				// Run one list scan shortly after boot to hydrate player list state.
				scheduleListRefreshLocked(rs, 2*time.Second)
				cfg := m.configs[id]
				if cfg != nil {
					log.Printf("[%s] Server is now running", cfg.Name)
				}
			}
		}

		if matches := joinPattern.FindStringSubmatch(clean); len(matches) >= 3 {
			playerName := matches[1]
			playerIP := matches[2]
			rs.players[playerName] = &onlinePlayer{
				Name:     playerName,
				IP:       playerIP,
				Ping:     -1,
				JoinedAt: time.Now(),
			}
			delete(rs.pingBlocked, playerName)
			// Reconcile player list state after join events without periodic list spam.
			scheduleListRefreshLocked(rs, 200*time.Millisecond)
		}

		if matches := leavePattern.FindStringSubmatch(clean); len(matches) >= 2 {
			playerName := matches[1]
			delete(rs.players, playerName)
			delete(rs.pingBlocked, playerName)
			// Reconcile player list state after leave events without periodic list spam.
			scheduleListRefreshLocked(rs, 200*time.Millisecond)
		}

		// Parse TPS response
		suppressLine := false
		internalCmdRecent := time.Since(rs.lastTpsCmd) < 5*time.Second
		playerCmdRecent := time.Since(rs.lastPlayerInfoCmd) < 10*time.Second
		pingCmdRecent := time.Since(rs.lastPingCmd) < 10*time.Second

		if matches := tpsPattern.FindStringSubmatch(clean); len(matches) >= 2 {
			if tpsVal, err := strconv.ParseFloat(matches[1], 64); err == nil {
				rs.tps = tpsVal
			}
			if internalCmdRecent {
				suppressLine = true
			}
		}
		if matches := forgeTpsPattern.FindStringSubmatch(clean); len(matches) >= 3 {
			tpsText := matches[1]
			if tpsText == "" {
				tpsText = matches[2]
			}
			if tpsVal, err := strconv.ParseFloat(tpsText, 64); err == nil {
				rs.tps = tpsVal
			}
			if internalCmdRecent {
				suppressLine = true
			}
		}
		if matches := simpleTpsPattern.FindStringSubmatch(clean); len(matches) >= 2 {
			if tpsVal, err := strconv.ParseFloat(matches[1], 64); err == nil {
				rs.tps = tpsVal
			}
			if internalCmdRecent {
				suppressLine = true
			}
		}

		// Parse dimension response
		if matches := dimensionPattern.FindStringSubmatch(clean); len(matches) >= 3 {
			playerName := matches[1]
			if p, ok := rs.players[playerName]; ok {
				world := matches[2]
				switch world {
				case "overworld":
					p.World = "Overworld"
				case "the_nether":
					p.World = "Nether"
				case "the_end":
					p.World = "The End"
				default:
					p.World = world
				}
			}
			if playerCmdRecent {
				suppressLine = true
			}
		}

		// Parse list response to verify online players
		if matches := listPattern.FindStringSubmatch(clean); matches != nil {
			nameStr := strings.TrimSpace(matches[3])
			if nameStr == "" {
				rs.players = make(map[string]*onlinePlayer)
			} else {
				names := strings.Split(nameStr, ",")
				onlineNames := make(map[string]bool)
				for _, n := range names {
					trimmed := strings.TrimSpace(n)
					if trimmed == "" {
						continue
					}
					onlineNames[trimmed] = true
					if _, ok := rs.players[trimmed]; !ok {
						rs.players[trimmed] = &onlinePlayer{
							Name:     trimmed,
							Ping:     -1,
							JoinedAt: time.Now(),
						}
					}
				}
				for name := range rs.players {
					if !onlineNames[name] {
						delete(rs.players, name)
						delete(rs.pingBlocked, name)
					}
				}
			}
			if playerCmdRecent {
				suppressLine = true
			}
		}

		parsePing := func(playerName string, pingStr string) {
			if pingVal, err := strconv.Atoi(pingStr); err == nil {
				if p, ok := rs.players[playerName]; ok {
					p.Ping = pingVal
				}
			}
		}

		if matches := pingPattern1.FindStringSubmatch(clean); len(matches) >= 3 {
			parsePing(matches[1], matches[2])
			if pingCmdRecent {
				suppressLine = true
			}
		} else if matches := pingPattern2.FindStringSubmatch(clean); len(matches) >= 3 {
			parsePing(matches[1], matches[2])
			if pingCmdRecent {
				suppressLine = true
			}
		} else if matches := pingPattern3.FindStringSubmatch(clean); len(matches) >= 3 {
			parsePing(matches[1], matches[2])
			if pingCmdRecent {
				suppressLine = true
			}
		} else if matches := pingPattern4.FindStringSubmatch(clean); len(matches) >= 3 {
			parsePing(matches[1], matches[2])
			if pingCmdRecent {
				suppressLine = true
			}
		} else if pingNotFoundPattern.MatchString(clean) {
			if pingCmdRecent && rs.lastPingPlayer != "" {
				rs.pingBlocked[rs.lastPingPlayer] = true
				if p, ok := rs.players[rs.lastPingPlayer]; ok {
					p.Ping = -1
				}
			}
			if pingCmdRecent {
				suppressLine = true
			}
		}

		// Suppress "issued server command" lines from internal polling
		if strings.Contains(clean, "issued server command: /tps") && internalCmdRecent {
			suppressLine = true
		}
		if playerCmdRecent {
			if strings.Contains(clean, "issued server command: /minecraft:list") ||
				strings.Contains(clean, "issued server command: /list") ||
				strings.Contains(clean, "issued server command: /data") {
				suppressLine = true
			}
			// Suppress "No entity was found" errors from dimension queries
			if strings.Contains(clean, "No entity was found") {
				suppressLine = true
			}
		}
		if pingCmdRecent {
			if strings.Contains(clean, "issued server command: /ping") ||
				strings.Contains(clean, "issued server command: /essentials:ping") {
				suppressLine = true
			}
		}

		rs.mu.Unlock()

		entry := m.appendLog(rs, line)
		if !suppressLine {
			m.broadcastLog(rs, entry)
		}
	}
}

// collectMetrics periodically reads CPU and RAM usage, and polls TPS
func (m *Manager) collectMetrics(id string, rs *runningServer) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	tpsTicks := 0
	pingTicks := 0
	listSafetyTicks := 0
	listCmd := "list"
	tpsCmd := ""
	hasTpsCmd := false
	serverType := ""
	serverDir := ""

	m.mu.RLock()
	if cfg, ok := m.configs[id]; ok && cfg != nil {
		listCmd = listCommandForType(cfg.Type)
		tpsCmd, hasTpsCmd = tpsCommandForType(cfg.Type)
		serverType = cfg.Type
		serverDir = cfg.Dir
	}
	m.mu.RUnlock()
	if isProxyType(serverType) {
		// Proxies are not gameplay servers, so list/tps polling is not useful.
		hasTpsCmd = false
	}
	if strings.EqualFold(serverType, "fabric") {
		hasTpsCmd = hasTpsCmd && hasFabricTps(filepath.Join(serverDir, "mods"))
	}

	for {
		select {
		case <-rs.stopMetrics:
			return
		case <-ticker.C:
			rs.mu.RLock()
			pid := rs.pid
			status := rs.status
			rs.mu.RUnlock()

			if pid == 0 {
				continue
			}

			// System-wide CPU usage
			cpuPercents, cpuErr := cpu.Percent(0, false)
			var cpuPercent float64
			if cpuErr == nil && len(cpuPercents) > 0 {
				cpuPercent = cpuPercents[0]
			}

			// Per-process RAM usage
			proc, err := process.NewProcess(int32(pid))
			if err != nil {
				continue
			}

			memInfo, err := proc.MemoryInfo()
			if err != nil {
				continue
			}

			rs.mu.Lock()
			rs.cpu = cpuPercent
			if memInfo != nil {
				rs.ram = float64(memInfo.RSS) / 1024 / 1024
			}
			rs.mu.Unlock()

			// Poll TPS every ~30 seconds
			tpsTicks++
			if tpsTicks >= 15 && status == "Running" && hasTpsCmd {
				tpsTicks = 0
				rs.mu.Lock()
				rs.lastTpsCmd = time.Now()
				rs.mu.Unlock()
				m.SendCommand(id, tpsCmd)
			}

			// Player list refresh is event-driven:
			// one scan after boot and re-scan on join/leave events.
			// Additionally run a low-frequency safety resync for custom/localized join/leave messages.
			if !isProxyType(serverType) && status == "Running" {
				listSafetyTicks++
				if listSafetyTicks >= 60 { // ~120 seconds at 2s tick interval
					listSafetyTicks = 0
					rs.mu.Lock()
					scheduleListRefreshLocked(rs, 0)
					rs.mu.Unlock()
				}

				shouldSendList := false
				now := time.Now()
				rs.mu.Lock()
				if rs.pendingListRefresh && (rs.nextListRefreshAt.IsZero() || !now.Before(rs.nextListRefreshAt)) {
					rs.pendingListRefresh = false
					rs.nextListRefreshAt = time.Time{}
					rs.lastPlayerInfoCmd = now
					shouldSendList = true
				}
				rs.mu.Unlock()
				if shouldSendList {
					m.SendCommand(id, listCmd)
				}
			} else if status != "Running" {
				listSafetyTicks = 0
			}

			// Poll ping via PingPlayer (if available) every ~20 seconds
			pingTicks++
			if pingTicks >= 10 && status == "Running" {
				pingTicks = 0
				rs.mu.RLock()
				pingSupported := rs.pingSupported
				playerNames := make([]string, 0, len(rs.players))
				for name := range rs.players {
					playerNames = append(playerNames, name)
				}
				rs.mu.RUnlock()

				if pingSupported && len(playerNames) > 0 {
					rs.mu.Lock()
					rs.lastPingCmd = time.Now()
					rs.mu.Unlock()
					for _, name := range playerNames {
						rs.mu.RLock()
						blocked := rs.pingBlocked[name]
						rs.mu.RUnlock()
						if blocked {
							continue
						}
						rs.mu.Lock()
						rs.lastPingPlayer = name
						rs.mu.Unlock()
						m.SendCommand(id, "ping "+name)
						time.Sleep(200 * time.Millisecond)
					}
				}
			}
		}
	}
}

// StopServer gracefully stops a Minecraft server
func (m *Manager) StopServer(id string) error {
	m.mu.RLock()
	cfg := m.configs[id]
	rs, ok := m.running[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.Lock()
	if rs.status != "Running" && rs.status != "Booting" {
		rs.mu.Unlock()
		return fmt.Errorf("server %s is not running (status: %s)", id, rs.status)
	}

	if rs.stdin != nil {
		_, err := io.WriteString(rs.stdin, "stop\n")
		if err != nil {
			log.Printf("[%s] Failed to send stop command: %v", cfg.Name, err)
		}
	}
	rs.mu.Unlock()

	done := make(chan struct{})
	go func() {
		if rs.cmd != nil && rs.cmd.Process != nil {
			rs.cmd.Process.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[%s] Server stopped", cfg.Name)
	case <-time.After(30 * time.Second):
		log.Printf("[%s] Stop timeout, killing process", cfg.Name)
		if rs.cmd != nil && rs.cmd.Process != nil {
			rs.cmd.Process.Kill()
		}
	}

	rs.mu.Lock()
	rs.status = "Stopped"
	rs.cpu = 0
	rs.ram = 0
	rs.pid = 0
	rs.players = make(map[string]*onlinePlayer)
	if rs.restartTimer != nil {
		rs.restartTimer.Stop()
		rs.restartTimer = nil
		rs.restartAt = time.Time{}
	}
	rs.mu.Unlock()

	return nil
}

// SendCommand writes a command to the server's stdin
func (m *Manager) SendCommand(id, command string) error {
	m.mu.RLock()
	rs, ok := m.running[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.status != "Running" && rs.status != "Booting" {
		return fmt.Errorf("server %s is not running", id)
	}

	if rs.stdin == nil {
		return fmt.Errorf("server %s has no stdin pipe", id)
	}

	_, err := io.WriteString(rs.stdin, command+"\n")
	return err
}

// RecordConsoleCommand appends and broadcasts a panel-issued command so it appears in live console history.
func (m *Manager) RecordConsoleCommand(id, command string) error {
	m.mu.RLock()
	rs, ok := m.running[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return nil
	}

	line := "> " + trimmed
	entry := m.appendLog(rs, line)
	m.broadcastLog(rs, entry)
	return nil
}

// SubscribeLogs returns a channel that receives log lines and an unsubscribe function
func (m *Manager) SubscribeLogs(id string) (chan string, func()) {
	snapshot, _, logCh, unsubscribe := m.SubscribeLogsWithSnapshot(id, 0)
	ch := make(chan string, len(snapshot)+1000)
	for _, entry := range snapshot {
		ch <- entry.Line
	}
	go func() {
		defer close(ch)
		for entry := range logCh {
			ch <- entry.Line
		}
	}()
	return ch, unsubscribe
}

// SubscribeLogsWithSnapshot returns missing log entries since lastSeq plus a live subscription channel.
func (m *Manager) SubscribeLogsWithSnapshot(id string, lastSeq uint64) ([]ConsoleLogEntry, bool, chan ConsoleLogEntry, func()) {
	m.mu.RLock()
	rs, ok := m.running[id]
	m.mu.RUnlock()

	if !ok {
		ch := make(chan ConsoleLogEntry)
		close(ch)
		return []ConsoleLogEntry{}, false, ch, func() {}
	}

	ch := make(chan ConsoleLogEntry, 1000)

	rs.mu.Lock()
	snapshot := make([]ConsoleLogEntry, 0, len(rs.logBuffer))
	reset := false
	if len(rs.logBuffer) > 0 {
		oldestSeq := rs.logBuffer[0].Seq
		newestSeq := rs.logBuffer[len(rs.logBuffer)-1].Seq
		requiresFullSnapshot := lastSeq == 0 || lastSeq+1 < oldestSeq || lastSeq > newestSeq
		if lastSeq > newestSeq {
			// Client has a newer sequence than this stream, which means server log stream restarted.
			reset = true
		}
		if requiresFullSnapshot {
			snapshot = append(snapshot, rs.logBuffer...)
		} else {
			for _, entry := range rs.logBuffer {
				if entry.Seq > lastSeq {
					snapshot = append(snapshot, entry)
				}
			}
		}
	} else if lastSeq > 0 {
		// Empty current buffer but client had history: treat as stream reset so UI can clear old logs.
		reset = true
	}
	rs.subscribers = append(rs.subscribers, ch)
	rs.mu.Unlock()

	unsubscribe := func() {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		for i, sub := range rs.subscribers {
			if sub == ch {
				rs.subscribers = append(rs.subscribers[:i], rs.subscribers[i+1:]...)
				break
			}
		}
	}

	return snapshot, reset, ch, unsubscribe
}

// appendLog adds a line to the circular log buffer
func (m *Manager) appendLog(rs *runningServer, line string) ConsoleLogEntry {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.nextLogSeq == 0 {
		rs.nextLogSeq = 1
	}
	entry := ConsoleLogEntry{
		Seq:  rs.nextLogSeq,
		Line: line,
	}
	rs.nextLogSeq++
	rs.logBuffer = append(rs.logBuffer, entry)
	if maxLogBuffer > 0 && len(rs.logBuffer) > maxLogBuffer {
		rs.logBuffer = rs.logBuffer[logTrimSize:]
	}
	return entry
}

// broadcastLog sends a line to all active subscribers
func (m *Manager) broadcastLog(rs *runningServer, entry ConsoleLogEntry) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	for _, ch := range rs.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

// GetStatus returns the current status and metrics for a server
func (m *Manager) GetStatus(id string) (*ServerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.configs[id]; !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	return m.serverInfo(id), nil
}

// ListServers returns all servers with their current status and metrics
func (m *Manager) ListServers() []ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	servers := make([]ServerInfo, 0, len(m.configs))
	for id := range m.configs {
		servers = append(servers, *m.serverInfo(id))
	}
	return servers
}

// serverInfo builds a ServerInfo from config and running state (caller must hold m.mu.RLock)
func (m *Manager) serverInfo(id string) *ServerInfo {
	cfg := m.configs[id]
	rs := m.running[id]

	info := &ServerInfo{
		ID:             cfg.ID,
		Name:           cfg.Name,
		Type:           cfg.Type,
		Version:        cfg.Version,
		Port:           cfg.Port,
		MaxRAM:         cfg.MaxRAM,
		MinRAM:         cfg.MinRAM,
		MaxPlayers:     cfg.MaxPlayers,
		AutoStart:      cfg.AutoStart,
		Flags:          cfg.Flags,
		AlwaysPreTouch: cfg.AlwaysPreTouch,
		Status:         "Stopped",
	}
	if strings.EqualFold(cfg.Type, "fabric") {
		info.FabricTpsAvailable = hasFabricTps(filepath.Join(cfg.Dir, "mods"))
	}

	if rs != nil {
		rs.mu.RLock()
		info.Status = rs.status
		info.CPU = rs.cpu
		info.RAM = rs.ram
		info.TPS = rs.tps
		info.InstallError = rs.installError
		rs.mu.RUnlock()
	}

	return info
}

// UpdateSettings updates RAM, MaxPlayers, and Port for a server (only when stopped)
func (m *Manager) UpdateSettings(id, minRAM, maxRAM string, maxPlayers int, port int) (*ServerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.configs[id]
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	rs := m.running[id]
	if rs != nil {
		rs.mu.RLock()
		status := rs.status
		rs.mu.RUnlock()
		if status == "Running" || status == "Booting" {
			return nil, fmt.Errorf("cannot change settings while server is running")
		}
	}

	if port < 1024 || port > 65535 {
		return nil, fmt.Errorf("port must be between 1024 and 65535")
	}
	if port != cfg.Port {
		for _, other := range m.configs {
			if other.ID != cfg.ID && other.Port == port {
				return nil, fmt.Errorf("port %d is already in use by server %s", port, other.Name)
			}
		}
	}

	cfg.MinRAM = minRAM
	cfg.MaxRAM = maxRAM
	cfg.MaxPlayers = maxPlayers
	cfg.Port = port
	m.persist()

	// Update max-players and server-port in server.properties
	propsPath := filepath.Join(cfg.Dir, "server.properties")
	data, err := os.ReadFile(propsPath)
	if err == nil {
		// Normalize line endings to \n for consistent processing
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		lines := strings.Split(content, "\n")
		foundPlayers := false
		foundPort := false
		for i, line := range lines {
			trimmed := strings.TrimRight(line, "\r ")
			if strings.HasPrefix(trimmed, "max-players=") {
				lines[i] = fmt.Sprintf("max-players=%d", maxPlayers)
				foundPlayers = true
			} else if strings.HasPrefix(trimmed, "server-port=") {
				lines[i] = fmt.Sprintf("server-port=%d", port)
				foundPort = true
			}
		}
		if !foundPlayers {
			lines = append(lines, fmt.Sprintf("max-players=%d", maxPlayers))
		}
		if !foundPort {
			lines = append(lines, fmt.Sprintf("server-port=%d", port))
		}
		os.WriteFile(propsPath, []byte(strings.Join(lines, "\n")), 0644)
	}

	return m.serverInfo(id), nil
}

// UpdateVersion updates a server to a newer server jar version (server must be stopped).
func (m *Manager) UpdateVersion(id, version string) (*ServerInfo, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, fmt.Errorf("version is required")
	}

	m.mu.Lock()
	cfg, ok := m.configs[id]
	rs, rsOk := m.running[id]
	if !ok || !rsOk {
		m.mu.Unlock()
		return nil, fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()
	if status == "Running" {
		m.mu.Unlock()
		return nil, fmt.Errorf("Can't update while server is running.")
	}
	if status == "Booting" || status == "Installing" {
		m.mu.Unlock()
		return nil, fmt.Errorf("server is busy")
	}

	rs.mu.Lock()
	rs.status = "Installing"
	rs.installError = ""
	rs.mu.Unlock()

	serverType := cfg.Type
	m.mu.Unlock()

	go m.installServerJar(id, serverType, version)

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serverInfo(id), nil
}

// SetAutoStart toggles the auto-start flag for a server
func (m *Manager) SetAutoStart(id string, enabled bool) (*ServerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.configs[id]
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	cfg.AutoStart = enabled
	m.persist()

	return m.serverInfo(id), nil
}

// SetFlags updates the JVM flags preset for a server
func (m *Manager) SetFlags(id, flags string, alwaysPreTouch bool) (*ServerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.configs[id]
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	cfg.Flags = flags
	cfg.AlwaysPreTouch = alwaysPreTouch
	m.persist()

	return m.serverInfo(id), nil
}

// RenameServer changes the display name of a server
func (m *Manager) RenameServer(id, name string) (*ServerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.configs[id]
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("server name cannot be empty")
	}

	oldBackupDir := m.backupDir(cfg)
	cfg.Name = name
	newBackupDir := m.backupDir(cfg)

	if oldBackupDir != newBackupDir {
		if err := m.migrateBackupDir(oldBackupDir, newBackupDir); err != nil {
			return nil, err
		}
	}

	if err := m.persist(); err != nil {
		return nil, err
	}

	return m.serverInfo(id), nil
}

func (m *Manager) migrateBackupDir(oldDir, newDir string) error {
	if _, err := os.Stat(oldDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect existing backup directory: %w", err)
	}

	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("failed to move backup directory: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to inspect target backup directory: %w", err)
	}

	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return fmt.Errorf("failed to read old backup directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(oldDir, entry.Name())
		targetName, err := uniqueFileNameInDir(newDir, entry.Name())
		if err != nil {
			return fmt.Errorf("failed to resolve backup name conflict for %q: %w", entry.Name(), err)
		}
		dstPath := filepath.Join(newDir, targetName)
		if err := os.Rename(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to move backup %q: %w", entry.Name(), err)
		}
	}

	if err := os.Remove(oldDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old backup directory: %w", err)
	}

	return nil
}

// StopAll gracefully stops all running servers
func (m *Manager) StopAll() {
	// Stop the backup scheduler
	close(m.stopScheduler)

	m.mu.RLock()
	ids := make([]string, 0)
	for id, rs := range m.running {
		rs.mu.RLock()
		if rs.status == "Running" || rs.status == "Booting" {
			ids = append(ids, id)
		}
		rs.mu.RUnlock()
	}
	m.mu.RUnlock()

	for _, id := range ids {
		log.Printf("Stopping server %s...", id)
		if err := m.StopServer(id); err != nil {
			log.Printf("Error stopping server %s: %v", id, err)
		}
	}
}

// DeleteServer removes a server config (must be stopped)
func (m *Manager) DeleteServer(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, cfgOk := m.configs[id]
	rs, rsOk := m.running[id]
	if !cfgOk || !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()

	if status == "Running" || status == "Booting" || status == "Installing" {
		return fmt.Errorf("cannot delete server %s while it is %s", id, status)
	}

	// Delete server directory
	if cfg.Dir != "" {
		if err := os.RemoveAll(cfg.Dir); err != nil {
			log.Printf("Warning: failed to delete server directory %s: %v", cfg.Dir, err)
		}
	}

	// Delete backup directory
	backupPath := m.backupDir(cfg)
	if err := os.RemoveAll(backupPath); err != nil {
		log.Printf("Warning: failed to delete backup directory %s: %v", backupPath, err)
	}

	delete(m.configs, id)
	delete(m.running, id)

	return m.persist()
}

// GetServerDir returns the directory path for a server
func (m *Manager) GetServerDir(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, ok := m.configs[id]
	if !ok {
		return "", fmt.Errorf("server %s not found", id)
	}
	return cfg.Dir, nil
}

// SafePath validates a subpath within a server directory to prevent path traversal
func SafePath(serverDir, subPath string) (string, error) {
	cleaned := filepath.Clean(subPath)
	if cleaned == "." || cleaned == "" {
		return serverDir, nil
	}
	fullPath := filepath.Join(serverDir, cleaned)
	absServer, _ := filepath.Abs(serverDir)
	absFull, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absFull, absServer) {
		return "", fmt.Errorf("invalid path: access denied")
	}
	return fullPath, nil
}

// GetFilePath returns the absolute safe path for a file within a server's directory
func (m *Manager) GetFilePath(id, subPath string) (string, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("server %s not found", id)
	}
	return SafePath(cfg.Dir, subPath)
}

func uniqueFileNameInDir(dirPath, fileName string) (string, error) {
	name := filepath.Base(strings.TrimSpace(fileName))
	if name == "" || name == "." || name == string(os.PathSeparator) {
		return "", fmt.Errorf("invalid file name")
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = name
		ext = ""
	}

	candidate := name
	for i := 1; ; i++ {
		candidatePath := filepath.Join(dirPath, candidate)
		if _, err := os.Stat(candidatePath); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
		candidate = fmt.Sprintf("%s(%d)%s", base, i, ext)
	}
}

// ResolveUploadSubPath returns a safe, non-conflicting path for an uploaded file.
func (m *Manager) ResolveUploadSubPath(id, subPath string) (string, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("server %s not found", id)
	}

	cleaned := filepath.Clean(subPath)
	dirSub := filepath.Dir(cleaned)
	fileName := filepath.Base(cleaned)

	dirPath, err := SafePath(cfg.Dir, dirSub)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", err
	}

	uniqueName, err := uniqueFileNameInDir(dirPath, fileName)
	if err != nil {
		return "", err
	}

	if dirSub == "." {
		return uniqueName, nil
	}
	return filepath.ToSlash(filepath.Join(dirSub, uniqueName)), nil
}

// ============================================================
// Plugin Methods
// ============================================================

// extensionsDir returns the correct directory for extensions based on server type.
// Forge, Fabric, and NeoForge use "mods"; everything else uses "plugins".
func extensionsDir(cfg *ServerConfig) string {
	switch cfg.Type {
	case "Forge", "Fabric", "NeoForge":
		return filepath.Join(cfg.Dir, "mods")
	default:
		return filepath.Join(cfg.Dir, "plugins")
	}
}

func legacyExtensionSourcesPath(cfg *ServerConfig) string {
	return filepath.Join(cfg.Dir, ".adpanel-extension-sources.json")
}

func normalizeExtensionSourceKey(fileName string) string {
	name := strings.TrimSpace(filepath.Base(fileName))
	if strings.HasSuffix(strings.ToLower(name), ".disabled") {
		name = name[:len(name)-len(".disabled")]
	}
	return name
}

func (m *Manager) extensionSourcesPath(cfg *ServerConfig) string {
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = sanitizeName(cfg.Name)
	}
	return filepath.Join(m.baseDir, "data", "extension-sources", id+".json")
}

func (m *Manager) loadExtensionSources(cfg *ServerConfig) map[string]string {
	path := m.extensionSourcesPath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}

	var sources map[string]string
	if err := json.Unmarshal(data, &sources); err != nil {
		return map[string]string{}
	}
	if sources == nil {
		return map[string]string{}
	}
	return sources
}

func (m *Manager) saveExtensionSources(cfg *ServerConfig, sources map[string]string) error {
	if sources == nil {
		sources = map[string]string{}
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.extensionSourcesPath(cfg)), 0755); err != nil {
		return err
	}
	return os.WriteFile(m.extensionSourcesPath(cfg), data, 0644)
}

func sourceForFile(sources map[string]string, fileName string) string {
	if sources == nil {
		return ""
	}
	key := normalizeExtensionSourceKey(fileName)
	return strings.TrimSpace(sources[key])
}

// ListPlugins scans the plugins/ or mods/ directory for .jar files
func (m *Manager) ListPlugins(id string) ([]PluginInfo, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	pluginsDir := extensionsDir(cfg)
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PluginInfo{}, nil
		}
		return nil, err
	}

	sources := m.loadExtensionSources(cfg)
	plugins := make([]PluginInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if strings.HasSuffix(lower, ".jar.disabled") {
			jarPath := filepath.Join(pluginsDir, entry.Name())
			pName, pVersion := extractPluginVersion(jarPath)
			if pName == "" {
				pName = strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".disabled"), ".jar")
			}
			plugins = append(plugins, PluginInfo{
				Name:      pName,
				FileName:  entry.Name(),
				Size:      formatFileSize(info.Size()),
				Enabled:   false,
				Version:   pVersion,
				SourceURL: sourceForFile(sources, entry.Name()),
			})
		} else if strings.HasSuffix(lower, ".jar") {
			jarPath := filepath.Join(pluginsDir, entry.Name())
			pName, pVersion := extractPluginVersion(jarPath)
			if pName == "" {
				pName = strings.TrimSuffix(entry.Name(), ".jar")
			}
			plugins = append(plugins, PluginInfo{
				Name:      pName,
				FileName:  entry.Name(),
				Size:      formatFileSize(info.Size()),
				Enabled:   true,
				Version:   pVersion,
				SourceURL: sourceForFile(sources, entry.Name()),
			})
		}
	}

	return plugins, nil
}

// UploadPlugin saves a .jar file to the server's plugins/mods directory.
// If a file with the same name exists, callers must choose whether to replace or skip it.
func (m *Manager) UploadPlugin(id, fileName string, data []byte, conflictAction string) (string, string, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("server %s not found", id)
	}

	if !strings.HasSuffix(strings.ToLower(fileName), ".jar") {
		return "", "", fmt.Errorf("only .jar files are allowed")
	}

	pDir := extensionsDir(cfg)
	os.MkdirAll(pDir, 0755)
	pluginPath, err := SafePath(pDir, fileName)
	if err != nil {
		return "", "", err
	}

	conflictAction = strings.ToLower(strings.TrimSpace(conflictAction))
	existingInfo, statErr := os.Stat(pluginPath)
	if statErr == nil {
		if existingInfo.IsDir() {
			return "", "", fmt.Errorf("cannot replace directory with file")
		}
		if conflictAction == "skip" {
			return fileName, "skipped", nil
		}
		if conflictAction != "replace" {
			return fileName, "conflict", os.ErrExist
		}
	} else if !os.IsNotExist(statErr) {
		return "", "", statErr
	}

	if err := os.WriteFile(pluginPath, data, 0644); err != nil {
		return "", "", err
	}
	status := "uploaded"
	if conflictAction == "replace" {
		status = "replaced"
	}
	return fileName, status, nil
}

// DeletePlugin removes a plugin jar from the server's plugins directory
func (m *Manager) DeletePlugin(id, fileName string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	pluginPath, err := SafePath(extensionsDir(cfg), fileName)
	if err != nil {
		return err
	}

	if err := os.Remove(pluginPath); err != nil {
		return err
	}

	sources := m.loadExtensionSources(cfg)
	key := normalizeExtensionSourceKey(fileName)
	if _, ok := sources[key]; ok {
		delete(sources, key)
		if err := m.saveExtensionSources(cfg, sources); err != nil {
			return err
		}
	}
	return nil
}

// TogglePlugin enables/disables a plugin by renaming .jar <-> .jar.disabled
func (m *Manager) TogglePlugin(id, fileName string) (*PluginInfo, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	pluginsDir := extensionsDir(cfg)

	if strings.HasSuffix(fileName, ".disabled") {
		oldPath, err := SafePath(pluginsDir, fileName)
		if err != nil {
			return nil, err
		}
		newName := strings.TrimSuffix(fileName, ".disabled")
		newPath, err := SafePath(pluginsDir, newName)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return nil, err
		}
		sources := m.loadExtensionSources(cfg)
		oldKey := normalizeExtensionSourceKey(fileName)
		newKey := normalizeExtensionSourceKey(newName)
		if oldKey != newKey {
			if src, ok := sources[oldKey]; ok && strings.TrimSpace(src) != "" {
				sources[newKey] = src
				delete(sources, oldKey)
				_ = m.saveExtensionSources(cfg, sources)
			}
		}
		info, _ := os.Stat(newPath)
		size := "0 B"
		if info != nil {
			size = formatFileSize(info.Size())
		}
		return &PluginInfo{
			Name:     strings.TrimSuffix(newName, ".jar"),
			FileName: newName,
			Size:     size,
			Enabled:  true,
		}, nil
	}

	oldPath, err := SafePath(pluginsDir, fileName)
	if err != nil {
		return nil, err
	}
	newName := fileName + ".disabled"
	newPath, err := SafePath(pluginsDir, newName)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return nil, err
	}
	sources := m.loadExtensionSources(cfg)
	oldKey := normalizeExtensionSourceKey(fileName)
	newKey := normalizeExtensionSourceKey(newName)
	if oldKey != newKey {
		if src, ok := sources[oldKey]; ok && strings.TrimSpace(src) != "" {
			sources[newKey] = src
			delete(sources, oldKey)
			_ = m.saveExtensionSources(cfg, sources)
		}
	}
	info, _ := os.Stat(newPath)
	size := "0 B"
	if info != nil {
		size = formatFileSize(info.Size())
	}
	return &PluginInfo{
		Name:     strings.TrimSuffix(fileName, ".jar"),
		FileName: newName,
		Size:     size,
		Enabled:  false,
	}, nil
}

// ============================================================
// Backup Methods
// ============================================================

// backupDir returns the centralized backup directory for a server
func (m *Manager) backupDir(cfg *ServerConfig) string {
	return filepath.Join(m.baseDir, "Backups", sanitizeName(cfg.Name))
}

// ListBackups returns all backup archives for a server
func (m *Manager) ListBackups(id string) ([]BackupInfo, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	backupsDir := m.backupDir(cfg)
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
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	backupsDir := m.backupDir(cfg)
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
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	backupPath, err := SafePath(m.backupDir(cfg), fileName)
	if err != nil {
		return err
	}

	return os.Remove(backupPath)
}

// GetBackupPath returns the full filesystem path for downloading a backup
func (m *Manager) GetBackupPath(id, fileName string) (string, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("server %s not found", id)
	}

	backupPath, err := SafePath(m.backupDir(cfg), fileName)
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
	cfg, ok := m.configs[id]
	rs, rsOk := m.running[id]
	m.mu.RUnlock()
	if !ok || !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()
	if status != "Stopped" && status != "Crashed" && status != "Error" {
		return fmt.Errorf("server must be stopped before restoring a backup")
	}

	backupPath, err := SafePath(m.backupDir(cfg), fileName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup %s not found", fileName)
	}

	// Clear server directory contents
	entries, err := os.ReadDir(cfg.Dir)
	if err != nil {
		return fmt.Errorf("failed to read server directory: %w", err)
	}
	for _, entry := range entries {
		os.RemoveAll(filepath.Join(cfg.Dir, entry.Name()))
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

	cfg, ok := m.configs[id]
	if !ok {
		return fmt.Errorf("server %s not found", id)
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

	cfg, ok := m.configs[id]
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
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

// ============================================================
// File Browser Methods
// ============================================================

// ListFiles returns directory contents at the given subpath
func (m *Manager) ListFiles(id, subPath string) ([]FileEntry, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
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
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryType := "file"
		if entry.IsDir() {
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
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
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
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
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
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	targetPath, err := SafePath(cfg.Dir, subPath)
	if err != nil {
		return err
	}

	absServer, _ := filepath.Abs(cfg.Dir)
	absTarget, _ := filepath.Abs(targetPath)
	if absServer == absTarget {
		return fmt.Errorf("cannot delete server root directory")
	}

	return os.RemoveAll(targetPath)
}

// CreateDirectory creates a directory within a server directory
func (m *Manager) CreateDirectory(id, subPath string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
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
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
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

// ============================================================
// Player Methods
// ============================================================

// ListPlayers returns currently online players tracked from log parsing
func (m *Manager) ListPlayers(id string) ([]PlayerInfo, error) {
	m.mu.RLock()
	rs, ok := m.running[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.status != "Running" {
		return []PlayerInfo{}, nil
	}

	players := make([]PlayerInfo, 0)
	for _, p := range rs.players {
		duration := time.Since(p.JoinedAt)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60

		var onlineTime string
		if hours > 0 {
			onlineTime = fmt.Sprintf("%dh %dm", hours, minutes)
		} else {
			onlineTime = fmt.Sprintf("%dm", minutes)
		}

		players = append(players, PlayerInfo{
			Name:       p.Name,
			IP:         p.IP,
			Ping:       p.Ping,
			World:      p.World,
			OnlineTime: onlineTime,
		})
	}

	sort.Slice(players, func(i, j int) bool {
		return players[i].Name < players[j].Name
	})

	return players, nil
}

func (m *Manager) GetPingSupport(id string) (bool, string, error) {
	m.mu.RLock()
	cfg, cfgOk := m.configs[id]
	rs, rsOk := m.running[id]
	m.mu.RUnlock()
	if !cfgOk {
		return false, "", fmt.Errorf("server %s not found", id)
	}

	if rsOk {
		rs.mu.RLock()
		supported := rs.pingSupported
		reason := rs.pingDisabledReason
		rs.mu.RUnlock()
		if supported || reason != "" {
			return supported, reason, nil
		}
	}

	if isModdedType(cfg.Type) {
		modsDir := filepath.Join(cfg.Dir, "mods")
		if !hasPingPlayerMod(modsDir) {
			return false, "missing_pingplayer_mod", nil
		}
		return true, "", nil
	}
	if strings.EqualFold(cfg.Type, "vanilla") {
		return false, "unsupported_server_type", nil
	}

	pluginsDir := filepath.Join(cfg.Dir, "plugins")
	if !hasPingPlayer(pluginsDir) {
		return false, "missing_pingplayer", nil
	}

	return true, "", nil
}

// KickPlayer sends a kick command to the server
func (m *Manager) KickPlayer(id, playerName, reason string) error {
	if reason == "" {
		return m.SendCommand(id, fmt.Sprintf("kick %s", playerName))
	}
	return m.SendCommand(id, fmt.Sprintf("kick %s %s", playerName, reason))
}

// BanPlayer sends a ban command to the server
func (m *Manager) BanPlayer(id, playerName, reason string) error {
	if reason == "" {
		return m.SendCommand(id, fmt.Sprintf("ban %s", playerName))
	}
	return m.SendCommand(id, fmt.Sprintf("ban %s %s", playerName, reason))
}

// KillPlayer sends a kill command to the server
func (m *Manager) KillPlayer(id, playerName string) error {
	return m.SendCommand(id, fmt.Sprintf("kill %s", playerName))
}

// ============================================================
// Schedule Restart
// ============================================================

// ScheduleRestart schedules a server restart after delaySeconds
func (m *Manager) ScheduleRestart(id string, delaySeconds int) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	rs, rsOk := m.running[id]
	m.mu.RUnlock()

	if !ok || !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.Lock()
	if rs.status != "Running" {
		rs.mu.Unlock()
		return fmt.Errorf("server must be running to schedule a restart")
	}

	// Cancel any existing scheduled restart
	if rs.restartTimer != nil {
		rs.restartTimer.Stop()
	}

	rs.restartAt = time.Now().Add(time.Duration(delaySeconds) * time.Second)

	rs.restartTimer = time.AfterFunc(time.Duration(delaySeconds)*time.Second, func() {
		log.Printf("[%s] Scheduled restart executing", cfg.Name)

		// Warn players
		m.SendCommand(id, "say Server restarting in 10 seconds...")
		time.Sleep(10 * time.Second)
		m.SendCommand(id, "say Server restarting now!")
		time.Sleep(1 * time.Second)

		if err := m.StopServer(id); err != nil {
			log.Printf("[%s] Scheduled restart - stop failed: %v", cfg.Name, err)
			return
		}

		// Wait for the server to fully stop
		time.Sleep(3 * time.Second)

		if err := m.StartServer(id); err != nil {
			log.Printf("[%s] Scheduled restart - start failed: %v", cfg.Name, err)
		} else {
			log.Printf("[%s] Scheduled restart completed", cfg.Name)
		}
	})
	rs.mu.Unlock()

	log.Printf("[%s] Restart scheduled in %d seconds", cfg.Name, delaySeconds)
	return nil
}

// CancelRestart cancels a scheduled restart
func (m *Manager) CancelRestart(id string) error {
	m.mu.RLock()
	rs, ok := m.running[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.restartTimer == nil {
		return fmt.Errorf("no restart scheduled for server %s", id)
	}

	rs.restartTimer.Stop()
	rs.restartTimer = nil
	rs.restartAt = time.Time{}

	return nil
}

// ============================================================
// Crash Reports
// ============================================================

// ListCrashReports scans the crash-reports/ directory
func (m *Manager) ListCrashReports(id string) ([]CrashReport, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
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

		// Try to extract cause from the file
		cause := extractCrashCause(filepath.Join(crashDir, entry.Name()))

		reports = append(reports, CrashReport{
			Name:  entry.Name(),
			Date:  info.ModTime().UTC().Format(time.RFC3339),
			Size:  formatFileSize(info.Size()),
			Cause: cause,
		})
	}

	// Newest first
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Date > reports[j].Date
	})

	return reports, nil
}

// extractCrashCause reads the first lines of a crash report to find the cause
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

// ReadCrashReport returns the content of a crash report file
func (m *Manager) ReadCrashReport(id, fileName string) ([]byte, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	filePath, err := SafePath(filepath.Join(cfg.Dir, "crash-reports"), fileName)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(filePath)
}

// ============================================================
// Log files (view when server is stopped)
// ============================================================

// ListLogFiles returns files under the server's logs/ directory
func (m *Manager) ListLogFiles(id string) ([]FileEntry, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
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
		// include common log file types: .log, .txt, .gz
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

	// sort newest first by modtime
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime > files[j].ModTime
	})

	return files, nil
}

// ReadLogFile returns the (possibly decompressed) content of a log file
func (m *Manager) ReadLogFile(id, fileName string) ([]byte, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	filePath, err := SafePath(filepath.Join(cfg.Dir, "logs"), fileName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// If gzipped, attempt to decompress
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

// CopyCrashReport duplicates a crash report file with a "-copy" suffix
func (m *Manager) CopyCrashReport(id, fileName string) (string, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("server %s not found", id)
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

// DeleteCrashReport deletes a crash report file
func (m *Manager) DeleteCrashReport(id, fileName string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	filePath, err := SafePath(filepath.Join(cfg.Dir, "crash-reports"), fileName)
	if err != nil {
		return err
	}

	return os.Remove(filePath)
}

// ============================================================
// Server Cloning
// ============================================================

// CloneServer creates a new server by copying data from a source server
func (m *Manager) CloneServer(sourceID, name string, port int, copyPlugins, copyWorlds, copyConfig bool) (*ServerInfo, error) {
	m.mu.RLock()
	sourceCfg, ok := m.configs[sourceID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("source server %s not found", sourceID)
	}

	// Create the new server first (this handles port conflicts, dir creation, etc.)
	newServer, err := m.CreateServer(name, sourceCfg.Type, sourceCfg.Version, port, sourceCfg.MinRAM, sourceCfg.MaxRAM, sourceCfg.MaxPlayers, sourceCfg.Flags, sourceCfg.AlwaysPreTouch)
	if err != nil {
		return nil, err
	}

	// Get the new server's directory
	m.mu.RLock()
	newCfg := m.configs[newServer.ID]
	m.mu.RUnlock()

	srcDir := sourceCfg.Dir
	dstDir := newCfg.Dir

	// Copy plugins
	if copyPlugins {
		srcPlugins := filepath.Join(srcDir, "plugins")
		dstPlugins := filepath.Join(dstDir, "plugins")
		if _, err := os.Stat(srcPlugins); err == nil {
			os.RemoveAll(dstPlugins)
			cmd := exec.Command("cp", "-r", srcPlugins, dstPlugins)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("Warning: failed to copy plugins: %s: %v", string(output), err)
			}
		}
	}

	// Copy worlds
	if copyWorlds {
		worldDirs := []string{"world", "world_nether", "world_the_end"}
		entries, _ := os.ReadDir(srcDir)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			isWorld := false
			for _, wd := range worldDirs {
				if entry.Name() == wd {
					isWorld = true
					break
				}
			}
			if !isWorld {
				continue
			}
			src := filepath.Join(srcDir, entry.Name())
			dst := filepath.Join(dstDir, entry.Name())
			cmd := exec.Command("cp", "-r", src, dst)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("Warning: failed to copy world %s: %s: %v", entry.Name(), string(output), err)
			}
		}
	}

	// Copy configuration files
	if copyConfig {
		configFiles := []string{
			"server.properties", "bukkit.yml", "spigot.yml", "paper.yml",
			"paper-global.yml", "purpur.yml", "config",
			"banned-players.json", "banned-ips.json", "ops.json", "whitelist.json",
		}
		for _, name := range configFiles {
			src := filepath.Join(srcDir, name)
			dst := filepath.Join(dstDir, name)
			info, err := os.Stat(src)
			if err != nil {
				continue
			}
			if info.IsDir() {
				cmd := exec.Command("cp", "-r", src, dst)
				cmd.CombinedOutput()
			} else {
				data, err := os.ReadFile(src)
				if err == nil {
					// Update port in server.properties for the new server
					if name == "server.properties" {
						content := string(data)
						content = regexp.MustCompile(`server-port=\d+`).ReplaceAllString(
							content, fmt.Sprintf("server-port=%d", port))
						data = []byte(content)
					}
					os.WriteFile(dst, data, 0644)
				}
			}
		}
	}

	return newServer, nil
}

// ============================================================
// Version Fetching & Jar Installation
// ============================================================

// GetVersions returns available versions for a server type (cached)
func (m *Manager) GetVersions(serverType string) ([]VersionInfo, error) {
	if cached, ok := globalVersionCache.Get(serverType); ok {
		return cached, nil
	}

	provider, err := GetProvider(serverType)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	versions, err := provider.FetchVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions for %s: %w", serverType, err)
	}

	globalVersionCache.Set(serverType, versions)
	return versions, nil
}

// installServerJar downloads and installs the server jar for a newly created server
func (m *Manager) installServerJar(id, serverType, version string) {
	m.mu.RLock()
	cfg := m.configs[id]
	rs := m.running[id]
	m.mu.RUnlock()

	if cfg == nil || rs == nil {
		return
	}

	provider, err := GetProvider(serverType)
	if err != nil {
		rs.mu.Lock()
		rs.status = "Error"
		rs.installError = err.Error()
		rs.mu.Unlock()
		log.Printf("[%s] Install error: %v", cfg.Name, err)
		return
	}

	// Resolve "Latest" to actual version
	actualVersion := version
	if strings.EqualFold(version, "latest") || strings.EqualFold(version, "") {
		versions, vErr := provider.FetchVersions(context.Background())
		if vErr != nil || len(versions) == 0 {
			rs.mu.Lock()
			rs.status = "Error"
			rs.installError = "Failed to resolve latest version"
			rs.mu.Unlock()
			return
		}
		for _, v := range versions {
			if v.Latest {
				actualVersion = v.Version
				break
			}
		}
		if strings.EqualFold(actualVersion, "latest") || actualVersion == "" {
			actualVersion = versions[0].Version
		}
	}

	progressFn := func(msg string) {
		log.Printf("[%s] Install: %s", cfg.Name, msg)
		entry := m.appendLog(rs, fmt.Sprintf("[Installer] %s", msg))
		m.broadcastLog(rs, entry)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	err = provider.DownloadJar(ctx, actualVersion, cfg.Dir, progressFn)
	if err != nil {
		rs.mu.Lock()
		rs.status = "Error"
		rs.installError = fmt.Sprintf("Download failed: %v", err)
		rs.mu.Unlock()
		log.Printf("[%s] Install failed: %v", cfg.Name, err)
		return
	}

	// For Forge/NeoForge: detect run.sh and set StartCommand
	if strings.EqualFold(serverType, "forge") || strings.EqualFold(serverType, "neoforge") {
		runSh := filepath.Join(cfg.Dir, "run.sh")
		if _, err := os.Stat(runSh); err == nil {
			os.Chmod(runSh, 0755)
			m.mu.Lock()
			cfg.StartCommand = []string{"bash", "run.sh", "nogui"}
			m.persist()
			m.mu.Unlock()
			progressFn("Detected run.sh — server will use Forge/NeoForge launch script.")
		}
	}

	// Persist resolved/new version after a successful install/update.
	m.mu.Lock()
	cfg.Version = actualVersion
	m.persist()
	m.mu.Unlock()

	rs.mu.Lock()
	rs.status = "Stopped"
	rs.installError = ""
	rs.mu.Unlock()

	log.Printf("[%s] Installation complete (version %s). Server is ready to start.", cfg.Name, actualVersion)
	progressFn(fmt.Sprintf("Installation complete! %s %s is ready to start.", serverType, actualVersion))
}

// RetryInstall retries a failed installation
func (m *Manager) RetryInstall(id string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	rs, rsOk := m.running[id]
	m.mu.RUnlock()

	if !ok || !rsOk {
		return fmt.Errorf("server %s not found", id)
	}

	rs.mu.Lock()
	if rs.status != "Error" {
		rs.mu.Unlock()
		return fmt.Errorf("server %s is not in error state (status: %s)", id, rs.status)
	}
	rs.status = "Installing"
	rs.installError = ""
	rs.mu.Unlock()

	go m.installServerJar(id, cfg.Type, cfg.Version)
	return nil
}
