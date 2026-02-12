package handlers

import (
	"fmt"
	"net/http"

	"minecraft-admin/minecraft"
)

// CrashReportHandler handles crash report endpoints
type CrashReportHandler struct {
	mgr *minecraft.Manager
}

// NewCrashReportHandler creates a new CrashReportHandler
func NewCrashReportHandler(mgr *minecraft.Manager) *CrashReportHandler {
	return &CrashReportHandler{mgr: mgr}
}

// List handles GET /api/servers/{id}/crash-reports
func (h *CrashReportHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reports, err := h.mgr.ListCrashReports(id)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, reports)
}

// Read handles GET /api/servers/{id}/crash-reports/{name}
func (h *CrashReportHandler) Read(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	content, err := h.mgr.ReadCrashReport(id, name)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// Copy handles POST /api/servers/{id}/crash-reports/{name}/copy
func (h *CrashReportHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	copyName, err := h.mgr.CopyCrashReport(id, name)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "copied", "name": copyName})
}

// Delete handles DELETE /api/servers/{id}/crash-reports/{name}
func (h *CrashReportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	if err := h.mgr.DeleteCrashReport(id, name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
