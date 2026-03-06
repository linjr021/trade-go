
export const MENU_ITEMS = [
  { key: 'assets', label: '资产详情' },
  { key: 'live', label: '实盘交易' },
  { key: 'paper', label: '模拟交易' },
  { key: 'skill_workflow', label: 'AI 工作流' },
  { key: 'advanced', label: '高级参数' },
  { key: 'builder', label: '策略生成' },
  { key: 'backtest', label: '历史回测' },
  { key: 'auth_admin', label: '权限审计' },
  { key: 'system', label: '系统设置' },
]
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
  { product: 'kimi', label: 'Kimi', base_url: 'https://api.moonshot.cn/v1' },
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
      { key: 'MODE', label: '运行模式（prod/test/dev）' },
      { key: 'HTTP_ADDR', label: '服务地址（:8080）' },
    ],
  },
  {
    title: '自动评估',
    fields: [
      { key: 'AUTO_REVIEW_ENABLED', label: '启用自动评估（true/false）' },
      { key: 'AUTO_REVIEW_AFTER_ORDER_ONLY', label: '仅下单后评估（true/false）' },
      { key: 'AUTO_REVIEW_INTERVAL_SEC', label: '评估间隔秒数（60-86400）' },
      { key: 'AUTO_REVIEW_VOLATILITY_PCT', label: '波动阈值%（>0）' },
      { key: 'AUTO_REVIEW_DRAWDOWN_WARN_PCT', label: '回撤预警比例（0-1）' },
      { key: 'AUTO_REVIEW_LOSS_STREAK_WARN', label: '连续亏损预警次数' },
      { key: 'AUTO_REVIEW_RISK_REDUCE_FACTOR', label: '风险收缩系数（0-1）' },
      { key: 'AUTO_STRATEGY_REGEN_ENABLED', label: '启用自动重生成（true/false）' },
      { key: 'AUTO_STRATEGY_REGEN_COOLDOWN_SEC', label: '重生成冷却秒数（300-604800）' },
      { key: 'AUTO_STRATEGY_REGEN_LOSS_STREAK', label: '重生成连续亏损阈值' },
      { key: 'AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT', label: '重生成回撤阈值（0-1）' },
      { key: 'AUTO_STRATEGY_REGEN_MIN_RR', label: '重生成最小盈亏比（1-10）' },
    ],
  },
]

export const AUTO_REVIEW_ENV_KEYS = [
  'AUTO_REVIEW_ENABLED',
  'AUTO_REVIEW_AFTER_ORDER_ONLY',
  'AUTO_REVIEW_INTERVAL_SEC',
  'AUTO_REVIEW_VOLATILITY_PCT',
  'AUTO_REVIEW_DRAWDOWN_WARN_PCT',
  'AUTO_REVIEW_LOSS_STREAK_WARN',
  'AUTO_REVIEW_RISK_REDUCE_FACTOR',
  'AUTO_STRATEGY_REGEN_ENABLED',
  'AUTO_STRATEGY_REGEN_COOLDOWN_SEC',
  'AUTO_STRATEGY_REGEN_LOSS_STREAK',
  'AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT',
  'AUTO_STRATEGY_REGEN_MIN_RR',
]

export const envFieldDefs = envFieldGroups.flatMap((group) => group.fields)

export const systemSettingDefaults = {
  PRODUCT_NAME: '21xG',
  AI_EXECUTION_STRATEGIES: '',
  TRADE_DB_PATH: 'data/trade.db',
  TIMEFRAME: '15m',
  DATA_POINTS: '96',
  ENABLE_WS_MARKET: 'true',
  REALTIME_MIN_INTERVAL_SEC: '5',
  STRATEGY_LLM_ENABLED: 'true',
  STRATEGY_LLM_TIMEOUT_SEC: '60',
  TEST_MODE: 'false',
  AUTO_REVIEW_ENABLED: 'true',
  AUTO_REVIEW_AFTER_ORDER_ONLY: 'true',
  AUTO_REVIEW_INTERVAL_SEC: '1800',
  AUTO_REVIEW_VOLATILITY_PCT: '1.2',
  AUTO_REVIEW_DRAWDOWN_WARN_PCT: '0.05',
  AUTO_REVIEW_LOSS_STREAK_WARN: '2',
  AUTO_REVIEW_RISK_REDUCE_FACTOR: '0.7',
  AUTO_STRATEGY_REGEN_ENABLED: 'true',
  AUTO_STRATEGY_REGEN_COOLDOWN_SEC: '21600',
  AUTO_STRATEGY_REGEN_LOSS_STREAK: '3',
  AUTO_STRATEGY_REGEN_DRAWDOWN_WARN_PCT: '0.08',
  AUTO_STRATEGY_REGEN_MIN_RR: '2.0',
}

export const strategyGeneratorPromptTemplateDefault = `你是资深量化策略研究员。请为 ${'${symbol}'} 在 ${'${habit}'} 交易习惯下生成一套可执行自动策略。
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
- 推荐回测区间、周期、指标、评估口径（总盈亏、胜率、盈亏比、回撤）`
