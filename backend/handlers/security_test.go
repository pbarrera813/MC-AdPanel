package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadMaxBytesFromEnv(t *testing.T) {
	t.Setenv("ADPANEL_MAX_UPLOAD_BYTES", "")
	if got := uploadMaxBytesFromEnv(); got != defaultMaxUploadBytes {
		t.Fatalf("expected default upload cap %d, got %d", defaultMaxUploadBytes, got)
	}

	t.Setenv("ADPANEL_MAX_UPLOAD_BYTES", "536870912")
	if got := uploadMaxBytesFromEnv(); got != 536870912 {
		t.Fatalf("expected configured upload cap, got %d", got)
	}
}

func TestServerImportMaxBytesFromEnv(t *testing.T) {
	t.Setenv("ADPANEL_MAX_SERVER_IMPORT_BYTES", "")
	if got := serverImportMaxBytesFromEnv(); got != defaultMaxServerImportBytes {
		t.Fatalf("expected default import cap %d, got %d", defaultMaxServerImportBytes, got)
	}

	t.Setenv("ADPANEL_MAX_SERVER_IMPORT_BYTES", "4294967296")
	if got := serverImportMaxBytesFromEnv(); got != 4294967296 {
		t.Fatalf("expected configured import cap, got %d", got)
	}
}

func TestServerImportMaxBytesFromEnvFallsBackOnInvalid(t *testing.T) {
	t.Setenv("ADPANEL_MAX_SERVER_IMPORT_BYTES", "0")
	if got := serverImportMaxBytesFromEnv(); got != defaultMaxServerImportBytes {
		t.Fatalf("expected default import cap on invalid value, got %d", got)
	}

	t.Setenv("ADPANEL_MAX_SERVER_IMPORT_BYTES", "invalid")
	if got := serverImportMaxBytesFromEnv(); got != defaultMaxServerImportBytes {
		t.Fatalf("expected default import cap on parse error, got %d", got)
	}
}

func TestTrustedProxySet(t *testing.T) {
	set := parseTrustedProxySet("10.0.0.0/8, 127.0.0.1")
	if !set.isTrusted("10.1.2.3:1234") {
		t.Fatalf("expected 10.1.2.3 to be trusted")
	}
	if !set.isTrusted("127.0.0.1:9999") {
		t.Fatalf("expected loopback to be trusted")
	}
	if set.isTrusted("192.168.1.5:443") {
		t.Fatalf("expected 192.168.1.5 to be untrusted")
	}
}

func TestForwardedForParsing(t *testing.T) {
	got := firstForwardedForIP("203.0.113.10, 10.0.0.2")
	if got != "203.0.113.10" {
		t.Fatalf("unexpected first forwarded IP: %q", got)
	}
}

func TestRealClientIPFromXFFUsesRightToLeftTrust(t *testing.T) {
	trusted := parseTrustedProxySet("10.0.0.0/8")
	got := realClientIPFromXFF("10.2.0.5:443", "9.9.9.9, 198.51.100.40", trusted)
	if got != "198.51.100.40" {
		t.Fatalf("expected right-most untrusted IP, got %q", got)
	}
}

func TestWebSocketOriginPolicy(t *testing.T) {
	allowed := map[string]struct{}{
		normalizeOrigin("https://panel.example.com"): {},
	}
	if !isAllowedWebSocketOrigin("https://panel.example.com", allowed) {
		t.Fatalf("expected allowlisted origin to pass")
	}
	if !isAllowedWebSocketOrigin("http://localhost:5173", allowed) {
		t.Fatalf("expected localhost dev origin fallback to pass")
	}
	if isAllowedWebSocketOrigin("https://evil.example.com", allowed) {
		t.Fatalf("expected unlisted origin to be rejected")
	}
}

func TestWebSocketOriginSameOriginFallback(t *testing.T) {
	req := httptest.NewRequest("GET", "http://panel.example.com/api/logs/server-1", nil)
	req.Host = "panel.example.com"

	if !isAllowedWebSocketOriginForRequest(req, "http://panel.example.com", map[string]struct{}{}, nil) {
		t.Fatalf("expected same-origin WebSocket request to be allowed without explicit allowlist")
	}
}

func TestRequestOriginMatchesCSRF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://panel.example.com/api/servers", nil)
	req.Host = "panel.example.com"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Origin", "https://panel.example.com")
	if !requestOriginMatchesCSRF(req, nil) {
		t.Fatalf("expected same-origin csrf check to pass")
	}
	req.Header.Set("Origin", "https://evil.example.com")
	if requestOriginMatchesCSRF(req, nil) {
		t.Fatalf("expected cross-origin csrf check to fail")
	}
}
