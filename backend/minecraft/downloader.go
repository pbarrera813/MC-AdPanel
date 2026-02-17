package minecraft

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// VersionInfo represents a single available version for a server type
type VersionInfo struct {
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

// JarProvider defines the interface for downloading server jars
type JarProvider interface {
	FetchVersions(ctx context.Context) ([]VersionInfo, error)
	DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error
}

// ---------------------------------------------------------------------------
// Version Cache
// ---------------------------------------------------------------------------

type versionCache struct {
	mu      sync.RWMutex
	entries map[string]cachedVersions
}

type cachedVersions struct {
	versions  []VersionInfo
	fetchedAt time.Time
}

const versionCacheTTL = 15 * time.Minute

var globalVersionCache = &versionCache{
	entries: make(map[string]cachedVersions),
}

func (vc *versionCache) Get(serverType string) ([]VersionInfo, bool) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	entry, ok := vc.entries[strings.ToLower(serverType)]
	if !ok || time.Since(entry.fetchedAt) > versionCacheTTL {
		return nil, false
	}
	return entry.versions, true
}

func (vc *versionCache) Set(serverType string, versions []VersionInfo) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.entries[strings.ToLower(serverType)] = cachedVersions{
		versions:  versions,
		fetchedAt: time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Provider Registry
// ---------------------------------------------------------------------------

var providers = map[string]JarProvider{
	"vanilla":   &VanillaProvider{},
	"paper":     &PaperMCProvider{project: "paper"},
	"folia":     &PaperMCProvider{project: "folia"},
	"velocity":  &PaperMCProvider{project: "velocity"},
	"waterfall": &PaperMCProvider{project: "waterfall"},
	"purpur":    &PurpurProvider{},
	"fabric":    &FabricProvider{},
	"forge":     &ForgeProvider{},
	"neoforge":  &NeoForgeProvider{},
	"spigot":    &SpigotProvider{},
}

// GetProvider returns the JarProvider for a server type
func GetProvider(serverType string) (JarProvider, error) {
	p, ok := providers[strings.ToLower(serverType)]
	if !ok {
		return nil, fmt.Errorf("unsupported server type: %s", serverType)
	}
	return p, nil
}

var stableMcVersionPattern = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

// ---------------------------------------------------------------------------
// Shared HTTP Helpers
// ---------------------------------------------------------------------------

func userAgent() string {
	return effectiveUserAgent()
}

func fetchJSON(ctx context.Context, url string, target interface{}) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request to %s failed with status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func downloadFile(ctx context.Context, url, destPath string, progressFn func(string)) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent())
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("download from %s failed with status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading %s ...", filepath.Base(destPath)))
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(destPath) // clean up partial download
		return fmt.Errorf("download write failed: %w", err)
	}
	return nil
}

// resolveLatest resolves "Latest" to the actual latest version from a provider
func resolveLatest(ctx context.Context, provider JarProvider, version string) (string, error) {
	if !strings.EqualFold(version, "latest") {
		return version, nil
	}
	versions, err := provider.FetchVersions(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to resolve latest version: %w", err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	for _, v := range versions {
		if v.Latest {
			return v.Version, nil
		}
	}
	return versions[0].Version, nil
}

// ---------------------------------------------------------------------------
// PaperMC Provider (Paper, Folia, Velocity, Waterfall)
// ---------------------------------------------------------------------------

type PaperMCProvider struct {
	project string
}

type paperProjectResponse struct {
	Project struct {
		ID string `json:"id"`
	} `json:"project"`
	Versions map[string][]string `json:"versions"`
}

type paperBuild struct {
	ID        int                           `json:"id"`
	Channel   string                        `json:"channel"`
	Downloads map[string]paperBuildArtifact `json:"downloads"`
}

type paperBuildArtifact struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (p *PaperMCProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	url := fmt.Sprintf("https://fill.papermc.io/v3/projects/%s", p.project)
	var resp paperProjectResponse
	if err := fetchJSON(ctx, url, &resp); err != nil {
		return nil, err
	}

	var versions []VersionInfo
	groups := make([]string, 0, len(resp.Versions))
	for group := range resp.Versions {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return compareVersions(groups[i], groups[j]) > 0
	})
	seen := make(map[string]bool)
	for _, group := range groups {
		groupVersions := append([]string(nil), resp.Versions[group]...)
		sort.Slice(groupVersions, func(i, j int) bool {
			return compareVersions(groupVersions[i], groupVersions[j]) > 0
		})
		for _, v := range groupVersions {
			// Skip pre-releases and release candidates
			if strings.Contains(v, "-pre") || strings.Contains(v, "-rc") {
				continue
			}
			if seen[v] {
				continue
			}
			seen[v] = true
			versions = append(versions, VersionInfo{Version: v})
		}
	}

	// Keep only versions that have a stable build available
	filtered := make([]VersionInfo, 0, len(versions))
	for _, v := range versions {
		buildsURL := fmt.Sprintf("https://fill.papermc.io/v3/projects/%s/versions/%s/builds", p.project, v.Version)
		var builds []paperBuild
		if err := fetchJSON(ctx, buildsURL, &builds); err != nil {
			return nil, err
		}
		hasStable := false
		for i := range builds {
			if strings.EqualFold(builds[i].Channel, "STABLE") {
				hasStable = true
				break
			}
		}
		if hasStable {
			filtered = append(filtered, v)
		}
	}
	versions = filtered

	if len(versions) > 0 {
		versions[0].Latest = true
	}
	return versions, nil
}

func (p *PaperMCProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	if progressFn != nil {
		progressFn(fmt.Sprintf("Fetching builds for %s %s...", p.project, resolved))
	}

	// Get builds for this version
	url := fmt.Sprintf("https://fill.papermc.io/v3/projects/%s/versions/%s/builds", p.project, resolved)
	var buildsResp []paperBuild
	if err := fetchJSON(ctx, url, &buildsResp); err != nil {
		return fmt.Errorf("failed to fetch builds: %w", err)
	}

	if len(buildsResp) == 0 {
		return fmt.Errorf("no builds available for %s %s", p.project, resolved)
	}

	var selected *paperBuild
	for i := range buildsResp {
		if strings.EqualFold(buildsResp[i].Channel, "stable") {
			selected = &buildsResp[i]
			break
		}
	}
	if selected == nil {
		selected = &buildsResp[0]
	}

	download, ok := selected.Downloads["server:default"]
	if !ok {
		download, ok = selected.Downloads["application"]
	}
	if !ok && len(selected.Downloads) > 0 {
		for _, candidate := range selected.Downloads {
			download = candidate
			ok = true
			break
		}
	}
	if !ok || download.URL == "" {
		return fmt.Errorf("no download URL found for build %d", selected.ID)
	}

	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading %s %s (build #%d)...", p.project, resolved, selected.ID))
	}

	return downloadFile(ctx, download.URL, filepath.Join(destDir, "server.jar"), progressFn)
}

// ---------------------------------------------------------------------------
// Purpur Provider
// ---------------------------------------------------------------------------

type PurpurProvider struct{}

type purpurProjectResponse struct {
	Versions []string `json:"versions"`
}

func (p *PurpurProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	var resp purpurProjectResponse
	if err := fetchJSON(ctx, "https://api.purpurmc.org/v2/purpur", &resp); err != nil {
		return nil, err
	}

	var versions []VersionInfo
	for _, v := range resp.Versions {
		versions = append(versions, VersionInfo{Version: v})
	}

	// Reverse so newest is first
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	if len(versions) > 0 {
		versions[0].Latest = true
	}
	return versions, nil
}

func (p *PurpurProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	downloadURL := fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/latest/download", resolved)
	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading Purpur %s...", resolved))
	}

	return downloadFile(ctx, downloadURL, filepath.Join(destDir, "server.jar"), progressFn)
}

// ---------------------------------------------------------------------------
// Fabric Provider
// ---------------------------------------------------------------------------

type FabricProvider struct{}

type fabricGameVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

type fabricLoaderVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

type fabricInstallerVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

func (p *FabricProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	var gameVersions []fabricGameVersion
	if err := fetchJSON(ctx, "https://meta.fabricmc.net/v2/versions/game", &gameVersions); err != nil {
		return nil, err
	}

	var versions []VersionInfo
	for _, gv := range gameVersions {
		if !gv.Stable {
			continue
		}
		versions = append(versions, VersionInfo{Version: gv.Version})
	}

	// Already sorted newest first from the API
	if len(versions) > 0 {
		versions[0].Latest = true
	}
	return versions, nil
}

func (p *FabricProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	// Get latest stable loader version
	if progressFn != nil {
		progressFn("Fetching Fabric loader versions...")
	}
	var loaders []fabricLoaderVersion
	if err := fetchJSON(ctx, "https://meta.fabricmc.net/v2/versions/loader", &loaders); err != nil {
		return fmt.Errorf("failed to fetch loader versions: %w", err)
	}

	loaderVersion := ""
	for _, l := range loaders {
		if l.Stable {
			loaderVersion = l.Version
			break
		}
	}
	if loaderVersion == "" && len(loaders) > 0 {
		loaderVersion = loaders[0].Version
	}
	if loaderVersion == "" {
		return fmt.Errorf("no Fabric loader versions available")
	}

	// Get latest stable installer version
	if progressFn != nil {
		progressFn("Fetching Fabric installer versions...")
	}
	var installers []fabricInstallerVersion
	if err := fetchJSON(ctx, "https://meta.fabricmc.net/v2/versions/installer", &installers); err != nil {
		return fmt.Errorf("failed to fetch installer versions: %w", err)
	}

	installerVersion := ""
	for _, ins := range installers {
		if ins.Stable {
			installerVersion = ins.Version
			break
		}
	}
	if installerVersion == "" && len(installers) > 0 {
		installerVersion = installers[0].Version
	}
	if installerVersion == "" {
		return fmt.Errorf("no Fabric installer versions available")
	}

	downloadURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/%s/server/jar", resolved, loaderVersion, installerVersion)
	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading Fabric %s with loader %s (installer %s)...", resolved, loaderVersion, installerVersion))
	}

	return downloadFile(ctx, downloadURL, filepath.Join(destDir, "server.jar"), progressFn)
}

// ---------------------------------------------------------------------------
// Forge Provider
// ---------------------------------------------------------------------------

type ForgeProvider struct{}

type forgePromotionsResponse struct {
	Promos map[string]string `json:"promos"`
}

func (p *ForgeProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	var resp forgePromotionsResponse
	if err := fetchJSON(ctx, "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", &resp); err != nil {
		return nil, err
	}

	// Extract unique MC versions from promo keys (format: "1.20.4-latest", "1.20.4-recommended")
	versionSet := make(map[string]bool)
	for key := range resp.Promos {
		parts := strings.SplitN(key, "-", 2)
		if len(parts) == 2 {
			if stableMcVersionPattern.MatchString(parts[0]) {
				versionSet[parts[0]] = true
			}
		}
	}

	var versions []VersionInfo
	for v := range versionSet {
		versions = append(versions, VersionInfo{Version: v})
	}

	// Sort descending by version
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	if len(versions) > 0 {
		versions[0].Latest = true
	}
	return versions, nil
}

func (p *ForgeProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	// Get the Forge build number for this MC version
	if progressFn != nil {
		progressFn(fmt.Sprintf("Fetching Forge version for MC %s...", resolved))
	}

	var promos forgePromotionsResponse
	if err := fetchJSON(ctx, "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", &promos); err != nil {
		return fmt.Errorf("failed to fetch Forge promotions: %w", err)
	}

	// Prefer recommended, fall back to latest
	forgeBuild := promos.Promos[resolved+"-recommended"]
	if forgeBuild == "" {
		forgeBuild = promos.Promos[resolved+"-latest"]
	}
	if forgeBuild == "" {
		return fmt.Errorf("no Forge build found for MC %s", resolved)
	}

	// Download installer
	installerName := fmt.Sprintf("forge-%s-%s-installer.jar", resolved, forgeBuild)
	installerURL := fmt.Sprintf("https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/%s",
		resolved, forgeBuild, installerName)
	installerPath := filepath.Join(destDir, "forge-installer.jar")

	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading Forge %s-%s installer...", resolved, forgeBuild))
	}

	if err := downloadFile(ctx, installerURL, installerPath, progressFn); err != nil {
		return fmt.Errorf("failed to download Forge installer: %w", err)
	}

	// Run the installer
	if progressFn != nil {
		progressFn("Running Forge installer (this may take a few minutes)...")
	}

	cmd := exec.CommandContext(ctx, "java", "-jar", "forge-installer.jar", "--installServer")
	cmd.Dir = destDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Forge installer failed: %s: %w", string(output), err)
	}

	// Clean up installer
	os.Remove(installerPath)
	os.Remove(filepath.Join(destDir, "forge-installer.jar.log"))

	if progressFn != nil {
		progressFn("Forge installation complete.")
	}

	return nil
}

// ---------------------------------------------------------------------------
// NeoForge Provider
// ---------------------------------------------------------------------------

type NeoForgeProvider struct{}

type neoforgeVersionsResponse struct {
	Versions []string `json:"versions"`
}

func (p *NeoForgeProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	var resp neoforgeVersionsResponse
	if err := fetchJSON(ctx, "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge", &resp); err != nil {
		return nil, err
	}

	// Map NeoForge versions to MC versions
	// NeoForge version: major.minor.patch → MC version: 1.major.minor
	// Filter out beta/alpha/snapshot versions
	mcVersionSet := make(map[string]string) // MC version → latest NeoForge version for it

	for _, v := range resp.Versions {
		if strings.Contains(v, "-beta") || strings.Contains(v, "-alpha") || strings.Contains(v, "+") {
			continue
		}
		parts := strings.SplitN(v, ".", 3)
		if len(parts) < 2 {
			continue
		}
		mcVersion := fmt.Sprintf("1.%s.%s", parts[0], parts[1])
		// Keep the latest NeoForge version for each MC version
		mcVersionSet[mcVersion] = v
	}

	var versions []VersionInfo
	for mc := range mcVersionSet {
		versions = append(versions, VersionInfo{Version: mc})
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	if len(versions) > 0 {
		versions[0].Latest = true
	}
	return versions, nil
}

func (p *NeoForgeProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	// Map MC version back to NeoForge version
	if progressFn != nil {
		progressFn(fmt.Sprintf("Fetching NeoForge version for MC %s...", resolved))
	}

	var resp neoforgeVersionsResponse
	if err := fetchJSON(ctx, "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge", &resp); err != nil {
		return fmt.Errorf("failed to fetch NeoForge versions: %w", err)
	}

	// Parse MC version "1.X.Y" → NeoForge prefix "X.Y."
	mcParts := strings.SplitN(resolved, ".", 3)
	if len(mcParts) < 3 {
		return fmt.Errorf("invalid MC version format: %s", resolved)
	}
	nfPrefix := mcParts[1] + "." + mcParts[2] + "."

	// Find the latest stable NeoForge version with this prefix
	nfVersion := ""
	for _, v := range resp.Versions {
		if strings.Contains(v, "-beta") || strings.Contains(v, "-alpha") || strings.Contains(v, "+") {
			continue
		}
		if strings.HasPrefix(v, nfPrefix) {
			nfVersion = v // keep last = latest
		}
	}
	if nfVersion == "" {
		return fmt.Errorf("no NeoForge version found for MC %s", resolved)
	}

	// Download installer
	installerName := fmt.Sprintf("neoforge-%s-installer.jar", nfVersion)
	installerURL := fmt.Sprintf("https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/%s",
		nfVersion, installerName)
	installerPath := filepath.Join(destDir, "neoforge-installer.jar")

	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading NeoForge %s installer...", nfVersion))
	}

	if err := downloadFile(ctx, installerURL, installerPath, progressFn); err != nil {
		return fmt.Errorf("failed to download NeoForge installer: %w", err)
	}

	// Run the installer
	if progressFn != nil {
		progressFn("Running NeoForge installer (this may take a few minutes)...")
	}

	cmd := exec.CommandContext(ctx, "java", "-jar", "neoforge-installer.jar", "--installServer")
	cmd.Dir = destDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("NeoForge installer failed: %s: %w", string(output), err)
	}

	// Clean up installer
	os.Remove(installerPath)
	os.Remove(filepath.Join(destDir, "neoforge-installer.jar.log"))

	if progressFn != nil {
		progressFn("NeoForge installation complete.")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Spigot Provider (via BuildTools)
// ---------------------------------------------------------------------------

type SpigotProvider struct{}

func (p *SpigotProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	// Use PaperMC's version list as reference since Paper tracks all Spigot-compatible versions
	paperProvider := &PaperMCProvider{project: "paper"}
	return paperProvider.FetchVersions(ctx)
}

func (p *SpigotProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	// Download BuildTools
	buildToolsURL := "https://hub.spigotmc.org/jenkins/job/BuildTools/lastSuccessfulBuild/artifact/target/BuildTools.jar"
	buildToolsPath := filepath.Join(destDir, "BuildTools.jar")

	if progressFn != nil {
		progressFn("Downloading Spigot BuildTools...")
	}

	if err := downloadFile(ctx, buildToolsURL, buildToolsPath, progressFn); err != nil {
		return fmt.Errorf("failed to download BuildTools: %w", err)
	}

	// Run BuildTools (this takes 10+ minutes)
	if progressFn != nil {
		progressFn(fmt.Sprintf("Building Spigot %s with BuildTools (this takes 10-15 minutes)...", resolved))
	}

	cmd := exec.CommandContext(ctx, "java", "-jar", "BuildTools.jar", "--rev", resolved)
	cmd.Dir = destDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("BuildTools failed: %s: %w", string(output), err)
	}

	// Find the built spigot jar and rename to server.jar
	matches, _ := filepath.Glob(filepath.Join(destDir, "spigot-*.jar"))
	if len(matches) == 0 {
		return fmt.Errorf("BuildTools completed but no spigot jar found")
	}

	if err := os.Rename(matches[0], filepath.Join(destDir, "server.jar")); err != nil {
		return fmt.Errorf("failed to rename spigot jar: %w", err)
	}

	// Clean up BuildTools artifacts
	os.Remove(buildToolsPath)
	for _, pattern := range []string{"apache-maven-*", "BuildData", "Bukkit", "CraftBukkit", "Spigot", "work"} {
		found, _ := filepath.Glob(filepath.Join(destDir, pattern))
		for _, f := range found {
			os.RemoveAll(f)
		}
	}

	if progressFn != nil {
		progressFn("Spigot build complete.")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Version comparison helper
// ---------------------------------------------------------------------------

func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &av)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bv)
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Vanilla Provider
// ---------------------------------------------------------------------------

type VanillaProvider struct{}

type mojangVersionManifest struct {
	Latest struct {
		Release string `json:"release"`
	} `json:"latest"`
	Versions []struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		URL         string `json:"url"`
		ReleaseTime string `json:"releaseTime"`
	} `json:"versions"`
}

type mojangVersionMeta struct {
	Downloads struct {
		Server struct {
			URL string `json:"url"`
		} `json:"server"`
	} `json:"downloads"`
}

func (p *VanillaProvider) FetchVersions(ctx context.Context) ([]VersionInfo, error) {
	var manifest mojangVersionManifest
	if err := fetchJSON(ctx, "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &manifest); err != nil {
		return nil, err
	}

	versions := make([]VersionInfo, 0, len(manifest.Versions))
	for _, v := range manifest.Versions {
		if !strings.EqualFold(v.Type, "release") {
			continue
		}
		versions = append(versions, VersionInfo{
			Version: v.ID,
			Latest:  v.ID == manifest.Latest.Release,
		})
	}

	if len(versions) > 0 {
		hasLatest := false
		for i := range versions {
			if versions[i].Latest {
				hasLatest = true
				break
			}
		}
		if !hasLatest {
			versions[0].Latest = true
		}
	}
	return versions, nil
}

func (p *VanillaProvider) DownloadJar(ctx context.Context, version string, destDir string, progressFn func(string)) error {
	resolved, err := resolveLatest(ctx, p, version)
	if err != nil {
		return err
	}

	var manifest mojangVersionManifest
	if err := fetchJSON(ctx, "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &manifest); err != nil {
		return err
	}

	metaURL := ""
	for _, v := range manifest.Versions {
		if v.ID == resolved {
			metaURL = v.URL
			break
		}
	}
	if metaURL == "" {
		return fmt.Errorf("vanilla version %s not found", resolved)
	}

	var meta mojangVersionMeta
	if err := fetchJSON(ctx, metaURL, &meta); err != nil {
		return fmt.Errorf("failed to fetch vanilla version metadata: %w", err)
	}
	if strings.TrimSpace(meta.Downloads.Server.URL) == "" {
		return fmt.Errorf("server jar URL unavailable for vanilla %s", resolved)
	}

	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading Vanilla %s...", resolved))
	}

	return downloadFile(ctx, meta.Downloads.Server.URL, filepath.Join(destDir, "server.jar"), progressFn)
}
