package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"minecraft-admin/minecraft"
)

const (
	sessionCookieName = "adpanel_session"
	sessionTTL        = 7 * 24 * time.Hour
)

type sessionRecord struct {
	username string
	expires  time.Time
}

type AuthHandler struct {
	mgr      *minecraft.Manager
	mu       sync.RWMutex
	sessions map[string]sessionRecord
}

func NewAuthHandler(mgr *minecraft.Manager) *AuthHandler {
	return &AuthHandler{
		mgr:      mgr,
		sessions: make(map[string]sessionRecord),
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
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
		respondError(w, http.StatusBadRequest, "Username and password are required")
		return
	}
	if !h.mgr.ValidateLogin(req.Username, req.Password) {
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	token, err := newSessionToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	expires := time.Now().Add(sessionTTL)
	h.mu.Lock()
	h.sessions[token] = sessionRecord{username: req.Username, expires: expires}
	h.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      req.Username,
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
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	respondJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	username, ok := h.usernameFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      username,
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
		if path == "/api/auth/login" || path == "/api/auth/logout" || path == "/api/auth/session" {
			next.ServeHTTP(w, r)
			return
		}

		if _, ok := h.usernameFromRequest(r); !ok {
			respondError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *AuthHandler) usernameFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c == nil || strings.TrimSpace(c.Value) == "" {
		return "", false
	}
	token := c.Value

	h.mu.RLock()
	rec, ok := h.sessions[token]
	h.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(rec.expires) {
		h.mu.Lock()
		delete(h.sessions, token)
		h.mu.Unlock()
		return "", false
	}
	return rec.username, true
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
