package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"minecraft-admin/minecraft"
)

// CreateServerRequest is the expected JSON body for POST /api/servers
type CreateServerRequest struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Version        string `json:"version"`
	Port           int    `json:"port"`
	MinRAM         string `json:"minRam"`
	MaxRAM         string `json:"maxRam"`
	MaxPlayers     int    `json:"maxPlayers"`
	Flags          string `json:"flags"`
	AlwaysPreTouch bool   `json:"alwaysPreTouch"`
}

// ServerHandler handles all server REST endpoints
type ServerHandler struct {
	mgr            *minecraft.Manager
	uploadMaxBytes int64
	importMaxBytes int64
}

// NewServerHandler creates a new ServerHandler
func NewServerHandler(mgr *minecraft.Manager) *ServerHandler {
	return &ServerHandler{
		mgr:            mgr,
		uploadMaxBytes: uploadMaxBytesFromEnv(),
		importMaxBytes: serverImportMaxBytesFromEnv(),
	}
}

// List handles GET /api/servers
func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	servers := h.mgr.ListServers()
	respondJSON(w, http.StatusOK, servers)
}

// Reorder handles PUT /api/servers/order
func (h *ServerHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderedIDs []string `json:"orderedIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.OrderedIDs) == 0 {
		respondError(w, http.StatusBadRequest, "orderedIds is required")
		return
	}

	if err := h.mgr.SetServerOrder(req.OrderedIDs); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Create handles POST /api/servers
func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "Server name is required")
		return
	}
	if req.Type == "" {
		respondError(w, http.StatusBadRequest, "Server type is required")
		return
	}
	if _, err := minecraft.GetProvider(req.Type); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Version == "" {
		req.Version = "Latest"
	}
	if req.Port == 0 {
		req.Port = 25565
	}
	if req.Port < 1024 || req.Port > 65535 {
		respondError(w, http.StatusBadRequest, "Port must be between 1024 and 65535")
		return
	}
	if req.MinRAM == "" {
		req.MinRAM = "512M"
	}
	if req.MaxRAM == "" {
		req.MaxRAM = "1024M"
	}
	if req.MaxPlayers <= 0 {
		req.MaxPlayers = 20
	}

	server, err := h.mgr.CreateServer(req.Name, req.Type, req.Version, req.Port, req.MinRAM, req.MaxRAM, req.MaxPlayers, req.Flags, req.AlwaysPreTouch)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, server)
}

// Start handles POST /api/servers/{id}/start
func (h *ServerHandler) Start(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	if err := h.mgr.StartServer(id); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	status, err := h.mgr.GetStatus(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// StartSafeMode handles POST /api/servers/{id}/start-safe
func (h *ServerHandler) StartSafeMode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	if err := h.mgr.StartServerSafeMode(id); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	status, err := h.mgr.GetStatus(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// Stop handles POST /api/servers/{id}/stop
func (h *ServerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	if err := h.mgr.StopServer(id); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	status, err := h.mgr.GetStatus(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// Status handles GET /api/servers/{id}/status
func (h *ServerHandler) Status(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	status, err := h.mgr.GetStatus(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// ScheduleRestart handles POST /api/servers/{id}/schedule-restart
func (h *ServerHandler) ScheduleRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		DelaySeconds int `json:"delaySeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.DelaySeconds < 0 {
		respondError(w, http.StatusBadRequest, "delaySeconds must be zero or positive")
		return
	}

	if err := h.mgr.ScheduleRestart(id, req.DelaySeconds); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "scheduled"})
}

// CancelRestart handles DELETE /api/servers/{id}/schedule-restart
func (h *ServerHandler) CancelRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.mgr.CancelRestart(id); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ScheduleStop handles POST /api/servers/{id}/schedule-stop
func (h *ServerHandler) ScheduleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		DelaySeconds int `json:"delaySeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.DelaySeconds <= 0 {
		respondError(w, http.StatusBadRequest, "delaySeconds must be positive")
		return
	}

	if err := h.mgr.ScheduleStop(id, req.DelaySeconds); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "scheduled"})
}

// Clone handles POST /api/servers/clone
func (h *ServerHandler) Clone(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID    string `json:"sourceId"`
		Name        string `json:"name"`
		Port        int    `json:"port"`
		CopyPlugins bool   `json:"copyPlugins"`
		CopyWorlds  bool   `json:"copyWorlds"`
		CopyConfig  bool   `json:"copyConfig"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.SourceID == "" || req.Name == "" {
		respondError(w, http.StatusBadRequest, "sourceId and name are required")
		return
	}
	if req.Port == 0 {
		req.Port = 25565
	}

	server, err := h.mgr.CloneServer(req.SourceID, req.Name, req.Port, req.CopyPlugins, req.CopyWorlds, req.CopyConfig)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, server)
}

// AnalyzeImport handles POST /api/servers/import/analyze (multipart form)
func (h *ServerHandler) AnalyzeImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.importMaxBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if isRequestBodyTooLarge(err) {
			respondError(w, http.StatusRequestEntityTooLarge, "uploaded file exceeds maximum allowed size")
			return
		}
		respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	result, err := h.mgr.AnalyzeServerImportArchive(header.Filename, file)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// CommitImport handles POST /api/servers/import/commit
func (h *ServerHandler) CommitImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AnalysisID   string  `json:"analysisId"`
		TypeOverride string  `json:"typeOverride"`
		Name         *string `json:"name"`
		Port         *int    `json:"port"`
		Version      *string `json:"version"`
		Properties   *struct {
			MaxPlayers *int    `json:"maxPlayers"`
			Motd       *string `json:"motd"`
			WhiteList  *bool   `json:"whiteList"`
			OnlineMode *bool   `json:"onlineMode"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if strings.TrimSpace(req.AnalysisID) == "" {
		respondError(w, http.StatusBadRequest, "analysisId is required")
		return
	}

	opts := minecraft.ServerImportCommitOptions{
		Name:         req.Name,
		Port:         req.Port,
		TypeOverride: req.TypeOverride,
		Version:      req.Version,
	}
	if req.Properties != nil {
		opts.MaxPlayers = req.Properties.MaxPlayers
		opts.Motd = req.Properties.Motd
		opts.WhiteList = req.Properties.WhiteList
		opts.OnlineMode = req.Properties.OnlineMode
	}

	server, err := h.mgr.CommitServerImport(req.AnalysisID, opts)
	if err != nil {
		var portErr *minecraft.ImportPortConflictError
		if errors.As(err, &portErr) {
			respondJSON(w, http.StatusBadRequest, map[string]any{
				"error":         "port_in_use",
				"message":       "That port is already in use.",
				"suggestedPort": portErr.SuggestedPort,
			})
			return
		}
		var versionErr *minecraft.ImportInvalidVersionError
		if errors.As(err, &versionErr) {
			respondJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "invalid_server_version",
				"message": versionErr.Error(),
			})
			return
		}
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, server)
}

// CancelImport handles DELETE /api/servers/import/analyze/{id}
func (h *ServerHandler) CancelImport(w http.ResponseWriter, r *http.Request) {
	analysisID := r.PathValue("id")
	if strings.TrimSpace(analysisID) == "" {
		respondError(w, http.StatusBadRequest, "analysis id is required")
		return
	}

	if err := h.mgr.CancelServerImportAnalysis(analysisID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// RetryInstall handles POST /api/servers/{id}/retry-install
func (h *ServerHandler) RetryInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.mgr.RetryInstall(id); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := h.mgr.GetStatus(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, status)
}

// UpdateVersion handles PUT /api/servers/{id}/version
func (h *ServerHandler) UpdateVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Version == "" {
		respondError(w, http.StatusBadRequest, "version is required")
		return
	}

	server, err := h.mgr.UpdateVersion(id, req.Version)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, server)
}

// UpdateSettings handles PUT /api/servers/{id}/settings
func (h *ServerHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		MinRAM     string `json:"minRam"`
		MaxRAM     string `json:"maxRam"`
		MaxPlayers int    `json:"maxPlayers"`
		Port       int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.MinRAM == "" || req.MaxRAM == "" || req.MaxPlayers <= 0 || req.Port == 0 {
		respondError(w, http.StatusBadRequest, "minRam, maxRam, maxPlayers, and port are required")
		return
	}

	server, err := h.mgr.UpdateSettings(id, req.MinRAM, req.MaxRAM, req.MaxPlayers, req.Port)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, server)
}

// SetFlags handles PUT /api/servers/{id}/flags
func (h *ServerHandler) SetFlags(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Flags          string `json:"flags"`
		AlwaysPreTouch bool   `json:"alwaysPreTouch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	server, err := h.mgr.SetFlags(id, req.Flags, req.AlwaysPreTouch)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, server)
}

// SetAutoStart handles PUT /api/servers/{id}/auto-start
func (h *ServerHandler) SetAutoStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		AutoStart bool `json:"autoStart"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	server, err := h.mgr.SetAutoStart(id, req.AutoStart)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, server)
}

// Rename handles PUT /api/servers/{id}/name
func (h *ServerHandler) Rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	server, err := h.mgr.RenameServer(id, req.Name)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, server)
}

// Delete handles DELETE /api/servers/{id}
func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	if err := h.mgr.DeleteServer(id); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// respondJSON writes a JSON response with the given status code
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes a JSON error response
func respondError(w http.ResponseWriter, status int, message string) {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "server_config_path_unsafe:") {
		detail := strings.TrimSpace(strings.TrimPrefix(trimmed, "server_config_path_unsafe:"))
		if detail == "" {
			detail = "Server path is outside managed directories."
		}
		respondJSON(w, status, map[string]string{
			"error":   "server_config_path_unsafe",
			"message": detail,
		})
		return
	}
	respondJSON(w, status, map[string]string{"error": message})
}
