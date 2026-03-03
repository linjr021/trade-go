package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const generatedStrategiesPath = "data/generated_strategies.json"

type generatedStrategyStore struct {
	Version    string                    `json:"version"`
	UpdatedAt  string                    `json:"updated_at"`
	Strategies []generatedStrategyRecord `json:"strategies"`
}

type generatedStrategyRecord struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	RuleKey          string   `json:"rule_key,omitempty"`
	PreferencePrompt string   `json:"preference_prompt"`
	GeneratorPrompt  string   `json:"generator_prompt"`
	Logic            string   `json:"logic"`
	Basis            string   `json:"basis"`
	CreatedAt        string   `json:"created_at"`
	LastUpdatedAt    string   `json:"last_updated_at"`
	Source           string   `json:"source"`
	WorkflowVersion  string   `json:"workflow_version"`
	WorkflowChain    []string `json:"workflow_chain"`
}

func defaultGeneratedStrategyStore() generatedStrategyStore {
	return generatedStrategyStore{
		Version:    "generated-strategies/v1",
		UpdatedAt:  time.Now().Format(time.RFC3339),
		Strategies: []generatedStrategyRecord{},
	}
}

func readGeneratedStrategies() generatedStrategyStore {
	raw, err := os.ReadFile(generatedStrategiesPath)
	if err != nil {
		return defaultGeneratedStrategyStore()
	}
	var st generatedStrategyStore
	if err := json.Unmarshal(raw, &st); err != nil {
		return defaultGeneratedStrategyStore()
	}
	if strings.TrimSpace(st.Version) == "" {
		st.Version = "generated-strategies/v1"
	}
	if strings.TrimSpace(st.UpdatedAt) == "" {
		st.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	st.Strategies = normalizeGeneratedStrategyRecords(st.Strategies)
	return st
}

func writeGeneratedStrategies(st generatedStrategyStore) error {
	st.Version = "generated-strategies/v1"
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	st.Strategies = normalizeGeneratedStrategyRecords(st.Strategies)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(generatedStrategiesPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(generatedStrategiesPath, append(raw, '\n'), 0o644)
}

func normalizeGeneratedStrategyRecords(items []generatedStrategyRecord) []generatedStrategyRecord {
	out := make([]generatedStrategyRecord, 0, len(items))
	now := time.Now().Format(time.RFC3339)
	nameCount := map[string]int{}
	seenRuleKeys := map[string]bool{}
	for i, it := range items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			id = "gs_" + time.Now().Format("20060102150405") + "_" + strconv.Itoa(i+1)
		}
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = "未命名策略"
		}
		source := normalizeStrategySource(it.Source)
		combinedText := strings.Join([]string{
			name,
			strings.TrimSpace(it.PreferencePrompt),
			strings.TrimSpace(it.GeneratorPrompt),
			strings.TrimSpace(it.Logic),
			strings.TrimSpace(it.Basis),
		}, " ")
		legacySymbol, legacyHabit, legacyStyle, inferredSignals := inferNamePartsFromLegacyText(combinedText)
		if shouldMigrateLegacyGeneratedName(name, source) {
			// 仅在名称为空或能从历史文本中提取到有效特征时才迁移，
			// 避免把用户自定义命名误判为“旧格式”并覆盖。
			if strings.TrimSpace(name) == "" || inferredSignals > 0 {
				name = buildStandardStrategyName(legacySymbol, legacyHabit, legacyStyle, source == "auto_regen")
			}
		}
		explicitRuleKey := strings.TrimSpace(it.RuleKey)
		ruleKey := explicitRuleKey
		// 仅在强信号（>=2）时推断 rule_key，避免弱推断带来的误覆盖。
		if ruleKey == "" && (source == "workflow_generated" || source == "auto_regen") && inferredSignals >= 2 {
			ruleKey = buildStrategyRuleKey(legacySymbol, legacyHabit, legacyStyle, source == "auto_regen")
		}
		if ruleKey != "" {
			rk := strings.ToLower(ruleKey)
			// 仅对显式 rule_key 做去重；推断 key 不参与去重，避免误删历史策略。
			if explicitRuleKey != "" {
				if seenRuleKeys[rk] {
					continue
				}
				seenRuleKeys[rk] = true
			}
		}
		base := name
		nameCount[base]++
		if nameCount[base] > 1 {
			name = base + "_" + strconv.Itoa(nameCount[base])
		}
		createdAt := strings.TrimSpace(it.CreatedAt)
		if createdAt == "" {
			createdAt = now
		}
		out = append(out, generatedStrategyRecord{
			ID:               id,
			Name:             name,
			RuleKey:          strings.TrimSpace(ruleKey),
			PreferencePrompt: strings.TrimSpace(it.PreferencePrompt),
			GeneratorPrompt:  strings.TrimSpace(it.GeneratorPrompt),
			Logic:            strings.TrimSpace(it.Logic),
			Basis:            strings.TrimSpace(it.Basis),
			CreatedAt:        createdAt,
			LastUpdatedAt:    fallbackString(strings.TrimSpace(it.LastUpdatedAt), createdAt),
			Source:           source,
			WorkflowVersion:  strings.TrimSpace(it.WorkflowVersion),
			WorkflowChain:    normalizeStringSlice(it.WorkflowChain),
		})
	}
	return out
}

func shouldMigrateLegacyGeneratedName(name, source string) bool {
	src := normalizeStrategySource(source)
	if src != "workflow_generated" && src != "auto_regen" {
		return false
	}
	n := strings.TrimSpace(name)
	if n == "" {
		return true
	}
	lower := strings.ToLower(n)
	if strings.Contains(lower, "_auto") || strings.HasPrefix(lower, "ai_") || strings.HasPrefix(lower, "auto_") {
		return true
	}
	if strings.HasPrefix(lower, "workflow_") || strings.HasPrefix(lower, "generated_") {
		return true
	}
	if isLikelyLegacyStructuredName(lower) {
		return true
	}
	return false
}

func isLikelyLegacyStructuredName(name string) bool {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(name)), "_")
	if len(parts) < 3 || len(parts) > 5 {
		return false
	}
	symbol := strings.ToUpper(strings.TrimSpace(parts[0]))
	switch symbol {
	case "BTC", "ETH", "BNB", "SOL", "XRP", "DOGE", "ADA",
		"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT":
	default:
		return false
	}
	timeframe := strings.TrimSpace(parts[1])
	switch timeframe {
	case "10m", "15m", "1h", "4h", "1d", "5d", "30d", "90d":
	default:
		return false
	}
	styleToken := strings.Join(parts[2:], "_")
	styleToken = strings.TrimSuffix(styleToken, "_auto")
	switch styleToken {
	case "trend", "trend_follow", "trend_following", "breakout", "mean_reversion", "hybrid":
		return true
	default:
		return false
	}
}

func inferNamePartsFromLegacyText(text string) (symbol, habit, style string, signalCount int) {
	upper := strings.ToUpper(text)
	lower := strings.ToLower(text)
	symbol = "BTCUSDT"
	switch {
	case strings.Contains(upper, "ETH"):
		symbol = "ETHUSDT"
		signalCount++
	case strings.Contains(upper, "BNB"):
		symbol = "BNBUSDT"
		signalCount++
	case strings.Contains(upper, "SOL"):
		symbol = "SOLUSDT"
		signalCount++
	case strings.Contains(upper, "XRP"):
		symbol = "XRPUSDT"
		signalCount++
	case strings.Contains(upper, "DOGE"):
		symbol = "DOGEUSDT"
		signalCount++
	case strings.Contains(upper, "ADA"):
		symbol = "ADAUSDT"
		signalCount++
	case strings.Contains(upper, "BTC"):
		symbol = "BTCUSDT"
		signalCount++
	}
	habit = "1h"
	switch {
	case strings.Contains(lower, "10m") || strings.Contains(lower, "15m"):
		habit = "10m"
		signalCount++
	case strings.Contains(lower, "4h"):
		habit = "4h"
		signalCount++
	case strings.Contains(lower, "1d") || strings.Contains(lower, "日线"):
		habit = "1D"
		signalCount++
	}
	style = "hybrid"
	switch {
	case strings.Contains(lower, "均值") || strings.Contains(lower, "mean") || strings.Contains(lower, "reversion") || strings.Contains(lower, "oscillation"):
		style = "mean_reversion"
		signalCount++
	case strings.Contains(lower, "突破") || strings.Contains(lower, "breakout"):
		style = "breakout"
		signalCount++
	case strings.Contains(lower, "趋势") || strings.Contains(lower, "trend"):
		style = "trend_following"
		signalCount++
	}
	return symbol, habit, style, signalCount
}

func normalizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

func normalizeStrategySource(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "workflow_generated", "workflow", "auto_regen", "manual_external":
		return v
	default:
		return ""
	}
}

func fallbackString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(fallback)
}

func mapToString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return strings.TrimSpace(anyToString(v))
		}
	}
	return ""
}

func mapToStringSlice(m map[string]any, keys ...string) []string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch items := v.(type) {
		case []any:
			out := make([]string, 0, len(items))
			for _, item := range items {
				s := strings.TrimSpace(anyToString(item))
				if s != "" {
					out = append(out, s)
				}
			}
			return normalizeStringSlice(out)
		case []string:
			return normalizeStringSlice(items)
		case string:
			parts := strings.Split(items, ",")
			return normalizeStringSlice(parts)
		}
	}
	return nil
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		raw, _ := json.Marshal(t)
		s := strings.TrimSpace(string(raw))
		s = strings.TrimPrefix(s, `"`)
		s = strings.TrimSuffix(s, `"`)
		return s
	}
}

func (s *Service) handleGeneratedStrategies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		st := readGeneratedStrategies()
		writeJSON(w, http.StatusOK, map[string]any{
			"version":    st.Version,
			"updated_at": st.UpdatedAt,
			"strategies": st.Strategies,
		})
	case http.MethodPost:
		var req struct {
			Strategies []map[string]any `json:"strategies"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		items := make([]generatedStrategyRecord, 0, len(req.Strategies))
		for _, row := range req.Strategies {
			items = append(items, generatedStrategyRecord{
				ID:               mapToString(row, "id"),
				Name:             mapToString(row, "name"),
				PreferencePrompt: mapToString(row, "preference_prompt", "preferencePrompt"),
				GeneratorPrompt:  mapToString(row, "generator_prompt", "generatorPrompt", "prompt"),
				Logic:            mapToString(row, "logic"),
				Basis:            mapToString(row, "basis"),
				RuleKey:          mapToString(row, "rule_key", "ruleKey"),
				CreatedAt:        mapToString(row, "created_at", "createdAt"),
				LastUpdatedAt:    mapToString(row, "last_updated_at", "lastUpdatedAt"),
				Source:           mapToString(row, "source"),
				WorkflowVersion:  mapToString(row, "workflow_version", "workflowVersion"),
				WorkflowChain:    mapToStringSlice(row, "workflow_chain", "workflowChain"),
			})
		}
		st := generatedStrategyStore{
			Version:    "generated-strategies/v1",
			UpdatedAt:  time.Now().Format(time.RFC3339),
			Strategies: items,
		}
		if err := writeGeneratedStrategies(st); err != nil {
			writeError(w, http.StatusInternalServerError, "save generated strategies failed: "+err.Error())
			return
		}
		st = readGeneratedStrategies()
		writeJSON(w, http.StatusOK, map[string]any{
			"message":    "generated strategies synced",
			"version":    st.Version,
			"updated_at": st.UpdatedAt,
			"strategies": st.Strategies,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
