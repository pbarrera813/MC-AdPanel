package minecraft

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyPlayerSampleLocked_AttachesAndKeepsUUID(t *testing.T) {
	now := time.Now()
	rs := &runningServer{
		players: map[string]*onlinePlayer{
			"Alice": {
				Name:     "Alice",
				Ping:     -1,
				JoinedAt: now.Add(-time.Minute),
			},
		},
		pingBlocked: make(map[string]bool),
	}

	applyPlayerSampleLocked(rs, []statusSamplePlayer{
		{Name: "Alice", UUID: "069a79f4-44e9-4726-a5be-fca90e38aaf5"},
		{Name: "Bob", UUID: "853c80ef3c3749fdaa49938b674adae6"},
	}, 2, now)

	if got := rs.players["Alice"].UUID; got != "069a79f4-44e9-4726-a5be-fca90e38aaf5" {
		t.Fatalf("expected Alice UUID from sample, got %q", got)
	}
	if got := rs.players["Bob"].UUID; got != "853c80ef-3c37-49fd-aa49-938b674adae6" {
		t.Fatalf("expected Bob UUID normalized from sample, got %q", got)
	}

	applyPlayerSampleLocked(rs, []statusSamplePlayer{
		{Name: "Alice"},
		{Name: "Bob"},
	}, 2, now.Add(10*time.Second))

	if got := rs.players["Alice"].UUID; got != "069a79f4-44e9-4726-a5be-fca90e38aaf5" {
		t.Fatalf("expected Alice UUID to be preserved when sample UUID missing, got %q", got)
	}
	if got := rs.players["Bob"].UUID; got != "853c80ef-3c37-49fd-aa49-938b674adae6" {
		t.Fatalf("expected Bob UUID to be preserved when sample UUID missing, got %q", got)
	}
}

func TestEnrichPlayersWithUserCache_FillsMissingUUIDs(t *testing.T) {
	serverDir := t.TempDir()
	cachePath := filepath.Join(serverDir, "usercache.json")
	cacheData := `[
		{"name":"Steve","uuid":"069a79f444e94726a5befca90e38aaf5"},
		{"name":"Alex","uuid":"not-a-uuid"}
	]`
	if err := os.WriteFile(cachePath, []byte(cacheData), 0o644); err != nil {
		t.Fatalf("failed to write usercache: %v", err)
	}

	players := []PlayerInfo{
		{Name: "Steve"},
		{Name: "Alex"},
		{Name: "Herobrine", UUID: "853c80ef-3c37-49fd-aa49-938b674adae6"},
	}

	enrichPlayersWithUserCache(players, serverDir)

	if players[0].UUID != "069a79f4-44e9-4726-a5be-fca90e38aaf5" {
		t.Fatalf("expected Steve UUID to be enriched from usercache, got %q", players[0].UUID)
	}
	if players[1].UUID != "" {
		t.Fatalf("expected invalid usercache UUID to be ignored, got %q", players[1].UUID)
	}
	if players[2].UUID != "853c80ef-3c37-49fd-aa49-938b674adae6" {
		t.Fatalf("expected existing UUID to remain unchanged, got %q", players[2].UUID)
	}
}

func TestNormalizePlayerUUID(t *testing.T) {
	if got := normalizePlayerUUID("853c80ef3c3749fdaa49938b674adae6"); got != "853c80ef-3c37-49fd-aa49-938b674adae6" {
		t.Fatalf("expected normalized UUID, got %q", got)
	}
	if got := normalizePlayerUUID("not-a-uuid"); got != "" {
		t.Fatalf("expected invalid UUID to return empty string, got %q", got)
	}
}
