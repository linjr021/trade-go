package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"trade-go/config"
)

var editableEnvKeys = []string{
	"AI_API_KEY",
	"AI_BASE_URL",
	"AI_MODEL",
	"PY_STRATEGY_URL",
	"BINANCE_API_KEY",
	"BINANCE_SECRET",
	"MODE",
	"HTTP_ADDR",
	"TRADE_DB_PATH",
	"ENABLE_WS_MARKET",
	"REALTIME_MIN_INTERVAL_SEC",
	"STRATEGY_LLM_ENABLED",
	"STRATEGY_LLM_TIMEOUT_SEC",
}

func (s *Service) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.handleGetSystemSettings(w, r)
	case "POST":
		s.handleSaveSystemSettings(w, r)
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (s *Service) handleGetSystemSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, k := range editableEnvKeys {
		out[k] = os.Getenv(k)
	}
	writeJSON(w, 200, map[string]any{"settings": out, "keys": editableEnvKeys})
}

func (s *Service) handleSaveSystemSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Settings map[string]string `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	if req.Settings == nil {
		req.Settings = map[string]string{}
	}

	allowed := map[string]bool{}
	for _, k := range editableEnvKeys {
		allowed[k] = true
	}
	updates := map[string]string{}
	for k, v := range req.Settings {
		if !allowed[k] {
			continue
		}
		updates[k] = strings.TrimSpace(v)
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		writeError(w, 500, "保存 .env 失败: "+err.Error())
		return
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	applyRuntimeConfigFromEnv()
	s.bot.ReloadClients()

	out := map[string]string{}
	for _, k := range editableEnvKeys {
		out[k] = os.Getenv(k)
	}
	writeJSON(w, 200, map[string]any{"message": "system settings updated", "settings": out})
}

func upsertDotEnv(path string, updates map[string]string) error {
	lines := []string{}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		_ = f.Close()
	}
	seen := map[string]bool{}
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || !strings.Contains(trim, "=") {
			continue
		}
		parts := strings.SplitN(trim, "=", 2)
		key := strings.TrimSpace(parts[0])
		if v, ok := updates[key]; ok {
			lines[i] = key + "=" + v
			seen[key] = true
		}
	}
	for k, v := range updates {
		if !seen[k] {
			lines = append(lines, k+"="+v)
		}
	}
	data := strings.Join(lines, "\n")
	if data != "" && !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func applyRuntimeConfigFromEnv() {
	cfg := config.Config
	if cfg == nil {
		return
	}
	cfg.AIAPIKey = os.Getenv("AI_API_KEY")
	cfg.AIBaseURL = os.Getenv("AI_BASE_URL")
	cfg.AIModel = os.Getenv("AI_MODEL")
	cfg.PyStrategyURL = os.Getenv("PY_STRATEGY_URL")
	cfg.BinanceAPIKey = os.Getenv("BINANCE_API_KEY")
	cfg.BinanceSecret = os.Getenv("BINANCE_SECRET")

	if v := os.Getenv("HIGH_CONFIDENCE_MARGIN_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.HighConfidenceMarginPct = f
		}
	}
	if v := os.Getenv("LOW_CONFIDENCE_MARGIN_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.LowConfidenceMarginPct = f
		}
	}
	if v := os.Getenv("POSITION_SIZING_MODE"); v != "" {
		cfg.Trade.PositionSizingMode = v
	}
}
