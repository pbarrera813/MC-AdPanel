package minecraft

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	SourceURL     string `json:"sourceUrl,omitempty"`
}

func debugPluginUpdatesEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ADPANEL_DEBUG_PLUGIN_UPDATES")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
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
	serverType := cfg.Type
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := make([]PluginUpdateInfo, len(plugins))
	var wg sync.WaitGroup

	for i, plugin := range plugins {
		wg.Add(1)
		go func(idx int, p PluginInfo) {
			defer wg.Done()

			cacheKey := fmt.Sprintf(
				"%s:%s:%s:%s:%s:%s",
				id,
				p.FileName,
				p.Version,
				strings.ToLower(strings.TrimSpace(p.SourceURL)),
				strings.ToLower(strings.TrimSpace(serverType)),
				strings.TrimSpace(mcVersion),
			)
			pluginUpdateCache.mu.RLock()
			cached, ok := pluginUpdateCache.entries[cacheKey]
			pluginUpdateCache.mu.RUnlock()
			if ok && time.Since(cached.fetchedAt) < pluginCacheTTL {
				results[idx] = *cached.result
				return
			}

			info := checkSinglePlugin(ctx, p, mcVersion, serverType)
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

func checkSinglePlugin(ctx context.Context, plugin PluginInfo, mcVersion, serverType string) PluginUpdateInfo {
	info := PluginUpdateInfo{
		Name:          plugin.Name,
		FileName:      plugin.FileName,
		Version:       plugin.Version,
		VersionStatus: "unknown",
		SourceURL:     plugin.SourceURL,
	}

	if plugin.Version == "" {
		return info
	}

	if strings.TrimSpace(plugin.SourceURL) != "" {
		if result, handled := checkBySourceURL(ctx, plugin.SourceURL, plugin.Name, plugin.Version, mcVersion, serverType); handled {
			if result != nil {
				result.FileName = plugin.FileName
				result.SourceURL = plugin.SourceURL
				return *result
			}
			return info
		}
	}

	if isModdedType(serverType) {
		// Modded servers: prioritize Modrinth.
		if result := checkModrinth(ctx, plugin.Name, plugin.Version, mcVersion, serverType); result != nil {
			result.FileName = plugin.FileName
			return *result
		}
		return info
	}

	// Plugin/proxy servers: check Spiget first, then Modrinth if no update is found there.
	spigetResult := checkSpiget(ctx, plugin.Name, plugin.Version, mcVersion)
	if spigetResult != nil && spigetResult.VersionStatus == "outdated" {
		spigetResult.FileName = plugin.FileName
		return *spigetResult
	}

	modrinthResult := checkModrinth(ctx, plugin.Name, plugin.Version, mcVersion, serverType)
	if modrinthResult != nil && modrinthResult.VersionStatus == "outdated" {
		modrinthResult.FileName = plugin.FileName
		return *modrinthResult
	}

	if spigetResult != nil {
		spigetResult.FileName = plugin.FileName
		return *spigetResult
	}
	if modrinthResult != nil {
		modrinthResult.FileName = plugin.FileName
		return *modrinthResult
	}

	return info
}

func checkBySourceURL(ctx context.Context, sourceURL, pluginName, currentVersion, mcVersion, serverType string) (*PluginUpdateInfo, bool) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return nil, false
	}

	if resourceID, ok := parseSpigotResourceIDFromURL(sourceURL); ok {
		if debugPluginUpdatesEnabled() {
			log.Printf("[UpdateDebug] source=spigot plugin=%q current=%q mc=%q resourceID=%d", pluginName, currentVersion, mcVersion, resourceID)
		}
		return checkSpigetByID(ctx, resourceID, pluginName, currentVersion, mcVersion), true
	}
	if projectID, ok := parseModrinthProjectFromURL(sourceURL); ok {
		return checkModrinthByProject(ctx, projectID, pluginName, currentVersion, mcVersion, serverType), true
	}
	if _, ok := parseCurseForgeProjectFromURL(sourceURL); ok {
		// CurseForge update checks are not available without external API credentials.
		// Treat as handled so we do not fall back to fuzzy name matching.
		return nil, true
	}
	return nil, false
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
	VersionType   string   `json:"version_type"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Files         []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
	} `json:"files"`
}

// loaderTagsForType returns the Modrinth loader tags that are compatible with the given server type.
func loaderTagsForType(serverType string) []string {
	switch strings.ToLower(serverType) {
	case "paper":
		return []string{"paper", "spigot", "bukkit"}
	case "spigot":
		return []string{"spigot", "bukkit"}
	case "purpur":
		return []string{"purpur", "paper", "spigot", "bukkit"}
	case "folia":
		return []string{"folia", "paper", "spigot", "bukkit"}
	case "fabric":
		return []string{"fabric"}
	case "forge":
		return []string{"forge"}
	case "neoforge":
		return []string{"neoforge"}
	case "velocity":
		return []string{"velocity"}
	default:
		return nil
	}
}

func normalizeProjectName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func namesLikelySame(a, b string) bool {
	na := normalizeProjectName(a)
	nb := normalizeProjectName(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	if strings.HasPrefix(na, nb) || strings.HasPrefix(nb, na) {
		diff := len(na) - len(nb)
		if diff < 0 {
			diff = -diff
		}
		return diff <= 3
	}
	return false
}

type parsedVersion struct {
	numbers    []int
	preRelease string
}

var versionTokenPattern = regexp.MustCompile(`(?i)v?(\d+(?:\.\d+)+)(?:-([0-9a-z\.-]+))?(?:\+([0-9a-z\.-]+))?`)

func parseVersionCandidates(raw string) []parsedVersion {
	raw = strings.TrimSpace(raw)
	matches := versionTokenPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}

	out := make([]parsedVersion, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		numParts := strings.Split(m[1], ".")
		nums := make([]int, 0, len(numParts))
		ok := true
		for _, p := range numParts {
			n, err := strconv.Atoi(p)
			if err != nil {
				ok = false
				break
			}
			nums = append(nums, n)
		}
		if !ok {
			continue
		}
		pre := ""
		if len(m) >= 3 {
			pre = strings.ToLower(strings.TrimSpace(m[2]))
		}
		out = append(out, parsedVersion{numbers: nums, preRelease: pre})

		// Some projects place the real plugin/mod version in pre-release suffixes
		// (e.g. "mc1.21.1-0.15.2"). Parse that part as additional candidates.
		if len(m) >= 3 {
			prePart := strings.TrimSpace(m[2])
			if prePart != "" {
				nestedPre := versionTokenPattern.FindAllStringSubmatch(prePart, -1)
				for _, nm := range nestedPre {
					if len(nm) < 2 {
						continue
					}
					nparts := strings.Split(nm[1], ".")
					nnums := make([]int, 0, len(nparts))
					okNested := true
					for _, p := range nparts {
						n, err := strconv.Atoi(p)
						if err != nil {
							okNested = false
							break
						}
						nnums = append(nnums, n)
					}
					if !okNested {
						continue
					}
					npre := ""
					if len(nm) >= 3 {
						npre = strings.ToLower(strings.TrimSpace(nm[2]))
					}
					out = append(out, parsedVersion{numbers: nnums, preRelease: npre})
				}
			}
		}

		// Some projects place the actual plugin/mod version in build metadata
		// (e.g. "mc1.21.1+0.15.2"). Parse that segment as additional candidates.
		if len(m) >= 4 {
			build := strings.TrimSpace(m[3])
			if build != "" {
				nested := versionTokenPattern.FindAllStringSubmatch(build, -1)
				for _, nm := range nested {
					if len(nm) < 2 {
						continue
					}
					nparts := strings.Split(nm[1], ".")
					nnums := make([]int, 0, len(nparts))
					okNested := true
					for _, p := range nparts {
						n, err := strconv.Atoi(p)
						if err != nil {
							okNested = false
							break
						}
						nnums = append(nnums, n)
					}
					if !okNested {
						continue
					}
					npre := ""
					if len(nm) >= 3 {
						npre = strings.ToLower(strings.TrimSpace(nm[2]))
					}
					out = append(out, parsedVersion{numbers: nnums, preRelease: npre})
				}
			}
		}
	}
	return out
}

func parseVersionToken(raw string) (parsedVersion, bool) {
	candidates := parseVersionCandidates(raw)
	if len(candidates) == 0 {
		return parsedVersion{}, false
	}
	best := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if compareParsedVersions(candidates[i], best) > 0 {
			best = candidates[i]
		}
	}
	return best, true
}

func isLikelyMinecraftVersion(v parsedVersion) bool {
	if len(v.numbers) < 2 {
		return false
	}
	major := v.numbers[0]
	minor := v.numbers[1]
	if major != 1 {
		return false
	}
	return minor >= 7 && minor <= 40
}

func chooseComparisonVersion(raw, current string) (parsedVersion, bool) {
	cands := parseVersionCandidates(raw)
	if len(cands) == 0 {
		return parsedVersion{}, false
	}

	// If multiple candidates are present (often MC version + plugin version),
	// prefer non-Minecraft-looking candidates.
	if len(cands) > 1 {
		nonMC := make([]parsedVersion, 0, len(cands))
		for _, c := range cands {
			if !isLikelyMinecraftVersion(c) {
				nonMC = append(nonMC, c)
			}
		}
		if len(nonMC) > 0 {
			cands = nonMC
		}
	}

	best := cands[0]
	for i := 1; i < len(cands); i++ {
		if compareParsedVersions(cands[i], best) > 0 {
			best = cands[i]
		}
	}
	return best, true
}

// compareParsedVersions returns:
//  1 if a > b, -1 if a < b, 0 if equal
func compareParsedVersions(a, b parsedVersion) int {
	maxLen := len(a.numbers)
	if len(b.numbers) > maxLen {
		maxLen = len(b.numbers)
	}
	for i := 0; i < maxLen; i++ {
		av, bv := 0, 0
		if i < len(a.numbers) {
			av = a.numbers[i]
		}
		if i < len(b.numbers) {
			bv = b.numbers[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}

	// Stable release is newer than a prerelease on the same numeric version.
	if a.preRelease == "" && b.preRelease != "" {
		return 1
	}
	if a.preRelease != "" && b.preRelease == "" {
		return -1
	}
	if a.preRelease > b.preRelease {
		return 1
	}
	if a.preRelease < b.preRelease {
		return -1
	}
	return 0
}

func versionsEquivalentByCandidates(a, b string) bool {
	ac := parseVersionCandidates(a)
	bc := parseVersionCandidates(b)
	if len(ac) == 0 || len(bc) == 0 {
		return false
	}
	filter := func(in []parsedVersion) []parsedVersion {
		if len(in) <= 1 {
			return in
		}
		out := make([]parsedVersion, 0, len(in))
		for _, c := range in {
			if !isLikelyMinecraftVersion(c) {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
		return in
	}
	ac = filter(ac)
	bc = filter(bc)
	for _, x := range ac {
		for _, y := range bc {
			if compareParsedVersions(x, y) == 0 {
				return true
			}
		}
	}
	return false
}

func canonicalizeCompositeVersion(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	parts := strings.Split(raw, "+")
	tokens := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimPrefix(p, "v"))
		if p == "" {
			continue
		}

		if pv, ok := parseVersionToken(p); ok {
			nums := make([]string, 0, len(pv.numbers))
			for _, n := range pv.numbers {
				nums = append(nums, strconv.Itoa(n))
			}
			token := strings.Join(nums, ".")
			if pv.preRelease != "" {
				token += "-" + pv.preRelease
			}
			tokens = append(tokens, token)
			continue
		}

		var b strings.Builder
		for _, r := range p {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
				b.WriteRune(r)
			}
		}
		clean := strings.Trim(b.String(), "._-")
		if clean != "" {
			tokens = append(tokens, clean)
		}
	}

	if len(tokens) == 0 {
		return ""
	}
	sort.Strings(tokens)
	return strings.Join(tokens, "+")
}

func compareLatestToCurrent(current, latest string) (int, bool) {
	cCurrent := canonicalizeCompositeVersion(current)
	cLatest := canonicalizeCompositeVersion(latest)
	if cCurrent != "" && cLatest != "" && cCurrent == cLatest {
		return 0, true
	}
	if versionsEquivalentByCandidates(current, latest) {
		return 0, true
	}
	cv, cok := chooseComparisonVersion(current, current)
	lv, lok := chooseComparisonVersion(latest, current)
	if cok && lok {
		return compareParsedVersions(lv, cv), true
	}
	if versionsMatch(current, latest) {
		return 0, true
	}
	return 0, false
}

func chooseBestSpigetVersion(versions spigetVersionResult, mcVersion string) (spigetVersionResult, bool) {
	if len(versions) == 0 {
		return nil, false
	}

	serverMinor := normalizeMcMinor(mcVersion)

	for i, v := range versions {
		if isLikelyUnstableVersionName(v.Name) {
			continue
		}
		if serverMinor != "" && len(v.TestedVersions) > 0 {
			compatible := false
			for _, tv := range v.TestedVersions {
				if normalizeMcMinor(tv) == serverMinor {
					compatible = true
					break
				}
			}
			if !compatible {
				continue
			}
		}
		// Versions endpoint is sorted newest-first by id/date (sort=-id),
		// so pick the first compatible stable entry to avoid semver outliers.
		return []struct {
			ID             int      `json:"id"`
			Name           string   `json:"name"`
			TestedVersions []string `json:"testedVersions"`
		}{versions[i]}, true
	}
	return nil, false
}

func isLikelyUnstableVersionName(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return false
	}
	unstableMarkers := []string{
		"snapshot",
		"alpha",
		"beta",
		"pre",
		"rc",
		"dev",
		"nightly",
		"build",
		"#",
	}
	for _, marker := range unstableMarkers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func isStableModrinthVersion(v *modrinthVersion) bool {
	if v == nil {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(v.VersionType))
	if t == "alpha" || t == "beta" {
		return false
	}
	return !isLikelyUnstableVersionName(v.VersionNumber)
}

func checkModrinth(ctx context.Context, pluginName, currentVersion, mcVersion, serverType string) *PluginUpdateInfo {
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
	exactName := normalizeProjectName(pluginName)
	for _, hit := range searchResult.Hits {
		if normalizeProjectName(hit.Title) == exactName || normalizeProjectName(hit.Slug) == exactName {
			projectID = hit.ProjectID
			break
		}
	}
	if projectID == "" {
		for _, hit := range searchResult.Hits {
			if namesLikelySame(hit.Title, pluginName) || namesLikelySame(hit.Slug, pluginName) {
				projectID = hit.ProjectID
				break
			}
		}
	}
	if projectID == "" {
		// Avoid false positives from unrelated first-result matches.
		return nil
	}

	return checkModrinthByProject(ctx, projectID, pluginName, currentVersion, mcVersion, serverType)
}

func checkModrinthByProject(ctx context.Context, projectID, pluginName, currentVersion, mcVersion, serverType string) *PluginUpdateInfo {
	// Get versions for the project
	versionsURL := fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version", projectID)
	var versions []modrinthVersion
	if err := fetchJSON(ctx, versionsURL, &versions); err != nil {
		return nil
	}

	if len(versions) == 0 {
		return nil
	}

	// Find latest compatible version (matching both MC version and loader)
	allowedLoaders := loaderTagsForType(serverType)
	var latestCompatible *modrinthVersion
	var latestAny *modrinthVersion
	for i := range versions {
		v := &versions[i]
		if !isStableModrinthVersion(v) {
			continue
		}
		// Check if this version matches the server's loader
		loaderMatch := len(allowedLoaders) == 0 // if no loader tags, accept all
		for _, vl := range v.Loaders {
			for _, al := range allowedLoaders {
				if strings.EqualFold(vl, al) {
					loaderMatch = true
					break
				}
			}
			if loaderMatch {
				break
			}
		}
		if !loaderMatch {
			continue
		}
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
		if cmp, confident := compareLatestToCurrent(currentVersion, latestCompatible.VersionNumber); !confident {
			info.VersionStatus = "unknown"
		} else if cmp > 0 {
			info.VersionStatus = "outdated"
			for _, f := range latestCompatible.Files {
				if strings.HasSuffix(strings.ToLower(f.Filename), ".jar") && (f.Primary || len(latestCompatible.Files) == 1) {
					info.UpdateURL = f.URL
					break
				}
			}
			if info.UpdateURL == "" {
				for _, f := range latestCompatible.Files {
					if strings.HasSuffix(strings.ToLower(f.Filename), ".jar") {
						info.UpdateURL = f.URL
						break
					}
				}
			}
			if info.UpdateURL == "" {
				info.VersionStatus = "unknown"
			}
		} else if cmp == 0 {
			info.VersionStatus = "latest"
		} else {
			info.VersionStatus = "unknown"
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
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	TestedVersions []string `json:"testedVersions"`
}

type spigetResourceResult struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Version struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"version"`
}

func checkSpiget(ctx context.Context, pluginName, currentVersion, mcVersion string) *PluginUpdateInfo {
	searchURL := fmt.Sprintf("https://api.spiget.org/v2/search/resources/%s?field=name&size=5", url.QueryEscape(pluginName))

	var searchResult spigetSearchResult
	if err := fetchJSON(ctx, searchURL, &searchResult); err != nil {
		return nil
	}

	if len(searchResult) == 0 {
		return nil
	}

	// Find best match (strictly) to avoid unrelated resources.
	resourceID := 0
	exactName := normalizeProjectName(pluginName)
	for _, r := range searchResult {
		if normalizeProjectName(r.Name) == exactName {
			resourceID = r.ID
			break
		}
	}
	if resourceID == 0 {
		for _, r := range searchResult {
			if namesLikelySame(r.Name, pluginName) {
				resourceID = r.ID
				break
			}
		}
	}
	if resourceID == 0 {
		return nil
	}

	return checkSpigetByID(ctx, resourceID, pluginName, currentVersion, mcVersion)
}

var mcVersionHintPattern = regexp.MustCompile(`(?i)(?:\bmc)?(1\.\d{1,2}(?:\.\d+)?)`)

func normalizeMcMinor(v string) string {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func versionHintsCompatibility(label, serverMCVersion string) (hasHints bool, compatible bool) {
	matches := mcVersionHintPattern.FindAllStringSubmatch(label, -1)
	if len(matches) == 0 {
		return false, true
	}
	serverMinor := normalizeMcMinor(serverMCVersion)
	if serverMinor == "" {
		return true, false
	}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		if normalizeMcMinor(m[1]) == serverMinor {
			return true, true
		}
	}
	return true, false
}

func checkSpigetByID(ctx context.Context, resourceID int, pluginName, currentVersion, mcVersion string) *PluginUpdateInfo {
	info := &PluginUpdateInfo{
		Name:    pluginName,
		Version: currentVersion,
	}

	// Resource details endpoint is useful as fallback but can be noisy.
	resourceLatest := ""
	resourceURL := fmt.Sprintf("https://api.spiget.org/v2/resources/%d", resourceID)
	var resource spigetResourceResult
	if err := fetchJSON(ctx, resourceURL, &resource); err == nil && strings.TrimSpace(resource.Version.Name) != "" {
		latest := strings.TrimSpace(resource.Version.Name)
		if !isLikelyUnstableVersionName(latest) {
			resourceLatest = latest
		}
	}
	if debugPluginUpdatesEnabled() {
		log.Printf("[UpdateDebug] spiget resource=%d plugin=%q current=%q mc=%q resourceLatest=%q", resourceID, pluginName, currentVersion, mcVersion, resourceLatest)
	}

	// Get versions
	versionsURL := fmt.Sprintf("https://api.spiget.org/v2/resources/%d/versions?sort=-id&size=50", resourceID)
	var versions spigetVersionResult
	if err := fetchJSON(ctx, versionsURL, &versions); err != nil {
		if debugPluginUpdatesEnabled() {
			log.Printf("[UpdateDebug] spiget resource=%d versions fetch failed: %v", resourceID, err)
		}
		if resourceLatest == "" {
			return nil
		}
		info.LatestVersion = resourceLatest
		if hasHints, compat := versionHintsCompatibility(resourceLatest, mcVersion); hasHints && !compat {
			info.VersionStatus = "unknown"
			return info
		}
		if cmp, confident := compareLatestToCurrent(currentVersion, resourceLatest); !confident {
			info.VersionStatus = "unknown"
		} else if cmp > 0 {
			info.VersionStatus = "outdated"
			info.UpdateURL = fmt.Sprintf("https://api.spiget.org/v2/resources/%d/download", resourceID)
		} else if cmp == 0 {
			info.VersionStatus = "latest"
		} else {
			info.VersionStatus = "unknown"
		}
		return info
	}

	if len(versions) == 0 {
		if debugPluginUpdatesEnabled() {
			log.Printf("[UpdateDebug] spiget resource=%d versions list empty, using resourceLatest fallback", resourceID)
		}
		if resourceLatest == "" {
			return nil
		}
		info.LatestVersion = resourceLatest
		if hasHints, compat := versionHintsCompatibility(resourceLatest, mcVersion); hasHints && !compat {
			info.VersionStatus = "unknown"
			return info
		}
		if cmp, confident := compareLatestToCurrent(currentVersion, resourceLatest); !confident {
			info.VersionStatus = "unknown"
		} else if cmp > 0 {
			info.VersionStatus = "outdated"
			info.UpdateURL = fmt.Sprintf("https://api.spiget.org/v2/resources/%d/download", resourceID)
		} else if cmp == 0 {
			info.VersionStatus = "latest"
		} else {
			info.VersionStatus = "unknown"
		}
		return info
	}

	selected := versions[0]
	if best, ok := chooseBestSpigetVersion(versions, mcVersion); ok && len(best) > 0 {
		selected = best[0]
	} else if isLikelyUnstableVersionName(selected.Name) {
		if debugPluginUpdatesEnabled() {
			log.Printf("[UpdateDebug] spiget resource=%d selected unstable=%q -> unknown", resourceID, selected.Name)
		}
		return &PluginUpdateInfo{
			Name:          pluginName,
			Version:       currentVersion,
			LatestVersion: selected.Name,
			VersionStatus: "unknown",
		}
	}
	if debugPluginUpdatesEnabled() {
		log.Printf("[UpdateDebug] spiget resource=%d versions_count=%d", resourceID, len(versions))
		limit := 12
		if len(versions) < limit {
			limit = len(versions)
		}
		for i := 0; i < limit; i++ {
			log.Printf("[UpdateDebug] spiget resource=%d candidate[%d] name=%q tested=%v", resourceID, i, versions[i].Name, versions[i].TestedVersions)
		}
	}
	selectedLatest := strings.TrimSpace(selected.Name)
	chosenLatest := selectedLatest
	if chosenLatest == "" && resourceLatest != "" {
		chosenLatest = resourceLatest
	}
	info.LatestVersion = chosenLatest

	// If Spiget summary and versions-list disagree in comparison direction,
	// prefer "unknown" to avoid false positives.
	if resourceLatest != "" && selectedLatest != "" {
		cmpSelected, okSelected := compareLatestToCurrent(currentVersion, selectedLatest)
		cmpResource, okResource := compareLatestToCurrent(currentVersion, resourceLatest)
		if okSelected && okResource {
			sign := func(v int) int {
				if v > 0 {
					return 1
				}
				if v < 0 {
					return -1
				}
				return 0
			}
			if sign(cmpSelected) != sign(cmpResource) {
				if debugPluginUpdatesEnabled() {
					log.Printf("[UpdateDebug] spiget resource=%d conflict selectedLatest=%q cmpSelected=%d resourceLatest=%q cmpResource=%d -> unknown", resourceID, selectedLatest, cmpSelected, resourceLatest, cmpResource)
				}
				info.VersionStatus = "unknown"
				return info
			}
		}
	}

	if hasHints, compat := versionHintsCompatibility(info.LatestVersion, mcVersion); hasHints && !compat {
		info.VersionStatus = "unknown"
		return info
	}
	if cmp, confident := compareLatestToCurrent(currentVersion, info.LatestVersion); !confident {
		info.VersionStatus = "unknown"
	} else if cmp > 0 {
		info.VersionStatus = "outdated"
		info.UpdateURL = fmt.Sprintf("https://api.spiget.org/v2/resources/%d/download", resourceID)
	} else if cmp == 0 {
		info.VersionStatus = "latest"
	} else {
		info.VersionStatus = "unknown"
	}
	if debugPluginUpdatesEnabled() {
		cmp, confident := compareLatestToCurrent(currentVersion, info.LatestVersion)
		log.Printf("[UpdateDebug] spiget resource=%d current=%q selected=%q chosen=%q status=%s cmp=%d confident=%t updateURL=%q", resourceID, currentVersion, selectedLatest, info.LatestVersion, info.VersionStatus, cmp, confident, info.UpdateURL)
	}

	return info
}

func parseSpigotResourceIDFromURL(raw string) (int, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}

	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host == "" || !strings.HasSuffix(host, "spigotmc.org") {
		return 0, false
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		if id, err := strconv.Atoi(seg); err == nil && id > 0 {
			return id, true
		}

		if dot := strings.LastIndex(seg, "."); dot >= 0 && dot < len(seg)-1 {
			idPart := seg[dot+1:]
			if id, err := strconv.Atoi(idPart); err == nil && id > 0 {
				return id, true
			}
		}
	}

	return 0, false
}

func parseModrinthProjectFromURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}

	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host == "" || !strings.HasSuffix(host, "modrinth.com") {
		return "", false
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(segments)-1; i++ {
		if strings.EqualFold(segments[i], "project") {
			project := strings.TrimSpace(segments[i+1])
			if project != "" {
				return project, true
			}
		}
	}

	return "", false
}

func parseCurseForgeProjectFromURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}

	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host == "" || !strings.HasSuffix(host, "curseforge.com") {
		return "", false
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(segments)-1; i++ {
		if strings.EqualFold(segments[i], "mc-mods") {
			project := strings.TrimSpace(segments[i+1])
			if project != "" {
				return project, true
			}
		}
	}

	return "", false
}

func validateSourceURLForServerType(serverType, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("source URL is required")
	}
	if _, ok := parseSpigotResourceIDFromURL(raw); ok {
		if isModdedType(serverType) {
			return fmt.Errorf("modded servers require a Modrinth project link")
		}
		return nil
	}
	if _, ok := parseModrinthProjectFromURL(raw); ok {
		return nil
	}
	if _, ok := parseCurseForgeProjectFromURL(raw); ok {
		if isModdedType(serverType) {
			return nil
		}
		return fmt.Errorf("plugin servers only accept Spigot or Modrinth links")
	}
	if isModdedType(serverType) {
		return fmt.Errorf("invalid source URL: expected a Modrinth or CurseForge mod link")
	}
	return fmt.Errorf("invalid source URL: expected a Spigot resource link or Modrinth project link")
}

// SetPluginSource stores or updates a source URL for a plugin/mod file.
func (m *Manager) SetPluginSource(id, fileName, sourceURL string) error {
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %s not found", id)
	}

	if err := validateSourceURLForServerType(cfg.Type, sourceURL); err != nil {
		return err
	}

	pDir := extensionsDir(cfg)
	if _, err := SafePath(pDir, filepath.Base(fileName)); err != nil {
		return fmt.Errorf("invalid plugin path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(pDir, filepath.Base(fileName))); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plugin file not found: %s", fileName)
		}
		return err
	}

	sources := m.loadExtensionSources(cfg)
	key := normalizeExtensionSourceKey(fileName)
	sources[key] = strings.TrimSpace(sourceURL)
	if err := m.saveExtensionSources(cfg, sources); err != nil {
		return fmt.Errorf("failed to save source link: %w", err)
	}

	// Source links directly change detection behavior; invalidate cached update
	// results for this server/file so next check is fresh.
	pluginUpdateCache.mu.Lock()
	defer pluginUpdateCache.mu.Unlock()
	fileKey := strings.TrimSpace(fileName)
	normalizedKey := normalizeExtensionSourceKey(fileName)
	for cacheKey := range pluginUpdateCache.entries {
		if strings.HasPrefix(cacheKey, id+":") &&
			(strings.Contains(cacheKey, ":"+fileKey+":") || strings.Contains(cacheKey, ":"+normalizedKey+":")) {
			delete(pluginUpdateCache.entries, cacheKey)
		}
	}

	return nil
}

func resolveUpdateJarFileName(downloadURL, fallbackName, contentDisposition string) string {
	if strings.TrimSpace(contentDisposition) != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if name := strings.TrimSpace(params["filename"]); name != "" {
				base := filepath.Base(name)
				if strings.HasSuffix(strings.ToLower(base), ".jar") {
					return base
				}
			}
			if name := strings.TrimSpace(params["filename*"]); name != "" {
				base := filepath.Base(name)
				if strings.HasSuffix(strings.ToLower(base), ".jar") {
					return base
				}
			}
		}
	}

	if u, err := url.Parse(downloadURL); err == nil {
		base := strings.TrimSpace(path.Base(u.Path))
		base = filepath.Base(base)
		if strings.HasSuffix(strings.ToLower(base), ".jar") {
			return base
		}
	}

	base := filepath.Base(strings.TrimSpace(fallbackName))
	if strings.HasSuffix(strings.ToLower(base), ".jar") {
		return base
	}
	return fallbackName
}

var numericJarNamePattern = regexp.MustCompile(`^\d+\.jar$`)

func isUnfriendlyJarFileName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if n == "" {
		return true
	}
	if numericJarNamePattern.MatchString(n) {
		return true
	}
	if strings.HasPrefix(n, "download") {
		return true
	}
	return false
}

func sanitizeFilenameComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._- ")
}

func suggestUpdatedFileName(originalName, currentVersion, newVersion string) string {
	orig := filepath.Base(strings.TrimSpace(originalName))
	if !strings.HasSuffix(strings.ToLower(orig), ".jar") {
		return orig
	}
	if strings.TrimSpace(currentVersion) == "" || strings.TrimSpace(newVersion) == "" {
		return orig
	}
	if currentVersion == newVersion {
		return orig
	}

	base := strings.TrimSuffix(orig, ".jar")
	if !strings.Contains(base, currentVersion) {
		return orig
	}
	updatedBase := strings.ReplaceAll(base, currentVersion, newVersion)
	updated := strings.TrimSpace(updatedBase) + ".jar"
	if updated == ".jar" {
		return orig
	}
	return updated
}

func isLikelyJarArchive(path string) bool {
	r, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer r.Close()

	for _, f := range r.File {
		name := strings.ToLower(strings.TrimSpace(f.Name))
		if name == "" {
			continue
		}
		if name == "plugin.yml" || name == "bungee.yml" || name == "fabric.mod.json" || name == "meta-inf/mods.toml" {
			return true
		}
		if strings.HasSuffix(name, ".class") {
			return true
		}
	}
	return false
}

func extractJarFromArchive(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	var selected *zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(f.Name), ".jar") {
			continue
		}
		if selected == nil || f.UncompressedSize64 > selected.UncompressedSize64 {
			selected = f
		}
	}

	if selected == nil {
		return "", fmt.Errorf("no jar file found inside downloaded archive")
	}

	rc, err := selected.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	outPath := archivePath + ".inner.jar"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		_ = os.Remove(outPath)
		return "", err
	}

	if !isLikelyJarArchive(outPath) {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("embedded jar does not look like a valid plugin/mod archive")
	}

	return outPath, nil
}

func materializeDownloadJar(tmpPath string) (string, error) {
	// Valid direct jar.
	if isLikelyJarArchive(tmpPath) {
		return tmpPath, nil
	}

	// Try archive that contains one or more jar files.
	innerJar, err := extractJarFromArchive(tmpPath)
	if err == nil && innerJar != "" {
		return innerJar, nil
	}
	return "", fmt.Errorf("downloaded file is not a valid plugin/mod jar (or jar-containing archive)")
}

// UpdatePlugin downloads a new version of a plugin from a URL and replaces the old JAR
func (m *Manager) UpdatePlugin(id, fileName, downloadURL string) (*PluginInfo, error) {
	// Validate server exists and that plugin path is safe
	m.mu.RLock()
	cfg, ok := m.configs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not found", id)
	}

	// Disallow updating while server is running to avoid file-locks / corruption
	status, _ := m.GetStatus(id)
	if status != nil && (status.Status == "Running" || status.Status == "Booting") {
		return nil, fmt.Errorf("cannot update plugins while server is running; stop the server first")
	}

	pDir := extensionsDir(cfg)
	// Use SafePath to prevent traversal and ensure jar is inside the extensions dir
	jarPath, err := SafePath(pDir, filepath.Base(fileName))
	if err != nil {
		return nil, fmt.Errorf("invalid plugin path: %w", err)
	}

	// Verify the plugin file exists
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin file not found: %s", fileName)
	}
	_, currentVersion := extractPluginVersion(jarPath)

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

	resolvedURL := downloadURL
	if resp.Request != nil && resp.Request.URL != nil {
		resolvedURL = resp.Request.URL.String()
	}
	targetFileName := resolveUpdateJarFileName(resolvedURL, fileName, resp.Header.Get("Content-Disposition"))

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

	downloadedJarPath, err := materializeDownloadJar(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if downloadedJarPath != tmpPath {
		_ = os.Remove(tmpPath)
	}

	newName, newVersion := extractPluginVersion(downloadedJarPath)
	if newVersion == "" {
		_ = os.Remove(downloadedJarPath)
		return nil, fmt.Errorf("downloaded file is valid but version metadata is not readable")
	}
	if currentVersion != "" {
		if cmp, confident := compareLatestToCurrent(currentVersion, newVersion); confident {
			if cmp <= 0 {
				_ = os.Remove(downloadedJarPath)
				return nil, fmt.Errorf("downloaded version (%s) is not a newer version than installed version (%s)", newVersion, currentVersion)
			}
		} else if versionsMatch(currentVersion, newVersion) {
			_ = os.Remove(downloadedJarPath)
			return nil, fmt.Errorf("downloaded version (%s) is not a newer version than installed version (%s)", newVersion, currentVersion)
		}
	}

	if targetFileName == fileName || isUnfriendlyJarFileName(targetFileName) || (strings.TrimSpace(currentVersion) != "" && strings.Contains(targetFileName, currentVersion)) {
		// If provider filename is missing or still tied to old version, keep filename in sync with new version.
		targetFileName = suggestUpdatedFileName(fileName, currentVersion, newVersion)
	}
	if isUnfriendlyJarFileName(targetFileName) {
		baseName := sanitizeFilenameComponent(newName)
		if baseName == "" {
			baseName = sanitizeFilenameComponent(strings.TrimSuffix(fileName, ".jar"))
		}
		versionPart := sanitizeFilenameComponent(newVersion)
		if baseName != "" && versionPart != "" {
			targetFileName = baseName + "-" + versionPart + ".jar"
		}
	}
	targetPath, err := SafePath(pDir, filepath.Base(targetFileName))
	if err != nil {
		_ = os.Remove(downloadedJarPath)
		return nil, fmt.Errorf("invalid target plugin path: %w", err)
	}

	// Backup old JAR
	backupPath := jarPath + ".bak"
	if err := os.Rename(jarPath, backupPath); err != nil {
		_ = os.Remove(downloadedJarPath)
		return nil, fmt.Errorf("failed to backup old plugin: %w", err)
	}

	if targetPath != jarPath {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			os.Rename(backupPath, jarPath)
			_ = os.Remove(downloadedJarPath)
			return nil, fmt.Errorf("failed to replace existing target plugin: %w", err)
		}
	}

	// Move new JAR into place
	if err := os.Rename(downloadedJarPath, targetPath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, jarPath)
		return nil, fmt.Errorf("failed to install update: %w", err)
	}

	// Clean up backup
	os.Remove(backupPath)

	if oldKey, newKey := normalizeExtensionSourceKey(fileName), normalizeExtensionSourceKey(targetFileName); oldKey != newKey {
		sources := m.loadExtensionSources(cfg)
		if src, ok := sources[oldKey]; ok && strings.TrimSpace(src) != "" {
			sources[newKey] = src
			delete(sources, oldKey)
			_ = m.saveExtensionSources(cfg, sources)
		}
	}

	// Invalidate cache for this plugin
	pluginUpdateCache.mu.Lock()
	for key := range pluginUpdateCache.entries {
		if strings.Contains(key, fileName) || strings.Contains(key, targetFileName) {
			delete(pluginUpdateCache.entries, key)
		}
	}
	pluginUpdateCache.mu.Unlock()

	log.Printf("Updated plugin %s for server %s (installed as %s)", fileName, id, targetFileName)

	// Return updated plugin info
	info, _ := os.Stat(targetPath)
	pName, pVersion := extractPluginVersion(targetPath)
	if pName == "" {
		pName = newName
	}
	if pName == "" {
		pName = strings.TrimSuffix(targetFileName, ".jar")
	}

	return &PluginInfo{
		Name:     pName,
		FileName: targetFileName,
		Size:     formatFileSize(info.Size()),
		Enabled:  true,
		Version:  pVersion,
	}, nil
}
