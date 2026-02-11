package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"minecraft-admin/minecraft"
)

// PluginHandler handles plugin-related REST endpoints
type PluginHandler struct {
	mgr *minecraft.Manager
}

// NewPluginHandler creates a new PluginHandler
func NewPluginHandler(mgr *minecraft.Manager) *PluginHandler {
	return &PluginHandler{mgr: mgr}
}

// List handles GET /api/servers/{id}/plugins
func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plugins, err := h.mgr.ListPlugins(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, plugins)
}

// Upload handles POST /api/servers/{id}/plugins (multipart form)
func (h *PluginHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to read uploaded file")
		return
	}

	if err := h.mgr.UploadPlugin(id, header.Filename, data); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "name": header.Filename})
}

// Delete handles DELETE /api/servers/{id}/plugins/{name}
func (h *PluginHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	if err := h.mgr.DeletePlugin(id, name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Toggle handles PUT /api/servers/{id}/plugins/{name}/toggle
func (h *PluginHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	plugin, err := h.mgr.TogglePlugin(id, name)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, plugin)
}

// CheckUpdates handles GET /api/servers/{id}/plugins/check-updates
func (h *PluginHandler) CheckUpdates(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	results, err := h.mgr.CheckPluginUpdates(id)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, results)
}

// Update handles POST /api/servers/{id}/plugins/{name}/update
func (h *PluginHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.URL == "" {
		respondError(w, http.StatusBadRequest, "Download URL is required")
		return
	}

	plugin, err := h.mgr.UpdatePlugin(id, name, req.URL)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, plugin)
}
