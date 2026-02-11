package handlers

import (
	"net/http"

	"minecraft-admin/minecraft"
)

// VersionHandler handles version-related endpoints
type VersionHandler struct {
	mgr *minecraft.Manager
}

// NewVersionHandler creates a new VersionHandler
func NewVersionHandler(mgr *minecraft.Manager) *VersionHandler {
	return &VersionHandler{mgr: mgr}
}

// List handles GET /api/versions/{type}
func (h *VersionHandler) List(w http.ResponseWriter, r *http.Request) {
	serverType := r.PathValue("type")
	if serverType == "" {
		respondError(w, http.StatusBadRequest, "Server type is required")
		return
	}

	versions, err := h.mgr.GetVersions(serverType)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, versions)
}
