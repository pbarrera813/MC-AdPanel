package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

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
	versionHandler := handlers.NewVersionHandler(mgr)
	settingsHandler := handlers.NewSettingsHandler(mgr)

	// Set up router using Go 1.22+ ServeMux
	mux := http.NewServeMux()

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

	// Crash reports
	mux.HandleFunc("GET /api/servers/{id}/crash-reports", crashHandler.List)
	mux.HandleFunc("GET /api/servers/{id}/crash-reports/{name}", crashHandler.Read)
	mux.HandleFunc("DELETE /api/servers/{id}/crash-reports/{name}", crashHandler.Delete)

	// WebSocket route for console logs
	mux.Handle("GET /api/logs/{id}", mcHandler.WebSocketLogs())

	// Plugin management
	mux.HandleFunc("GET /api/servers/{id}/plugins", pluginHandler.List)
	mux.HandleFunc("POST /api/servers/{id}/plugins", pluginHandler.Upload)
	mux.HandleFunc("DELETE /api/servers/{id}/plugins/{name}", pluginHandler.Delete)
	mux.HandleFunc("PUT /api/servers/{id}/plugins/{name}/toggle", pluginHandler.Toggle)
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
	handler := corsMiddleware(mux)

	log.Println("=== Minecraft Admin Panel ===")
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
