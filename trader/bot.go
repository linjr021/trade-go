package trader

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"trade-go/ai"
	"trade-go/config"
	"trade-go/exchange"
	"trade-go/indicators"
	"trade-go/models"
	"trade-go/risk"
	"trade-go/storage"
)

type RuntimeSnapshot struct {
	LastRunAt       time.Time           `json:"last_run_at"`
	LastError       string              `json:"last_error"`
	LastSignal      *models.TradeSignal `json:"last_signal"`
	LastPrice       *models.PriceData   `json:"last_price"`
	CurrentPosition *models.Position    `json:"current_position"`
}

type TradeSettingsUpdate struct {
	Symbol                  *string
	HighConfidenceAmount    *float64
	LowConfidenceAmount     *float64
	PositionSizingMode      *string
	HighConfidenceMarginPct *float64
	LowConfidenceMarginPct  *float64
	Leverage                *int
	MaxRiskPerTradePct      *float64
	MaxPositionPct          *float64
	MaxConsecutiveLosses    *int
	MaxDailyLossPct         *float64
	MaxDrawdownPct          *float64
	LiquidationBufferPct    *float64
}

// Bot 交易机器人
type Bot struct {
	exchange      *exchange.Client
	aiClient      *ai.Client
	riskEngine    *risk.Engine
	store         *storage.Store
	signalHistory []models.TradeSignal
	cfg           *config.TradeConfig
	mu            sync.RWMutex
	runtime       RuntimeSnapshot
}

func NewBot() *Bot {
	cfg := &config.Config.Trade
	return &Bot{
		exchange:   exchange.NewClient(),
		aiClient:   ai.NewClient(),
		riskEngine: risk.NewEngine(cfg),
		cfg:        cfg,
	}
}

func (b *Bot) SetStore(s *storage.Store) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store = s
}

func (b *Bot) HasStore() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.store != nil
}

func (b *Bot) ReloadClients() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reload clients panic: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.exchange = exchange.NewClient()
	b.aiClient = ai.NewClient()
	return nil
}

// Setup 初始化交易所设置
func (b *Bot) Setup() error {
	cfg := b.TradeConfig()
	if err := b.exchange.SetLeverage(cfg.Symbol, cfg.Leverage); err != nil {
		return fmt.Errorf("设置杠杆失败: %w", err)
	}
	fmt.Printf("设置杠杆倍数: %dx\n", cfg.Leverage)

	balance, err := b.exchange.FetchBalance()
	if err != nil {
		return fmt.Errorf("获取余额失败: %w", err)
	}
	fmt.Printf("当前USDT余额: %.2f\n", balance)

	pos, err := b.exchange.FetchPosition(cfg.Symbol)
	if err == nil && pos != nil {
		_ = b.savePosition(*pos)
	}
	_ = b.saveEquity(balance, pos)
	_ = b.reconcileOpenOrders()
	return nil
}

// Run 单次交易周期
func (b *Bot) Run() {
	cfg := b.TradeConfig()
	b.setRuntime(time.Now(), "", nil, nil, nil)
	fmt.Println("\n" + line())
	fmt.Printf("执行时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(line())

	// 1. 获取行情数据
	priceData, err := b.fetchPriceData()
	if err != nil {
		fmt.Printf("获取行情失败: %v\n", err)
		b.setRuntime(time.Now(), err.Error(), nil, nil, nil)
		return
	}
	fmt.Printf("BTC当前价格: $%.2f | 变化: %+.2f%%\n", priceData.Price, priceData.PriceChange)

	// 2. 获取持仓
	currentPos, err := b.exchange.FetchPosition(cfg.Symbol)
	if err != nil {
		fmt.Printf("获取持仓失败: %v\n", err)
		b.setRuntime(time.Now(), err.Error(), nil, &priceData, nil)
	}

	// 3. AI 分析（带重试）
	signal := b.analyzeWithRetry(priceData, currentPos)
	if signal.IsFallback {
		fmt.Println("⚠️ 使用备用交易信号")
	}
	b.setRuntime(time.Now(), "", &signal, &priceData, currentPos)

	// 4. 风控仓位决策（AI 只决定方向）
	tradeAmount, allow, riskReason := b.buildRiskPosition(signal, priceData, currentPos)
	b.saveAIDecision(signal, priceData, tradeAmount, allow, riskReason)
	if !allow {
		fmt.Printf("⛔ 风控阻断: %s\n", riskReason)
		_ = b.saveRiskEvent("risk_block", riskReason)
		return
	}

	// 4. 执行交易
	b.executeTrade(signal, priceData, currentPos, tradeAmount)
	newPos, _ := b.exchange.FetchPosition(cfg.Symbol)
	if newPos != nil {
		_ = b.savePosition(*newPos)
	}
	newBalance, _ := b.exchange.FetchBalance()
	equity := newBalance
	if newPos != nil {
		equity += newPos.UnrealizedPnL
	}
	if b.store != nil && signal.StrategyCombo != "" {
		if score, err := b.store.UpdateStrategyComboScore(signal.StrategyCombo, equity); err == nil {
			signal.StrategyScore = score
		}
	}
	_ = b.saveEquity(newBalance, newPos)
	b.setRuntime(time.Now(), "", &signal, &priceData, newPos)
}

func (b *Bot) fetchPriceData() (models.PriceData, error) {
	cfg := b.TradeConfig()
	candles, err := b.exchange.FetchOHLCV(cfg.Symbol, cfg.Timeframe, cfg.DataPoints)
	if err != nil {
		return models.PriceData{}, err
	}
	if len(candles) < 2 {
		return models.PriceData{}, fmt.Errorf("K线数据不足")
	}

	ind := indicators.Calculate(candles)
	trend := indicators.AnalyzeTrend(candles, ind)
	levels := indicators.AnalyzeLevels(candles, ind)

	cur := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	priceChange := (cur.Close - prev.Close) / prev.Close * 100

	return models.PriceData{
		Price:       cur.Close,
		Timestamp:   cur.Timestamp,
		High:        cur.High,
		Low:         cur.Low,
		Volume:      cur.Volume,
		Timeframe:   cfg.Timeframe,
		PriceChange: priceChange,
		KlineData:   candles,
		Technical:   ind,
		Trend:       trend,
		Levels:      levels,
	}, nil
}

func (b *Bot) analyzeWithRetry(pd models.PriceData, pos *models.Position) models.TradeSignal {
	history := b.SignalHistory(0)
	for attempt := 0; attempt < 2; attempt++ {
		sig, err := b.aiClient.Analyze(pd, pos, history)
		if err != nil {
			fmt.Printf("第%d次 AI 分析失败: %v\n", attempt+1, err)
			time.Sleep(time.Second)
			continue
		}
		if !sig.IsFallback {
			b.addSignal(sig)
			return sig
		}
		time.Sleep(time.Second)
	}
	fb := models.TradeSignal{
		Signal:        "HOLD",
		Reason:        "AI 分析失败，采取保守策略",
		StopLoss:      pd.Price * 0.98,
		TakeProfit:    pd.Price * 1.02,
		Confidence:    "LOW",
		StrategyCombo: "fallback_conservative",
		IsFallback:    true,
		Timestamp:     time.Now(),
	}
	return fb
}

func (b *Bot) addSignal(sig models.TradeSignal) {
	b.mu.Lock()
	b.signalHistory = append(b.signalHistory, sig)
	if len(b.signalHistory) > 30 {
		b.signalHistory = b.signalHistory[1:]
	}
	snapshot := make([]models.TradeSignal, len(b.signalHistory))
	copy(snapshot, b.signalHistory)
	b.mu.Unlock()

	// 统计
	count := 0
	for _, s := range snapshot {
		if s.Signal == sig.Signal {
			count++
		}
	}
	fmt.Printf("信号统计: %s (最近%d次中出现%d次)\n", sig.Signal, len(snapshot), count)

	// 连续3次检查
	if len(snapshot) >= 3 {
		last3 := snapshot[len(snapshot)-3:]
		all := last3[0].Signal == last3[1].Signal && last3[1].Signal == last3[2].Signal
		if all {
			fmt.Printf("⚠️ 注意：连续3次 %s 信号\n", sig.Signal)
		}
	}
}

func (b *Bot) executeTrade(signal models.TradeSignal, pd models.PriceData, currentPos *models.Position, tradeAmount float64) {
	cfg := b.TradeConfig()
	fmt.Printf("交易信号: %s | 信心: %s\n", signal.Signal, signal.Confidence)
	fmt.Printf("理由: %s\n", signal.Reason)
	fmt.Printf("止损: $%.2f | 止盈: $%.2f\n", signal.StopLoss, signal.TakeProfit)
	fmt.Printf("开仓数量(按信心): %.4f\n", tradeAmount)
	fmt.Printf("当前持仓: %v\n", currentPos)

	// 防频繁反转
	if currentPos != nil && signal.Signal != "HOLD" {
		newSide := "long"
		if signal.Signal == "SELL" {
			newSide = "short"
		}
		if newSide != currentPos.Side {
			if signal.Confidence != "HIGH" {
				fmt.Printf("🔒 非高信心反转信号，保持现有%s仓\n", currentPos.Side)
				return
			}
			history := b.SignalHistory(2)
			if len(history) >= 2 {
				last2 := history[len(history)-2:]
				for _, s := range last2 {
					if s.Signal == signal.Signal {
						fmt.Printf("🔒 近期已出现%s信号，避免频繁反转\n", signal.Signal)
						return
					}
				}
			}
		}
	}

	if cfg.TestMode {
		fmt.Println("测试模式 - 仅模拟，不真实下单")
		return
	}

	// 检查保证金
	balance, err := b.exchange.FetchBalance()
	if err != nil {
		fmt.Printf("获取余额失败: %v\n", err)
		return
	}
	requiredMargin := pd.Price * tradeAmount / float64(cfg.Leverage)
	if requiredMargin > balance*0.8 {
		fmt.Printf("⚠️ 保证金不足，需要 %.2f USDT，可用 %.2f USDT\n", requiredMargin, balance)
		return
	}

	var execErr error
	var orders []models.OrderResult
	switch signal.Signal {
	case "BUY":
		orders, execErr = b.openLong(currentPos, tradeAmount)
	case "SELL":
		orders, execErr = b.openShort(currentPos, tradeAmount)
	default:
		fmt.Println("HOLD - 不操作")
	}

	if execErr != nil {
		fmt.Printf("订单执行失败: %v\n", execErr)
		_ = b.saveRiskEvent("order_error", execErr.Error())
		return
	}
	for _, od := range orders {
		_ = b.saveOrder(od)
		if err := b.confirmOrder(od); err != nil {
			fmt.Printf("订单确认失败: %v\n", err)
			_ = b.saveRiskEvent("order_confirm_error", err.Error())
		}
	}

	fmt.Println("订单执行成功")
	time.Sleep(2 * time.Second)
	newPos, _ := b.exchange.FetchPosition(cfg.Symbol)
	fmt.Printf("更新后持仓: %v\n", newPos)
}

func (b *Bot) setRuntime(runAt time.Time, errMsg string, sig *models.TradeSignal, pd *models.PriceData, pos *models.Position) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.runtime.LastRunAt = runAt
	b.runtime.LastError = errMsg
	if sig != nil {
		s := *sig
		b.runtime.LastSignal = &s
	}
	if pd != nil {
		p := *pd
		b.runtime.LastPrice = &p
	}
	if pos != nil {
		ps := *pos
		b.runtime.CurrentPosition = &ps
	}
}

func (b *Bot) Snapshot() RuntimeSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := b.runtime
	if b.runtime.LastSignal != nil {
		s := *b.runtime.LastSignal
		cp.LastSignal = &s
	}
	if b.runtime.LastPrice != nil {
		p := *b.runtime.LastPrice
		cp.LastPrice = &p
	}
	if b.runtime.CurrentPosition != nil {
		pos := *b.runtime.CurrentPosition
		cp.CurrentPosition = &pos
	}
	return cp
}

func (b *Bot) SignalHistory(limit int) []models.TradeSignal {
	b.mu.RLock()
	defer b.mu.RUnlock()
	total := len(b.signalHistory)
	if limit <= 0 || limit > total {
		limit = total
	}
	start := total - limit
	out := make([]models.TradeSignal, limit)
	copy(out, b.signalHistory[start:])
	return out
}

func (b *Bot) FetchBalance() (float64, error) {
	return b.exchange.FetchBalance()
}

func (b *Bot) FetchPosition() (*models.Position, error) {
	cfg := b.TradeConfig()
	return b.exchange.FetchPosition(cfg.Symbol)
}

func (b *Bot) TradeConfig() config.TradeConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return *b.cfg
}

func (b *Bot) StrategyComboScores(limit int) []storage.StrategyComboScore {
	if b.store == nil {
		return nil
	}
	scores, err := b.store.GetStrategyComboScores(limit)
	if err != nil {
		return nil
	}
	return scores
}

func (b *Bot) TradeRecords(limit int) []storage.TradeRecord {
	if b.store == nil {
		return nil
	}
	records, err := b.store.RecentTradeRecords(limit)
	if err != nil {
		return nil
	}
	return records
}

func (b *Bot) EquitySummary() (storage.EquitySummary, bool) {
	if b.store == nil {
		return storage.EquitySummary{}, false
	}
	out, err := b.store.EquitySummary()
	if err != nil {
		return storage.EquitySummary{}, false
	}
	return out, true
}

func (b *Bot) EquityTrendSince(since time.Time) ([]storage.EquityPoint, bool) {
	if b.store == nil {
		return nil, false
	}
	out, err := b.store.EquityTrendSince(since)
	if err != nil {
		return nil, false
	}
	return out, true
}

func (b *Bot) DailyPnLByMonth(month string) ([]storage.DailyPnL, bool) {
	if b.store == nil {
		return nil, false
	}
	out, err := b.store.DailyPnLByMonth(month)
	if err != nil {
		return nil, false
	}
	return out, true
}

func (b *Bot) SaveBacktestRun(run storage.BacktestRun, records []storage.BacktestRunRecord) (int64, bool) {
	if b.store == nil {
		return 0, false
	}
	id, err := b.store.SaveBacktestRun(run, records)
	if err != nil {
		return 0, false
	}
	return id, true
}

func (b *Bot) BacktestRuns(limit int) []storage.BacktestRun {
	if b.store == nil {
		return nil
	}
	out, err := b.store.BacktestRuns(limit)
	if err != nil {
		return nil
	}
	return out
}

func (b *Bot) BacktestRunDetail(id int64) (storage.BacktestRun, []storage.BacktestRunRecord, bool) {
	if b.store == nil {
		return storage.BacktestRun{}, nil, false
	}
	run, records, err := b.store.BacktestRunDetail(id)
	if err != nil {
		return storage.BacktestRun{}, nil, false
	}
	return run, records, true
}

func (b *Bot) DeleteBacktestRun(id int64) bool {
	if b.store == nil {
		return false
	}
	return b.store.DeleteBacktestRun(id) == nil
}

func (b *Bot) openLong(currentPos *models.Position, amount float64) ([]models.OrderResult, error) {
	cfg := b.TradeConfig()
	var orders []models.OrderResult
	if currentPos != nil && currentPos.Side == "short" {
		fmt.Println("平空仓并开多仓...")
		closeOrder, err := b.exchange.PlaceMarketOrderWithResult(cfg.Symbol, "buy", currentPos.Size, true)
		if err != nil {
			return nil, fmt.Errorf("平空仓失败: %w", err)
		}
		orders = append(orders, closeOrder)
		time.Sleep(time.Second)
	} else if currentPos != nil && currentPos.Side == "long" {
		fmt.Println("已有多头持仓，保持现状")
		return nil, nil
	}
	fmt.Println("开多仓...")
	openOrder, err := b.exchange.PlaceMarketOrderWithResult(cfg.Symbol, "buy", amount, false)
	if err != nil {
		return nil, err
	}
	orders = append(orders, openOrder)
	return orders, nil
}

func (b *Bot) openShort(currentPos *models.Position, amount float64) ([]models.OrderResult, error) {
	cfg := b.TradeConfig()
	var orders []models.OrderResult
	if currentPos != nil && currentPos.Side == "long" {
		fmt.Println("平多仓并开空仓...")
		closeOrder, err := b.exchange.PlaceMarketOrderWithResult(cfg.Symbol, "sell", currentPos.Size, true)
		if err != nil {
			return nil, fmt.Errorf("平多仓失败: %w", err)
		}
		orders = append(orders, closeOrder)
		time.Sleep(time.Second)
	} else if currentPos != nil && currentPos.Side == "short" {
		fmt.Println("已有空头持仓，保持现状")
		return nil, nil
	}
	fmt.Println("开空仓...")
	openOrder, err := b.exchange.PlaceMarketOrderWithResult(cfg.Symbol, "sell", amount, false)
	if err != nil {
		return nil, err
	}
	orders = append(orders, openOrder)
	return orders, nil
}

func (b *Bot) UpdateTradeSettings(update TradeSettingsUpdate) (config.TradeConfig, error) {
	current := b.TradeConfig()
	next := current

	if update.Symbol != nil {
		sym := strings.ToUpper(strings.TrimSpace(*update.Symbol))
		if len(sym) < 6 || len(sym) > 16 {
			return current, fmt.Errorf("symbol 格式无效")
		}
		next.Symbol = sym
	}
	if update.HighConfidenceAmount != nil {
		if *update.HighConfidenceAmount <= 0 {
			return current, fmt.Errorf("high_confidence_amount 必须大于 0")
		}
		next.HighConfidenceAmount = *update.HighConfidenceAmount
	}
	if update.LowConfidenceAmount != nil {
		if *update.LowConfidenceAmount <= 0 {
			return current, fmt.Errorf("low_confidence_amount 必须大于 0")
		}
		next.LowConfidenceAmount = *update.LowConfidenceAmount
	}
	if update.PositionSizingMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*update.PositionSizingMode))
		if mode != "contracts" && mode != "margin_pct" {
			return current, fmt.Errorf("position_sizing_mode 仅支持 contracts 或 margin_pct")
		}
		next.PositionSizingMode = mode
	}
	if update.HighConfidenceMarginPct != nil {
		if *update.HighConfidenceMarginPct <= 0 || *update.HighConfidenceMarginPct > 1 {
			return current, fmt.Errorf("high_confidence_margin_pct 需在 (0,1] 之间")
		}
		next.HighConfidenceMarginPct = *update.HighConfidenceMarginPct
	}
	if update.LowConfidenceMarginPct != nil {
		if *update.LowConfidenceMarginPct <= 0 || *update.LowConfidenceMarginPct > 1 {
			return current, fmt.Errorf("low_confidence_margin_pct 需在 (0,1] 之间")
		}
		next.LowConfidenceMarginPct = *update.LowConfidenceMarginPct
	}
	if update.Leverage != nil {
		if *update.Leverage <= 0 || *update.Leverage > 150 {
			return current, fmt.Errorf("leverage 需在 1-150 之间")
		}
		next.Leverage = *update.Leverage
	}
	if update.MaxRiskPerTradePct != nil {
		if *update.MaxRiskPerTradePct <= 0 || *update.MaxRiskPerTradePct > 1 {
			return current, fmt.Errorf("max_risk_per_trade_pct 需在 (0,1] 之间")
		}
		next.MaxRiskPerTradePct = *update.MaxRiskPerTradePct
	}
	if update.MaxPositionPct != nil {
		if *update.MaxPositionPct <= 0 || *update.MaxPositionPct > 1 {
			return current, fmt.Errorf("max_position_pct 需在 (0,1] 之间")
		}
		next.MaxPositionPct = *update.MaxPositionPct
	}
	if update.MaxConsecutiveLosses != nil {
		if *update.MaxConsecutiveLosses < 0 {
			return current, fmt.Errorf("max_consecutive_losses 不能小于 0")
		}
		next.MaxConsecutiveLosses = *update.MaxConsecutiveLosses
	}
	if update.MaxDailyLossPct != nil {
		if *update.MaxDailyLossPct <= 0 || *update.MaxDailyLossPct > 1 {
			return current, fmt.Errorf("max_daily_loss_pct 需在 (0,1] 之间")
		}
		next.MaxDailyLossPct = *update.MaxDailyLossPct
	}
	if update.MaxDrawdownPct != nil {
		if *update.MaxDrawdownPct <= 0 || *update.MaxDrawdownPct > 1 {
			return current, fmt.Errorf("max_drawdown_pct 需在 (0,1] 之间")
		}
		next.MaxDrawdownPct = *update.MaxDrawdownPct
	}
	if update.LiquidationBufferPct != nil {
		if *update.LiquidationBufferPct <= 0 || *update.LiquidationBufferPct > 1 {
			return current, fmt.Errorf("liquidation_buffer_pct 需在 (0,1] 之间")
		}
		next.LiquidationBufferPct = *update.LiquidationBufferPct
	}

	if (update.Leverage != nil && next.Leverage != current.Leverage) || (update.Symbol != nil && next.Symbol != current.Symbol) {
		if err := b.exchange.SetLeverage(next.Symbol, next.Leverage); err != nil {
			return current, fmt.Errorf("设置杠杆失败: %w", err)
		}
	}

	b.mu.Lock()
	b.cfg.Symbol = next.Symbol
	b.cfg.HighConfidenceAmount = next.HighConfidenceAmount
	b.cfg.LowConfidenceAmount = next.LowConfidenceAmount
	b.cfg.PositionSizingMode = next.PositionSizingMode
	b.cfg.HighConfidenceMarginPct = next.HighConfidenceMarginPct
	b.cfg.LowConfidenceMarginPct = next.LowConfidenceMarginPct
	b.cfg.Leverage = next.Leverage
	b.cfg.MaxRiskPerTradePct = next.MaxRiskPerTradePct
	b.cfg.MaxPositionPct = next.MaxPositionPct
	b.cfg.MaxConsecutiveLosses = next.MaxConsecutiveLosses
	b.cfg.MaxDailyLossPct = next.MaxDailyLossPct
	b.cfg.MaxDrawdownPct = next.MaxDrawdownPct
	b.cfg.LiquidationBufferPct = next.LiquidationBufferPct
	b.mu.Unlock()
	return b.TradeConfig(), nil
}

func suggestedAmountByConfidence(confidence string, cfg config.TradeConfig, balance, price float64) float64 {
	mode := strings.ToLower(strings.TrimSpace(cfg.PositionSizingMode))
	if mode == "" {
		mode = "contracts"
	}
	isHigh := strings.ToUpper(strings.TrimSpace(confidence)) == "HIGH"
	if mode == "margin_pct" {
		pct := cfg.LowConfidenceMarginPct
		if isHigh {
			pct = cfg.HighConfidenceMarginPct
		}
		if pct <= 0 {
			return 0
		}
		if pct > 1 {
			pct = 1
		}
		if balance <= 0 || price <= 0 || cfg.Leverage <= 0 {
			return 0
		}
		margin := balance * pct
		return margin * float64(cfg.Leverage) / price
	}

	high := cfg.HighConfidenceAmount
	low := cfg.LowConfidenceAmount
	if high <= 0 {
		high = cfg.Amount
	}
	if low <= 0 {
		low = cfg.Amount
	}
	if isHigh {
		return high
	}
	return low
}

func (b *Bot) buildRiskPosition(signal models.TradeSignal, pd models.PriceData, pos *models.Position) (float64, bool, string) {
	cfg := b.TradeConfig()
	balance, err := b.exchange.FetchBalance()
	if err != nil {
		return 0, false, fmt.Sprintf("读取余额失败: %v", err)
	}
	suggested := suggestedAmountByConfidence(signal.Confidence, cfg, balance, pd.Price)
	snapshot := risk.Snapshot{
		Balance:       balance,
		CurrentEquity: balance,
		PeakEquity:    balance,
	}
	if b.store != nil {
		rs, rsErr := b.store.LoadRiskSnapshot()
		if rsErr == nil {
			snapshot.TodayPnL = rs.TodayPnL
			if rs.PeakEquity > 0 {
				snapshot.PeakEquity = rs.PeakEquity
			}
			if rs.CurrentEquity > 0 {
				snapshot.CurrentEquity = rs.CurrentEquity
			}
			snapshot.ConsecutiveLosses = rs.ConsecutiveLosses
		}
	}
	plan := b.riskEngine.BuildOrderPlan(risk.OrderPlanInput{
		Price:         pd.Price,
		StopLoss:      signal.StopLoss,
		Confidence:    signal.Confidence,
		SuggestedSize: suggested,
		Leverage:      cfg.Leverage,
	}, snapshot)
	if !plan.Approved {
		return 0, false, plan.Reason
	}
	// 交易所精度保护
	size := math.Round(plan.Size*10000) / 10000
	if size <= 0 {
		return 0, false, "风控后仓位过小"
	}
	return size, true, ""
}

func (b *Bot) confirmOrder(order models.OrderResult) error {
	if order.OrderID == "" {
		return nil
	}
	for i := 0; i < 6; i++ {
		status, err := b.exchange.FetchOrder(order.Symbol, order.OrderID)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if status == nil {
			time.Sleep(time.Second)
			continue
		}
		_ = b.saveOrderStatus(order.OrderID, status.State, status)
		if status.FilledSize > 0 {
			fillID := fmt.Sprintf("%s-%s-%.4f", status.OrderID, status.UpdateTime, status.FilledSize)
			_ = b.saveFill(fillID, status)
		}
		if status.State == "filled" || status.State == "canceled" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("订单%s状态确认超时", order.OrderID)
}

func (b *Bot) reconcileOpenOrders() error {
	if b.store == nil {
		return nil
	}
	cfg := b.TradeConfig()
	ids, err := b.store.OpenOrders()
	if err != nil {
		return err
	}
	for _, id := range ids {
		st, e := b.exchange.FetchOrder(cfg.Symbol, id)
		if e != nil || st == nil {
			continue
		}
		_ = b.saveOrderStatus(id, st.State, st)
	}
	return nil
}

func (b *Bot) saveAIDecision(sig models.TradeSignal, pd models.PriceData, approvedSize float64, approved bool, riskReason string) {
	if b.store == nil {
		return
	}
	cfg := b.TradeConfig()
	balance, _ := b.exchange.FetchBalance()
	suggested := suggestedAmountByConfidence(sig.Confidence, cfg, balance, pd.Price)
	_ = b.store.SaveAIDecision(time.Now(), map[string]any{
		"signal":         sig.Signal,
		"confidence":     sig.Confidence,
		"reason":         sig.Reason,
		"price":          pd.Price,
		"stop_loss":      sig.StopLoss,
		"take_profit":    sig.TakeProfit,
		"strategy_combo": sig.StrategyCombo,
		"strategy_score": sig.StrategyScore,
		"suggested_size": suggested,
		"approved_size":  approvedSize,
		"approved":       approved,
		"risk_reason":    riskReason,
	})
}

func (b *Bot) saveOrder(order models.OrderResult) error {
	if b.store == nil {
		return nil
	}
	return b.store.SaveOrder(order.OrderID, order.Symbol, order.Side, order.Size, order.ReduceOnly, order.State, order)
}

func (b *Bot) saveOrderStatus(orderID, status string, payload any) error {
	if b.store == nil {
		return nil
	}
	return b.store.SaveOrder(orderID, "", "", 0, false, status, payload)
}

func (b *Bot) saveFill(fillID string, status *models.OrderStatus) error {
	if b.store == nil || status == nil {
		return nil
	}
	return b.store.SaveFill(fillID, status.OrderID, status.Symbol, status.Side, status.FilledSize, status.AvgPrice, status.UpdateTime)
}

func (b *Bot) savePosition(pos models.Position) error {
	if b.store == nil {
		return nil
	}
	return b.store.SavePositionSnapshot(pos.Symbol, pos.Side, pos.Size, pos.EntryPrice, pos.UnrealizedPnL, pos.Leverage)
}

func (b *Bot) saveEquity(balance float64, pos *models.Position) error {
	if b.store == nil {
		return nil
	}
	upl := 0.0
	if pos != nil {
		upl = pos.UnrealizedPnL
	}
	return b.store.SaveEquity(balance, upl)
}

func (b *Bot) saveRiskEvent(eventType, details string) error {
	if b.store == nil {
		return nil
	}
	return b.store.SaveRiskEvent(eventType, details)
}

// WaitForNextPeriod 等待到下一个 15 分钟整点，返回需等待秒数
func WaitForNextPeriod() int {
	now := time.Now()
	nextMinute := ((now.Minute() / 15) + 1) * 15
	var next time.Time
	if nextMinute >= 60 {
		next = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
	} else {
		next = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), nextMinute, 0, 0, now.Location())
	}
	secs := int(time.Until(next).Seconds())
	if secs < 0 {
		secs = 0
	}
	return secs
}

func line() string {
	return "============================================================"
}
