package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type TradeConfig struct {
	Symbol                string
	Amount                float64
	HighConfidenceAmount  float64
	LowConfidenceAmount   float64
	Leverage              int
	Timeframe             string
	TestMode              bool
	DataPoints            int
	MaxRiskPerTradePct    float64
	MaxPositionPct        float64
	MaxConsecutiveLosses  int
	MaxDailyLossPct       float64
	MaxDrawdownPct        float64
	LiquidationBufferPct  float64

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
		BinanceAPIKey:  getEnv("BINANCE_API_KEY", ""),
		BinanceSecret:  getEnv("BINANCE_SECRET", ""),
		OKXAPIKey:      getEnv("OKX_API_KEY", ""),
		OKXSecret:      getEnv("OKX_SECRET", ""),
		OKXPassword:    getEnv("OKX_PASSWORD", ""),
		Trade: TradeConfig{
			Symbol:               "BTCUSDT",
			Amount:               0.01,
			HighConfidenceAmount: 0.01,
			LowConfidenceAmount:  0.005,
			Leverage:             10,
			Timeframe:            "15m",
			TestMode:             getEnvBool("TEST_MODE", false),
			DataPoints:           96,
			MaxRiskPerTradePct:   0.01,
			MaxPositionPct:       0.20,
			MaxConsecutiveLosses: 3,
			MaxDailyLossPct:      0.05,
			MaxDrawdownPct:       0.12,
			LiquidationBufferPct: 0.02,
			ShortTermPeriod:      20,
			MediumTermPeriod:     50,
			LongTermPeriod:       96,
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
