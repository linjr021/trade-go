package server

import (
	"net/http"
	"strconv"
	"strings"
	"trade-go/exchange"
)

func (s *Service) handleMarketSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.bot.TradeConfig()
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	}
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	timeframe := strings.TrimSpace(r.URL.Query().Get("timeframe"))
	if timeframe == "" {
		timeframe = strings.TrimSpace(cfg.Timeframe)
	}
	if timeframe == "" {
		timeframe = "1h"
	}
	limit := 2
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 2 && n <= 500 {
			limit = n
		}
	}

	client := exchange.NewClient()
	candles, err := client.FetchOHLCV(symbol, timeframe, limit)
	if err != nil || len(candles) == 0 {
		if err != nil {
			writeError(w, http.StatusBadGateway, "获取行情快照失败: "+err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "获取行情快照失败: K线为空")
		return
	}
	last := candles[len(candles)-1]
	prev := last
	if len(candles) >= 2 {
		prev = candles[len(candles)-2]
	}
	changePct := 0.0
	if prev.Close > 0 {
		changePct = (last.Close - prev.Close) / prev.Close * 100
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"symbol":          symbol,
		"timeframe":       timeframe,
		"active_exchange": client.ActiveExchange(),
		"price":           last.Close,
		"open":            last.Open,
		"high":            last.High,
		"low":             last.Low,
		"volume":          last.Volume,
		"timestamp":       last.Timestamp,
		"change_pct":      changePct,
	})
}
