# Python 策略服务

该服务由 Go 后台调用。当配置了 `PY_STRATEGY_URL` 后，Go 会优先请求该服务生成交易信号。

## 启动方式

```bash
python3 service.py
```

可选环境变量：

- `PY_STRATEGY_HOST`：监听地址，默认 `0.0.0.0`
- `PY_STRATEGY_PORT`：监听端口，默认 `9000`

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
  "confidence": "HIGH|MEDIUM|LOW"
}
```

## 对接方式

Go 后台 `.env` 示例：

```bash
PY_STRATEGY_URL=http://127.0.0.1:9000
```

配置后，Go 会调用 `POST /analyze` 获取信号。
