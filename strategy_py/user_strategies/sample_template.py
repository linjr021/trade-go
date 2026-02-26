"""Sample custom strategy.

Rename/copy this file and adjust logic.
"""

STRATEGY_ID = "sample_custom"


def analyze(payload, features=None):
    # payload: raw request body from Go
    # features: extracted metrics (price/rsi/macd/atr_ratio/...)
    price = 0.0
    if isinstance(features, dict):
        price = float(features.get("price", 0.0) or 0.0)

    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": STRATEGY_ID,
        }

    # Replace with your own logic
    return {
        "signal": "HOLD",
        "reason": "template strategy: no entry",
        "stop_loss": round(price * 0.99, 4),
        "take_profit": round(price * 1.01, 4),
        "confidence": "LOW",
        "strategy_combo": STRATEGY_ID,
    }
