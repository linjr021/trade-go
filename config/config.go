package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type TradeConfig struct {
	Symbol                           string
	Amount                           float64
	HighConfidenceAmount             float64
	LowConfidenceAmount              float64
	PositionSizingMode               string
	HighConfidenceMarginPct          float64
	LowConfidenceMarginPct           float64
	Leverage                         int
	Timeframe                        string
	TestMode                         bool
	DataPoints                       int
	MaxRiskPerTradePct               float64
	MaxPositionPct                   float64
	MaxConsecutiveLosses             int
	MaxDailyLossPct                  float64
	MaxDrawdownPct                   float64
	LiquidationBufferPct             float64
	AutoReviewEnabled                bool
	AutoReviewAfterOrderOnly         bool
	AutoReviewIntervalSec            int
	AutoReviewVolatilityPct          float64
	AutoReviewDrawdownWarnPct        float64
	AutoReviewLossStreakWarn         int
	AutoReviewRiskReduceFactor       float64
	AutoStrategyRegenEnabled         bool
	AutoStrategyRegenCooldownSec     int
	AutoStrategyRegenLossStreak      int
	AutoStrategyRegenDrawdownWarnPct float64
	AutoStrategyRegenMinRR           float64

	// Analysis periods
	ShortTermPeriod  int
	MediumTermPeriod int
	LongTermPeriod   int
}

type AppConfig struct {
	AIAPIKey       string
	AIBaseURL      string
	AIModel        string
	PyStrategyURL  string
	ActiveExchange string
	BinanceAPIKey  string
	BinanceSecret  string
	OKXAPIKey      string
	OKXSecret      string
	OKXPassword    string
	Trade          TradeConfig
}

var Config *AppConfig

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用系统环境变量")
	}

	Config = &AppConfig{
		AIAPIKey:       getEnv("AI_API_KEY", ""),
		AIBaseURL:      getEnv("AI_BASE_URL", ""),
		AIModel:        getEnv("AI_MODEL", ""),
		PyStrategyURL:  getEnv("PY_STRATEGY_URL", ""),
		ActiveExchange: getEnv("ACTIVE_EXCHANGE", "binance"),
		BinanceAPIKey:  getEnv("BINANCE_API_KEY", ""),
		BinanceSecret:  getEnv("BINANCE_SECRET", ""),
		OKXAPIKey:      getEnv("OKX_API_KEY", ""),
		OKXSecret:      getEnv("OKX_SECRET", ""),
		OKXPassword:    getEnv("OKX_PASSWORD", ""),
		Trade: TradeConfig{
			Symbol:                           getEnv("TRADE_SYMBOL", "BTCUSDT"),
			Amount:                           getEnvFloat("TRADE_AMOUNT", 0.01),
			HighConfidenceAmount:             getEnvFloat("HIGH_CONFIDENCE_AMOUNT", 0.01),
			LowConfidenceAmount:              getEnvFloat("LOW_CONFIDENCE_AMOUNT", 0.005),
			PositionSizingMode:               getEnv("POSITION_SIZING_MODE", "margin_pct"),
			HighConfidenceMarginPct:          getEnvFloat("HIGH_CONFIDENCE_MARGIN_PCT", 0.05),
			LowConfidenceMarginPct:           getEnvFloat("LOW_CONFIDENCE_MARGIN_PCT", 0.00),
			Leverage:                         getEnvInt("LEVERAGE", 20),
			Timeframe:                        getEnv("TIMEFRAME", "15m"),
			TestMode:                         getEnvBool("TEST_MODE", false),
			DataPoints:                       getEnvInt("DATA_POINTS", 96),
			MaxRiskPerTradePct:               getEnvFloat("MAX_RISK_PER_TRADE_PCT", 0.01),
			MaxPositionPct:                   getEnvFloat("MAX_POSITION_PCT", 0.20),
			MaxConsecutiveLosses:             getEnvInt("MAX_CONSECUTIVE_LOSSES", 3),
			MaxDailyLossPct:                  getEnvFloat("MAX_DAILY_LOSS_PCT", 0.05),
			MaxDrawdownPct:                   getEnvFloat("MAX_DRAWDOWN_PCT", 0.12),
			LiquidationBufferPct:             getEnvFloat("LIQUIDATION_BUFFER_PCT", 0.02),
			AutoReviewEnabled:                getEnvBool("AUTO_REVIEW_ENABLED", true),
			AutoReviewAfterOrderOnly:         getEnvBool("AUTO_REVIEW_AFTER_ORDER_ONLY", true),
			AutoReviewIntervalSec:            getEnvInt("AUTO_REVIEW_INTERVAL_SEC", 1800),
			AutoReviewVolatilityPct:          getEnvFloat("AUTO_REVIEW_VOLATILITY_PCT", 1.2),
			AutoReviewDrawdownWarnPct:        getEnvFloat("AUTO_REVIEW_DRAWDOWN_WARN_PCT", 0.05),
			AutoReviewLossStreakWarn:         getEnvInt("AUTO_REVIEW_LOSS_STREAK_WARN", 2),
			AutoReviewRiskReduceFactor:       getEnvFloat("AUTO_REVIEW_RISK_REDUCE_FACTOR", 0.7),
			AutoStrategyRegenEnabled:         getEnvBool("AUTO_STRATEGY_REGEN_ENABLED", true),
			AutoStrategyRegenCooldownSec:     getEnvInt("AUTO_STRATEGY_REGEN_COOLDOWN_SEC", 21600),
			AutoStrategyRegenLossStreak:      getEnvInt("AUTO_STRATEGY_REGEN_LOSS_STREAK", 3),
			AutoStrategyRegenDrawdownWarnPct: getEnvFloat("AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT", 0.08),
			AutoStrategyRegenMinRR:           getEnvFloat("AUTO_STRATEGY_REGEN_MIN_RR", 2.0),
			ShortTermPeriod:                  getEnvInt("SHORT_TERM_PERIOD", 20),
			MediumTermPeriod:                 getEnvInt("MEDIUM_TERM_PERIOD", 50),
			LongTermPeriod:                   getEnvInt("LONG_TERM_PERIOD", 96),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func getEnvFloat(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
