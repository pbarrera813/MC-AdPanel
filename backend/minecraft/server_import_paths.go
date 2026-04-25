package minecraft

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

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
		return "", fmt.Errorf("file contains invalid path")
	}
	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == "/" {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("file contains unsafe path %q", name)
	}
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		return "", fmt.Errorf("file contains unsafe path %q", name)
	}
	return cleaned, nil
}
