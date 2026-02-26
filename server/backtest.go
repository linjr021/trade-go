package server

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"trade-go/storage"
)

type backtestRequest struct {
	StrategyName       string  `json:"strategy_name"`
	Pair               string  `json:"pair"`
	Habit              string  `json:"habit"`
	StartMonth         string  `json:"start_month"`
	EndMonth           string  `json:"end_month"`
	InitialMargin      float64 `json:"initial_margin"`
	Leverage           int     `json:"leverage"`
	PositionSizingMode string  `json:"position_sizing_mode"`
	HighConfAmt        float64 `json:"high_confidence_amount"`
	LowConfAmt         float64 `json:"low_confidence_amount"`
	HighConfMarginPct  float64 `json:"high_confidence_margin_pct"`
	LowConfMarginPct   float64 `json:"low_confidence_margin_pct"`
	PaperMargin        float64 `json:"paper_margin"`
}

type klineItem struct {
	TS    int64
	Open  float64
	High  float64
	Low   float64
	Close float64
}

type backtestRecord struct {
	ID         string  `json:"id"`
	TS         int64   `json:"ts"`
	Side       string  `json:"side"`
	Confidence string  `json:"confidence"`
	Size       float64 `json:"size"`
	Leverage   int     `json:"leverage"`
	Entry      float64 `json:"entry"`
	Exit       float64 `json:"exit"`
	PnL        float64 `json:"pnl"`
}

func (s *Service) handleBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req backtestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	pair := strings.ToUpper(strings.TrimSpace(req.Pair))
	if pair == "" {
		pair = "BTCUSDT"
	}
	interval := intervalByHabit(req.Habit)
	startMs, err := monthToMs(req.StartMonth, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start_month")
		return
	}
	endMs, err := monthToMs(req.EndMonth, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end_month")
		return
	}
	if endMs <= startMs {
		writeError(w, http.StatusBadRequest, "end_month must be later than start_month")
		return
	}

	margin := req.PaperMargin
	if margin <= 0 {
		margin = 100
	}
	if margin > 1_000_000 {
		margin = 1_000_000
	}
	initialMargin := req.InitialMargin
	if initialMargin <= 0 {
		initialMargin = 1000
	}
	if initialMargin > 1_000_000_000 {
		initialMargin = 1_000_000_000
	}
	leverage := req.Leverage
	if leverage <= 0 {
		leverage = 10
	}
	if leverage > 150 {
		leverage = 150
	}
	sizingMode := strings.ToLower(strings.TrimSpace(req.PositionSizingMode))
	if sizingMode != "contracts" && sizingMode != "margin_pct" {
		sizingMode = "contracts"
	}

	highAmt := req.HighConfAmt
	if highAmt < 0 {
		highAmt = 0
	}
	lowAmt := req.LowConfAmt
	if lowAmt < 0 {
		lowAmt = 0
	}
	highPct := req.HighConfMarginPct
	if highPct < 0 {
		highPct = 0
	}
	if highPct > 1 {
		highPct = 1
	}
	lowPct := req.LowConfMarginPct
	if lowPct < 0 {
		lowPct = 0
	}
	if lowPct > 1 {
		lowPct = 1
	}

	klines, err := fetchBinanceKlinesRange(pair, interval, startMs, endMs)
	if err != nil {
		writeError(w, http.StatusBadGateway, "fetch kline failed: "+err.Error())
		return
	}
	if len(klines) < 8 {
		writeError(w, http.StatusBadRequest, "kline data not enough for backtest")
		return
	}

	records := make([]backtestRecord, 0, len(klines)/2)
	totalPnL := 0.0
	wins := 0
	losses := 0
	equity := initialMargin
	for i := 6; i < len(klines)-1; i += 2 {
		cur := klines[i]
		nxt := klines[i+1]
		prev := klines[i-1]
		side := "BUY"
		if cur.Close < prev.Close {
			side = "SELL"
		}

		movePct := math.Abs((cur.Close - prev.Close) / prev.Close * 100)
		confidence := "LOW"
		size := lowAmt
		if movePct >= 0.35 {
			confidence = "HIGH"
			size = highAmt
		}
		if sizingMode == "margin_pct" {
			pct := lowPct
			if confidence == "HIGH" {
				pct = highPct
			}
			if pct > 1 {
				pct = 1
			}
			if pct < 0 {
				pct = 0
			}
			if pct > 0 && cur.Close > 0 && leverage > 0 {
				size = (equity * pct * float64(leverage)) / cur.Close
			} else {
				size = 0
			}
			if highPct == 0 && lowPct == 0 && cur.Close > 0 {
				// Backward-compatible fallback: keep historical behavior based on paper_margin.
				size = margin / cur.Close
			}
		} else if highAmt == 0 && lowAmt == 0 && cur.Close > 0 {
			// Backward-compatible fallback: keep historical behavior based on paper_margin.
			size = margin / cur.Close
		}
		if size < 0 {
			size = 0
		}

		maxSize := 0.0
		if cur.Close > 0 && leverage > 0 && equity > 0 {
			maxSize = (equity * float64(leverage)) / cur.Close
		}
		if size > maxSize {
			size = maxSize
		}

		pnl := 0.0
		if size > 0 {
			if side == "BUY" {
				pnl = (nxt.Close - cur.Close) * size
			} else {
				pnl = (cur.Close - nxt.Close) * size
			}
		}
		totalPnL += pnl
		equity += pnl
		if pnl > 0 {
			wins++
		} else if pnl < 0 {
			losses++
		}
		records = append(records, backtestRecord{
			ID:         fmt.Sprintf("%d-%d", cur.TS, i),
			TS:         cur.TS,
			Side:       side,
			Confidence: confidence,
			Size:       round(size, 8),
			Leverage:   leverage,
			Entry:      round(cur.Close, 6),
			Exit:       round(nxt.Close, 6),
			PnL:        round(pnl, 6),
		})
	}

	ratio := 0.0
	ratioInfinite := false
	if losses == 0 {
		if wins > 0 {
			ratioInfinite = true
		}
	} else {
		ratio = float64(wins) / float64(losses)
	}
	finalEquity := initialMargin + totalPnL
	returnPct := 0.0
	if initialMargin > 0 {
		returnPct = (totalPnL / initialMargin) * 100
	}

	if len(records) > 500 {
		records = records[len(records)-500:]
	}

	createdAt := time.Now().Format(time.RFC3339)
	run := storage.BacktestRun{
		CreatedAt:               createdAt,
		Strategy:                strings.TrimSpace(req.StrategyName),
		Pair:                    pair,
		Habit:                   req.Habit,
		Start:                   req.StartMonth,
		End:                     req.EndMonth,
		Bars:                    len(klines),
		InitialMargin:           round(initialMargin, 6),
		Leverage:                leverage,
		PositionSizingMode:      sizingMode,
		HighConfidenceAmount:    round(highAmt, 8),
		LowConfidenceAmount:     round(lowAmt, 8),
		HighConfidenceMarginPct: round(highPct, 6),
		LowConfidenceMarginPct:  round(lowPct, 6),
		TotalPnL:                round(totalPnL, 6),
		FinalEquity:             round(finalEquity, 6),
		ReturnPct:               round(returnPct, 6),
		Wins:                    wins,
		Losses:                  losses,
		Ratio:                   round(ratio, 6),
	}
	saveRecords := make([]storage.BacktestRunRecord, 0, len(records))
	for _, r := range records {
		saveRecords = append(saveRecords, storage.BacktestRunRecord{
			ID:         r.ID,
			TS:         r.TS,
			Side:       r.Side,
			Confidence: r.Confidence,
			Size:       r.Size,
			Leverage:   r.Leverage,
			Entry:      r.Entry,
			Exit:       r.Exit,
			PnL:        r.PnL,
		})
	}
	historyID, saved := s.bot.SaveBacktestRun(run, saveRecords)

	summary := map[string]any{
		"history_id":                 historyID,
		"id":                         historyID,
		"created_at":                 createdAt,
		"strategy":                   run.Strategy,
		"pair":                       run.Pair,
		"habit":                      run.Habit,
		"start":                      run.Start,
		"end":                        run.End,
		"bars":                       run.Bars,
		"initial_margin":             run.InitialMargin,
		"leverage":                   run.Leverage,
		"position_sizing_mode":       run.PositionSizingMode,
		"high_confidence_amount":     run.HighConfidenceAmount,
		"low_confidence_amount":      run.LowConfidenceAmount,
		"high_confidence_margin_pct": run.HighConfidenceMarginPct,
		"low_confidence_margin_pct":  run.LowConfidenceMarginPct,
		"total_pnl":                  run.TotalPnL,
		"final_equity":               run.FinalEquity,
		"return_pct":                 run.ReturnPct,
		"wins":                       run.Wins,
		"losses":                     run.Losses,
		"ratio":                      run.Ratio,
		"ratio_infinite":             ratioInfinite,
	}
	resp := map[string]any{
		"summary": summary,
		"records": records,
	}
	if !saved {
		resp["history_warning"] = "回测已完成，但回测记录未写入SQLite（请检查数据库配置）"
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) handleBacktestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	runs := s.bot.BacktestRuns(limit)
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Service) handleBacktestHistoryDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	run, records, ok := s.bot.BacktestRunDetail(id)
	if !ok {
		writeError(w, http.StatusNotFound, "backtest history not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": run,
		"records": records,
	})
}

func (s *Service) handleBacktestHistoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.ID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !s.bot.DeleteBacktestRun(req.ID) {
		writeError(w, http.StatusInternalServerError, "delete backtest history failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "backtest history deleted", "id": req.ID})
}

func monthToMs(v string, endOfMonth bool) (int64, error) {
	if !strings.Contains(v, "-") || len(v) != 7 {
		return 0, fmt.Errorf("invalid format")
	}
	t, err := time.Parse("2006-01", v)
	if err != nil {
		return 0, err
	}
	if endOfMonth {
		next := t.AddDate(0, 1, 0)
		return next.Add(-time.Millisecond).UnixMilli(), nil
	}
	return t.UnixMilli(), nil
}

func intervalByHabit(habit string) string {
	switch strings.TrimSpace(habit) {
	case "10m":
		return "15m"
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "1D", "5D", "30D", "90D":
		return "1d"
	default:
		return "1h"
	}
}

func fetchBinanceKlinesRange(symbol, interval string, startMs, endMs int64) ([]klineItem, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	cursor := startMs
	out := make([]klineItem, 0, 1200)

	for cursor < endMs {
		u := url.URL{Scheme: "https", Host: "api.binance.com", Path: "/api/v3/klines"}
		q := u.Query()
		q.Set("symbol", symbol)
		q.Set("interval", interval)
		q.Set("startTime", strconv.FormatInt(cursor, 10))
		q.Set("endTime", strconv.FormatInt(endMs, 10))
		q.Set("limit", "1000")
		u.RawQuery = q.Encode()

		resp, err := client.Get(u.String())
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("binance http %d", resp.StatusCode)
		}

		var arr [][]interface{}
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, err
		}
		if len(arr) == 0 {
			break
		}

		lastTs := int64(0)
		for _, row := range arr {
			if len(row) < 6 {
				continue
			}
			ts, ok := asInt64(row[0])
			if !ok {
				continue
			}
			open, ok1 := asFloat(row[1])
			high, ok2 := asFloat(row[2])
			low, ok3 := asFloat(row[3])
			closePrice, ok4 := asFloat(row[4])
			if !(ok1 && ok2 && ok3 && ok4) {
				continue
			}
			if high <= 0 || low <= 0 || closePrice <= 0 {
				continue
			}
			out = append(out, klineItem{TS: ts, Open: open, High: high, Low: low, Close: closePrice})
			if ts > lastTs {
				lastTs = ts
			}
		}
		if lastTs == 0 {
			break
		}
		next := lastTs + 1
		if next <= cursor {
			break
		}
		cursor = next
		if len(arr) < 1000 {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	return out, nil
}

func asInt64(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func asFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		n, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func round(v float64, digits int) float64 {
	if digits < 0 {
		return v
	}
	pow := math.Pow(10, float64(digits))
	return math.Round(v*pow) / pow
}
