package models

import "time"

// OHLCV K线数据
type OHLCV struct {
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

// TechnicalIndicators 技术指标
type TechnicalIndicators struct {
	SMA5      float64
	SMA20     float64
	SMA50     float64
	EMA12     float64
	EMA26     float64
	MACD      float64
	MACDSignal float64
	MACDHist  float64
	RSI       float64
	BBUpper   float64
	BBMiddle  float64
	BBLower   float64
	BBPosition float64
	VolumeMA  float64
	VolumeRatio float64
	Resistance float64
	Support    float64
}

// TrendAnalysis 趋势分析
type TrendAnalysis struct {
	ShortTerm  string // 上涨/下跌
	MediumTerm string
	MACD       string // bullish/bearish
	Overall    string // 强势上涨/强势下跌/震荡整理
	RSILevel   float64
}

// LevelsAnalysis 支撑阻力位分析
type LevelsAnalysis struct {
	StaticResistance   float64
	StaticSupport      float64
	DynamicResistance  float64
	DynamicSupport     float64
	PriceVsResistance  float64
	PriceVsSupport     float64
}

// PriceData 完整行情数据
type PriceData struct {
	Price       float64
	Timestamp   time.Time
	High        float64
	Low         float64
	Volume      float64
	Timeframe   string
	PriceChange float64
	KlineData   []OHLCV
	Technical   TechnicalIndicators
	Trend       TrendAnalysis
	Levels      LevelsAnalysis
}

// TradeSignal AI 返回的交易信号
type TradeSignal struct {
	Signal     string  `json:"signal"`      // BUY/SELL/HOLD
	Reason     string  `json:"reason"`
	StopLoss   float64 `json:"stop_loss"`
	TakeProfit float64 `json:"take_profit"`
	Confidence string  `json:"confidence"`  // HIGH/MEDIUM/LOW
	Timestamp  time.Time
	IsFallback bool
}

// Position 持仓信息
type Position struct {
	Side          string  // long/short
	Size          float64
	EntryPrice    float64
	UnrealizedPnL float64
	Leverage      float64
	Symbol        string
}

type OrderResult struct {
	OrderID    string `json:"order_id"`
	ClientID   string `json:"client_id"`
	State      string `json:"state"`
	Symbol     string `json:"symbol"`
	Side       string `json:"side"`
	Size       float64 `json:"size"`
	ReduceOnly bool   `json:"reduce_only"`
}

type OrderStatus struct {
	OrderID     string  `json:"order_id"`
	State       string  `json:"state"`
	FilledSize  float64 `json:"filled_size"`
	AvgPrice    float64 `json:"avg_price"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	ReduceOnly  bool    `json:"reduce_only"`
	UpdateTime  string  `json:"update_time"`
}
