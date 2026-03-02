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
	"PY_STRATEGY_ENABLED",
	"BINANCE_API_KEY",
	"BINANCE_SECRET",
	"MODE",
	"HTTP_ADDR",
	"TRADE_DB_PATH",
	"ENABLE_WS_MARKET",
	"REALTIME_MIN_INTERVAL_SEC",
	"STRATEGY_LLM_ENABLED",
	"STRATEGY_LLM_TIMEOUT_SEC",
	"AUTO_REVIEW_ENABLED",
	"AUTO_REVIEW_AFTER_ORDER_ONLY",
	"AUTO_REVIEW_INTERVAL_SEC",
	"AUTO_REVIEW_VOLATILITY_PCT",
	"AUTO_REVIEW_DRAWDOWN_WARN_PCT",
	"AUTO_REVIEW_LOSS_STREAK_WARN",
	"AUTO_REVIEW_RISK_REDUCE_FACTOR",
	"AUTO_STRATEGY_REGEN_ENABLED",
	"AUTO_STRATEGY_REGEN_COOLDOWN_SEC",
	"AUTO_STRATEGY_REGEN_LOSS_STREAK",
	"AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT",
	"AUTO_STRATEGY_REGEN_MIN_RR",
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
	// Keep .env in sync with front-end integration state before exposing runtime settings.
	// This avoids startup race where system settings are read before integrations cleanup.
	_, _ = readIntegrations()

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
	if v := get("AUTO_REVIEW_ENABLED"); v != "" {
		if _, err := strconv.ParseBool(v); err != nil {
			errs["AUTO_REVIEW_ENABLED"] = "仅支持 true/false"
		}
	}
	if v := get("AUTO_REVIEW_AFTER_ORDER_ONLY"); v != "" {
		if _, err := strconv.ParseBool(v); err != nil {
			errs["AUTO_REVIEW_AFTER_ORDER_ONLY"] = "仅支持 true/false"
		}
	}
	if v := get("AUTO_REVIEW_INTERVAL_SEC"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 60 || n > 86400 {
			errs["AUTO_REVIEW_INTERVAL_SEC"] = "应为 60-86400 的整数"
		}
	}
	if v := get("AUTO_REVIEW_VOLATILITY_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 20 {
			errs["AUTO_REVIEW_VOLATILITY_PCT"] = "应为 (0,20] 的数字"
		}
	}
	if v := get("AUTO_REVIEW_DRAWDOWN_WARN_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 1 {
			errs["AUTO_REVIEW_DRAWDOWN_WARN_PCT"] = "应为 (0,1] 的数字"
		}
	}
	if v := get("AUTO_REVIEW_LOSS_STREAK_WARN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			errs["AUTO_REVIEW_LOSS_STREAK_WARN"] = "应为 1-100 的整数"
		}
	}
	if v := get("AUTO_REVIEW_RISK_REDUCE_FACTOR"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 1 {
			errs["AUTO_REVIEW_RISK_REDUCE_FACTOR"] = "应为 (0,1] 的数字"
		}
	}
	if v := get("AUTO_STRATEGY_REGEN_ENABLED"); v != "" {
		if _, err := strconv.ParseBool(v); err != nil {
			errs["AUTO_STRATEGY_REGEN_ENABLED"] = "仅支持 true/false"
		}
	}
	if v := get("AUTO_STRATEGY_REGEN_COOLDOWN_SEC"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 300 || n > 604800 {
			errs["AUTO_STRATEGY_REGEN_COOLDOWN_SEC"] = "应为 300-604800 的整数"
		}
	}
	if v := get("AUTO_STRATEGY_REGEN_LOSS_STREAK"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			errs["AUTO_STRATEGY_REGEN_LOSS_STREAK"] = "应为 1-100 的整数"
		}
	}
	if v := get("AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 1 {
			errs["AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT"] = "应为 (0,1] 的数字"
		}
	}
	if v := get("AUTO_STRATEGY_REGEN_MIN_RR"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1 || f > 10 {
			errs["AUTO_STRATEGY_REGEN_MIN_RR"] = "应为 [1,10] 的数字"
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
	cfg.ActiveExchange = os.Getenv("ACTIVE_EXCHANGE")
	cfg.BinanceAPIKey = os.Getenv("BINANCE_API_KEY")
	cfg.BinanceSecret = os.Getenv("BINANCE_SECRET")
	cfg.OKXAPIKey = os.Getenv("OKX_API_KEY")
	cfg.OKXSecret = os.Getenv("OKX_SECRET")
	cfg.OKXPassword = os.Getenv("OKX_PASSWORD")
	if v := strings.TrimSpace(os.Getenv("TRADE_SYMBOL")); v != "" {
		cfg.Trade.Symbol = strings.ToUpper(v)
	}
	if v := strings.TrimSpace(os.Getenv("TRADE_AMOUNT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.Amount = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("HIGH_CONFIDENCE_AMOUNT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.HighConfidenceAmount = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("LOW_CONFIDENCE_AMOUNT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.LowConfidenceAmount = f
		}
	}

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
	if v := strings.TrimSpace(os.Getenv("LEVERAGE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.Leverage = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("TIMEFRAME")); v != "" {
		cfg.Trade.Timeframe = v
	}
	if v := strings.TrimSpace(os.Getenv("TEST_MODE")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Trade.TestMode = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("DATA_POINTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.DataPoints = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_RISK_PER_TRADE_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.MaxRiskPerTradePct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_POSITION_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.MaxPositionPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_CONSECUTIVE_LOSSES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.MaxConsecutiveLosses = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_DAILY_LOSS_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.MaxDailyLossPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_DRAWDOWN_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.MaxDrawdownPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("LIQUIDATION_BUFFER_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.LiquidationBufferPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_ENABLED")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Trade.AutoReviewEnabled = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_AFTER_ORDER_ONLY")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Trade.AutoReviewAfterOrderOnly = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_INTERVAL_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.AutoReviewIntervalSec = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_VOLATILITY_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.AutoReviewVolatilityPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_DRAWDOWN_WARN_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.AutoReviewDrawdownWarnPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_LOSS_STREAK_WARN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.AutoReviewLossStreakWarn = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_REVIEW_RISK_REDUCE_FACTOR")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.AutoReviewRiskReduceFactor = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_STRATEGY_REGEN_ENABLED")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Trade.AutoStrategyRegenEnabled = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_STRATEGY_REGEN_COOLDOWN_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.AutoStrategyRegenCooldownSec = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_STRATEGY_REGEN_LOSS_STREAK")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Trade.AutoStrategyRegenLossStreak = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.AutoStrategyRegenDrawdownWarnPct = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("AUTO_STRATEGY_REGEN_MIN_RR")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Trade.AutoStrategyRegenMinRR = f
		}
	}
}
