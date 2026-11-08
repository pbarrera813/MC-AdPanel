package minecraft

import (
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// USER_HZ: /proc reports CPU time in 1/100 s ticks on Linux.
const procClockTicksPerSecond = 100

type procSample struct {
	ticks uint64
	at    time.Time
}

func parseMemTotalBytes(meminfo string) uint64 {
	for _, line := range strings.Split(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024
		}
	}
	return 0
}

// The command name (field 2) may contain spaces and parentheses, so fields are counted after the last ')'.
func parseProcStatTicks(stat string) (uint64, bool) {
	end := strings.LastIndex(stat, ")")
	if end < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return utime + stime, true
}

func parseStatmRSSBytes(statm string, pageSize int) (uint64, bool) {
	fields := strings.Fields(statm)
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(pageSize), true
}

func readProcUsage(pid int) (ticks uint64, rssBytes uint64, err error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	statm, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0, 0, err
	}
	ticks, ok := parseProcStatTicks(string(stat))
	if !ok {
		return 0, 0, fmt.Errorf("unparseable /proc/%d/stat", pid)
	}
	rssBytes, ok = parseStatmRSSBytes(string(statm), os.Getpagesize())
	if !ok {
		return 0, 0, fmt.Errorf("unparseable /proc/%d/statm", pid)
	}
	return ticks, rssBytes, nil
}

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
	m.hostLogicalCPUs = runtime.NumCPU()
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		m.hostTotalRAMBytes = parseMemTotalBytes(string(data))
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
	previous := make(map[int]procSample)
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	lastSummary := time.Time{}

	sampleProcess := func(pid int) (float64, uint64, bool) {
		if pid <= 0 {
			return 0, 0, false
		}
		ticks, rssBytes, err := readProcUsage(pid)
		if err != nil {
			delete(previous, pid)
			return 0, 0, false
		}
		now := time.Now()
		rawCPU := 0.0
		if prev, ok := previous[pid]; ok && ticks >= prev.ticks {
			if elapsed := now.Sub(prev.at).Seconds(); elapsed > 0 {
				rawCPU = float64(ticks-prev.ticks) / procClockTicksPerSecond / elapsed * 100
			}
		}
		previous[pid] = procSample{ticks: ticks, at: now}
		return m.hostCPUSharePercent(rawCPU), rssBytes, true
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
