package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const promptSettingsPath = "data/prompt_settings.json"

type promptSettings struct {
	TradingAISystemPrompt           string `json:"trading_ai_system_prompt"`
	TradingAIPolicyPrompt           string `json:"trading_ai_policy_prompt"`
	StrategyGeneratorPromptTemplate string `json:"strategy_generator_prompt_template"`
}

func defaultPromptSettings() promptSettings {
	return promptSettings{
		TradingAISystemPrompt: `你是加密永续量化交易决策引擎，交易标的默认 ${symbol}。
你必须遵守：
1) 仅输出严格JSON；
2) 先判断市场状态（trend/range/breakout）再给信号；
3) 信号不充分或冲突时优先HOLD；
4) 你只负责方向、入场区、止损、止盈、盈亏比（盈利/亏损）；仓位由风控引擎执行；
5) 不得决定固定下单金额、固定仓位和固定杠杆；这些均由实盘设置与风控引擎执行。`,
		TradingAIPolicyPrompt: `硬边界：
1) 只能输出方向/入场区间/止损/止盈/盈亏比（盈利/亏损），不输出固定开仓金额或固定杠杆。
2) 最低盈亏比（盈利/亏损）门槛：盈亏比 = |TP-Entry| / |Entry-SL| >= 1.5，推荐>=2.0。
3) 若关键条件不足或冲突，必须输出HOLD并给出触发条件。
4) 非高信心不允许频繁反转，优先顺势交易。
5) 输出必须包含：首选策略、备选策略、入场区间、止损、目标、盈亏比估算。`,
		StrategyGeneratorPromptTemplate: `你是资深量化策略研究员。请为 ${symbol} 在 ${habit} 交易习惯下生成一套可执行自动策略。
按以下结构输出：

1) 市场状态识别
- 趋势/震荡/突破判定规则（必须可量化）

2) 关键位定义
- 支撑/阻力计算逻辑与技术含义

3) 入场与出场
- 首选策略（触发条件、入场区间、SL、TP）
- 备选策略（触发条件、入场区间、SL、TP）

4) 风险管理（硬约束）
- 仓位/杠杆由实盘执行参数与风控引擎统一决定（策略中不固定金额）
- 可给出风险预算公式，但不得写死固定金额阈值
- 最小盈亏比（盈利/亏损）要求：目标>=2.0（最低1.5）

5) 观望与失效条件
- 明确“什么情况下不交易”
- 明确“策略何时失效需停用”

6) 回测建议
- 推荐回测区间、周期、指标、评估口径（总盈亏、胜率、盈亏比、回撤）`,
	}
}

func (s *Service) handlePromptSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetPromptSettings(w)
	case http.MethodPost:
		s.handleSavePromptSettings(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleGetPromptSettings(w http.ResponseWriter) {
	cfg, _ := readPromptSettings()
	cfg = mergePromptDefaults(cfg)
	writeJSON(w, http.StatusOK, map[string]any{"prompts": cfg})
}

func (s *Service) handleSavePromptSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompts      promptSettings `json:"prompts"`
		ResetDefault bool           `json:"reset_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	cfg := mergePromptDefaults(req.Prompts)
	if req.ResetDefault {
		cfg = defaultPromptSettings()
	}
	if err := writePromptSettings(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "保存提示词失败: "+err.Error())
		return
	}
	applyPromptSettingsToEnv(cfg)
	writeJSON(w, http.StatusOK, map[string]any{"message": "prompt settings updated", "prompts": cfg})
}

func loadPromptSettingsToEnv() {
	cfg, err := readPromptSettings()
	if err != nil {
		cfg = defaultPromptSettings()
	}
	applyPromptSettingsToEnv(mergePromptDefaults(cfg))
}

func applyPromptSettingsToEnv(cfg promptSettings) {
	_ = os.Setenv("TRADING_AI_SYSTEM_PROMPT", strings.TrimSpace(cfg.TradingAISystemPrompt))
	_ = os.Setenv("TRADING_AI_POLICY_PROMPT", strings.TrimSpace(cfg.TradingAIPolicyPrompt))
	_ = os.Setenv("STRATEGY_GENERATOR_PROMPT_TEMPLATE", strings.TrimSpace(cfg.StrategyGeneratorPromptTemplate))
}

func mergePromptDefaults(in promptSettings) promptSettings {
	d := defaultPromptSettings()
	out := in
	legacySystem := strings.Contains(out.TradingAISystemPrompt, "账户100U") || strings.Contains(out.TradingAISystemPrompt, "最大亏损<=1.5U")
	legacyPolicy := strings.Contains(out.TradingAIPolicyPrompt, "|Entry-SL|*Size") || strings.Contains(out.TradingAIPolicyPrompt, "账户100U")
	if strings.TrimSpace(out.TradingAISystemPrompt) == "" {
		out.TradingAISystemPrompt = d.TradingAISystemPrompt
	} else if legacySystem {
		out.TradingAISystemPrompt = d.TradingAISystemPrompt
	}
	if strings.TrimSpace(out.TradingAIPolicyPrompt) == "" {
		out.TradingAIPolicyPrompt = d.TradingAIPolicyPrompt
	} else if legacyPolicy {
		out.TradingAIPolicyPrompt = d.TradingAIPolicyPrompt
	}
	if strings.TrimSpace(out.StrategyGeneratorPromptTemplate) == "" {
		out.StrategyGeneratorPromptTemplate = d.StrategyGeneratorPromptTemplate
	}
	return out
}

func readPromptSettings() (promptSettings, error) {
	var cfg promptSettings
	raw, err := os.ReadFile(promptSettingsPath)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func writePromptSettings(cfg promptSettings) error {
	if err := os.MkdirAll(filepath.Dir(promptSettingsPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(promptSettingsPath, raw, 0o644)
}
