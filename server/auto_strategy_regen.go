package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"trade-go/exchange"
	"trade-go/indicators"
	"trade-go/storage"
)

const (
	executionStrategiesEnvKey = "AI_EXECUTION_STRATEGIES"
)

func isDeprecatedBuiltinStrategy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ai_assisted", "trend_following", "mean_reversion", "breakout":
		return true
	default:
		return false
	}
}

func enabledStrategiesEnvRaw() string {
	return strings.TrimSpace(os.Getenv(executionStrategiesEnvKey))
}

func normalizeAutoRegenCooldownSec(v int) int {
	if v < 300 {
		return 300
	}
	if v > 604800 {
		return 604800
	}
	return v
}

func normalizeAutoRegenMinRR(v float64) float64 {
	if v < 1 {
		return 1
	}
	if v > 10 {
		return 10
	}
	return v
}

func habitByTimeframe(tf string) string {
	v := strings.TrimSpace(strings.ToLower(tf))
	switch v {
	case "1m", "3m", "5m", "10m", "15m":
		return "10m"
	case "30m", "1h":
		return "1h"
	case "2h", "4h":
		return "4h"
	case "1d", "1day":
		return "1D"
	default:
		return "1h"
	}
}

func parseEnabledStrategiesEnv(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		raw = enabledStrategiesEnvRaw()
	}
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if isDeprecatedBuiltinStrategy(v) {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

func (s *Service) autoRegenTriggered(rs storage.RiskSnapshot) (bool, string, float64) {
	cfg := s.bot.TradeConfig()
	maxLossStreak := cfg.AutoStrategyRegenLossStreak
	if maxLossStreak < 1 {
		maxLossStreak = 3
	}
	maxDrawdownWarn := cfg.AutoStrategyRegenDrawdownWarnPct
	if maxDrawdownWarn <= 0 || maxDrawdownWarn > 1 {
		maxDrawdownWarn = 0.08
	}
	drawdown := 0.0
	if rs.PeakEquity > 0 && rs.CurrentEquity > 0 && rs.CurrentEquity < rs.PeakEquity {
		drawdown = (rs.PeakEquity - rs.CurrentEquity) / rs.PeakEquity
	}
	reasons := make([]string, 0, 2)
	if rs.ConsecutiveLosses >= maxLossStreak {
		reasons = append(reasons, fmt.Sprintf("连续亏损达到%d", rs.ConsecutiveLosses))
	}
	if drawdown >= maxDrawdownWarn {
		reasons = append(reasons, fmt.Sprintf("回撤达到%.2f%%", drawdown*100))
	}
	if len(reasons) == 0 {
		return false, "", drawdown
	}
	return true, strings.Join(reasons, " / "), drawdown
}

func (s *Service) generateAutoStrategy(reason string, drawdown float64, rs storage.RiskSnapshot) (generatedStrategyRecord, []string, error) {
	cfg := s.bot.TradeConfig()
	symbol := strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	habit := habitByTimeframe(cfg.Timeframe)
	profile := habitProfileOf(habit)
	tf := profile.Timeframe
	minRR := normalizeAutoRegenMinRR(cfg.AutoStrategyRegenMinRR)

	client := exchange.NewClient()
	candles, err := client.FetchOHLCV(symbol, tf, 120)
	if err != nil || len(candles) < 30 {
		if err == nil {
			err = fmt.Errorf("K线数据不足")
		}
		return generatedStrategyRecord{}, nil, err
	}
	ind := indicators.Calculate(candles)
	trend := indicators.AnalyzeTrend(candles, ind)
	levels := indicators.AnalyzeLevels(candles, ind)
	cur := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	change := (cur.Close - prev.Close) / prev.Close * 100

	style := "hybrid"
	directionBias := "balanced"
	if strings.Contains(trend.Overall, "上涨") {
		style = "trend_follow"
		directionBias = "long_only"
	} else if strings.Contains(trend.Overall, "下跌") {
		style = "trend_follow"
		directionBias = "short_only"
	}

	gen, _, _ := generatePreferenceByLLM(
		symbol,
		habit,
		tf,
		cur.Close,
		change,
		trend.Overall,
		ind,
		levels,
		style,
		minRR,
		false,
		"hold",
		directionBias,
		cfg,
	)
	gen.StrategyName = buildStandardStrategyName(symbol, habit, style, true)

	record := generatedStrategyRecord{
		ID:               fmt.Sprintf("auto_%d", time.Now().UnixNano()),
		Name:             gen.StrategyName,
		RuleKey:          buildStrategyRuleKey(symbol, habit, style, true),
		PreferencePrompt: gen.PreferencePrompt,
		GeneratorPrompt:  gen.GeneratorPrompt,
		Logic:            gen.Logic,
		Basis:            strings.TrimSpace(gen.Basis + " | 自动重生成原因: " + reason),
		CreatedAt:        time.Now().Format(time.RFC3339),
		LastUpdatedAt:    time.Now().Format(time.RFC3339),
		Source:           "auto_regen",
		WorkflowVersion:  loadSkillWorkflowConfig().Version,
		WorkflowChain:    enabledSkillWorkflowSteps(loadSkillWorkflowConfig()),
	}
	final, nextEnabled, _, err := s.saveAndActivateGeneratedStrategy(record)
	if err != nil {
		return generatedStrategyRecord{}, nil, err
	}

	_ = s.bot.EmitRiskEvent("auto_strategy_regen", mustJSON(map[string]any{
		"symbol":             symbol,
		"timeframe":          tf,
		"reason":             reason,
		"drawdown_pct":       drawdown,
		"consecutive_losses": rs.ConsecutiveLosses,
		"strategy_name":      final.Name,
		"enabled":            nextEnabled,
	}))
	return final, nextEnabled, nil
}

func (s *Service) maybeAutoRegenerateStrategy() {
	cfg := s.bot.TradeConfig()
	now := time.Now()
	cooldown := normalizeAutoRegenCooldownSec(cfg.AutoStrategyRegenCooldownSec)

	s.mu.Lock()
	if !cfg.AutoStrategyRegenEnabled {
		s.nextAutoStrategyRegenAt = time.Time{}
		s.mu.Unlock()
		return
	}
	if s.lastAutoStrategyRegenAt.IsZero() {
		s.nextAutoStrategyRegenAt = now.Add(time.Duration(cooldown) * time.Second)
	} else {
		s.nextAutoStrategyRegenAt = s.lastAutoStrategyRegenAt.Add(time.Duration(cooldown) * time.Second)
	}
	nextDue := s.nextAutoStrategyRegenAt
	if !s.lastAutoStrategyRegenAt.IsZero() && now.Before(nextDue) {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	rs, ok := s.bot.RiskSnapshot()
	if !ok {
		return
	}
	triggered, reason, drawdown := s.autoRegenTriggered(rs)
	if !triggered {
		s.mu.Lock()
		if s.lastAutoStrategyRegenAt.IsZero() {
			s.nextAutoStrategyRegenAt = now.Add(time.Duration(cooldown) * time.Second)
		} else {
			s.nextAutoStrategyRegenAt = s.lastAutoStrategyRegenAt.Add(time.Duration(cooldown) * time.Second)
		}
		s.mu.Unlock()
		return
	}

	final, _, err := s.generateAutoStrategy(reason, drawdown, rs)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.lastAutoStrategyRegenReason = "自动重生成失败: " + err.Error()
		s.nextAutoStrategyRegenAt = now.Add(10 * time.Minute)
		return
	}
	s.lastAutoStrategyRegenAt = now
	s.nextAutoStrategyRegenAt = now.Add(time.Duration(cooldown) * time.Second)
	s.lastAutoStrategyRegenReason = "已自动生成并启用策略: " + final.Name + "；触发原因: " + reason
}

func (s *Service) runAutoStrategyRegenNow(force bool) (map[string]any, error) {
	now := time.Now()
	cfg := s.bot.TradeConfig()
	cooldown := normalizeAutoRegenCooldownSec(cfg.AutoStrategyRegenCooldownSec)
	rs, ok := s.bot.RiskSnapshot()
	if !ok {
		return nil, fmt.Errorf("风险快照不可用，请先执行至少一次交易循环")
	}
	triggered, reason, drawdown := s.autoRegenTriggered(rs)
	if !triggered && !force {
		s.mu.Lock()
		if s.lastAutoStrategyRegenAt.IsZero() {
			s.nextAutoStrategyRegenAt = now.Add(time.Duration(cooldown) * time.Second)
		} else {
			s.nextAutoStrategyRegenAt = s.lastAutoStrategyRegenAt.Add(time.Duration(cooldown) * time.Second)
		}
		s.lastAutoStrategyRegenReason = "手动触发策略升级检查：当前未达到升级条件"
		s.mu.Unlock()
		return map[string]any{
			"upgraded":           false,
			"triggered":          false,
			"force":              force,
			"message":            "当前未触发升级条件（未达到连续亏损/回撤阈值）",
			"reason":             "insufficient_trigger",
			"drawdown_pct":       drawdown,
			"consecutive_losses": rs.ConsecutiveLosses,
			"checked_at":         now.Format(time.RFC3339),
		}, nil
	}
	regenReason := reason
	if !triggered && force {
		regenReason = "手动强制升级（未触发自动条件）"
	}
	if strings.TrimSpace(regenReason) == "" {
		regenReason = "手动触发策略升级"
	}

	final, nextEnabled, err := s.generateAutoStrategy(regenReason, drawdown, rs)
	if err != nil {
		s.mu.Lock()
		s.lastAutoStrategyRegenReason = "手动升级失败: " + err.Error()
		s.nextAutoStrategyRegenAt = now.Add(10 * time.Minute)
		s.mu.Unlock()
		return nil, err
	}

	s.mu.Lock()
	s.lastAutoStrategyRegenAt = now
	s.nextAutoStrategyRegenAt = now.Add(time.Duration(cooldown) * time.Second)
	s.lastAutoStrategyRegenReason = "手动触发已升级并启用策略: " + final.Name + "；触发原因: " + regenReason
	s.mu.Unlock()

	return map[string]any{
		"upgraded":           true,
		"triggered":          triggered,
		"force":              force,
		"message":            "策略升级完成并已启用",
		"strategy_name":      final.Name,
		"reason":             regenReason,
		"drawdown_pct":       drawdown,
		"consecutive_losses": rs.ConsecutiveLosses,
		"checked_at":         now.Format(time.RFC3339),
		"active_strategy":    final.Name,
		"enabled_strategies": nextEnabled,
	}, nil
}

func (s *Service) handleAutoStrategyRegenNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()
	result, err := s.runAutoStrategyRegenNow(req.Force)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "立刻升级失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
