package trader

import (
	"fmt"
	"math"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/models"
)

func autoReviewIntervalSec(cfg config.TradeConfig) int {
	sec := cfg.AutoReviewIntervalSec
	if sec < 60 {
		sec = 60
	}
	if sec > 86400 {
		sec = 86400
	}
	return sec
}

func clampAutoReduceFactor(v float64) float64 {
	if v <= 0 {
		return 0.7
	}
	if v > 1 {
		return 1
	}
	if v < 0.1 {
		return 0.1
	}
	return v
}

func (b *Bot) syncRuntimeAutoLocked() {
	b.runtime.LastOrderExecutedAt = b.lastOrderExecutedAt
	b.runtime.LastAutoReviewAt = b.lastAutoReviewAt
	b.runtime.NextAutoReviewAt = b.nextAutoReviewAt
	b.runtime.AutoRiskProfile = b.autoRiskProfile
	b.runtime.AutoReviewReason = b.autoReviewReason
}

func (b *Bot) markOrderExecuted(executedAt time.Time) {
	if executedAt.IsZero() {
		executedAt = time.Now()
	}
	b.mu.Lock()
	b.lastOrderExecutedAt = executedAt
	cfg := *b.cfg
	if cfg.AutoReviewEnabled && cfg.AutoReviewAfterOrderOnly {
		b.nextAutoReviewAt = executedAt.Add(time.Duration(autoReviewIntervalSec(cfg)) * time.Second)
	}
	b.syncRuntimeAutoLocked()
	b.mu.Unlock()
}

func (b *Bot) applyAutoRiskProfile(profile string, reason string, reviewAt time.Time, reduceFactor float64) (bool, config.TradeConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()

	base := b.baseCfg
	next := base
	factor := clampAutoReduceFactor(reduceFactor)
	if profile == "cautious" {
		next.Leverage = int(math.Round(float64(base.Leverage) * factor))
		if next.Leverage < 1 {
			next.Leverage = 1
		}
		next.HighConfidenceAmount = base.HighConfidenceAmount * factor
		next.LowConfidenceAmount = base.LowConfidenceAmount * factor
		next.HighConfidenceMarginPct = base.HighConfidenceMarginPct * factor
		next.LowConfidenceMarginPct = base.LowConfidenceMarginPct * factor
		next.MaxRiskPerTradePct = base.MaxRiskPerTradePct * factor
		next.MaxPositionPct = base.MaxPositionPct * factor
	}

	prev := *b.cfg
	prevProfile := b.autoRiskProfile
	*b.cfg = next
	b.autoRiskProfile = profile
	b.autoReviewReason = reason
	b.lastAutoReviewAt = reviewAt
	if next.AutoReviewEnabled && !next.AutoReviewAfterOrderOnly {
		b.nextAutoReviewAt = reviewAt.Add(time.Duration(autoReviewIntervalSec(next)) * time.Second)
	} else {
		b.nextAutoReviewAt = time.Time{}
	}
	b.syncRuntimeAutoLocked()

	changed := profile != prevProfile ||
		prev.Leverage != next.Leverage ||
		math.Abs(prev.HighConfidenceAmount-next.HighConfidenceAmount) > 1e-12 ||
		math.Abs(prev.LowConfidenceAmount-next.LowConfidenceAmount) > 1e-12 ||
		math.Abs(prev.HighConfidenceMarginPct-next.HighConfidenceMarginPct) > 1e-12 ||
		math.Abs(prev.LowConfidenceMarginPct-next.LowConfidenceMarginPct) > 1e-12 ||
		math.Abs(prev.MaxRiskPerTradePct-next.MaxRiskPerTradePct) > 1e-12 ||
		math.Abs(prev.MaxPositionPct-next.MaxPositionPct) > 1e-12
	return changed, next
}

func (b *Bot) maybeAutoReview(cycleID string, pd models.PriceData) {
	cfg := b.TradeConfig()
	if !cfg.AutoReviewEnabled {
		return
	}
	now := time.Now()
	interval := autoReviewIntervalSec(cfg)

	b.mu.RLock()
	lastOrderAt := b.lastOrderExecutedAt
	lastReviewAt := b.lastAutoReviewAt
	b.mu.RUnlock()

	if cfg.AutoReviewAfterOrderOnly {
		if lastOrderAt.IsZero() {
			b.mu.Lock()
			b.nextAutoReviewAt = time.Time{}
			b.syncRuntimeAutoLocked()
			b.mu.Unlock()
			return
		}
		nextDue := lastOrderAt.Add(time.Duration(interval) * time.Second)
		if now.Before(nextDue) {
			b.mu.Lock()
			b.nextAutoReviewAt = nextDue
			b.syncRuntimeAutoLocked()
			b.mu.Unlock()
			return
		}
		if !lastReviewAt.IsZero() && !lastReviewAt.Before(lastOrderAt) {
			b.mu.Lock()
			b.nextAutoReviewAt = time.Time{}
			b.syncRuntimeAutoLocked()
			b.mu.Unlock()
			return
		}
	} else {
		if !lastReviewAt.IsZero() {
			nextDue := lastReviewAt.Add(time.Duration(interval) * time.Second)
			if now.Before(nextDue) {
				b.mu.Lock()
				b.nextAutoReviewAt = nextDue
				b.syncRuntimeAutoLocked()
				b.mu.Unlock()
				return
			}
		}
	}

	volThreshold := cfg.AutoReviewVolatilityPct
	if volThreshold <= 0 {
		volThreshold = 1.2
	}
	drawdownWarn := cfg.AutoReviewDrawdownWarnPct
	if drawdownWarn <= 0 {
		drawdownWarn = 0.05
	}
	lossWarn := cfg.AutoReviewLossStreakWarn
	if lossWarn <= 0 {
		lossWarn = 2
	}

	bbWidthPct := 0.0
	if pd.Technical.BBMiddle > 0 {
		bbWidthPct = math.Abs(pd.Technical.BBUpper-pd.Technical.BBLower) / pd.Technical.BBMiddle * 100
	}

	drawdownPct := 0.0
	consecutiveLosses := 0
	if b.store != nil {
		if snap, err := b.store.LoadRiskSnapshot(); err == nil {
			consecutiveLosses = snap.ConsecutiveLosses
			if snap.PeakEquity > 0 && snap.CurrentEquity > 0 && snap.CurrentEquity < snap.PeakEquity {
				drawdownPct = (snap.PeakEquity - snap.CurrentEquity) / snap.PeakEquity
			}
		}
	}

	reasons := make([]string, 0, 4)
	cautious := false
	if math.Abs(pd.PriceChange) >= volThreshold {
		cautious = true
		reasons = append(reasons, fmt.Sprintf("波动 %.2f%%", math.Abs(pd.PriceChange)))
	}
	if bbWidthPct >= volThreshold*2 {
		cautious = true
		reasons = append(reasons, fmt.Sprintf("布林宽度 %.2f%%", bbWidthPct))
	}
	if drawdownPct >= drawdownWarn {
		cautious = true
		reasons = append(reasons, fmt.Sprintf("回撤 %.2f%%", drawdownPct*100))
	}
	if consecutiveLosses >= lossWarn {
		cautious = true
		reasons = append(reasons, fmt.Sprintf("连续亏损 %d", consecutiveLosses))
	}
	if strings.Contains(pd.Trend.Overall, "下跌") && pd.Technical.RSI < 45 {
		cautious = true
		reasons = append(reasons, "趋势偏弱")
	}

	profile := "normal"
	reason := "市场稳定，维持基线参数"
	if cautious {
		profile = "cautious"
		if len(reasons) > 0 {
			reason = "触发风险收缩: " + strings.Join(reasons, " / ")
		} else {
			reason = "触发风险收缩"
		}
	}

	changed, applied := b.applyAutoRiskProfile(profile, reason, now, cfg.AutoReviewRiskReduceFactor)
	if changed || profile == "cautious" {
		payload := map[string]any{
			"cycle_id":             cycleID,
			"profile":              profile,
			"reason":               reason,
			"price_change_pct":     pd.PriceChange,
			"bb_width_pct":         bbWidthPct,
			"drawdown_pct":         drawdownPct,
			"consecutive_losses":   consecutiveLosses,
			"applied_leverage":     applied.Leverage,
			"applied_high_amount":  applied.HighConfidenceAmount,
			"applied_low_amount":   applied.LowConfidenceAmount,
			"applied_high_margin":  applied.HighConfidenceMarginPct,
			"applied_low_margin":   applied.LowConfidenceMarginPct,
			"applied_max_risk_pct": applied.MaxRiskPerTradePct,
			"applied_max_pos_pct":  applied.MaxPositionPct,
			"auto_review_interval": interval,
			"after_order_only":     applied.AutoReviewAfterOrderOnly,
			"risk_reduce_factor":   applied.AutoReviewRiskReduceFactor,
		}
		_ = b.saveRiskEvent("auto_review", mustJSON(payload))
	}
}
