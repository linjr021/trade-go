#!/usr/bin/env python3
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def _pick(obj, *keys, default=None):
    for k in keys:
        if isinstance(obj, dict) and k in obj:
            return obj[k]
    return default


def _f(v, default=0.0):
    try:
        if v is None:
            return default
        return float(v)
    except Exception:
        return default


def build_signal(payload):
    pd = payload.get("price_data", {}) or {}
    technical = _pick(pd, "Technical", "technical", default={}) or {}
    trend = _pick(pd, "Trend", "trend", default={}) or {}

    price = _f(_pick(pd, "Price", "price"), 0.0)
    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
        }

    price_change = _f(_pick(pd, "PriceChange", "price_change"), 0.0)
    rsi = _f(_pick(technical, "RSI", "rsi"), 50.0)
    macd = _f(_pick(technical, "MACD", "macd"), 0.0)
    macd_sig = _f(_pick(technical, "MACDSignal", "macd_signal"), 0.0)
    sma20 = _f(_pick(technical, "SMA20", "sma20"), price)
    overall = str(_pick(trend, "Overall", "overall", default=""))

    score = 0
    reasons = []

    if "上涨" in overall or "bull" in overall.lower():
        score += 1
        reasons.append("trend up")
    if "下跌" in overall or "bear" in overall.lower():
        score -= 1
        reasons.append("trend down")

    if macd > macd_sig:
        score += 1
        reasons.append("macd bullish")
    elif macd < macd_sig:
        score -= 1
        reasons.append("macd bearish")

    if rsi < 35:
        score += 1
        reasons.append("rsi oversold")
    elif rsi > 65:
        score -= 1
        reasons.append("rsi overbought")

    if price > sma20:
        score += 1
        reasons.append("above sma20")
    elif price < sma20:
        score -= 1
        reasons.append("below sma20")

    if price_change > 1.0:
        score += 1
        reasons.append("momentum positive")
    elif price_change < -1.0:
        score -= 1
        reasons.append("momentum negative")

    if score >= 2:
        signal = "BUY"
    elif score <= -2:
        signal = "SELL"
    else:
        signal = "HOLD"

    abs_score = abs(score)
    confidence = "LOW"
    if abs_score >= 4:
        confidence = "HIGH"
    elif abs_score >= 2:
        confidence = "MEDIUM"

    if signal == "BUY":
        stop_loss = price * 0.992
        take_profit = price * (1.018 if confidence == "HIGH" else 1.012)
    elif signal == "SELL":
        stop_loss = price * 1.008
        take_profit = price * (0.982 if confidence == "HIGH" else 0.988)
    else:
        stop_loss = price * 0.99
        take_profit = price * 1.01

    return {
        "signal": signal,
        "reason": "; ".join(reasons[:4]) or "no strong edge",
        "stop_loss": round(stop_loss, 4),
        "take_profit": round(take_profit, 4),
        "confidence": confidence,
    }


class Handler(BaseHTTPRequestHandler):
    def _json(self, status, payload):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"ok": True})
            return
        self._json(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/analyze":
            self._json(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            raw = self.rfile.read(length)
            payload = json.loads(raw.decode("utf-8"))
            signal = build_signal(payload)
            if signal.get("stop_loss", 0) <= 0:
                self._json(400, {"error": "invalid payload"})
                return
            self._json(200, signal)
        except Exception as e:
            self._json(500, {"error": str(e)})


def main():
    host = os.getenv("PY_STRATEGY_HOST", "0.0.0.0")
    port = int(os.getenv("PY_STRATEGY_PORT", "9000"))
    server = ThreadingHTTPServer((host, port), Handler)
    print(f"python strategy server listening on {host}:{port}")
    server.serve_forever()


if __name__ == "__main__":
    main()
