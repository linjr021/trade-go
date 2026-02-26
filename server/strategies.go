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

	fallback := map[string]any{
		"available": []string{"ai_assisted", "trend_following", "mean_reversion", "breakout"},
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
	if _, ok := out["available"]; !ok {
		out["available"] = fallback["available"]
	}
	if _, ok := out["enabled"]; !ok {
		out["enabled"] = []string{}
	}
	out["source"] = fmt.Sprintf("%s/strategies", strings.TrimRight(pyURL, "/"))
	writeJSON(w, http.StatusOK, out)
}
