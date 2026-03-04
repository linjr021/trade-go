package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"trade-go/config"
)

const (
	liveRuntimePath     = "data/live_runtime.json"
	liveRuntimeVersion  = "live-runtime/v1"
	liveMaxStrategyHist = 120
)

type liveStrategyHistoryEntry struct {
	ID         string                 `json:"id"`
	TS         string                 `json:"ts"`
	Strategies []string               `json:"strategies"`
	Source     string                 `json:"source"`
	Meta       map[string]interface{} `json:"meta"`
	Params     map[string]interface{} `json:"params"`
}

type liveRuntimeState struct {
	Version          string                     `json:"version"`
	UpdatedAt        string                     `json:"updated_at"`
	ActiveStrategies []string                   `json:"active_strategies"`
	ActiveSince      string                     `json:"active_since"`
	StrategyHistory  []liveStrategyHistoryEntry `json:"strategy_history"`
}

func defaultLiveRuntimeState() liveRuntimeState {
	return liveRuntimeState{
		Version:          liveRuntimeVersion,
		UpdatedAt:        time.Now().Format(time.RFC3339),
		ActiveStrategies: []string{},
		ActiveSince:      "",
		StrategyHistory:  []liveStrategyHistoryEntry{},
	}
}

func readLiveRuntimeState() (liveRuntimeState, error) {
	raw, err := os.ReadFile(liveRuntimePath)
	if err != nil {
		return liveRuntimeState{}, err
	}
	var out liveRuntimeState
	if err := json.Unmarshal(raw, &out); err != nil {
		return liveRuntimeState{}, err
	}
	return out, nil
}

func writeLiveRuntimeState(st liveRuntimeState) error {
	st.Version = liveRuntimeVersion
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(liveRuntimePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(liveRuntimePath, append(raw, '\n'), 0o644)
}

func (s *Service) initLiveRuntime() {
	state := defaultLiveRuntimeState()
	if loaded, err := readLiveRuntimeState(); err == nil {
		state = loaded
	}
	state = normalizeLiveRuntimeState(state)
	s.mu.Lock()
	s.liveState = state
	cfg := s.bot.TradeConfig()
	enabled := parseEnabledStrategiesEnv("")
	s.syncLiveRuntimeLocked(enabled, cfg, "live_init")
	_ = s.persistLiveStateLocked()
	s.mu.Unlock()
}

func normalizeLiveRuntimeState(in liveRuntimeState) liveRuntimeState {
	out := in
	out.Version = liveRuntimeVersion
	if strings.TrimSpace(out.UpdatedAt) == "" {
		out.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	out.ActiveStrategies = normalizeLiveStrategySelection(out.ActiveStrategies)
	if out.StrategyHistory == nil {
		out.StrategyHistory = []liveStrategyHistoryEntry{}
	}
	if len(out.StrategyHistory) > liveMaxStrategyHist {
		out.StrategyHistory = append([]liveStrategyHistoryEntry{}, out.StrategyHistory[:liveMaxStrategyHist]...)
	}
	return out
}

func normalizeLiveStrategySelection(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		name := strings.TrimSpace(item)
		if name == "" || isDeprecatedBuiltinStrategy(name) {
			continue
		}
		k := strings.ToLower(name)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, name)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func sameLiveStrategyList(a, b []string) bool {
	aa := normalizeLiveStrategySelection(a)
	bb := normalizeLiveStrategySelection(b)
	if len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if strings.ToLower(aa[i]) != strings.ToLower(bb[i]) {
			return false
		}
	}
	return true
}

func (s *Service) persistLiveStateLocked() error {
	return writeLiveRuntimeState(s.liveState)
}

func (s *Service) syncLiveRuntime(enabled []string, cfg config.TradeConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncLiveRuntimeLocked(enabled, cfg, "live_sync") {
		_ = s.persistLiveStateLocked()
	}
}

func (s *Service) syncLiveRuntimeLocked(enabled []string, cfg config.TradeConfig, source string) bool {
	normalized := normalizeLiveStrategySelection(enabled)
	changed := false
	if !sameLiveStrategyList(s.liveState.ActiveStrategies, normalized) {
		s.liveState.ActiveStrategies = append([]string{}, normalized...)
		if len(normalized) > 0 {
			s.liveState.ActiveSince = time.Now().Format(time.RFC3339)
			entry := s.makeLiveStrategyHistoryEntry(normalized, cfg, source)
			s.liveState.StrategyHistory = append([]liveStrategyHistoryEntry{entry}, s.liveState.StrategyHistory...)
			if len(s.liveState.StrategyHistory) > liveMaxStrategyHist {
				s.liveState.StrategyHistory = s.liveState.StrategyHistory[:liveMaxStrategyHist]
			}
		} else {
			s.liveState.ActiveSince = ""
		}
		changed = true
	}
	if s.patchLiveStrategyHistoryMetaLocked() {
		changed = true
	}
	if changed {
		s.liveState.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	return changed
}

func (s *Service) makeLiveStrategyHistoryEntry(strategies []string, cfg config.TradeConfig, source string) liveStrategyHistoryEntry {
	list := append([]string{}, normalizeLiveStrategySelection(strategies)...)
	meta := s.paperStrategyMetaMap(list)
	params := map[string]interface{}{
		"positionSizingMode":      cfg.PositionSizingMode,
		"leverage":                cfg.Leverage,
		"highConfidenceAmount":    cfg.HighConfidenceAmount,
		"lowConfidenceAmount":     cfg.LowConfidenceAmount,
		"highConfidenceMarginPct": cfg.HighConfidenceMarginPct * 100,
		"lowConfidenceMarginPct":  cfg.LowConfidenceMarginPct * 100,
	}
	now := time.Now().Format(time.RFC3339)
	return liveStrategyHistoryEntry{
		ID:         "live_strategy_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		TS:         now,
		Strategies: list,
		Source:     strings.TrimSpace(source),
		Meta:       meta,
		Params:     params,
	}
}

func (s *Service) patchLiveStrategyHistoryMetaLocked() bool {
	if len(s.liveState.StrategyHistory) == 0 {
		return false
	}
	changed := false
	for i := range s.liveState.StrategyHistory {
		row := s.liveState.StrategyHistory[i]
		list := normalizeLiveStrategySelection(row.Strategies)
		meta := s.paperStrategyMetaMap(list)
		oldRaw, _ := json.Marshal(row.Meta)
		newRaw, _ := json.Marshal(meta)
		if string(oldRaw) != string(newRaw) {
			row.Meta = meta
			s.liveState.StrategyHistory[i] = row
			changed = true
		}
	}
	return changed
}
