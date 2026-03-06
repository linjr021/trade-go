package server

import (
	"fmt"
	"strings"
	"time"
	"trade-go/config"
)

type habitProfile struct {
	Habit             string  `json:"habit"`
	Label             string  `json:"label"`
	Timeframe         string  `json:"timeframe"`
	MaxLeverage       int     `json:"max_leverage"`
	MaxDrawdownPct    float64 `json:"max_drawdown_pct"`
	MaxRiskPerTrade   float64 `json:"max_risk_per_trade_pct"`
	AllowAddPosition  bool    `json:"allow_add_position"`
	HoldBarsMin       int     `json:"hold_bars_min"`
	HoldBarsMax       int     `json:"hold_bars_max"`
	Description       string  `json:"description"`
	ExecutionHint     string  `json:"execution_hint"`
	PreferredDataSpan int     `json:"preferred_data_span"`
}

type strategySkillPackage struct {
	Version         string                 `json:"version"`
	GeneratedAt     string                 `json:"generated_at"`
	Workflow        []string               `json:"workflow"`
	Symbol          string                 `json:"symbol"`
	Habit           string                 `json:"habit"`
	HabitProfile    habitProfile           `json:"habit_profile"`
	SpecBuilder     map[string]any         `json:"spec_builder"`
	StrategyDraft   map[string]any         `json:"strategy_draft"`
	Optimizer       map[string]any         `json:"optimizer"`
	RiskReviewer    map[string]any         `json:"risk_reviewer"`
	ReleasePackager map[string]any         `json:"release_packager"`
	RuntimeContext  map[string]any         `json:"runtime_context"`
	Metadata        map[string]interface{} `json:"metadata"`
}

func normalizeHabitInput(h string) string {
	raw := strings.TrimSpace(h)
	if raw == "" {
		return "1h"
	}
	key := strings.ToLower(raw)
	profiles := loadHabitProfiles()
	for _, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p.Habit), raw) {
			return strings.TrimSpace(p.Habit)
		}
	}
	for _, p := range profiles {
		if strings.ToLower(strings.TrimSpace(p.Habit)) == key {
			return strings.TrimSpace(p.Habit)
		}
	}
	switch key {
	case "1d":
		return "1D"
	case "5d":
		return "5D"
	case "30d":
		return "30D"
	case "90d":
		return "90D"
	default:
		return strings.TrimSpace(raw)
	}
}

func habitProfileOf(habit string) habitProfile {
	normalized := normalizeHabitInput(habit)
	profiles := loadHabitProfiles()
	for _, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p.Habit), normalized) {
			return p
		}
	}
	for _, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p.Habit), "1h") {
			return p
		}
	}
	if len(profiles) > 0 {
		return profiles[0]
	}
	defaults := defaultHabitProfiles()
	for _, p := range defaults {
		if strings.EqualFold(strings.TrimSpace(p.Habit), "1h") {
			return p
		}
	}
	return defaults[0]
}

func clampByProfile(value float64, min float64, max float64) float64 {
	v := value
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func buildStrategySkillPackage(
	symbol string,
	habit string,
	style string,
	minRR float64,
	allowReversal bool,
	lowConfAction string,
	directionBias string,
	tradeCfg config.TradeConfig,
	market map[string]any,
) strategySkillPackage {
	workflowCfg := loadSkillWorkflowConfig()
	workflowSteps := enabledSkillWorkflowSteps(workflowCfg)
	profile := habitProfileOf(habit)
	habit = profile.Habit
	minRRFloor := workflowCfg.Constraints.MinProfitLossFloor
	if minRRFloor <= 0 {
		minRRFloor = 1.5
	}
	minRR = clampByProfile(minRR, minRRFloor, 10.0)
	hardMaxLeverage := profile.MaxLeverage
	if workflowCfg.Constraints.MaxLeverageCap > 0 && workflowCfg.Constraints.MaxLeverageCap < hardMaxLeverage {
		hardMaxLeverage = workflowCfg.Constraints.MaxLeverageCap
	}
	if tradeCfg.Leverage > 0 && tradeCfg.Leverage < hardMaxLeverage {
		hardMaxLeverage = tradeCfg.Leverage
	}
	maxRiskPerTrade := profile.MaxRiskPerTrade
	if workflowCfg.Constraints.MaxRiskPerTradeCap > 0 && workflowCfg.Constraints.MaxRiskPerTradeCap < maxRiskPerTrade {
		maxRiskPerTrade = workflowCfg.Constraints.MaxRiskPerTradeCap
	}
	if tradeCfg.MaxRiskPerTradePct > 0 && tradeCfg.MaxRiskPerTradePct < maxRiskPerTrade {
		maxRiskPerTrade = tradeCfg.MaxRiskPerTradePct
	}
	maxDrawdown := profile.MaxDrawdownPct
	if workflowCfg.Constraints.MaxDrawdownCapPct > 0 && workflowCfg.Constraints.MaxDrawdownCapPct < maxDrawdown {
		maxDrawdown = workflowCfg.Constraints.MaxDrawdownCapPct
	}
	if tradeCfg.MaxDrawdownPct > 0 && tradeCfg.MaxDrawdownPct < maxDrawdown {
		maxDrawdown = tradeCfg.MaxDrawdownPct
	}
	blockTradeOnSkillFail := workflowCfg.Constraints.BlockTradeOnSkillErr

	specBuilder := map[string]any{
		"goal": "将交易习惯与风险边界转为可执行约束",
		"hard_constraints": map[string]any{
			"max_leverage":              hardMaxLeverage,
			"max_drawdown_pct":          maxDrawdown,
			"max_risk_per_trade_pct":    maxRiskPerTrade,
			"allow_add_position":        profile.AllowAddPosition,
			"min_profit_loss_ratio":     minRR,
			"block_trade_on_skill_fail": blockTradeOnSkillFail,
		},
		"position_constraints": map[string]any{
			"position_sizing_mode":       tradeCfg.PositionSizingMode,
			"high_confidence_amount":     tradeCfg.HighConfidenceAmount,
			"low_confidence_amount":      tradeCfg.LowConfidenceAmount,
			"high_confidence_margin_pct": tradeCfg.HighConfidenceMarginPct,
			"low_confidence_margin_pct":  tradeCfg.LowConfidenceMarginPct,
		},
		"workflow_step_policies": workflowCfg.Steps,
	}

	strategyDraft := map[string]any{
		"goal":  "输出仅结构化 DSL/JSON 的策略草案",
		"style": style,
		"dsl_outline": []string{
			"market_state",
			"key_levels",
			"entry_rules",
			"exit_rules",
			"risk_filters",
			"hold_conditions",
			"invalidation_conditions",
		},
		"direction_bias":  directionBias,
		"allow_reversal":  allowReversal,
		"low_conf_action": lowConfAction,
	}

	optimizer := map[string]any{
		"goal":           "基于回测做迭代，仅调参数或有限改规则",
		"editable_scope": []string{"entry_threshold", "exit_threshold", "filter_strength", "rr_floor", "time_filter"},
		"frozen_scope":   []string{"max_leverage", "max_drawdown_pct", "max_risk_per_trade_pct", "order_reconcile_required"},
		"target_metrics": []string{"total_pnl", "win_rate", "profit_loss_ratio", "max_drawdown"},
	}

	riskReviewer := map[string]any{
		"goal": "识别过拟合、脆弱点、极端行情风险暴露",
		"checks": []string{
			"walk_forward_consistency",
			"volatility_regime_shift",
			"liquidity_shock_exposure",
			"consecutive_losses_stop",
			"daily_loss_stop",
		},
		"pass_gate": map[string]any{
			"max_drawdown_pct":      maxDrawdown,
			"min_profit_loss_ratio": minRR,
			"hard_fail_action": func() string {
				if blockTradeOnSkillFail {
					return "HOLD"
				}
				return "WARN_ONLY"
			}(),
		},
	}

	releasePackager := map[string]any{
		"goal": "打包可上线策略包（版本、摘要、监控、回滚）",
		"required_fields": []string{
			"strategy_version",
			"change_summary",
			"runtime_monitors",
			"rollback_conditions",
			"shadow_run_plan",
		},
		"rollback_conditions": []string{
			fmt.Sprintf("intraday_drawdown > %.2f%%", maxDrawdown*100),
			"api_order_reconcile_failures >= 3",
			"continuous_loss_streak threshold reached",
		},
	}

	return strategySkillPackage{
		Version:         "skill-pipeline/v1",
		GeneratedAt:     time.Now().Format(time.RFC3339),
		Workflow:        workflowSteps,
		Symbol:          symbol,
		Habit:           habit,
		HabitProfile:    profile,
		SpecBuilder:     specBuilder,
		StrategyDraft:   strategyDraft,
		Optimizer:       optimizer,
		RiskReviewer:    riskReviewer,
		ReleasePackager: releasePackager,
		RuntimeContext: map[string]any{
			"execution_exchange": func() string {
				ex := strings.ToLower(strings.TrimSpace(config.Config.ActiveExchange))
				if ex == "okx" {
					return "okx"
				}
				return "binance"
			}(),
			"default_symbol": tradeCfg.Symbol,
			"timeframe":      profile.Timeframe,
		},
		Metadata: map[string]interface{}{
			"source":              "strategy-builder-habit-input",
			"market":              market,
			"workflow_config":     workflowCfg,
			"workflow_steps_used": workflowSteps,
		},
	}
}
