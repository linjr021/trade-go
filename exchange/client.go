package exchange

import (
	"fmt"
	"trade-go/config"
	"trade-go/models"
)

// Client 统一交易所客户端，对外暴露稳定接口。
type Client struct {
	exchange string
	impl     backend
}

func NewClient() *Client {
	exName := normalizeExchangeName("")
	if config.Config != nil {
		exName = normalizeExchangeName(config.Config.ActiveExchange)
	}

	var impl backend
	switch exName {
	case "okx":
		impl = newOKXClient(config.Config)
	default:
		exName = "binance"
		impl = newBinanceClient(config.Config)
	}

	return &Client{
		exchange: exName,
		impl:     impl,
	}
}

func (c *Client) ActiveExchange() string {
	if c == nil {
		return "binance"
	}
	return normalizeExchangeName(c.exchange)
}

func (c *Client) FetchOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	if c == nil || c.impl == nil {
		return nil, fmt.Errorf("exchange client not initialized")
	}
	return c.impl.FetchOHLCV(symbol, timeframe, limit)
}

func (c *Client) FetchBalance() (float64, error) {
	if c == nil || c.impl == nil {
		return 0, fmt.Errorf("exchange client not initialized")
	}
	return c.impl.FetchBalance()
}

func (c *Client) FetchAvailableBalance() (float64, error) {
	if c == nil || c.impl == nil {
		return 0, fmt.Errorf("exchange client not initialized")
	}
	return c.impl.FetchAvailableBalance()
}

func (c *Client) SetLeverage(symbol string, leverage int) error {
	if c == nil || c.impl == nil {
		return fmt.Errorf("exchange client not initialized")
	}
	return c.impl.SetLeverage(symbol, leverage)
}

func (c *Client) FetchPosition(symbol string) (*models.Position, error) {
	if c == nil || c.impl == nil {
		return nil, fmt.Errorf("exchange client not initialized")
	}
	return c.impl.FetchPosition(symbol)
}

func (c *Client) PlaceMarketOrder(symbol, side string, size float64, reduceOnly bool) error {
	if c == nil || c.impl == nil {
		return fmt.Errorf("exchange client not initialized")
	}
	return c.impl.PlaceMarketOrder(symbol, side, size, reduceOnly)
}

func (c *Client) PlaceMarketOrderWithResult(symbol, side string, size float64, reduceOnly bool) (models.OrderResult, error) {
	if c == nil || c.impl == nil {
		return models.OrderResult{}, fmt.Errorf("exchange client not initialized")
	}
	return c.impl.PlaceMarketOrderWithResult(symbol, side, size, reduceOnly)
}

func (c *Client) FetchOrder(symbol, orderID string) (*models.OrderStatus, error) {
	if c == nil || c.impl == nil {
		return nil, fmt.Errorf("exchange client not initialized")
	}
	return c.impl.FetchOrder(symbol, orderID)
}
