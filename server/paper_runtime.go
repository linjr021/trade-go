package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"trade-go/trader"
)

const (
	paperRuntimePath       = "data/paper_runtime.json"
	paperRuntimeVersion    = "paper-runtime/v1"
	paperDefaultInterval   = 60
	paperMinInterval       = 5
	paperMaxInterval       = 3600
	paperMaxRecordKeep     = 2000
	paperMaxStrategyHist   = 60
	paperDefaultSimBalance = 200
)

type paperRuntimeConfig struct {
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
	IntervalSec             int      `json:"interval_sec"`
}

type paperTradeRecord struct {
	ID                 string  `json:"id"`
	TS                 string  `json:"ts"`
	Symbol             string  `json:"symbol"`
	Signal             string  `json:"signal"`
	Confidence         string  `json:"confidence"`
	StrategyCombo      string  `json:"strategy_combo"`
	Approved           bool    `json:"approved"`
	ApprovedSize       float64 `json:"approved_size"`
	Price              float64 `json:"price"`
	StopLoss           float64 `json:"stop_loss"`
	TakeProfit         float64 `json:"take_profit"`
	UnrealizedPnL      float64 `json:"unrealized_pnl"`
	Mode               string  `json:"mode"`
	Leverage           int     `json:"leverage"`
	Source             string  `json:"source"`
	RiskReason         string  `json:"risk_reason"`
	ExecutionCode      string  `json:"execution_code"`
	CurrentExecStatus  string  `json:"current_execution_status,omitempty"`
	ExecutionTraceNote string  `json:"execution_trace_note,omitempty"`
}

type paperStrategyHistoryEntry struct {
	ID         string                 `json:"id"`
	TS         string                 `json:"ts"`
	Strategies []string               `json:"strategies"`
	Source     string                 `json:"source"`
	Meta       map[string]interface{} `json:"meta"`
	Params     map[string]interface{} `json:"params"`
}

type paperRuntimeState struct {
	Version           string                                  `json:"version"`
	UpdatedAt         string                                  `json:"updated_at"`
	Config            paperRuntimeConfig                      `json:"config"`
	Running           bool                                    `json:"running"`
	LastRunAt         time.Time                               `json:"last_run_at"`
	NextRunAt         time.Time                               `json:"next_run_at"`
	LastError         string                                  `json:"last_error"`
	LatestDecisionMap map[string]trader.PaperSimulationResult `json:"latest_decision_map"`
	Records           []paperTradeRecord                      `json:"records"`
	PnLBaselineMap    map[string]float64                      `json:"pnl_baseline_map"`
	RiskResetAtMap    map[string]string                       `json:"risk_reset_at_map"`
	StrategyHistory   []paperStrategyHistoryEntry             `json:"strategy_history"`
}

type paperConfigPatch struct {
	Symbol                  *string   `json:"symbol"`
	Balance                 *float64  `json:"balance"`
	PositionSizingMode      *string   `json:"position_sizing_mode"`
	HighConfidenceAmount    *float64  `json:"high_confidence_amount"`
	LowConfidenceAmount     *float64  `json:"low_confidence_amount"`
	HighConfidenceMarginPct *float64  `json:"high_confidence_margin_pct"`
	LowConfidenceMarginPct  *float64  `json:"low_confidence_margin_pct"`
	Leverage                *int      `json:"leverage"`
	MaxRiskPerTradePct      *float64  `json:"max_risk_per_trade_pct"`
	MaxPositionPct          *float64  `json:"max_position_pct"`
	MaxConsecutiveLosses    *int      `json:"max_consecutive_losses"`
	MaxDailyLossPct         *float64  `json:"max_daily_loss_pct"`
	MaxDrawdownPct          *float64  `json:"max_drawdown_pct"`
	LiquidationBufferPct    *float64  `json:"liquidation_buffer_pct"`
	EnabledStrategies       *[]string `json:"enabled_strategies"`
	IntervalSec             *int      `json:"interval_sec"`
}

type paperResetPnLRequest struct {
	Symbol string `json:"symbol"`
}

func (s *Service) initPaperRuntime() {
	defaultCfg := s.defaultPaperRuntimeConfig()
	state := paperRuntimeState{
		Version:           paperRuntimeVersion,
		UpdatedAt:         time.Now().Format(time.RFC3339),
		Config:            defaultCfg,
		Running:           false,
		LastRunAt:         time.Time{},
		NextRunAt:         time.Time{},
		LastError:         "",
		LatestDecisionMap: map[string]trader.PaperSimulationResult{},
		Records:           []paperTradeRecord{},
		PnLBaselineMap:    map[string]float64{},
		RiskResetAtMap:    map[string]string{},
		StrategyHistory:   []paperStrategyHistoryEntry{},
	}
	if loaded, err := readPaperRuntimeState(); err == nil {
		state = loaded
	}
	state = s.normalizePaperRuntimeState(state, defaultCfg)
	if state.Running {
		state.Running = false
		state.NextRunAt = time.Time{}
		state.LastError = "服务已重启，模拟交易默认暂停，请手动重新开始"
	}
	if len(state.StrategyHistory) == 0 && len(state.Config.EnabledStrategies) > 0 {
		state.StrategyHistory = append([]paperStrategyHistoryEntry{
			s.makePaperStrategyHistoryEntry(state.Config.EnabledStrategies, state.Config, "paper_init"),
		}, state.StrategyHistory...)
	}
	s.mu.Lock()
	s.paperState = state
	_ = s.persistPaperStateLocked()
	s.mu.Unlock()
}

func (s *Service) defaultPaperRuntimeConfig() paperRuntimeConfig {
	cfg := s.bot.TradeConfig()
	symbol := strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.PositionSizingMode))
	if mode != "contracts" {
		mode = "margin_pct"
	}
	enabled := parseEnabledStrategiesEnv("")
	if len(enabled) == 0 {
		for _, item := range readGeneratedStrategies().Strategies {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			enabled = append(enabled, name)
			if len(enabled) >= 3 {
				break
			}
		}
	}
	out := paperRuntimeConfig{
		Symbol:                  symbol,
		Balance:                 paperDefaultSimBalance,
		PositionSizingMode:      mode,
		HighConfidenceAmount:    cfg.HighConfidenceAmount,
		LowConfidenceAmount:     cfg.LowConfidenceAmount,
		HighConfidenceMarginPct: cfg.HighConfidenceMarginPct * 100,
		LowConfidenceMarginPct:  cfg.LowConfidenceMarginPct * 100,
		Leverage:                cfg.Leverage,
		MaxRiskPerTradePct:      cfg.MaxRiskPerTradePct,
		MaxPositionPct:          cfg.MaxPositionPct,
		MaxConsecutiveLosses:    cfg.MaxConsecutiveLosses,
		MaxDailyLossPct:         cfg.MaxDailyLossPct,
		MaxDrawdownPct:          cfg.MaxDrawdownPct,
		LiquidationBufferPct:    cfg.LiquidationBufferPct,
		EnabledStrategies:       enabled,
		IntervalSec:             paperDefaultInterval,
	}
	return s.normalizePaperRuntimeConfig(out, out)
}

func readPaperRuntimeState() (paperRuntimeState, error) {
	raw, err := os.ReadFile(paperRuntimePath)
	if err != nil {
		return paperRuntimeState{}, err
	}
	var out paperRuntimeState
	if err := json.Unmarshal(raw, &out); err != nil {
		return paperRuntimeState{}, err
	}
	return out, nil
}

func writePaperRuntimeState(st paperRuntimeState) error {
	st.Version = paperRuntimeVersion
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paperRuntimePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(paperRuntimePath, append(raw, '\n'), 0o644)
}

func (s *Service) persistPaperStateLocked() error {
	return writePaperRuntimeState(s.paperState)
}

func (s *Service) paperRuntimeSummaryLocked() map[string]any {
	return map[string]any{
		"running":      s.paperState.Running,
		"symbol":       s.paperState.Config.Symbol,
		"interval_sec": s.paperState.Config.IntervalSec,
		"last_run_at":  s.paperState.LastRunAt,
		"next_run_at":  s.paperState.NextRunAt,
		"last_error":   s.paperState.LastError,
	}
}

func (s *Service) normalizePaperRuntimeState(in paperRuntimeState, fallback paperRuntimeConfig) paperRuntimeState {
	out := in
	out.Version = paperRuntimeVersion
	out.Config = s.normalizePaperRuntimeConfig(out.Config, fallback)
	out.UpdatedAt = strings.TrimSpace(out.UpdatedAt)
	if out.UpdatedAt == "" {
		out.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	if out.LatestDecisionMap == nil {
		out.LatestDecisionMap = map[string]trader.PaperSimulationResult{}
	}
	if out.PnLBaselineMap == nil {
		out.PnLBaselineMap = map[string]float64{}
	}
	if out.RiskResetAtMap == nil {
		out.RiskResetAtMap = map[string]string{}
	}
	if out.Records == nil {
		out.Records = []paperTradeRecord{}
	}
	if len(out.Records) > paperMaxRecordKeep {
		out.Records = append([]paperTradeRecord{}, out.Records[:paperMaxRecordKeep]...)
	}
	if out.StrategyHistory == nil {
		out.StrategyHistory = []paperStrategyHistoryEntry{}
	}
	if len(out.StrategyHistory) > paperMaxStrategyHist {
		out.StrategyHistory = append([]paperStrategyHistoryEntry{}, out.StrategyHistory[:paperMaxStrategyHist]...)
	}
	return out
}

func (s *Service) normalizePaperRuntimeConfig(in paperRuntimeConfig, fallback paperRuntimeConfig) paperRuntimeConfig {
	out := in
	symbol := strings.ToUpper(strings.TrimSpace(out.Symbol))
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(fallback.Symbol))
	}
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	out.Symbol = symbol

	mode := strings.ToLower(strings.TrimSpace(out.PositionSizingMode))
	if mode != "contracts" {
		mode = "margin_pct"
	}
	out.PositionSizingMode = mode

	if !isFinite(out.Balance) {
		out.Balance = fallback.Balance
	}
	if !isFinite(out.Balance) || out.Balance < 0 {
		out.Balance = paperDefaultSimBalance
	}
	out.Balance = round2(out.Balance)

	if !isFinite(out.HighConfidenceAmount) {
		out.HighConfidenceAmount = fallback.HighConfidenceAmount
	}
	if !isFinite(out.LowConfidenceAmount) {
		out.LowConfidenceAmount = fallback.LowConfidenceAmount
	}
	out.HighConfidenceAmount = round2(paperClamp(out.HighConfidenceAmount, 0, 1_000_000))
	out.LowConfidenceAmount = round2(paperClamp(out.LowConfidenceAmount, 0, 1_000_000))

	if !isFinite(out.HighConfidenceMarginPct) {
		out.HighConfidenceMarginPct = fallback.HighConfidenceMarginPct
	}
	if !isFinite(out.LowConfidenceMarginPct) {
		out.LowConfidenceMarginPct = fallback.LowConfidenceMarginPct
	}
	out.HighConfidenceMarginPct = round2(paperClamp(out.HighConfidenceMarginPct, 0, 100))
	out.LowConfidenceMarginPct = round2(paperClamp(out.LowConfidenceMarginPct, 0, 100))

	if out.Leverage <= 0 {
		out.Leverage = fallback.Leverage
	}
	if out.Leverage <= 0 {
		out.Leverage = 1
	}
	if out.Leverage > 150 {
		out.Leverage = 150
	}

	if !isFinite(out.MaxRiskPerTradePct) || out.MaxRiskPerTradePct <= 0 {
		out.MaxRiskPerTradePct = fallback.MaxRiskPerTradePct
	}
	if !isFinite(out.MaxRiskPerTradePct) || out.MaxRiskPerTradePct <= 0 {
		out.MaxRiskPerTradePct = 0.01
	}
	out.MaxRiskPerTradePct = paperClamp(out.MaxRiskPerTradePct, 0.0001, 1)

	if !isFinite(out.MaxPositionPct) || out.MaxPositionPct <= 0 {
		out.MaxPositionPct = fallback.MaxPositionPct
	}
	if !isFinite(out.MaxPositionPct) || out.MaxPositionPct <= 0 {
		out.MaxPositionPct = 0.20
	}
	out.MaxPositionPct = paperClamp(out.MaxPositionPct, 0.0001, 1)

	if out.MaxConsecutiveLosses < 0 {
		out.MaxConsecutiveLosses = fallback.MaxConsecutiveLosses
	}
	if out.MaxConsecutiveLosses < 0 {
		out.MaxConsecutiveLosses = 3
	}
	if out.MaxConsecutiveLosses > 100 {
		out.MaxConsecutiveLosses = 100
	}

	if !isFinite(out.MaxDailyLossPct) || out.MaxDailyLossPct <= 0 {
		out.MaxDailyLossPct = fallback.MaxDailyLossPct
	}
	if !isFinite(out.MaxDailyLossPct) || out.MaxDailyLossPct <= 0 {
		out.MaxDailyLossPct = 0.05
	}
	out.MaxDailyLossPct = paperClamp(out.MaxDailyLossPct, 0.0001, 1)

	if !isFinite(out.MaxDrawdownPct) || out.MaxDrawdownPct <= 0 {
		out.MaxDrawdownPct = fallback.MaxDrawdownPct
	}
	if !isFinite(out.MaxDrawdownPct) || out.MaxDrawdownPct <= 0 {
		out.MaxDrawdownPct = 0.12
	}
	out.MaxDrawdownPct = paperClamp(out.MaxDrawdownPct, 0.0001, 1)

	if !isFinite(out.LiquidationBufferPct) || out.LiquidationBufferPct <= 0 {
		out.LiquidationBufferPct = fallback.LiquidationBufferPct
	}
	if !isFinite(out.LiquidationBufferPct) || out.LiquidationBufferPct <= 0 {
		out.LiquidationBufferPct = 0.02
	}
	out.LiquidationBufferPct = paperClamp(out.LiquidationBufferPct, 0.0001, 1)

	if out.IntervalSec <= 0 {
		out.IntervalSec = fallback.IntervalSec
	}
	// 兼容旧版本默认值（8 秒），自动迁移到当前默认值，避免过高频率触发。
	if out.IntervalSec == 8 && paperDefaultInterval != 8 {
		out.IntervalSec = paperDefaultInterval
	}
	if out.IntervalSec <= 0 {
		out.IntervalSec = paperDefaultInterval
	}
	if out.IntervalSec < paperMinInterval {
		out.IntervalSec = paperMinInterval
	}
	if out.IntervalSec > paperMaxInterval {
		out.IntervalSec = paperMaxInterval
	}

	out.EnabledStrategies = s.normalizePaperStrategySelection(out.EnabledStrategies)
	return out
}

func (s *Service) normalizePaperStrategySelection(in []string) []string {
	clean := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		name := strings.TrimSpace(item)
		if name == "" || isDeprecatedBuiltinStrategy(name) {
			continue
		}
		k := strings.ToLower(name)
		if seen[k] {
			continue
		}
		seen[k] = true
		clean = append(clean, name)
	}
	available := paperAvailableStrategyNames()
	if len(available) > 0 {
		availSet := map[string]bool{}
		for _, item := range available {
			availSet[strings.ToLower(strings.TrimSpace(item))] = true
		}
		filtered := make([]string, 0, len(clean))
		for _, item := range clean {
			if availSet[strings.ToLower(strings.TrimSpace(item))] {
				filtered = append(filtered, item)
			}
		}
		clean = filtered
		if len(clean) == 0 {
			clean = append(clean, available[0])
		}
	}
	if len(clean) > 3 {
		clean = clean[:3]
	}
	return clean
}

func paperAvailableStrategyNames() []string {
	store := readGeneratedStrategies()
	out := make([]string, 0, len(store.Strategies))
	seen := map[string]bool{}
	for _, item := range store.Strategies {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		k := strings.ToLower(name)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, name)
	}
	return out
}

func paperClamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (s *Service) clonePaperStateLocked() paperRuntimeState {
	src := s.paperState
	dst := src

	dst.Config.EnabledStrategies = append([]string{}, src.Config.EnabledStrategies...)
	dst.Records = append([]paperTradeRecord{}, src.Records...)
	dst.StrategyHistory = append([]paperStrategyHistoryEntry{}, src.StrategyHistory...)

	dst.PnLBaselineMap = map[string]float64{}
	for k, v := range src.PnLBaselineMap {
		dst.PnLBaselineMap[k] = v
	}
	dst.RiskResetAtMap = map[string]string{}
	for k, v := range src.RiskResetAtMap {
		key := strings.ToUpper(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		dst.RiskResetAtMap[key] = strings.TrimSpace(v)
	}

	dst.LatestDecisionMap = map[string]trader.PaperSimulationResult{}
	for k, v := range src.LatestDecisionMap {
		dst.LatestDecisionMap[k] = v
	}

	return dst
}

func (s *Service) handlePaperState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 500
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > paperMaxRecordKeep {
		limit = paperMaxRecordKeep
	}
	filterSymbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))

	s.mu.RLock()
	st := s.clonePaperStateLocked()
	s.mu.RUnlock()

	records := st.Records
	if filterSymbol != "" {
		filtered := make([]paperTradeRecord, 0, len(records))
		for _, row := range records {
			symbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
			if symbol == "" || symbol == filterSymbol {
				filtered = append(filtered, row)
			}
		}
		records = filtered
	}
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config":              st.Config,
		"runtime":             s.paperRuntimeSummary(st),
		"latest_decision_map": st.LatestDecisionMap,
		"records":             records,
		"pnl_baseline_map":    st.PnLBaselineMap,
		"risk_reset_at_map":   st.RiskResetAtMap,
		"strategy_history":    st.StrategyHistory,
	})
}

func (s *Service) paperRuntimeSummary(st paperRuntimeState) map[string]any {
	return map[string]any{
		"running":      st.Running,
		"symbol":       st.Config.Symbol,
		"interval_sec": st.Config.IntervalSec,
		"last_run_at":  st.LastRunAt,
		"next_run_at":  st.NextRunAt,
		"last_error":   st.LastError,
	}
}

func (s *Service) handlePaperConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req paperConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updated, err := s.applyPaperConfigPatch(req, "paper_config")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "paper config updated",
		"config":  updated,
	})
}

func (s *Service) applyPaperConfigPatch(req paperConfigPatch, source string) (paperRuntimeConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.paperState.Config
	if req.Symbol != nil {
		next.Symbol = strings.ToUpper(strings.TrimSpace(*req.Symbol))
	}
	if req.Balance != nil {
		next.Balance = *req.Balance
	}
	if req.PositionSizingMode != nil {
		next.PositionSizingMode = strings.ToLower(strings.TrimSpace(*req.PositionSizingMode))
	}
	if req.HighConfidenceAmount != nil {
		next.HighConfidenceAmount = *req.HighConfidenceAmount
	}
	if req.LowConfidenceAmount != nil {
		next.LowConfidenceAmount = *req.LowConfidenceAmount
	}
	if req.HighConfidenceMarginPct != nil {
		next.HighConfidenceMarginPct = *req.HighConfidenceMarginPct
	}
	if req.LowConfidenceMarginPct != nil {
		next.LowConfidenceMarginPct = *req.LowConfidenceMarginPct
	}
	if req.Leverage != nil {
		next.Leverage = *req.Leverage
	}
	if req.MaxRiskPerTradePct != nil {
		next.MaxRiskPerTradePct = *req.MaxRiskPerTradePct
	}
	if req.MaxPositionPct != nil {
		next.MaxPositionPct = *req.MaxPositionPct
	}
	if req.MaxConsecutiveLosses != nil {
		next.MaxConsecutiveLosses = *req.MaxConsecutiveLosses
	}
	if req.MaxDailyLossPct != nil {
		next.MaxDailyLossPct = *req.MaxDailyLossPct
	}
	if req.MaxDrawdownPct != nil {
		next.MaxDrawdownPct = *req.MaxDrawdownPct
	}
	if req.LiquidationBufferPct != nil {
		next.LiquidationBufferPct = *req.LiquidationBufferPct
	}
	if req.EnabledStrategies != nil {
		next.EnabledStrategies = append([]string{}, (*req.EnabledStrategies)...)
	}
	if req.IntervalSec != nil {
		next.IntervalSec = *req.IntervalSec
	}

	prevKey := strings.Join(s.paperState.Config.EnabledStrategies, "|")
	next = s.normalizePaperRuntimeConfig(next, s.defaultPaperRuntimeConfig())
	if next.Symbol == "" {
		return s.paperState.Config, ErrBadRequest("symbol is required")
	}
	if next.Balance < 0 {
		return s.paperState.Config, ErrBadRequest("balance cannot be negative")
	}
	s.paperState.Config = next
	nextKey := strings.Join(next.EnabledStrategies, "|")
	if nextKey != prevKey && len(next.EnabledStrategies) > 0 {
		s.paperState.StrategyHistory = append([]paperStrategyHistoryEntry{
			s.makePaperStrategyHistoryEntry(next.EnabledStrategies, next, source),
		}, s.paperState.StrategyHistory...)
		if len(s.paperState.StrategyHistory) > paperMaxStrategyHist {
			s.paperState.StrategyHistory = s.paperState.StrategyHistory[:paperMaxStrategyHist]
		}
	}
	if s.paperState.Running {
		s.paperState.NextRunAt = time.Now().Add(time.Duration(next.IntervalSec) * time.Second)
	}
	_ = s.persistPaperStateLocked()
	return next, nil
}

func (s *Service) makePaperStrategyHistoryEntry(strategies []string, cfg paperRuntimeConfig, source string) paperStrategyHistoryEntry {
	list := append([]string{}, strategies...)
	meta := s.paperStrategyMetaMap(list)
	params := map[string]interface{}{
		"positionSizingMode":      cfg.PositionSizingMode,
		"leverage":                cfg.Leverage,
		"highConfidenceAmount":    cfg.HighConfidenceAmount,
		"lowConfidenceAmount":     cfg.LowConfidenceAmount,
		"highConfidenceMarginPct": cfg.HighConfidenceMarginPct,
		"lowConfidenceMarginPct":  cfg.LowConfidenceMarginPct,
		"maxRiskPerTradePct":      cfg.MaxRiskPerTradePct,
		"maxPositionPct":          cfg.MaxPositionPct,
		"maxConsecutiveLosses":    cfg.MaxConsecutiveLosses,
		"maxDailyLossPct":         cfg.MaxDailyLossPct,
		"maxDrawdownPct":          cfg.MaxDrawdownPct,
		"liquidationBufferPct":    cfg.LiquidationBufferPct,
	}
	now := time.Now().Format(time.RFC3339)
	return paperStrategyHistoryEntry{
		ID:         "paper_strategy_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		TS:         now,
		Strategies: list,
		Source:     strings.TrimSpace(source),
		Meta:       meta,
		Params:     params,
	}
}

func (s *Service) paperStrategyMetaMap(strategies []string) map[string]interface{} {
	store := readGeneratedStrategies()
	byName := map[string]generatedStrategyRecord{}
	for _, item := range store.Strategies {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if name == "" {
			continue
		}
		byName[name] = item
	}
	out := map[string]interface{}{}
	for _, strategy := range strategies {
		name := strings.TrimSpace(strategy)
		if name == "" {
			continue
		}
		item, ok := byName[strings.ToLower(name)]
		sourceLabel := "外部/手动"
		workflowVersion := "skill-workflow/v1"
		lastUpdatedAt := ""
		if ok {
			sourceLabel = paperStrategySourceLabel(item.Source)
			if v := strings.TrimSpace(item.WorkflowVersion); v != "" {
				workflowVersion = v
			}
			lastUpdatedAt = strings.TrimSpace(item.LastUpdatedAt)
			if lastUpdatedAt == "" {
				lastUpdatedAt = strings.TrimSpace(item.CreatedAt)
			}
		}
		out[name] = map[string]interface{}{
			"source":          sourceLabel,
			"workflowVersion": workflowVersion,
			"lastUpdatedAt":   lastUpdatedAt,
		}
	}
	return out
}

func paperStrategySourceLabel(source string) string {
	switch normalizeStrategySource(source) {
	case "workflow_generated":
		return "工作流生成"
	case "auto_regen":
		return "自动重生成"
	case "manual_external":
		return "外部/手动"
	default:
		return "外部/手动"
	}
}

func (s *Service) handlePaperStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req paperConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Symbol != nil || req.Balance != nil || req.PositionSizingMode != nil ||
		req.HighConfidenceAmount != nil || req.LowConfidenceAmount != nil ||
		req.HighConfidenceMarginPct != nil || req.LowConfidenceMarginPct != nil ||
		req.Leverage != nil || req.MaxRiskPerTradePct != nil || req.MaxPositionPct != nil ||
		req.MaxConsecutiveLosses != nil || req.MaxDailyLossPct != nil || req.MaxDrawdownPct != nil ||
		req.LiquidationBufferPct != nil || req.EnabledStrategies != nil || req.IntervalSec != nil {
		if _, err := s.applyPaperConfigPatch(req, "paper_start"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	s.mu.Lock()
	if s.paperState.Running {
		state := s.clonePaperStateLocked()
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "paper simulation already running",
			"runtime": s.paperRuntimeSummary(state),
		})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.paperCancel = cancel
	s.paperState.Running = true
	s.paperState.LastError = ""
	s.paperState.NextRunAt = time.Now()
	_ = s.persistPaperStateLocked()
	s.mu.Unlock()

	go s.paperLoop(ctx)

	s.mu.RLock()
	state := s.clonePaperStateLocked()
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "paper simulation started",
		"runtime": s.paperRuntimeSummary(state),
	})
}

func (s *Service) handlePaperStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	cancel := s.paperCancel
	s.paperCancel = nil
	s.paperState.Running = false
	s.paperState.NextRunAt = time.Time{}
	_ = s.persistPaperStateLocked()
	state := s.clonePaperStateLocked()
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "paper simulation stopped",
		"runtime": s.paperRuntimeSummary(state),
	})
}

func (s *Service) handlePaperResetPnL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req paperResetPnLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	s.mu.Lock()
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(s.paperState.Config.Symbol))
	}
	if symbol == "" {
		symbol = "BTCUSDT"
	}

	removed := 0
	kept := make([]paperTradeRecord, 0, len(s.paperState.Records))
	for _, row := range s.paperState.Records {
		rowSymbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
		if rowSymbol == "" || rowSymbol == symbol {
			removed++
			continue
		}
		kept = append(kept, row)
	}
	s.paperState.Records = kept

	if s.paperState.LatestDecisionMap == nil {
		s.paperState.LatestDecisionMap = map[string]trader.PaperSimulationResult{}
	}
	delete(s.paperState.LatestDecisionMap, symbol)

	if s.paperState.PnLBaselineMap == nil {
		s.paperState.PnLBaselineMap = map[string]float64{}
	}
	// 重置后以“清空后快照”为基线，保证模拟总盈亏归零。
	s.paperState.PnLBaselineMap[symbol] = 0
	_ = s.persistPaperStateLocked()
	st := s.clonePaperStateLocked()
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"symbol":              symbol,
		"baseline":            st.PnLBaselineMap[symbol],
		"cleared_records":     removed,
		"message":             "paper simulation state reset",
		"config":              st.Config,
		"runtime":             s.paperRuntimeSummary(st),
		"latest_decision_map": st.LatestDecisionMap,
		"records":             st.Records,
		"pnl_baseline_map":    st.PnLBaselineMap,
		"risk_reset_at_map":   st.RiskResetAtMap,
		"strategy_history":    st.StrategyHistory,
	})
}

type paperRiskResetRequest struct {
	Symbol string `json:"symbol"`
	Reason string `json:"reason"`
}

func (s *Service) handlePaperRiskReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req paperRiskResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	principal, _ := principalFromRequest(r)
	operator := strings.TrimSpace(principal.Username)
	if operator == "" {
		operator = "unknown"
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual_clear_paper"
	}
	symbol, resetAt, err := s.resetPaperRiskBaseline(req.Symbol)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "重置模拟风控基线失败: "+err.Error())
		return
	}
	_ = s.saveAuthAudit(r, principal, "paper_risk_manual_reset", "paper", symbol, "ok", map[string]any{
		"reason":   reason,
		"reset_at": resetAt,
		"symbol":   symbol,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "模拟风控基线已重置",
		"symbol":   symbol,
		"reset_at": resetAt,
		"operator": operator,
		"reason":   reason,
	})
}

func (s *Service) resetPaperRiskBaseline(rawSymbol string) (string, string, error) {
	symbol := strings.ToUpper(strings.TrimSpace(rawSymbol))
	s.mu.Lock()
	defer s.mu.Unlock()
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(s.paperState.Config.Symbol))
	}
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	if s.paperState.RiskResetAtMap == nil {
		s.paperState.RiskResetAtMap = map[string]string{}
	}
	resetAt := time.Now().Format(time.RFC3339)
	s.paperState.RiskResetAtMap[symbol] = resetAt
	if err := s.persistPaperStateLocked(); err != nil {
		return "", "", err
	}
	return symbol, resetAt, nil
}

func (s *Service) paperLoop(ctx context.Context) {
	s.runPaperCycle()
	for {
		s.mu.RLock()
		running := s.paperState.Running
		interval := s.paperState.Config.IntervalSec
		s.mu.RUnlock()
		if !running {
			return
		}
		if interval < paperMinInterval {
			interval = paperDefaultInterval
		}
		nextAt := time.Now().Add(time.Duration(interval) * time.Second)
		s.mu.Lock()
		if !s.paperState.Running {
			s.mu.Unlock()
			return
		}
		s.paperState.NextRunAt = nextAt
		_ = s.persistPaperStateLocked()
		s.mu.Unlock()

		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.runPaperCycle()
		}
	}
}

func (s *Service) runPaperCycle() {
	s.mu.RLock()
	cfg := s.paperState.Config
	running := s.paperState.Running
	resetAt := ""
	if s.paperState.RiskResetAtMap != nil {
		resetAt = strings.TrimSpace(s.paperState.RiskResetAtMap[strings.ToUpper(strings.TrimSpace(cfg.Symbol))])
	}
	riskSnap := buildPaperRiskSnapshot(s.paperState.Records, cfg.Symbol, cfg.Balance, resetAt)
	s.mu.RUnlock()
	if !running {
		return
	}

	input := trader.PaperSimulationInput{
		Symbol:                  cfg.Symbol,
		Balance:                 cfg.Balance,
		PositionSizingMode:      cfg.PositionSizingMode,
		HighConfidenceAmount:    cfg.HighConfidenceAmount,
		LowConfidenceAmount:     cfg.LowConfidenceAmount,
		HighConfidenceMarginPct: cfg.HighConfidenceMarginPct,
		LowConfidenceMarginPct:  cfg.LowConfidenceMarginPct,
		Leverage:                cfg.Leverage,
		MaxRiskPerTradePct:      cfg.MaxRiskPerTradePct,
		MaxPositionPct:          cfg.MaxPositionPct,
		MaxConsecutiveLosses:    cfg.MaxConsecutiveLosses,
		MaxDailyLossPct:         cfg.MaxDailyLossPct,
		MaxDrawdownPct:          cfg.MaxDrawdownPct,
		LiquidationBufferPct:    cfg.LiquidationBufferPct,
		RiskTodayPnL:            riskSnap.TodayPnL,
		RiskPeakEquity:          riskSnap.PeakEquity,
		RiskCurrentEquity:       riskSnap.CurrentEquity,
		RiskConsecutiveLosses:   riskSnap.ConsecutiveLosses,
		EnabledStrategies:       append([]string{}, cfg.EnabledStrategies...),
	}

	s.runMu.Lock()
	result, err := s.bot.RunPaperSimulation(input)
	s.runMu.Unlock()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.paperState.Running {
		return
	}
	s.paperState.LastRunAt = now
	s.paperState.NextRunAt = now.Add(time.Duration(cfg.IntervalSec) * time.Second)
	if err != nil {
		s.paperState.LastError = err.Error()
	} else {
		s.paperState.LastError = ""
	}
	symbol := strings.ToUpper(strings.TrimSpace(result.Symbol))
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	}
	result.Symbol = symbol
	if s.paperState.LatestDecisionMap == nil {
		s.paperState.LatestDecisionMap = map[string]trader.PaperSimulationResult{}
	}
	s.paperState.LatestDecisionMap[symbol] = result
	if shouldPersistPaperTradeRecord(result) {
		record := buildPaperTradeRecord(result, s.paperState.Records)
		s.paperState.Records = append([]paperTradeRecord{record}, s.paperState.Records...)
		if len(s.paperState.Records) > paperMaxRecordKeep {
			s.paperState.Records = s.paperState.Records[:paperMaxRecordKeep]
		}
	}
	_ = s.persistPaperStateLocked()
}

func shouldPersistPaperTradeRecord(result trader.PaperSimulationResult) bool {
	signal := strings.ToUpper(strings.TrimSpace(result.Signal))
	if signal == "" || signal == "HOLD" {
		return false
	}
	if !result.Approved {
		return false
	}
	return result.ApprovedSize > 0
}

func buildPaperTradeRecord(result trader.PaperSimulationResult, existing []paperTradeRecord) paperTradeRecord {
	symbol := strings.ToUpper(strings.TrimSpace(result.Symbol))
	prevPrice := result.Price
	for _, row := range existing {
		if strings.ToUpper(strings.TrimSpace(row.Symbol)) == symbol {
			if row.Price > 0 {
				prevPrice = row.Price
			}
			break
		}
	}
	pnl := calcPaperRuntimePnL(result.Signal, prevPrice, result.Price, result.ApprovedSize)
	ts := result.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	mode := strings.ToLower(strings.TrimSpace(result.PositionSizingMode))
	if mode != "contracts" {
		mode = "margin_pct"
	}
	return paperTradeRecord{
		ID:            strings.TrimSpace(result.ID),
		TS:            ts.Format(time.RFC3339),
		Symbol:        symbol,
		Signal:        strings.ToUpper(strings.TrimSpace(result.Signal)),
		Confidence:    strings.ToUpper(strings.TrimSpace(result.Confidence)),
		StrategyCombo: strings.TrimSpace(result.StrategyCombo),
		Approved:      result.Approved,
		ApprovedSize:  round2(result.ApprovedSize),
		Price:         result.Price,
		StopLoss:      result.StopLoss,
		TakeProfit:    result.TakeProfit,
		UnrealizedPnL: round2(pnl),
		Mode:          mode,
		Leverage:      result.Leverage,
		Source:        strings.TrimSpace(result.Source),
		RiskReason:    strings.TrimSpace(result.RiskReason),
		ExecutionCode: strings.TrimSpace(result.ExecutionCode),
	}
}

type paperRiskSnapshot struct {
	TodayPnL          float64
	PeakEquity        float64
	CurrentEquity     float64
	ConsecutiveLosses int
}

func buildPaperRiskSnapshot(records []paperTradeRecord, symbol string, baseBalance float64, resetAt string) paperRiskSnapshot {
	base := baseBalance
	if !isFinite(base) || base <= 0 {
		base = paperDefaultSimBalance
	}
	target := strings.ToUpper(strings.TrimSpace(symbol))
	resetAt = strings.TrimSpace(resetAt)
	var resetAtTime time.Time
	var hasReset bool
	if resetAt != "" {
		if t, err := time.Parse(time.RFC3339, resetAt); err == nil {
			resetAtTime = t
			hasReset = true
		} else if t, err := time.Parse(time.RFC3339Nano, resetAt); err == nil {
			resetAtTime = t
			hasReset = true
		}
	}
	snap := paperRiskSnapshot{
		TodayPnL:          0,
		PeakEquity:        base,
		CurrentEquity:     base,
		ConsecutiveLosses: 0,
	}
	today := time.Now().Format("2006-01-02")
	equity := base
	for i := len(records) - 1; i >= 0; i-- {
		row := records[i]
		rowSymbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
		if target != "" && rowSymbol != "" && rowSymbol != target {
			continue
		}
		rowTS := strings.TrimSpace(row.TS)
		if hasReset {
			if ts, ok := parsePaperRecordTime(rowTS); ok {
				if ts.Before(resetAtTime) {
					continue
				}
			} else if rowTS != "" && resetAt != "" && rowTS < resetAt {
				continue
			}
		}
		pnl := row.UnrealizedPnL
		if !isFinite(pnl) {
			continue
		}
		equity += pnl
		if equity > snap.PeakEquity {
			snap.PeakEquity = equity
		}
		if ts, ok := parsePaperRecordTime(rowTS); ok {
			if ts.In(time.Local).Format("2006-01-02") == today {
				snap.TodayPnL += pnl
			}
		} else if strings.HasPrefix(rowTS, today) {
			snap.TodayPnL += pnl
		}
	}
	snap.CurrentEquity = equity
	if snap.PeakEquity < snap.CurrentEquity {
		snap.PeakEquity = snap.CurrentEquity
	}
	for _, row := range records {
		rowSymbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
		if target != "" && rowSymbol != "" && rowSymbol != target {
			continue
		}
		rowTS := strings.TrimSpace(row.TS)
		if hasReset {
			if ts, ok := parsePaperRecordTime(rowTS); ok {
				if ts.Before(resetAtTime) {
					continue
				}
			} else if rowTS != "" && resetAt != "" && rowTS < resetAt {
				continue
			}
		}
		pnl := row.UnrealizedPnL
		if !isFinite(pnl) {
			continue
		}
		if pnl < 0 {
			snap.ConsecutiveLosses++
			continue
		}
		break
	}
	return snap
}

func parsePaperRecordTime(raw string) (time.Time, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func calcPaperRuntimePnL(signal string, lastPrice, currentPrice, size float64) float64 {
	if size <= 0 || lastPrice <= 0 || currentPrice <= 0 {
		return 0
	}
	side := strings.ToUpper(strings.TrimSpace(signal))
	switch side {
	case "BUY":
		return (currentPrice - lastPrice) * size
	case "SELL":
		return (lastPrice - currentPrice) * size
	default:
		return 0
	}
}

type badRequestError struct {
	msg string
}

func (e badRequestError) Error() string { return e.msg }

func ErrBadRequest(msg string) error {
	return badRequestError{msg: strings.TrimSpace(msg)}
}
