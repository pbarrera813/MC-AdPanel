package handlers

import (
	"fmt"
	"net/http"

	"minecraft-admin/minecraft"
)

// LogHandler exposes file-listing / read endpoints for server logs
type LogHandler struct {
	mgr *minecraft.Manager
}

func NewLogHandler(mgr *minecraft.Manager) *LogHandler {
	return &LogHandler{mgr: mgr}
}

// List handles GET /api/servers/{id}/logs
func (h *LogHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	files, err := h.mgr.ListLogFiles(id)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, files)
}

// Read handles GET /api/servers/{id}/logs/{name}
func (h *LogHandler) Read(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	content, err := h.mgr.ReadLogFile(id, name)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}
