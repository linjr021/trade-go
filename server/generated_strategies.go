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
	ID               string `json:"id"`
	Name             string `json:"name"`
	PreferencePrompt string `json:"preference_prompt"`
	GeneratorPrompt  string `json:"generator_prompt"`
	Logic            string `json:"logic"`
	Basis            string `json:"basis"`
	CreatedAt        string `json:"created_at"`
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
	for i, it := range items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			id = "gs_" + time.Now().Format("20060102150405") + "_" + strconv.Itoa(i+1)
		}
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = "未命名策略"
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
			PreferencePrompt: strings.TrimSpace(it.PreferencePrompt),
			GeneratorPrompt:  strings.TrimSpace(it.GeneratorPrompt),
			Logic:            strings.TrimSpace(it.Logic),
			Basis:            strings.TrimSpace(it.Basis),
			CreatedAt:        createdAt,
		})
	}
	return out
}

func mapToString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return strings.TrimSpace(anyToString(v))
		}
	}
	return ""
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
				CreatedAt:        mapToString(row, "created_at", "createdAt"),
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
