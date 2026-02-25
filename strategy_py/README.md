# Python 策略服务

该服务由 Go 后台调用。当配置了 `PY_STRATEGY_URL` 后，Go 会优先请求该服务生成交易信号。

## 启动方式

```bash
python3 service.py
```

可选环境变量：

- `PY_STRATEGY_HOST`：监听地址，默认 `0.0.0.0`
- `PY_STRATEGY_PORT`：监听端口，默认 `9000`
- `STRATEGY_LLM_ENABLED`：是否启用 LLM 策略调参（`true/false`，默认 `false`）
- `STRATEGY_LLM_TIMEOUT_SEC`：LLM 调参超时秒数（默认 `8`）
- `AI_API_KEY`：LLM Key（启用调参时必填）
- `AI_BASE_URL`：LLM 接口地址（OpenAI 兼容 chat/completions）
- `AI_MODEL`：LLM 模型名（默认 `chat-model`）

## 接口说明

- `GET /health`：健康检查
- `POST /analyze`：策略分析

## /analyze 返回格式

返回 JSON 必须包含以下字段：

```json
{
  "signal": "BUY|SELL|HOLD",
  "reason": "策略原因",
  "stop_loss": 0,
  "take_profit": 0,
  "confidence": "HIGH|MEDIUM|LOW",
  "strategy_combo": "trend_following|mean_reversion|momentum_breakout|range_neutral"
}
```

说明：
- `strategy_combo` 用于策略组合分类与评分统计。
- `strategy_score` 由 Go 后台根据历史盈亏自动计算（0-10 分），无需 Python 返回。
- 启用 `STRATEGY_LLM_ENABLED=true` 后，LLM 不直接下指令，而是调节阈值、过滤强度、SL/TP 参数，再由规则引擎输出最终信号。

## 对接方式

Go 后台 `.env` 示例：

```bash
PY_STRATEGY_URL=http://127.0.0.1:9000
```

配置后，Go 会调用 `POST /analyze` 获取信号。
