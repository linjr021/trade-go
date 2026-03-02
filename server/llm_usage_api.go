package server

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Service) handleLLMUsageLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	logs := getLLMUsageLogs(limit, channel)
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":     logs,
		"count":    len(logs),
		"channel":  channel,
		"limit":    limit,
		"snapshot": getLLMUsageSnapshot(),
	})
}
