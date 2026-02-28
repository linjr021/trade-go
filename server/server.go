package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	startedAt        time.Time
	restartCount     int
}

func NewService(bot *trader.Bot) *Service {
	loadPromptSettingsToEnv()
	return &Service{
		bot:       bot,
		startedAt: time.Now(),
	}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/account", s.handleAccount)
	mux.HandleFunc("/api/assets/overview", s.handleAssetOverview)
	mux.HandleFunc("/api/assets/trend", s.handleAssetTrend)
	mux.HandleFunc("/api/assets/pnl-calendar", s.handleAssetPnLCalendar)
	mux.HandleFunc("/api/assets/distribution", s.handleAssetDistribution)
	mux.HandleFunc("/api/signals", s.handleSignals)
	mux.HandleFunc("/api/trade-records", s.handleTradeRecords)
	mux.HandleFunc("/api/strategy-scores", s.handleStrategyScores)
	mux.HandleFunc("/api/strategies", s.handleStrategies)
	mux.HandleFunc("/api/strategies/template", s.handleStrategyTemplate)
	mux.HandleFunc("/api/strategies/upload", s.handleStrategyUpload)
	mux.HandleFunc("/api/strategy-preference/generate", s.handleGenerateStrategyPreference)
	mux.HandleFunc("/api/backtest", s.handleBacktest)
	mux.HandleFunc("/api/backtest-history", s.handleBacktestHistory)
	mux.HandleFunc("/api/backtest-history/detail", s.handleBacktestHistoryDetail)
	mux.HandleFunc("/api/backtest-history/delete", s.handleBacktestHistoryDelete)
	mux.HandleFunc("/api/system-settings", s.handleSystemSettings)
	mux.HandleFunc("/api/integrations", s.handleIntegrations)
	mux.HandleFunc("/api/integrations/llm", s.handleAddLLMIntegration)
	mux.HandleFunc("/api/integrations/llm-product", s.handleAddLLMProduct)
	mux.HandleFunc("/api/integrations/llm-product/update", s.handleUpdateLLMProduct)
	mux.HandleFunc("/api/integrations/llm-product/delete", s.handleDeleteLLMProduct)
	mux.HandleFunc("/api/integrations/llm/test", s.handleTestLLMIntegration)
	mux.HandleFunc("/api/integrations/llm/models", s.handleProbeLLMModels)
	mux.HandleFunc("/api/integrations/llm/update", s.handleUpdateLLMIntegration)
	mux.HandleFunc("/api/integrations/llm/delete", s.handleDeleteLLMIntegration)
	mux.HandleFunc("/api/integrations/exchange", s.handleAddExchangeIntegration)
	mux.HandleFunc("/api/integrations/exchange/activate", s.handleActivateExchangeIntegration)
	mux.HandleFunc("/api/integrations/exchange/delete", s.handleDeleteExchangeIntegration)
	mux.HandleFunc("/api/system/runtime", s.handleSystemRuntimeStatus)
	mux.HandleFunc("/api/system/restart", s.handleSystemSoftRestart)
	mux.HandleFunc("/api/prompt-settings", s.handlePromptSettings)
	mux.HandleFunc("/api/llm/chat", s.handleLLMChat)
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
		"trade_config":      tradeConfigMap(cfg),
		"scheduler_running": s.schedulerRunning,
		"next_run_at":       s.nextRunAt,
		"runtime":           snap,
		"strategy_scores":   s.bot.StrategyComboScores(20),
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

func (s *Service) handleStrategyScores(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scores": s.bot.StrategyComboScores(limit),
	})
}

func (s *Service) handleTradeRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 40
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records": s.bot.TradeRecords(limit),
	})
}

func (s *Service) handleAssetOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	summary, ok := s.bot.EquitySummary()
	if !ok {
		balance, _ := s.bot.FetchBalance()
		summary.TotalFunds = balance
	}
	writeJSON(w, http.StatusOK, map[string]any{"overview": summary})
}

func (s *Service) handleAssetTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "30D"
	}
	days := 30
	switch rng {
	case "7D":
		days = 7
	case "30D":
		days = 30
	case "3M":
		days = 90
	case "6M":
		days = 180
	case "1Y":
		days = 365
	}
	since := time.Now().AddDate(0, 0, -days)
	points, ok := s.bot.EquityTrendSince(since)
	if !ok {
		points = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"range":  rng,
		"points": points,
	})
}

func (s *Service) handleAssetPnLCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	items, ok := s.bot.DailyPnLByMonth(month)
	if !ok {
		items = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"month": month,
		"days":  items,
	})
}

func (s *Service) handleAssetDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.bot.TradeConfig()
	balance, _ := s.bot.FetchBalance()
	pos, _ := s.bot.FetchPosition()
	hold := 0.0
	label := "持仓保证金"
	if pos != nil && pos.EntryPrice > 0 {
		lev := pos.Leverage
		if lev <= 0 {
			lev = float64(cfg.Leverage)
		}
		if lev <= 0 {
			lev = 1
		}
		hold = math.Abs(pos.Size*pos.EntryPrice) / lev
		if pos.Symbol != "" {
			label = pos.Symbol + " 持仓保证金"
		}
	}
	if hold < 0 {
		hold = 0
	}
	cash := balance
	if cash < 0 {
		cash = 0
	}
	total := cash + hold
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total,
		"items": []map[string]any{
			{"label": "可用资金", "value": cash, "color": "#2b6cd0"},
			{"label": label, "value": hold, "color": "#0f996e"},
		},
	})
}

func (s *Service) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Symbol                  *string  `json:"symbol"`
		HighConfidenceAmount    *float64 `json:"high_confidence_amount"`
		LowConfidenceAmount     *float64 `json:"low_confidence_amount"`
		PositionSizingMode      *string  `json:"position_sizing_mode"`
		HighConfidenceMarginPct *float64 `json:"high_confidence_margin_pct"`
		LowConfidenceMarginPct  *float64 `json:"low_confidence_margin_pct"`
		Leverage                *int     `json:"leverage"`
		MaxRiskPerTradePct      *float64 `json:"max_risk_per_trade_pct"`
		MaxPositionPct          *float64 `json:"max_position_pct"`
		MaxConsecutiveLosses    *int     `json:"max_consecutive_losses"`
		MaxDailyLossPct         *float64 `json:"max_daily_loss_pct"`
		MaxDrawdownPct          *float64 `json:"max_drawdown_pct"`
		LiquidationBufferPct    *float64 `json:"liquidation_buffer_pct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	cfg, err := s.bot.UpdateTradeSettings(trader.TradeSettingsUpdate{
		Symbol:                  req.Symbol,
		HighConfidenceAmount:    req.HighConfidenceAmount,
		LowConfidenceAmount:     req.LowConfidenceAmount,
		PositionSizingMode:      req.PositionSizingMode,
		HighConfidenceMarginPct: req.HighConfidenceMarginPct,
		LowConfidenceMarginPct:  req.LowConfidenceMarginPct,
		Leverage:                req.Leverage,
		MaxRiskPerTradePct:      req.MaxRiskPerTradePct,
		MaxPositionPct:          req.MaxPositionPct,
		MaxConsecutiveLosses:    req.MaxConsecutiveLosses,
		MaxDailyLossPct:         req.MaxDailyLossPct,
		MaxDrawdownPct:          req.MaxDrawdownPct,
		LiquidationBufferPct:    req.LiquidationBufferPct,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":      "settings updated",
		"trade_config": tradeConfigMap(cfg),
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
