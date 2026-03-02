package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/models"
)

const binanceBaseURL = "https://fapi.binance.com"

type binanceClient struct {
	apiKey     string
	secret     string
	httpClient *http.Client
}

func newBinanceClient(cfg *config.AppConfig) *binanceClient {
	key := ""
	secret := ""
	if cfg != nil {
		key = strings.TrimSpace(cfg.BinanceAPIKey)
		secret = strings.TrimSpace(cfg.BinanceSecret)
	}
	return &binanceClient{
		apiKey:     key,
		secret:     secret,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *binanceClient) signQuery(raw string) string {
	h := hmac.New(sha256.New, []byte(c.secret))
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *binanceClient) requestPublic(path string, values url.Values) ([]byte, error) {
	fullURL := binanceBaseURL + path
	if values != nil && len(values) > 0 {
		fullURL += "?" + values.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *binanceClient) requestSigned(method, path string, values url.Values) ([]byte, error) {
	if values == nil {
		values = url.Values{}
	}
	values.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	values.Set("recvWindow", "5000")
	raw := values.Encode()
	sig := c.signQuery(raw)
	fullURL := binanceBaseURL + path + "?" + raw + "&signature=" + sig

	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *binanceClient) FetchOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	barMap := map[string]string{
		"1m": "1m", "5m": "5m", "15m": "15m",
		"30m": "30m", "1h": "1h", "4h": "4h", "1d": "1d",
	}
	interval, ok := barMap[timeframe]
	if !ok {
		interval = "15m"
	}
	vals := url.Values{}
	vals.Set("symbol", normalizeSymbol(symbol))
	vals.Set("interval", interval)
	vals.Set("limit", strconv.Itoa(limit))
	data, err := c.requestPublic("/fapi/v1/klines", vals)
	if err != nil {
		return nil, err
	}

	var rows [][]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	candles := make([]models.OHLCV, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		tsMs := int64(toFloat(row[0]))
		o := toFloat(row[1])
		h := toFloat(row[2])
		l := toFloat(row[3])
		cl := toFloat(row[4])
		v := toFloat(row[5])
		candles = append(candles, models.OHLCV{
			Timestamp: time.UnixMilli(tsMs),
			Open:      o,
			High:      h,
			Low:       l,
			Close:     cl,
			Volume:    v,
		})
	}
	return candles, nil
}

func (c *binanceClient) FetchBalance() (float64, error) {
	data, err := c.requestSigned(http.MethodGet, "/fapi/v2/account", nil)
	if err != nil {
		return 0, err
	}
	var resp struct {
		TotalMarginBalance string `json:"totalMarginBalance"`
		TotalWalletBalance string `json:"totalWalletBalance"`
		Assets             []struct {
			Asset            string `json:"asset"`
			WalletBalance    string `json:"walletBalance"`
			MarginBalance    string `json:"marginBalance"`
			AvailableBalance string `json:"availableBalance"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, err
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(resp.TotalMarginBalance), 64); err == nil && v > 0 {
		return v, nil
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(resp.TotalWalletBalance), 64); err == nil && v > 0 {
		return v, nil
	}
	for _, b := range resp.Assets {
		if strings.EqualFold(strings.TrimSpace(b.Asset), "USDT") {
			if v, err := strconv.ParseFloat(strings.TrimSpace(b.MarginBalance), 64); err == nil && v > 0 {
				return v, nil
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(b.WalletBalance), 64); err == nil && v > 0 {
				return v, nil
			}
			v, _ := strconv.ParseFloat(strings.TrimSpace(b.AvailableBalance), 64)
			return v, nil
		}
	}
	return 0, nil
}

func (c *binanceClient) SetLeverage(symbol string, leverage int) error {
	vals := url.Values{}
	vals.Set("symbol", normalizeSymbol(symbol))
	vals.Set("leverage", strconv.Itoa(leverage))
	_, err := c.requestSigned(http.MethodPost, "/fapi/v1/leverage", vals)
	return err
}

func (c *binanceClient) FetchPosition(symbol string) (*models.Position, error) {
	vals := url.Values{}
	vals.Set("symbol", normalizeSymbol(symbol))
	data, err := c.requestSigned(http.MethodGet, "/fapi/v2/positionRisk", vals)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Symbol           string `json:"symbol"`
		PositionAmt      string `json:"positionAmt"`
		EntryPrice       string `json:"entryPrice"`
		UnRealizedProfit string `json:"unRealizedProfit"`
		Leverage         string `json:"leverage"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	sz, _ := strconv.ParseFloat(row.PositionAmt, 64)
	if sz == 0 {
		return nil, nil
	}
	entry, _ := strconv.ParseFloat(row.EntryPrice, 64)
	upl, _ := strconv.ParseFloat(row.UnRealizedProfit, 64)
	lev, _ := strconv.ParseFloat(row.Leverage, 64)
	side := "long"
	if sz < 0 {
		side = "short"
		sz = -sz
	}
	return &models.Position{
		Side:          side,
		Size:          sz,
		EntryPrice:    entry,
		UnrealizedPnL: upl,
		Leverage:      lev,
		Symbol:        row.Symbol,
	}, nil
}

func (c *binanceClient) PlaceMarketOrder(symbol, side string, size float64, reduceOnly bool) error {
	_, err := c.PlaceMarketOrderWithResult(symbol, side, size, reduceOnly)
	return err
}

func (c *binanceClient) PlaceMarketOrderWithResult(symbol, side string, size float64, reduceOnly bool) (models.OrderResult, error) {
	vals := url.Values{}
	vals.Set("symbol", normalizeSymbol(symbol))
	vals.Set("side", strings.ToUpper(side))
	vals.Set("type", "MARKET")
	vals.Set("positionSide", "BOTH")
	vals.Set("quantity", fmt.Sprintf("%.4f", size))
	if reduceOnly {
		vals.Set("reduceOnly", "true")
	}
	data, err := c.requestSigned(http.MethodPost, "/fapi/v1/order", vals)
	if err != nil {
		return models.OrderResult{}, err
	}
	var resp struct {
		OrderID       int64  `json:"orderId"`
		ClientOrderID string `json:"clientOrderId"`
		Status        string `json:"status"`
		Symbol        string `json:"symbol"`
		Side          string `json:"side"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return models.OrderResult{}, err
	}
	if resp.OrderID == 0 {
		return models.OrderResult{}, fmt.Errorf("下单失败: %s", string(data))
	}
	return models.OrderResult{
		OrderID:    strconv.FormatInt(resp.OrderID, 10),
		ClientID:   resp.ClientOrderID,
		State:      mapOrderState(resp.Status),
		Symbol:     resp.Symbol,
		Side:       strings.ToLower(resp.Side),
		Size:       size,
		ReduceOnly: reduceOnly,
	}, nil
}

func (c *binanceClient) FetchOrder(symbol, orderID string) (*models.OrderStatus, error) {
	vals := url.Values{}
	vals.Set("symbol", normalizeSymbol(symbol))
	vals.Set("orderId", orderID)
	data, err := c.requestSigned(http.MethodGet, "/fapi/v1/order", vals)
	if err != nil {
		return nil, err
	}
	var row struct {
		OrderID     int64  `json:"orderId"`
		Symbol      string `json:"symbol"`
		Status      string `json:"status"`
		ExecutedQty string `json:"executedQty"`
		AvgPrice    string `json:"avgPrice"`
		Side        string `json:"side"`
		ReduceOnly  bool   `json:"reduceOnly"`
		UpdateTime  int64  `json:"updateTime"`
	}
	if err := json.Unmarshal(data, &row); err != nil {
		return nil, err
	}
	if row.OrderID == 0 {
		return nil, nil
	}
	filled, _ := strconv.ParseFloat(row.ExecutedQty, 64)
	avg, _ := strconv.ParseFloat(row.AvgPrice, 64)
	return &models.OrderStatus{
		OrderID:    strconv.FormatInt(row.OrderID, 10),
		State:      mapOrderState(row.Status),
		FilledSize: filled,
		AvgPrice:   avg,
		Symbol:     row.Symbol,
		Side:       strings.ToLower(row.Side),
		ReduceOnly: row.ReduceOnly,
		UpdateTime: strconv.FormatInt(row.UpdateTime, 10),
	}, nil
}
