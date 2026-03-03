# 21xG 交易系统（trade-go）

基于 Go + React 的 AI 量化交易平台，当前支持 Binance / OKX 永续合约接入，包含：

- 实盘交易（风控约束 + 交易执行 + 记录）
- 模拟交易（本地 Dry-Run，走 AI 决策链，不动用交易所资金）
- AI 工作流（5 段 skill 流程，可在线调参）
- 策略生成与自动升级
- 历史回测与回测记录持久化
- 资产详情面板（总资产、可用资金、趋势、盈亏日历、资产分布）

## 1. 当前项目状态（按代码真实行为）

- 后端是纯 Go（`main.go` + `app/` + `server/` + `trader/`）。
- 前端是 React 19 + TypeScript + Vite + Tailwind + shadcn/ui（含一层 Antd 风格兼容原语组件）。
- Python 策略运行时已不再是主链路依赖，当前交易/决策主链路不依赖 Python。
- 策略生成后会写入 `data/generated_strategies.json`，并自动激活到执行策略列表（最多 3 条）。
- 实时模式下支持 WebSocket 触发交易循环；若未启用或启动失败会回退调度器模式。

## 2. 技术栈

### 前端

- React 19
- TypeScript
- Vite 6
- TailwindCSS 3
- shadcn/ui（Radix UI）
- Axios

### 后端

- Go 1.21+
- gorilla/websocket
- joho/godotenv
- SQLite（`modernc.org/sqlite`）

### 交易所与 AI

- Binance Futures API（REST + 公共 WS）
- OKX API（REST + 公共 WS）
- OpenAI 兼容 Chat Completions / Models 协议（可配置 ChatGPT、DeepSeek、GLM、Qwen、MiniMax）

## 3. 核心架构与执行链路

### 3.1 交易执行链（实盘/模拟统一决策逻辑）

每次执行都按固定链路运行：

1. `market-read`：读取市场数据与指标
2. `strategy-select`：AI 输出信号（严格 JSON）
3. `risk-plan`：风险引擎审批仓位/杠杆可行性
4. `order-plan`：下单前校验，实盘执行或模拟执行

任何一步失败都会阻断下单并记录失败原因（通常回退为 HOLD 或 risk block）。

### 3.2 AI 工作流（策略生成/升级）

工作流配置在 `data/skill_workflow.json`，默认 5 步：

1. `spec-builder`（规格构建）
2. `strategy-draft`（策略草案）
3. `optimizer`（参数优化）
4. `risk-reviewer`（风险复核）
5. `release-packager`（发布打包）

工作流提示词会映射到运行时环境变量：

- `TRADING_AI_SYSTEM_PROMPT`
- `TRADING_AI_POLICY_PROMPT`

### 3.3 参数优先级（重要）

当前实现中，集成参数以**前端系统设置/集成管理**为准：

- 智能体参数存于 `data/integrations.json`
- 交易所参数存于 `data/integrations.json`
- 后端会将 active 配置同步写入 `.env`
- 若前端未配置对应项，会清空相关 `.env` 字段（避免旧环境变量误生效）

## 4. 功能模块（前端菜单）

- 资产详情：资产总览、趋势图、盈亏日历、资产分布
- 实盘交易：交易对/策略选择、参数设置、K 线与交易总览、交易记录
- 模拟交易：本地模拟参数、AI 决策链模拟执行、模拟记录
- AI 工作流：步骤配置、约束配置、提示词配置、执行日志与 token 消耗
- 策略生成：按交易习惯生成策略，支持命名、重命名、删除
- 历史回测：参数化回测、结果明细、记录入库、历史可删除
- 系统设置：系统环境变量、智能体参数、交易所参数、系统状态

## 5. 项目结构

```text
trade-go/
├── main.go
├── Dockerfile.backend
├── docker-compose.yml
├── .dockerignore
├── scripts/
│   └── install_linux.sh          # Linux 一键安装（systemd + nginx）
├── app/
│   └── app.go                    # MODE=web|cli 启动编排
├── ai/
│   └── provider.go               # AI 决策调用（OpenAI 兼容）
├── exchange/
│   ├── client.go                 # 交易所统一接口工厂
│   ├── binance.go                # Binance 实现
│   └── okx.go                    # OKX 实现
├── trader/
│   ├── bot.go                    # 主交易流程（四段链路）
│   ├── auto_review.go            # 自动评估与风险收缩
│   └── paper_simulation.go       # 模拟交易 dry-run
├── server/
│   ├── server.go                 # API 路由与主处理
│   ├── integrations.go           # 智能体/交易所集成管理
│   ├── strategy_preference.go    # 策略生成
│   ├── skill_workflow.go         # AI 工作流配置
│   ├── backtest.go               # 回测与历史记录
│   ├── system_settings.go        # 环境变量读写/校验
│   └── system_runtime.go         # 系统状态/软重启
├── storage/
│   └── sqlite.go                 # SQLite schema 与读写
├── data/
│   ├── trade.db                  # SQLite 数据库
│   ├── integrations.json         # 前端集成配置
│   ├── generated_strategies.json # 生成策略库
│   └── skill_workflow.json       # AI 工作流配置
└── frontend/                     # React 19 + TS + Vite
    ├── Dockerfile
    └── docker/nginx.conf
```

## 6. Docker Compose 部署（推荐）

### 6.1 准备

- Docker 24+
- Docker Compose v2+

### 6.2 初始化环境变量

```bash
cp .env.example .env
```

然后在 `.env` 中填写你的智能体与交易所参数（例如 `AI_*`、`BINANCE_*` / `OKX_*`）。

### 6.3 启动

```bash
docker compose up -d --build
```

启动后访问：

- 前端：`http://localhost:5173`
- 后端 API：`http://localhost:8080`

可快速检查：

```bash
curl http://localhost:8080/api/status
```

### 6.4 常用运维命令

```bash
# 查看日志
docker compose logs -f backend
docker compose logs -f frontend

# 重启
docker compose restart backend
docker compose restart frontend

# 停止并删除容器
docker compose down
```

说明：

- `docker-compose.yml` 已将 `./data` 挂载到容器内 `/app/data`，用于持久化数据库与策略文件。
- `./.env` 已挂载到后端容器 `/app/.env`，前端“系统设置”修改环境变量后会回写到该文件。

## 7. Linux 一键安装脚本（systemd + nginx）

除 Docker Compose 外，项目也支持 Linux 原生部署（适合你后续在服务器上长期运行）。

脚本路径：

- `scripts/install_linux.sh`

执行方式：

```bash
chmod +x scripts/install_linux.sh
sudo bash scripts/install_linux.sh
```

脚本会自动完成：

- 安装依赖（默认）：Go、Node.js、Nginx（Debian/Ubuntu）
- 构建后端二进制与前端静态文件
- 安装到 `/opt/trade-go`（可自定义）
- 创建并启动 systemd 服务（后端）
- 配置并重启 Nginx（前端静态 + `/api` 反代）

常用参数：

```bash
sudo bash scripts/install_linux.sh \
  --domain trade.example.com \
  --web-port 80 \
  --api-port 8080 \
  --install-dir /opt/trade-go
```

查看运行状态：

```bash
systemctl status trade-go-backend
journalctl -u trade-go-backend -f
```

说明：

- 当前脚本的自动依赖安装适配 Debian/Ubuntu（`apt-get`）。
- 若你的发行版不是 Debian 系，可先手动安装 `go`/`npm`/`nginx` 后，使用 `--skip-deps` 执行。

## 8. 运行模式

- `MODE=web`：HTTP API + 前端控制；可结合 WS 实时触发
- `MODE=cli`：纯命令行周期执行

说明：若 `MODE` 为空，程序会按 `cli` 处理。

## 9. 配置说明（.env）

> 实际线上建议优先通过前端“系统设置/集成管理”维护，避免手工配置与 UI 状态不一致。

### 8.1 基础运行

- `PRODUCT_NAME`：产品名（前端展示）
- `MODE`：`web` / `cli`
- `HTTP_ADDR`：后端地址，默认 `:8080`
- `TRADE_DB_PATH`：SQLite 路径，默认 `data/trade.db`

### 8.2 AI（智能体）

- `AI_PRODUCT`：产品类型（chatgpt/deepseek/glm/qwen/minimax）
- `AI_BASE_URL`：模型服务 base URL
- `AI_API_KEY`：API Key
- `AI_MODEL`：模型名
- `AI_EXECUTION_STRATEGIES`：启用策略名（逗号分隔，最多 3 条）

### 8.3 交易所

- `ACTIVE_EXCHANGE`：`binance` / `okx`
- Binance：`BINANCE_API_KEY` / `BINANCE_SECRET`
- OKX：`OKX_API_KEY` / `OKX_SECRET` / `OKX_PASSWORD`

### 8.4 交易与风控

- `TRADE_SYMBOL`（默认 `BTCUSDT`）
- `POSITION_SIZING_MODE`：`contracts` / `margin_pct`
- `HIGH_CONFIDENCE_AMOUNT` / `LOW_CONFIDENCE_AMOUNT`
- `HIGH_CONFIDENCE_MARGIN_PCT` / `LOW_CONFIDENCE_MARGIN_PCT`
- `LEVERAGE`（1-150，默认 20）
- `MAX_RISK_PER_TRADE_PCT`
- `MAX_POSITION_PCT`
- `MAX_CONSECUTIVE_LOSSES`
- `MAX_DAILY_LOSS_PCT`
- `MAX_DRAWDOWN_PCT`
- `LIQUIDATION_BUFFER_PCT`

### 8.5 实时触发

- `ENABLE_WS_MARKET`：`true/false`
- `REALTIME_MIN_INTERVAL_SEC`：实时最小执行间隔

### 8.6 自动评估与自动策略升级

- `AUTO_REVIEW_ENABLED`
- `AUTO_REVIEW_AFTER_ORDER_ONLY`
- `AUTO_REVIEW_INTERVAL_SEC`
- `AUTO_REVIEW_VOLATILITY_PCT`
- `AUTO_REVIEW_DRAWDOWN_WARN_PCT`
- `AUTO_REVIEW_LOSS_STREAK_WARN`
- `AUTO_REVIEW_RISK_REDUCE_FACTOR`
- `AUTO_STRATEGY_REGEN_ENABLED`
- `AUTO_STRATEGY_REGEN_COOLDOWN_SEC`
- `AUTO_STRATEGY_REGEN_LOSS_STREAK`
- `AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT`
- `AUTO_STRATEGY_REGEN_MIN_RR`

## 10. API 总览

### 9.1 运行状态与账户

- `GET /api/status`
- `GET /api/account`
- `GET /api/system/runtime`
- `POST /api/system/restart`（软重启：重载客户端，不是进程重启）

### 9.2 资产详情

- `GET /api/assets/overview`
- `GET /api/assets/trend?range=7D|30D|3M|6M|1Y`
- `GET /api/assets/pnl-calendar?month=YYYY-MM`
- `GET /api/assets/distribution`

### 9.3 交易与信号

- `GET /api/market/snapshot`
- `GET /api/signals`
- `GET /api/trade-records`
- `GET /api/strategy-scores`
- `POST /api/settings`
- `POST /api/run`
- `POST /api/scheduler/start`
- `POST /api/scheduler/stop`

### 9.4 策略与工作流

- `POST /api/strategy-preference/generate`
- `GET/POST /api/generated-strategies`
- `GET /api/strategies`
- `GET/POST /api/skill-workflow`
- `POST /api/auto-strategy/regen-now`
- `GET /api/llm-usage/logs`

### 9.5 模拟与回测

- `POST /api/paper/simulate-step`
- `POST /api/backtest`
- `GET /api/backtest-history`
- `GET /api/backtest-history/detail`
- `POST /api/backtest-history/delete`

### 9.6 集成管理

- `GET /api/integrations`
- `POST /api/integrations/llm`
- `POST /api/integrations/llm/update`
- `POST /api/integrations/llm/delete`
- `POST /api/integrations/llm/test`
- `POST /api/integrations/llm/models`
- `POST /api/integrations/exchange`
- `POST /api/integrations/exchange/activate`
- `POST /api/integrations/exchange/delete`
- `GET/POST /api/system-settings`

## 11. 数据持久化

SQLite 默认文件：`data/trade.db`

关键表（部分）：

- `ai_decisions`：AI 决策记录
- `orders` / `fills`：订单与成交
- `position_snapshots`：持仓快照
- `equity_curve`：权益曲线
- `risk_events`：风控与流程事件
- `strategy_combo_stats`：策略组合评分
- `backtest_runs` / `backtest_run_records`：回测历史与明细

另外还有 JSON 配置文件：

- `data/integrations.json`
- `data/generated_strategies.json`
- `data/skill_workflow.json`

## 12. 开发与自检

```bash
# 后端测试
go test ./...

# 全量检查（Go + 前端）
make check

# 前端单独检查
cd frontend && npm run check

# 前端构建
cd frontend && npm run build
```

## 13. 常见问题

### 12.1 为什么“智能体已配置”但策略生成显示回退模板？

常见原因：

- `AI_BASE_URL` / `AI_API_KEY` / `AI_MODEL` 之一为空
- 模型不可用（例如 429 配额不足）
- 路由可达但请求超时

建议先用系统设置里的“测试可达”功能验证。

### 12.2 为什么切换交易所后数据没同步？

交易所生效依赖 `active_exchange_id`，请在系统设置中执行“绑定/激活”。

### 12.3 回测数据来源是当前交易所吗？

当前回测 K 线抓取逻辑走 Binance 公共历史 K 线接口（`server/backtest.go`）。

### 12.4 `MODE` 该填什么？

推荐 `web`。代码运行层支持 `web/cli`，系统设置校验同时兼容了 `prod/test/dev` 文本。

## 14. 安全与风险说明

- 本项目仅用于研究与开发测试，不构成投资建议。
- 实盘前请确认：
  - API Key 权限仅开通必要交易权限
  - 账户模式/持仓模式与策略一致
  - 已在小资金和模拟盘验证风控边界
