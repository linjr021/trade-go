package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	legacyStrategyGeneratorSystemPrompt = "You are a quant strategy architect. Return strict JSON only."
	legacyStrategyGeneratorTaskPrompt   = "Generate a practical trading preference prompt and strategy template from selected options and current market state."
)

var legacyStrategyGeneratorRequirementMap = map[string]string{
	"Output strict JSON only": "仅输出严格 JSON",
	"preference_prompt must include entry/SL/TP/RR and HOLD condition":      "preference_prompt 必须包含入场区、止损、止盈、盈亏比与 HOLD 条件",
	"generator_prompt must include ${symbol} and ${habit}":                  "generator_prompt 必须包含 ${symbol} 与 ${habit}",
	"do not output fixed order amount or fixed leverage":                    "不要输出固定下单金额或固定杠杆",
	"actual order size/margin/leverage must follow live execution settings": "实际下单张数/保证金/杠杆必须遵循实盘执行设置",
}

type skillWorkflowStep struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	TimeoutSec  int    `json:"timeout_sec"`
	MaxRetry    int    `json:"max_retry"`
	OnFail      string `json:"on_fail"`
}

type skillWorkflowConstraints struct {
	MaxLeverageCap       int     `json:"max_leverage_cap"`
	MaxDrawdownCapPct    float64 `json:"max_drawdown_cap_pct"`
	MaxRiskPerTradeCap   float64 `json:"max_risk_per_trade_cap_pct"`
	MinProfitLossFloor   float64 `json:"min_profit_loss_floor"`
	BlockTradeOnSkillErr bool    `json:"block_trade_on_skill_fail"`
}

type skillWorkflowPrompts struct {
	StrategyGeneratorSystemPrompt string   `json:"strategy_generator_system_prompt"`
	StrategyGeneratorTaskPrompt   string   `json:"strategy_generator_task_prompt"`
	StrategyGeneratorRequirements []string `json:"strategy_generator_requirements"`
	DecisionSystemPrompt          string   `json:"decision_system_prompt"`
	DecisionPolicyPrompt          string   `json:"decision_policy_prompt"`
}

type skillWorkflowConfig struct {
	Version     string                   `json:"version"`
	UpdatedAt   string                   `json:"updated_at"`
	Steps       []skillWorkflowStep      `json:"steps"`
	Constraints skillWorkflowConstraints `json:"constraints"`
	Prompts     skillWorkflowPrompts     `json:"prompts"`
}

func defaultSkillWorkflowConfigBuiltin() skillWorkflowConfig {
	return skillWorkflowConfig{
		Version:   "skill-workflow/v1",
		UpdatedAt: time.Now().Format(time.RFC3339),
		Steps: []skillWorkflowStep{
			{
				ID:          "spec-builder",
				Name:        "规格构建",
				Description: "交易习惯转执行约束（硬边界）",
				Enabled:     true,
				TimeoutSec:  8,
				MaxRetry:    1,
				OnFail:      "hard_fail",
			},
			{
				ID:          "strategy-draft",
				Name:        "策略草案",
				Description: "生成结构化策略草案",
				Enabled:     true,
				TimeoutSec:  16,
				MaxRetry:    1,
				OnFail:      "hold",
			},
			{
				ID:          "optimizer",
				Name:        "参数优化",
				Description: "回测驱动参数优化",
				Enabled:     true,
				TimeoutSec:  18,
				MaxRetry:    1,
				OnFail:      "hold",
			},
			{
				ID:          "risk-reviewer",
				Name:        "风险复核",
				Description: "过拟合与极端行情风险复核",
				Enabled:     true,
				TimeoutSec:  10,
				MaxRetry:    0,
				OnFail:      "hard_fail",
			},
			{
				ID:          "release-packager",
				Name:        "发布打包",
				Description: "打包上线策略版本与监控建议",
				Enabled:     true,
				TimeoutSec:  10,
				MaxRetry:    0,
				OnFail:      "hold",
			},
		},
		Constraints: skillWorkflowConstraints{
			MaxLeverageCap:       150,
			MaxDrawdownCapPct:    0.20,
			MaxRiskPerTradeCap:   0.03,
			MinProfitLossFloor:   1.5,
			BlockTradeOnSkillErr: true,
		},
		Prompts: skillWorkflowPrompts{
			StrategyGeneratorSystemPrompt: "你是量化策略架构师，只能返回严格 JSON。",
			StrategyGeneratorTaskPrompt:   "请基于用户选项与当前市场状态，生成可落地的交易偏好提示词与策略模板。",
			StrategyGeneratorRequirements: []string{
				"仅输出严格 JSON",
				"preference_prompt 必须包含入场区、止损、止盈、盈亏比与 HOLD 条件",
				"preference_prompt 优先使用相对规则（EMA/ATR/百分比区间），避免写死绝对价格；若给出绝对价位，需附带动态重算条件",
				"generator_prompt 必须包含 ${symbol} 与 ${habit}",
				"不要输出固定下单金额或固定杠杆",
				"实际下单张数/保证金/杠杆必须遵循实盘执行设置",
			},
			DecisionSystemPrompt: "你是专业量化交易决策引擎。你只能输出严格JSON，不要输出任何额外文本。你负责方向与SL/TP建议，仓位和风控由系统执行。",
			DecisionPolicyPrompt: "优先保护本金；信号冲突或不确定时返回HOLD；避免低置信度反转。",
		},
	}
}

func defaultSkillWorkflowConfig() skillWorkflowConfig {
	return loadAISettingsDocument().Workflow
}

func localizeLegacyWorkflowPrompts(in skillWorkflowPrompts, defaults skillWorkflowPrompts) skillWorkflowPrompts {
	out := in
	if strings.TrimSpace(out.StrategyGeneratorSystemPrompt) == legacyStrategyGeneratorSystemPrompt {
		out.StrategyGeneratorSystemPrompt = defaults.StrategyGeneratorSystemPrompt
	}
	if strings.TrimSpace(out.StrategyGeneratorTaskPrompt) == legacyStrategyGeneratorTaskPrompt {
		out.StrategyGeneratorTaskPrompt = defaults.StrategyGeneratorTaskPrompt
	}
	if len(out.StrategyGeneratorRequirements) > 0 {
		next := make([]string, 0, len(out.StrategyGeneratorRequirements))
		for _, item := range out.StrategyGeneratorRequirements {
			v := strings.TrimSpace(item)
			if v == "" {
				continue
			}
			if mapped, ok := legacyStrategyGeneratorRequirementMap[v]; ok {
				v = mapped
			}
			next = append(next, v)
		}
		if len(next) > 0 {
			out.StrategyGeneratorRequirements = next
		}
	}
	return out
}

func normalizeSkillWorkflowConfig(in skillWorkflowConfig) skillWorkflowConfig {
	d := defaultSkillWorkflowConfigBuiltin()
	out := in
	if strings.TrimSpace(out.Version) == "" {
		out.Version = d.Version
	}
	if strings.TrimSpace(out.UpdatedAt) == "" {
		out.UpdatedAt = time.Now().Format(time.RFC3339)
	}

	stepMap := map[string]skillWorkflowStep{}
	for _, st := range out.Steps {
		id := strings.TrimSpace(st.ID)
		if id == "" {
			continue
		}
		stepMap[id] = st
	}
	mergedSteps := make([]skillWorkflowStep, 0, len(d.Steps))
	for _, base := range d.Steps {
		cur := base
		if st, ok := stepMap[base.ID]; ok {
			cur.Enabled = st.Enabled
			if st.TimeoutSec > 0 {
				cur.TimeoutSec = st.TimeoutSec
			}
			if st.MaxRetry >= 0 {
				cur.MaxRetry = st.MaxRetry
			}
			if v := strings.TrimSpace(st.OnFail); v != "" {
				cur.OnFail = strings.ToLower(v)
			}
		}
		mergedSteps = append(mergedSteps, cur)
	}
	out.Steps = mergedSteps

	if out.Constraints.MaxLeverageCap <= 0 {
		out.Constraints.MaxLeverageCap = d.Constraints.MaxLeverageCap
	}
	if out.Constraints.MaxDrawdownCapPct <= 0 {
		out.Constraints.MaxDrawdownCapPct = d.Constraints.MaxDrawdownCapPct
	}
	if out.Constraints.MaxRiskPerTradeCap <= 0 {
		out.Constraints.MaxRiskPerTradeCap = d.Constraints.MaxRiskPerTradeCap
	}
	if out.Constraints.MinProfitLossFloor <= 0 {
		out.Constraints.MinProfitLossFloor = d.Constraints.MinProfitLossFloor
	}

	out.Prompts.StrategyGeneratorSystemPrompt = strings.TrimSpace(out.Prompts.StrategyGeneratorSystemPrompt)
	if out.Prompts.StrategyGeneratorSystemPrompt == "" {
		out.Prompts.StrategyGeneratorSystemPrompt = d.Prompts.StrategyGeneratorSystemPrompt
	}
	out.Prompts.StrategyGeneratorTaskPrompt = strings.TrimSpace(out.Prompts.StrategyGeneratorTaskPrompt)
	if out.Prompts.StrategyGeneratorTaskPrompt == "" {
		out.Prompts.StrategyGeneratorTaskPrompt = d.Prompts.StrategyGeneratorTaskPrompt
	}
	reqs := make([]string, 0, len(out.Prompts.StrategyGeneratorRequirements))
	for _, item := range out.Prompts.StrategyGeneratorRequirements {
		v := strings.TrimSpace(item)
		if v != "" {
			reqs = append(reqs, v)
		}
	}
	if len(reqs) == 0 {
		reqs = append(reqs, d.Prompts.StrategyGeneratorRequirements...)
	}
	out.Prompts.StrategyGeneratorRequirements = reqs
	out.Prompts.DecisionSystemPrompt = strings.TrimSpace(out.Prompts.DecisionSystemPrompt)
	if out.Prompts.DecisionSystemPrompt == "" {
		out.Prompts.DecisionSystemPrompt = d.Prompts.DecisionSystemPrompt
	}
	out.Prompts.DecisionPolicyPrompt = strings.TrimSpace(out.Prompts.DecisionPolicyPrompt)
	if out.Prompts.DecisionPolicyPrompt == "" {
		out.Prompts.DecisionPolicyPrompt = d.Prompts.DecisionPolicyPrompt
	}
	out.Prompts = localizeLegacyWorkflowPrompts(out.Prompts, d.Prompts)
	return out
}

func validateSkillWorkflowConfig(cfg skillWorkflowConfig) error {
	cfg = normalizeSkillWorkflowConfig(cfg)
	if len(cfg.Steps) == 0 {
		return fmt.Errorf("steps 不能为空")
	}
	seen := map[string]bool{}
	for _, st := range cfg.Steps {
		id := strings.TrimSpace(st.ID)
		if id == "" {
			return fmt.Errorf("step id 不能为空")
		}
		if seen[id] {
			return fmt.Errorf("step id 重复: %s", id)
		}
		seen[id] = true
		if st.TimeoutSec < 1 || st.TimeoutSec > 300 {
			return fmt.Errorf("%s timeout_sec 需在 1-300", id)
		}
		if st.MaxRetry < 0 || st.MaxRetry > 5 {
			return fmt.Errorf("%s max_retry 需在 0-5", id)
		}
		switch strings.ToLower(strings.TrimSpace(st.OnFail)) {
		case "hold", "hard_fail":
		default:
			return fmt.Errorf("%s on_fail 仅支持 hold/hard_fail", id)
		}
	}
	if cfg.Constraints.MaxLeverageCap < 1 || cfg.Constraints.MaxLeverageCap > 150 {
		return fmt.Errorf("max_leverage_cap 需在 1-150")
	}
	if cfg.Constraints.MaxDrawdownCapPct < 0.01 || cfg.Constraints.MaxDrawdownCapPct > 0.80 {
		return fmt.Errorf("max_drawdown_cap_pct 需在 0.01-0.80")
	}
	if cfg.Constraints.MaxRiskPerTradeCap < 0.001 || cfg.Constraints.MaxRiskPerTradeCap > 0.20 {
		return fmt.Errorf("max_risk_per_trade_cap_pct 需在 0.001-0.20")
	}
	if cfg.Constraints.MinProfitLossFloor < 1.0 || cfg.Constraints.MinProfitLossFloor > 10.0 {
		return fmt.Errorf("min_profit_loss_floor 需在 1.0-10.0")
	}
	if strings.TrimSpace(cfg.Prompts.StrategyGeneratorSystemPrompt) == "" {
		return fmt.Errorf("strategy_generator_system_prompt 不能为空")
	}
	if strings.TrimSpace(cfg.Prompts.StrategyGeneratorTaskPrompt) == "" {
		return fmt.Errorf("strategy_generator_task_prompt 不能为空")
	}
	if len(cfg.Prompts.StrategyGeneratorRequirements) == 0 {
		return fmt.Errorf("strategy_generator_requirements 至少包含 1 条")
	}
	if strings.TrimSpace(cfg.Prompts.DecisionSystemPrompt) == "" {
		return fmt.Errorf("decision_system_prompt 不能为空")
	}
	if strings.TrimSpace(cfg.Prompts.DecisionPolicyPrompt) == "" {
		return fmt.Errorf("decision_policy_prompt 不能为空")
	}
	return nil
}

func readSkillWorkflowConfig() (skillWorkflowConfig, error) {
	doc := loadAISettingsDocument()
	return normalizeSkillWorkflowConfig(doc.Workflow), nil
}

func writeSkillWorkflowConfig(cfg skillWorkflowConfig) error {
	doc := loadAISettingsDocument()
	doc.Workflow = normalizeSkillWorkflowConfig(cfg)
	return writeAISettingsDocument(doc)
}

func loadSkillWorkflowConfig() skillWorkflowConfig {
	cfg, err := readSkillWorkflowConfig()
	if err != nil {
		return defaultSkillWorkflowConfigBuiltin()
	}
	return normalizeSkillWorkflowConfig(cfg)
}

func applySkillWorkflowPromptsToEnv(cfg skillWorkflowConfig) {
	normalized := normalizeSkillWorkflowConfig(cfg)
	_ = os.Setenv("TRADING_AI_SYSTEM_PROMPT", normalized.Prompts.DecisionSystemPrompt)
	_ = os.Setenv("TRADING_AI_POLICY_PROMPT", normalized.Prompts.DecisionPolicyPrompt)
}

func enabledSkillWorkflowSteps(cfg skillWorkflowConfig) []string {
	names := make([]string, 0, len(cfg.Steps))
	for _, st := range cfg.Steps {
		if st.Enabled {
			names = append(names, st.ID)
		}
	}
	if len(names) == 0 {
		for _, st := range defaultSkillWorkflowConfigBuiltin().Steps {
			names = append(names, st.ID)
		}
	}
	return names
}

func (s *Service) handleSkillWorkflow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc := loadAISettingsDocument()
		writeJSON(w, http.StatusOK, map[string]any{
			"workflow":                normalizeSkillWorkflowConfig(doc.Workflow),
			"habit_profiles":          normalizeHabitProfiles(doc.HabitProfiles),
			"strategy_package_schema": doc.StrategyPackageSchema,
			"ai_settings_path":        aiSettingsPath,
			"updated_at":              doc.UpdatedAt,
		})
	case http.MethodPost:
		var req struct {
			Workflow              skillWorkflowConfig    `json:"workflow"`
			HabitProfiles         []habitProfile         `json:"habit_profiles"`
			StrategyPackageSchema map[string]interface{} `json:"strategy_package_schema"`
			ResetDefault          bool                   `json:"reset_default"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		doc := loadAISettingsDocument()
		cfg := req.Workflow
		if req.ResetDefault {
			doc = defaultAISettingsDocument()
			cfg = doc.Workflow
		} else {
			if len(req.HabitProfiles) > 0 {
				doc.HabitProfiles = normalizeHabitProfiles(req.HabitProfiles)
			}
			if len(req.StrategyPackageSchema) > 0 {
				doc.StrategyPackageSchema = req.StrategyPackageSchema
			}
			// Empty workflow payload means only update other AI settings (e.g. habit_profiles).
			if len(req.Workflow.Steps) == 0 {
				cfg = doc.Workflow
			}
		}
		cfg = normalizeSkillWorkflowConfig(cfg)
		if err := validateSkillWorkflowConfig(cfg); err != nil {
			writeError(w, http.StatusBadRequest, "skill workflow 校验失败: "+err.Error())
			return
		}
		doc.Workflow = cfg
		if err := writeAISettingsDocument(doc); err != nil {
			writeError(w, http.StatusInternalServerError, "AI 设置保存失败: "+err.Error())
			return
		}
		applySkillWorkflowPromptsToEnv(cfg)
		saved := loadAISettingsDocument()
		writeJSON(w, http.StatusOK, map[string]any{
			"message":                 "AI 工作流已更新",
			"workflow":                normalizeSkillWorkflowConfig(saved.Workflow),
			"habit_profiles":          normalizeHabitProfiles(saved.HabitProfiles),
			"strategy_package_schema": saved.StrategyPackageSchema,
			"ai_settings_path":        aiSettingsPath,
			"updated_at":              saved.UpdatedAt,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
