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

	s.runMu.Lock()
	record, err := s.bot.RunPaperSimulation(trader.PaperSimulationInput{
		Symbol:                  req.Symbol,
		Balance:                 req.Balance,
		PositionSizingMode:      req.PositionSizingMode,
		HighConfidenceAmount:    req.HighConfidenceAmount,
		LowConfidenceAmount:     req.LowConfidenceAmount,
		HighConfidenceMarginPct: req.HighConfidenceMarginPct,
		LowConfidenceMarginPct:  req.LowConfidenceMarginPct,
		Leverage:                req.Leverage,
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
