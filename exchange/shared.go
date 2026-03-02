package exchange

import (
	"strconv"
	"strings"
)

func normalizeExchangeName(exchange string) string {
	name := strings.ToLower(strings.TrimSpace(exchange))
	switch name {
	case "okx":
		return "okx"
	default:
		return "binance"
	}
}

func normalizeSymbol(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "SWAP", "")
	return s
}

func mapOrderState(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "NEW", "LIVE":
		return "live"
	case "PARTIALLY_FILLED":
		return "partially_filled"
	case "FILLED":
		return "filled"
	case "CANCELED", "CANCELLED", "EXPIRED", "REJECTED":
		return "canceled"
	default:
		if strings.Contains(s, "CANCELED") || strings.Contains(s, "CANCELLED") {
			return "canceled"
		}
		return strings.ToLower(s)
	}
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	default:
		return 0
	}
}

func formatSize(v float64) string {
	raw := strconv.FormatFloat(v, 'f', 8, 64)
	raw = strings.TrimRight(raw, "0")
	raw = strings.TrimRight(raw, ".")
	if raw == "" || raw == "-0" {
		return "0"
	}
	return raw
}
