# user_strategies

将自定义策略脚本放到此目录，Python 策略服务会自动加载。

约定：

- 文件名：`*.py`
- 必须提供函数：`analyze(payload)` 或 `analyze(payload, features)`
- 可选常量：`STRATEGY_ID = "my_strategy"`

函数返回 JSON 字段建议包含：

```python
{
  "signal": "BUY|SELL|HOLD",
  "reason": "...",
  "stop_loss": 0,
  "take_profit": 0,
  "confidence": "HIGH|MEDIUM|LOW",
  "strategy_combo": "my_strategy"
}
```

上传脚本后可调用 `POST /strategies/reload` 触发重载。
