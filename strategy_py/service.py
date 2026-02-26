#!/usr/bin/env python3
import importlib.util
import inspect
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
        "task": "Tune strategy parameters for current market regime with risk-aware bias.",
        "constraints": {
            "bias_shift": "[-8,8] points",
            "entry_threshold": "[8,16]",
            "sl_mult": "[1.2,2.2]",
            "tp_mult": "[1.6,3.4]",
            "filter_mode": "strict|normal|loose",
            "confidence_delta": "-1|0|1",
        },
        "rules": [
            "Do not output trading direction, only parameter tuning.",
            "If uncertainty is high, prefer stricter filters and higher entry threshold.",
            "Keep risk-reward coherent: tp_mult should usually be >= sl_mult.",
            "Output JSON object only.",
        ],
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
            {"role": "system", "content": "You are a risk-aware quant parameter tuner. Return strict JSON only."},
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


def _extract_features(payload):
    pd = payload.get("price_data", {}) or {}
    technical = _pick(pd, "Technical", "technical", default={}) or {}
    trend = _pick(pd, "Trend", "trend", default={}) or {}

    price = _f(_pick(pd, "Price", "price"), 0.0)
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

    atr_fallback = price * 0.006
    atr_use = atr if atr > 0 else atr_fallback

    return {
        "price": price,
        "price_change": price_change,
        "rsi": rsi,
        "macd": macd,
        "macd_sig": macd_sig,
        "sma20": sma20,
        "sma50": sma50,
        "bb_pos": bb_pos,
        "volume_ratio": volume_ratio,
        "overall": overall,
        "atr": atr,
        "atr_use": atr_use,
        "atr_ratio": atr_ratio,
        "range_pct": range_pct,
    }


def _sl_tp(price, atr_use, signal, sl_mult=1.6, tp_mult=2.2):
    if signal == "BUY":
        return round(price - sl_mult * atr_use, 4), round(price + tp_mult * atr_use, 4)
    if signal == "SELL":
        return round(price + sl_mult * atr_use, 4), round(price - tp_mult * atr_use, 4)
    return round(price - atr_use, 4), round(price + atr_use, 4)


def strategy_ai_assisted(payload):
    ft = _extract_features(payload)
    price = ft["price"]
    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": "no_trade",
        }

    trend_bias = 0.5
    if ("上涨" in ft["overall"]) or ("bull" in ft["overall"]):
        trend_bias += 0.25
    if ("下跌" in ft["overall"]) or ("bear" in ft["overall"]):
        trend_bias -= 0.25
    if price > ft["sma20"] > 0:
        trend_bias += 0.10
    elif price < ft["sma20"] and ft["sma20"] > 0:
        trend_bias -= 0.10
    if price > ft["sma50"] > 0:
        trend_bias += 0.10
    elif price < ft["sma50"] and ft["sma50"] > 0:
        trend_bias -= 0.10
    trend_bias = _clamp(trend_bias, 0.0, 1.0)

    momentum_bias = 0.5
    macd_diff = ft["macd"] - ft["macd_sig"]
    momentum_bias += _clamp(macd_diff * 1000.0, -0.20, 0.20)
    if ft["rsi"] <= 30:
        momentum_bias += 0.08
    elif ft["rsi"] >= 70:
        momentum_bias -= 0.08
    momentum_bias += _clamp(ft["price_change"] / 10.0, -0.12, 0.12)
    momentum_bias = _clamp(momentum_bias, 0.0, 1.0)

    volpos_bias = 0.5
    volpos_bias += _clamp((0.5 - ft["bb_pos"]) * 0.5, -0.15, 0.15)
    volpos_bias += _clamp((ft["range_pct"] - 0.012) * 4.0, -0.12, 0.12)
    volpos_bias = _clamp(volpos_bias, 0.0, 1.0)

    volume_bias = _clamp(0.5 + (ft["volume_ratio"] - 1.0) * 0.3, 0.0, 1.0)

    long_score = trend_bias * 40.0 + momentum_bias * 30.0 + volpos_bias * 20.0 + volume_bias * 10.0
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

    min_atr = 0.002
    min_range = 0.004
    if params["filter_mode"] == "strict":
        min_atr = 0.0028
        min_range = 0.005
    elif params["filter_mode"] == "loose":
        min_atr = 0.0016
        min_range = 0.003

    if ft["atr_ratio"] < min_atr or ft["range_pct"] < min_range:
        signal = "HOLD"
    elif edge >= params["entry_threshold"]:
        signal = "BUY"
    elif edge <= -params["entry_threshold"]:
        signal = "SELL"
    else:
        signal = "HOLD"

    confidence = _shift_confidence(_confidence_from_edge(edge), params["confidence_delta"])
    sl_mult = params["sl_mult"]
    tp_mult = params["tp_mult"]
    if confidence == "HIGH":
        sl_mult = max(sl_mult, 1.8)
        tp_mult = max(tp_mult, 2.8)
    elif confidence == "LOW":
        sl_mult = min(sl_mult, 1.4)
        tp_mult = min(tp_mult, 2.0)

    stop_loss, take_profit = _sl_tp(price, ft["atr_use"], signal, sl_mult, tp_mult)
    combo = _strategy_combo(signal, trend_bias, ft["rsi"], ft["bb_pos"], ft["atr_ratio"])
    reason = (
        f"L{long_score:.1f}/S{short_score:.1f}, edge={edge:.1f}, "
        f"trend={trend_bias:.2f}, mom={momentum_bias:.2f}, atr%={ft['atr_ratio']*100:.2f}"
    )
    if params["llm_reason"]:
        reason = f"{reason}; llm={params['llm_reason'][:80]}"

    return {
        "signal": signal,
        "reason": reason,
        "stop_loss": stop_loss,
        "take_profit": take_profit,
        "confidence": confidence,
        "strategy_combo": combo,
    }


def strategy_trend_following(payload):
    ft = _extract_features(payload)
    price = ft["price"]
    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": "trend_following",
        }

    signal = "HOLD"
    edge = 0.0
    if price > ft["sma20"] > ft["sma50"] and ft["macd"] >= ft["macd_sig"]:
        signal = "BUY"
        edge = 14 + _clamp((ft["volume_ratio"] - 1.0) * 6.0, 0.0, 6.0)
    elif price < ft["sma20"] < ft["sma50"] and ft["macd"] <= ft["macd_sig"]:
        signal = "SELL"
        edge = 14 + _clamp((ft["volume_ratio"] - 1.0) * 6.0, 0.0, 6.0)

    stop_loss, take_profit = _sl_tp(price, ft["atr_use"], signal, 1.7, 2.5)
    return {
        "signal": signal,
        "reason": f"trend_following: sma20/sma50 + macd, vol={ft['volume_ratio']:.2f}",
        "stop_loss": stop_loss,
        "take_profit": take_profit,
        "confidence": _confidence_from_edge(edge),
        "strategy_combo": "trend_following",
    }


def strategy_mean_reversion(payload):
    ft = _extract_features(payload)
    price = ft["price"]
    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": "mean_reversion",
        }

    signal = "HOLD"
    edge = 0.0
    if ft["rsi"] <= 32 and ft["bb_pos"] <= 0.2:
        signal = "BUY"
        edge = 12 + (32 - ft["rsi"]) * 0.5
    elif ft["rsi"] >= 68 and ft["bb_pos"] >= 0.8:
        signal = "SELL"
        edge = 12 + (ft["rsi"] - 68) * 0.5

    stop_loss, take_profit = _sl_tp(price, ft["atr_use"], signal, 1.3, 1.9)
    return {
        "signal": signal,
        "reason": f"mean_reversion: rsi={ft['rsi']:.1f}, bb={ft['bb_pos']:.2f}",
        "stop_loss": stop_loss,
        "take_profit": take_profit,
        "confidence": _confidence_from_edge(edge),
        "strategy_combo": "mean_reversion",
    }


def strategy_breakout(payload):
    ft = _extract_features(payload)
    price = ft["price"]
    if price <= 0:
        return {
            "signal": "HOLD",
            "reason": "invalid price",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": "breakout",
        }

    signal = "HOLD"
    edge = 0.0
    if ft["atr_ratio"] >= 0.004 and ft["range_pct"] >= 0.007 and ft["volume_ratio"] >= 1.2:
        if ft["price_change"] > 0.35:
            signal = "BUY"
            edge = 15 + _clamp(ft["price_change"] * 3, 0.0, 8.0)
        elif ft["price_change"] < -0.35:
            signal = "SELL"
            edge = 15 + _clamp(abs(ft["price_change"]) * 3, 0.0, 8.0)

    stop_loss, take_profit = _sl_tp(price, ft["atr_use"], signal, 1.8, 3.0)
    return {
        "signal": signal,
        "reason": f"breakout: atr%={ft['atr_ratio']*100:.2f}, range%={ft['range_pct']*100:.2f}, vol={ft['volume_ratio']:.2f}",
        "stop_loss": stop_loss,
        "take_profit": take_profit,
        "confidence": _confidence_from_edge(edge),
        "strategy_combo": "breakout",
    }


def _normalize_signal(sig, fallback_price, combo):
    if not isinstance(sig, dict):
        sig = {}
    signal = str(sig.get("signal", "HOLD")).upper().strip()
    if signal not in ("BUY", "SELL", "HOLD"):
        signal = "HOLD"
    confidence = str(sig.get("confidence", "LOW")).upper().strip()
    if confidence not in ("HIGH", "MEDIUM", "LOW"):
        confidence = "LOW"

    stop_loss = _f(sig.get("stop_loss"), 0.0)
    take_profit = _f(sig.get("take_profit"), 0.0)
    if fallback_price > 0 and (stop_loss <= 0 or take_profit <= 0):
        atr = max(fallback_price * 0.005, 1e-8)
        stop_loss, take_profit = _sl_tp(fallback_price, atr, signal)

    return {
        "signal": signal,
        "reason": str(sig.get("reason", ""))[:240],
        "stop_loss": round(stop_loss, 4),
        "take_profit": round(take_profit, 4),
        "confidence": confidence,
        "strategy_combo": str(sig.get("strategy_combo", combo)),
    }


def _load_user_strategies():
    reg = {}
    base_dir = os.path.dirname(os.path.abspath(__file__))
    user_dir = os.path.join(base_dir, "user_strategies")
    os.makedirs(user_dir, exist_ok=True)

    for name in sorted(os.listdir(user_dir)):
        if not name.endswith(".py") or name.startswith("_"):
            continue
        file_path = os.path.join(user_dir, name)
        module_name = f"user_strategy_{name[:-3]}"
        try:
            spec = importlib.util.spec_from_file_location(module_name, file_path)
            if spec is None or spec.loader is None:
                continue
            mod = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(mod)
            fn = getattr(mod, "analyze", None)
            if not callable(fn):
                continue
            sid = str(getattr(mod, "STRATEGY_ID", name[:-3])).strip().lower().replace(" ", "_")
            if not sid:
                sid = name[:-3]
            reg[sid] = fn
        except Exception as e:
            print(f"skip user strategy {name}: {e}")
    return reg


def _builtin_registry():
    return {
        "ai_assisted": strategy_ai_assisted,
        "trend_following": strategy_trend_following,
        "mean_reversion": strategy_mean_reversion,
        "breakout": strategy_breakout,
    }


def _registry():
    reg = _builtin_registry()
    reg.update(_load_user_strategies())
    return reg


def _default_enabled_ids(reg):
    env_val = os.getenv("PY_STRATEGY_ENABLED", "").strip()
    if env_val:
        ids = [x.strip().lower() for x in env_val.split(",") if x.strip()]
        return [x for x in ids if x in reg][:3]
    defaults = ["ai_assisted", "trend_following", "mean_reversion", "breakout"]
    return [x for x in defaults if x in reg][:3]


def _choose_enabled_strategies(payload, reg):
    req_ids = payload.get("enabled_strategies")
    if isinstance(req_ids, list):
        ids = [str(x).strip().lower() for x in req_ids if str(x).strip()]
    else:
        ids = _default_enabled_ids(reg)

    out = []
    for sid in ids:
        if sid in reg and sid not in out:
            out.append(sid)
    return (out[:3]) or _default_enabled_ids(reg)


def _call_strategy(fn, payload):
    try:
        sig = inspect.signature(fn)
        if len(sig.parameters) >= 2:
            return fn(payload, _extract_features(payload))
    except Exception:
        pass
    return fn(payload)


def _weight_of(conf):
    if conf == "HIGH":
        return 3
    if conf == "MEDIUM":
        return 2
    return 1


def _summarize_advisories(items):
    buy_w = 0
    sell_w = 0
    hold_w = 0
    for it in items:
        w = _weight_of(it.get("confidence"))
        s = it.get("signal")
        if s == "BUY":
            buy_w += w
        elif s == "SELL":
            sell_w += w
        else:
            hold_w += w

    consensus = "HOLD"
    if buy_w > sell_w and buy_w >= hold_w:
        consensus = "BUY"
    elif sell_w > buy_w and sell_w >= hold_w:
        consensus = "SELL"

    return {
        "buy_weight": buy_w,
        "sell_weight": sell_w,
        "hold_weight": hold_w,
        "consensus": consensus,
        "strategy_count": len(items),
    }


def analyze_advisory(payload):
    reg = _registry()
    enabled = _choose_enabled_strategies(payload or {}, reg)
    price = _f(_pick(payload.get("price_data", {}), "Price", "price"), 0.0)

    items = []
    for sid in enabled:
        fn = reg.get(sid)
        if fn is None:
            continue
        try:
            raw = _call_strategy(fn, payload)
            out = _normalize_signal(raw, price, sid)
            out["strategy_id"] = sid
            items.append(out)
        except Exception as e:
            items.append({
                "strategy_id": sid,
                "signal": "HOLD",
                "reason": f"strategy_error: {str(e)[:80]}",
                "stop_loss": 0,
                "take_profit": 0,
                "confidence": "LOW",
                "strategy_combo": sid,
            })

    summary = _summarize_advisories(items)
    return {
        "strategies": items,
        "summary": summary,
        "enabled_strategies": enabled,
    }


def analyze_consensus(payload):
    advisory = analyze_advisory(payload)
    items = advisory.get("strategies", [])
    summary = advisory.get("summary", {})
    price = _f(_pick(payload.get("price_data", {}), "Price", "price"), 0.0)

    if not items:
        return {
            "signal": "HOLD",
            "reason": "no strategy available",
            "stop_loss": 0,
            "take_profit": 0,
            "confidence": "LOW",
            "strategy_combo": "no_trade",
        }

    best = sorted(items, key=lambda x: (_weight_of(x.get("confidence")), x.get("strategy_id", "")), reverse=True)[0]
    signal = summary.get("consensus", "HOLD")

    if signal == "HOLD":
        stop_loss = best.get("stop_loss", 0)
        take_profit = best.get("take_profit", 0)
    else:
        candidate = [x for x in items if x.get("signal") == signal]
        if candidate:
            candidate.sort(key=lambda x: _weight_of(x.get("confidence")), reverse=True)
            best = candidate[0]
        stop_loss = best.get("stop_loss", 0)
        take_profit = best.get("take_profit", 0)

    out = _normalize_signal(
        {
            "signal": signal,
            "reason": (
                f"consensus={signal}, buy={summary.get('buy_weight', 0)}, "
                f"sell={summary.get('sell_weight', 0)}, hold={summary.get('hold_weight', 0)}, "
                f"anchor={best.get('strategy_id', '-')}: {best.get('reason', '')}"
            ),
            "stop_loss": stop_loss,
            "take_profit": take_profit,
            "confidence": best.get("confidence", "LOW"),
            "strategy_combo": f"consensus_{signal.lower()}",
        },
        price,
        "consensus",
    )
    out["advisory"] = advisory
    return out


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
        if self.path == "/strategies":
            reg = _registry()
            self._json(
                200,
                {
                    "available": sorted(list(reg.keys())),
                    "enabled": _default_enabled_ids(reg),
                    "upload_dir": "strategy_py/user_strategies",
                },
            )
            return
        self._json(404, {"error": "not found"})

    def do_POST(self):
        if self.path not in ("/analyze", "/advisory", "/strategies/reload"):
            self._json(404, {"error": "not found"})
            return

        if self.path == "/strategies/reload":
            reg = _registry()
            self._json(200, {"reloaded": True, "available": sorted(list(reg.keys()))})
            return

        try:
            length = int(self.headers.get("Content-Length", "0"))
            raw = self.rfile.read(length)
            payload = json.loads(raw.decode("utf-8")) if raw else {}
            if self.path == "/advisory":
                self._json(200, analyze_advisory(payload))
                return

            signal = analyze_consensus(payload)
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
