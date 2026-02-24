# trade-go

基于 Binance USDT 永续的 AI 量化交易系统（Python 策略 + Go 后台 + Vue3 前端）

## 功能说明

- 支持两种执行模式：
  - `MODE=cli`：固定周期执行（默认每 15 分钟）
  - `MODE=web`：WebSocket 事件驱动实时执行
- 从 Binance 拉取 K 线数据（默认 `15m`，`96` 根）
- 计算技术指标：SMA/EMA、MACD、RSI、布林带、成交量、支撑阻力
- 优先调用 Python 策略服务输出交易信号（失败可回退通用 AI 接口）
- 自动在 Binance USDT 永续进行市价开平仓
- 风控引擎统一决定仓位（AI 只负责方向）
- 订单状态确认、部分成交处理、重启后订单状态恢复
- SQLite 持久化：AI 决策、订单、成交、持仓快照、权益曲线、风险事件
- Binance 公共 WebSocket 行情：ticker / kline / funding

## 项目结构

```text
trade-go/
├── main.go              # 程序入口（MODE=cli|web）
├── app/app.go           # 运行模式编排与实时触发
├── server/server.go     # HTTP API 服务
├── trader/bot.go        # 交易主流程（信号、风控、执行）
├── exchange/okx.go      # Binance REST 客户端（保留原文件名）
├── market/ws.go         # Binance WebSocket 行情流
├── risk/engine.go       # 风控引擎
├── storage/sqlite.go    # SQLite 持久化
├── ai/provider.go       # 策略客户端（Python优先，通用AI兜底）
├── strategy_py/         # Python 策略服务
├── frontend/            # Vue3 前端
└── .env.example         # 环境变量示例
```

## 快速开始

### 1. 安装 Go 依赖

```bash
go mod tidy
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env，填写 Binance 与策略相关配置
```

### 3. 启动 Python 策略服务（推荐）

```bash
python3 strategy_py/service.py
```

默认监听 `0.0.0.0:9000`。

### 4. 启动 Go 后台（Web 实时模式）

```bash
MODE=web go run .
```

可选：

```bash
HTTP_ADDR=:9090 MODE=web go run .
```

### 5. 启动前端（Vue3）

```bash
cd frontend
npm install
npm run dev
```

默认前端地址：`http://127.0.0.1:5173`，已代理 `/api` 到 `http://127.0.0.1:8080`。

## API 列表

- `GET /api/status`：机器人状态、交易配置、最近执行信息
- `GET /api/account`：余额与持仓
- `GET /api/signals?limit=30`：最近信号历史
- `POST /api/settings`：更新仓位参数、杠杆、风控阈值
- `POST /api/run`：立即执行一次策略
- `POST /api/scheduler/start`：启动定时调度（非实时模式）
- `POST /api/scheduler/stop`：停止定时调度

## 运行模式

- `MODE=cli`：命令行定时执行模式
- `MODE=web`：API 服务模式（可配合 WebSocket 实时触发）

## 关键配置项

`config/config.go` 中默认配置：

- `Symbol`: `BTCUSDT`
- `HighConfidenceAmount`: `0.01`
- `LowConfidenceAmount`: `0.005`
- `Leverage`: `10`
- `Timeframe`: `15m`
- `DataPoints`: `96`
- `MaxRiskPerTradePct`: `0.01`
- `MaxPositionPct`: `0.20`
- `MaxConsecutiveLosses`: `3`
- `MaxDailyLossPct`: `0.05`
- `MaxDrawdownPct`: `0.12`
- `LiquidationBufferPct`: `0.02`

## 环境变量说明

- `BINANCE_API_KEY`：Binance API Key
- `BINANCE_SECRET`：Binance API Secret
- `PY_STRATEGY_URL`：Python 策略服务地址（如 `http://127.0.0.1:9000`）
- `AI_API_KEY`：通用 AI 接口 Key（Python 服务不可用时兜底可用）
- `AI_BASE_URL`：通用 AI 接口请求地址（如 `https://xxx/v1/chat/completions`）
- `AI_MODEL`：通用 AI 接口模型名
- `MODE`：运行模式（`cli` / `web`）
- `HTTP_ADDR`：Go API 监听地址（默认 `:8080`）
- `ENABLE_WS_MARKET`：是否启用 WebSocket 行情（`true/false`）
- `REALTIME_MIN_INTERVAL_SEC`：实时模式最小执行间隔秒数（默认 `10`）
- `TRADE_DB_PATH`：SQLite 数据文件路径（默认 `data/trade.db`）
- `TEST_MODE`：测试模式（`true` 时不真实下单）

## 重要提示

- 当前下单按 Binance 单向持仓模式（`positionSide=BOTH`）。
- 若你的账户启用了双向持仓，需要调整下单参数后再实盘。
- 本项目仅用于学习和研究，不构成投资建议，实盘风险自担。
