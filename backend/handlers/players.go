package handlers

import (
	"encoding/json"
	"net/http"

	"minecraft-admin/minecraft"
)

// PlayerHandler handles player-related REST endpoints
type PlayerHandler struct {
	mgr *minecraft.Manager
}

type PlayersResponse struct {
	Players       []minecraft.PlayerInfo `json:"players"`
	PingSupported bool                   `json:"pingSupported"`
	PingStatus    string                 `json:"pingStatus,omitempty"` // missing_pingplayer | modded
}

// NewPlayerHandler creates a new PlayerHandler
func NewPlayerHandler(mgr *minecraft.Manager) *PlayerHandler {
	return &PlayerHandler{mgr: mgr}
}

// List handles GET /api/servers/{id}/players
func (h *PlayerHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	players, err := h.mgr.ListPlayers(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	pingSupported, pingStatus, err := h.mgr.GetPingSupport(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, PlayersResponse{
		Players:       players,
		PingSupported: pingSupported,
		PingStatus:    pingStatus,
	})
}

// Kick handles POST /api/servers/{id}/players/{name}/kick
func (h *PlayerHandler) Kick(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.mgr.KickPlayer(id, name, req.Reason); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "kicked", "player": name})
}

// Ban handles POST /api/servers/{id}/players/{name}/ban
func (h *PlayerHandler) Ban(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.mgr.BanPlayer(id, name, req.Reason); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "banned", "player": name})
}

// Kill handles POST /api/servers/{id}/players/{name}/kill
func (h *PlayerHandler) Kill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	if err := h.mgr.KillPlayer(id, name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "killed", "player": name})
}
