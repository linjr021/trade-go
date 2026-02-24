package market

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type StreamSnapshot struct {
	TickerPrice    string            `json:"ticker_price"`
	Ticker         map[string]string `json:"ticker"`
	Kline          []string          `json:"kline"`
	KlineClosed    bool              `json:"kline_closed"`
	KlineCloseTime int64             `json:"kline_close_time"`
	FundingRate    string            `json:"funding_rate"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type Stream struct {
	symbol string
	candle string

	mu   sync.RWMutex
	conn *websocket.Conn
	last StreamSnapshot
}

func NewStream(symbol, candle string) *Stream {
	if candle == "" {
		candle = "15m"
	}
	return &Stream{symbol: normalizeSymbol(symbol), candle: candle}
}

func normalizeSymbol(symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "swap", "")
	return s
}

func (s *Stream) Start() error {
	url := fmt.Sprintf(
		"wss://fstream.binance.com/stream?streams=%s@ticker/%s@kline_%s/%s@markPrice",
		s.symbol, s.symbol, s.candle, s.symbol,
	)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	s.conn = c
	go s.readLoop()
	return nil
}

func (s *Stream) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

func (s *Stream) Snapshot() StreamSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}

func (s *Stream) readLoop() {
	for {
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		var payload struct {
			Stream string          `json:"stream"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &payload); err != nil {
			continue
		}

		s.mu.Lock()
		s.last.KlineClosed = false
		s.last.UpdatedAt = time.Now()

		switch {
		case strings.Contains(payload.Stream, "@ticker"):
			var t struct {
				LastPrice string `json:"c"`
			}
			if json.Unmarshal(payload.Data, &t) == nil {
				s.last.TickerPrice = t.LastPrice
				if s.last.Ticker == nil {
					s.last.Ticker = map[string]string{}
				}
				s.last.Ticker["last_price"] = t.LastPrice
			}
		case strings.Contains(payload.Stream, "@markPrice"):
			var m struct {
				FundingRate string `json:"r"`
			}
			if json.Unmarshal(payload.Data, &m) == nil {
				s.last.FundingRate = m.FundingRate
			}
		case strings.Contains(payload.Stream, "@kline_"):
			var k struct {
				K struct {
					OpenTime  int64  `json:"t"`
					CloseTime int64  `json:"T"`
					Open      string `json:"o"`
					High      string `json:"h"`
					Low       string `json:"l"`
					Close     string `json:"c"`
					Volume    string `json:"v"`
					Closed    bool   `json:"x"`
				} `json:"k"`
			}
			if json.Unmarshal(payload.Data, &k) == nil {
				s.last.Kline = []string{
					fmt.Sprintf("%d", k.K.OpenTime),
					k.K.Open,
					k.K.High,
					k.K.Low,
					k.K.Close,
					k.K.Volume,
				}
				s.last.KlineClosed = k.K.Closed
				s.last.KlineCloseTime = k.K.CloseTime
			}
		}
		s.mu.Unlock()
	}
}
