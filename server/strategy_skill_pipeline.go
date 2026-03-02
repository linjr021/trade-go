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
	switch strings.TrimSpace(strings.ToLower(h)) {
	case "10m":
		return "10m"
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "1d":
		return "1D"
	case "5d":
		return "5D"
	case "30d":
		return "30D"
	case "90d":
		return "90D"
	default:
		return "1h"
	}
}

func habitProfileOf(habit string) habitProfile {
	switch normalizeHabitInput(habit) {
	case "10m":
		return habitProfile{
			Habit:             "10m",
			Label:             "超短线",
			Timeframe:         "15m",
			MaxLeverage:       30,
			MaxDrawdownPct:    0.04,
			MaxRiskPerTrade:   0.008,
			AllowAddPosition:  false,
			HoldBarsMin:       1,
			HoldBarsMax:       16,
			Description:       "高频、轻仓、快进快出，优先执行明确触发条件。",
			ExecutionHint:     "触发条件不完整时保持 HOLD，减少噪音交易。",
			PreferredDataSpan: 120,
		}
	case "1h":
		return habitProfile{
			Habit:             "1h",
			Label:             "日内短线",
			Timeframe:         "1h",
			MaxLeverage:       20,
			MaxDrawdownPct:    0.06,
			MaxRiskPerTrade:   0.010,
			AllowAddPosition:  true,
			HoldBarsMin:       2,
			HoldBarsMax:       24,
			Description:       "兼顾趋势与结构，适合主流交易对日内执行。",
			ExecutionHint:     "信号冲突时优先观望，等待关键位确认。",
			PreferredDataSpan: 160,
		}
	case "4h":
		return habitProfile{
			Habit:             "4h",
			Label:             "波段",
			Timeframe:         "4h",
			MaxLeverage:       10,
			MaxDrawdownPct:    0.08,
			MaxRiskPerTrade:   0.012,
			AllowAddPosition:  true,
			HoldBarsMin:       4,
			HoldBarsMax:       40,
			Description:       "偏趋势波段，减少频繁交易，重视结构完整性。",
			ExecutionHint:     "重点观察突破回踩与均线共振。",
			PreferredDataSpan: 200,
		}
	case "1D":
		return habitProfile{
			Habit:             "1D",
			Label:             "日线趋势",
			Timeframe:         "1d",
			MaxLeverage:       6,
			MaxDrawdownPct:    0.10,
			MaxRiskPerTrade:   0.015,
			AllowAddPosition:  true,
			HoldBarsMin:       3,
			HoldBarsMax:       60,
			Description:       "低频趋势策略，强调资金曲线稳定性。",
			ExecutionHint:     "优先主趋势方向，逆势只做备选策略。",
			PreferredDataSpan: 220,
		}
	case "5D":
		return habitProfile{
			Habit:             "5D",
			Label:             "周内波段",
			Timeframe:         "1d",
			MaxLeverage:       5,
			MaxDrawdownPct:    0.10,
			MaxRiskPerTrade:   0.015,
			AllowAddPosition:  true,
			HoldBarsMin:       5,
			HoldBarsMax:       90,
			Description:       "周级别持仓，限制噪音交易与高杠杆。",
			ExecutionHint:     "以趋势延续为主，设置明确失效条件。",
			PreferredDataSpan: 240,
		}
	case "30D":
		return habitProfile{
			Habit:             "30D",
			Label:             "中期配置",
			Timeframe:         "1d",
			MaxLeverage:       3,
			MaxDrawdownPct:    0.12,
			MaxRiskPerTrade:   0.020,
			AllowAddPosition:  false,
			HoldBarsMin:       12,
			HoldBarsMax:       180,
			Description:       "中周期配置，强调回撤控制与风险暴露。",
			ExecutionHint:     "尽量减少频繁换向，趋势失效再切换。",
			PreferredDataSpan: 280,
		}
	case "90D":
		return habitProfile{
			Habit:             "90D",
			Label:             "长周期配置",
			Timeframe:         "1d",
			MaxLeverage:       2,
			MaxDrawdownPct:    0.15,
			MaxRiskPerTrade:   0.020,
			AllowAddPosition:  false,
			HoldBarsMin:       20,
			HoldBarsMax:       260,
			Description:       "长周期低频策略，优先风控与稳定收益。",
			ExecutionHint:     "避免噪音驱动交易，严格执行风控停机。",
			PreferredDataSpan: 320,
		}
	default:
		return habitProfileOf("1h")
	}
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
