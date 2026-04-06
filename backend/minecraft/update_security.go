package minecraft

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxPluginUpdateBytes int64 = 256 * 1024 * 1024
	maxPluginUpdateRedirects          = 5
)

var defaultPluginUpdateAllowedHosts = map[string]struct{}{
	"api.spiget.org":                {},
	"spigotmc.org":                  {},
	"www.spigotmc.org":              {},
	"modrinth.com":                  {},
	"api.modrinth.com":              {},
	"cdn.modrinth.com":              {},
	"curseforge.com":                {},
	"www.curseforge.com":            {},
	"mediafilez.forgecdn.net":       {},
	"edge.forgecdn.net":             {},
	"github.com":                    {},
	"raw.githubusercontent.com":     {},
	"objects.githubusercontent.com": {},
}

type pluginUpdateDownloadResult struct {
	ResolvedURL        string
	ContentDisposition string
}

func maxPluginUpdateBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("ADPANEL_MAX_PLUGIN_UPDATE_BYTES"))
	if raw == "" {
		return defaultMaxPluginUpdateBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		log.Printf("Invalid ADPANEL_MAX_PLUGIN_UPDATE_BYTES value %q, using default %d", raw, defaultMaxPluginUpdateBytes)
		return defaultMaxPluginUpdateBytes
	}
	return n
}

func normalizeHostForPolicy(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	host = strings.TrimSuffix(host, ".")
	return host
}

func pluginUpdateAllowedHosts() map[string]struct{} {
	allowed := make(map[string]struct{}, len(defaultPluginUpdateAllowedHosts))
	for host := range defaultPluginUpdateAllowedHosts {
		allowed[normalizeHostForPolicy(host)] = struct{}{}
	}
	for _, part := range strings.Split(os.Getenv("ADPANEL_PLUGIN_UPDATE_ALLOWED_HOSTS"), ",") {
		host := normalizeHostForPolicy(part)
		if host == "" {
			continue
		}
		allowed[host] = struct{}{}
	}
	return allowed
}

func hostAllowedByPolicy(host string, allowed map[string]struct{}) bool {
	host = normalizeHostForPolicy(host)
	if host == "" {
		return false
	}
	if _, ok := allowed[host]; ok {
		return true
	}
	for candidate := range allowed {
		if strings.HasPrefix(candidate, ".") {
			trimmed := strings.TrimPrefix(candidate, ".")
			if host == trimmed || strings.HasSuffix(host, "."+trimmed) {
				return true
			}
			continue
		}
		if host == candidate || strings.HasSuffix(host, "."+candidate) {
			return true
		}
	}
	return false
}

func ipBlockedForPluginUpdate(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}

func validatePluginUpdateURL(ctx context.Context, raw string, allowedHosts map[string]struct{}) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid download URL")
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("download URL must use https")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("download URL host is required")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("download URL must not include credentials")
	}

	host := normalizeHostForPolicy(parsed.Hostname())
	if !hostAllowedByPolicy(host, allowedHosts) {
		return nil, fmt.Errorf("download host is not allowed")
	}

	if ip := net.ParseIP(host); ip != nil {
		if ipBlockedForPluginUpdate(ip) {
			return nil, fmt.Errorf("download host resolves to blocked address")
		}
		return parsed, nil
	}

	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve download host")
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("download host has no address records")
	}
	for _, entry := range resolved {
		if ipBlockedForPluginUpdate(entry.IP) {
			return nil, fmt.Errorf("download host resolves to blocked address")
		}
	}
	return parsed, nil
}

func secureDownloadPluginUpdate(ctx context.Context, downloadURL, tmpPath string, maxBytes int64) (*pluginUpdateDownloadResult, error) {
	allowedHosts := pluginUpdateAllowedHosts()
	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	currentURL := strings.TrimSpace(downloadURL)
	for hop := 0; hop <= maxPluginUpdateRedirects; hop++ {
		validated, err := validatePluginUpdateURL(ctx, currentURL, allowedHosts)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, validated.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create download request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent())

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to download update: %w", err)
		}

		status := resp.StatusCode
		switch status {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			location := strings.TrimSpace(resp.Header.Get("Location"))
			_ = resp.Body.Close()
			if location == "" {
				return nil, fmt.Errorf("download redirect missing location header")
			}
			nextURL, err := validated.Parse(location)
			if err != nil {
				return nil, fmt.Errorf("invalid download redirect URL")
			}
			currentURL = nextURL.String()
			continue
		}

		if status != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("download failed with status %d", status)
		}

		tmpFile, err := os.Create(tmpPath)
		if err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}

		reader := io.LimitReader(resp.Body, maxBytes+1)
		written, copyErr := io.Copy(tmpFile, reader)
		closeErr := tmpFile.Close()
		_ = resp.Body.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("failed to save update: %w", copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("failed to finalize update file: %w", closeErr)
		}
		if written > maxBytes {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("download exceeds maximum allowed size")
		}

		resolved := currentURL
		if resp.Request != nil && resp.Request.URL != nil {
			resolved = resp.Request.URL.String()
		}
		return &pluginUpdateDownloadResult{
			ResolvedURL:        resolved,
			ContentDisposition: resp.Header.Get("Content-Disposition"),
		}, nil
	}
	return nil, fmt.Errorf("too many redirects while downloading update")
}
