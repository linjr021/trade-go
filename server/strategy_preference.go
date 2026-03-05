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
	habit := normalizeHabitInput(req.Habit)
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
	profile := habitProfileOf(habit)
	f := profile.Timeframe
	tradeCfg := s.bot.TradeConfig()
	workflowCfg := loadSkillWorkflowConfig()
	workflowVersion := strings.TrimSpace(workflowCfg.Version)
	if workflowVersion == "" {
		workflowVersion = "skill-workflow/v1"
	}
	workflowChain := enabledSkillWorkflowSteps(workflowCfg)
	activateGenerated := func(gen generatedPreference, source string) (generatedPreference, generatedStrategyRecord, []string, generatedStrategyStore, error) {
		gen.StrategyName = buildStandardStrategyName(symbol, habit, style, false)
		record := generatedStrategyRecord{
			ID:               newGeneratedStrategyID("workflow"),
			Name:             strings.TrimSpace(gen.StrategyName),
			RuleKey:          buildStrategyRuleKey(symbol, habit, style, false),
			PreferencePrompt: strings.TrimSpace(gen.PreferencePrompt),
			GeneratorPrompt:  strings.TrimSpace(gen.GeneratorPrompt),
			Logic:            strings.TrimSpace(gen.Logic),
			Basis:            strings.TrimSpace(gen.Basis),
			CreatedAt:        time.Now().Format(time.RFC3339),
			LastUpdatedAt:    time.Now().Format(time.RFC3339),
			Source:           normalizeStrategySource(source),
			WorkflowVersion:  workflowVersion,
			WorkflowChain:    workflowChain,
		}
		if record.Source == "" {
			record.Source = "workflow_generated"
		}
		final, enabled, store, err := s.saveAndActivateGeneratedStrategy(record)
		if err != nil {
			return generatedPreference{}, generatedStrategyRecord{}, nil, generatedStrategyStore{}, err
		}
		gen.StrategyName = final.Name
		gen.PreferencePrompt = final.PreferencePrompt
		gen.GeneratorPrompt = final.GeneratorPrompt
		gen.Logic = final.Logic
		gen.Basis = final.Basis
		return gen, final, enabled, store, nil
	}

	client := exchange.NewClient()
	candles, err := client.FetchOHLCV(symbol, f, 120)
	if err != nil || len(candles) < 30 {
		fb := fallbackGeneratedPreference(symbol, habit, f, style, minRR, req.AllowReversal, lowConfAction, directionBias, "行情抓取失败，回退模板生成", tradeCfg)
		finalGenerated, stored, enabled, store, activateErr := activateGenerated(fb, "workflow_generated")
		if activateErr != nil {
			writeError(w, http.StatusInternalServerError, "策略生成成功但激活失败: "+activateErr.Error())
			return
		}
		skillPkg := buildStrategySkillPackage(
			symbol,
			habit,
			style,
			minRR,
			req.AllowReversal,
			lowConfAction,
			directionBias,
			tradeCfg,
			map[string]any{
				"symbol":    symbol,
				"timeframe": f,
				"error":     "market_data_unavailable",
			},
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"generated":            finalGenerated,
			"fallback":             true,
			"fallback_reason":      "行情抓取失败，回退模板生成",
			"skill_package":        skillPkg,
			"auto_activated":       true,
			"active_strategy":      stored.Name,
			"enabled_strategies":   enabled,
			"generated_strategy":   stored,
			"generated_strategies": store.Strategies,
			"market": map[string]any{
				"symbol":        symbol,
				"timeframe":     f,
				"habit_profile": habitProfileOf(habit),
			},
		})
		return
	}

	ind := indicators.Calculate(candles)
	trend := indicators.AnalyzeTrend(candles, ind)
	levels := indicators.AnalyzeLevels(candles, ind)
	cur := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	change := (cur.Close - prev.Close) / prev.Close * 100

	marketView := map[string]any{
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
	}
	skillPkg := buildStrategySkillPackage(
		symbol,
		habit,
		style,
		minRR,
		req.AllowReversal,
		lowConfAction,
		directionBias,
		tradeCfg,
		marketView,
	)

	gen, usedFallback, fallbackReason := generatePreferenceByLLM(
		symbol, habit, f, cur.Close, change, trend.Overall, ind, levels,
		style, minRR, req.AllowReversal, lowConfAction, directionBias, tradeCfg,
	)
	finalGenerated, stored, enabled, store, activateErr := activateGenerated(gen, "workflow_generated")
	if activateErr != nil {
		writeError(w, http.StatusInternalServerError, "策略生成成功但激活失败: "+activateErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generated":            finalGenerated,
		"fallback":             usedFallback,
		"fallback_reason":      strings.TrimSpace(fallbackReason),
		"skill_package":        skillPkg,
		"auto_activated":       true,
		"active_strategy":      stored.Name,
		"enabled_strategies":   enabled,
		"generated_strategy":   stored,
		"generated_strategies": store.Strategies,
		"market": map[string]any{
			"symbol":           symbol,
			"timeframe":        f,
			"habit_profile":    profile,
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
) (generatedPreference, bool, string) {
	apiKey := strings.TrimSpace(config.Config.AIAPIKey)
	baseURL := strings.TrimSpace(config.Config.AIBaseURL)
	model := strings.TrimSpace(config.Config.AIModel)
	if model == "" {
		model = "chat-model"
	}
	if apiKey == "" || baseURL == "" {
		reason := "AI未配置，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}

	workflowCfg := loadSkillWorkflowConfig()
	promptCfg := workflowCfg.Prompts
	profile := habitProfileOf(habit)
	workflowSteps := enabledSkillWorkflowSteps(workflowCfg)
	workflowStepLabels := make([]string, 0, len(workflowSteps))
	for _, sid := range workflowSteps {
		label := sid
		for _, st := range workflowCfg.Steps {
			if strings.TrimSpace(st.ID) != sid {
				continue
			}
			if name := strings.TrimSpace(st.Name); name != "" {
				label = name
			}
			break
		}
		workflowStepLabels = append(workflowStepLabels, label)
	}
	requirements := append([]string{}, promptCfg.StrategyGeneratorRequirements...)
	requirements = append(requirements,
		fmt.Sprintf("策略必须兼容当前工作流（%s）", strings.Join(workflowStepLabels, " -> ")),
		fmt.Sprintf("最小盈亏比（盈利/亏损）需 >= %.4f，除非明确严格不交易", minRR),
	)
	prompt := map[string]any{
		"task":               promptCfg.StrategyGeneratorTaskPrompt,
		"skill_workflow":     workflowStepLabels,
		"skill_workflow_ids": workflowSteps,
		"selected": map[string]any{
			"symbol":          symbol,
			"habit":           habit,
			"habit_profile":   profile,
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
		"requirements": requirements,
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
			{"role": "system", "content": promptCfg.StrategyGeneratorSystemPrompt},
			{"role": "user", "content": mustJSON(prompt)},
		},
		"temperature": 0.2,
		"stream":      false,
	}
	endpoint, err := llmapi.ResolveChatEndpoint(baseURL)
	if err != nil {
		reason := "AI_BASE_URL配置错误，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}
	rawReq, _ := json.Marshal(body)
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(rawReq))
	if err != nil {
		reason := "请求构建失败，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	cli := &http.Client{Timeout: 45 * time.Second}
	resp, err := cli.Do(httpReq)
	if err != nil {
		reason := "AI请求失败，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
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
		finalReason := reason + "，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, finalReason, tradeCfg), true, finalReason
	}
	var chat llmChatResponse
	if err := json.Unmarshal(rawResp, &chat); err != nil || len(chat.Choices) == 0 {
		reason := "AI解析失败，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}
	content := chat.Choices[0].Message.Content
	recordLLMUsageWithMeta("strategy_generator", model, mustJSON(prompt), content)
	obj, ok := extractJSONObject(content)
	if !ok {
		reason := "AI未输出JSON，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}
	var out generatedPreference
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		reason := "AI JSON无效，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}
	out.StrategyName = strings.TrimSpace(out.StrategyName)
	out.PreferencePrompt = strings.TrimSpace(out.PreferencePrompt)
	out.GeneratorPrompt = strings.TrimSpace(out.GeneratorPrompt)
	out.Logic = strings.TrimSpace(out.Logic)
	out.Basis = strings.TrimSpace(out.Basis)
	if out.PreferencePrompt == "" || out.GeneratorPrompt == "" {
		reason := "AI内容不完整，回退模板生成"
		return fallbackGeneratedPreference(symbol, habit, tf, style, minRR, allowReversal, lowConfAction, directionBias, reason, tradeCfg), true, reason
	}
	if !strings.Contains(out.GeneratorPrompt, "${symbol}") || !strings.Contains(out.GeneratorPrompt, "${habit}") {
		out.GeneratorPrompt = ensureGeneratorVars(out.GeneratorPrompt)
	}
	out.StrategyName = buildStandardStrategyName(symbol, habit, style, false)
	return out, false, ""
}

func fallbackGeneratedPreference(
	symbol, habit, tf, style string,
	minRR float64,
	allowReversal bool,
	lowConfAction, directionBias, reason string,
	tradeCfg config.TradeConfig,
) generatedPreference {
	strategyName := buildStandardStrategyName(symbol, habit, style, false)
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
+反转策略：允许反转=%t；低信心处理=%s。
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
