package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"trade-go/config"
)

func (s *Service) handleStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	generatedNames := []string{}
	for _, st := range readGeneratedStrategies().Strategies {
		name := strings.TrimSpace(st.Name)
		if name != "" {
			generatedNames = append(generatedNames, name)
		}
	}
	baseAvailable := []string{"ai_assisted", "trend_following", "mean_reversion", "breakout"}
	mergedFallbackAvailable := append([]string{}, baseAvailable...)
	mergedFallbackAvailable = append(mergedFallbackAvailable, generatedNames...)
	fallback := map[string]any{
		"available": mergedFallbackAvailable,
		"enabled":   []string{},
	}
	pyURL := strings.TrimSpace(config.Config.PyStrategyURL)
	if pyURL == "" {
		writeJSON(w, http.StatusOK, fallback)
		return
	}

	url := strings.TrimRight(pyURL, "/") + "/strategies"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, fallback)
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, fallback)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		writeJSON(w, http.StatusOK, fallback)
		return
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		writeJSON(w, http.StatusOK, fallback)
		return
	}
	if out == nil {
		out = map[string]any{}
	}
	if val, ok := out["available"]; ok {
		merged := append([]string{}, parseStrategiesAny(val)...)
		merged = append(merged, generatedNames...)
		out["available"] = uniqueStrings(merged)
	} else {
		out["available"] = fallback["available"]
	}
	if _, ok := out["enabled"]; !ok {
		out["enabled"] = []string{}
	}
	out["source"] = fmt.Sprintf("%s/strategies", strings.TrimRight(pyURL, "/"))
	writeJSON(w, http.StatusOK, out)
}

func parseStrategiesAny(v any) []string {
	items, ok := v.([]interface{})
	if !ok {
		ss, ok2 := v.([]string)
		if !ok2 {
			return nil
		}
		return uniqueStrings(ss)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		name := strings.TrimSpace(anyToString(it))
		if name != "" {
			out = append(out, name)
		}
	}
	return uniqueStrings(out)
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}
