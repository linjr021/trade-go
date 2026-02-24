package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/market"
	"trade-go/server"
	"trade-go/storage"
	"trade-go/trader"
)

const (
	ModeCLI = "cli"
	ModeWeb = "web"
)

type Runner struct {
	bot    *trader.Bot
	store  *storage.Store
	stream *market.Stream
}

func NewRunner() *Runner {
	return &Runner{bot: trader.NewBot()}
}

func (r *Runner) Setup() error {
	store, err := storage.Open(os.Getenv("TRADE_DB_PATH"))
	if err == nil {
		r.store = store
		r.bot.SetStore(store)
	} else {
		fmt.Printf("SQLite初始化失败，继续无持久化模式: %v\n", err)
	}
	return r.bot.Setup()
}

func (r *Runner) Run(mode string) error {
	if r.store != nil {
		defer func() { _ = r.store.Close() }()
	}
	switch normalizeMode(mode) {
	case ModeWeb:
		return r.runWeb()
	case ModeCLI:
		return r.runCLI()
	default:
		return fmt.Errorf("unsupported mode: %s (supported: %s,%s)", mode, ModeCLI, ModeWeb)
	}
}

func (r *Runner) runCLI() error {
	cfg := &config.Config.Trade
	fmt.Println("BTC/USDT Binance 自动交易机器人启动！（Go 版本）")
	fmt.Printf("运行模式: %s | 交易周期: %s | 杠杆: %dx | 交易量: %.4f BTC\n", ModeCLI, cfg.Timeframe, cfg.Leverage, cfg.Amount)

	if cfg.TestMode {
		fmt.Println("⚠️ 当前为测试模式，不会真实下单")
	} else {
		fmt.Println("🔴 实盘交易模式，请谨慎操作！")
	}
	fmt.Println("执行频率: 每 15 分钟整点执行")

	for {
		secs := trader.WaitForNextPeriod()
		if secs > 0 {
			mins := secs / 60
			s := secs % 60
			if mins > 0 {
				fmt.Printf("🕒 等待 %d 分 %d 秒到整点...\n", mins, s)
			} else {
				fmt.Printf("🕒 等待 %d 秒到整点...\n", s)
			}
			time.Sleep(time.Duration(secs) * time.Second)
		}

		r.bot.Run()
		time.Sleep(60 * time.Second)
	}
}

func (r *Runner) runWeb() error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	cfg := r.bot.TradeConfig()
	wsEnabled := os.Getenv("ENABLE_WS_MARKET") == "true"
	if wsEnabled {
		r.stream = market.NewStream(cfg.Symbol, cfg.Timeframe)
		if err := r.stream.Start(); err != nil {
			fmt.Printf("行情WebSocket启动失败，回退REST: %v\n", err)
			wsEnabled = false
		} else {
			fmt.Printf("Binance 行情WebSocket已启动: %s %s\n", cfg.Symbol, cfg.Timeframe)
			defer func() { _ = r.stream.Stop() }()
		}
	}
	svc := server.NewService(r.bot)
	if wsEnabled {
		go r.runRealtimeStrategyLoop(svc)
	} else {
		svc.StartScheduler()
	}
	fmt.Printf("运行模式: %s\n", ModeWeb)
	return server.Serve(addr, svc)
}

func (r *Runner) runRealtimeStrategyLoop(svc *server.Service) {
	minIntervalSec := 10
	if raw := strings.TrimSpace(os.Getenv("REALTIME_MIN_INTERVAL_SEC")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			minIntervalSec = n
		}
	}
	fmt.Printf("策略触发模式: WebSocket事件驱动（最小执行间隔 %d 秒）\n", minIntervalSec)
	var lastCloseTime int64
	var lastSeen time.Time
	lastRun := time.Time{}
	for {
		snap := r.stream.Snapshot()
		if snap.KlineClosed && snap.KlineCloseTime > 0 && snap.KlineCloseTime != lastCloseTime {
			lastCloseTime = snap.KlineCloseTime
			svc.RunOnce()
			lastRun = time.Now()
			lastSeen = snap.UpdatedAt
		} else if !snap.UpdatedAt.IsZero() && snap.UpdatedAt.After(lastSeen) &&
			(lastRun.IsZero() || time.Since(lastRun) >= time.Duration(minIntervalSec)*time.Second) {
			svc.RunOnce()
			lastRun = time.Now()
			lastSeen = snap.UpdatedAt
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func normalizeMode(mode string) string {
	v := strings.TrimSpace(strings.ToLower(mode))
	if v == "" {
		return ModeCLI
	}
	return v
}
