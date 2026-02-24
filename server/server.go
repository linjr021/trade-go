package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
	"trade-go/trader"
)

type Service struct {
	bot *trader.Bot

	runMu sync.Mutex
	mu    sync.RWMutex

	schedulerRunning bool
	nextRunAt        time.Time
	cancelScheduler  context.CancelFunc
}

func NewService(bot *trader.Bot) *Service {
	return &Service{bot: bot}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/account", s.handleAccount)
	mux.HandleFunc("/api/signals", s.handleSignals)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/run", s.handleRunNow)
	mux.HandleFunc("/api/scheduler/start", s.handleStartScheduler)
	mux.HandleFunc("/api/scheduler/stop", s.handleStopScheduler)
}

func (s *Service) StartScheduler() {
	s.mu.Lock()
	if s.schedulerRunning {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelScheduler = cancel
	s.schedulerRunning = true
	s.mu.Unlock()

	go s.loop(ctx)
}

func (s *Service) StopScheduler() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelScheduler != nil {
		s.cancelScheduler()
	}
	s.schedulerRunning = false
	s.nextRunAt = time.Time{}
}

func (s *Service) loop(ctx context.Context) {
	for {
		waitSec := trader.WaitForNextPeriod()
		next := time.Now().Add(time.Duration(waitSec) * time.Second)

		s.mu.Lock()
		s.nextRunAt = next
		s.mu.Unlock()

		timer := time.NewTimer(time.Duration(waitSec) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.runCycle()
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(60 * time.Second):
		}
	}
}

func (s *Service) runCycle() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.bot.Run()
}

func (s *Service) RunOnce() {
	s.runCycle()
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.bot.Snapshot()
	cfg := s.bot.TradeConfig()

	s.mu.RLock()
	resp := map[string]any{
		"trade_config": map[string]any{
			"symbol":                 cfg.Symbol,
			"amount":                 cfg.Amount,
			"high_confidence_amount": cfg.HighConfidenceAmount,
			"low_confidence_amount":  cfg.LowConfidenceAmount,
			"leverage":               cfg.Leverage,
			"timeframe":              cfg.Timeframe,
			"test_mode":              cfg.TestMode,
			"data_points":            cfg.DataPoints,
			"max_risk_per_trade_pct": cfg.MaxRiskPerTradePct,
			"max_position_pct":       cfg.MaxPositionPct,
			"max_consecutive_losses": cfg.MaxConsecutiveLosses,
			"max_daily_loss_pct":     cfg.MaxDailyLossPct,
			"max_drawdown_pct":       cfg.MaxDrawdownPct,
			"liquidation_buffer_pct": cfg.LiquidationBufferPct,
		},
		"scheduler_running": s.schedulerRunning,
		"next_run_at":       s.nextRunAt,
		"runtime":           snap,
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) handleAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	balance, balanceErr := s.bot.FetchBalance()
	position, posErr := s.bot.FetchPosition()

	resp := map[string]any{
		"balance":  balance,
		"position": position,
	}
	if balanceErr != nil {
		resp["balance_error"] = balanceErr.Error()
	}
	if posErr != nil {
		resp["position_error"] = posErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) handleSignals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	signals := s.bot.SignalHistory(limit)
	writeJSON(w, http.StatusOK, map[string]any{"signals": signals})
}

func (s *Service) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		HighConfidenceAmount *float64 `json:"high_confidence_amount"`
		LowConfidenceAmount  *float64 `json:"low_confidence_amount"`
		Leverage             *int     `json:"leverage"`
		MaxRiskPerTradePct   *float64 `json:"max_risk_per_trade_pct"`
		MaxPositionPct       *float64 `json:"max_position_pct"`
		MaxConsecutiveLosses *int     `json:"max_consecutive_losses"`
		MaxDailyLossPct      *float64 `json:"max_daily_loss_pct"`
		MaxDrawdownPct       *float64 `json:"max_drawdown_pct"`
		LiquidationBufferPct *float64 `json:"liquidation_buffer_pct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	cfg, err := s.bot.UpdateTradeSettings(trader.TradeSettingsUpdate{
		HighConfidenceAmount: req.HighConfidenceAmount,
		LowConfidenceAmount:  req.LowConfidenceAmount,
		Leverage:             req.Leverage,
		MaxRiskPerTradePct:   req.MaxRiskPerTradePct,
		MaxPositionPct:       req.MaxPositionPct,
		MaxConsecutiveLosses: req.MaxConsecutiveLosses,
		MaxDailyLossPct:      req.MaxDailyLossPct,
		MaxDrawdownPct:       req.MaxDrawdownPct,
		LiquidationBufferPct: req.LiquidationBufferPct,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "settings updated",
		"trade_config": map[string]any{
			"symbol":                 cfg.Symbol,
			"amount":                 cfg.Amount,
			"high_confidence_amount": cfg.HighConfidenceAmount,
			"low_confidence_amount":  cfg.LowConfidenceAmount,
			"leverage":               cfg.Leverage,
			"timeframe":              cfg.Timeframe,
			"test_mode":              cfg.TestMode,
			"data_points":            cfg.DataPoints,
			"max_risk_per_trade_pct": cfg.MaxRiskPerTradePct,
			"max_position_pct":       cfg.MaxPositionPct,
			"max_consecutive_losses": cfg.MaxConsecutiveLosses,
			"max_daily_loss_pct":     cfg.MaxDailyLossPct,
			"max_drawdown_pct":       cfg.MaxDrawdownPct,
			"liquidation_buffer_pct": cfg.LiquidationBufferPct,
		},
	})
}

func (s *Service) handleRunNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.runCycle()
	writeJSON(w, http.StatusOK, map[string]any{"message": "run completed"})
}

func (s *Service) handleStartScheduler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.StartScheduler()
	writeJSON(w, http.StatusOK, map[string]string{"message": "scheduler started"})
}

func (s *Service) handleStopScheduler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.StopScheduler()
	writeJSON(w, http.StatusOK, map[string]string{"message": "scheduler stopped"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func Serve(addr string, service *Service) error {
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "trade-go api",
			"hint":    "frontend should request /api/* endpoints",
		})
	})

	handler := withCORS(mux)
	fmt.Printf("HTTP 服务已启动: %s\n", addr)
	return http.ListenAndServe(addr, handler)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
