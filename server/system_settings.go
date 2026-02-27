package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"trade-go/config"
)

var editableEnvKeys = []string{
	"PRODUCT_NAME",
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
	fieldErrors, warnings := validateSystemSettings(updates)
	if len(fieldErrors) > 0 {
		writeJSON(w, 400, map[string]any{
			"error":        "环境变量校验失败",
			"field_errors": fieldErrors,
			"warnings":     warnings,
		})
		return
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		writeError(w, 500, "保存 .env 失败: "+err.Error())
		return
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	applyRuntimeConfigFromEnv()
	if err := s.bot.ReloadClients(); err != nil {
		writeError(w, 500, "重载交易所/智能体客户端失败: "+err.Error())
		return
	}

	out := map[string]string{}
	for _, k := range editableEnvKeys {
		out[k] = os.Getenv(k)
	}
	writeJSON(w, 200, map[string]any{
		"message":  "system settings updated",
		"settings": out,
		"warnings": warnings,
	})
}

func validateSystemSettings(settings map[string]string) (map[string]string, map[string]string) {
	errs := map[string]string{}
	warns := map[string]string{}
	get := func(k string) string { return strings.TrimSpace(settings[k]) }

	aiBase := get("AI_BASE_URL")
	aiKey := get("AI_API_KEY")
	aiModel := get("AI_MODEL")
	if aiBase != "" {
		u, err := url.Parse(aiBase)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			errs["AI_BASE_URL"] = "必须是合法的 http/https URL"
		}
		if aiKey == "" {
			errs["AI_API_KEY"] = "已填写 AI_BASE_URL 时必须填写 AI_API_KEY"
		}
		if aiModel == "" {
			errs["AI_MODEL"] = "已填写 AI_BASE_URL 时必须填写 AI 模型名称"
		}
	}
	if aiKey != "" && aiBase == "" {
		errs["AI_BASE_URL"] = "已填写 AI_API_KEY 时必须填写 AI_BASE_URL"
	}

	pyURL := get("PY_STRATEGY_URL")
	if pyURL != "" {
		u, err := url.Parse(pyURL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			errs["PY_STRATEGY_URL"] = "必须是合法的 http/https URL"
		} else {
			// 连通性告警，不阻断保存
			check := strings.TrimRight(pyURL, "/") + "/health"
			cli := &http.Client{Timeout: 2 * time.Second}
			resp, reqErr := cli.Get(check)
			if reqErr != nil {
				warns["PY_STRATEGY_URL"] = "无法连通策略服务 /health（可稍后再试）"
			} else {
				_ = resp.Body.Close()
				if resp.StatusCode >= 300 {
					warns["PY_STRATEGY_URL"] = fmt.Sprintf("策略服务 /health 返回 HTTP %d", resp.StatusCode)
				}
			}
		}
	}

	binKey := get("BINANCE_API_KEY")
	binSecret := get("BINANCE_SECRET")
	if (binKey == "") != (binSecret == "") {
		if binKey == "" {
			errs["BINANCE_API_KEY"] = "与 BINANCE_SECRET 需成对填写"
		}
		if binSecret == "" {
			errs["BINANCE_SECRET"] = "与 BINANCE_API_KEY 需成对填写"
		}
	}

	mode := strings.ToLower(get("MODE"))
	if mode != "" && mode != "prod" && mode != "test" && mode != "dev" && mode != "development" {
		errs["MODE"] = "仅支持 prod / test / dev"
	}

	httpAddr := get("HTTP_ADDR")
	if httpAddr != "" {
		if strings.HasPrefix(httpAddr, ":") {
			p, pErr := strconv.Atoi(strings.TrimPrefix(httpAddr, ":"))
			if pErr != nil || p <= 0 || p > 65535 {
				errs["HTTP_ADDR"] = "端口格式无效，应为 :8080 或 host:8080"
			}
		} else {
			host, port, splitErr := net.SplitHostPort(httpAddr)
			if splitErr != nil {
				errs["HTTP_ADDR"] = "格式无效，应为 :8080 或 host:8080"
			} else {
				_ = host
				p, pErr := strconv.Atoi(port)
				if pErr != nil || p <= 0 || p > 65535 {
					errs["HTTP_ADDR"] = "端口范围应在 1-65535"
				}
			}
		}
	}

	if v := get("ENABLE_WS_MARKET"); v != "" {
		if _, err := strconv.ParseBool(v); err != nil {
			errs["ENABLE_WS_MARKET"] = "仅支持 true/false"
		}
	}
	if v := get("STRATEGY_LLM_ENABLED"); v != "" {
		if _, err := strconv.ParseBool(v); err != nil {
			errs["STRATEGY_LLM_ENABLED"] = "仅支持 true/false"
		}
	}
	if v := get("REALTIME_MIN_INTERVAL_SEC"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 300 {
			errs["REALTIME_MIN_INTERVAL_SEC"] = "应为 1-300 的整数"
		}
	}
	if v := get("STRATEGY_LLM_TIMEOUT_SEC"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 300 {
			errs["STRATEGY_LLM_TIMEOUT_SEC"] = "应为 1-300 的整数"
		}
	}

	return errs, warns
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
