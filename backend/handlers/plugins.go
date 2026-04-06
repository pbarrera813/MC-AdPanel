package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"minecraft-admin/minecraft"
)

// PluginHandler handles plugin-related REST endpoints
type PluginHandler struct {
	mgr            *minecraft.Manager
	uploadMaxBytes int64
}

// NewPluginHandler creates a new PluginHandler
func NewPluginHandler(mgr *minecraft.Manager) *PluginHandler {
	return &PluginHandler{
		mgr:            mgr,
		uploadMaxBytes: uploadMaxBytesFromEnv(),
	}
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

	r.Body = http.MaxBytesReader(w, r.Body, h.uploadMaxBytes)
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

	tmpFile, err := os.CreateTemp("", "orexa-plugin-upload-*"+filepath.Ext(header.Filename))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create temporary upload file")
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := io.Copy(tmpFile, file); err != nil {
		_ = tmpFile.Close()
		respondError(w, http.StatusInternalServerError, "Failed to store uploaded file")
		return
	}
	if err := tmpFile.Close(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to finalize uploaded file")
		return
	}

	conflictAction := strings.ToLower(strings.TrimSpace(r.FormValue("conflictAction")))
	savedName, status, err := h.mgr.UploadPluginFromFile(id, header.Filename, tmpPath, conflictAction)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": "file_exists",
				"name":  header.Filename,
			})
			return
		}
		if errors.Is(err, minecraft.ErrExtensionAlreadyInstalled) {
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": "already_installed",
			})
			return
		}
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": status, "name": savedName})
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

// SetSource handles PUT /api/servers/{id}/plugins/{name}/source
func (h *PluginHandler) SetSource(w http.ResponseWriter, r *http.Request) {
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
		respondError(w, http.StatusBadRequest, "Source URL is required")
		return
	}

	if err := h.mgr.SetPluginSource(id, name, req.URL); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
