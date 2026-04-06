package handlers

import (
	"encoding/json"
	"net/http"

	"minecraft-admin/minecraft"
)

type SettingsHandler struct {
	mgr *minecraft.Manager
}

func NewSettingsHandler(mgr *minecraft.Manager) *SettingsHandler {
	return &SettingsHandler{mgr: mgr}
}

func (h *SettingsHandler) Get(w http.ResponseWriter, _ *http.Request) {
	settings := h.mgr.GetSettings()
	respondJSON(w, http.StatusOK, map[string]any{
		"userAgent":          settings.UserAgent,
		"defaultMinRam":      settings.DefaultMinRAM,
		"defaultMaxRam":      settings.DefaultMaxRAM,
		"defaultFlags":       settings.DefaultFlags,
		"statusPollInterval": settings.StatusPollInterval,
		"tpsPollInterval":    settings.TpsPollInterval,
		"playerSyncInterval": settings.PlayerSyncInterval,
		"pingPollInterval":   settings.PingPollInterval,
		"loginUser":          settings.LoginUser,
		"passwordMinLength":  minecraft.LoginPasswordMinLength,
		"maxUploadBytes":     uploadMaxBytesFromEnv(),
	})
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserAgent          string `json:"userAgent"`
		DefaultMinRAM      string `json:"defaultMinRam"`
		DefaultMaxRAM      string `json:"defaultMaxRam"`
		DefaultFlags       string `json:"defaultFlags"`
		StatusPollInterval int    `json:"statusPollInterval"`
		TpsPollInterval    int    `json:"tpsPollInterval"`
		PlayerSyncInterval int    `json:"playerSyncInterval"`
		PingPollInterval   int    `json:"pingPollInterval"`
		LoginUser          string `json:"loginUser"`
		LoginPassword      string `json:"loginPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	settings, err := h.mgr.UpdateAppSettings(
		req.UserAgent,
		req.DefaultMinRAM,
		req.DefaultMaxRAM,
		req.DefaultFlags,
		req.StatusPollInterval,
		req.TpsPollInterval,
		req.PlayerSyncInterval,
		req.PingPollInterval,
		req.LoginUser,
		req.LoginPassword,
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"userAgent":          settings.UserAgent,
		"defaultMinRam":      settings.DefaultMinRAM,
		"defaultMaxRam":      settings.DefaultMaxRAM,
		"defaultFlags":       settings.DefaultFlags,
		"statusPollInterval": settings.StatusPollInterval,
		"tpsPollInterval":    settings.TpsPollInterval,
		"playerSyncInterval": settings.PlayerSyncInterval,
		"pingPollInterval":   settings.PingPollInterval,
		"loginUser":          settings.LoginUser,
		"passwordMinLength":  minecraft.LoginPasswordMinLength,
		"maxUploadBytes":     uploadMaxBytesFromEnv(),
	})
}
