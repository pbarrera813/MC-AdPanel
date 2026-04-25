package minecraft

import (
	"log"
	"math"
	"os"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

func bytesToMB(value uint64) float64 {
	return float64(value) / 1024 / 1024
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func (m *Manager) hostCPUSharePercent(rawPercent float64) float64 {
	if math.IsNaN(rawPercent) || math.IsInf(rawPercent, 0) || rawPercent < 0 {
		return 0
	}
	divisor := float64(m.hostLogicalCPUs)
	if divisor <= 0 {
		divisor = 1
	}
	return clampPercent(rawPercent / divisor)
}

func (m *Manager) hostRAMSharePercent(ramBytes uint64) float64 {
	if m.hostTotalRAMBytes == 0 {
		return 0
	}
	return clampPercent((float64(ramBytes) / float64(m.hostTotalRAMBytes)) * 100)
}

func (m *Manager) loadHostUsageMetadata() {
	m.hostLogicalCPUs = 1
	if count, err := cpu.Counts(true); err == nil && count > 0 {
		m.hostLogicalCPUs = count
	}
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		m.hostTotalRAMBytes = vm.Total
	}
}

type usageServerTarget struct {
	ID     string
	Name   string
	Type   string
	Status string
	PID    int
	RS     *runningServer
}

func (m *Manager) runUsageSampler() {
	const sampleInterval = 2 * time.Second
	const summaryInterval = 60 * time.Second

	log.Printf("Usage sampler started (interval=%s, logical_cpus=%d, total_ram_bytes=%d)", sampleInterval, m.hostLogicalCPUs, m.hostTotalRAMBytes)

	panelPID := os.Getpid()
	knownProcesses := make(map[int]*process.Process)
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	lastSummary := time.Time{}

	sampleProcess := func(pid int) (float64, uint64, bool) {
		if pid <= 0 {
			return 0, 0, false
		}
		proc := knownProcesses[pid]
		if proc == nil {
			nextProc, err := process.NewProcess(int32(pid))
			if err != nil {
				delete(knownProcesses, pid)
				return 0, 0, false
			}
			proc = nextProc
			knownProcesses[pid] = proc
		}

		rawCPU, err := proc.Percent(0)
		if err != nil {
			delete(knownProcesses, pid)
			return 0, 0, false
		}
		memInfo, err := proc.MemoryInfo()
		if err != nil || memInfo == nil {
			delete(knownProcesses, pid)
			return 0, 0, false
		}
		return m.hostCPUSharePercent(rawCPU), memInfo.RSS, true
	}

	sampleOnce := func() {
		targets := make([]usageServerTarget, 0, len(m.running))

		m.mu.RLock()
		for id, rs := range m.running {
			cfg, ok := m.configs[id]
			if !ok || cfg == nil || rs == nil {
				continue
			}
			rs.mu.RLock()
			target := usageServerTarget{
				ID:     id,
				Name:   cfg.Name,
				Type:   cfg.Type,
				Status: rs.status,
				PID:    rs.pid,
				RS:     rs,
			}
			rs.mu.RUnlock()
			targets = append(targets, target)
		}
		m.mu.RUnlock()

		panelCPU := 0.0
		panelRAM := uint64(0)
		if cpuPercent, ramBytes, ok := sampleProcess(panelPID); ok {
			panelCPU = cpuPercent
			panelRAM = ramBytes
		}

		serverSnapshots := make([]UsageProcessSnapshot, 0, len(targets))
		totalCPU := panelCPU
		totalRAM := panelRAM

		for _, target := range targets {
			serverCPU := 0.0
			serverRAM := uint64(0)
			if cpuPercent, ramBytes, ok := sampleProcess(target.PID); ok {
				serverCPU = cpuPercent
				serverRAM = ramBytes
			}
			target.RS.mu.Lock()
			target.RS.cpu = serverCPU
			target.RS.ram = m.hostRAMSharePercent(serverRAM)
			target.RS.ramBytes = serverRAM
			target.RS.mu.Unlock()

			if target.PID <= 0 {
				continue
			}

			snapshot := UsageProcessSnapshot{
				ID:         target.ID,
				Name:       target.Name,
				Type:       target.Type,
				Status:     target.Status,
				PID:        target.PID,
				CPUPercent: serverCPU,
				RAMBytes:   serverRAM,
				RAMPercent: m.hostRAMSharePercent(serverRAM),
			}
			serverSnapshots = append(serverSnapshots, snapshot)
			totalCPU += snapshot.CPUPercent
			totalRAM += snapshot.RAMBytes
		}

		snapshot := SystemUsageSnapshot{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Host: UsageHostInfo{
				LogicalCPUCount: m.hostLogicalCPUs,
				TotalRAMBytes:   m.hostTotalRAMBytes,
			},
			Panel: UsageProcessSnapshot{
				Name:       "Orexa Panel",
				Type:       "panel",
				Status:     "Running",
				PID:        panelPID,
				CPUPercent: panelCPU,
				RAMBytes:   panelRAM,
				RAMPercent: m.hostRAMSharePercent(panelRAM),
			},
			Servers: serverSnapshots,
			Total: UsageTotalsSnapshot{
				CPUPercent: clampPercent(totalCPU),
				RAMBytes:   totalRAM,
				RAMPercent: m.hostRAMSharePercent(totalRAM),
			},
		}

		m.usageMu.Lock()
		m.systemUsage = snapshot
		m.usageMu.Unlock()

		now := time.Now()
		if lastSummary.IsZero() || now.Sub(lastSummary) >= summaryInterval {
			lastSummary = now
			log.Printf(
				"Usage summary: panel_cpu=%.2f%% panel_ram_mb=%.2f total_cpu=%.2f%% total_ram_mb=%.2f running_servers=%d",
				snapshot.Panel.CPUPercent,
				bytesToMB(snapshot.Panel.RAMBytes),
				snapshot.Total.CPUPercent,
				bytesToMB(snapshot.Total.RAMBytes),
				len(snapshot.Servers),
			)
		}
	}

	sampleOnce()

	for {
		select {
		case <-m.stopUsageSampler:
			return
		case <-ticker.C:
			sampleOnce()
		}
	}
}

func (m *Manager) GetSystemUsage() SystemUsageSnapshot {
	m.usageMu.RLock()
	defer m.usageMu.RUnlock()

	servers := make([]UsageProcessSnapshot, len(m.systemUsage.Servers))
	copy(servers, m.systemUsage.Servers)

	return SystemUsageSnapshot{
		Timestamp: m.systemUsage.Timestamp,
		Host:      m.systemUsage.Host,
		Panel:     m.systemUsage.Panel,
		Servers:   servers,
		Total:     m.systemUsage.Total,
	}
}
