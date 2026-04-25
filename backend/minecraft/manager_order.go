package minecraft

import (
	"fmt"
	"sort"
	"strings"
)

func (m *Manager) normalizeServerOrderLocked() bool {
	if len(m.configs) == 0 {
		return false
	}

	cfgs := make([]*ServerConfig, 0, len(m.configs))
	seenOrder := make(map[int]struct{}, len(m.configs))
	hasInvalidOrder := false

	for _, cfg := range m.configs {
		cfgs = append(cfgs, cfg)
		if cfg.Order <= 0 {
			hasInvalidOrder = true
			continue
		}
		if _, exists := seenOrder[cfg.Order]; exists {
			hasInvalidOrder = true
			continue
		}
		seenOrder[cfg.Order] = struct{}{}
	}

	if hasInvalidOrder {
		sort.Slice(cfgs, func(i, j int) bool {
			left := strings.TrimSpace(strings.ToLower(cfgs[i].Name))
			right := strings.TrimSpace(strings.ToLower(cfgs[j].Name))
			if left == right {
				return cfgs[i].ID < cfgs[j].ID
			}
			return left < right
		})
	} else {
		sort.Slice(cfgs, func(i, j int) bool {
			if cfgs[i].Order == cfgs[j].Order {
				left := strings.TrimSpace(strings.ToLower(cfgs[i].Name))
				right := strings.TrimSpace(strings.ToLower(cfgs[j].Name))
				if left == right {
					return cfgs[i].ID < cfgs[j].ID
				}
				return left < right
			}
			return cfgs[i].Order < cfgs[j].Order
		})
	}

	changed := false
	for i, cfg := range cfgs {
		nextOrder := i + 1
		if cfg.Order != nextOrder {
			cfg.Order = nextOrder
			changed = true
		}
	}

	return changed
}

func (m *Manager) nextServerOrderLocked() int {
	maxOrder := 0
	for _, cfg := range m.configs {
		if cfg.Order > maxOrder {
			maxOrder = cfg.Order
		}
	}
	return maxOrder + 1
}

func (m *Manager) isAlphabeticalOrderLocked() bool {
	if len(m.configs) <= 1 {
		return true
	}
	ordered := make([]*ServerConfig, 0, len(m.configs))
	alpha := make([]*ServerConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		ordered = append(ordered, cfg)
		alpha = append(alpha, cfg)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Order == ordered[j].Order {
			left := strings.TrimSpace(strings.ToLower(ordered[i].Name))
			right := strings.TrimSpace(strings.ToLower(ordered[j].Name))
			if left == right {
				return ordered[i].ID < ordered[j].ID
			}
			return left < right
		}
		return ordered[i].Order < ordered[j].Order
	})
	sort.Slice(alpha, func(i, j int) bool {
		left := strings.TrimSpace(strings.ToLower(alpha[i].Name))
		right := strings.TrimSpace(strings.ToLower(alpha[j].Name))
		if left == right {
			return alpha[i].ID < alpha[j].ID
		}
		return left < right
	})
	for i := range ordered {
		if ordered[i].ID != alpha[i].ID {
			return false
		}
	}
	return true
}

func (m *Manager) assignAlphabeticalOrderLocked() {
	cfgs := make([]*ServerConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		cfgs = append(cfgs, cfg)
	}
	sort.Slice(cfgs, func(i, j int) bool {
		left := strings.TrimSpace(strings.ToLower(cfgs[i].Name))
		right := strings.TrimSpace(strings.ToLower(cfgs[j].Name))
		if left == right {
			return cfgs[i].ID < cfgs[j].ID
		}
		return left < right
	})
	for i, cfg := range cfgs {
		cfg.Order = i + 1
	}
}

func (m *Manager) assignNewServerOrderLocked(newCfg *ServerConfig) {
	if m.isAlphabeticalOrderLocked() {
		m.assignAlphabeticalOrderLocked()
		return
	}
	newCfg.Order = m.nextServerOrderLocked()
}

func (m *Manager) SetServerOrder(orderedIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(orderedIDs) != len(m.configs) {
		return fmt.Errorf("orderedIds must include every server exactly once")
	}

	seen := make(map[string]struct{}, len(orderedIDs))
	validatedIDs := make([]string, 0, len(orderedIDs))
	for _, rawID := range orderedIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return fmt.Errorf("orderedIds contains an empty server id")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("orderedIds contains duplicate server id %q", id)
		}
		_, exists := m.configs[id]
		if !exists {
			return fmt.Errorf("orderedIds contains unknown server id %q", id)
		}
		seen[id] = struct{}{}
		validatedIDs = append(validatedIDs, id)
	}

	for id := range m.configs {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("orderedIds is missing server id %q", id)
		}
	}

	for i, id := range validatedIDs {
		m.configs[id].Order = i + 1
	}

	return m.persist()
}
