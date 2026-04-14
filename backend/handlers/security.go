package handlers

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultMaxUploadBytes       int64 = 256 * 1024 * 1024
	defaultMaxServerImportBytes int64 = 8 * 1024 * 1024 * 1024
	defaultCSRFMode                   = "enforce"
)

type trustedProxySet struct {
	nets []*net.IPNet
}

func newTrustedProxySetFromEnv() *trustedProxySet {
	return parseTrustedProxySet(strings.TrimSpace(os.Getenv("ADPANEL_TRUSTED_PROXIES")))
}

func parseTrustedProxySet(raw string) *trustedProxySet {
	set := &trustedProxySet{nets: make([]*net.IPNet, 0)}
	if strings.TrimSpace(raw) == "" {
		return set
	}

	parts := strings.Split(raw, ",")
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		if _, ipNet, err := net.ParseCIDR(entry); err == nil {
			set.nets = append(set.nets, ipNet)
			continue
		}

		ip := net.ParseIP(entry)
		if ip == nil {
			log.Printf("Ignoring invalid trusted proxy entry %q", entry)
			continue
		}

		maskBits := 32
		if ip.To4() == nil {
			maskBits = 128
		}
		set.nets = append(set.nets, &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(maskBits, maskBits),
		})
	}
	return set
}

func (s *trustedProxySet) isTrusted(remoteAddr string) bool {
	if s == nil || len(s.nets) == 0 {
		return false
	}
	ip := remoteAddrIP(remoteAddr)
	return s.containsIP(ip)
}

func (s *trustedProxySet) containsIP(ip net.IP) bool {
	if s == nil || len(s.nets) == 0 || ip == nil {
		return false
	}
	for _, ipNet := range s.nets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteAddrIP(remoteAddr string) net.IP {
	addr := strings.TrimSpace(remoteAddr)
	if addr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func firstForwardedForIP(xffHeader string) string {
	for _, part := range strings.Split(xffHeader, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		candidate = strings.Trim(candidate, "[]")
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func parseForwardedForIPs(xffHeader string) []net.IP {
	parts := strings.Split(xffHeader, ",")
	out := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		candidate = strings.Trim(candidate, "[]")
		if ip := net.ParseIP(candidate); ip != nil {
			out = append(out, ip)
		}
	}
	return out
}

// Resolve trusted proxy chains from right-to-left to avoid spoofed left-most entries.
func realClientIPFromXFF(remoteAddr string, xffHeader string, trusted *trustedProxySet) string {
	remoteIP := remoteAddrIP(remoteAddr)
	if remoteIP == nil {
		return strings.TrimSpace(remoteAddr)
	}
	if trusted == nil || !trusted.isTrusted(remoteAddr) {
		return remoteIP.String()
	}

	chain := parseForwardedForIPs(xffHeader)
	if len(chain) == 0 {
		return remoteIP.String()
	}

	for i := len(chain) - 1; i >= 0; i-- {
		candidate := chain[i]
		if !trusted.containsIP(candidate) {
			return candidate.String()
		}
	}

	// All addresses in the chain were trusted; use the oldest visible hop.
	return chain[0].String()
}

func firstHeaderToken(raw string) string {
	token := strings.TrimSpace(strings.Split(raw, ",")[0])
	return strings.TrimSpace(token)
}

func parseAllowedOriginsEnv() map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, part := range strings.Split(os.Getenv("ADPANEL_ALLOWED_ORIGINS"), ",") {
		if normalized := normalizeOrigin(part); normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	return allowed
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return ""
	}
	port := strings.TrimSpace(u.Port())
	if port == "" {
		return fmt.Sprintf("%s://%s", scheme, host)
	}
	if strings.Contains(host, ":") {
		return fmt.Sprintf("%s://[%s]:%s", scheme, host, port)
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port)
}

func isLocalDevOrigin(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func isAllowedWebSocketOrigin(origin string, allowed map[string]struct{}) bool {
	normalized := normalizeOrigin(origin)
	if normalized == "" {
		return false
	}
	if _, ok := allowed[normalized]; ok {
		return true
	}
	return isLocalDevOrigin(normalized)
}

func requestScheme(r *http.Request, trusted *trustedProxySet) string {
	if r != nil && r.TLS != nil {
		return "https"
	}
	if r != nil && trusted != nil && trusted.isTrusted(r.RemoteAddr) {
		proto := strings.ToLower(firstHeaderToken(r.Header.Get("X-Forwarded-Proto")))
		if proto == "http" || proto == "https" {
			return proto
		}
	}
	return "http"
}

func requestHost(r *http.Request, trusted *trustedProxySet) string {
	if r == nil {
		return ""
	}
	if trusted != nil && trusted.isTrusted(r.RemoteAddr) {
		if xfh := firstHeaderToken(r.Header.Get("X-Forwarded-Host")); strings.TrimSpace(xfh) != "" {
			return strings.TrimSpace(xfh)
		}
	}
	return strings.TrimSpace(r.Host)
}

func requestOrigin(r *http.Request, trusted *trustedProxySet) string {
	host := requestHost(r, trusted)
	if host == "" {
		return ""
	}
	return normalizeOrigin(fmt.Sprintf("%s://%s", requestScheme(r, trusted), host))
}

func isAllowedWebSocketOriginForRequest(r *http.Request, origin string, allowed map[string]struct{}, trusted *trustedProxySet) bool {
	normalizedOrigin := normalizeOrigin(origin)
	if normalizedOrigin == "" {
		return false
	}
	if _, ok := allowed[normalizedOrigin]; ok {
		return true
	}
	if isLocalDevOrigin(normalizedOrigin) {
		return true
	}
	normalizedRequestOrigin := requestOrigin(r, trusted)
	return normalizedRequestOrigin != "" && normalizedRequestOrigin == normalizedOrigin
}

func uploadMaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("ADPANEL_MAX_UPLOAD_BYTES"))
	if raw == "" {
		return defaultMaxUploadBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		log.Printf("Invalid ADPANEL_MAX_UPLOAD_BYTES value %q, using default %d", raw, defaultMaxUploadBytes)
		return defaultMaxUploadBytes
	}
	return n
}

func serverImportMaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("ADPANEL_MAX_SERVER_IMPORT_BYTES"))
	if raw == "" {
		return defaultMaxServerImportBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		log.Printf("Invalid ADPANEL_MAX_SERVER_IMPORT_BYTES value %q, using default %d", raw, defaultMaxServerImportBytes)
		return defaultMaxServerImportBytes
	}
	return n
}

func csrfModeFromEnv() string {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("ADPANEL_CSRF_MODE")))
	switch raw {
	case "", "enforce":
		return defaultCSRFMode
	case "report":
		return "report"
	case "off":
		return "off"
	default:
		log.Printf("Invalid ADPANEL_CSRF_MODE value %q, using %q", raw, defaultCSRFMode)
		return defaultCSRFMode
	}
}

func isUnsafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestOriginMatchesCSRF(r *http.Request, trusted *trustedProxySet) bool {
	expectedOrigin := requestOrigin(r, trusted)
	if expectedOrigin == "" {
		return false
	}

	originHeader := normalizeOrigin(strings.TrimSpace(r.Header.Get("Origin")))
	if originHeader != "" {
		return originHeader == expectedOrigin
	}

	refererHeader := strings.TrimSpace(r.Header.Get("Referer"))
	if refererHeader != "" {
		if refURL, err := url.Parse(refererHeader); err == nil {
			refererOrigin := normalizeOrigin(fmt.Sprintf("%s://%s", refURL.Scheme, refURL.Host))
			return refererOrigin != "" && refererOrigin == expectedOrigin
		}
	}

	// Non-browser or stripped-header clients: allow to avoid breaking automation.
	return true
}

func isRequestBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "request body too large") || strings.Contains(lower, "multipart: message too large")
}
