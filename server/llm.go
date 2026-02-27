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
	"trade-go/trader"
)

type llmChatRequest struct {
	Message string         `json:"message"`
	Context map[string]any `json:"context"`
}

type llmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type llmSettingPatch struct {
	PositionSizingMode      *string  `json:"position_sizing_mode"`
	HighConfidenceAmount    *float64 `json:"high_confidence_amount"`
	LowConfidenceAmount     *float64 `json:"low_confidence_amount"`
	HighConfidenceMarginPct *float64 `json:"high_confidence_margin_pct"`
	LowConfidenceMarginPct  *float64 `json:"low_confidence_margin_pct"`
	Leverage                *int     `json:"leverage"`
	MaxRiskPerTradePct      *float64 `json:"max_risk_per_trade_pct"`
	MaxPositionPct          *float64 `json:"max_position_pct"`
	MaxConsecutiveLosses    *int     `json:"max_consecutive_losses"`
	MaxDailyLossPct         *float64 `json:"max_daily_loss_pct"`
	MaxDrawdownPct          *float64 `json:"max_drawdown_pct"`
	LiquidationBufferPct    *float64 `json:"liquidation_buffer_pct"`
}

func (s *Service) handleLLMChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req llmChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	apiKey := strings.TrimSpace(config.Config.AIAPIKey)
	baseURL := strings.TrimSpace(config.Config.AIBaseURL)
	model := strings.TrimSpace(config.Config.AIModel)
	if model == "" {
		model = "chat-model"
	}
	if apiKey == "" || baseURL == "" {
		writeError(w, http.StatusBadRequest, "AI_API_KEY/AI_BASE_URL 未配置")
		return
	}

	cfg := s.bot.TradeConfig()
	prompt := fmt.Sprintf(`你是交易系统参数助手。基于用户意图决定是否修改配置。
当前配置:
%s

规则:
1) 仅修改与用户明确要求相关的字段。
2) 若用户未要求修改，settings_patch 返回空对象 {}。
3) 输出严格 JSON，不要 Markdown。

输出格式:
{"reply":"给用户的简短回复","settings_patch":{"position_sizing_mode":"contracts|margin_pct","high_confidence_amount":0.01,"low_confidence_amount":0.005,"high_confidence_margin_pct":0.1,"low_confidence_margin_pct":0.05,"leverage":10,"max_risk_per_trade_pct":0.01,"max_position_pct":0.2,"max_consecutive_losses":3,"max_daily_loss_pct":0.05,"max_drawdown_pct":0.12,"liquidation_buffer_pct":0.02}}

用户消息:
%s`, mustJSON(cfg), msg)

	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是严谨的量化交易参数助手。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
		"stream":      false,
	}
	rawReq, _ := json.Marshal(body)
	httpReq, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(rawReq))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	cli := &http.Client{Timeout: 45 * time.Second}
	resp, err := cli.Do(httpReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "LLM 请求失败")
		return
	}
	defer resp.Body.Close()
	rawResp, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("LLM HTTP %d", resp.StatusCode))
		return
	}
	var chat llmChatResponse
	if err := json.Unmarshal(rawResp, &chat); err != nil || len(chat.Choices) == 0 {
		writeError(w, http.StatusBadGateway, "LLM 响应解析失败")
		return
	}
	content := chat.Choices[0].Message.Content
	recordLLMUsage("chat_assistant", prompt, content)
	obj, ok := extractJSONObject(content)
	if !ok {
		writeError(w, http.StatusBadGateway, "LLM 未返回可解析JSON")
		return
	}
	var out struct {
		Reply         string         `json:"reply"`
		SettingsPatch map[string]any `json:"settings_patch"`
	}
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		writeError(w, http.StatusBadGateway, "LLM JSON 格式无效")
		return
	}

	applied := false
	var appliedCfg config.TradeConfig
	if len(out.SettingsPatch) > 0 {
		patchRaw, _ := json.Marshal(out.SettingsPatch)
		var patch llmSettingPatch
		if err := json.Unmarshal(patchRaw, &patch); err == nil {
			newCfg, upErr := s.bot.UpdateTradeSettings(trader.TradeSettingsUpdate{
				PositionSizingMode:      patch.PositionSizingMode,
				HighConfidenceAmount:    patch.HighConfidenceAmount,
				LowConfidenceAmount:     patch.LowConfidenceAmount,
				HighConfidenceMarginPct: patch.HighConfidenceMarginPct,
				LowConfidenceMarginPct:  patch.LowConfidenceMarginPct,
				Leverage:                patch.Leverage,
				MaxRiskPerTradePct:      patch.MaxRiskPerTradePct,
				MaxPositionPct:          patch.MaxPositionPct,
				MaxConsecutiveLosses:    patch.MaxConsecutiveLosses,
				MaxDailyLossPct:         patch.MaxDailyLossPct,
				MaxDrawdownPct:          patch.MaxDrawdownPct,
				LiquidationBufferPct:    patch.LiquidationBufferPct,
			})
			if upErr == nil {
				applied = true
				appliedCfg = newCfg
			} else {
				out.Reply = strings.TrimSpace(out.Reply + "（参数未应用：" + upErr.Error() + "）")
			}
		}
	}
	if strings.TrimSpace(out.Reply) == "" {
		out.Reply = "已完成分析。"
	}
	result := map[string]any{
		"reply":          out.Reply,
		"settings_patch": out.SettingsPatch,
		"applied":        applied,
	}
	if applied {
		result["trade_config"] = tradeConfigMap(appliedCfg)
	}
	writeJSON(w, http.StatusOK, result)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func extractJSONObject(s string) (string, bool) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return "", false
	}
	return s[start : end+1], true
}

func tradeConfigMap(cfg config.TradeConfig) map[string]any {
	return map[string]any{
		"symbol":                     cfg.Symbol,
		"amount":                     cfg.Amount,
		"high_confidence_amount":     cfg.HighConfidenceAmount,
		"low_confidence_amount":      cfg.LowConfidenceAmount,
		"position_sizing_mode":       cfg.PositionSizingMode,
		"high_confidence_margin_pct": cfg.HighConfidenceMarginPct,
		"low_confidence_margin_pct":  cfg.LowConfidenceMarginPct,
		"leverage":                   cfg.Leverage,
		"timeframe":                  cfg.Timeframe,
		"test_mode":                  cfg.TestMode,
		"data_points":                cfg.DataPoints,
		"max_risk_per_trade_pct":     cfg.MaxRiskPerTradePct,
		"max_position_pct":           cfg.MaxPositionPct,
		"max_consecutive_losses":     cfg.MaxConsecutiveLosses,
		"max_daily_loss_pct":         cfg.MaxDailyLossPct,
		"max_drawdown_pct":           cfg.MaxDrawdownPct,
		"liquidation_buffer_pct":     cfg.LiquidationBufferPct,
	}
}
