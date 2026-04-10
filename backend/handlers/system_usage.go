package handlers

import (
	"net/http"

	"minecraft-admin/minecraft"
)

type SystemUsageHandler struct {
	mgr *minecraft.Manager
}

func NewSystemUsageHandler(mgr *minecraft.Manager) *SystemUsageHandler {
	return &SystemUsageHandler{mgr: mgr}
}

func (h *SystemUsageHandler) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, h.mgr.GetSystemUsage())
}
