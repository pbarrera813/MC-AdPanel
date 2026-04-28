package minecraft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultJavaExec = "java"
)

var bundledJDKPaths = map[int]string{
	17: "/opt/jdks/jdk17/bin/java",
	21: "/opt/jdks/jdk21/bin/java",
	25: "/opt/jdks/jdk25/bin/java",
}

type javaRequirementResolver struct {
	mu              sync.RWMutex
	availableByMaj  map[int]string
	vanillaReqCache map[string]int
}

func newJavaRequirementResolver() *javaRequirementResolver {
	r := &javaRequirementResolver{
		availableByMaj:  make(map[int]string),
		vanillaReqCache: make(map[string]int),
	}
	r.refreshAvailableJDKs()
	return r
}

func (r *javaRequirementResolver) refreshAvailableJDKs() {
	available := make(map[int]string)
	for major, javaPath := range bundledJDKPaths {
		if _, err := os.Stat(filepath.Clean(javaPath)); err == nil {
			available[major] = javaPath
		}
	}
	r.mu.Lock()
	r.availableByMaj = available
	r.mu.Unlock()
}

func (r *javaRequirementResolver) availableMajors() []int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	majors := make([]int, 0, len(r.availableByMaj))
	for major := range r.availableByMaj {
		majors = append(majors, major)
	}
	sort.Ints(majors)
	return majors
}

func (r *javaRequirementResolver) resolve(serverType, version string) (javaExec string, requiredMajor int, selectedMajor int, err error) {
	requiredMajor = r.requiredMajor(serverType, version)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.availableByMaj) == 0 {
		return "", requiredMajor, 0, fmt.Errorf("no bundled JDK runtimes available")
	}

	majors := make([]int, 0, len(r.availableByMaj))
	for major := range r.availableByMaj {
		majors = append(majors, major)
	}
	sort.Ints(majors)

	for _, major := range majors {
		if major >= requiredMajor {
			return r.availableByMaj[major], requiredMajor, major, nil
		}
	}

	return "", requiredMajor, 0, fmt.Errorf("required Java %d is not available (available: %v)", requiredMajor, majors)
}

func (r *javaRequirementResolver) requiredMajor(serverType, version string) int {
	serverType = strings.ToLower(strings.TrimSpace(serverType))
	if serverType == "vanilla" {
		if major, ok := r.resolveVanillaMajor(version); ok {
			return major
		}
	}

	return requiredJavaByMCVersion(version)
}

func requiredJavaByMCVersion(version string) int {
	version = strings.TrimSpace(strings.ToLower(version))
	if version == "" || version == "latest" {
		return 21
	}
	if strings.HasPrefix(version, "1.") {
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			minor, err := strconv.Atoi(parts[1])
			if err == nil {
				if minor >= 22 {
					return 25
				}
				if minor >= 21 {
					return 21
				}
			}
		}
	}
	return 17
}

func (r *javaRequirementResolver) resolveVanillaMajor(version string) (int, bool) {
	v := strings.TrimSpace(version)
	if v == "" || strings.EqualFold(v, "latest") {
		return 0, false
	}

	r.mu.RLock()
	cached, ok := r.vanillaReqCache[v]
	r.mu.RUnlock()
	if ok {
		return cached, true
	}

	major, err := fetchVanillaJavaMajor(v)
	if err != nil || major <= 0 {
		return 0, false
	}

	r.mu.Lock()
	r.vanillaReqCache[v] = major
	r.mu.Unlock()
	return major, true
}

type vanillaManifestRef struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type vanillaManifest struct {
	Versions []vanillaManifestRef `json:"versions"`
}

type vanillaVersionMeta struct {
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
}

func fetchVanillaJavaMajor(version string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://launchermeta.mojang.com/mc/game/version_manifest.json", nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var manifest vanillaManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return 0, err
	}

	metaURL := ""
	for _, entry := range manifest.Versions {
		if entry.ID == version {
			metaURL = entry.URL
			break
		}
	}
	if metaURL == "" {
		return 0, fmt.Errorf("version not found")
	}

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return 0, err
	}
	resp2, err := client.Do(req2)
	if err != nil {
		return 0, err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp2.StatusCode)
	}
	var meta vanillaVersionMeta
	if err := json.NewDecoder(resp2.Body).Decode(&meta); err != nil {
		return 0, err
	}
	if meta.JavaVersion.MajorVersion <= 0 {
		return 0, fmt.Errorf("java requirement unavailable")
	}
	return meta.JavaVersion.MajorVersion, nil
}
