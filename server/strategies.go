package server

import (
	"net/http"
	"strings"
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
	available := append([]string{}, generatedNames...)
	available = uniqueStrings(available)

	enabledRaw := parseEnabledStrategiesEnv("")
	enabled := make([]string, 0, len(enabledRaw))
	availSet := map[string]bool{}
	for _, item := range available {
		availSet[strings.ToLower(strings.TrimSpace(item))] = true
	}
	for _, item := range enabledRaw {
		k := strings.ToLower(strings.TrimSpace(item))
		if !availSet[k] {
			continue
		}
		enabled = append(enabled, strings.TrimSpace(item))
		if len(enabled) >= 3 {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"available": available,
		"enabled":   enabled,
		"source":    "local_ai_workflow",
	})
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
