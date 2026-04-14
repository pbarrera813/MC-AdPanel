package minecraft

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const importAnalysisTTL = 15 * time.Minute

var (
	versionDetectionPattern = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:[-+._][A-Za-z0-9._-]+)?`)
	mcVersionPattern        = regexp.MustCompile(`\b1\.\d{1,2}(?:\.\d+)?\b`)
	loadingPaperPattern     = regexp.MustCompile(`(?i)Loading\s+Paper\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)
	forMinecraftPattern     = regexp.MustCompile(`(?i)for\s+Minecraft\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)
)

type ServerImportProperties struct {
	MaxPlayers *int   `json:"maxPlayers,omitempty"`
	Motd       string `json:"motd,omitempty"`
	WhiteList  *bool  `json:"whiteList,omitempty"`
	OnlineMode *bool  `json:"onlineMode,omitempty"`
}

type ServerImportAnalysisResult struct {
	AnalysisID   string                 `json:"analysisId"`
	ServerType   string                 `json:"serverType"`
	TypeDetected bool                   `json:"typeDetected"`
	Version      string                 `json:"version"`
	Worlds       []string               `json:"worlds"`
	Plugins      []string               `json:"plugins"`
	Mods         []string               `json:"mods"`
	Properties   ServerImportProperties `json:"properties"`
	ResolvedName string                 `json:"resolvedName"`
	ResolvedPort int                    `json:"resolvedPort"`
}

type ServerImportAnalysis struct {
	ID         string
	CreatedAt  time.Time
	WorkingDir string
	ExtractDir string
	RootDir    string
	Result     ServerImportAnalysisResult
}

type ServerImportCommitOptions struct {
	Name         *string
	Port         *int
	TypeOverride string
	Version      *string
	MaxPlayers   *int
	Motd         *string
	WhiteList    *bool
	OnlineMode   *bool
}

type ImportPortConflictError struct {
	RequestedPort int
	SuggestedPort int
}

func (e *ImportPortConflictError) Error() string {
	if e == nil {
		return "port conflict"
	}
	if e.SuggestedPort > 0 {
		return fmt.Sprintf("port %d is already in use (closest free: %d)", e.RequestedPort, e.SuggestedPort)
	}
	return fmt.Sprintf("port %d is already in use", e.RequestedPort)
}

type ImportInvalidVersionError struct {
	Message string
}

func (e *ImportInvalidVersionError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "Selected version is not valid for this server type."
	}
	return e.Message
}

func canonicalServerType(serverType string) string {
	switch strings.ToLower(strings.TrimSpace(serverType)) {
	case "vanilla":
		return "Vanilla"
	case "spigot":
		return "Spigot"
	case "paper":
		return "Paper"
	case "folia":
		return "Folia"
	case "purpur":
		return "Purpur"
	case "velocity":
		return "Velocity"
	case "forge":
		return "Forge"
	case "fabric":
		return "Fabric"
	case "neoforge":
		return "NeoForge"
	default:
		return ""
	}
}

func isModType(serverType string) bool {
	switch strings.ToLower(strings.TrimSpace(serverType)) {
	case "forge", "fabric", "neoforge":
		return true
	default:
		return false
	}
}

func stripImportArchiveExt(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return strings.TrimSuffix(base, base[len(base)-7:])
	case strings.HasSuffix(lower, ".tgz"):
		return strings.TrimSuffix(base, base[len(base)-4:])
	case strings.HasSuffix(lower, ".zip"):
		return strings.TrimSuffix(base, base[len(base)-4:])
	default:
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
}

func sanitizeArchiveEntryPath(name string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if normalized == "" {
		return "", nil
	}
	if strings.ContainsRune(normalized, '\x00') {
		return "", fmt.Errorf("archive contains invalid path")
	}
	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == "/" {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	return cleaned, nil
}

func writeArchiveFile(targetPath string, src io.Reader, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

func extractZipArchive(archivePath, destDir string) error {
	baseAbs, err := filepath.Abs(filepath.Clean(destDir))
	if err != nil {
		return err
	}
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		rel, err := sanitizeArchiveEntryPath(f.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		targetAbs, err := filepath.Abs(filepath.Clean(target))
		if err != nil {
			return err
		}
		if err := ensurePathWithinBase(baseAbs, targetAbs); err != nil {
			return fmt.Errorf("archive entry escapes extraction root")
		}

		mode := f.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("archive symlinks are not supported")
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetAbs, 0755); err != nil {
				return err
			}
			continue
		}

		in, err := f.Open()
		if err != nil {
			return err
		}
		fileMode := mode.Perm()
		if fileMode == 0 {
			fileMode = 0644
		}
		writeErr := writeArchiveFile(targetAbs, in, fileMode)
		closeErr := in.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractTarGzArchive(archivePath, destDir string) error {
	baseAbs, err := filepath.Abs(filepath.Clean(destDir))
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		rel, err := sanitizeArchiveEntryPath(header.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		targetAbs, err := filepath.Abs(filepath.Clean(target))
		if err != nil {
			return err
		}
		if err := ensurePathWithinBase(baseAbs, targetAbs); err != nil {
			return fmt.Errorf("archive entry escapes extraction root")
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetAbs, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			mode := fs.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0644
			}
			if err := writeArchiveFile(targetAbs, tr, mode); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive links are not supported")
		default:
			// Ignore other entry kinds.
		}
	}
	return nil
}

func listJarNames(dirPath string) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".jar.disabled") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func detectWorldDirectories(rootDir string) []string {
	worldMap := make(map[string]struct{})
	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.EqualFold(d.Name(), "level.dat") {
			return nil
		}
		parent := filepath.Dir(path)
		rel, relErr := filepath.Rel(rootDir, parent)
		if relErr != nil || rel == "." {
			return nil
		}
		worldMap[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	worlds := make([]string, 0, len(worldMap))
	for world := range worldMap {
		worlds = append(worlds, world)
	}
	sort.Strings(worlds)
	return worlds
}

func parseServerPropertiesFile(path string) map[string]string {
	file, err := os.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()

	props := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		props[strings.ToLower(key)] = value
	}
	return props
}

func parseBoolPtr(value string) *bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return nil
	}
	switch trimmed {
	case "true", "on", "yes", "1":
		v := true
		return &v
	case "false", "off", "no", "0":
		v := false
		return &v
	default:
		return nil
	}
}

func parseIntPtr(value string) *int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil
	}
	return &n
}

func parseVelocityToml(path string) (port *int, maxPlayers *int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "show-max-players") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				maxPlayers = parseIntPtr(strings.TrimSpace(parts[1]))
			}
			continue
		}
		if strings.HasPrefix(trimmed, "bind") {
			start := strings.Index(trimmed, "\"")
			if start < 0 {
				continue
			}
			rest := trimmed[start+1:]
			end := strings.Index(rest, "\"")
			if end < 0 {
				continue
			}
			rawBind := strings.TrimSpace(rest[:end])
			if host, p, err := netSplitHostPortBestEffort(rawBind); err == nil && host != "" {
				port = &p
			}
		}
	}
	return port, maxPlayers
}

func netSplitHostPortBestEffort(bind string) (string, int, error) {
	host, p, err := net.SplitHostPort(bind)
	if err == nil {
		port, convErr := strconv.Atoi(p)
		if convErr != nil {
			return "", 0, convErr
		}
		return host, port, nil
	}
	idx := strings.LastIndex(bind, ":")
	if idx <= 0 || idx >= len(bind)-1 {
		return "", 0, fmt.Errorf("invalid bind")
	}
	host = bind[:idx]
	port, convErr := strconv.Atoi(bind[idx+1:])
	if convErr != nil {
		return "", 0, convErr
	}
	return host, port, nil
}

func extractVersionFromText(value string) string {
	match := versionDetectionPattern.FindString(value)
	return strings.TrimSpace(match)
}

func extractMCVersionFromText(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if match := loadingPaperPattern.FindStringSubmatch(text); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	if match := forMinecraftPattern.FindStringSubmatch(text); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	if match := mcVersionPattern.FindString(text); match != "" {
		return strings.TrimSpace(match)
	}
	return ""
}

func readFileHead(path string, maxBytes int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	if maxBytes <= 0 {
		maxBytes = 512 * 1024
	}
	limited := io.LimitReader(file, maxBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return ""
	}
	return string(data)
}

func detectVersionFromLogs(rootDir string) string {
	for _, rel := range []string{
		filepath.Join("logs", "latest.log"),
		filepath.Join("logs", "debug.log"),
		filepath.Join("logs", "server.log"),
	} {
		snippet := readFileHead(filepath.Join(rootDir, rel), 768*1024)
		if snippet == "" {
			continue
		}
		if version := extractMCVersionFromText(snippet); version != "" {
			return version
		}
	}
	return ""
}

func detectVersionFromJarPaths(rootDir string) string {
	keywords := []string{"paper", "spigot", "purpur", "folia", "velocity", "server", "forge", "fabric", "neoforge"}
	scanned := 0
	found := ""
	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == "world" || strings.HasPrefix(name, "world_") || name == "region" || name == "playerdata" {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(d.Name())
		if !strings.HasSuffix(lower, ".jar") {
			return nil
		}
		scanned++
		if scanned > 500 {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			rel = d.Name()
		}
		relText := strings.ToLower(filepath.ToSlash(rel))
		for _, keyword := range keywords {
			if strings.Contains(relText, keyword) {
				if version := extractMCVersionFromText(relText); version != "" {
					found = version
					return filepath.SkipAll
				}
				if version := extractVersionFromText(relText); version != "" && mcVersionPattern.MatchString(version) {
					found = version
					return filepath.SkipAll
				}
				break
			}
		}
		return nil
	})
	return found
}

func detectVersion(rootDir string) string {
	if version := detectVersionFromLogs(rootDir); version != "" {
		return version
	}

	rootEntries, _ := os.ReadDir(rootDir)
	for _, entry := range rootEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".jar") {
			if version := extractMCVersionFromText(name); version != "" {
				return version
			}
		}
	}

	if version := detectVersionFromJarPaths(rootDir); version != "" {
		return version
	}

	runShPath := filepath.Join(rootDir, "run.sh")
	if data, err := os.ReadFile(runShPath); err == nil {
		if version := extractMCVersionFromText(string(data)); version != "" {
			return version
		}
	}

	return ""
}

func hasAnyFile(rootDir string, names ...string) bool {
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(rootDir, name)); err == nil {
			return true
		}
	}
	return false
}

func hasPaperConfig(rootDir string) bool {
	return hasAnyFile(
		rootDir,
		"paper.yml",
		"paper-global.yml",
		"paper-world-defaults.yml",
		filepath.Join("config", "paper-global.yml"),
		filepath.Join("config", "paper-world-defaults.yml"),
		filepath.Join("config", "paper-world.yml"),
	)
}

func hasZipEntry(jarPath string, entries ...string) bool {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return false
	}
	defer reader.Close()
	lookup := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		lookup[entry] = struct{}{}
	}
	for _, file := range reader.File {
		if _, ok := lookup[file.Name]; ok {
			return true
		}
	}
	return false
}

func detectServerType(rootDir string, plugins, mods []string) (string, bool) {
	velocityPath := filepath.Join(rootDir, "velocity.toml")
	if _, err := os.Stat(velocityPath); err == nil {
		return "Velocity", true
	}

	if len(mods) > 0 {
		if hasAnyFile(rootDir, "fabric-server-launch.jar", ".fabric") {
			return "Fabric", true
		}
		modsDir := filepath.Join(rootDir, "mods")
		for _, mod := range mods {
			modPath := filepath.Join(modsDir, mod)
			if hasZipEntry(modPath, "META-INF/neoforge.mods.toml") {
				return "NeoForge", true
			}
		}
		for _, mod := range mods {
			modPath := filepath.Join(modsDir, mod)
			if hasZipEntry(modPath, "fabric.mod.json") {
				return "Fabric", true
			}
		}
		for _, mod := range mods {
			modPath := filepath.Join(modsDir, mod)
			if hasZipEntry(modPath, "META-INF/mods.toml") {
				return "Forge", true
			}
		}
		runShPath := filepath.Join(rootDir, "run.sh")
		if data, err := os.ReadFile(runShPath); err == nil {
			lower := strings.ToLower(string(data))
			if strings.Contains(lower, "neoforge") {
				return "NeoForge", true
			}
			if strings.Contains(lower, "forge") {
				return "Forge", true
			}
		}
		return "", false
	}

	if len(plugins) > 0 {
		switch {
		case hasAnyFile(rootDir, "purpur.yml"):
			return "Purpur", true
		case hasPaperConfig(rootDir):
			return "Paper", true
		case hasAnyFile(rootDir, "spigot.yml", "bukkit.yml"):
			return "Spigot", true
		default:
			return "Paper", true
		}
	}

	rootEntries, _ := os.ReadDir(rootDir)
	for _, entry := range rootEntries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			continue
		}
		lower := strings.ToLower(entry.Name())
		switch {
		case strings.Contains(lower, "velocity"):
			return "Velocity", true
		case strings.Contains(lower, "purpur"):
			return "Purpur", true
		case strings.Contains(lower, "folia"):
			return "Folia", true
		case strings.Contains(lower, "paper"):
			return "Paper", true
		case strings.Contains(lower, "spigot"):
			return "Spigot", true
		case strings.Contains(lower, "neoforge"):
			return "NeoForge", true
		case strings.Contains(lower, "fabric"):
			return "Fabric", true
		case strings.Contains(lower, "forge"):
			return "Forge", true
		}
	}

	if _, err := os.Stat(filepath.Join(rootDir, "server.properties")); err == nil {
		return "Vanilla", true
	}
	return "", false
}

func normalizeExtractedImportRoot(extractDir string) (string, string, error) {
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", "", err
	}
	filtered := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "__MACOSX" {
			continue
		}
		filtered = append(filtered, entry)
	}
	if len(filtered) == 1 && filtered[0].IsDir() {
		name := filtered[0].Name()
		return filepath.Join(extractDir, name), name, nil
	}
	return extractDir, "", nil
}

func parseImportPort(serverProperties map[string]string, velocityPath string) int {
	if port := parseIntPtr(serverProperties["server-port"]); port != nil {
		return *port
	}
	if vPort, _ := parseVelocityToml(velocityPath); vPort != nil {
		return *vPort
	}
	return 25565
}

func (m *Manager) resolveImportedServerNameLocked(baseName string) string {
	candidate := strings.TrimSpace(baseName)
	if candidate == "" {
		candidate = "Imported Server"
	}
	used := make(map[string]struct{}, len(m.configs))
	for _, cfg := range m.configs {
		used[strings.ToLower(strings.TrimSpace(cfg.Name))] = struct{}{}
	}
	if _, exists := used[strings.ToLower(candidate)]; !exists {
		return candidate
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%d", candidate, i)
		if _, exists := used[strings.ToLower(next)]; !exists {
			return next
		}
	}
}

func (m *Manager) resolveImportedPortLocked(startPort int) (int, error) {
	port := startPort
	if port < 1024 || port > 65535 {
		port = 25565
	}
	used := make(map[int]struct{}, len(m.configs))
	for _, cfg := range m.configs {
		used[cfg.Port] = struct{}{}
	}
	for p := port; p <= 65535; p++ {
		if _, exists := used[p]; !exists {
			return p, nil
		}
	}
	for p := 1024; p < port; p++ {
		if _, exists := used[p]; !exists {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no available server ports")
}

func (m *Manager) nearestAvailablePortLocked(aroundPort int) (int, error) {
	port := aroundPort
	if port < 1024 || port > 65535 {
		port = 25565
	}
	used := make(map[int]struct{}, len(m.configs))
	for _, cfg := range m.configs {
		used[cfg.Port] = struct{}{}
	}
	if _, exists := used[port]; !exists {
		return port, nil
	}
	for offset := 1; offset <= 65535; offset++ {
		high := port + offset
		if high <= 65535 {
			if _, exists := used[high]; !exists {
				return high, nil
			}
		}
		low := port - offset
		if low >= 1024 {
			if _, exists := used[low]; !exists {
				return low, nil
			}
		}
	}
	return 0, fmt.Errorf("no available server ports")
}

func containsVersion(versions []VersionInfo, target string) bool {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return false
	}
	for _, item := range versions {
		if strings.EqualFold(strings.TrimSpace(item.Version), trimmed) {
			return true
		}
	}
	return false
}

func chooseImportedJarFile(serverDir string, serverType string) string {
	if _, err := os.Stat(filepath.Join(serverDir, "server.jar")); err == nil {
		return "server.jar"
	}
	entries, err := os.ReadDir(serverDir)
	if err != nil {
		return "server.jar"
	}
	jars := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			jars = append(jars, entry.Name())
		}
	}
	if len(jars) == 0 {
		return "server.jar"
	}
	sort.Strings(jars)
	lowerType := strings.ToLower(serverType)
	for _, jar := range jars {
		lower := strings.ToLower(jar)
		if strings.Contains(lower, lowerType) || strings.Contains(lower, "server") {
			return jar
		}
	}
	return jars[0]
}

func detectImportedStartCommand(serverDir string, serverType string) []string {
	if !strings.EqualFold(serverType, "forge") && !strings.EqualFold(serverType, "neoforge") {
		return nil
	}
	runSh := filepath.Join(serverDir, "run.sh")
	if _, err := os.Stat(runSh); err != nil {
		return nil
	}
	_ = os.Chmod(runSh, 0755)
	return []string{"bash", "run.sh", "nogui"}
}

func toRAMMBString(value string, fallbackMB int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Sprintf("%dM", fallbackMB)
	}
	gb, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || gb <= 0 {
		return fmt.Sprintf("%dM", fallbackMB)
	}
	return fmt.Sprintf("%dM", int(gb*1024))
}

func boolToString(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "true"
	}
	return "false"
}

func applyJavaImportPropertyOverrides(path string, maxPlayers int, port int, motd *string, whiteList *bool, onlineMode *bool) error {
	lines := []string{}
	data, err := os.ReadFile(path)
	if err == nil {
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		lines = strings.Split(content, "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	setKey := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		prefix := key + "="
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, prefix) {
				lines[i] = fmt.Sprintf("%s=%s", key, value)
				return
			}
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	setKey("max-players", strconv.Itoa(maxPlayers))
	setKey("server-port", strconv.Itoa(port))
	if motd != nil {
		setKey("motd", strings.TrimSpace(*motd))
	}
	if whiteList != nil {
		setKey("white-list", boolToString(whiteList))
	}
	if onlineMode != nil {
		setKey("online-mode", boolToString(onlineMode))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func isCrossDeviceErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "cross-device link")
}

func copyDirectory(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return os.MkdirAll(dstDir, 0755)
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported")
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		src, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			_ = src.Close()
			return err
		}
		dst, createErr := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if createErr != nil {
			_ = src.Close()
			return createErr
		}
		_, copyErr := io.Copy(dst, src)
		srcCloseErr := src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if srcCloseErr != nil {
			return srcCloseErr
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	})
}

func moveDirectory(srcDir, dstDir string) error {
	if err := os.Rename(srcDir, dstDir); err == nil {
		return nil
	} else if !isCrossDeviceErr(err) {
		return err
	}
	if err := copyDirectory(srcDir, dstDir); err != nil {
		return err
	}
	return os.RemoveAll(srcDir)
}

func (m *Manager) runImportAnalysisCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanupExpiredImportAnalyses()
		case <-m.stopImportCleanup:
			return
		}
	}
}

func (m *Manager) cleanupExpiredImportAnalyses() {
	now := time.Now()
	stale := make([]*ServerImportAnalysis, 0)

	m.mu.Lock()
	for id, analysis := range m.importAnalyses {
		if now.Sub(analysis.CreatedAt) > importAnalysisTTL {
			stale = append(stale, analysis)
			delete(m.importAnalyses, id)
		}
	}
	m.mu.Unlock()

	for _, analysis := range stale {
		_ = os.RemoveAll(analysis.WorkingDir)
	}
}

func (m *Manager) AnalyzeServerImportArchive(fileName string, src io.Reader) (*ServerImportAnalysisResult, error) {
	if src == nil {
		return nil, fmt.Errorf("no archive data provided")
	}
	trimmedName := strings.TrimSpace(fileName)
	lowerName := strings.ToLower(trimmedName)
	archiveType := ""
	switch {
	case strings.HasSuffix(lowerName, ".zip"):
		archiveType = "zip"
	case strings.HasSuffix(lowerName, ".tar.gz"), strings.HasSuffix(lowerName, ".tgz"):
		archiveType = "tar.gz"
	default:
		return nil, fmt.Errorf("unsupported archive format, use .zip or .tar.gz")
	}

	analysisID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	workingDir := filepath.Join(m.importsRoot, "analysis-"+analysisID)
	extractDir := filepath.Join(workingDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare import workspace: %w", err)
	}
	archivePath := filepath.Join(workingDir, "upload")
	if archiveType == "zip" {
		archivePath += ".zip"
	} else {
		archivePath += ".tar.gz"
	}
	fail := true
	defer func() {
		if fail {
			_ = os.RemoveAll(workingDir)
		}
	}()

	archiveFile, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary archive file: %w", err)
	}
	if _, err := io.Copy(archiveFile, src); err != nil {
		_ = archiveFile.Close()
		return nil, fmt.Errorf("failed to store uploaded archive: %w", err)
	}
	if err := archiveFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize uploaded archive: %w", err)
	}

	switch archiveType {
	case "zip":
		if err := extractZipArchive(archivePath, extractDir); err != nil {
			return nil, fmt.Errorf("failed to extract zip archive: %w", err)
		}
	default:
		if err := extractTarGzArchive(archivePath, extractDir); err != nil {
			return nil, fmt.Errorf("failed to extract tar.gz archive: %w", err)
		}
	}

	rootDir, topLevelName, err := normalizeExtractedImportRoot(extractDir)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect extracted archive: %w", err)
	}
	if _, err := os.Stat(rootDir); err != nil {
		return nil, fmt.Errorf("archive appears empty")
	}

	plugins := listJarNames(filepath.Join(rootDir, "plugins"))
	mods := listJarNames(filepath.Join(rootDir, "mods"))
	worlds := detectWorldDirectories(rootDir)
	serverProps := parseServerPropertiesFile(filepath.Join(rootDir, "server.properties"))
	velocityPath := filepath.Join(rootDir, "velocity.toml")

	serverType, typeDetected := detectServerType(rootDir, plugins, mods)
	version := detectVersion(rootDir)

	props := ServerImportProperties{
		MaxPlayers: parseIntPtr(serverProps["max-players"]),
		Motd:       strings.TrimSpace(serverProps["motd"]),
		WhiteList:  parseBoolPtr(serverProps["white-list"]),
		OnlineMode: parseBoolPtr(serverProps["online-mode"]),
	}

	velocityPort, velocityMaxPlayers := parseVelocityToml(velocityPath)
	if props.MaxPlayers == nil {
		props.MaxPlayers = velocityMaxPlayers
	}
	detectedPort := parseImportPort(serverProps, velocityPath)
	if velocityPort != nil {
		detectedPort = *velocityPort
	}

	candidateName := strings.TrimSpace(stripImportArchiveExt(trimmedName))
	if candidateName == "" {
		candidateName = strings.TrimSpace(topLevelName)
	}
	if candidateName == "" {
		candidateName = "Imported Server"
	}

	m.mu.Lock()
	resolvedName := m.resolveImportedServerNameLocked(candidateName)
	resolvedPort, err := m.resolveImportedPortLocked(detectedPort)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	result := ServerImportAnalysisResult{
		AnalysisID:   analysisID,
		ServerType:   serverType,
		TypeDetected: typeDetected,
		Version:      version,
		Worlds:       worlds,
		Plugins:      plugins,
		Mods:         mods,
		Properties:   props,
		ResolvedName: resolvedName,
		ResolvedPort: resolvedPort,
	}

	m.importAnalyses[analysisID] = &ServerImportAnalysis{
		ID:         analysisID,
		CreatedAt:  time.Now(),
		WorkingDir: workingDir,
		ExtractDir: extractDir,
		RootDir:    rootDir,
		Result:     result,
	}
	m.mu.Unlock()

	fail = false
	return &result, nil
}

func (m *Manager) CancelServerImportAnalysis(analysisID string) error {
	id := strings.TrimSpace(analysisID)
	if id == "" {
		return fmt.Errorf("analysisId is required")
	}
	var workingDir string
	m.mu.Lock()
	analysis, ok := m.importAnalyses[id]
	if ok {
		workingDir = analysis.WorkingDir
		delete(m.importAnalyses, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("import analysis not found")
	}
	return os.RemoveAll(workingDir)
}

func (m *Manager) CommitServerImport(analysisID string, opts ServerImportCommitOptions) (*ServerInfo, error) {
	id := strings.TrimSpace(analysisID)
	if id == "" {
		return nil, fmt.Errorf("analysisId is required")
	}

	var cleanupDir string
	m.mu.Lock()
	analysis, ok := m.importAnalyses[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("import analysis not found")
	}
	if time.Since(analysis.CreatedAt) > importAnalysisTTL {
		delete(m.importAnalyses, id)
		m.mu.Unlock()
		_ = os.RemoveAll(analysis.WorkingDir)
		return nil, fmt.Errorf("import analysis expired, upload the archive again")
	}

	serverType := canonicalServerType(opts.TypeOverride)
	if serverType == "" {
		serverType = strings.TrimSpace(analysis.Result.ServerType)
		if !analysis.Result.TypeDetected {
			m.mu.Unlock()
			return nil, fmt.Errorf("server type is required")
		}
		serverType = canonicalServerType(serverType)
		if serverType == "" {
			m.mu.Unlock()
			return nil, fmt.Errorf("detected server type is not supported")
		}
	}

	if _, err := GetProvider(serverType); err != nil {
		m.mu.Unlock()
		return nil, err
	}

	detectedVersion := strings.TrimSpace(analysis.Result.Version)
	manualVersion := ""
	if opts.Version != nil {
		manualVersion = strings.TrimSpace(*opts.Version)
	}
	if detectedVersion != "" {
		if manualVersion != "" && !strings.EqualFold(manualVersion, detectedVersion) {
			m.mu.Unlock()
			return nil, &ImportInvalidVersionError{Message: "Detected version is locked and cannot be changed before import."}
		}
	} else {
		if manualVersion == "" {
			m.mu.Unlock()
			return nil, &ImportInvalidVersionError{Message: "Select a valid server version before importing."}
		}
		versions, err := m.GetVersions(serverType)
		if err != nil {
			m.mu.Unlock()
			return nil, &ImportInvalidVersionError{Message: "Unable to validate the selected version right now."}
		}
		if !containsVersion(versions, manualVersion) {
			m.mu.Unlock()
			return nil, &ImportInvalidVersionError{Message: "Selected version is not available for this server type."}
		}
	}

	baseName := analysis.Result.ResolvedName
	if opts.Name != nil {
		trimmedName := strings.TrimSpace(*opts.Name)
		if trimmedName == "" {
			m.mu.Unlock()
			return nil, fmt.Errorf("server name cannot be empty")
		}
		baseName = trimmedName
	}
	resolvedName := m.resolveImportedServerNameLocked(baseName)

	resolvedPort := analysis.Result.ResolvedPort
	if opts.Port != nil {
		requestedPort := *opts.Port
		if requestedPort < 1024 || requestedPort > 65535 {
			m.mu.Unlock()
			return nil, fmt.Errorf("port must be between 1024 and 65535")
		}
		inUse := false
		for _, cfg := range m.configs {
			if cfg.Port == requestedPort {
				inUse = true
				break
			}
		}
		if inUse {
			suggestedPort, suggestErr := m.nearestAvailablePortLocked(requestedPort)
			if suggestErr != nil {
				m.mu.Unlock()
				return nil, &ImportPortConflictError{RequestedPort: requestedPort}
			}
			m.mu.Unlock()
			return nil, &ImportPortConflictError{
				RequestedPort: requestedPort,
				SuggestedPort: suggestedPort,
			}
		}
		resolvedPort = requestedPort
	} else {
		autoPort, portErr := m.resolveImportedPortLocked(resolvedPort)
		if portErr != nil {
			m.mu.Unlock()
			return nil, portErr
		}
		resolvedPort = autoPort
	}

	importsAbs, err := filepath.Abs(filepath.Clean(m.importsRoot))
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to resolve imports root: %w", err)
	}
	rootAbs, err := filepath.Abs(filepath.Clean(analysis.RootDir))
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to resolve import root: %w", err)
	}
	if err := ensurePathWithinBase(importsAbs, rootAbs); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid import root path")
	}

	newID := uuid.New().String()[:8]
	dirName := sanitizeName(resolvedName)
	serverDir := filepath.Join(m.serversRoot, dirName)
	if _, err := os.Stat(serverDir); err == nil {
		serverDir = filepath.Join(m.serversRoot, fmt.Sprintf("%s_%s", dirName, newID))
	}
	serverDir = filepath.Clean(serverDir)
	if err := m.validateManagedServerDir(serverDir); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid target server directory: %w", err)
	}
	if _, exists := m.configs[newID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("generated server id conflict, retry import")
	}

	cleanupDir = analysis.WorkingDir
	delete(m.importAnalyses, id)
	m.mu.Unlock()

	if err := moveDirectory(rootAbs, serverDir); err != nil {
		// Restore staged entry so user can retry commit after fixing.
		m.mu.Lock()
		m.importAnalyses[id] = analysis
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to move imported server files: %w", err)
	}

	m.settingsMu.RLock()
	minRAM := toRAMMBString(m.settings.DefaultMinRAM, 512)
	maxRAM := toRAMMBString(m.settings.DefaultMaxRAM, 1024)
	defaultFlags := strings.TrimSpace(m.settings.DefaultFlags)
	m.settingsMu.RUnlock()
	if defaultFlags == "" {
		defaultFlags = "none"
	}

	maxPlayers := 20
	switch {
	case opts.MaxPlayers != nil && *opts.MaxPlayers > 0:
		maxPlayers = *opts.MaxPlayers
	case analysis.Result.Properties.MaxPlayers != nil && *analysis.Result.Properties.MaxPlayers > 0:
		maxPlayers = *analysis.Result.Properties.MaxPlayers
	}

	version := strings.TrimSpace(analysis.Result.Version)
	if version == "" && opts.Version != nil {
		version = strings.TrimSpace(*opts.Version)
	}
	if version == "" {
		version = "Unknown"
	}

	motd := opts.Motd
	if motd == nil && strings.TrimSpace(analysis.Result.Properties.Motd) != "" {
		value := analysis.Result.Properties.Motd
		motd = &value
	}
	whiteList := opts.WhiteList
	if whiteList == nil && analysis.Result.Properties.WhiteList != nil {
		value := *analysis.Result.Properties.WhiteList
		whiteList = &value
	}
	onlineMode := opts.OnlineMode
	if onlineMode == nil && analysis.Result.Properties.OnlineMode != nil {
		value := *analysis.Result.Properties.OnlineMode
		onlineMode = &value
	}

	if strings.EqualFold(serverType, "velocity") {
		velocityPath := filepath.Join(serverDir, "velocity.toml")
		if _, statErr := os.Stat(velocityPath); statErr == nil {
			if err := updateVelocityToml(velocityPath, maxPlayers, resolvedPort); err != nil {
				return nil, fmt.Errorf("failed to update velocity.toml: %w", err)
			}
		}
	} else {
		propsPath := filepath.Join(serverDir, "server.properties")
		if err := applyJavaImportPropertyOverrides(propsPath, maxPlayers, resolvedPort, motd, whiteList, onlineMode); err != nil {
			return nil, fmt.Errorf("failed to update server.properties: %w", err)
		}
	}

	jarFile := chooseImportedJarFile(serverDir, serverType)
	startCommand := detectImportedStartCommand(serverDir, serverType)

	cfg := &ServerConfig{
		ID:             newID,
		Name:           resolvedName,
		Type:           serverType,
		Version:        version,
		Port:           resolvedPort,
		JarFile:        jarFile,
		MaxRAM:         maxRAM,
		MinRAM:         minRAM,
		MaxPlayers:     maxPlayers,
		Dir:            serverDir,
		StartCommand:   startCommand,
		AutoStart:      false,
		Flags:          defaultFlags,
		AlwaysPreTouch: false,
	}

	m.mu.Lock()
	m.configs[newID] = cfg
	m.running[newID] = &runningServer{
		status:      "Stopped",
		logBuffer:   make([]ConsoleLogEntry, 0),
		nextLogSeq:  1,
		players:     make(map[string]*onlinePlayer),
		pingBlocked: make(map[string]bool),
	}
	if err := m.persist(); err != nil {
		delete(m.configs, newID)
		delete(m.running, newID)
		m.mu.Unlock()
		return nil, err
	}
	info := m.serverInfo(newID)
	m.mu.Unlock()

	_ = os.RemoveAll(cleanupDir)
	return info, nil
}
