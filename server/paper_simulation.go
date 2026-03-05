package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"trade-go/trader"
)

type paperSimulateStepRequest struct {
	Symbol                  string   `json:"symbol"`
	Balance                 float64  `json:"balance"`
	PositionSizingMode      string   `json:"position_sizing_mode"`
	HighConfidenceAmount    float64  `json:"high_confidence_amount"`
	LowConfidenceAmount     float64  `json:"low_confidence_amount"`
	HighConfidenceMarginPct float64  `json:"high_confidence_margin_pct"`
	LowConfidenceMarginPct  float64  `json:"low_confidence_margin_pct"`
	Leverage                int      `json:"leverage"`
	MaxRiskPerTradePct      float64  `json:"max_risk_per_trade_pct"`
	MaxPositionPct          float64  `json:"max_position_pct"`
	MaxConsecutiveLosses    int      `json:"max_consecutive_losses"`
	MaxDailyLossPct         float64  `json:"max_daily_loss_pct"`
	MaxDrawdownPct          float64  `json:"max_drawdown_pct"`
	LiquidationBufferPct    float64  `json:"liquidation_buffer_pct"`
	EnabledStrategies       []string `json:"enabled_strategies"`
}

func (s *Service) handlePaperSimulateStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req paperSimulateStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	if req.Symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}

	s.mu.RLock()
	paperCfg := s.paperState.Config
	baseBalance := req.Balance
	if baseBalance <= 0 {
		baseBalance = paperCfg.Balance
	}
	riskSnap := buildPaperRiskSnapshot(s.paperState.Records, req.Symbol, baseBalance)
	s.mu.RUnlock()

	maxRiskPerTrade := req.MaxRiskPerTradePct
	if maxRiskPerTrade <= 0 {
		maxRiskPerTrade = paperCfg.MaxRiskPerTradePct
	}
	maxPosition := req.MaxPositionPct
	if maxPosition <= 0 {
		maxPosition = paperCfg.MaxPositionPct
	}
	maxConsecutiveLosses := req.MaxConsecutiveLosses
	if maxConsecutiveLosses < 0 {
		maxConsecutiveLosses = paperCfg.MaxConsecutiveLosses
	}
	maxDailyLoss := req.MaxDailyLossPct
	if maxDailyLoss <= 0 {
		maxDailyLoss = paperCfg.MaxDailyLossPct
	}
	maxDrawdown := req.MaxDrawdownPct
	if maxDrawdown <= 0 {
		maxDrawdown = paperCfg.MaxDrawdownPct
	}
	liqBuffer := req.LiquidationBufferPct
	if liqBuffer <= 0 {
		liqBuffer = paperCfg.LiquidationBufferPct
	}

	s.runMu.Lock()
	record, err := s.bot.RunPaperSimulation(trader.PaperSimulationInput{
		Symbol:                  req.Symbol,
		Balance:                 baseBalance,
		PositionSizingMode:      req.PositionSizingMode,
		HighConfidenceAmount:    req.HighConfidenceAmount,
		LowConfidenceAmount:     req.LowConfidenceAmount,
		HighConfidenceMarginPct: req.HighConfidenceMarginPct,
		LowConfidenceMarginPct:  req.LowConfidenceMarginPct,
		Leverage:                req.Leverage,
		MaxRiskPerTradePct:      maxRiskPerTrade,
		MaxPositionPct:          maxPosition,
		MaxConsecutiveLosses:    maxConsecutiveLosses,
		MaxDailyLossPct:         maxDailyLoss,
		MaxDrawdownPct:          maxDrawdown,
		LiquidationBufferPct:    liqBuffer,
		RiskTodayPnL:            riskSnap.TodayPnL,
		RiskPeakEquity:          riskSnap.PeakEquity,
		RiskCurrentEquity:       riskSnap.CurrentEquity,
		RiskConsecutiveLosses:   riskSnap.ConsecutiveLosses,
		EnabledStrategies:       req.EnabledStrategies,
	})
	s.runMu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadGateway, "paper simulation failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"record": record,
	})
}
