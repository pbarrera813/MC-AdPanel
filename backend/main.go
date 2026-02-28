package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"minecraft-admin/handlers"
	"minecraft-admin/minecraft"
)

func main() {
	// Base directory for AdPanel — configurable via env var, defaults to /AdPanel
	baseDir := os.Getenv("ADPANEL_DIR")
	if baseDir == "" {
		baseDir = "/AdPanel"
	}

	// Static files directory for the React SPA
	distDir := filepath.Join(baseDir, "dist")
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		// Fallback: look relative to working directory (for development)
		wd, _ := os.Getwd()
		distDir = filepath.Join(wd, "..", "dist")
		if _, err := os.Stat(distDir); os.IsNotExist(err) {
			distDir = filepath.Join(wd, "dist")
		}
	}

	log.Printf("Base directory: %s", baseDir)
	log.Printf("Static files: %s", distDir)
	log.Printf("Running startup self-checks...")
	if err := runStartupChecks(baseDir, distDir); err != nil {
		log.Fatalf("Startup self-check failed: %v", err)
	}
	log.Printf("Startup self-checks passed.")

	// Initialize the Minecraft process manager
	mgr, err := minecraft.NewManager(baseDir)
	if err != nil {
		log.Fatalf("Failed to initialize manager: %v", err)
	}
	defer mgr.StopAll()

	// Create handlers
	serverHandler := handlers.NewServerHandler(mgr)
	mcHandler := handlers.NewMinecraftHandler(mgr)
	pluginHandler := handlers.NewPluginHandler(mgr)
	backupHandler := handlers.NewBackupHandler(mgr)
	fileHandler := handlers.NewFileHandler(mgr)
	playerHandler := handlers.NewPlayerHandler(mgr)
	crashHandler := handlers.NewCrashReportHandler(mgr)
	logHandler := handlers.NewLogHandler(mgr)
	versionHandler := handlers.NewVersionHandler(mgr)
	settingsHandler := handlers.NewSettingsHandler(mgr)
	authHandler := handlers.NewAuthHandler(mgr, baseDir)

	// Set up router using Go 1.22+ ServeMux
	mux := http.NewServeMux()
	startedAt := time.Now()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]any{
			"status":        "ok",
			"service":       "orexa-panel",
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
			"uptimeSeconds": int(time.Since(startedAt).Seconds()),
		})
	})
	mux.HandleFunc("GET /api/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := runReadinessChecks(baseDir, distDir, mgr); err != nil {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":    "not_ready",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
				"error":     err.Error(),
			})
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"status":    "ready",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Server CRUD & lifecycle
	mux.HandleFunc("GET /api/servers", serverHandler.List)
	mux.HandleFunc("POST /api/servers", serverHandler.Create)
	mux.HandleFunc("POST /api/servers/{id}/start", serverHandler.Start)
	mux.HandleFunc("POST /api/servers/{id}/start-safe", serverHandler.StartSafeMode)
	mux.HandleFunc("POST /api/servers/{id}/stop", serverHandler.Stop)
	mux.HandleFunc("GET /api/servers/{id}/status", serverHandler.Status)
	mux.HandleFunc("POST /api/servers/{id}/schedule-restart", serverHandler.ScheduleRestart)
	mux.HandleFunc("DELETE /api/servers/{id}/schedule-restart", serverHandler.CancelRestart)
	mux.HandleFunc("POST /api/servers/{id}/retry-install", serverHandler.RetryInstall)
	mux.HandleFunc("PUT /api/servers/{id}/version", serverHandler.UpdateVersion)
	mux.HandleFunc("PUT /api/servers/{id}/settings", serverHandler.UpdateSettings)
	mux.HandleFunc("PUT /api/servers/{id}/auto-start", serverHandler.SetAutoStart)
	mux.HandleFunc("PUT /api/servers/{id}/flags", serverHandler.SetFlags)
	mux.HandleFunc("PUT /api/servers/{id}/name", serverHandler.Rename)
	mux.HandleFunc("DELETE /api/servers/{id}", serverHandler.Delete)
	mux.HandleFunc("POST /api/servers/clone", serverHandler.Clone)

	// Version fetching
	mux.HandleFunc("GET /api/versions/{type}", versionHandler.List)

	// System settings
	mux.HandleFunc("GET /api/settings", settingsHandler.Get)
	mux.HandleFunc("PUT /api/settings", settingsHandler.Update)

	// Authentication
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.Logout)
	mux.HandleFunc("GET /api/auth/session", authHandler.Session)

	// Crash reports
	mux.HandleFunc("GET /api/servers/{id}/crash-reports", crashHandler.List)
	mux.HandleFunc("GET /api/servers/{id}/crash-reports/{name}", crashHandler.Read)
	mux.HandleFunc("POST /api/servers/{id}/crash-reports/{name}/copy", crashHandler.Copy)
	mux.HandleFunc("DELETE /api/servers/{id}/crash-reports/{name}", crashHandler.Delete)

	// WebSocket route for console logs (live streaming)
	mux.Handle("GET /api/logs/{id}", mcHandler.WebSocketLogs())

	// HTTP routes to list/read saved log files when server is offline
	mux.HandleFunc("GET /api/servers/{id}/logs", logHandler.List)
	mux.HandleFunc("GET /api/servers/{id}/logs/{name}", logHandler.Read)

	// Plugin management
	mux.HandleFunc("GET /api/servers/{id}/plugins", pluginHandler.List)
	mux.HandleFunc("POST /api/servers/{id}/plugins", pluginHandler.Upload)
	mux.HandleFunc("DELETE /api/servers/{id}/plugins/{name}", pluginHandler.Delete)
	mux.HandleFunc("PUT /api/servers/{id}/plugins/{name}/toggle", pluginHandler.Toggle)
	mux.HandleFunc("PUT /api/servers/{id}/plugins/{name}/source", pluginHandler.SetSource)
	mux.HandleFunc("GET /api/servers/{id}/plugins/check-updates", pluginHandler.CheckUpdates)
	mux.HandleFunc("POST /api/servers/{id}/plugins/{name}/update", pluginHandler.Update)

	// Backup management
	mux.HandleFunc("GET /api/servers/{id}/backups", backupHandler.List)
	mux.HandleFunc("POST /api/servers/{id}/backups", backupHandler.Create)
	mux.HandleFunc("DELETE /api/servers/{id}/backups/{name}", backupHandler.Delete)
	mux.HandleFunc("GET /api/servers/{id}/backups/{name}/download", backupHandler.Download)
	mux.HandleFunc("POST /api/servers/{id}/backups/{name}/restore", backupHandler.Restore)
	mux.HandleFunc("GET /api/servers/{id}/backup-schedule", backupHandler.GetSchedule)
	mux.HandleFunc("PUT /api/servers/{id}/backup-schedule", backupHandler.SetSchedule)

	// File browser
	mux.HandleFunc("GET /api/servers/{id}/files", fileHandler.List)
	mux.HandleFunc("GET /api/servers/{id}/files/exists", fileHandler.Exists)
	mux.HandleFunc("GET /api/servers/{id}/files/content", fileHandler.ReadContent)
	mux.HandleFunc("PUT /api/servers/{id}/files/content", fileHandler.WriteContent)
	mux.HandleFunc("POST /api/servers/{id}/files/upload", fileHandler.Upload)
	mux.HandleFunc("DELETE /api/servers/{id}/files", fileHandler.Delete)
	mux.HandleFunc("POST /api/servers/{id}/files/mkdir", fileHandler.MkDir)
	mux.HandleFunc("PUT /api/servers/{id}/files/rename", fileHandler.Rename)
	mux.HandleFunc("POST /api/servers/{id}/files/download", fileHandler.Download)

	// Player management
	mux.HandleFunc("GET /api/servers/{id}/players", playerHandler.List)
	mux.HandleFunc("POST /api/servers/{id}/players/{name}/kick", playerHandler.Kick)
	mux.HandleFunc("POST /api/servers/{id}/players/{name}/ban", playerHandler.Ban)
	mux.HandleFunc("POST /api/servers/{id}/players/{name}/kill", playerHandler.Kill)

	// Serve static files (React SPA)
	mux.Handle("/", spaHandler(distDir))

	// Wrap with CORS middleware
	handler := corsMiddleware(authHandler.Middleware(mux))

	log.Println("=== Orexa Panel ===")
	log.Printf("Servers directory: %s", filepath.Join(baseDir, "Servers"))
	log.Println("Server running on http://localhost:4010")
	log.Fatal(http.ListenAndServe(":4010", handler))
}

// spaHandler serves static files from distDir, falling back to index.html for client-side routes
func spaHandler(distDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(distDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(distDir, filepath.Clean(r.URL.Path))

		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for development (Vite dev server on different port)
func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := parseAllowedOrigins(os.Getenv("ADPANEL_ALLOWED_ORIGINS"))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowed := ""
		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				allowed = origin
			}
		}

		if allowed != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == "OPTIONS" {
			if origin != "" && allowed == "" {
				http.Error(w, "CORS origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func parseAllowedOrigins(raw string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		out[origin] = struct{}{}
	}
	return out
}

func runStartupChecks(baseDir, distDir string) error {
	requiredDirs := []struct {
		name string
		path string
	}{
		{name: "servers", path: filepath.Join(baseDir, "Servers")},
		{name: "data", path: filepath.Join(baseDir, "data")},
		{name: "backups", path: filepath.Join(baseDir, "Backups")},
	}

	for _, d := range requiredDirs {
		if err := os.MkdirAll(d.path, 0755); err != nil {
			return err
		}
		if err := requireDirectory(d.path); err != nil {
			return err
		}
		if err := checkDirectoryWritable(d.path); err != nil {
			return err
		}
		log.Printf("Self-check ok: %s directory is ready (%s)", d.name, d.path)
	}

	distIndex := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(distIndex); err != nil {
		return err
	}
	log.Printf("Self-check ok: frontend assets detected (%s)", distIndex)
	return nil
}

func runReadinessChecks(baseDir, distDir string, mgr *minecraft.Manager) error {
	if mgr == nil {
		return &checkError{message: "manager is not initialized"}
	}

	requiredDirs := []string{
		filepath.Join(baseDir, "Servers"),
		filepath.Join(baseDir, "data"),
		filepath.Join(baseDir, "Backups"),
	}
	for _, p := range requiredDirs {
		if err := requireDirectory(p); err != nil {
			return err
		}
	}

	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return err
	}

	_ = mgr.ListServers()
	return nil
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &checkError{message: "path is not a directory: " + path}
	}
	return nil
}

func checkDirectoryWritable(path string) error {
	f, err := os.CreateTemp(path, ".selfcheck-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(name)
		return cerr
	}
	return os.Remove(name)
}

type checkError struct {
	message string
}

func (e *checkError) Error() string {
	return e.message
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
