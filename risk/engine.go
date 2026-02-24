package risk

import (
	"fmt"
	"math"
	"trade-go/config"
)

type Snapshot struct {
	Balance           float64
	TodayPnL          float64
	PeakEquity        float64
	CurrentEquity     float64
	ConsecutiveLosses int
}

type OrderPlanInput struct {
	Price         float64
	StopLoss      float64
	Confidence    string
	SuggestedSize float64
	Leverage      int
}

type OrderPlan struct {
	Approved bool
	Size     float64
	Reason   string
}

type Engine struct {
	cfg *config.TradeConfig
}

func NewEngine(cfg *config.TradeConfig) *Engine {
	return &Engine{cfg: cfg}
}

func (e *Engine) EvaluateGlobalStop(s Snapshot) (bool, string) {
	if s.Balance <= 0 {
		return false, "balance unavailable"
	}
	if e.cfg.MaxConsecutiveLosses > 0 && s.ConsecutiveLosses >= e.cfg.MaxConsecutiveLosses {
		return true, fmt.Sprintf("连续亏损达到上限(%d)", e.cfg.MaxConsecutiveLosses)
	}
	if e.cfg.MaxDailyLossPct > 0 && -s.TodayPnL >= s.Balance*e.cfg.MaxDailyLossPct {
		return true, fmt.Sprintf("当日亏损达到上限(%.2f%%)", e.cfg.MaxDailyLossPct*100)
	}
	if e.cfg.MaxDrawdownPct > 0 && s.PeakEquity > 0 {
		drawdown := (s.PeakEquity - s.CurrentEquity) / s.PeakEquity
		if drawdown >= e.cfg.MaxDrawdownPct {
			return true, fmt.Sprintf("最大回撤达到上限(%.2f%%)", e.cfg.MaxDrawdownPct*100)
		}
	}
	return false, ""
}

func (e *Engine) BuildOrderPlan(in OrderPlanInput, s Snapshot) OrderPlan {
	stop, reason := e.EvaluateGlobalStop(s)
	if stop {
		return OrderPlan{Approved: false, Reason: reason}
	}
	if in.Price <= 0 || s.Balance <= 0 {
		return OrderPlan{Approved: false, Reason: "价格或余额无效"}
	}
	if in.SuggestedSize <= 0 {
		return OrderPlan{Approved: false, Reason: "建议仓位为0"}
	}
	lev := in.Leverage
	if lev <= 0 {
		lev = 1
	}

	size := in.SuggestedSize

	if e.cfg.MaxPositionPct > 0 {
		maxMargin := s.Balance * e.cfg.MaxPositionPct
		maxSizeByPos := maxMargin * float64(lev) / in.Price
		size = math.Min(size, maxSizeByPos)
	}

	if e.cfg.MaxRiskPerTradePct > 0 && in.StopLoss > 0 {
		stopDist := math.Abs(in.Price - in.StopLoss)
		if stopDist > 0 {
			maxLoss := s.Balance * e.cfg.MaxRiskPerTradePct
			maxSizeByRisk := maxLoss / stopDist
			size = math.Min(size, maxSizeByRisk)
		}
	}

	// 强平保护（简化版）：仓位杠杆越高，要求止损与入场价至少有缓冲距离。
	if e.cfg.LiquidationBufferPct > 0 && in.StopLoss > 0 {
		minDistPct := e.cfg.LiquidationBufferPct / float64(lev)
		actualDistPct := math.Abs(in.Price-in.StopLoss) / in.Price
		if actualDistPct < minDistPct {
			return OrderPlan{Approved: false, Reason: "止损距离过近，触发强平保护"}
		}
	}

	if size <= 0 {
		return OrderPlan{Approved: false, Reason: "风控后仓位为0"}
	}

	return OrderPlan{Approved: true, Size: size}
}
