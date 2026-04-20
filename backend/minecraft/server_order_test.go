package minecraft

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerOrderMigrationAppliesAlphabeticalDefaults(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "data")
	serversDir := filepath.Join(base, "Servers")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}
	if err := os.MkdirAll(serversDir, 0755); err != nil {
		t.Fatalf("failed to create servers dir: %v", err)
	}

	cfgs := []*ServerConfig{
		{
			ID:         "srv1",
			Name:       "Zulu",
			Type:       "Vanilla",
			Version:    "1.21.10",
			Port:       25565,
			JarFile:    "server.jar",
			MinRAM:     "512M",
			MaxRAM:     "1024M",
			MaxPlayers: 20,
			Dir:        filepath.Join(serversDir, "zulu"),
		},
		{
			ID:         "srv2",
			Name:       "alpha",
			Type:       "Vanilla",
			Version:    "1.21.10",
			Port:       25566,
			JarFile:    "server.jar",
			MinRAM:     "512M",
			MaxRAM:     "1024M",
			MaxPlayers: 20,
			Dir:        filepath.Join(serversDir, "alpha"),
		},
		{
			ID:         "srv3",
			Name:       "Bravo",
			Type:       "Vanilla",
			Version:    "1.21.10",
			Port:       25567,
			JarFile:    "server.jar",
			MinRAM:     "512M",
			MaxRAM:     "1024M",
			MaxPlayers: 20,
			Dir:        filepath.Join(serversDir, "bravo"),
		},
	}
	for _, cfg := range cfgs {
		if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
			t.Fatalf("failed to create server dir: %v", err)
		}
	}
	raw, err := json.Marshal(cfgs)
	if err != nil {
		t.Fatalf("failed to marshal configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "servers.json"), raw, 0644); err != nil {
		t.Fatalf("failed to write servers.json: %v", err)
	}

	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	list := mgr.ListServers()
	if len(list) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(list))
	}
	gotNames := []string{list[0].Name, list[1].Name, list[2].Name}
	wantNames := []string{"alpha", "Bravo", "Zulu"}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Fatalf("expected alphabetical migration order %v, got %v", wantNames, gotNames)
		}
	}

	persistedBytes, err := os.ReadFile(filepath.Join(dataDir, "servers.json"))
	if err != nil {
		t.Fatalf("failed reading persisted servers.json: %v", err)
	}
	var persisted []*ServerConfig
	if err := json.Unmarshal(persistedBytes, &persisted); err != nil {
		t.Fatalf("failed parsing persisted servers.json: %v", err)
	}
	nameToOrder := make(map[string]int, len(persisted))
	for _, cfg := range persisted {
		nameToOrder[cfg.Name] = cfg.Order
	}
	if nameToOrder["alpha"] != 1 || nameToOrder["Bravo"] != 2 || nameToOrder["Zulu"] != 3 {
		t.Fatalf("unexpected persisted order map: %+v", nameToOrder)
	}
}

func TestSetServerOrderValidationAndAppendBehavior(t *testing.T) {
	base := t.TempDir()
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.StopAll()

	makeCfg := func(id, name string, order, port int) *ServerConfig {
		return &ServerConfig{
			ID:         id,
			Name:       name,
			Order:      order,
			Type:       "Vanilla",
			Version:    "1.21.10",
			Port:       port,
			JarFile:    "server.jar",
			MinRAM:     "512M",
			MaxRAM:     "1024M",
			MaxPlayers: 20,
			Dir:        filepath.Join(base, "Servers", sanitizeName(name)+"_"+id),
		}
	}

	mgr.mu.Lock()
	cfg1 := makeCfg("srv1", "One", 1, 25570)
	cfg2 := makeCfg("srv2", "Two", 2, 25571)
	mgr.configs[cfg1.ID] = cfg1
	mgr.configs[cfg2.ID] = cfg2
	mgr.running[cfg1.ID] = &runningServer{status: "Stopped"}
	mgr.running[cfg2.ID] = &runningServer{status: "Stopped"}
	mgr.mu.Unlock()
	if err := os.MkdirAll(cfg1.Dir, 0755); err != nil {
		t.Fatalf("failed to create cfg1 dir: %v", err)
	}
	if err := os.MkdirAll(cfg2.Dir, 0755); err != nil {
		t.Fatalf("failed to create cfg2 dir: %v", err)
	}
	if err := mgr.persist(); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	if err := mgr.SetServerOrder([]string{"srv2", "srv1"}); err != nil {
		t.Fatalf("SetServerOrder valid call failed: %v", err)
	}
	list := mgr.ListServers()
	if len(list) != 2 || list[0].ID != "srv2" || list[1].ID != "srv1" {
		t.Fatalf("expected reordered IDs [srv2 srv1], got [%s %s]", list[0].ID, list[1].ID)
	}

	created, err := mgr.CreateServer("Three", "Vanilla", "1.21.10", 25572, "512M", "1024M", 20, "none", false)
	if err != nil {
		t.Fatalf("CreateServer failed: %v", err)
	}
	mgr.mu.RLock()
	createdCfg := mgr.configs[created.ID]
	mgr.mu.RUnlock()
	if createdCfg == nil {
		t.Fatalf("expected created server config")
	}
	if createdCfg.Order != 3 {
		t.Fatalf("expected appended order 3, got %d", createdCfg.Order)
	}

	if err := mgr.SetServerOrder([]string{"srv1", "srv1", created.ID}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate validation error, got %v", err)
	}
	if err := mgr.SetServerOrder([]string{"srv1"}); err == nil || !strings.Contains(err.Error(), "exactly once") {
		t.Fatalf("expected missing ID validation error, got %v", err)
	}
	if err := mgr.SetServerOrder([]string{"srv1", "ghost", created.ID}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown ID validation error, got %v", err)
	}
}
