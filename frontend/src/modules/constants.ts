// @ts-nocheck

export const MENU_ITEMS = [
  { key: 'assets', label: '资产详情' },
  { key: 'live', label: '实盘交易' },
  { key: 'paper', label: '模拟交易' },
  { key: 'builder', label: '策略生成' },
  { key: 'backtest', label: '历史回测' },
  { key: 'system', label: '系统设置' },
]

export const HABIT_OPTIONS = ['10m', '1h', '4h', '1D', '5D', '30D', '90D']
export const PAIRS = ['BTCUSDT', 'ETHUSDT', 'BNBUSDT', 'SOLUSDT', 'XRPUSDT']
export const ASSET_MIN_MONTH = '2020-01'
export const BACKTEST_MIN_MONTH = '2018-01'
export const BACKTEST_MAX_MONTH = '2025-12'

export const LLM_PRODUCT_CATALOG = [
  { product: 'chatgpt', label: 'ChatGPT', base_url: 'https://api.openai.com/v1' },
  { product: 'deepseek', label: 'DeepSeek', base_url: 'https://api.deepseek.com/v1' },
  { product: 'glm', label: 'GLM', base_url: 'https://open.bigmodel.cn/api/paas/v4' },
  { product: 'qwen', label: 'Qwen', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1' },
  { product: 'minimax', label: 'MiniMax', base_url: 'https://api.minimax.chat/v1' },
]

function getCurrentMonth() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

export const HISTORY_MAX_MONTH = getCurrentMonth()

export const envFieldGroups = [
  {
    title: '运行配置',
    fields: [
      { key: 'PRODUCT_NAME', label: '产品名称' },
      { key: 'PY_STRATEGY_URL', label: 'Python 策略服务 URL' },
      { key: 'MODE', label: '运行模式（prod/test/dev）' },
      { key: 'HTTP_ADDR', label: '服务地址（:8080）' },
    ],
  },
]

export const envFieldDefs = envFieldGroups.flatMap((group) => group.fields)

export const systemSettingDefaults = {
  PRODUCT_NAME: '21xG交易',
  PY_STRATEGY_URL: 'http://127.0.0.1:9000',
  PY_STRATEGY_ENABLED: 'ai_assisted,trend_following,mean_reversion,breakout',
}

export const promptSettingDefaults = {
  trading_ai_system_prompt: `你是加密永续量化交易决策引擎，交易标的默认 ${'${symbol}'}。
你必须遵守：
1) 仅输出严格JSON；
2) 先判断市场状态（trend/range/breakout）再给信号；
3) 信号不充分或冲突时优先HOLD；
4) 你只负责方向、入场区、止损、止盈、盈亏比（盈利/亏损）；仓位由风控引擎执行；
5) 不得决定固定下单金额、固定仓位和固定杠杆；这些均由实盘设置与风控引擎执行。`,
  trading_ai_policy_prompt: `硬边界：
1) 只能输出方向/入场区间/止损/止盈/盈亏比（盈利/亏损），不输出固定开仓金额或固定杠杆。
2) 最低盈亏比（盈利/亏损）门槛：盈亏比 = |TP-Entry| / |Entry-SL| >= 1.5，推荐>=2.0。
3) 若关键条件不足或冲突，必须输出HOLD并给出触发条件。
4) 非高信心不允许频繁反转，优先顺势交易。
5) 输出必须包含：首选策略、备选策略、入场区间、止损、目标、盈亏比估算。`,
  strategy_generator_prompt_template: `你是资深量化策略研究员。请为 ${'${symbol}'} 在 ${'${habit}'} 交易习惯下生成一套可执行自动策略。
按以下结构输出：

1) 市场状态识别
- 趋势/震荡/突破判定规则（必须可量化）

2) 关键位定义
- 支撑/阻力计算逻辑与技术含义

3) 入场与出场
- 首选策略（触发条件、入场区间、SL、TP）
- 备选策略（触发条件、入场区间、SL、TP）

4) 风险管理（硬约束）
- 仓位/杠杆由实盘执行参数与风控引擎统一决定（策略中不固定金额）
- 可给出风险预算公式，但不得写死固定金额阈值
- 最小盈亏比（盈利/亏损）要求：目标>=2.0（最低1.5）

5) 观望与失效条件
- 明确“什么情况下不交易”
- 明确“策略何时失效需停用”

6) 回测建议
- 推荐回测区间、周期、指标、评估口径（总盈亏、胜率、盈亏比、回撤）`,
}

export const strategyTemplateFallback = `"""Sample custom strategy.

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
`
