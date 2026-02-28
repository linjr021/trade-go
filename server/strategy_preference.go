package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/exchange"
	"trade-go/indicators"
	"trade-go/llmapi"
)

type generatePreferenceRequest struct {
	Symbol        string  `json:"symbol"`
	Habit         string  `json:"habit"`
	StrategyStyle string  `json:"strategy_style"`
	MinRR         float64 `json:"min_rr"`
	AllowReversal bool    `json:"allow_reversal"`
	LowConfAction string  `json:"low_conf_action"`
	DirectionBias string  `json:"direction_bias"`
}

type generatedPreference struct {
	StrategyName     string `json:"strategy_name"`
	PreferencePrompt string `json:"preference_prompt"`
	GeneratorPrompt  string `json:"generator_prompt"`
	Logic            string `json:"logic"`
	Basis            string `json:"basis"`
}

func (s *Service) handleGenerateStrategyPreference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req generatePreferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	habit := strings.TrimSpace(req.Habit)
	if habit == "" {
		habit = "1h"
	}
	style := strings.TrimSpace(req.StrategyStyle)
	if style == "" {
		style = "hybrid"
	}
	minRR := req.MinRR
	if minRR <= 0 {
		minRR = 2.0
	}
	lowConfAction := strings.TrimSpace(req.LowConfAction)
	if lowConfAction == "" {
		lowConfAction = "hold"
	}
	directionBias := strings.TrimSpace(req.DirectionBias)
	if directionBias == "" {
		directionBias = "balanced"
	}
	f := timeframeByHabit(habit)
	tradeCfg := s.bot.TradeConfig()

	client := exchange.NewClient()
	candles, err := client.FetchOHLCV(symbol, f, 120)
	if err != nil || len(candles) < 30 {
		fb := fallbackGeneratedPreference(symbol, habit, f, style, minRR, req.AllowReversal, lowConfAction, directionBias, "行情抓取失败，回退模板生成", tradeCfg)
		writeJSON(w, http.StatusOK, map[string]any{"generated": fb, "fallback": true})
		return
	}

	ind := indicators.Calculate(candles)
	trend := indicators.AnalyzeTrend(candles, ind)
	levels := indicators.AnalyzeLevels(candles, ind)
	cur := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	change := (cur.Close - prev.Close) / prev.Close * 100

	gen, usedFallback := generatePreferenceByLLM(
		symbol, habit, f, cur.Close, change, trend.Overall, ind, levels,
		style, minRR, req.AllowReversal, lowConfAction, directionBias, tradeCfg,
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"generated": gen,
		"fallback":  usedFallback,
		"market": map[string]any{
			"symbol":           symbol,
			"timeframe":        f,
			"price":            cur.Close,
			"price_change_pct": change,
			"trend":            trend.Overall,
			"rsi":              ind.RSI,
			"macd":             ind.MACD,
			"macd_signal":      ind.MACDSignal,
			"ema7":             ind.EMA12,
			"ema25":            ind.EMA26,
			"ema99":            ind.SMA50,
			"resistance":       levels.StaticResistance,
			"support":          levels.StaticSupport,
			"selection": map[string]any{
				"strategy_style":  style,
				"min_rr":          minRR,
				"allow_reversal":  req.AllowReversal,
				"low_conf_action": lowConfAction,
				"direction_bias":  directionBias,
				"live_execution": map[string]any{
					"position_sizing_mode":       tradeCfg.PositionSizingMode,
					"high_confidence_amount":     tradeCfg.HighConfidenceAmount,
					"low_confidence_amount":      tradeCfg.LowConfidenceAmount,
					"high_confidence_margin_pct": tradeCfg.HighConfidenceMarginPct,
					"low_confidence_margin_pct":  tradeCfg.LowConfidenceMarginPct,
					"leverage":                   tradeCfg.Leverage,
				},
			},
		},
	})
}

func timeframeByHabit(h string) string {
	switch strings.TrimSpace(h) {
	case "10m":
		return "15m"
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "1D", "5D", "30D", "90D":
		return "1d"
	default:
		return "1h"
	}
}

func generatePreferenceByLLM(
	symbol, habit, tf string,
	price, change float64,
	overall string,
	ind any,
	levels any,
	style string,
	minRR float64,
	allowReversal bool,
	lowConfAction string,
	directionBias string,
	tradeCfg config.TradeConfig,
) (generatedPreference, bool) {
	apiKey := strings.TrimSpace(config.Config.AIAPIKey)
	baseURL := strings.TrimSpace(config.Config.AIBaseURL)
	model := strings.TrimSpace(config.Config.AIModel)
	if model == "" {
		model = "chat-model"
	}
	if apiKey == "" || baseURL == "" {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI未配置，回退模板生成", tradeCfg), true
	}

	prompt := map[string]any{
		"task": "Generate a practical trading preference prompt and strategy template from selected options and current market state.",
		"selected": map[string]any{
			"symbol":          symbol,
			"habit":           habit,
			"timeframe":       tf,
			"strategy_style":  style,
			"min_rr":          minRR,
			"allow_reversal":  allowReversal,
			"low_conf_action": lowConfAction,
			"direction_bias":  directionBias,
			"live_execution": map[string]any{
				"position_sizing_mode":       tradeCfg.PositionSizingMode,
				"high_confidence_amount":     tradeCfg.HighConfidenceAmount,
				"low_confidence_amount":      tradeCfg.LowConfidenceAmount,
				"high_confidence_margin_pct": tradeCfg.HighConfidenceMarginPct,
				"low_confidence_margin_pct":  tradeCfg.LowConfidenceMarginPct,
				"leverage":                   tradeCfg.Leverage,
			},
		},
		"market": map[string]any{
			"price":         price,
			"change_pct":    change,
			"overall_trend": overall,
			"technical":     ind,
			"levels":        levels,
		},
		"requirements": []string{
			"Output strict JSON only",
			"preference_prompt must include entry/SL/TP/RR and HOLD condition",
			"generator_prompt must include ${symbol} and ${habit}",
			"do not output fixed order amount or fixed leverage",
			"actual order size/margin/leverage must follow live execution settings",
			fmt.Sprintf("RR must be >= %.4f unless strict no-trade", minRR),
		},
		"schema": map[string]any{
			"strategy_name":     "string",
			"preference_prompt": "string",
			"generator_prompt":  "string",
			"logic":             "string",
			"basis":             "string",
		},
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a quant strategy architect. Return strict JSON only."},
			{"role": "user", "content": mustJSON(prompt)},
		},
		"temperature": 0.2,
		"stream":      false,
	}
	endpoint, err := llmapi.ResolveChatEndpoint(baseURL)
	if err != nil {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI_BASE_URL配置错误，回退模板生成", tradeCfg), true
	}
	rawReq, _ := json.Marshal(body)
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(rawReq))
	if err != nil {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "请求构建失败，回退模板生成", tradeCfg), true
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	cli := &http.Client{Timeout: 45 * time.Second}
	resp, err := cli.Do(httpReq)
	if err != nil {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI请求失败，回退模板生成", tradeCfg), true
	}
	defer resp.Body.Close()
	rawResp, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		reason := fmt.Sprintf("AI响应异常(HTTP %d)", resp.StatusCode)
		bodyText := strings.TrimSpace(string(rawResp))
		if bodyText != "" {
			if len(bodyText) > 180 {
				bodyText = bodyText[:180] + "..."
			}
			reason += ": " + bodyText
		}
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason+"，回退模板生成", tradeCfg), true
	}
	var chat llmChatResponse
	if err := json.Unmarshal(rawResp, &chat); err != nil || len(chat.Choices) == 0 {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI解析失败，回退模板生成", tradeCfg), true
	}
	content := chat.Choices[0].Message.Content
	recordLLMUsage("strategy_generator", mustJSON(prompt), content)
	obj, ok := extractJSONObject(content)
	if !ok {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI未输出JSON，回退模板生成", tradeCfg), true
	}
	var out generatedPreference
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI JSON无效，回退模板生成", tradeCfg), true
	}
	out.StrategyName = strings.TrimSpace(out.StrategyName)
	out.PreferencePrompt = strings.TrimSpace(out.PreferencePrompt)
	out.GeneratorPrompt = strings.TrimSpace(out.GeneratorPrompt)
	out.Logic = strings.TrimSpace(out.Logic)
	out.Basis = strings.TrimSpace(out.Basis)
	if out.StrategyName == "" || out.PreferencePrompt == "" || out.GeneratorPrompt == "" {
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, "AI内容不完整，回退模板生成", tradeCfg), true
	}
	if !strings.Contains(out.GeneratorPrompt, "${symbol}") || !strings.Contains(out.GeneratorPrompt, "${habit}") {
		out.GeneratorPrompt = ensureGeneratorVars(out.GeneratorPrompt)
	}
	return out, false
}

func fallbackGeneratedPreference(
	symbol, habit, tf, style string,
	minRR float64,
	allowReversal bool,
	lowConfAction, directionBias, reason string,
	tradeCfg config.TradeConfig,
) generatedPreference {
	strategyName := fmt.Sprintf("AI_%s_%s_%s", symbol, habit, time.Now().Format("20060102"))
	execDesc := fmt.Sprintf(
		"执行参数以实盘设置为准：mode=%s, high_amount=%.6f, low_amount=%.6f, high_margin=%.2f%%, low_margin=%.2f%%, leverage=%d。",
		tradeCfg.PositionSizingMode,
		tradeCfg.HighConfidenceAmount,
		tradeCfg.LowConfidenceAmount,
		tradeCfg.HighConfidenceMarginPct*100,
		tradeCfg.LowConfidenceMarginPct*100,
		tradeCfg.Leverage,
	)
	preference := fmt.Sprintf(`交易风格：%s（周期=%s），策略样式=%s，方向偏好=%s。
+方向判定：Score = w1*Trend + w2*Momentum + w3*Volatility + w4*Volume。
+入场条件：关键支撑/阻力、突破回踩、均线共振。
+风险收益：盈亏比（盈利/亏损）=|TP-Entry|/|Entry-SL|，目标>=%.2f。
+仓位与杠杆：由实盘执行参数与风控引擎统一决定，不在策略内固定金额。
+反转策略：allow_reversal=%t；低信心处理=%s。
+若条件不足，返回HOLD并给出触发价位。
%s`, habit, tf, style, directionBias, minRR, allowReversal, lowConfAction, execDesc)
	generator := `你是资深量化策略研究员。请为 ${symbol} 在 ${habit} 交易习惯下生成一套可执行自动策略。
按以下结构输出：市场状态识别、关键位定义、入场与出场（首选+备选）、风险管理（仅定义风控规则，不固定下单金额与杠杆）、观望与失效条件、回测建议。`
	return generatedPreference{
		StrategyName:     strategyName,
		PreferencePrompt: preference,
		GeneratorPrompt:  generator,
		Logic:            "按市场状态识别 -> 多因子确认 -> 风控过滤 -> 执行建议四层生成。",
		Basis:            "基于实时K线、EMA/RSI/MACD/量能、支撑阻力与实盘执行参数约束。" + reason,
	}
}

func ensureGeneratorVars(in string) string {
	base := strings.TrimSpace(in)
	if base == "" {
		base = "请生成可执行自动策略。"
	}
	if !strings.Contains(base, "${symbol}") {
		base = "针对 ${symbol}：" + base
	}
	if !strings.Contains(base, "${habit}") {
		base += "（交易习惯：${habit}）"
	}
	return base
}
