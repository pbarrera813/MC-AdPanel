package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minecraft-admin/minecraft"
)

const (
	sessionCookieName = "orexa_session"
	sessionTTL        = 7 * 24 * time.Hour
	loginWindow       = 15 * time.Minute
	loginBlockTime    = 15 * time.Minute
	loginMaxFailures  = 10
)

type sessionRecord struct {
	Username           string    `json:"username"`
	Expires            time.Time `json:"expires"`
	MustChangePassword bool      `json:"mustChangePassword"`
}

type loginAttempt struct {
	Count        int
	WindowStart  time.Time
	BlockedUntil time.Time
}

type AuthHandler struct {
	mgr            *minecraft.Manager
	mu             sync.RWMutex
	sessions       map[string]sessionRecord
	loginAttempts  map[string]loginAttempt
	trustedProxies *trustedProxySet
	csrfMode       string
}

func NewAuthHandler(mgr *minecraft.Manager, baseDir string) *AuthHandler {
	_ = baseDir
	return &AuthHandler{
		mgr:            mgr,
		sessions:       make(map[string]sessionRecord),
		loginAttempts:  make(map[string]loginAttempt),
		trustedProxies: newTrustedProxySetFromEnv(),
		csrfMode:       csrfModeFromEnv(),
	}
}

func (h *AuthHandler) cleanupExpiredSessionsLocked() {
	now := time.Now()
	for token, rec := range h.sessions {
		if now.After(rec.Expires) {
			delete(h.sessions, token)
		}
	}
}

func (h *AuthHandler) clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		return realClientIPFromXFF(r.RemoteAddr, xff, h.trustedProxies)
	}
	if ip := remoteAddrIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (h *AuthHandler) isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !h.trustedProxies.isTrusted(r.RemoteAddr) {
		return false
	}
	protoHeader := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if protoHeader == "" {
		return false
	}
	firstProto := strings.TrimSpace(strings.Split(protoHeader, ",")[0])
	return strings.EqualFold(firstProto, "https")
}

func (h *AuthHandler) loginBlocked(ip string) (bool, time.Duration) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	attempt, ok := h.loginAttempts[ip]
	if !ok {
		return false, 0
	}
	if attempt.BlockedUntil.After(time.Now()) {
		return true, time.Until(attempt.BlockedUntil)
	}
	return false, 0
}

func (h *AuthHandler) noteLoginFailure(ip string) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()

	attempt := h.loginAttempts[ip]
	if attempt.WindowStart.IsZero() || now.Sub(attempt.WindowStart) > loginWindow {
		attempt = loginAttempt{Count: 0, WindowStart: now}
	}
	attempt.Count++
	if attempt.Count >= loginMaxFailures {
		attempt.BlockedUntil = now.Add(loginBlockTime)
	}
	h.loginAttempts[ip] = attempt
}

func (h *AuthHandler) clearLoginFailures(ip string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.loginAttempts, ip)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ip := h.clientIP(r)
	if blocked, wait := h.loginBlocked(ip); blocked {
		seconds := int(wait.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
		respondError(w, http.StatusTooManyRequests, "Too many failed login attempts. Try again later.")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		h.noteLoginFailure(ip)
		respondError(w, http.StatusBadRequest, "Username and password are required")
		return
	}
	if !h.mgr.ValidateLogin(req.Username, req.Password) {
		h.noteLoginFailure(ip)
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	h.clearLoginFailures(ip)
	mustChangePassword := h.mgr.IsUsingDefaultLogin()

	token, err := newSessionToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	expires := time.Now().Add(sessionTTL)
	h.mu.Lock()
	h.sessions[token] = sessionRecord{
		Username:           req.Username,
		Expires:            expires,
		MustChangePassword: mustChangePassword,
	}
	h.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"authenticated":      true,
		"username":           req.Username,
		"mustChangePassword": mustChangePassword,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		h.mu.Lock()
		delete(h.sessions, c.Value)
		h.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	respondJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.sessionFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"authenticated":      true,
		"username":           rec.Username,
		"mustChangePassword": rec.MustChangePassword,
	})
}

func (h *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if path == "/api/auth/login" || path == "/api/auth/logout" || path == "/api/auth/session" || path == "/api/health" || path == "/api/ready" {
			next.ServeHTTP(w, r)
			return
		}

		rec, ok := h.sessionFromRequest(r)
		if !ok {
			respondError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		if rec.MustChangePassword && !h.isPasswordChangeAllowedRoute(path, r.Method) {
			respondJSON(w, http.StatusPreconditionRequired, map[string]string{
				"error":   "password_change_required",
				"message": "Change default credentials before using other API endpoints.",
			})
			return
		}
		if isUnsafeHTTPMethod(r.Method) && !h.isCSRFIgnoredRoute(path) {
			if !requestOriginMatchesCSRF(r, h.trustedProxies) {
				if h.csrfMode == "report" {
					ip := h.clientIP(r)
					log.Printf("CSRF report-only mismatch: method=%s path=%s client_ip=%s origin=%q referer=%q", r.Method, path, ip, r.Header.Get("Origin"), r.Header.Get("Referer"))
				} else if h.csrfMode == "enforce" {
					respondJSON(w, http.StatusForbidden, map[string]string{
						"error":   "csrf_origin_mismatch",
						"message": "Cross-origin request rejected.",
					})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *AuthHandler) isPasswordChangeAllowedRoute(path, method string) bool {
	if path == "/api/auth/logout" || path == "/api/auth/session" || path == "/api/health" || path == "/api/ready" {
		return true
	}
	if path == "/api/settings" && (method == http.MethodPut || method == http.MethodGet) {
		return true
	}
	return false
}

func (h *AuthHandler) isCSRFIgnoredRoute(path string) bool {
	switch path {
	case "/api/auth/login", "/api/auth/logout", "/api/auth/session", "/api/health", "/api/ready":
		return true
	default:
		return false
	}
}

func (h *AuthHandler) usernameFromRequest(r *http.Request) (string, bool) {
	rec, ok := h.sessionFromRequest(r)
	if !ok {
		return "", false
	}
	return rec.Username, true
}

func (h *AuthHandler) sessionFromRequest(r *http.Request) (sessionRecord, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c == nil || strings.TrimSpace(c.Value) == "" {
		return sessionRecord{}, false
	}
	token := c.Value

	h.mu.RLock()
	rec, ok := h.sessions[token]
	h.mu.RUnlock()
	if !ok {
		return sessionRecord{}, false
	}
	if time.Now().After(rec.Expires) {
		h.mu.Lock()
		delete(h.sessions, token)
		h.mu.Unlock()
		return sessionRecord{}, false
	}

	needsChange := h.mgr.IsUsingDefaultLogin()
	if rec.MustChangePassword != needsChange {
		rec.MustChangePassword = needsChange
		h.mu.Lock()
		if latest, exists := h.sessions[token]; exists {
			latest.MustChangePassword = needsChange
			h.sessions[token] = latest
		}
		h.mu.Unlock()
	}
	return rec, true
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
