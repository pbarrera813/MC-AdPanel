package handlers

import (
	"encoding/json"
	"net/http"

	"minecraft-admin/minecraft"
)

// BackupHandler handles backup-related REST endpoints
type BackupHandler struct {
	mgr *minecraft.Manager
}

// NewBackupHandler creates a new BackupHandler
func NewBackupHandler(mgr *minecraft.Manager) *BackupHandler {
	return &BackupHandler{mgr: mgr}
}

// List handles GET /api/servers/{id}/backups
func (h *BackupHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	backups, err := h.mgr.ListBackups(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, backups)
}

// Create handles POST /api/servers/{id}/backups
func (h *BackupHandler) Create(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	backup, err := h.mgr.CreateBackup(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, backup)
}

// Delete handles DELETE /api/servers/{id}/backups/{name}
func (h *BackupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	if err := h.mgr.DeleteBackup(id, name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Download handles GET /api/servers/{id}/backups/{name}/download
func (h *BackupHandler) Download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	backupPath, err := h.mgr.GetBackupPath(id, name)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, backupPath)
}

// Restore handles POST /api/servers/{id}/backups/{name}/restore
func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	if err := h.mgr.RestoreBackup(id, name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// GetSchedule handles GET /api/servers/{id}/backup-schedule
func (h *BackupHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := h.mgr.GetBackupSchedule(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, info)
}

// SetSchedule handles PUT /api/servers/{id}/backup-schedule
func (h *BackupHandler) SetSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Schedule string `json:"schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.mgr.SetBackupSchedule(id, req.Schedule); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, _ := h.mgr.GetBackupSchedule(id)
	respondJSON(w, http.StatusOK, info)
}
