#!/usr/bin/env python3
import json
import os
import urllib.error
import urllib.request
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


def _clamp(v, lo, hi):
    return max(lo, min(hi, v))


def _build_kline_rows(pd):
    klines = _pick(pd, "KlineData", "kline_data", default=[]) or []
    rows = []
    for k in klines:
        if not isinstance(k, dict):
            continue
        high = _f(_pick(k, "High", "high"), 0.0)
        low = _f(_pick(k, "Low", "low"), 0.0)
        close = _f(_pick(k, "Close", "close"), 0.0)
        open_ = _f(_pick(k, "Open", "open"), close)
        if high <= 0 or low <= 0 or close <= 0:
            continue
        rows.append({"open": open_, "high": high, "low": low, "close": close})
    return rows


def _calc_atr(rows, period=14):
    if len(rows) < 2:
        return 0.0
    trs = []
    prev_close = rows[0]["close"]
    for r in rows[1:]:
        high = r["high"]
        low = r["low"]
        close = r["close"]
        tr = max(high - low, abs(high - prev_close), abs(low - prev_close))
        trs.append(tr)
        prev_close = close
    if not trs:
        return 0.0
    use = trs[-period:] if len(trs) >= period else trs
    return sum(use) / len(use)


def _calc_recent_range_pct(rows, lookback=8):
    if not rows:
        return 0.0
    use = rows[-lookback:] if len(rows) >= lookback else rows
    high = max(r["high"] for r in use)
    low = min(r["low"] for r in use)
    close = use[-1]["close"]
    if close <= 0:
        return 0.0
    return (high - low) / close


def _confidence_from_edge(edge):
    a = abs(edge)
    if a >= 18:
        return "HIGH"
    if a >= 10:
        return "MEDIUM"
    return "LOW"


def _strategy_combo(signal, trend_bias, rsi, bb_pos, atr_ratio):
    if signal == "HOLD":
        return "no_trade"
    if atr_ratio > 0.012 and trend_bias > 0.55:
        return "breakout"
    if (rsi < 35 and bb_pos < 0.25) or (rsi > 65 and bb_pos > 0.75):
        return "mean_reversion"
    return "trend_following"


def _llm_enabled():
    return os.getenv("STRATEGY_LLM_ENABLED", "false").lower() == "true"


def _llm_settings():
    return {
        "api_key": os.getenv("AI_API_KEY", "").strip(),
        "base_url": os.getenv("AI_BASE_URL", "").strip(),
        "model": os.getenv("AI_MODEL", "chat-model").strip() or "chat-model",
        "timeout_sec": _f(os.getenv("STRATEGY_LLM_TIMEOUT_SEC", "8"), 8.0),
    }


def _parse_json_obj(text):
    if not text:
        return None
    try:
        return json.loads(text)
    except Exception:
        pass
    start = text.find("{")
    end = text.rfind("}")
    if start == -1 or end <= start:
        return None
    try:
        return json.loads(text[start : end + 1])
    except Exception:
        return None


def _call_llm_tuner(payload):
    if not _llm_enabled():
        return None
    settings = _llm_settings()
    if not settings["api_key"] or not settings["base_url"]:
        return None

    prompt = {
        "task": "Tune crypto strategy parameters for current market regime.",
        "constraints": {
            "bias_shift": "[-8,8] points",
            "entry_threshold": "[8,16]",
            "sl_mult": "[1.2,2.2]",
            "tp_mult": "[1.6,3.4]",
            "filter_mode": "strict|normal|loose",
            "confidence_delta": "-1|0|1",
        },
        "market": payload.get("price_data", {}),
        "recent_signals": payload.get("last_signals", [])[-8:],
        "output_json_only": True,
        "schema": {
            "bias_shift": 0,
            "entry_threshold": 10,
            "sl_mult": 1.6,
            "tp_mult": 2.2,
            "filter_mode": "normal",
            "confidence_delta": 0,
            "reason": "short reason",
        },
    }
    req = {
        "model": settings["model"],
        "messages": [
            {"role": "system", "content": "You are a quant strategy parameter tuner. Output JSON only."},
            {"role": "user", "content": json.dumps(prompt, ensure_ascii=False)},
        ],
        "temperature": 0.1,
        "stream": False,
    }
    body = json.dumps(req).encode("utf-8")
    http_req = urllib.request.Request(
        settings["base_url"],
        data=body,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {settings['api_key']}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(http_req, timeout=settings["timeout_sec"]) as resp:
            raw = resp.read().decode("utf-8", errors="ignore")
    except (urllib.error.URLError, TimeoutError, ValueError):
        return None

    obj = _parse_json_obj(raw)
    if isinstance(obj, dict) and "choices" in obj:
        try:
            content = obj["choices"][0]["message"]["content"]
        except Exception:
            return None
        obj = _parse_json_obj(content)
    if not isinstance(obj, dict):
        return None
    return obj


def _apply_llm_tune(base, tune):
    if not isinstance(tune, dict):
        return base
    out = dict(base)
    out["bias_shift"] = _clamp(_f(tune.get("bias_shift"), out["bias_shift"]), -8.0, 8.0)
    out["entry_threshold"] = _clamp(_f(tune.get("entry_threshold"), out["entry_threshold"]), 8.0, 16.0)
    out["sl_mult"] = _clamp(_f(tune.get("sl_mult"), out["sl_mult"]), 1.2, 2.2)
    out["tp_mult"] = _clamp(_f(tune.get("tp_mult"), out["tp_mult"]), 1.6, 3.4)
    mode = str(tune.get("filter_mode", out["filter_mode"])).strip().lower()
    out["filter_mode"] = mode if mode in ("strict", "normal", "loose") else out["filter_mode"]
    out["confidence_delta"] = int(_clamp(_f(tune.get("confidence_delta"), out["confidence_delta"]), -1, 1))
    out["llm_reason"] = str(tune.get("reason", "")).strip()
    return out


def _shift_confidence(conf, delta):
    order = ["LOW", "MEDIUM", "HIGH"]
    idx = order.index(conf) if conf in order else 0
    idx = int(_clamp(idx + delta, 0, 2))
    return order[idx]


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
    sma50 = _f(_pick(technical, "SMA50", "sma50"), sma20)
    bb_pos = _f(_pick(technical, "BBPosition", "bb_position"), 0.5)
    volume_ratio = _f(_pick(technical, "VolumeRatio", "volume_ratio"), 1.0)
    overall = str(_pick(trend, "Overall", "overall", default="")).lower()

    rows = _build_kline_rows(pd)
    atr = _calc_atr(rows, period=14)
    atr_ratio = (atr / price) if price > 0 else 0.0
    range_pct = _calc_recent_range_pct(rows, lookback=8)

    # 1) 趋势因子（权重40）
    trend_bias = 0.5
    if ("上涨" in overall) or ("bull" in overall):
        trend_bias += 0.25
    if ("下跌" in overall) or ("bear" in overall):
        trend_bias -= 0.25
    if price > sma20 > 0:
        trend_bias += 0.10
    elif price < sma20 and sma20 > 0:
        trend_bias -= 0.10
    if price > sma50 > 0:
        trend_bias += 0.10
    elif price < sma50 and sma50 > 0:
        trend_bias -= 0.10
    trend_bias = _clamp(trend_bias, 0.0, 1.0)

    # 2) 动量因子（权重30）
    momentum_bias = 0.5
    macd_diff = macd - macd_sig
    momentum_bias += _clamp(macd_diff * 1000.0, -0.20, 0.20)
    if rsi <= 30:
        momentum_bias += 0.08
    elif rsi >= 70:
        momentum_bias -= 0.08
    momentum_bias += _clamp(price_change / 10.0, -0.12, 0.12)
    momentum_bias = _clamp(momentum_bias, 0.0, 1.0)

    # 3) 波动/位置因子（权重20）
    volpos_bias = 0.5
    volpos_bias += _clamp((0.5 - bb_pos) * 0.5, -0.15, 0.15)
    volpos_bias += _clamp((range_pct - 0.012) * 4.0, -0.12, 0.12)
    volpos_bias = _clamp(volpos_bias, 0.0, 1.0)

    # 4) 成交量因子（权重10）
    volume_bias = _clamp(0.5 + (volume_ratio - 1.0) * 0.3, 0.0, 1.0)

    long_score = (
        trend_bias * 40.0
        + momentum_bias * 30.0
        + volpos_bias * 20.0
        + volume_bias * 10.0
    )
    short_score = 100.0 - long_score
    edge = long_score - short_score

    params = {
        "bias_shift": 0.0,
        "entry_threshold": 10.0,
        "sl_mult": 1.6,
        "tp_mult": 2.2,
        "filter_mode": "normal",
        "confidence_delta": 0,
        "llm_reason": "",
    }
    llm_tune = _call_llm_tuner(payload)
    params = _apply_llm_tune(params, llm_tune)
    edge += params["bias_shift"]

    # 交易过滤：低波动 + 优势不足 => HOLD
    min_atr = 0.002
    min_range = 0.004
    if params["filter_mode"] == "strict":
        min_atr = 0.0028
        min_range = 0.005
    elif params["filter_mode"] == "loose":
        min_atr = 0.0016
        min_range = 0.003

    if atr_ratio < min_atr or range_pct < min_range:
        signal = "HOLD"
    elif edge >= params["entry_threshold"]:
        signal = "BUY"
    elif edge <= -params["entry_threshold"]:
        signal = "SELL"
    else:
        signal = "HOLD"

    confidence = _confidence_from_edge(edge)
    confidence = _shift_confidence(confidence, params["confidence_delta"])

    # ATR 自适应止盈止损
    atr_fallback = price * 0.006
    atr_use = atr if atr > 0 else atr_fallback
    sl_mult = params["sl_mult"]
    tp_mult = params["tp_mult"]
    if confidence == "HIGH":
        sl_mult = max(sl_mult, 1.8)
        tp_mult = max(tp_mult, 2.8)
    elif confidence == "LOW":
        sl_mult = min(sl_mult, 1.4)
        tp_mult = min(tp_mult, 2.0)

    if signal == "BUY":
        stop_loss = price - sl_mult * atr_use
        take_profit = price + tp_mult * atr_use
    elif signal == "SELL":
        stop_loss = price + sl_mult * atr_use
        take_profit = price - tp_mult * atr_use
    else:
        stop_loss = price - 1.0 * atr_use
        take_profit = price + 1.0 * atr_use

    combo = _strategy_combo(signal, trend_bias, rsi, bb_pos, atr_ratio)
    reason = (
        f"L{long_score:.1f}/S{short_score:.1f}, edge={edge:.1f}, "
        f"trend={trend_bias:.2f}, mom={momentum_bias:.2f}, atr%={atr_ratio*100:.2f}"
    )
    if params["llm_reason"]:
        reason = f"{reason}; llm={params['llm_reason'][:80]}"

    return {
        "signal": signal,
        "reason": reason,
        "stop_loss": round(stop_loss, 4),
        "take_profit": round(take_profit, 4),
        "confidence": confidence,
        "strategy_combo": combo,
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
