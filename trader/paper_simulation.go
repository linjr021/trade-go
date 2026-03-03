package trader

import (
	"fmt"
	"math"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/indicators"
	"trade-go/models"
	"trade-go/risk"
)

type PaperSimulationInput struct {
	Symbol                  string
	Balance                 float64
	PositionSizingMode      string
	HighConfidenceAmount    float64
	LowConfidenceAmount     float64
	HighConfidenceMarginPct float64
	LowConfidenceMarginPct  float64
	Leverage                int
	EnabledStrategies       []string
}

type PaperSimulationResult struct {
	ID                 string                 `json:"id"`
	TS                 time.Time              `json:"ts"`
	Symbol             string                 `json:"symbol"`
	Signal             string                 `json:"signal"`
	Confidence         string                 `json:"confidence"`
	StrategyCombo      string                 `json:"strategy_combo"`
	Reason             string                 `json:"reason"`
	Price              float64                `json:"price"`
	StopLoss           float64                `json:"stop_loss"`
	TakeProfit         float64                `json:"take_profit"`
	Approved           bool                   `json:"approved"`
	ApprovedSize       float64                `json:"approved_size"`
	RiskReason         string                 `json:"risk_reason"`
	OrderPlan          map[string]any         `json:"order_plan"`
	ExecutionCode      string                 `json:"execution_code"`
	Leverage           int                    `json:"leverage"`
	PositionSizingMode string                 `json:"position_sizing_mode"`
	EnabledStrategies  []string               `json:"enabled_strategies"`
	Source             string                 `json:"source"`
	DryRun             bool                   `json:"dry_run"`
	PriceSnapshot      map[string]any         `json:"price_snapshot"`
	Meta               map[string]interface{} `json:"meta,omitempty"`
}

func (b *Bot) RunPaperSimulation(in PaperSimulationInput) (PaperSimulationResult, error) {
	now := time.Now()
	out := PaperSimulationResult{
		ID:                 fmt.Sprintf("paper-%d", now.UnixNano()),
		TS:                 now,
		Approved:           false,
		ExecutionCode:      "paper_pending",
		Source:             "paper_ai_dry_run",
		DryRun:             true,
		EnabledStrategies:  normalizePaperStrategies(in.EnabledStrategies),
		Meta:               map[string]interface{}{"workflow": "market-read>strategy-select>risk-plan>order-plan"},
		PositionSizingMode: "margin_pct",
	}

	simCfg := b.buildPaperConfig(in)
	out.Symbol = simCfg.Symbol
	out.Leverage = simCfg.Leverage
	out.PositionSizingMode = simCfg.PositionSizingMode

	balance := in.Balance
	if !isPositiveNumber(balance) {
		balance = 100
	}

	cycleID := fmt.Sprintf("paper_cycle_%d", now.UnixNano())

	marketReadAt := time.Now()
	pd, err := b.fetchPriceDataByConfig(simCfg)
	if err != nil {
		b.saveSkillStepAudit(cycleID, "market-read", "failed", "market_unavailable", "", marketReadAt,
			map[string]any{"symbol": simCfg.Symbol, "timeframe": simCfg.Timeframe, "paper": true},
			map[string]any{"error": err.Error()},
			"blocked")
		out.RiskReason = "market_unavailable"
		out.ExecutionCode = "paper_market_unavailable"
		return out, err
	}
	out.Price = pd.Price
	out.PriceSnapshot = map[string]any{
		"price":        pd.Price,
		"price_change": pd.PriceChange,
		"timestamp":    pd.Timestamp,
		"timeframe":    pd.Timeframe,
	}
	b.saveSkillStepAudit(cycleID, "market-read", "ok", "ok", "", marketReadAt,
		map[string]any{"symbol": simCfg.Symbol, "timeframe": simCfg.Timeframe, "paper": true},
		map[string]any{
			"price":        pd.Price,
			"price_change": pd.PriceChange,
			"timestamp":    pd.Timestamp.Format(time.RFC3339),
		},
		"continue")

	strategySelectAt := time.Now()
	signal := b.analyzeWithRetryWithStrategies(pd, nil, out.EnabledStrategies)
	out.Signal = strings.ToUpper(strings.TrimSpace(signal.Signal))
	out.Confidence = strings.ToUpper(strings.TrimSpace(signal.Confidence))
	out.Reason = strings.TrimSpace(signal.Reason)
	out.StopLoss = signal.StopLoss
	out.TakeProfit = signal.TakeProfit
	out.StrategyCombo = strings.TrimSpace(signal.StrategyCombo)
	if out.StrategyCombo == "" {
		out.StrategyCombo = "ai_hold_generic"
	}

	if signal.IsFallback {
		b.saveSkillStepAudit(cycleID, "strategy-select", "failed", "insufficient_signal", config.Config.AIModel, strategySelectAt,
			map[string]any{
				"price":       pd.Price,
				"position":    nil,
				"history_len": len(b.SignalHistory(10)),
				"paper":       true,
			},
			map[string]any{"signal": signal},
			"blocked")
		out.RiskReason = "insufficient_signal"
		out.ExecutionCode = "paper_strategy_fallback"
		if out.Signal == "" {
			out.Signal = "HOLD"
		}
		return out, nil
	}
	if ok, code, reason := validateSignalStrict(signal, pd.Price); !ok {
		b.saveSkillStepAudit(cycleID, "strategy-select", "failed", code, config.Config.AIModel, strategySelectAt,
			map[string]any{
				"price":       pd.Price,
				"position":    nil,
				"history_len": len(b.SignalHistory(10)),
				"paper":       true,
			},
			map[string]any{"signal": signal, "reason": reason},
			"blocked")
		out.RiskReason = reason
		out.ExecutionCode = code
		if out.Signal == "" {
			out.Signal = "HOLD"
		}
		return out, nil
	}
	b.saveSkillStepAudit(cycleID, "strategy-select", "ok", "ok", config.Config.AIModel, strategySelectAt,
		map[string]any{
			"price":       pd.Price,
			"position":    nil,
			"history_len": len(b.SignalHistory(10)),
			"paper":       true,
		},
		map[string]any{"signal": signal},
		"continue")

	riskPlanAt := time.Now()
	tradeAmount, allow, riskReason := buildRiskPositionByConfig(b.riskEngine, signal, pd, simCfg, balance)
	if !allow {
		b.saveSkillStepAudit(cycleID, "risk-plan", "failed", "risk_blocked", "", riskPlanAt,
			map[string]any{"signal": signal, "price": pd.Price, "paper": true},
			map[string]any{"approved": false, "reason": riskReason},
			"blocked")
		out.RiskReason = riskReason
		out.ExecutionCode = "paper_risk_blocked"
		return out, nil
	}
	b.saveSkillStepAudit(cycleID, "risk-plan", "ok", "ok", "", riskPlanAt,
		map[string]any{"signal": signal, "price": pd.Price, "paper": true},
		map[string]any{"approved": true, "size": tradeAmount},
		"continue")

	orderPlanAt := time.Now()
	planOK, planCode, planReason, planOutput := preflightOrderPlanByConfig(signal, pd, tradeAmount, simCfg, balance)
	out.OrderPlan = planOutput
	if !planOK {
		b.saveSkillStepAudit(cycleID, "order-plan", "failed", planCode, "", orderPlanAt,
			map[string]any{"signal": signal, "price": pd.Price, "size": tradeAmount, "paper": true},
			planOutput,
			"blocked")
		out.RiskReason = planReason
		out.ExecutionCode = planCode
		return out, nil
	}

	execCode := "paper_simulated"
	approved := strings.ToUpper(strings.TrimSpace(signal.Signal)) != "HOLD"
	if !approved {
		execCode = "hold"
	}
	out.Approved = approved
	out.ApprovedSize = tradeAmount
	out.ExecutionCode = execCode
	b.saveSkillStepAudit(cycleID, "order-plan", "ok", "ok", "", orderPlanAt,
		map[string]any{"signal": signal, "price": pd.Price, "size": tradeAmount, "paper": true},
		map[string]any{"plan": planOutput, "execution_code": execCode},
		"simulated")

	return out, nil
}

func (b *Bot) buildPaperConfig(in PaperSimulationInput) config.TradeConfig {
	cfg := b.TradeConfig()

	symbol := strings.ToUpper(strings.TrimSpace(in.Symbol))
	if symbol != "" {
		cfg.Symbol = symbol
	}

	mode := strings.ToLower(strings.TrimSpace(in.PositionSizingMode))
	if mode != "contracts" && mode != "margin_pct" {
		mode = strings.ToLower(strings.TrimSpace(cfg.PositionSizingMode))
	}
	if mode != "contracts" && mode != "margin_pct" {
		mode = "margin_pct"
	}
	cfg.PositionSizingMode = mode

	if isValidNumber(in.HighConfidenceAmount) {
		cfg.HighConfidenceAmount = clampFloat(in.HighConfidenceAmount, 0, 1_000_000)
	}
	if isValidNumber(in.LowConfidenceAmount) {
		cfg.LowConfidenceAmount = clampFloat(in.LowConfidenceAmount, 0, 1_000_000)
	}
	if isValidNumber(in.HighConfidenceMarginPct) {
		cfg.HighConfidenceMarginPct = clampFloat(in.HighConfidenceMarginPct, 0, 100) / 100
	}
	if isValidNumber(in.LowConfidenceMarginPct) {
		cfg.LowConfidenceMarginPct = clampFloat(in.LowConfidenceMarginPct, 0, 100) / 100
	}

	if in.Leverage > 0 {
		cfg.Leverage = in.Leverage
	}
	if cfg.Leverage < 1 {
		cfg.Leverage = 1
	}
	if cfg.Leverage > 150 {
		cfg.Leverage = 150
	}
	return cfg
}

func (b *Bot) fetchPriceDataByConfig(cfg config.TradeConfig) (models.PriceData, error) {
	candles, err := b.exchange.FetchOHLCV(cfg.Symbol, cfg.Timeframe, cfg.DataPoints)
	if err != nil {
		return models.PriceData{}, err
	}
	if len(candles) < 2 {
		return models.PriceData{}, fmt.Errorf("K线数据不足")
	}

	ind := indicators.Calculate(candles)
	trend := indicators.AnalyzeTrend(candles, ind)
	levels := indicators.AnalyzeLevels(candles, ind)

	cur := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	priceChange := (cur.Close - prev.Close) / prev.Close * 100

	return models.PriceData{
		Symbol:      cfg.Symbol,
		Price:       cur.Close,
		Timestamp:   cur.Timestamp,
		High:        cur.High,
		Low:         cur.Low,
		Volume:      cur.Volume,
		Timeframe:   cfg.Timeframe,
		PriceChange: priceChange,
		KlineData:   candles,
		Technical:   ind,
		Trend:       trend,
		Levels:      levels,
	}, nil
}

func (b *Bot) analyzeWithRetryWithStrategies(pd models.PriceData, pos *models.Position, enabledStrategies []string) models.TradeSignal {
	history := b.SignalHistory(0)
	for attempt := 0; attempt < 1; attempt++ {
		sig, err := b.aiClient.AnalyzeWithStrategies(pd, pos, history, enabledStrategies)
		if err != nil {
			fmt.Printf("第%d次 AI 分析失败: %v\n", attempt+1, err)
			continue
		}
		if !sig.IsFallback {
			return sig
		}
	}
	fb := models.TradeSignal{
		Signal:        "HOLD",
		Reason:        "AI 分析失败，采取保守策略",
		StopLoss:      pd.Price * 0.98,
		TakeProfit:    pd.Price * 1.02,
		Confidence:    "LOW",
		StrategyCombo: "fallback_conservative",
		IsFallback:    true,
		Timestamp:     time.Now(),
	}
	return fb
}

func buildRiskPositionByConfig(engine *risk.Engine, signal models.TradeSignal, pd models.PriceData, cfg config.TradeConfig, balance float64) (float64, bool, string) {
	if strings.ToUpper(strings.TrimSpace(signal.Signal)) == "HOLD" {
		return 0, true, ""
	}
	if !isPositiveNumber(balance) {
		return 0, false, "模拟保证金无效"
	}
	suggested := suggestedAmountByConfidence(signal.Confidence, cfg, balance, pd.Price)
	snapshot := risk.Snapshot{
		Balance:       balance,
		CurrentEquity: balance,
		PeakEquity:    balance,
	}
	plan := engine.BuildOrderPlan(risk.OrderPlanInput{
		Price:         pd.Price,
		StopLoss:      signal.StopLoss,
		Confidence:    signal.Confidence,
		SuggestedSize: suggested,
		Leverage:      cfg.Leverage,
	}, snapshot)
	if !plan.Approved {
		return 0, false, plan.Reason
	}
	size := math.Round(plan.Size*10000) / 10000
	if size <= 0 {
		return 0, false, "风控后仓位过小"
	}
	return size, true, ""
}

func preflightOrderPlanByConfig(signal models.TradeSignal, pd models.PriceData, tradeAmount float64, cfg config.TradeConfig, balance float64) (bool, string, string, map[string]any) {
	out := map[string]any{
		"signal":       signal.Signal,
		"confidence":   signal.Confidence,
		"trade_amount": tradeAmount,
		"price":        pd.Price,
		"leverage":     cfg.Leverage,
		"paper":        true,
	}
	if strings.ToUpper(strings.TrimSpace(signal.Signal)) == "HOLD" {
		out["action"] = "hold"
		return true, "ok", "", out
	}
	if tradeAmount <= 0 {
		out["reason"] = "trade_amount <= 0"
		return false, "order_plan_invalid", "开仓数量必须大于 0", out
	}
	if cfg.Leverage <= 0 {
		out["reason"] = "invalid leverage"
		return false, "order_plan_invalid", "杠杆配置无效", out
	}
	if !isPositiveNumber(balance) {
		out["reason"] = "invalid_balance"
		return false, "balance_unavailable", "模拟保证金无效", out
	}
	requiredMargin := pd.Price * tradeAmount / float64(cfg.Leverage)
	out["balance"] = balance
	out["required_margin"] = requiredMargin
	if requiredMargin > balance*0.8 {
		out["reason"] = "insufficient_margin"
		return false, "insufficient_margin", "保证金不足", out
	}
	return true, "ok", "", out
}

func normalizePaperStrategies(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func isValidNumber(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func isPositiveNumber(v float64) bool {
	return isValidNumber(v) && v > 0
}

func clampFloat(v, min, max float64) float64 {
	if !isValidNumber(v) {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
