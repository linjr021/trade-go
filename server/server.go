package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"trade-go/ai"
	"trade-go/config"
	"trade-go/trader"
)

type Service struct {
	bot *trader.Bot

	runMu sync.Mutex
	mu    sync.RWMutex

	schedulerRunning            bool
	realtimeLoopRunning         bool
	triggerMode                 string
	nextRunAt                   time.Time
	cancelScheduler             context.CancelFunc
	startedAt                   time.Time
	restartCount                int
	lastAutoStrategyRegenAt     time.Time
	nextAutoStrategyRegenAt     time.Time
	lastAutoStrategyRegenReason string
}

func NewService(bot *trader.Bot) *Service {
	applySkillWorkflowPromptsToEnv(loadSkillWorkflowConfig())
	ai.SetUsageRecorder(recordLLMUsageWithMeta)
	return &Service{
		bot:                         bot,
		startedAt:                   time.Now(),
		triggerMode:                 "idle",
		lastAutoStrategyRegenReason: "等待自动重生成触发",
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
	mux.HandleFunc("/api/market/snapshot", s.handleMarketSnapshot)
	mux.HandleFunc("/api/trade-records", s.handleTradeRecords)
	mux.HandleFunc("/api/strategy-scores", s.handleStrategyScores)
	mux.HandleFunc("/api/strategies", s.handleStrategies)
	mux.HandleFunc("/api/strategy-preference/generate", s.handleGenerateStrategyPreference)
	mux.HandleFunc("/api/generated-strategies", s.handleGeneratedStrategies)
	mux.HandleFunc("/api/skill-workflow", s.handleSkillWorkflow)
	mux.HandleFunc("/api/llm-usage/logs", s.handleLLMUsageLogs)
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
	mux.HandleFunc("/api/llm/chat", s.handleLLMChat)
	mux.HandleFunc("/api/auto-strategy/regen-now", s.handleAutoStrategyRegenNow)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/run", s.handleRunNow)
	mux.HandleFunc("/api/scheduler/start", s.handleStartScheduler)
	mux.HandleFunc("/api/scheduler/stop", s.handleStopScheduler)
	mux.HandleFunc("/api/paper/simulate-step", s.handlePaperSimulateStep)
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
	s.realtimeLoopRunning = false
	s.triggerMode = "scheduler"
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
	if !s.realtimeLoopRunning {
		s.triggerMode = "idle"
	}
}

func (s *Service) SetRealtimeLoopRunning(running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.realtimeLoopRunning = running
	if running {
		s.triggerMode = "realtime"
	} else if !s.schedulerRunning {
		s.triggerMode = "idle"
	}
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
	s.maybeAutoRegenerateStrategy()
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
	enabledStrategies := parseEnabledStrategiesEnv("")
	activeStrategy := ""
	if len(enabledStrategies) > 0 {
		activeStrategy = enabledStrategies[0]
	}
	enabledStrategyDetails := buildEnabledStrategyDetails(enabledStrategies)
	nextOrderPreview, hasOrderPreview := s.bot.LatestAIDecisionPreview()

	s.mu.RLock()
	resp := map[string]any{
		"trade_config":             tradeConfigMap(cfg),
		"enabled_strategies":       enabledStrategies,
		"enabled_strategy_details": enabledStrategyDetails,
		"active_strategy":          activeStrategy,
		"scheduler_running":        s.schedulerRunning,
		"realtime_running":         s.realtimeLoopRunning,
		"trigger_mode":             s.triggerMode,
		"next_run_at":              s.nextRunAt,
		"runtime":                  snap,
		"strategy_scores":          s.bot.StrategyComboScores(20),
		"auto_strategy_regen": map[string]any{
			"last_at":     s.lastAutoStrategyRegenAt,
			"next_at":     s.nextAutoStrategyRegenAt,
			"last_reason": s.lastAutoStrategyRegenReason,
		},
	}
	if hasOrderPreview {
		resp["next_order_preview"] = nextOrderPreview
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
	availableBalance, availableErr := s.bot.FetchAvailableBalance()
	position, posErr := s.bot.FetchPosition()
	cfg := s.bot.TradeConfig()
	activeExchange := s.bot.ActiveExchange()
	positionSymbol := cfg.Symbol
	if position != nil && strings.TrimSpace(position.Symbol) != "" {
		positionSymbol = position.Symbol
	}

	resp := map[string]any{
		"balance":           balance,
		"available_balance": availableBalance,
		"position":          position,
		"symbol":            cfg.Symbol,
		"position_symbol":   positionSymbol,
		"active_exchange":   activeExchange,
	}
	if balanceErr != nil {
		resp["balance_error"] = balanceErr.Error()
	}
	if posErr != nil {
		resp["position_error"] = posErr.Error()
	}
	if availableErr != nil {
		resp["available_balance_error"] = availableErr.Error()
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
	totalFunds, totalErr := s.bot.FetchBalance()
	availableFunds, availErr := s.bot.FetchAvailableBalance()
	if !ok {
		summary.TotalFunds = totalFunds
	}
	if totalErr == nil && totalFunds > 0 {
		summary.TotalFunds = totalFunds
	}
	if availErr == nil && availableFunds >= 0 {
		summary.AvailableFunds = availableFunds
	}
	if (availErr != nil || math.IsNaN(summary.AvailableFunds) || math.IsInf(summary.AvailableFunds, 0) || summary.AvailableFunds < 0) && summary.TotalFunds > 0 {
		summary.AvailableFunds = summary.TotalFunds
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"overview":        summary,
		"active_exchange": s.bot.ActiveExchange(),
	})
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
		"range":           rng,
		"points":          points,
		"active_exchange": s.bot.ActiveExchange(),
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
		"month":           month,
		"days":            items,
		"active_exchange": s.bot.ActiveExchange(),
	})
}

func (s *Service) handleAssetDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.bot.TradeConfig()
	totalFunds, _ := s.bot.FetchBalance()
	availableFunds, availErr := s.bot.FetchAvailableBalance()
	pos, _ := s.bot.FetchPosition()
	cash := availableFunds
	if availErr != nil || math.IsNaN(cash) || math.IsInf(cash, 0) {
		cash = totalFunds
	}
	if cash < 0 {
		cash = 0
	}
	total := totalFunds
	if total < 0 {
		total = 0
	}
	if total == 0 {
		total = cash
	}
	if cash > total {
		cash = total
	}
	hold := total - cash
	label := "已占用保证金"
	if hold <= 0 && pos != nil && pos.EntryPrice > 0 {
		lev := pos.Leverage
		if lev <= 0 {
			lev = float64(cfg.Leverage)
		}
		if lev <= 0 {
			lev = 1
		}
		hold = math.Abs(pos.Size*pos.EntryPrice) / lev
		if hold < 0 {
			hold = 0
		}
		if pos.Symbol != "" {
			label = pos.Symbol + " 持仓保证金"
		}
		if total < cash+hold {
			total = cash + hold
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":           total,
		"active_exchange": s.bot.ActiveExchange(),
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
		Symbol                           *string  `json:"symbol"`
		HighConfidenceAmount             *float64 `json:"high_confidence_amount"`
		LowConfidenceAmount              *float64 `json:"low_confidence_amount"`
		PositionSizingMode               *string  `json:"position_sizing_mode"`
		HighConfidenceMarginPct          *float64 `json:"high_confidence_margin_pct"`
		LowConfidenceMarginPct           *float64 `json:"low_confidence_margin_pct"`
		Leverage                         *int     `json:"leverage"`
		MaxRiskPerTradePct               *float64 `json:"max_risk_per_trade_pct"`
		MaxPositionPct                   *float64 `json:"max_position_pct"`
		MaxConsecutiveLosses             *int     `json:"max_consecutive_losses"`
		MaxDailyLossPct                  *float64 `json:"max_daily_loss_pct"`
		MaxDrawdownPct                   *float64 `json:"max_drawdown_pct"`
		LiquidationBufferPct             *float64 `json:"liquidation_buffer_pct"`
		AutoReviewEnabled                *bool    `json:"auto_review_enabled"`
		AutoReviewAfterOrderOnly         *bool    `json:"auto_review_after_order_only"`
		AutoReviewIntervalSec            *int     `json:"auto_review_interval_sec"`
		AutoReviewVolatilityPct          *float64 `json:"auto_review_volatility_pct"`
		AutoReviewDrawdownWarnPct        *float64 `json:"auto_review_drawdown_warn_pct"`
		AutoReviewLossStreakWarn         *int     `json:"auto_review_loss_streak_warn"`
		AutoReviewRiskReduceFactor       *float64 `json:"auto_review_risk_reduce_factor"`
		AutoStrategyRegenEnabled         *bool    `json:"auto_strategy_regen_enabled"`
		AutoStrategyRegenCooldownSec     *int     `json:"auto_strategy_regen_cooldown_sec"`
		AutoStrategyRegenLossStreak      *int     `json:"auto_strategy_regen_loss_streak"`
		AutoStrategyRegenDrawdownWarnPct *float64 `json:"auto_strategy_regen_drawdown_warn_pct"`
		AutoStrategyRegenMinRR           *float64 `json:"auto_strategy_regen_min_rr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	cfg, err := s.bot.UpdateTradeSettings(trader.TradeSettingsUpdate{
		Symbol:                           req.Symbol,
		HighConfidenceAmount:             req.HighConfidenceAmount,
		LowConfidenceAmount:              req.LowConfidenceAmount,
		PositionSizingMode:               req.PositionSizingMode,
		HighConfidenceMarginPct:          req.HighConfidenceMarginPct,
		LowConfidenceMarginPct:           req.LowConfidenceMarginPct,
		Leverage:                         req.Leverage,
		MaxRiskPerTradePct:               req.MaxRiskPerTradePct,
		MaxPositionPct:                   req.MaxPositionPct,
		MaxConsecutiveLosses:             req.MaxConsecutiveLosses,
		MaxDailyLossPct:                  req.MaxDailyLossPct,
		MaxDrawdownPct:                   req.MaxDrawdownPct,
		LiquidationBufferPct:             req.LiquidationBufferPct,
		AutoReviewEnabled:                req.AutoReviewEnabled,
		AutoReviewAfterOrderOnly:         req.AutoReviewAfterOrderOnly,
		AutoReviewIntervalSec:            req.AutoReviewIntervalSec,
		AutoReviewVolatilityPct:          req.AutoReviewVolatilityPct,
		AutoReviewDrawdownWarnPct:        req.AutoReviewDrawdownWarnPct,
		AutoReviewLossStreakWarn:         req.AutoReviewLossStreakWarn,
		AutoReviewRiskReduceFactor:       req.AutoReviewRiskReduceFactor,
		AutoStrategyRegenEnabled:         req.AutoStrategyRegenEnabled,
		AutoStrategyRegenCooldownSec:     req.AutoStrategyRegenCooldownSec,
		AutoStrategyRegenLossStreak:      req.AutoStrategyRegenLossStreak,
		AutoStrategyRegenDrawdownWarnPct: req.AutoStrategyRegenDrawdownWarnPct,
		AutoStrategyRegenMinRR:           req.AutoStrategyRegenMinRR,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := persistTradeConfigToEnv(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "参数已生效但持久化失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":      "settings updated",
		"trade_config": tradeConfigMap(cfg),
	})
}

func persistTradeConfigToEnv(cfg config.TradeConfig) error {
	updates := map[string]string{
		"TRADE_SYMBOL":                          strings.ToUpper(strings.TrimSpace(cfg.Symbol)),
		"TRADE_AMOUNT":                          strconv.FormatFloat(cfg.Amount, 'f', -1, 64),
		"HIGH_CONFIDENCE_AMOUNT":                strconv.FormatFloat(cfg.HighConfidenceAmount, 'f', -1, 64),
		"LOW_CONFIDENCE_AMOUNT":                 strconv.FormatFloat(cfg.LowConfidenceAmount, 'f', -1, 64),
		"POSITION_SIZING_MODE":                  strings.ToLower(strings.TrimSpace(cfg.PositionSizingMode)),
		"HIGH_CONFIDENCE_MARGIN_PCT":            strconv.FormatFloat(cfg.HighConfidenceMarginPct, 'f', -1, 64),
		"LOW_CONFIDENCE_MARGIN_PCT":             strconv.FormatFloat(cfg.LowConfidenceMarginPct, 'f', -1, 64),
		"LEVERAGE":                              strconv.Itoa(cfg.Leverage),
		"TIMEFRAME":                             strings.TrimSpace(cfg.Timeframe),
		"DATA_POINTS":                           strconv.Itoa(cfg.DataPoints),
		"MAX_RISK_PER_TRADE_PCT":                strconv.FormatFloat(cfg.MaxRiskPerTradePct, 'f', -1, 64),
		"MAX_POSITION_PCT":                      strconv.FormatFloat(cfg.MaxPositionPct, 'f', -1, 64),
		"MAX_CONSECUTIVE_LOSSES":                strconv.Itoa(cfg.MaxConsecutiveLosses),
		"MAX_DAILY_LOSS_PCT":                    strconv.FormatFloat(cfg.MaxDailyLossPct, 'f', -1, 64),
		"MAX_DRAWDOWN_PCT":                      strconv.FormatFloat(cfg.MaxDrawdownPct, 'f', -1, 64),
		"LIQUIDATION_BUFFER_PCT":                strconv.FormatFloat(cfg.LiquidationBufferPct, 'f', -1, 64),
		"AUTO_REVIEW_ENABLED":                   strconv.FormatBool(cfg.AutoReviewEnabled),
		"AUTO_REVIEW_AFTER_ORDER_ONLY":          strconv.FormatBool(cfg.AutoReviewAfterOrderOnly),
		"AUTO_REVIEW_INTERVAL_SEC":              strconv.Itoa(cfg.AutoReviewIntervalSec),
		"AUTO_REVIEW_VOLATILITY_PCT":            strconv.FormatFloat(cfg.AutoReviewVolatilityPct, 'f', -1, 64),
		"AUTO_REVIEW_DRAWDOWN_WARN_PCT":         strconv.FormatFloat(cfg.AutoReviewDrawdownWarnPct, 'f', -1, 64),
		"AUTO_REVIEW_LOSS_STREAK_WARN":          strconv.Itoa(cfg.AutoReviewLossStreakWarn),
		"AUTO_REVIEW_RISK_REDUCE_FACTOR":        strconv.FormatFloat(cfg.AutoReviewRiskReduceFactor, 'f', -1, 64),
		"AUTO_STRATEGY_REGEN_ENABLED":           strconv.FormatBool(cfg.AutoStrategyRegenEnabled),
		"AUTO_STRATEGY_REGEN_COOLDOWN_SEC":      strconv.Itoa(cfg.AutoStrategyRegenCooldownSec),
		"AUTO_STRATEGY_REGEN_LOSS_STREAK":       strconv.Itoa(cfg.AutoStrategyRegenLossStreak),
		"AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT": strconv.FormatFloat(cfg.AutoStrategyRegenDrawdownWarnPct, 'f', -1, 64),
		"AUTO_STRATEGY_REGEN_MIN_RR":            strconv.FormatFloat(cfg.AutoStrategyRegenMinRR, 'f', -1, 64),
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		return err
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	applyRuntimeConfigFromEnv()
	return nil
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

func buildEnabledStrategyDetails(enabled []string) []map[string]any {
	if len(enabled) == 0 {
		return []map[string]any{}
	}
	store := readGeneratedStrategies()
	byName := map[string]generatedStrategyRecord{}
	for _, item := range store.Strategies {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		byName[strings.ToLower(name)] = item
	}
	out := make([]map[string]any, 0, len(enabled))
	for _, raw := range enabled {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		source := "manual_external"
		workflowVersion := ""
		workflowChain := []string{}
		lastUpdatedAt := ""
		if item, ok := byName[key]; ok {
			if s := strings.TrimSpace(item.Source); s != "" {
				source = s
			} else {
				source = "workflow_generated"
			}
			workflowVersion = strings.TrimSpace(item.WorkflowVersion)
			workflowChain = append([]string{}, item.WorkflowChain...)
			lastUpdatedAt = strings.TrimSpace(item.LastUpdatedAt)
			if lastUpdatedAt == "" {
				lastUpdatedAt = strings.TrimSpace(item.CreatedAt)
			}
		}
		out = append(out, map[string]any{
			"name":             name,
			"source":           source,
			"workflow_version": workflowVersion,
			"workflow_chain":   workflowChain,
			"last_updated_at":  lastUpdatedAt,
		})
	}
	return out
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
