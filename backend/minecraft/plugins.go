package minecraft

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// extractPluginVersion opens a JAR (ZIP) and reads plugin metadata
// Returns (name, version). Either may be empty if not found.
func extractPluginVersion(jarPath string) (string, string) {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return "", ""
	}
	defer r.Close()

	for _, f := range r.File {
		switch f.Name {
		case "plugin.yml", "bungee.yml":
			name, version := parsePluginYML(f)
			if version != "" {
				return name, version
			}
		case "fabric.mod.json":
			name, version := parseFabricModJSON(f)
			if version != "" {
				return name, version
			}
		}
	}

	// Fallback: try META-INF/mods.toml for Forge/NeoForge
	for _, f := range r.File {
		if f.Name == "META-INF/mods.toml" {
			name, version := parseModsToml(f)
			if version != "" {
				return name, version
			}
		}
	}

	return "", ""
}

func parsePluginYML(f *zip.File) (string, string) {
	rc, err := f.Open()
	if err != nil {
		return "", ""
	}
	defer rc.Close()

	var data struct {
		Name    string      `yaml:"name"`
		Version interface{} `yaml:"version"`
	}
	if err := yaml.NewDecoder(rc).Decode(&data); err != nil {
		return "", ""
	}
	return data.Name, fmt.Sprintf("%v", data.Version)
}

func parseFabricModJSON(f *zip.File) (string, string) {
	rc, err := f.Open()
	if err != nil {
		return "", ""
	}
	defer rc.Close()

	var data struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(rc).Decode(&data); err != nil {
		return "", ""
	}
	return data.Name, data.Version
}

func parseModsToml(f *zip.File) (string, string) {
	rc, err := f.Open()
	if err != nil {
		return "", ""
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return "", ""
	}
	content := string(raw)

	var name, version string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "displayName") {
			if v := extractTomlValue(line); v != "" {
				name = v
			}
		}
		if strings.HasPrefix(line, "version") && !strings.HasPrefix(line, "versionRange") {
			if v := extractTomlValue(line); v != "" && v != "${file.jarVersion}" {
				version = v
			}
		}
	}
	return name, version
}

func extractTomlValue(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	val := strings.TrimSpace(parts[1])
	val = strings.Trim(val, "\"'")
	return val
}

// normalizeVersion strips common prefixes/suffixes and lowercases for comparison
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ToLower(v)
	v = strings.TrimPrefix(v, "v")
	// Remove common suffixes like -SNAPSHOT, -beta, etc. for core comparison
	return v
}

// versionsMatch checks if two version strings represent the same version
func versionsMatch(current, latest string) bool {
	c := normalizeVersion(current)
	l := normalizeVersion(latest)
	if c == l {
		return true
	}
	// Check if one is a prefix of the other followed by non-numeric chars
	// e.g., "1.2.3" matches "1.2.3-SNAPSHOT" but "1.0" does NOT match "1.0.1"
	if strings.HasPrefix(l, c) {
		rest := l[len(c):]
		// Only match if the remaining part starts with a non-digit separator
		// This prevents "1.0" matching "1.0.1" but allows "1.0" matching "1.0-beta"
		if len(rest) > 0 && rest[0] != '.' && (rest[0] < '0' || rest[0] > '9') {
			return true
		}
	}
	if strings.HasPrefix(c, l) {
		rest := c[len(l):]
		if len(rest) > 0 && rest[0] != '.' && (rest[0] < '0' || rest[0] > '9') {
			return true
		}
	}
	return false
}

// ============================================================
// Plugin Update Checking (Modrinth + Spiget APIs)
// ============================================================

// PluginUpdateInfo holds version check results for a single plugin
type PluginUpdateInfo struct {
	Name          string `json:"name"`
	FileName      string `json:"fileName"`
	Version       string `json:"version"`
	LatestVersion string `json:"latestVersion,omitempty"`
	VersionStatus string `json:"versionStatus"` // latest, outdated, incompatible, unknown
	UpdateURL     string `json:"updateUrl,omitempty"`
}

// pluginUpdateCache caches update check results
var pluginUpdateCache = struct {
	mu      sync.RWMutex
	entries map[string]pluginUpdateCacheEntry
}{
	entries: make(map[string]pluginUpdateCacheEntry),
}

type pluginUpdateCacheEntry struct {
	result    *PluginUpdateInfo
	fetchedAt time.Time
}

const pluginCacheTTL = 15 * time.Minute

// CheckPluginUpdates checks all plugins for a server against Modrinth/Spiget APIs
func (m *Manager) CheckPluginUpdates(id string) ([]PluginUpdateInfo, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	plugins, err := m.ListPlugins(id)
	if err != nil {
		return nil, err
	}

	mcVersion := cfg.Version
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := make([]PluginUpdateInfo, len(plugins))
	var wg sync.WaitGroup

	for i, plugin := range plugins {
		wg.Add(1)
		go func(idx int, p PluginInfo) {
			defer wg.Done()

			cacheKey := fmt.Sprintf("%s:%s:%s", id, p.FileName, p.Version)
			pluginUpdateCache.mu.RLock()
			cached, ok := pluginUpdateCache.entries[cacheKey]
			pluginUpdateCache.mu.RUnlock()
			if ok && time.Since(cached.fetchedAt) < pluginCacheTTL {
				results[idx] = *cached.result
				return
			}

			info := checkSinglePlugin(ctx, p, mcVersion)
			results[idx] = info

			pluginUpdateCache.mu.Lock()
			pluginUpdateCache.entries[cacheKey] = pluginUpdateCacheEntry{
				result:    &info,
				fetchedAt: time.Now(),
			}
			pluginUpdateCache.mu.Unlock()
		}(i, plugin)
	}
	wg.Wait()

	return results, nil
}

func checkSinglePlugin(ctx context.Context, plugin PluginInfo, mcVersion string) PluginUpdateInfo {
	info := PluginUpdateInfo{
		Name:          plugin.Name,
		FileName:      plugin.FileName,
		Version:       plugin.Version,
		VersionStatus: "unknown",
	}

	if plugin.Version == "" {
		return info
	}

	// Try Modrinth first
	if result := checkModrinth(ctx, plugin.Name, plugin.Version, mcVersion); result != nil {
		result.FileName = plugin.FileName
		return *result
	}

	// Fallback to Spiget
	if result := checkSpiget(ctx, plugin.Name, plugin.Version); result != nil {
		result.FileName = plugin.FileName
		return *result
	}

	return info
}

// Modrinth API types
type modrinthSearchResult struct {
	Hits []struct {
		ProjectID string `json:"project_id"`
		Slug      string `json:"slug"`
		Title     string `json:"title"`
	} `json:"hits"`
}

type modrinthVersion struct {
	VersionNumber string   `json:"version_number"`
	GameVersions  []string `json:"game_versions"`
	Files         []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
	} `json:"files"`
}

func checkModrinth(ctx context.Context, pluginName, currentVersion, mcVersion string) *PluginUpdateInfo {
	// Search for the plugin on Modrinth
	searchURL := fmt.Sprintf("https://api.modrinth.com/v2/search?query=%s&limit=5", url.QueryEscape(pluginName))

	var searchResult modrinthSearchResult
	if err := fetchJSON(ctx, searchURL, &searchResult); err != nil {
		return nil
	}

	if len(searchResult.Hits) == 0 {
		return nil
	}

	// Find the best matching project
	var projectID string
	for _, hit := range searchResult.Hits {
		if strings.EqualFold(hit.Title, pluginName) || strings.EqualFold(hit.Slug, strings.ToLower(strings.ReplaceAll(pluginName, " ", "-"))) {
			projectID = hit.ProjectID
			break
		}
	}
	if projectID == "" {
		projectID = searchResult.Hits[0].ProjectID
	}

	// Get versions for the project
	versionsURL := fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version", projectID)
	var versions []modrinthVersion
	if err := fetchJSON(ctx, versionsURL, &versions); err != nil {
		return nil
	}

	if len(versions) == 0 {
		return nil
	}

	// Find latest compatible version
	var latestCompatible *modrinthVersion
	var latestAny *modrinthVersion
	for i := range versions {
		v := &versions[i]
		if latestAny == nil {
			latestAny = v
		}
		for _, gv := range v.GameVersions {
			if gv == mcVersion {
				if latestCompatible == nil {
					latestCompatible = v
				}
				break
			}
		}
	}

	info := &PluginUpdateInfo{
		Name:    pluginName,
		Version: currentVersion,
	}

	if latestCompatible != nil {
		info.LatestVersion = latestCompatible.VersionNumber
		if versionsMatch(currentVersion, latestCompatible.VersionNumber) {
			info.VersionStatus = "latest"
		} else {
			info.VersionStatus = "outdated"
			for _, f := range latestCompatible.Files {
				if f.Primary || len(latestCompatible.Files) == 1 {
					info.UpdateURL = f.URL
					break
				}
			}
		}
	} else if latestAny != nil {
		info.LatestVersion = latestAny.VersionNumber
		info.VersionStatus = "incompatible"
	}

	return info
}

// Spiget API types
type spigetSearchResult []struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type spigetVersionResult []struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func checkSpiget(ctx context.Context, pluginName, currentVersion string) *PluginUpdateInfo {
	searchURL := fmt.Sprintf("https://api.spiget.org/v2/search/resources/%s?field=name&size=5", url.QueryEscape(pluginName))

	var searchResult spigetSearchResult
	if err := fetchJSON(ctx, searchURL, &searchResult); err != nil {
		return nil
	}

	if len(searchResult) == 0 {
		return nil
	}

	// Find best match
	resourceID := searchResult[0].ID
	for _, r := range searchResult {
		if strings.EqualFold(r.Name, pluginName) {
			resourceID = r.ID
			break
		}
	}

	// Get versions
	versionsURL := fmt.Sprintf("https://api.spiget.org/v2/resources/%d/versions?sort=-id&size=1", resourceID)
	var versions spigetVersionResult
	if err := fetchJSON(ctx, versionsURL, &versions); err != nil {
		return nil
	}

	if len(versions) == 0 {
		return nil
	}

	info := &PluginUpdateInfo{
		Name:          pluginName,
		Version:       currentVersion,
		LatestVersion: versions[0].Name,
	}

	if versionsMatch(currentVersion, versions[0].Name) {
		info.VersionStatus = "latest"
	} else {
		info.VersionStatus = "outdated"
		info.UpdateURL = fmt.Sprintf("https://api.spiget.org/v2/resources/%d/download", resourceID)
	}

	return info
}

// UpdatePlugin downloads a new version of a plugin from a URL and replaces the old JAR
func (m *Manager) UpdatePlugin(id, fileName, downloadURL string) (*PluginInfo, error) {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	pDir := extensionsDir(cfg)
	jarPath := filepath.Join(pDir, filepath.Base(fileName))

	// Verify the plugin file exists
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin file not found: %s", fileName)
	}

	// Download the new JAR to a temp file
	tmpPath := jarPath + ".update"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent())
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to save update: %w", err)
	}
	tmpFile.Close()

	// Backup old JAR
	backupPath := jarPath + ".bak"
	if err := os.Rename(jarPath, backupPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to backup old plugin: %w", err)
	}

	// Move new JAR into place
	if err := os.Rename(tmpPath, jarPath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, jarPath)
		return nil, fmt.Errorf("failed to install update: %w", err)
	}

	// Clean up backup
	os.Remove(backupPath)

	// Invalidate cache for this plugin
	pluginUpdateCache.mu.Lock()
	for key := range pluginUpdateCache.entries {
		if strings.Contains(key, fileName) {
			delete(pluginUpdateCache.entries, key)
		}
	}
	pluginUpdateCache.mu.Unlock()

	log.Printf("Updated plugin %s for server %s", fileName, id)

	// Return updated plugin info
	info, _ := os.Stat(jarPath)
	pName, pVersion := extractPluginVersion(jarPath)
	if pName == "" {
		pName = strings.TrimSuffix(fileName, ".jar")
	}

	return &PluginInfo{
		Name:     pName,
		FileName: fileName,
		Size:     formatFileSize(info.Size()),
		Enabled:  true,
		Version:  pVersion,
	}, nil
}
