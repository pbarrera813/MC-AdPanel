package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minecraft-admin/minecraft"
)

func TestDefaultCredentialGateAndUnlockFlow(t *testing.T) {
	base := t.TempDir()
	mgr, err := minecraft.NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	handler := NewAuthHandler(mgr, base)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"mcpanel","password":"mcpanel"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.Login(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d", loginRec.Code)
	}

	var loginBody map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if v, ok := loginBody["mustChangePassword"].(bool); !ok || !v {
		t.Fatalf("expected mustChangePassword=true, got %v", loginBody["mustChangePassword"])
	}

	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}
	sessionCookie := cookies[0]

	middleware := handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	blockedReq := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	blockedReq.AddCookie(sessionCookie)
	blockedRec := httptest.NewRecorder()
	middleware.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected %d while password change required, got %d", http.StatusPreconditionRequired, blockedRec.Code)
	}

	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", nil)
	settingsReq.AddCookie(sessionCookie)
	settingsRec := httptest.NewRecorder()
	middleware.ServeHTTP(settingsRec, settingsReq)
	if settingsRec.Code != http.StatusOK {
		t.Fatalf("expected settings endpoint to be allowed during gate, got %d", settingsRec.Code)
	}

	if _, err := mgr.UpdateAppSettings("", "0.5", "1", "none", 3, 30, 15, 20, "adminuser", "strongpass123"); err != nil {
		t.Fatalf("UpdateAppSettings failed: %v", err)
	}

	unblockedReq := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	unblockedReq.AddCookie(sessionCookie)
	unblockedRec := httptest.NewRecorder()
	middleware.ServeHTTP(unblockedRec, unblockedReq)
	if unblockedRec.Code != http.StatusOK {
		t.Fatalf("expected access to be restored after password update, got %d", unblockedRec.Code)
	}
}

func TestTrustedProxyForwardedHeaderUsage(t *testing.T) {
	t.Setenv("ADPANEL_TRUSTED_PROXIES", "127.0.0.1")

	handler := NewAuthHandler(nil, "")

	trustedReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	trustedReq.RemoteAddr = "127.0.0.1:9000"
	trustedReq.Header.Set("X-Forwarded-For", "203.0.113.200, 198.51.100.44")
	trustedReq.Header.Set("X-Forwarded-Proto", "https")
	if got := handler.clientIP(trustedReq); got != "198.51.100.44" {
		t.Fatalf("expected forwarded client IP for trusted proxy, got %q", got)
	}
	if !handler.isSecureRequest(trustedReq) {
		t.Fatalf("expected forwarded proto to mark trusted request as secure")
	}

	untrustedReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	untrustedReq.RemoteAddr = "198.51.100.10:8080"
	untrustedReq.Header.Set("X-Forwarded-For", "203.0.113.99")
	untrustedReq.Header.Set("X-Forwarded-Proto", "https")
	if got := handler.clientIP(untrustedReq); got != "198.51.100.10" {
		t.Fatalf("expected direct remote IP for untrusted source, got %q", got)
	}
	if handler.isSecureRequest(untrustedReq) {
		t.Fatalf("expected untrusted forwarded proto to be ignored")
	}
}

func TestCSRFMiddlewareRejectsCrossOriginUnsafeRequest(t *testing.T) {
	base := t.TempDir()
	mgr, err := minecraft.NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	handler := NewAuthHandler(mgr, base)
	if _, err := mgr.UpdateAppSettings("", "0.5", "1", "none", 3, 30, 15, 20, "adminuser", "strongpass123"); err != nil {
		t.Fatalf("UpdateAppSettings failed: %v", err)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"adminuser","password":"strongpass123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.Login(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d", loginRec.Code)
	}
	sessionCookie := loginRec.Result().Cookies()[0]

	middleware := handler.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/servers/test/settings", strings.NewReader(`{}`))
	req.AddCookie(sessionCookie)
	req.Host = "panel.example.com"
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin unsafe request, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode csrf response: %v", err)
	}
	if body["error"] != "csrf_origin_mismatch" {
		t.Fatalf("unexpected csrf error payload: %v", body)
	}
}
