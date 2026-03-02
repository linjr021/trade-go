package exchange

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/models"
)

const okxBaseURL = "https://www.okx.com"

type okxClient struct {
	apiKey     string
	secret     string
	passphrase string
	httpClient *http.Client
}

func newOKXClient(cfg *config.AppConfig) *okxClient {
	key := ""
	secret := ""
	passphrase := ""
	if cfg != nil {
		key = strings.TrimSpace(cfg.OKXAPIKey)
		secret = strings.TrimSpace(cfg.OKXSecret)
		passphrase = strings.TrimSpace(cfg.OKXPassword)
	}
	return &okxClient{
		apiKey:     key,
		secret:     secret,
		passphrase: passphrase,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func toOKXInstID(symbol string) string {
	s := normalizeSymbol(symbol)
	if strings.HasSuffix(s, "USDT") {
		base := strings.TrimSuffix(s, "USDT")
		return base + "-USDT-SWAP"
	}
	if strings.Contains(strings.TrimSpace(symbol), "-") {
		raw := strings.ToUpper(strings.TrimSpace(symbol))
		if strings.HasSuffix(raw, "-SWAP") {
			return raw
		}
		return raw + "-SWAP"
	}
	return s + "-SWAP"
}

func fromOKXInstID(instID string) string {
	raw := strings.ToUpper(strings.TrimSpace(instID))
	raw = strings.ReplaceAll(raw, "-", "")
	raw = strings.ReplaceAll(raw, "SWAP", "")
	return raw
}

func toOKXBar(interval string) string {
	m := map[string]string{
		"1m":  "1m",
		"3m":  "3m",
		"5m":  "5m",
		"15m": "15m",
		"30m": "30m",
		"1h":  "1H",
		"2h":  "2H",
		"4h":  "4H",
		"6h":  "6H",
		"8h":  "8H",
		"12h": "12H",
		"1d":  "1D",
		"3d":  "3D",
		"1w":  "1W",
		"1M":  "1M",
	}
	if v, ok := m[strings.TrimSpace(interval)]; ok {
		return v
	}
	return "15m"
}

func (c *okxClient) signPayload(preHash string) string {
	h := hmac.New(sha256.New, []byte(c.secret))
	h.Write([]byte(preHash))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (c *okxClient) requestPublic(path string, values url.Values) ([]byte, error) {
	fullURL := okxBaseURL + path
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
		return nil, fmt.Errorf("okx http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		code := strings.TrimSpace(envelope.Code)
		if code != "" && code != "0" {
			return nil, fmt.Errorf("okx code=%s: %s", code, strings.TrimSpace(envelope.Msg))
		}
	}
	return body, nil
}

func (c *okxClient) requestSigned(method, path string, query url.Values, body []byte) ([]byte, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}

	requestPath := path
	if query != nil && len(query) > 0 {
		requestPath += "?" + query.Encode()
	}
	bodyStr := ""
	if len(body) > 0 {
		bodyStr = string(body)
	}
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	signature := c.signPayload(ts + method + requestPath + bodyStr)

	fullURL := okxBaseURL + requestPath
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, fullURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("OK-ACCESS-KEY", c.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", ts)
	req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("okx http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var envelope struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &envelope); err == nil {
		code := strings.TrimSpace(envelope.Code)
		if code != "" && code != "0" {
			return nil, fmt.Errorf("okx code=%s: %s", code, strings.TrimSpace(envelope.Msg))
		}
	}
	return respBody, nil
}

func (c *okxClient) FetchOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	vals := url.Values{}
	vals.Set("instId", toOKXInstID(symbol))
	vals.Set("bar", toOKXBar(timeframe))
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}
	vals.Set("limit", strconv.Itoa(limit))
	data, err := c.requestPublic("/api/v5/market/candles", vals)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return []models.OHLCV{}, nil
	}
	candles := make([]models.OHLCV, 0, len(resp.Data))
	for i := len(resp.Data) - 1; i >= 0; i-- {
		row := resp.Data[i]
		if len(row) < 6 {
			continue
		}
		tsMs, _ := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
		o, _ := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		h, _ := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		l, _ := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
		cl, _ := strconv.ParseFloat(strings.TrimSpace(row[4]), 64)
		v, _ := strconv.ParseFloat(strings.TrimSpace(row[5]), 64)
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

func (c *okxClient) FetchBalance() (float64, error) {
	query := url.Values{}
	query.Set("ccy", "USDT")
	data, err := c.requestSigned(http.MethodGet, "/api/v5/account/balance", query, nil)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			TotalEq string `json:"totalEq"`
			Details []struct {
				Ccy      string `json:"ccy"`
				Eq       string `json:"eq"`
				CashBal  string `json:"cashBal"`
				AvailBal string `json:"availBal"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, err
	}
	if len(resp.Data) == 0 {
		return 0, nil
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(resp.Data[0].TotalEq), 64); err == nil && v > 0 {
		return v, nil
	}
	for _, d := range resp.Data[0].Details {
		if !strings.EqualFold(strings.TrimSpace(d.Ccy), "USDT") {
			continue
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(d.Eq), 64); err == nil && v > 0 {
			return v, nil
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(d.CashBal), 64); err == nil && v > 0 {
			return v, nil
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(d.AvailBal), 64); err == nil && v > 0 {
			return v, nil
		}
	}
	return 0, nil
}

func (c *okxClient) SetLeverage(symbol string, leverage int) error {
	payload := map[string]string{
		"instId":  toOKXInstID(symbol),
		"lever":   strconv.Itoa(leverage),
		"mgnMode": "cross",
	}
	body, _ := json.Marshal(payload)
	_, err := c.requestSigned(http.MethodPost, "/api/v5/account/set-leverage", nil, body)
	return err
}

func (c *okxClient) FetchPosition(symbol string) (*models.Position, error) {
	query := url.Values{}
	query.Set("instId", toOKXInstID(symbol))
	data, err := c.requestSigned(http.MethodGet, "/api/v5/account/positions", query, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID  string `json:"instId"`
			Pos     string `json:"pos"`
			AvgPx   string `json:"avgPx"`
			Upl     string `json:"upl"`
			Lever   string `json:"lever"`
			PosSide string `json:"posSide"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	for _, row := range resp.Data {
		posRaw, _ := strconv.ParseFloat(strings.TrimSpace(row.Pos), 64)
		if posRaw == 0 {
			continue
		}
		side := strings.ToLower(strings.TrimSpace(row.PosSide))
		if side != "long" && side != "short" {
			if posRaw < 0 {
				side = "short"
			} else {
				side = "long"
			}
		}
		entry, _ := strconv.ParseFloat(strings.TrimSpace(row.AvgPx), 64)
		upl, _ := strconv.ParseFloat(strings.TrimSpace(row.Upl), 64)
		lev, _ := strconv.ParseFloat(strings.TrimSpace(row.Lever), 64)
		inst := strings.TrimSpace(row.InstID)
		outSymbol := normalizeSymbol(symbol)
		if inst != "" {
			outSymbol = fromOKXInstID(inst)
		}
		return &models.Position{
			Side:          side,
			Size:          math.Abs(posRaw),
			EntryPrice:    entry,
			UnrealizedPnL: upl,
			Leverage:      lev,
			Symbol:        outSymbol,
		}, nil
	}
	return nil, nil
}

func (c *okxClient) PlaceMarketOrder(symbol, side string, size float64, reduceOnly bool) error {
	_, err := c.PlaceMarketOrderWithResult(symbol, side, size, reduceOnly)
	return err
}

func (c *okxClient) PlaceMarketOrderWithResult(symbol, side string, size float64, reduceOnly bool) (models.OrderResult, error) {
	side = strings.ToLower(strings.TrimSpace(side))
	if side != "buy" && side != "sell" {
		return models.OrderResult{}, fmt.Errorf("invalid side: %s", side)
	}
	if size <= 0 {
		return models.OrderResult{}, fmt.Errorf("invalid size: %.8f", size)
	}
	payload := map[string]any{
		"instId":  toOKXInstID(symbol),
		"tdMode":  "cross",
		"side":    side,
		"ordType": "market",
		"sz":      formatSize(size),
	}
	if reduceOnly {
		payload["reduceOnly"] = true
	}
	body, _ := json.Marshal(payload)
	data, err := c.requestSigned(http.MethodPost, "/api/v5/trade/order", nil, body)
	if err != nil {
		return models.OrderResult{}, err
	}
	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdID   string `json:"ordId"`
			ClOrdID string `json:"clOrdId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return models.OrderResult{}, err
	}
	if len(resp.Data) == 0 {
		return models.OrderResult{}, fmt.Errorf("okx 下单失败: 空响应")
	}
	row := resp.Data[0]
	if code := strings.TrimSpace(row.SCode); code != "" && code != "0" {
		return models.OrderResult{}, fmt.Errorf("okx 下单失败: code=%s msg=%s", code, strings.TrimSpace(row.SMsg))
	}
	if strings.TrimSpace(row.OrdID) == "" {
		return models.OrderResult{}, fmt.Errorf("okx 下单失败: 无订单ID")
	}
	return models.OrderResult{
		OrderID:    strings.TrimSpace(row.OrdID),
		ClientID:   strings.TrimSpace(row.ClOrdID),
		State:      "live",
		Symbol:     normalizeSymbol(symbol),
		Side:       side,
		Size:       size,
		ReduceOnly: reduceOnly,
	}, nil
}

func (c *okxClient) FetchOrder(symbol, orderID string) (*models.OrderStatus, error) {
	query := url.Values{}
	query.Set("instId", toOKXInstID(symbol))
	query.Set("ordId", strings.TrimSpace(orderID))
	data, err := c.requestSigned(http.MethodGet, "/api/v5/trade/order", query, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdID      string `json:"ordId"`
			InstID     string `json:"instId"`
			State      string `json:"state"`
			AccFillSz  string `json:"accFillSz"`
			AvgPx      string `json:"avgPx"`
			Side       string `json:"side"`
			ReduceOnly string `json:"reduceOnly"`
			UTime      string `json:"uTime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, nil
	}
	row := resp.Data[0]
	filled, _ := strconv.ParseFloat(strings.TrimSpace(row.AccFillSz), 64)
	avg, _ := strconv.ParseFloat(strings.TrimSpace(row.AvgPx), 64)
	symbolOut := normalizeSymbol(symbol)
	if strings.TrimSpace(row.InstID) != "" {
		symbolOut = fromOKXInstID(row.InstID)
	}
	return &models.OrderStatus{
		OrderID:    strings.TrimSpace(row.OrdID),
		State:      mapOrderState(row.State),
		FilledSize: filled,
		AvgPrice:   avg,
		Symbol:     symbolOut,
		Side:       strings.ToLower(strings.TrimSpace(row.Side)),
		ReduceOnly: strings.EqualFold(strings.TrimSpace(row.ReduceOnly), "true"),
		UpdateTime: strings.TrimSpace(row.UTime),
	}, nil
}
