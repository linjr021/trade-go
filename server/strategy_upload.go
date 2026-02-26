package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"trade-go/config"
)

func (s *Service) handleStrategyUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	name := filepath.Base(strings.TrimSpace(header.Filename))
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\\`) {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	if strings.ToLower(filepath.Ext(name)) != ".py" {
		writeError(w, http.StatusBadRequest, "only .py file is allowed")
		return
	}

	dir := filepath.Join("strategy_py", "user_strategies")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "create strategy directory failed")
		return
	}

	tmp := filepath.Join(dir, ".upload_"+name)
	dst := filepath.Join(dir, name)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open destination file failed")
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, "write destination file failed")
		return
	}
	_ = out.Close()
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, "save strategy file failed")
		return
	}

	available, reloadErr := reloadPythonStrategies()
	resp := map[string]any{
		"message":    "uploaded",
		"filename":   name,
		"saved_path": dst,
	}
	if len(available) > 0 {
		resp["available"] = available
	}
	if reloadErr != nil {
		resp["reload_error"] = reloadErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func reloadPythonStrategies() ([]string, error) {
	pyURL := strings.TrimSpace(config.Config.PyStrategyURL)
	if pyURL == "" {
		return nil, nil
	}
	url := strings.TrimRight(pyURL, "/") + "/strategies/reload"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, nil
	}

	var out struct {
		Available []string `json:"available"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil
	}
	return out.Available, nil
}
