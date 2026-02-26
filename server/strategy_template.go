package server

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Service) handleStrategyTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := filepath.Join("strategy_py", "user_strategies", "sample_template.py")
	raw, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"filename": "sample_template.py",
		"content":  string(raw),
	})
}
