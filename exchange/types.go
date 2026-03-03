package exchange

import "trade-go/models"

// backend 定义统一交易所能力，便于多交易所扩展。
type backend interface {
	FetchOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
	FetchBalance() (float64, error)
	FetchAvailableBalance() (float64, error)
	SetLeverage(symbol string, leverage int) error
	FetchPosition(symbol string) (*models.Position, error)
	PlaceMarketOrder(symbol, side string, size float64, reduceOnly bool) error
	PlaceMarketOrderWithResult(symbol, side string, size float64, reduceOnly bool) (models.OrderResult, error)
	FetchOrder(symbol, orderID string) (*models.OrderStatus, error)
}
