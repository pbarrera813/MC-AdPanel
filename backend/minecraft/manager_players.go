package minecraft

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (m *Manager) listPlayersSnapshot(id string) ([]PlayerInfo, bool, time.Time, error) {
	m.mu.RLock()
	if _, err := m.serverConfigForOperationLocked(id); err != nil {
		m.mu.RUnlock()
		return nil, false, time.Time{}, err
	}
	rs, ok := m.running[id]
	m.mu.RUnlock()
	if !ok {
		return nil, false, time.Time{}, fmt.Errorf("server %s not found", id)
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.status != "Running" {
		return []PlayerInfo{}, false, time.Time{}, nil
	}
	lastSync := rs.lastPlayersSync

	players := make([]PlayerInfo, 0)
	for _, p := range rs.players {
		duration := time.Since(p.JoinedAt)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60

		var onlineTime string
		if hours > 0 {
			onlineTime = fmt.Sprintf("%dh %dm", hours, minutes)
		} else {
			onlineTime = fmt.Sprintf("%dm", minutes)
		}

		players = append(players, PlayerInfo{
			Name:       p.Name,
			IP:         p.IP,
			Ping:       p.Ping,
			World:      p.World,
			OnlineTime: onlineTime,
		})
	}

	sort.Slice(players, func(i, j int) bool {
		return players[i].Name < players[j].Name
	})

	pollCfg := m.currentPollIntervals()
	staleThreshold := time.Duration((pollCfg.playerSyncSeconds*2)+5) * time.Second
	isStale := len(players) > 0 && (lastSync.IsZero() || time.Since(lastSync) > staleThreshold)

	return players, isStale, lastSync, nil
}

// ListPlayers returns currently online players tracked from log parsing
func (m *Manager) ListPlayers(id string) ([]PlayerInfo, error) {
	players, _, _, err := m.listPlayersSnapshot(id)
	return players, err
}

// ListPlayersWithFreshness returns players plus freshness metadata for UI hints.
func (m *Manager) ListPlayersWithFreshness(id string) ([]PlayerInfo, bool, time.Time, error) {
	return m.listPlayersSnapshot(id)
}

func (m *Manager) GetPingSupport(id string) (bool, string, error) {
	m.mu.RLock()
	cfg, err := m.serverConfigForOperationLocked(id)
	rs, rsOk := m.running[id]
	m.mu.RUnlock()
	if err != nil {
		return false, "", err
	}

	if rsOk {
		rs.mu.RLock()
		supported := rs.pingSupported
		reason := rs.pingDisabledReason
		rs.mu.RUnlock()
		if supported || reason != "" {
			return supported, reason, nil
		}
	}

	if isModdedType(cfg.Type) {
		modsDir := filepath.Join(cfg.Dir, "mods")
		if !hasPingPlayerMod(modsDir) {
			return false, "missing_pingplayer_mod", nil
		}
		return true, "", nil
	}
	if strings.EqualFold(cfg.Type, "vanilla") {
		return false, "unsupported_server_type", nil
	}

	pluginsDir := filepath.Join(cfg.Dir, "plugins")
	if !hasPingPlayer(pluginsDir) {
		return false, "missing_pingplayer", nil
	}

	return true, "", nil
}

// KickPlayer sends a kick command to the server
func (m *Manager) KickPlayer(id, playerName, reason string) error {
	if reason == "" {
		return m.SendCommand(id, fmt.Sprintf("kick %s", playerName))
	}
	return m.SendCommand(id, fmt.Sprintf("kick %s %s", playerName, reason))
}

// BanPlayer sends a ban command to the server
func (m *Manager) BanPlayer(id, playerName, reason string) error {
	if reason == "" {
		return m.SendCommand(id, fmt.Sprintf("ban %s", playerName))
	}
	return m.SendCommand(id, fmt.Sprintf("ban %s %s", playerName, reason))
}

// KillPlayer sends a kill command to the server
func (m *Manager) KillPlayer(id, playerName string) error {
	return m.SendCommand(id, fmt.Sprintf("kill %s", playerName))
}
