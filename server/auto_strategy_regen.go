package server

import (
	"fmt"
	"os"
	"strings"
	"time"
	"trade-go/exchange"
	"trade-go/indicators"
	"trade-go/storage"
)

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
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
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

func (s *Service) generateAutoStrategy(reason string, drawdown float64, rs storage.RiskSnapshot) (generatedStrategyRecord, error) {
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
		return generatedStrategyRecord{}, err
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

	gen, _ := generatePreferenceByLLM(
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
	if strings.TrimSpace(gen.StrategyName) == "" {
		gen.StrategyName = fmt.Sprintf("AUTO_%s_%s_%s", symbol, habit, time.Now().Format("150405"))
	}
	gen.StrategyName = strings.TrimSpace(gen.StrategyName) + "_auto"

	record := generatedStrategyRecord{
		ID:               fmt.Sprintf("auto_%d", time.Now().UnixNano()),
		Name:             gen.StrategyName,
		PreferencePrompt: gen.PreferencePrompt,
		GeneratorPrompt:  gen.GeneratorPrompt,
		Logic:            gen.Logic,
		Basis:            strings.TrimSpace(gen.Basis + " | 自动重生成原因: " + reason),
		CreatedAt:        time.Now().Format(time.RFC3339),
	}
	st := readGeneratedStrategies()
	st.Strategies = append([]generatedStrategyRecord{record}, st.Strategies...)
	if len(st.Strategies) > 300 {
		st.Strategies = st.Strategies[:300]
	}
	if err := writeGeneratedStrategies(st); err != nil {
		return generatedStrategyRecord{}, err
	}
	finalStore := readGeneratedStrategies()
	final := record
	for _, item := range finalStore.Strategies {
		if strings.TrimSpace(item.ID) == strings.TrimSpace(record.ID) {
			final = item
			break
		}
	}

	currentEnabled := parseEnabledStrategiesEnv(os.Getenv("PY_STRATEGY_ENABLED"))
	nextEnabled := []string{final.Name}
	for _, item := range currentEnabled {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(final.Name)) {
			continue
		}
		nextEnabled = append(nextEnabled, item)
		if len(nextEnabled) >= 3 {
			break
		}
	}
	updates := map[string]string{
		"PY_STRATEGY_ENABLED": strings.Join(nextEnabled, ","),
	}
	if err := upsertDotEnv(".env", updates); err == nil {
		for k, v := range updates {
			_ = os.Setenv(k, v)
		}
		applyRuntimeConfigFromEnv()
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
	return final, nil
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

	final, err := s.generateAutoStrategy(reason, drawdown, rs)
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
