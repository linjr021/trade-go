import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Bot,
  CandlestickChart,
  FlaskConical,
  History,
  Settings2,
  ShieldCheck,
  Wallet,
} from 'lucide-react'
import { fmtTime } from '@/modules/format'
import { useCloseOnOutside } from '@/modules/use-close-on-outside'
import {
  AUTO_REVIEW_ENV_KEYS,
  envFieldDefs,
  HISTORY_MAX_MONTH,
  LLM_PRODUCT_CATALOG as DEFAULT_LLM_PRODUCT_CATALOG,
  strategyGeneratorPromptTemplateDefault,
  systemSettingDefaults,
} from '@/modules/constants'
import {
  clamp,
  countHanChars,
  joinFieldMessages,
  makeUniqueNameWithIndex,
  mapBacktestSummary,
  mergeSystemDefaults,
  normalizeDecimal,
  normalizeLeverage,
  normalizeTradeSettings,
  parseStrategies,
  resolveRequestError,
  sleep,
} from '@/modules/trade-utils'
import {
  getAccount,
  getAssetOverview,
  getAssetPnLCalendar,
  getAssetDistribution,
  getAssetTrend,
  getIntegrations,
  addExchangeIntegration,
  addLLMIntegration,
  probeLLMModels,
  testLLMIntegration,
  updateLLMIntegration,
  deleteLLMIntegration,
  activateExchangeIntegration,
  deleteExchangeIntegration,
  getMarketSnapshot,
  getTradeRecords,
  getStatus,
  getGeneratedStrategies,
  getStrategies,
  getStrategyScores,
  getSystemSettings,
  generateStrategyPreference,
  getSkillWorkflow,
  saveSkillWorkflow,
  resetSkillWorkflow,
  runAutoStrategyRegenNow,
  resetRiskBaseline,
  getLLMUsageLogs,
  runBacktestApi,
  getBacktestHistory,
  getBacktestHistoryDetail,
  deleteBacktestHistory,
  runNow,
  getPaperState,
  updatePaperConfig,
  startPaperSimulation,
  stopPaperSimulation,
  resetPaperPnL,
  saveSystemSettings,
  getSystemRuntimeStatus,
  restartSystemRuntime,
  startScheduler,
  stopScheduler,
  syncGeneratedStrategies,
  updateSettings,
} from '../api'

const DEFAULT_SKILL_WORKFLOW = {
  version: 'skill-workflow/v1',
  updated_at: '',
  steps: [
    { id: 'spec-builder', name: '规格构建', description: '交易习惯转执行约束（硬边界）', enabled: true, timeout_sec: 8, max_retry: 1, on_fail: 'hard_fail' },
    { id: 'strategy-draft', name: '策略草案', description: '生成结构化策略草案', enabled: true, timeout_sec: 16, max_retry: 1, on_fail: 'hold' },
    { id: 'optimizer', name: '参数优化', description: '回测驱动参数优化', enabled: true, timeout_sec: 18, max_retry: 1, on_fail: 'hold' },
    { id: 'risk-reviewer', name: '风险复核', description: '过拟合与极端行情风险复核', enabled: true, timeout_sec: 10, max_retry: 0, on_fail: 'hard_fail' },
    { id: 'release-packager', name: '发布打包', description: '打包上线策略版本与监控建议', enabled: true, timeout_sec: 10, max_retry: 0, on_fail: 'hold' },
  ],
  constraints: {
    max_leverage_cap: 150,
    max_drawdown_cap_pct: 0.2,
    max_risk_per_trade_cap_pct: 0.03,
    min_profit_loss_floor: 1.5,
    block_trade_on_skill_fail: true,
  },
  prompts: {
    strategy_generator_system_prompt: '你是量化策略架构师，只能返回严格 JSON。',
    strategy_generator_task_prompt: '请基于用户选项与当前市场状态，生成可落地的交易偏好提示词与策略模板。',
    strategy_generator_requirements: [
      '仅输出严格 JSON',
      'preference_prompt 必须包含入场区、止损、止盈、盈亏比与 HOLD 条件',
      'preference_prompt 优先使用相对规则（EMA/ATR/百分比区间），避免写死绝对价格；若给出绝对价位，需附带动态重算条件',
      'generator_prompt 必须包含 ${symbol} 与 ${habit}',
      '不要输出固定下单金额或固定杠杆',
      '实际下单张数/保证金/杠杆必须遵循实盘执行设置',
    ],
    decision_system_prompt: '你是专业量化交易决策引擎。你只能输出严格JSON，不要输出任何额外文本。你负责方向与SL/TP建议，仓位和风控由系统执行。',
    decision_policy_prompt: '优先保护本金；信号冲突或不确定时返回HOLD；避免低置信度反转。',
  },
}

const WORKFLOW_STEP_NAME_BY_ID = {
  'spec-builder': '规格构建',
  'strategy-draft': '策略草案',
  optimizer: '参数优化',
  'risk-reviewer': '风险复核',
  'release-packager': '发布打包',
}

const LEGACY_WORKFLOW_STEP_NAME_MAP = {
  SpecBuilder: '规格构建',
  StrategyDraft: '策略草案',
  Optimizer: '参数优化',
  RiskReviewer: '风险复核',
  ReleasePackager: '发布打包',
}

type CoreRiskSettings = {
  maxRiskPerTradePct: number
  maxPositionPct: number
  maxConsecutiveLosses: number
  maxDailyLossPct: number
  maxDrawdownPct: number
  liquidationBufferPct: number
}

function isDeprecatedBuiltinStrategyName(name) {
  const v = String(name || '').trim().toLowerCase()
  return v === 'ai_assisted' || v === 'trend_following' || v === 'mean_reversion' || v === 'breakout'
}

function strategySymbolNameCN(symbol) {
  const raw = String(symbol || '').trim().toUpperCase()
  if (!raw) return '通用'
  let base = raw
  if (base.includes('-')) {
    base = base.split('-')[0] || base
  } else if (base.endsWith('USDT') && base.length > 4) {
    base = base.slice(0, -4)
  }
  if (base === 'BTC') return '比特币'
  if (base === 'ETH') return '以太坊'
  if (base === 'BNB') return '币安币'
  if (base === 'SOL') return '索拉纳'
  if (base === 'XRP') return '瑞波币'
  if (base === 'DOGE') return '狗狗币'
  if (base === 'ADA') return '艾达币'
  return base
}

function strategyHabitNameCN(habit) {
  const raw = String(habit || '').trim().toLowerCase()
  if (raw === '10m') return '10分钟'
  if (raw === '1h') return '1小时'
  if (raw === '4h') return '4小时'
  if (raw === '1d') return '1日'
  if (raw === '5d') return '5日'
  if (raw === '30d') return '30日'
  if (raw === '90d') return '90日'
  return raw ? raw.toUpperCase() : '标准周期'
}

function strategyStyleNameCN(style) {
  const raw = String(style || '').trim().toLowerCase()
  if (raw === 'trend_follow' || raw === 'trend_following' || raw === 'trend') return '趋势'
  if (raw === 'breakout') return '突破'
  if (raw === 'mean_reversion') return '均值回归'
  if (raw === 'hybrid') return '混合'
  return raw ? '自定义' : '综合'
}

function buildCnStrategyName(symbol, habit, style, autoGenerated = false) {
  const symbolLabel = strategySymbolNameCN(symbol)
  const habitLabel = strategyHabitNameCN(habit)
  const styleLabel = strategyStyleNameCN(style)
  const base = `${symbolLabel}-${habitLabel}-${styleLabel}策略`
  return autoGenerated ? `自动重生成-${base}` : base
}

function localizeWorkflowStepName(name, id, fallback) {
  const raw = String(name || '').trim()
  if (LEGACY_WORKFLOW_STEP_NAME_MAP[raw]) return LEGACY_WORKFLOW_STEP_NAME_MAP[raw]
  if (raw) return raw
  const byID = WORKFLOW_STEP_NAME_BY_ID[String(id || '').trim()]
  if (byID) return byID
  return String(fallback || id || '')
}

function strategyListKey(list = []) {
  return parseStrategies(Array.isArray(list) ? list : [])
    .slice(0, 3)
    .join('|')
}

function sameStrategyList(a = [], b = []) {
  return strategyListKey(a) === strategyListKey(b)
}

function executionStrategiesFromSettings(settings: Record<string, any> = {}) {
  const raw = String(
    settings?.AI_EXECUTION_STRATEGIES ||
    '',
  )
  return parseStrategies(raw.split(',')).filter((name) => !isDeprecatedBuiltinStrategyName(name))
}

function normalizeStrategySource(raw) {
  const v = String(raw || '').trim().toLowerCase()
  if (v === 'workflow_generated') return '工作流生成'
  if (v === 'auto_regen') return '自动重生成'
  if (v === 'manual_external') return '外部/手动'
  return '未知'
}

function normalizeStrategyMetaByName(name: any, raw: Record<string, any> = {}) {
  const strategyName = String(name || '').trim()
  return {
    name: strategyName,
    source: normalizeStrategySource(raw?.source),
    source_code: String(raw?.source || '').trim().toLowerCase() || '',
    workflowVersion: String((raw?.workflow_version ?? raw?.workflowVersion) || '').trim() || '-',
    workflowChain: parseStrategies(
      Array.isArray(raw?.workflow_chain)
        ? raw.workflow_chain
        : (Array.isArray(raw?.workflowChain) ? raw.workflowChain : []),
    ),
    lastUpdatedAt: String((raw?.last_updated_at ?? raw?.lastUpdatedAt) || '').trim() || '-',
  }
}

function buildStrategyMetaMap(details = []) {
  const rows = Array.isArray(details) ? details : []
  const out = {}
  for (const row of rows) {
    const name = String(row?.name || '').trim()
    if (!name) continue
    out[name] = normalizeStrategyMetaByName(name, row)
  }
  return out
}

function mergeStrategyMetaMap(prev: Record<string, any> = {}, next: Record<string, any> = {}) {
  return { ...(prev || {}), ...(next || {}) }
}

function normalizeParamSnapshot(raw: Record<string, any> = {}) {
  const mode = String(raw?.positionSizingMode || raw?.position_sizing_mode || 'margin_pct').trim().toLowerCase()
  const levRaw = Number(raw?.leverage)
  return {
    positionSizingMode: mode === 'contracts' ? 'contracts' : 'margin_pct',
    leverage: Number.isFinite(levRaw) && levRaw > 0 ? normalizeLeverage(levRaw) : 0,
    highConfidenceAmount: normalizeDecimal(raw?.highConfidenceAmount ?? raw?.high_confidence_amount, 0, 1000000),
    lowConfidenceAmount: normalizeDecimal(raw?.lowConfidenceAmount ?? raw?.low_confidence_amount, 0, 1000000),
    highConfidenceMarginPct: normalizeDecimal(raw?.highConfidenceMarginPct ?? raw?.high_confidence_margin_pct, 0, 100),
    lowConfidenceMarginPct: normalizeDecimal(raw?.lowConfidenceMarginPct ?? raw?.low_confidence_margin_pct, 0, 100),
  }
}

function patchStrategyHistoryMeta(prev: any[] = [], metaMap: Record<string, any> = {}) {
  const list = Array.isArray(prev) ? prev : []
  if (!list.length) return list
  let changed = false
  const next = list.map((row) => {
    const strategies = parseStrategies(Array.isArray(row?.strategies) ? row.strategies : []).slice(0, 3)
    if (!strategies.length) return row
    const meta = {}
    for (const strategy of strategies) {
      meta[strategy] = normalizeStrategyMetaByName(strategy, metaMap?.[strategy] || {})
    }
    const same = JSON.stringify(meta) === JSON.stringify(row?.meta || {})
    if (!same) changed = true
    if (same) return row
    return { ...row, meta }
  })
  return changed ? next : list
}

function normalizeGeneratedStrategyItem(row) {
  if (!row || typeof row !== 'object') return null
  const id = String(row.id || '').trim() || `st_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`
  const name = String(row.name || '').trim()
  if (!name) return null
  const preferencePrompt = String(row.preference_prompt ?? row.preferencePrompt ?? '').trim()
  const generatorPrompt = String(row.generator_prompt ?? row.generatorPrompt ?? row.prompt ?? '').trim()
  return {
    id,
    name,
    ruleKey: String(row.rule_key ?? row.ruleKey ?? '').trim(),
    createdAt: String(row.created_at ?? row.createdAt ?? '').trim() || new Date().toISOString(),
    updatedAt: String(row.last_updated_at ?? row.lastUpdatedAt ?? row.created_at ?? row.createdAt ?? '').trim() || new Date().toISOString(),
    source: String(row.source || '').trim() || 'workflow_generated',
    workflowVersion: String(row.workflow_version ?? row.workflowVersion ?? '').trim() || 'skill-workflow/v1',
    workflowChain: parseStrategies(Array.isArray(row.workflow_chain) ? row.workflow_chain : (Array.isArray(row.workflowChain) ? row.workflowChain : [])),
    preferencePrompt,
    prompt: generatorPrompt || strategyGeneratorPromptTemplateDefault,
    logic: String(row.logic || '').trim(),
    basis: String(row.basis || '').trim(),
  }
}

function toGeneratedStrategyPayload(row) {
  return {
    id: String(row?.id || '').trim(),
    name: String(row?.name || '').trim(),
    rule_key: String(row?.ruleKey || '').trim(),
    preference_prompt: String(row?.preferencePrompt || '').trim(),
    generator_prompt: String(row?.prompt || '').trim(),
    logic: String(row?.logic || '').trim(),
    basis: String(row?.basis || '').trim(),
    created_at: String(row?.createdAt || '').trim(),
    last_updated_at: String(row?.updatedAt || row?.createdAt || '').trim(),
    source: String(row?.source || 'workflow_generated').trim(),
    workflow_version: String(row?.workflowVersion || '').trim(),
    workflow_chain: parseStrategies(Array.isArray(row?.workflowChain) ? row.workflowChain : []),
  }
}

export function useDashboardController() {
  const [menu, setMenu] = useState('assets')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [themeMode, setThemeMode] = useState(() => localStorage.getItem('ui-theme-mode') || 'system')
  const [prefersDark, setPrefersDark] = useState(
    () => window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
  )

  const [status, setStatus] = useState<Record<string, any>>({})
  const [account, setAccount] = useState<Record<string, any>>({})
  const [tradeRecords, setTradeRecords] = useState([])
  const [strategyScores, setStrategyScores] = useState([])

  const [strategyOptions, setStrategyOptions] = useState([])
  const [enabledStrategies, setEnabledStrategies] = useState([])
  const [strategyMetaMap, setStrategyMetaMap] = useState<Record<string, any>>({})
  const [activeStrategy, setActiveStrategy] = useState('')
  const [liveStrategyHistory, setLiveStrategyHistory] = useState([])
  const [paperStrategy, setPaperStrategy] = useState('')
  const [paperStrategyHistory, setPaperStrategyHistory] = useState([])
  const [activePair, setActivePairState] = useState('BTCUSDT')
  const [paperPair, setPaperPair] = useState('BTCUSDT')
  const activePairHydratedRef = useRef(false)
  const activePairUserOverrideRef = useRef(false)
  const [liveViewTab, setLiveViewTab] = useState('overview')
  const [paperViewTab, setPaperViewTab] = useState('overview')
  const [strategyPickerOpen, setStrategyPickerOpen] = useState(false)
  const [strategyDraft, setStrategyDraft] = useState([])
  const strategyPickerRef = useRef(null)
  const [paperStrategySelection, setPaperStrategySelection] = useState([])
  const [paperStrategyPickerOpen, setPaperStrategyPickerOpen] = useState(false)
  const [paperStrategyDraft, setPaperStrategyDraft] = useState([])
  const paperStrategyPickerRef = useRef(null)
  const paperStrategyManualRef = useRef(false)
  const [btStrategySelection, setBtStrategySelection] = useState([])
  const [btStrategyPickerOpen, setBtStrategyPickerOpen] = useState(false)
  const [btStrategyDraft, setBtStrategyDraft] = useState([])
  const btStrategyPickerRef = useRef(null)

  const [settings, setSettings] = useState({
    positionSizingMode: 'margin_pct',
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    highConfidenceMarginPct: 5,
    lowConfidenceMarginPct: 0,
    leverage: 20,
  })
  const [paperSettings, setPaperSettings] = useState({
    positionSizingMode: 'margin_pct',
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    highConfidenceMarginPct: 5,
    lowConfidenceMarginPct: 0,
    leverage: 20,
  })
  const [systemSettings, setSystemSettings] = useState<Record<string, any>>({ ...systemSettingDefaults })
  const [systemSubTab, setSystemSubTab] = useState('env')
  const [systemRuntime, setSystemRuntime] = useState<Record<string, any> | null>(null)
  const [backendReachability, setBackendReachability] = useState({
    status: 'unconfigured',
    message: '未检测',
    checkedAt: '',
  })
  const [loadingSystemRuntime, setLoadingSystemRuntime] = useState(false)
  const [restartingBackend, setRestartingBackend] = useState(false)
  const [llmConfigs, setLlmConfigs] = useState<any[]>([])
  const [llmProductCatalog, setLlmProductCatalog] = useState<any[]>(() => DEFAULT_LLM_PRODUCT_CATALOG)
  const [exchangeConfigs, setExchangeConfigs] = useState<any[]>([])
  const [activeExchangeId, setActiveExchangeId] = useState('')
  const [exchangeBound, setExchangeBound] = useState(false)
  const [activatingExchangeId, setActivatingExchangeId] = useState('')
  const [deletingExchangeId, setDeletingExchangeId] = useState('')
  const [addingLLM, setAddingLLM] = useState(false)
  const [editingLLMId, setEditingLLMId] = useState('')
  const [deletingLLMId, setDeletingLLMId] = useState('')
  const [testingLLMId, setTestingLLMId] = useState('')
  const [llmStatusMap, setLlmStatusMap] = useState<Record<string, any>>({})
  const [addingExchange, setAddingExchange] = useState(false)
  const [showLLMModal, setShowLLMModal] = useState(false)
  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [newLLM, setNewLLM] = useState<Record<string, any>>({
    name: '',
    product: 'chatgpt',
    base_url: 'https://api.openai.com/v1',
    api_key: '',
    model: '',
  })
  const [llmModelOptions, setLlmModelOptions] = useState<any[]>([])
  const [probingLLMModels, setProbingLLMModels] = useState(false)
  const [llmProbeMessage, setLlmProbeMessage] = useState('')
  const llmProbeTimerRef = useRef(null)
  const llmProbeSeqRef = useRef(0)
  const llmProbeKeyRef = useRef('')
  const [newExchange, setNewExchange] = useState<Record<string, any>>({
    name: '',
    exchange: 'binance',
    api_key: '',
    secret: '',
    passphrase: '',
  })
  const [paperMargin, setPaperMargin] = useState(200)
  const [paperRecords, setPaperRecords] = useState<any[]>([])
  const [paperLatestDecisionMap, setPaperLatestDecisionMap] = useState({})
  const [paperPnlBaselineMap, setPaperPnlBaselineMap] = useState({})
  const [paperRuntime, setPaperRuntime] = useState<Record<string, any>>({})
  const [paperSimRunning, setPaperSimRunning] = useState(false)
  const [paperSimLoading, setPaperSimLoading] = useState(false)
  const paperConfigSyncTimerRef = useRef<any>(null)
  const paperConfigHashRef = useRef('')
  const paperSettingsHydratedRef = useRef(false)

  const [runningNow, setRunningNow] = useState(false)
  const [startingLive, setStartingLive] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const liveSettingsDirtyRef = useRef(false)
  const [savingSystemSettings, setSavingSystemSettings] = useState(false)
  const [systemSaveHint, setSystemSaveHint] = useState('')
  const [savingAutoReviewSettings, setSavingAutoReviewSettings] = useState(false)
  const [autoReviewSaveHint, setAutoReviewSaveHint] = useState('')
  const [coreRiskSettings, setCoreRiskSettings] = useState<CoreRiskSettings>({
    maxRiskPerTradePct: 1,
    maxPositionPct: 20,
    maxConsecutiveLosses: 3,
    maxDailyLossPct: 5,
    maxDrawdownPct: 12,
    liquidationBufferPct: 2,
  })
  const coreRiskDirtyRef = useRef(false)
  const [savingCoreRiskSettings, setSavingCoreRiskSettings] = useState(false)
  const [coreRiskSaveHint, setCoreRiskSaveHint] = useState('')
  const [resettingRiskBaseline, setResettingRiskBaseline] = useState(false)
  const [toast, setToast] = useState({ visible: false, type: 'success', message: '' })

  const [builderTab, setBuilderTab] = useState('generate')
  const [habit, setHabit] = useState('1h')
  const [genPair, setGenPair] = useState('BTCUSDT')
  const [genStyle, setGenStyle] = useState('hybrid')
  const [genMinRR, setGenMinRR] = useState(2.0)
  const [genAllowReversal, setGenAllowReversal] = useState(false)
  const [genLowConfAction, setGenLowConfAction] = useState('hold')
  const [genDirectionBias, setGenDirectionBias] = useState('balanced')
  const [strategyGenMode, setStrategyGenMode] = useState('')
  const [generatedStrategies, setGeneratedStrategies] = useState([])
  const [selectedRuleId, setSelectedRuleId] = useState('')
  const [renameRuleName, setRenameRuleName] = useState('')
  const [generatingStrategy, setGeneratingStrategy] = useState(false)
  const [skillWorkflow, setSkillWorkflow] = useState(DEFAULT_SKILL_WORKFLOW)
  const [loadingSkillWorkflow, setLoadingSkillWorkflow] = useState(false)
  const [savingSkillWorkflow, setSavingSkillWorkflow] = useState(false)
  const [runningWorkflowUpgradeNow, setRunningWorkflowUpgradeNow] = useState(false)
  const [aiWorkflowTab, setAiWorkflowTab] = useState('config')
  const [aiWorkflowLogs, setAiWorkflowLogs] = useState([])
  const [aiWorkflowLogsLoading, setAiWorkflowLogsLoading] = useState(false)
  const [aiWorkflowLogChannel, setAiWorkflowLogChannel] = useState('strategy_generator')
  const [aiWorkflowLogLimit, setAiWorkflowLogLimit] = useState(50)

  const [btStrategy, setBtStrategy] = useState('')
  const [btPair, setBtPair] = useState('BTCUSDT')
  const [btInitialMargin, setBtInitialMargin] = useState(1000)
  const [btLeverage, setBtLeverage] = useState(20)
  const [btPositionSizingMode, setBtPositionSizingMode] = useState('margin_pct')
  const [btHighConfidenceAmount, setBtHighConfidenceAmount] = useState(0.01)
  const [btLowConfidenceAmount, setBtLowConfidenceAmount] = useState(0.005)
  const [btHighConfidenceMarginPct, setBtHighConfidenceMarginPct] = useState(5)
  const [btLowConfidenceMarginPct, setBtLowConfidenceMarginPct] = useState(0)
  const [btStart, setBtStart] = useState('2021-01')
  const [btEnd, setBtEnd] = useState('2024-12')
  const [btRunning, setBtRunning] = useState(false)
  const [btSummary, setBtSummary] = useState(null)
  const [btRecords, setBtRecords] = useState([])
  const [btHistory, setBtHistory] = useState([])
  const [btHistoryLoading, setBtHistoryLoading] = useState(false)
  const [btHistoryDeletingId, setBtHistoryDeletingId] = useState(0)
  const [btHistorySelectedId, setBtHistorySelectedId] = useState(0)

  const [assetRange, setAssetRange] = useState('30D')
  const [assetMonth, setAssetMonth] = useState(HISTORY_MAX_MONTH)
  const [assetOverview, setAssetOverview] = useState({})
  const [assetTrend, setAssetTrend] = useState([])
  const [assetCalendar, setAssetCalendar] = useState([])
  const [assetDistribution, setAssetDistribution] = useState([])
  const [liveMarketSnapshot, setLiveMarketSnapshot] = useState({})

  const schedulerRunning = Boolean(status?.scheduler_running)
  const resolvedTheme = useMemo(() => {
    if (themeMode === 'system') return prefersDark ? 'dark' : 'light'
    return themeMode === 'dark' ? 'dark' : 'light'
  }, [themeMode, prefersDark])
  const rawProductName = String(systemSettings?.PRODUCT_NAME || '').trim()
  const productName = !rawProductName || rawProductName === 'AI 交易看板' || rawProductName === '21xG交易'
    ? '21xG'
    : rawProductName
  const generatedStrategyNames = useMemo(
    () => generatedStrategies.map((s) => String(s?.name || '').trim()).filter(Boolean),
    [generatedStrategies],
  )
  const executionStrategyOptions = useMemo(
    () => Array.from(new Set([...strategyOptions, ...generatedStrategyNames])),
    [strategyOptions, generatedStrategyNames],
  )
  const selectedStrategyText = enabledStrategies.length ? enabledStrategies.join(', ') : '请选择策略'
  const paperSelectedStrategyText = paperStrategySelection.length ? paperStrategySelection.join(', ') : '请选择策略'
  const btSelectedStrategyText = btStrategySelection.length ? btStrategySelection.join(', ') : '请选择策略'
  const liveStrategyLabel = enabledStrategies.length ? enabledStrategies.join(' / ') : (activeStrategy || '-')
  const activeExchangeType = useMemo(() => {
    const fromAccount = String(account?.active_exchange || '').trim().toLowerCase()
    if (fromAccount === 'okx' || fromAccount === 'binance') {
      return fromAccount
    }
    const fromRuntime = String(systemRuntime?.integration?.exchange?.exchange || '').trim().toLowerCase()
    if (fromRuntime === 'okx' || fromRuntime === 'binance') {
      return fromRuntime
    }
    const id = String(activeExchangeId || '').trim()
    const matched = exchangeConfigs.find((x) => String(x?.id || '').trim() === id)
    const fromConfig = String(matched?.exchange || '').trim().toLowerCase()
    if (fromConfig === 'okx' || fromConfig === 'binance') {
      return fromConfig
    }
    return 'binance'
  }, [account?.active_exchange, systemRuntime, activeExchangeId, exchangeConfigs])
  const selectedLLMPreset = useMemo(
    () => llmProductCatalog.find((p) => String(p?.product || '') === String(newLLM?.product || '')) || llmProductCatalog[0],
    [newLLM?.product, llmProductCatalog],
  )
  const setActivePair = useCallback((nextPair) => {
    const symbol = String(nextPair || '').toUpperCase()
    activePairUserOverrideRef.current = true
    setActivePairState(symbol || 'BTCUSDT')
  }, [])
  const setLiveSettings = useCallback((updater) => {
    liveSettingsDirtyRef.current = true
    setSettings(updater)
  }, [])
  const setCoreRiskField = useCallback((key: keyof CoreRiskSettings, value: number) => {
    coreRiskDirtyRef.current = true
    setCoreRiskSettings((old) => ({ ...old, [key]: value }))
  }, [])
  const sidebarMenuItems = useMemo(
    () => [
      { key: 'assets', label: '资产详情', icon: <Wallet size={16} /> },
      { key: 'live', label: '实盘交易', icon: <CandlestickChart size={16} /> },
      { key: 'paper', label: '模拟交易', icon: <FlaskConical size={16} /> },
      { key: 'skill_workflow', label: 'AI 工作流', icon: <Bot size={16} /> },
      { key: 'builder', label: '策略生成', icon: <Bot size={16} /> },
      { key: 'backtest', label: '历史回测', icon: <History size={16} /> },
      { key: 'auth_admin', label: '权限审计', icon: <ShieldCheck size={16} />, permModule: 'auth_admin' },
      { key: 'system', label: '系统设置', icon: <Settings2 size={16} /> },
    ],
    [],
  )
  const runtimeComponents = useMemo(() => {
    const checkedAt = String(backendReachability.checkedAt || '').trim()
    const probeMessage = checkedAt
      ? `${backendReachability.message}（${fmtTime(checkedAt)}）`
      : backendReachability.message
    const probe = {
      name: '后端连通性',
      status: backendReachability.status || 'unconfigured',
      message: probeMessage || '未检测',
    }
    const serverComponents = Array.isArray(systemRuntime?.components) ? systemRuntime.components : []
    return [probe, ...serverComponents]
  }, [backendReachability, systemRuntime])

  const refreshCore = async (silent = false) => {
    if (!silent) {
      setLoading(true)
      setError('')
    }
    try {
      const [tradeRes, statusRes, accountRes, scoreRes] = await Promise.all([
        getTradeRecords(60),
        getStatus(),
        getAccount(),
        getStrategyScores(40),
      ])
      setTradeRecords(tradeRes?.data?.records || [])
      const st = statusRes.data || {}
      setStatus(st)
      setAccount(accountRes.data || {})
      setStrategyScores(scoreRes?.data?.scores || st?.strategy_scores || [])
      const statusMetaMap = buildStrategyMetaMap(st?.enabled_strategy_details)
      if (Object.keys(statusMetaMap).length) {
        setStrategyMetaMap((old) => mergeStrategyMetaMap(old, statusMetaMap))
      }
      const enabledFromStatus = parseStrategies(Array.isArray(st?.enabled_strategies) ? st.enabled_strategies : []).slice(0, 3)
      if (enabledFromStatus.length) {
        setStrategyOptions((old) => Array.from(new Set([...old, ...enabledFromStatus])))
        setEnabledStrategies((old) => (sameStrategyList(old, enabledFromStatus) ? old : enabledFromStatus))
        setStrategyDraft((old) => {
          const normalized = parseStrategies(old)
            .filter((x) => enabledFromStatus.includes(x))
            .slice(0, 3)
          if (normalized.length) return normalized
          return enabledFromStatus
        })
        if (!enabledFromStatus.includes(activeStrategy)) {
          setActiveStrategy(enabledFromStatus[0])
        }
        if (!paperStrategyManualRef.current) {
          setPaperStrategySelection((old) => (sameStrategyList(old, enabledFromStatus) ? old : enabledFromStatus))
          setPaperStrategyDraft((old) => {
            const normalized = parseStrategies(old)
              .filter((x) => enabledFromStatus.includes(x))
              .slice(0, 3)
            if (normalized.length) return normalized
            return enabledFromStatus
          })
          setPaperStrategy((old) => (enabledFromStatus.includes(old) ? old : enabledFromStatus[0]))
        }
      }

      const cfg = st?.trade_config || {}
      setLiveStrategyHistory(Array.isArray(st?.live_strategy_history) ? st.live_strategy_history : [])
      if (!liveSettingsDirtyRef.current) {
        setSettings((old) => ({
          ...old,
          ...normalizeTradeSettings({
            positionSizingMode: String(cfg.position_sizing_mode ?? old.positionSizingMode ?? 'margin_pct'),
            highConfidenceAmount: Number(cfg.high_confidence_amount ?? old.highConfidenceAmount ?? 0.01),
            lowConfidenceAmount: Number(cfg.low_confidence_amount ?? old.lowConfidenceAmount ?? 0.005),
            highConfidenceMarginPct: Number(cfg.high_confidence_margin_pct ?? 0.05) * 100,
            lowConfidenceMarginPct: Number(cfg.low_confidence_margin_pct ?? 0) * 100,
            leverage: Number(cfg.leverage ?? old.leverage ?? 20),
          }),
        }))
      }
      if (!paperSettingsHydratedRef.current) {
        setPaperSettings((old) => ({
          ...old,
          ...normalizeTradeSettings({
            positionSizingMode: String(cfg.position_sizing_mode ?? old.positionSizingMode ?? 'margin_pct'),
            highConfidenceAmount: Number(cfg.high_confidence_amount ?? old.highConfidenceAmount ?? 0.01),
            lowConfidenceAmount: Number(cfg.low_confidence_amount ?? old.lowConfidenceAmount ?? 0.005),
            highConfidenceMarginPct: Number(cfg.high_confidence_margin_pct ?? 0.05) * 100,
            lowConfidenceMarginPct: Number(cfg.low_confidence_margin_pct ?? 0) * 100,
            leverage: Number(cfg.leverage ?? old.leverage ?? 20),
          }),
        }))
      }
      if (!coreRiskDirtyRef.current) {
        setCoreRiskSettings((old) => ({
          ...old,
          maxRiskPerTradePct: normalizeDecimal(Number(cfg.max_risk_per_trade_pct ?? 0.01) * 100, 0.01, 100),
          maxPositionPct: normalizeDecimal(Number(cfg.max_position_pct ?? 0.2) * 100, 0.01, 100),
          maxConsecutiveLosses: Math.max(0, Math.round(Number(cfg.max_consecutive_losses ?? 3))),
          maxDailyLossPct: normalizeDecimal(Number(cfg.max_daily_loss_pct ?? 0.05) * 100, 0.01, 100),
          maxDrawdownPct: normalizeDecimal(Number(cfg.max_drawdown_pct ?? 0.12) * 100, 0.01, 100),
          liquidationBufferPct: normalizeDecimal(Number(cfg.liquidation_buffer_pct ?? 0.02) * 100, 0.01, 100),
        }))
      }
      if (cfg?.symbol) {
        const symbol = String(cfg.symbol).toUpperCase()
        setActivePairState((prev) => {
          if (!activePairHydratedRef.current) {
            activePairHydratedRef.current = true
            if (activePairUserOverrideRef.current) return prev || symbol || 'BTCUSDT'
            activePairUserOverrideRef.current = false
            return symbol || prev || 'BTCUSDT'
          }
          if (activePairUserOverrideRef.current) return prev || symbol || 'BTCUSDT'
          return symbol || prev || 'BTCUSDT'
        })
        setPaperPair((p) => p || symbol)
      }

      const preferredSymbol = String(activePair || cfg?.symbol || 'BTCUSDT').toUpperCase()
      const preferredTimeframe = String(cfg?.timeframe || cfg?.Timeframe || '1h').trim() || '1h'
      const runtimePrice = Number(st?.runtime?.last_price?.price || 0)
      const runtimeTimestamp = st?.runtime?.last_price?.timestamp || st?.runtime?.last_run_at || null
      const marketRes = await Promise.allSettled([
        getMarketSnapshot({ symbol: preferredSymbol, timeframe: preferredTimeframe }),
      ])
      const liveSnap = marketRes[0]
      if (liveSnap?.status === 'fulfilled' && liveSnap?.value?.data) {
        const data = liveSnap.value.data
        setLiveMarketSnapshot({
          symbol: String(data?.symbol || preferredSymbol).toUpperCase(),
          timeframe: String(data?.timeframe || preferredTimeframe),
          price: Number(data?.price || 0),
          timestamp: data?.timestamp || null,
          change_pct: Number(data?.change_pct || 0),
          active_exchange: String(data?.active_exchange || accountRes?.data?.active_exchange || '').trim().toLowerCase(),
        })
      } else if (runtimePrice > 0) {
        setLiveMarketSnapshot({
          symbol: preferredSymbol,
          timeframe: preferredTimeframe,
          price: runtimePrice,
          timestamp: runtimeTimestamp,
          change_pct: Number(st?.runtime?.last_price?.price_change || 0),
          active_exchange: String(accountRes?.data?.active_exchange || '').trim().toLowerCase(),
        })
      }
    } catch (e) {
      if (!silent) setError(e?.response?.data?.error || e?.message || '请求失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  const paperTradeRecords = useMemo(
    () => paperRecords.filter((r) => {
      const isApproved = r?.approved === undefined
        ? String(r?.signal || '').toUpperCase() !== 'HOLD'
        : Boolean(r?.approved)
      return isApproved && (!r?.symbol || String(r.symbol).toUpperCase() === String(paperPair || '').toUpperCase())
    }),
    [paperRecords, paperPair],
  )

  const buildPaperConfigPayload = useCallback((override: Record<string, any> = {}) => {
    const modeRaw = String(override?.position_sizing_mode || paperSettings.positionSizingMode || 'margin_pct').trim().toLowerCase()
    const mode = modeRaw === 'contracts' ? 'contracts' : 'margin_pct'
    return {
      symbol: String(override?.symbol || paperPair || 'BTCUSDT').toUpperCase(),
      balance: normalizeDecimal(override?.balance ?? paperMargin, 0, 1_000_000_000),
      position_sizing_mode: mode,
      high_confidence_amount: normalizeDecimal(override?.high_confidence_amount ?? paperSettings.highConfidenceAmount, 0, 1_000_000),
      low_confidence_amount: normalizeDecimal(override?.low_confidence_amount ?? paperSettings.lowConfidenceAmount, 0, 1_000_000),
      high_confidence_margin_pct: normalizeDecimal(override?.high_confidence_margin_pct ?? paperSettings.highConfidenceMarginPct, 0, 100),
      low_confidence_margin_pct: normalizeDecimal(override?.low_confidence_margin_pct ?? paperSettings.lowConfidenceMarginPct, 0, 100),
      leverage: normalizeLeverage(override?.leverage ?? paperSettings.leverage),
      enabled_strategies: parseStrategies(
        override?.enabled_strategies !== undefined
          ? (Array.isArray(override.enabled_strategies) ? override.enabled_strategies : [])
          : paperStrategySelection,
      ).slice(0, 3),
      interval_sec: Math.round(clamp(override?.interval_sec ?? paperRuntime?.interval_sec ?? 8, 2, 300)),
    }
  }, [paperPair, paperMargin, paperSettings, paperStrategySelection, paperRuntime?.interval_sec])

  const applyPaperState = useCallback((payload: Record<string, any>, hydrateConfig = false) => {
    const cfg = payload?.config || {}
    const runtime = payload?.runtime || {}
    setPaperRuntime(runtime)
    setPaperSimRunning(Boolean(runtime?.running))
    setPaperRecords(Array.isArray(payload?.records) ? payload.records : [])
    setPaperLatestDecisionMap(payload?.latest_decision_map && typeof payload.latest_decision_map === 'object'
      ? payload.latest_decision_map
      : {})
    setPaperPnlBaselineMap(payload?.pnl_baseline_map && typeof payload.pnl_baseline_map === 'object'
      ? payload.pnl_baseline_map
      : {})
    setPaperStrategyHistory(Array.isArray(payload?.strategy_history) ? payload.strategy_history : [])

    if (!hydrateConfig && paperSettingsHydratedRef.current) {
      return
    }

    const symbol = String(cfg?.symbol || 'BTCUSDT').toUpperCase()
    const normalizedSettings = normalizeTradeSettings({
      positionSizingMode: String(cfg?.position_sizing_mode || 'margin_pct'),
      highConfidenceAmount: Number(cfg?.high_confidence_amount ?? 0.01),
      lowConfidenceAmount: Number(cfg?.low_confidence_amount ?? 0.005),
      highConfidenceMarginPct: Number(cfg?.high_confidence_margin_pct ?? 5),
      lowConfidenceMarginPct: Number(cfg?.low_confidence_margin_pct ?? 0),
      leverage: Number(cfg?.leverage ?? 20),
    })
    const selection = parseStrategies(Array.isArray(cfg?.enabled_strategies) ? cfg.enabled_strategies : []).slice(0, 3)

    setPaperPair(symbol || 'BTCUSDT')
    setPaperMargin(normalizeDecimal(Number(cfg?.balance ?? 200), 0, 1_000_000_000))
    setPaperSettings(normalizedSettings)
    setPaperStrategySelection(selection)
    setPaperStrategyDraft(selection)
    setPaperStrategy(selection[0] || '')
    paperStrategyManualRef.current = true

    paperConfigHashRef.current = JSON.stringify(buildPaperConfigPayload({
      symbol,
      balance: Number(cfg?.balance ?? 200),
      position_sizing_mode: normalizedSettings.positionSizingMode,
      high_confidence_amount: normalizedSettings.highConfidenceAmount,
      low_confidence_amount: normalizedSettings.lowConfidenceAmount,
      high_confidence_margin_pct: normalizedSettings.highConfidenceMarginPct,
      low_confidence_margin_pct: normalizedSettings.lowConfidenceMarginPct,
      leverage: normalizedSettings.leverage,
      enabled_strategies: selection,
      interval_sec: Number(cfg?.interval_sec ?? runtime?.interval_sec ?? 8),
    }))
    paperSettingsHydratedRef.current = true
  }, [buildPaperConfigPayload])

  const loadPaperState = useCallback(async (silent = true, hydrateConfig = false) => {
    try {
      const res = await getPaperState({ limit: 2000 })
      applyPaperState(res?.data || {}, hydrateConfig)
    } catch (e) {
      if (!silent) {
        const reason = resolveRequestError(e, '加载模拟交易状态失败')
        setError(reason)
        showToast('error', `加载模拟交易状态失败：${reason}`)
      }
    }
  }, [applyPaperState])

  const syncPaperConfigToBackend = useCallback(async (silent = true, force = false) => {
    if (!paperSettingsHydratedRef.current) return
    const payload = buildPaperConfigPayload()
    const hash = JSON.stringify(payload)
    if (!force && hash === paperConfigHashRef.current) return
    try {
      const res = await updatePaperConfig(payload)
      const cfg = res?.data?.config || payload
      paperConfigHashRef.current = JSON.stringify(buildPaperConfigPayload(cfg))
    } catch (e) {
      if (!silent) {
        const reason = resolveRequestError(e, '模拟配置保存失败')
        setError(reason)
        showToast('error', `模拟配置保存失败：${reason}`)
      }
    }
  }, [buildPaperConfigPayload])

  const startPaperSim = async () => {
    if (paperSimLoading) return
    setPaperSimLoading(true)
    setError('')
    try {
      const payload = buildPaperConfigPayload()
      await startPaperSimulation(payload)
      paperConfigHashRef.current = JSON.stringify(payload)
      await loadPaperState(true, false)
      showToast('success', '模拟交易已开始（后台持续运行，关闭浏览器不会中断）')
    } catch (e) {
      const reason = resolveRequestError(e, '模拟启动失败')
      setError(reason)
      showToast('error', `模拟启动失败：${reason}`)
    } finally {
      setPaperSimLoading(false)
    }
  }

  const pausePaperSim = async () => {
    if (paperSimLoading) return
    setPaperSimLoading(true)
    setError('')
    try {
      await stopPaperSimulation()
      await loadPaperState(true, false)
      showToast('warning', '模拟交易已暂停')
    } catch (e) {
      const reason = resolveRequestError(e, '模拟停止失败')
      setError(reason)
      showToast('error', `模拟停止失败：${reason}`)
    } finally {
      setPaperSimLoading(false)
    }
  }

  const resetPaperCurrentPnL = useCallback(async () => {
    const pair = String(paperPair || 'BTCUSDT').toUpperCase()
    try {
      const res = await resetPaperPnL({ symbol: pair })
      setPaperPnlBaselineMap(res?.data?.pnl_baseline_map && typeof res.data.pnl_baseline_map === 'object'
        ? res.data.pnl_baseline_map
        : {})
      showToast('success', `${pair} 当前盈亏已重置`)
    } catch (e) {
      const reason = resolveRequestError(e, '重置当前盈亏失败')
      setError(reason)
      showToast('error', `重置失败：${reason}`)
    }
  }, [paperPair])

  useEffect(() => {
    if (!paperSettingsHydratedRef.current) return
    if (paperConfigSyncTimerRef.current) {
      clearTimeout(paperConfigSyncTimerRef.current)
    }
    paperConfigSyncTimerRef.current = setTimeout(() => {
      void syncPaperConfigToBackend(true, false)
    }, 450)
    return () => {
      if (paperConfigSyncTimerRef.current) {
        clearTimeout(paperConfigSyncTimerRef.current)
        paperConfigSyncTimerRef.current = null
      }
    }
  }, [paperPair, paperMargin, paperSettings, paperStrategySelection, syncPaperConfigToBackend])

  useEffect(() => {
    const interval = paperSimRunning ? 5000 : 15000
    const timer = setInterval(() => {
      void loadPaperState(true, false)
    }, interval)
    return () => clearInterval(timer)
  }, [paperSimRunning, loadPaperState])

  useEffect(() => {
    if (menu !== 'paper') return
    void loadPaperState(true, false)
  }, [menu, loadPaperState])

  useEffect(() => {
    setStrategyDraft(enabledStrategies)
  }, [enabledStrategies])

  useEffect(() => {
    setPaperStrategyDraft(paperStrategySelection)
  }, [paperStrategySelection])

  useEffect(() => {
    setBtStrategyDraft(btStrategySelection)
  }, [btStrategySelection])

  useEffect(() => {
    if (!Object.keys(strategyMetaMap || {}).length) return
    setPaperStrategyHistory((prev) => patchStrategyHistoryMeta(prev, strategyMetaMap))
  }, [strategyMetaMap])

  const closeStrategyPicker = useCallback(() => setStrategyPickerOpen(false), [])
  const closePaperStrategyPicker = useCallback(() => setPaperStrategyPickerOpen(false), [])
  const closeBtStrategyPicker = useCallback(() => setBtStrategyPickerOpen(false), [])

  useCloseOnOutside(strategyPickerOpen, strategyPickerRef, closeStrategyPicker)
  useCloseOnOutside(paperStrategyPickerOpen, paperStrategyPickerRef, closePaperStrategyPicker)
  useCloseOnOutside(btStrategyPickerOpen, btStrategyPickerRef, closeBtStrategyPicker)

  const syncGeneratedStrategiesToBackend = useCallback(async (rules, silent = true) => {
    try {
      const payload = (Array.isArray(rules) ? rules : [])
        .map((row) => toGeneratedStrategyPayload(row))
        .filter((row) => row.name)
      const res = await syncGeneratedStrategies(payload)
      const synced = Array.isArray(res?.data?.strategies)
        ? res.data.strategies
          .map((row) => normalizeGeneratedStrategyItem(row))
          .filter(Boolean)
        : payload
      setGeneratedStrategies(synced)
      return synced
    } catch (e) {
      if (!silent) {
        const reason = e?.response?.data?.error || e?.message || '后端同步失败'
        showToast('warning', `策略已本地更新，但后端同步失败：${reason}`)
      }
      return Array.isArray(rules) ? rules : []
    }
  }, [])

  const loadSystemAndStrategies = async () => {
    const [sysRes, strategyRes, integrationRes, workflowRes, generatedRes] = await Promise.allSettled([
      getSystemSettings(),
      getStrategies(),
      getIntegrations(),
      getSkillWorkflow(),
      getGeneratedStrategies(),
    ])

    let merged = mergeSystemDefaults(systemSettings)
    if (sysRes.status === 'fulfilled') {
      merged = mergeSystemDefaults(sysRes.value?.data?.settings || {})
    }
    setSystemSettings(merged)

    let generatedNamesLoaded = []
    if (generatedRes.status === 'fulfilled') {
      const generated = Array.isArray(generatedRes.value?.data?.strategies)
        ? generatedRes.value.data.strategies
          .map((row) => normalizeGeneratedStrategyItem(row))
          .filter(Boolean)
        : []
      setGeneratedStrategies(generated)
      generatedNamesLoaded = generated.map((x) => String(x?.name || '').trim()).filter(Boolean)
      if (generated.length) {
        const generatedMeta = {}
        for (const item of generated) {
          const name = String(item?.name || '').trim()
          if (!name) continue
          generatedMeta[name] = normalizeStrategyMetaByName(name, {
            source: item?.source || 'workflow_generated',
            workflow_version: item?.workflowVersion || 'skill-workflow/v1',
            workflow_chain: item?.workflowChain || [],
            last_updated_at: item?.updatedAt || item?.createdAt || '',
          })
        }
        setStrategyMetaMap((old) => mergeStrategyMetaMap(old, generatedMeta))
      }
    }

    if (strategyRes.status === 'fulfilled') {
      const available = parseStrategies(strategyRes.value?.data?.available)
      const enabled = parseStrategies(strategyRes.value?.data?.enabled)
      const byEnv = executionStrategiesFromSettings(merged)
      const mergedSet = Array.from(new Set([...available, ...enabled, ...byEnv]))
      const mergedExecution = Array.from(new Set([...mergedSet, ...generatedNamesLoaded, ...generatedStrategyNames]))
      const enabledMerged = Array.from(new Set([...enabled, ...byEnv])).filter((x) => mergedExecution.includes(x))
      setStrategyOptions(mergedSet)
      if (mergedExecution.length) {
        if (!mergedExecution.includes(activeStrategy)) setActiveStrategy(mergedExecution[0])
        if (!mergedExecution.includes(paperStrategy)) setPaperStrategy(mergedExecution[0])
        if (!btStrategy || !mergedExecution.includes(btStrategy)) setBtStrategy(mergedExecution[0])
        setPaperStrategySelection((prev) => {
          const normalized = prev.filter((x) => mergedExecution.includes(x)).slice(0, 3)
          return normalized.length ? normalized : [mergedExecution[0]]
        })
        setBtStrategySelection((prev) => {
          const normalized = prev.filter((x) => mergedExecution.includes(x)).slice(0, 3)
          if (normalized.length) return normalized
          if (btStrategy && mergedExecution.includes(btStrategy)) return [btStrategy]
          return [mergedExecution[0]]
        })
        setEnabledStrategies((enabledMerged.length ? enabledMerged : [mergedExecution[0]]).slice(0, 3))
      } else {
        setEnabledStrategies([])
        setStrategyDraft([])
        setActiveStrategy('')
        setPaperStrategy('')
        setPaperStrategySelection([])
        setPaperStrategyDraft([])
        setBtStrategy('')
        setBtStrategySelection([])
        setBtStrategyDraft([])
      }
    }
    if (integrationRes.status === 'fulfilled') {
      const data = integrationRes.value?.data || {}
      const llms = Array.isArray(data.llms) ? data.llms : []
      const catalog = Array.isArray(data.llm_product_catalog) && data.llm_product_catalog.length
        ? data.llm_product_catalog
        : DEFAULT_LLM_PRODUCT_CATALOG
      setLlmProductCatalog(catalog)
      setNewLLM((prev) => {
        const currentProduct = String(prev?.product || '')
        const matched = catalog.find((x) => String(x?.product || '') === currentProduct)
        if (matched) {
          return { ...prev, product: String(matched.product || 'chatgpt'), base_url: String(matched.base_url || '') }
        }
        const first = catalog[0] || DEFAULT_LLM_PRODUCT_CATALOG[0]
        return {
          ...prev,
          product: String(first?.product || 'chatgpt'),
          base_url: String(first?.base_url || ''),
          model: '',
        }
      })
      setLlmConfigs(llms)
      setLlmStatusMap((prev) => {
        const next = {}
        for (const row of llms) {
          const id = String(row?.id || '').trim()
          if (!id) continue
          next[id] = prev[id] || { state: 'unknown', message: '未检测' }
        }
        return next
      })
      setExchangeConfigs(Array.isArray(data.exchanges) ? data.exchanges : [])
      setActiveExchangeId(String(data.active_exchange_id || ''))
      setExchangeBound(Boolean(data.exchange_bound))
    }
    if (workflowRes.status === 'fulfilled') {
      setSkillWorkflow(normalizeSkillWorkflowState(workflowRes.value?.data?.workflow || DEFAULT_SKILL_WORKFLOW))
    } else {
      setSkillWorkflow((prev) => normalizeSkillWorkflowState(prev))
    }
  }

  useEffect(() => {
    refreshCore(false)
    loadSystemAndStrategies()
    void loadPaperState(true, true)
    const timer = setInterval(() => refreshCore(true), 15000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!toast.visible) return undefined
    const timer = setTimeout(() => {
      setToast((t) => ({ ...t, visible: false }))
    }, 2600)
    return () => clearTimeout(timer)
  }, [toast.visible, toast.message, toast.type])

  const showToast = (type, message) => {
    setToast({ visible: true, type, message: String(message || '') })
  }

  const normalizeSkillWorkflowState = (raw) => {
    const base = raw && typeof raw === 'object' ? raw : {}
    const stepsIn = Array.isArray(base.steps) ? base.steps : DEFAULT_SKILL_WORKFLOW.steps
    const defaultsByID = {}
    for (const s of DEFAULT_SKILL_WORKFLOW.steps) defaultsByID[s.id] = s
    const steps = DEFAULT_SKILL_WORKFLOW.steps.map((d) => {
      const m = stepsIn.find((x) => String(x?.id || '') === d.id) || {}
      return {
        ...d,
        enabled: Boolean(m?.enabled ?? d.enabled),
        timeout_sec: Math.max(1, Math.min(300, Number(m?.timeout_sec ?? d.timeout_sec) || d.timeout_sec)),
        max_retry: Math.max(0, Math.min(5, Number(m?.max_retry ?? d.max_retry) || d.max_retry)),
        on_fail: String(m?.on_fail || d.on_fail).toLowerCase() === 'hard_fail' ? 'hard_fail' : 'hold',
        name: localizeWorkflowStepName(m?.name, d.id, defaultsByID[d.id]?.name || d.name),
        description: String(m?.description || defaultsByID[d.id]?.description || d.description),
      }
    })
    const constraintsIn = base.constraints && typeof base.constraints === 'object'
      ? base.constraints
      : DEFAULT_SKILL_WORKFLOW.constraints
    const promptsIn = base.prompts && typeof base.prompts === 'object'
      ? base.prompts
      : DEFAULT_SKILL_WORKFLOW.prompts
    const requirements = Array.isArray(promptsIn.strategy_generator_requirements)
      ? promptsIn.strategy_generator_requirements
      : []
    const normalizedRequirements = requirements
      .map((item) => String(item || '').trim())
      .filter(Boolean)
    return {
      version: String(base.version || DEFAULT_SKILL_WORKFLOW.version),
      updated_at: String(base.updated_at || ''),
      steps,
      constraints: {
        max_leverage_cap: Math.max(1, Math.min(150, Number(constraintsIn.max_leverage_cap) || DEFAULT_SKILL_WORKFLOW.constraints.max_leverage_cap)),
        max_drawdown_cap_pct: Math.max(0.01, Math.min(0.8, Number(constraintsIn.max_drawdown_cap_pct) || DEFAULT_SKILL_WORKFLOW.constraints.max_drawdown_cap_pct)),
        max_risk_per_trade_cap_pct: Math.max(0.001, Math.min(0.2, Number(constraintsIn.max_risk_per_trade_cap_pct) || DEFAULT_SKILL_WORKFLOW.constraints.max_risk_per_trade_cap_pct)),
        min_profit_loss_floor: Math.max(1, Math.min(10, Number(constraintsIn.min_profit_loss_floor) || DEFAULT_SKILL_WORKFLOW.constraints.min_profit_loss_floor)),
        block_trade_on_skill_fail: Boolean(
          constraintsIn.block_trade_on_skill_fail ?? DEFAULT_SKILL_WORKFLOW.constraints.block_trade_on_skill_fail,
        ),
      },
      prompts: {
        strategy_generator_system_prompt: String(
          promptsIn.strategy_generator_system_prompt || DEFAULT_SKILL_WORKFLOW.prompts.strategy_generator_system_prompt,
        ),
        strategy_generator_task_prompt: String(
          promptsIn.strategy_generator_task_prompt || DEFAULT_SKILL_WORKFLOW.prompts.strategy_generator_task_prompt,
        ),
        strategy_generator_requirements: normalizedRequirements.length
          ? normalizedRequirements
          : [...DEFAULT_SKILL_WORKFLOW.prompts.strategy_generator_requirements],
        decision_system_prompt: String(
          promptsIn.decision_system_prompt || DEFAULT_SKILL_WORKFLOW.prompts.decision_system_prompt,
        ),
        decision_policy_prompt: String(
          promptsIn.decision_policy_prompt || DEFAULT_SKILL_WORKFLOW.prompts.decision_policy_prompt,
        ),
      },
    }
  }

  const loadSkillWorkflowConfig = async (silent = false) => {
    if (!silent) setLoadingSkillWorkflow(true)
    try {
      const res = await getSkillWorkflow()
      const incoming = res?.data?.workflow || DEFAULT_SKILL_WORKFLOW
      setSkillWorkflow(normalizeSkillWorkflowState(incoming))
    } catch (e) {
      if (!silent) {
        const reason = e?.response?.data?.error || e?.message || '加载失败'
        setError(reason)
        showToast('error', `加载 AI 工作流失败：${reason}`)
      }
      setSkillWorkflow((prev) => normalizeSkillWorkflowState(prev))
    } finally {
      if (!silent) setLoadingSkillWorkflow(false)
    }
  }

  const loadAIWorkflowLogs = async (silent = false) => {
    if (!silent) setAiWorkflowLogsLoading(true)
    try {
      const params = {
        limit: Math.max(1, Math.min(500, Number(aiWorkflowLogLimit) || 50)),
        channel: aiWorkflowLogChannel === 'all' ? '' : aiWorkflowLogChannel,
      }
      const res = await getLLMUsageLogs(params)
      const rows = Array.isArray(res?.data?.logs) ? res.data.logs : []
      setAiWorkflowLogs(rows)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '加载失败'
      if (!silent) {
        setError(reason)
        showToast('error', `加载执行记录失败：${reason}`)
      }
    } finally {
      if (!silent) setAiWorkflowLogsLoading(false)
    }
  }

  const updateSkillStepField = (id, key, value) => {
    const stepID = String(id || '').trim()
    if (!stepID) return
    setSkillWorkflow((prev) => {
      const next = normalizeSkillWorkflowState(prev)
      next.steps = next.steps.map((st) => {
        if (String(st.id) !== stepID) return st
        if (key === 'enabled') return { ...st, enabled: Boolean(value) }
        if (key === 'timeout_sec') return { ...st, timeout_sec: Math.max(1, Math.min(300, Number(value) || st.timeout_sec)) }
        if (key === 'max_retry') return { ...st, max_retry: Math.max(0, Math.min(5, Number(value) || st.max_retry)) }
        if (key === 'on_fail') return { ...st, on_fail: String(value || st.on_fail).toLowerCase() === 'hard_fail' ? 'hard_fail' : 'hold' }
        return st
      })
      return next
    })
  }

  const updateSkillConstraintField = (key, value) => {
    setSkillWorkflow((prev) => {
      const next = normalizeSkillWorkflowState(prev)
      const c = { ...next.constraints }
      if (key === 'block_trade_on_skill_fail') {
        c.block_trade_on_skill_fail = Boolean(value)
      } else if (key === 'max_leverage_cap') {
        c.max_leverage_cap = Math.max(1, Math.min(150, Number(value) || c.max_leverage_cap))
      } else if (key === 'max_drawdown_cap_pct') {
        c.max_drawdown_cap_pct = Math.max(0.01, Math.min(0.8, Number(value) || c.max_drawdown_cap_pct))
      } else if (key === 'max_risk_per_trade_cap_pct') {
        c.max_risk_per_trade_cap_pct = Math.max(0.001, Math.min(0.2, Number(value) || c.max_risk_per_trade_cap_pct))
      } else if (key === 'min_profit_loss_floor') {
        c.min_profit_loss_floor = Math.max(1, Math.min(10, Number(value) || c.min_profit_loss_floor))
      }
      return { ...next, constraints: c }
    })
  }

  const updateSkillPromptField = (key, value) => {
    setSkillWorkflow((prev) => {
      const next = normalizeSkillWorkflowState(prev)
      const prompts = { ...next.prompts }
      if (key === 'strategy_generator_requirements') {
        prompts.strategy_generator_requirements = String(value || '')
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean)
      } else if (key === 'strategy_generator_system_prompt') {
        prompts.strategy_generator_system_prompt = String(value || '')
      } else if (key === 'strategy_generator_task_prompt') {
        prompts.strategy_generator_task_prompt = String(value || '')
      } else if (key === 'decision_system_prompt') {
        prompts.decision_system_prompt = String(value || '')
      } else if (key === 'decision_policy_prompt') {
        prompts.decision_policy_prompt = String(value || '')
      }
      return { ...next, prompts }
    })
  }

  const saveSkillWorkflowConfig = async () => {
    setSavingSkillWorkflow(true)
    setError('')
    try {
      const payload = normalizeSkillWorkflowState(skillWorkflow)
      const res = await saveSkillWorkflow(payload)
      const next = normalizeSkillWorkflowState(res?.data?.workflow || payload)
      setSkillWorkflow(next)
      showToast('success', 'AI 工作流保存成功')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '保存失败'
      setError(reason)
      showToast('error', `AI 工作流保存失败：${reason}`)
    } finally {
      setSavingSkillWorkflow(false)
    }
  }

  const resetSkillWorkflowConfig = async () => {
    setSavingSkillWorkflow(true)
    setError('')
    try {
      const res = await resetSkillWorkflow()
      const next = normalizeSkillWorkflowState(res?.data?.workflow || DEFAULT_SKILL_WORKFLOW)
      setSkillWorkflow(next)
      showToast('success', 'AI 工作流已恢复默认')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '恢复默认失败'
      setError(reason)
      showToast('error', `恢复失败：${reason}`)
    } finally {
      setSavingSkillWorkflow(false)
    }
  }

  const runWorkflowUpgradeNow = async () => {
    setRunningWorkflowUpgradeNow(true)
    setError('')
    try {
      const res = await runAutoStrategyRegenNow({ force: false })
      const upgraded = Boolean(res?.data?.upgraded)
      const message = String(res?.data?.message || '').trim()
      const strategyName = String(res?.data?.strategy_name || '').trim()
      if (upgraded) {
        showToast('success', strategyName ? `策略升级完成：${strategyName}` : '策略升级完成')
      } else {
        showToast('warning', message || '当前未触发升级条件，无需升级')
      }
      await Promise.allSettled([
        refreshCore(true),
        loadSystemAndStrategies(),
        loadAIWorkflowLogs(true),
      ])
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '立刻升级失败'
      setError(reason)
      showToast('error', `立刻升级失败：${reason}`)
    } finally {
      setRunningWorkflowUpgradeNow(false)
    }
  }

  const resetLLMModalDraft = () => {
    const firstProduct = llmProductCatalog[0] || DEFAULT_LLM_PRODUCT_CATALOG[0]
    setNewLLM({
      name: '',
      product: String(firstProduct?.product || 'chatgpt'),
      base_url: String(firstProduct?.base_url || 'https://api.openai.com/v1'),
      api_key: '',
      model: '',
    })
    setLlmModelOptions([])
    setProbingLLMModels(false)
    setLlmProbeMessage('')
    llmProbeKeyRef.current = ''
  }

  const probeLLMModelOptions = async (input: Record<string, any> = {}) => {
    const normalizedProduct = String(
      input?.product || selectedLLMPreset?.product || newLLM?.product || 'chatgpt',
    ).trim().toLowerCase() || 'chatgpt'
    const normalizedBaseURL = String(
      input?.base_url || selectedLLMPreset?.base_url || newLLM?.base_url || '',
    ).trim()
    const normalizedAPIKey = String(input?.api_key || newLLM?.api_key || '').trim()
    if (!normalizedBaseURL || !normalizedAPIKey) {
      showToast('warning', '请先选择智能体产品并填写 API Key')
      return
    }
    const probeKey = `${normalizedProduct}|${normalizedBaseURL}|${normalizedAPIKey}`
    if (probeKey === llmProbeKeyRef.current) {
      return
    }
    const currentSeq = llmProbeSeqRef.current + 1
    llmProbeSeqRef.current = currentSeq
    setProbingLLMModels(true)
    setLlmProbeMessage('正在检测 API 路由可达性...')
    try {
      const res = await probeLLMModels({
        product: normalizedProduct,
        base_url: normalizedBaseURL,
        api_key: normalizedAPIKey,
      })
      if (llmProbeSeqRef.current !== currentSeq) return
      const routeReachable = Boolean(res?.data?.route_reachable)
      if (!routeReachable) {
        throw new Error(String(res?.data?.message || '模型路由不可达'))
      }
      setLlmProbeMessage('路由可达，正在加载可用模型...')
      const rawModels = Array.isArray(res?.data?.models) ? res.data.models : []
      const models = rawModels
        .map((m) => String(m || '').trim())
        .filter(Boolean)
      if (!models.length) {
        throw new Error(String(res?.data?.message || '未获取到可用模型'))
      }
      setLlmModelOptions(models)
      setNewLLM((prev) => {
        const current = String(prev?.model || '').trim()
        return {
          ...prev,
          product: normalizedProduct,
          base_url: normalizedBaseURL,
          model: models.includes(current) ? current : models[0],
        }
      })
      llmProbeKeyRef.current = probeKey
      showToast('success', `模型检测完成，找到 ${models.length} 个可用模型`)
    } catch (e) {
      if (llmProbeSeqRef.current !== currentSeq) return
      const reason = resolveRequestError(e, '模型检测失败')
      setLlmModelOptions([])
      setNewLLM((prev) => ({ ...prev, model: '' }))
      llmProbeKeyRef.current = ''
      setError(reason)
      showToast('error', `模型检测失败：${reason}`)
    } finally {
      if (llmProbeSeqRef.current === currentSeq) {
        setProbingLLMModels(false)
        setLlmProbeMessage('')
      }
    }
  }

  useEffect(() => {
    if (!window.matchMedia) return undefined
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (e) => setPrefersDark(Boolean(e.matches))
    if (media.addEventListener) media.addEventListener('change', onChange)
    else media.addListener(onChange)
    return () => {
      if (media.removeEventListener) media.removeEventListener('change', onChange)
      else media.removeListener(onChange)
    }
  }, [])

  useEffect(() => {
    localStorage.setItem('ui-theme-mode', themeMode)
  }, [themeMode])

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', resolvedTheme)
  }, [resolvedTheme])

  useEffect(() => {
    if (!showLLMModal) return undefined
    const product = String(selectedLLMPreset?.product || newLLM?.product || 'chatgpt').trim().toLowerCase() || 'chatgpt'
    const baseURL = String(selectedLLMPreset?.base_url || newLLM?.base_url || '').trim()
    const apiKey = String(newLLM?.api_key || '').trim()
    if (!baseURL || !apiKey) {
      llmProbeKeyRef.current = ''
      setLlmModelOptions([])
      setNewLLM((prev) => {
        if (!String(prev?.model || '').trim()) return prev
        return { ...prev, model: '' }
      })
      return undefined
    }
    if (!/^https?:\/\//i.test(baseURL) || apiKey.length < 16) {
      return undefined
    }

    if (llmProbeTimerRef.current) {
      clearTimeout(llmProbeTimerRef.current)
    }
    llmProbeTimerRef.current = setTimeout(() => {
      probeLLMModelOptions({ product, base_url: baseURL, api_key: apiKey })
    }, 650)
    return () => {
      if (llmProbeTimerRef.current) {
        clearTimeout(llmProbeTimerRef.current)
        llmProbeTimerRef.current = null
      }
    }
  }, [showLLMModal, newLLM?.product, newLLM?.api_key, selectedLLMPreset?.product, selectedLLMPreset?.base_url])

  const loadSystemRuntime = async (silent = false) => {
    if (!silent) setLoadingSystemRuntime(true)
    try {
      const res = await getSystemRuntimeStatus()
      setSystemRuntime(res?.data || null)
      setBackendReachability({
        status: 'connected',
        message: '后端 API 可达',
        checkedAt: new Date().toISOString(),
      })
    } catch (e) {
      const reason = resolveRequestError(e, '加载系统状态失败')
      setBackendReachability({
        status: 'warning',
        message: `后端不可达：${reason}`,
        checkedAt: new Date().toISOString(),
      })
      if (!silent) setError(reason)
    } finally {
      if (!silent) setLoadingSystemRuntime(false)
    }
  }

  const restartBackend = async () => {
    setRestartingBackend(true)
    setError('')
    try {
      await restartSystemRuntime()
      await sleep(200)
      const settled = await Promise.allSettled([refreshCore(true), loadSystemAndStrategies(), loadSystemRuntime(true)])
      const failed = settled.find((item) => item.status === 'rejected')
      if (failed && failed.status === 'rejected') {
        const reason = resolveRequestError(failed.reason, '状态刷新失败')
        setError(`后台已重启，但状态刷新失败：${reason}`)
        showToast('warning', `后台已重启，但状态刷新失败：${reason}`)
      } else {
        showToast('success', '后台软重启完成')
      }
    } catch (e) {
      const reason = resolveRequestError(e, '后台重启失败')
      setError(reason)
      showToast('error', `后台重启失败：${reason}`)
    } finally {
      setRestartingBackend(false)
    }
  }

  const loadAssetOverview = async () => {
    try {
      const res = await getAssetOverview()
      setAssetOverview(res?.data?.overview || {})
    } catch {
      // keep previous data
    }
  }

  const loadAssetTrend = async (range) => {
    try {
      const res = await getAssetTrend(range)
      setAssetTrend(Array.isArray(res?.data?.points) ? res.data.points : [])
    } catch {
      setAssetTrend([])
    }
  }

  const loadAssetCalendar = async (month) => {
    try {
      const res = await getAssetPnLCalendar(month)
      setAssetCalendar(Array.isArray(res?.data?.days) ? res.data.days : [])
    } catch {
      setAssetCalendar([])
    }
  }

  const loadAssetDistribution = async () => {
    try {
      const res = await getAssetDistribution()
      setAssetDistribution(Array.isArray(res?.data?.items) ? res.data.items : [])
    } catch {
      setAssetDistribution([])
    }
  }

  const loadBacktestHistory = async (silent = false) => {
    if (!silent) setBtHistoryLoading(true)
    try {
      const res = await getBacktestHistory(120)
      setBtHistory(Array.isArray(res?.data?.runs) ? res.data.runs : [])
    } catch {
      if (!silent) setBtHistory([])
    } finally {
      if (!silent) setBtHistoryLoading(false)
    }
  }

  useEffect(() => {
    if (menu !== 'assets') return
    loadAssetOverview()
    loadAssetTrend(assetRange)
    loadAssetCalendar(assetMonth)
    loadAssetDistribution()
  }, [menu])

  useEffect(() => {
    if (menu !== 'assets') return
    loadAssetTrend(assetRange)
  }, [assetRange, menu])

  useEffect(() => {
    if (menu !== 'assets') return
    loadAssetCalendar(assetMonth)
  }, [assetMonth, menu])

  useEffect(() => {
    if (menu !== 'backtest') return
    loadBacktestHistory(false)
  }, [menu])

  useEffect(() => {
    if (menu !== 'system' || systemSubTab !== 'status') return undefined
    loadSystemRuntime(false)
    const timer = setInterval(() => loadSystemRuntime(true), 10000)
    return () => clearInterval(timer)
  }, [menu, systemSubTab])

  useEffect(() => {
    if (menu !== 'skill_workflow' || aiWorkflowTab !== 'logs') return undefined
    loadAIWorkflowLogs(false)
    const timer = setInterval(() => loadAIWorkflowLogs(true), 15000)
    return () => clearInterval(timer)
  }, [menu, aiWorkflowTab, aiWorkflowLogChannel, aiWorkflowLogLimit])

  const marketEmotion = useMemo(() => {
    const confidence = String(status?.runtime?.last_signal?.confidence || 'LOW').toUpperCase()
    const change = Number(status?.runtime?.last_price?.price_change || 0)
    if (confidence === 'HIGH' && change >= 0) return '偏多'
    if (confidence === 'HIGH' && change < 0) return '偏空'
    if (Math.abs(change) < 0.15) return '中性'
    return change > 0 ? '温和偏多' : '温和偏空'
  }, [status])

  const totalPnL = Number(account?.position?.unrealized_pnl || 0)

  const pnlRatio = useMemo(() => {
    const wins = strategyScores.reduce((a, b) => a + Number(b?.wins || 0), 0)
    const losses = strategyScores.reduce((a, b) => a + Number(b?.losses || 0), 0)
    if (losses === 0) return wins > 0 ? '∞' : '0'
    return (wins / losses).toFixed(2)
  }, [strategyScores])

  const strategyDurationText = useMemo(() => {
    const startedAtRaw = String(status?.live_strategy_started_at || '').trim()
    const startedAtMs = Date.parse(startedAtRaw)
    if (!Number.isFinite(startedAtMs) || startedAtMs <= 0) {
      return '0m'
    }
    const diff = Math.max(0, Date.now() - startedAtMs)
    const mins = Math.floor(diff / 60000)
    const hours = Math.floor(mins / 60)
    const rem = mins % 60
    if (hours > 0) return `${hours}h ${rem}m`
    return `${mins}m`
  }, [status?.live_strategy_started_at, status?.runtime?.last_run_at])

  const persistLiveConfigSettings = async () => {
    const normalizedSettings = normalizeTradeSettings(settings)
    setSettings((old) => ({ ...old, ...normalizedSettings }))
    await updateSettings({
      symbol: String(activePair || 'BTCUSDT').toUpperCase(),
      position_sizing_mode: String(normalizedSettings.positionSizingMode || 'contracts'),
      high_confidence_amount: normalizedSettings.highConfidenceAmount,
      low_confidence_amount: normalizedSettings.lowConfidenceAmount,
      high_confidence_margin_pct: normalizedSettings.highConfidenceMarginPct / 100,
      low_confidence_margin_pct: normalizedSettings.lowConfidenceMarginPct / 100,
      leverage: normalizedSettings.leverage,
    })

    const normalized = enabledStrategies.filter((x) => executionStrategyOptions.includes(x))
    const fallbackStrategy = executionStrategyOptions.includes(activeStrategy)
      ? activeStrategy
      : (executionStrategyOptions[0] || '')
    const withFallback = normalized.length
      ? normalized
      : (fallbackStrategy ? [fallbackStrategy] : [])
    if (fallbackStrategy && !withFallback.includes(fallbackStrategy)) withFallback.push(fallbackStrategy)
    const nextEnabled = Array.from(new Set(withFallback)).slice(0, 3)
    const nextEnabledValue = nextEnabled.join(',')
    const res = await saveSystemSettings({ AI_EXECUTION_STRATEGIES: nextEnabledValue })
    const settingsFromServer = res?.data?.settings
    if (settingsFromServer && typeof settingsFromServer === 'object') {
      setSystemSettings((old) => ({ ...(old || {}), ...settingsFromServer }))
    } else {
      setSystemSettings((old) => ({ ...(old || {}), AI_EXECUTION_STRATEGIES: nextEnabledValue }))
    }
    liveSettingsDirtyRef.current = false
    activePairHydratedRef.current = true
    activePairUserOverrideRef.current = false
  }

  const saveLiveConfig = async () => {
    setSavingSettings(true)
    setError('')
    try {
      await persistLiveConfigSettings()
      await refreshCore(false)
      showToast('success', '实盘配置保存成功')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '保存失败'
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingSettings(false)
    }
  }

  const startLiveTrading = async () => {
    setStartingLive(true)
    setError('')
    try {
      await persistLiveConfigSettings()
      await startScheduler()
      await runNow()
      await refreshCore(false)
      showToast('success', '实盘交易已开始，已按当前参数执行')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '启动失败'
      setError(reason)
      showToast('error', `实盘启动失败：${reason}`)
    } finally {
      setStartingLive(false)
    }
  }

  const saveSystemEnv = async () => {
    setSavingSystemSettings(true)
    setSystemSaveHint('')
    setError('')
    try {
      const payload = {}
      for (const f of envFieldDefs) payload[f.key] = String(systemSettings?.[f.key] || '')
      const res = await saveSystemSettings(payload)
      setSystemSettings(mergeSystemDefaults(res?.data?.settings || payload))
      setSystemSaveHint(`已保存 ${new Date().toLocaleTimeString()}`)
      const warnMsg = joinFieldMessages(res?.data?.warnings)
      if (warnMsg) {
        showToast('warning', `系统设置已保存，但存在告警：${warnMsg}`)
      } else {
        showToast('success', '系统设置保存成功')
      }
    } catch (e) {
      const base = e?.response?.data?.error || e?.message || '系统设置保存失败'
      const detail = joinFieldMessages(e?.response?.data?.field_errors)
      const reason = detail ? `${base}：${detail}` : base
      setSystemSaveHint('')
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingSystemSettings(false)
    }
  }

  const persistEnabledStrategiesEnv = async (nextEnabled = []) => {
    const normalized = Array.from(new Set(
      (Array.isArray(nextEnabled) ? nextEnabled : [])
        .map((x) => String(x || '').trim())
        .filter(Boolean),
    )).slice(0, 3)
    const payload = { AI_EXECUTION_STRATEGIES: normalized.join(',') }
    const res = await saveSystemSettings(payload)
    setSystemSettings((prev) => mergeSystemDefaults(res?.data?.settings || { ...prev, ...payload }))
    return normalized
  }

  const saveAutoReviewEnv = async () => {
    setSavingAutoReviewSettings(true)
    setAutoReviewSaveHint('')
    setError('')
    try {
      const payload = {}
      for (const key of AUTO_REVIEW_ENV_KEYS) {
        payload[key] = String(systemSettings?.[key] || '')
      }
      const res = await saveSystemSettings(payload)
      setSystemSettings(mergeSystemDefaults(res?.data?.settings || { ...systemSettings, ...payload }))
      setAutoReviewSaveHint(`已保存 ${new Date().toLocaleTimeString()}`)
      const warnMsg = joinFieldMessages(res?.data?.warnings)
      if (warnMsg) {
        showToast('warning', `自动评估参数已保存，但存在告警：${warnMsg}`)
      } else {
        showToast('success', '自动评估参数保存成功')
      }
      await refreshCore(true)
    } catch (e) {
      const base = e?.response?.data?.error || e?.message || '自动评估参数保存失败'
      const detail = joinFieldMessages(e?.response?.data?.field_errors)
      const reason = detail ? `${base}：${detail}` : base
      setAutoReviewSaveHint('')
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingAutoReviewSettings(false)
    }
  }

  const saveCoreRiskSettings = async () => {
    setSavingCoreRiskSettings(true)
    setCoreRiskSaveHint('')
    setError('')
    try {
      const normalized = {
        maxRiskPerTradePct: normalizeDecimal(Number(coreRiskSettings?.maxRiskPerTradePct || 0), 0.01, 100),
        maxPositionPct: normalizeDecimal(Number(coreRiskSettings?.maxPositionPct || 0), 0.01, 100),
        maxConsecutiveLosses: Math.max(0, Math.round(Number(coreRiskSettings?.maxConsecutiveLosses || 0))),
        maxDailyLossPct: normalizeDecimal(Number(coreRiskSettings?.maxDailyLossPct || 0), 0.01, 100),
        maxDrawdownPct: normalizeDecimal(Number(coreRiskSettings?.maxDrawdownPct || 0), 0.01, 100),
        liquidationBufferPct: normalizeDecimal(Number(coreRiskSettings?.liquidationBufferPct || 0), 0.01, 100),
      }
      setCoreRiskSettings(normalized)
      await updateSettings({
        max_risk_per_trade_pct: normalized.maxRiskPerTradePct / 100,
        max_position_pct: normalized.maxPositionPct / 100,
        max_consecutive_losses: normalized.maxConsecutiveLosses,
        max_daily_loss_pct: normalized.maxDailyLossPct / 100,
        max_drawdown_pct: normalized.maxDrawdownPct / 100,
        liquidation_buffer_pct: normalized.liquidationBufferPct / 100,
      })
      coreRiskDirtyRef.current = false
      setCoreRiskSaveHint(`已保存 ${new Date().toLocaleTimeString()}`)
      showToast('success', '核心风控参数保存成功')
      await refreshCore(true)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '核心风控参数保存失败'
      setCoreRiskSaveHint('')
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingCoreRiskSettings(false)
    }
  }

  const resetRiskManually = async () => {
    const confirmed = window.prompt('这是危险操作。输入 RESET 确认手动解除风控：', '')
    if (String(confirmed || '').trim().toUpperCase() !== 'RESET') {
      return
    }
    setResettingRiskBaseline(true)
    setError('')
    try {
      const reason = `manual_ui_reset_${Date.now()}`
      const res = await resetRiskBaseline({ reason })
      const at = String(res?.data?.reset_at || '').trim()
      showToast('success', at ? `已解除风控基线（${fmtTime(at)}）` : '已解除风控基线')
      await refreshCore(true)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '解除失败'
      setError(reason)
      showToast('error', `解除失败：${reason}`)
    } finally {
      setResettingRiskBaseline(false)
    }
  }

  const handleAddLLM = async () => {
    setAddingLLM(true)
    setError('')
    try {
      const selectedProduct = llmProductCatalog.find(
        (p) => String(p?.product || '') === String(newLLM?.product || ''),
      ) || llmProductCatalog[0] || DEFAULT_LLM_PRODUCT_CATALOG[0]
      const selectedModel = String(newLLM.model || '').trim()
      if (!selectedModel) {
        throw new Error('请先填写 API Key 并选择可用模型')
      }
      const payload = {
        id: String(editingLLMId || '').trim(),
        name: String(newLLM.name || '').trim(),
        product: String(selectedProduct.product || 'chatgpt').trim().toLowerCase() || 'chatgpt',
        base_url: String(selectedProduct.base_url || '').trim(),
        api_key: String(newLLM.api_key || '').trim(),
        model: selectedModel,
      }
      const res = editingLLMId ? await updateLLMIntegration(payload) : await addLLMIntegration(payload)
      const llms = Array.isArray(res?.data?.llms) ? res.data.llms : []
      const savedID = String(editingLLMId || res?.data?.added?.id || res?.data?.updated?.id || '').trim()
      setLlmConfigs(llms)
      setLlmStatusMap((prev) => {
        const next = {}
        for (const row of llms) {
          const id = String(row?.id || '').trim()
          if (!id) continue
          next[id] = prev[id] || { state: 'unknown', message: '未检测' }
        }
        if (savedID) {
          next[savedID] = { state: 'reachable', message: 'API 可达' }
        }
        return next
      })
      resetLLMModalDraft()
      setEditingLLMId('')
      setShowLLMModal(false)
      if (editingLLMId) {
        showToast('success', `智能体参数更新并验证成功（ID=${payload.id || '-'})`)
      } else {
        showToast('success', `智能体参数添加并验证成功（ID=${res?.data?.added?.id || '-'})`)
      }
    } catch (e) {
      const reason = resolveRequestError(e, editingLLMId ? '智能体参数更新失败' : '智能体参数添加失败')
      setError(reason)
      showToast('error', `${editingLLMId ? '更新失败' : '添加失败'}：${reason}`)
    } finally {
      setAddingLLM(false)
    }
  }

  const openEditLLMModal = (row) => {
    const id = String(row?.id || '').trim()
    if (!id) return
    const normalizedProduct = String(row?.product || '').trim().toLowerCase() || 'chatgpt'
    const matchedProduct = llmProductCatalog.find((p) => p.product === normalizedProduct) || llmProductCatalog[0] || DEFAULT_LLM_PRODUCT_CATALOG[0]
    setEditingLLMId(id)
    setNewLLM({
      name: String(row?.name || '').trim(),
      product: String(matchedProduct?.product || 'chatgpt').trim().toLowerCase() || 'chatgpt',
      base_url: String(matchedProduct?.base_url || row?.base_url || '').trim(),
      api_key: String(row?.api_key || '').trim(),
      model: String(row?.model || '').trim(),
    })
    const existingModel = String(row?.model || '').trim()
    setLlmModelOptions(existingModel ? [existingModel] : [])
    llmProbeKeyRef.current = ''
    setShowLLMModal(true)
  }

  const removeLLMConfig = async (id) => {
    const llmID = String(id || '').trim()
    if (!llmID) return
    if (!window.confirm(`确认删除智能体参数 ID=${llmID} 吗？`)) return
    setDeletingLLMId(llmID)
    setError('')
    try {
      const res = await deleteLLMIntegration(llmID)
      setLlmConfigs(Array.isArray(res?.data?.llms) ? res.data.llms : [])
      setLlmStatusMap((prev) => {
        const next = { ...prev }
        delete next[llmID]
        return next
      })
      if (editingLLMId === llmID) {
        setEditingLLMId('')
        setShowLLMModal(false)
        resetLLMModalDraft()
      }
      showToast('success', `智能体参数已删除（ID=${llmID}）`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '删除失败'
      setError(reason)
      showToast('error', `删除失败：${reason}`)
    } finally {
      setDeletingLLMId('')
    }
  }

  const testLLMConfigReachability = async (id) => {
    const llmID = String(id || '').trim()
    if (!llmID) return
    setTestingLLMId(llmID)
    setError('')
    try {
      const res = await testLLMIntegration(llmID)
      const reachable = Boolean(res?.data?.reachable)
      const message = String(res?.data?.message || (reachable ? 'API 可达' : 'API 不可达')).trim()
      setLlmStatusMap((prev) => ({
        ...prev,
        [llmID]: { state: reachable ? 'reachable' : 'unreachable', message },
      }))
      if (reachable) {
        showToast('success', `智能体参数 #${llmID} 可达`)
      } else {
        showToast('warning', `智能体参数 #${llmID} 不可达：${message}`)
      }
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '测试失败'
      setError(reason)
      setLlmStatusMap((prev) => ({
        ...prev,
        [llmID]: { state: 'unreachable', message: reason },
      }))
      showToast('error', `测试失败：${reason}`)
    } finally {
      setTestingLLMId('')
    }
  }

  const handleAddExchange = async () => {
    setAddingExchange(true)
    setError('')
    try {
      const payload = {
        name: String(newExchange.name || '').trim(),
        exchange: String(newExchange.exchange || 'binance').trim(),
        api_key: String(newExchange.api_key || '').trim(),
        secret: String(newExchange.secret || '').trim(),
        passphrase: String(newExchange.passphrase || '').trim(),
      }
      if (String(payload.exchange).toLowerCase() === 'okx' && !String(payload.passphrase || '').trim()) {
        throw new Error('OKX 需要填写 passphrase')
      }
      const res = await addExchangeIntegration(payload)
      setExchangeConfigs(Array.isArray(res?.data?.exchanges) ? res.data.exchanges : [])
      setActiveExchangeId(String(res?.data?.active_exchange_id || ''))
      setExchangeBound(Boolean(res?.data?.exchange_bound))
      const boundExchange = String(res?.data?.active_exchange?.exchange || '').trim().toLowerCase()
      if (boundExchange === 'okx' || boundExchange === 'binance') {
        setAccount((prev) => ({ ...prev, active_exchange: boundExchange }))
      }
      setNewExchange({
        name: '',
        exchange: 'binance',
        api_key: '',
        secret: '',
        passphrase: '',
      })
      setShowExchangeModal(false)
      await refreshCore(true)
      await Promise.all([
        loadAssetOverview(),
        loadAssetTrend(assetRange),
        loadAssetCalendar(assetMonth),
        loadAssetDistribution(),
      ])
      showToast('success', `交易所参数添加并验证成功（ID=${res?.data?.added?.id || '-'})`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '交易所参数添加失败'
      setError(reason)
      showToast('error', `添加失败：${reason}`)
    } finally {
      setAddingExchange(false)
    }
  }

  const bindExchangeAccount = async (id) => {
    const exchangeID = String(id || '').trim()
    if (!exchangeID) return
    setActivatingExchangeId(exchangeID)
    setError('')
    try {
      const res = await activateExchangeIntegration(exchangeID)
      setActiveExchangeId(String(res?.data?.active_exchange_id || exchangeID))
      setExchangeBound(Boolean(res?.data?.exchange_bound))
      const boundExchange = String(res?.data?.active_exchange?.exchange || '').trim().toLowerCase()
      if (boundExchange === 'okx' || boundExchange === 'binance') {
        setAccount((prev) => ({ ...prev, active_exchange: boundExchange }))
      }
      await loadSystemAndStrategies()
      await refreshCore(true)
      await Promise.all([
        loadAssetOverview(),
        loadAssetTrend(assetRange),
        loadAssetCalendar(assetMonth),
        loadAssetDistribution(),
      ])
      showToast('success', `账号绑定成功（ID=${exchangeID}）`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '绑定失败'
      setError(reason)
      showToast('error', `绑定失败：${reason}`)
    } finally {
      setActivatingExchangeId('')
    }
  }

  const removeExchangeAccount = async (id) => {
    const exchangeID = String(id || '').trim()
    if (!exchangeID) return
    if (!window.confirm(`确认删除交易所账号 ID=${exchangeID} 吗？`)) return
    setDeletingExchangeId(exchangeID)
    setError('')
    try {
      const res = await deleteExchangeIntegration(exchangeID)
      setExchangeConfigs(Array.isArray(res?.data?.exchanges) ? res.data.exchanges : [])
      setActiveExchangeId(String(res?.data?.active_exchange_id || ''))
      setExchangeBound(Boolean(res?.data?.exchange_bound))
      const boundExchange = String(res?.data?.active_exchange?.exchange || '').trim().toLowerCase()
      if (boundExchange === 'okx' || boundExchange === 'binance') {
        setAccount((prev) => ({ ...prev, active_exchange: boundExchange }))
      } else {
        setAccount((prev) => ({ ...prev, active_exchange: '' }))
      }
      await refreshCore(true)
      await Promise.all([
        loadAssetOverview(),
        loadAssetTrend(assetRange),
        loadAssetCalendar(assetMonth),
        loadAssetDistribution(),
      ])
      showToast('success', `账号已删除（ID=${exchangeID}）`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '删除失败'
      setError(reason)
      showToast('error', `删除失败：${reason}`)
    } finally {
      setDeletingExchangeId('')
    }
  }

  const runOneCycle = async () => {
    setRunningNow(true)
    setError('')
    try {
      await runNow()
      await refreshCore(false)
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '执行失败')
    } finally {
      setRunningNow(false)
    }
  }

  const toggleScheduler = async (start) => {
    setError('')
    try {
      if (start) await startScheduler()
      else await stopScheduler()
      await refreshCore(false)
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '调度操作失败')
    }
  }

  const generateStrategy = async () => {
    setGeneratingStrategy(true)
    setError('')
    setStrategyGenMode('')
    const id = `st_${Date.now()}`
    const activeWorkflow = (Array.isArray(skillWorkflow?.steps) ? skillWorkflow.steps : [])
      .filter((st) => Boolean(st?.enabled))
      .map((st) => String(st?.id || '').trim())
      .filter(Boolean)
    const buildLocalFallbackRule = (reason) => {
      const nowISO = new Date().toISOString()
      const fallbackName = buildCnStrategyName(genPair, habit, genStyle, false)
      return {
        id,
        name: fallbackName,
        ruleKey: `${String(genPair || '').toUpperCase()}|${String(habit || '').toLowerCase()}|${String(genStyle || '').toLowerCase()}`,
        habit,
        symbol: genPair,
        style: genStyle,
        minRR: clamp(genMinRR, 1.0, 10),
        allowReversal: Boolean(genAllowReversal),
        lowConfAction: genLowConfAction,
        directionBias: genDirectionBias,
        createdAt: nowISO,
        updatedAt: nowISO,
        source: 'workflow_generated',
        workflowVersion: 'skill-workflow/v1',
        workflowChain: activeWorkflow.length
          ? activeWorkflow
          : ['spec-builder', 'strategy-draft', 'optimizer', 'risk-reviewer', 'release-packager'],
        prompt: strategyGeneratorPromptTemplateDefault,
        preferencePrompt:
          `交易风格=${habit}；策略样式=${genStyle}；方向偏好=${genDirectionBias}；` +
          `低信心处理=${genLowConfAction}；允许反转=${genAllowReversal ? '是' : '否'}。`,
        logic: '前端本地回退：按当前选项生成默认规则，待后端恢复后可重新生成。',
        basis: `回退原因：${reason}`,
        skillPackage: {
          version: 'skill-pipeline/v1',
          workflow: activeWorkflow.length ? activeWorkflow : ['spec-builder', 'strategy-draft', 'optimizer', 'risk-reviewer', 'release-packager'],
          habit_profile: { habit, timeframe: habit === '10m' ? '15m' : (habit === '1h' ? '1h' : (habit === '4h' ? '4h' : '1d')) },
        },
      }
    }
    const activateGeneratedForExecution = async (strategyName, enabledFromServer = []) => {
      const candidate = String(strategyName || '').trim()
      const fromServer = parseStrategies(Array.isArray(enabledFromServer) ? enabledFromServer : [])
        .slice(0, 3)
      const nextEnabled = Array.from(new Set(
        (fromServer.length ? fromServer : [candidate, ...enabledStrategies])
          .map((x) => String(x || '').trim())
          .filter(Boolean),
      )).slice(0, 3)
      if (!nextEnabled.length) return

      setEnabledStrategies(nextEnabled)
      setStrategyDraft(nextEnabled)
      if (!nextEnabled.includes(activeStrategy)) {
        setActiveStrategy(nextEnabled[0])
      }
      if (!paperStrategyManualRef.current) {
        setPaperStrategySelection((old) => (sameStrategyList(old, nextEnabled) ? old : nextEnabled))
        setPaperStrategyDraft((old) => {
          const normalized = parseStrategies(old).filter((x) => nextEnabled.includes(x)).slice(0, 3)
          if (normalized.length) return normalized
          return nextEnabled
        })
        setPaperStrategy((old) => (nextEnabled.includes(old) ? old : nextEnabled[0]))
      }
      setSystemSettings((old) => mergeSystemDefaults({
        ...(old || {}),
        AI_EXECUTION_STRATEGIES: nextEnabled.join(','),
      }))
      if (!fromServer.length) {
        await persistEnabledStrategiesEnv(nextEnabled)
      }
    }
    try {
      const payload = {
        symbol: genPair,
        habit,
        strategy_style: genStyle,
        min_rr: clamp(genMinRR, 1.0, 10),
        allow_reversal: Boolean(genAllowReversal),
        low_conf_action: genLowConfAction,
        direction_bias: genDirectionBias,
      }
      let res
      try {
        res = await generateStrategyPreference(payload)
      } catch {
        // 网络抖动时重试一次
        await sleep(300)
        res = await generateStrategyPreference(payload)
      }
      const usedFallback = Boolean(res?.data?.fallback)
      const generated = res?.data?.generated || {}
      const skillPackage = res?.data?.skill_package || null
      const baseName =
        String(generated.strategy_name || '').trim() ||
        buildCnStrategyName(genPair, habit, genStyle, false)
      const name = baseName
      const preferencePrompt = String(generated.preference_prompt || '').trim()
      const generatorPrompt = String(generated.generator_prompt || '').trim()
      const logic = String(generated.logic || '').trim()
      const basis = String(generated.basis || '').trim()
      const nowISO = new Date().toISOString()
      const workflowVersion = String(skillPackage?.version || 'skill-workflow/v1').trim() || 'skill-workflow/v1'
      const workflowChain = parseStrategies(Array.isArray(skillPackage?.workflow) ? skillPackage.workflow : activeWorkflow)
      const backendGenerated = normalizeGeneratedStrategyItem(res?.data?.generated_strategy)
      const backendGeneratedList = Array.isArray(res?.data?.generated_strategies)
        ? res.data.generated_strategies
          .map((row) => normalizeGeneratedStrategyItem(row))
          .filter(Boolean)
        : []
      const backendEnabled = parseStrategies(Array.isArray(res?.data?.enabled_strategies) ? res.data.enabled_strategies : []).slice(0, 3)

      const rule = {
        id: backendGenerated?.id || id,
        name: backendGenerated?.name || name,
        ruleKey: backendGenerated?.ruleKey || '',
        habit,
        symbol: genPair,
        style: genStyle,
        minRR: clamp(genMinRR, 1.0, 10),
        allowReversal: Boolean(genAllowReversal),
        lowConfAction: genLowConfAction,
        directionBias: genDirectionBias,
        createdAt: backendGenerated?.createdAt || nowISO,
        updatedAt: backendGenerated?.updatedAt || nowISO,
        source: backendGenerated?.source || 'workflow_generated',
        workflowVersion: backendGenerated?.workflowVersion || workflowVersion,
        workflowChain: backendGenerated?.workflowChain || workflowChain,
        prompt: backendGenerated?.prompt || generatorPrompt || strategyGeneratorPromptTemplateDefault,
        preferencePrompt: backendGenerated?.preferencePrompt || preferencePrompt || '',
        logic: backendGenerated?.logic || logic || '按市场状态识别 -> 多因子确认 -> 风控过滤 -> 执行建议生成。',
        basis: backendGenerated?.basis || basis || '基于实时行情与技术指标综合生成。',
        skillPackage,
      }
      let nextRules
      if (backendGeneratedList.length) {
        const byID = String(rule.id || '').trim()
        let merged = backendGeneratedList.map((item) => {
          if (String(item?.id || '').trim() !== byID) return item
          return { ...item, ...rule }
        })
        if (!merged.some((item) => String(item?.id || '').trim() === byID)) {
          merged = [rule, ...merged]
        }
        nextRules = merged
      } else {
        nextRules = [rule, ...generatedStrategies]
      }
      setGeneratedStrategies(nextRules)
      setStrategyMetaMap((old) => mergeStrategyMetaMap(old, {
        [rule.name]: normalizeStrategyMetaByName(rule.name, {
          source: rule.source,
          workflow_version: rule.workflowVersion,
          workflow_chain: rule.workflowChain,
          last_updated_at: rule.updatedAt,
        }),
      }))
      if (!backendGenerated) {
        const synced = await syncGeneratedStrategiesToBackend(nextRules, false)
        if (Array.isArray(synced) && synced.length) {
          nextRules = synced
          setGeneratedStrategies(synced)
        }
      }
      setStrategyOptions((old) => Array.from(new Set([
        ...old,
        ...nextRules.map((item) => String(item?.name || '').trim()).filter(Boolean),
      ])))
      setSelectedRuleId(rule.id)
      setBuilderTab('rules')
      setStrategyGenMode(usedFallback ? 'fallback' : 'llm')
      if (!btStrategy) setBtStrategy(rule.name)
      await activateGeneratedForExecution(rule.name, backendEnabled)
      if (usedFallback) {
        showToast('warning', '策略已生成（智能体未接入或调用失败，使用模板回退）')
      } else {
        showToast('success', baseName !== rule.name ? `策略生成成功（已自动命名为 ${rule.name}）` : '策略生成成功')
      }

    } catch (e) {
      const reason = resolveRequestError(e, '策略生成失败')
      const fallbackRule = buildLocalFallbackRule(reason)
      const rule = { ...fallbackRule, name: String(fallbackRule.name || '').trim() || buildCnStrategyName(genPair, habit, genStyle, false) }
      const nextRules = [
        rule,
        ...generatedStrategies.filter((item) => String(item?.name || '').trim() !== rule.name),
      ]
      setGeneratedStrategies(nextRules)
      setStrategyMetaMap((old) => mergeStrategyMetaMap(old, {
        [rule.name]: normalizeStrategyMetaByName(rule.name, {
          source: rule.source,
          workflow_version: rule.workflowVersion,
          workflow_chain: rule.workflowChain,
          last_updated_at: rule.updatedAt || rule.createdAt,
        }),
      }))
      await syncGeneratedStrategiesToBackend(nextRules, false)
      setSelectedRuleId(id)
      setBuilderTab('rules')
      setStrategyGenMode('fallback')
      if (!btStrategy) setBtStrategy(rule.name)
      await activateGeneratedForExecution(rule.name, [])
      setError(`策略服务异常，已本地回退生成：${reason}`)
      showToast('warning', `策略服务异常，已本地回退生成：${reason}`)
    } finally {
      setGeneratingStrategy(false)
    }
  }

  const selectedRule = generatedStrategies.find((s) => s.id === selectedRuleId)
  const selectedBacktestHistory = useMemo(
    () => btHistory.find((row) => Number(row?.id || 0) === Number(btHistorySelectedId || 0)) || null,
    [btHistory, btHistorySelectedId],
  )
  const renameHanCount = useMemo(() => countHanChars(renameRuleName), [renameRuleName])

  useEffect(() => {
    setRenameRuleName(selectedRule?.name || '')
  }, [selectedRuleId, selectedRule?.name])

  const resolveUniqueGeneratedName = (targetName, excludeID = '') => {
    const excluded = String(excludeID || '').trim()
    const existingNames = new Set([
      ...generatedStrategies
        .filter((s) => String(s?.id || '').trim() !== excluded)
        .map((s) => String(s?.name || '').trim())
        .filter(Boolean),
      ...strategyOptions.map((n) => String(n || '').trim()).filter(Boolean),
    ])
    return makeUniqueNameWithIndex(targetName, existingNames)
  }

  const toggleStrategyDraft = (id) => {
    setStrategyDraft((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length >= 3) {
        setError('最多同时选择 3 条策略')
        return prev
      }
      return [...prev, id]
    })
  }

  const togglePaperStrategyDraft = (id) => {
    setPaperStrategyDraft((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length >= 3) {
        setError('最多同时选择 3 条策略')
        return prev
      }
      return [...prev, id]
    })
  }

  const toggleBtStrategyDraft = (id) => {
    setBtStrategyDraft((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length >= 3) {
        setError('最多同时选择 3 条策略')
        return prev
      }
      return [...prev, id]
    })
  }

  const confirmStrategySelection = async () => {
    const normalized = strategyDraft.filter((x) => executionStrategyOptions.includes(x)).slice(0, 3)
    const next = normalized.length ? normalized : (executionStrategyOptions[0] ? [executionStrategyOptions[0]] : [])
    if (!next.length) {
      setStrategyPickerOpen(false)
      return
    }
    const prevEnabled = [...enabledStrategies]
    const prevActive = activeStrategy
    const prevSystemSettings = { ...(systemSettings || {}) }
    setEnabledStrategies(next)
    if (next.length && !next.includes(activeStrategy)) {
      setActiveStrategy(next[0])
    }
    setSystemSettings((old) => mergeSystemDefaults({
      ...(old || {}),
      AI_EXECUTION_STRATEGIES: next.join(','),
    }))
    setStrategyPickerOpen(false)
    try {
      await persistEnabledStrategiesEnv(next)
      showToast('success', '交易策略已保存并生效')
      await refreshCore(true)
    } catch (e) {
      setEnabledStrategies(prevEnabled)
      setStrategyDraft(prevEnabled)
      setActiveStrategy(prevActive)
      setSystemSettings(mergeSystemDefaults(prevSystemSettings))
      const reason = e?.response?.data?.error || e?.message || '策略保存失败'
      setError(reason)
      showToast('error', `策略保存失败：${reason}`)
    }
  }

  const confirmPaperStrategySelection = () => {
    const normalized = paperStrategyDraft.filter((x) => executionStrategyOptions.includes(x)).slice(0, 3)
    const next = normalized.length ? normalized : (executionStrategyOptions[0] ? [executionStrategyOptions[0]] : [])
    paperStrategyManualRef.current = true
    setPaperStrategySelection(next)
    setPaperStrategy(next[0] || '')
    setPaperStrategyPickerOpen(false)
  }

  const confirmBtStrategySelection = () => {
    const normalized = btStrategyDraft.filter((x) => executionStrategyOptions.includes(x)).slice(0, 3)
    const next = normalized.length ? normalized : (executionStrategyOptions[0] ? [executionStrategyOptions[0]] : [])
    setBtStrategySelection(next)
    setBtStrategy(next[0] || '')
    setBtStrategyPickerOpen(false)
  }

  const renameGeneratedStrategy = async () => {
    if (!selectedRule) {
      setError('请先选择要改名的策略')
      return
    }
    const oldName = String(selectedRule.name || '').trim()
    const nextName = String(renameRuleName || '').trim()
    if (!nextName) {
      setError('策略名称不能为空')
      showToast('error', '重命名失败：策略名称不能为空')
      return
    }
    if (countHanChars(nextName) > 10) {
      setError('策略名称最多 10 个汉字')
      showToast('error', '重命名失败：策略名称最多 10 个汉字')
      return
    }
    if (nextName === oldName) {
      showToast('warning', '名称未变化')
      return
    }
    const uniqueName = resolveUniqueGeneratedName(nextName, selectedRule.id)

    const availableAfterRename = Array.from(new Set(
      strategyOptions
        .map((x) => (String(x || '').trim() === oldName ? uniqueName : String(x || '').trim()))
        .filter(Boolean),
    ))

    const updatedStrategies = generatedStrategies.map((s) => (s.id === selectedRule.id ? { ...s, name: uniqueName } : s))
    setGeneratedStrategies(updatedStrategies)
    setStrategyMetaMap((prev) => {
      const next = { ...(prev || {}) }
      const moved = next[oldName] || normalizeStrategyMetaByName(oldName, {
        source: selectedRule?.source || 'workflow_generated',
        workflow_version: selectedRule?.workflowVersion || 'skill-workflow/v1',
        workflow_chain: selectedRule?.workflowChain || [],
        last_updated_at: selectedRule?.updatedAt || selectedRule?.createdAt || '',
      })
      delete next[oldName]
      next[uniqueName] = normalizeStrategyMetaByName(uniqueName, moved)
      return next
    })
    await syncGeneratedStrategiesToBackend(updatedStrategies, false)
    setStrategyOptions(availableAfterRename)
    setEnabledStrategies((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setStrategyDraft((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setPaperStrategySelection((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setPaperStrategyDraft((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setBtStrategySelection((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setBtStrategyDraft((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setActiveStrategy((v) => (v === oldName ? uniqueName : v))
    setPaperStrategy((v) => (v === oldName ? uniqueName : v))
    setBtStrategy((v) => (v === oldName ? uniqueName : v))
    try {
      const envEnabled = executionStrategiesFromSettings(systemSettings)
      const nextEnvEnabled = Array.from(new Set(
        envEnabled
          .map((x) => (x === oldName ? uniqueName : x))
          .filter((x) => availableAfterRename.includes(x)),
      )).slice(0, 3)
      await persistEnabledStrategiesEnv(nextEnvEnabled)
    } catch {
      // keep local state; env sync can be retried via manual save
    }
    setRenameRuleName(uniqueName)
    setError('')
    showToast('success', uniqueName !== nextName ? `策略重命名成功（已自动命名为 ${uniqueName}）` : '策略重命名成功')
  }

  const deleteGeneratedStrategy = async (ruleID) => {
    const targetID = String(ruleID || '').trim()
    if (!targetID) return
    const target = generatedStrategies.find((s) => String(s?.id || '') === targetID)
    if (!target) return

    const targetName = String(target.name || '').trim()
    if (!targetName) return
    if (!window.confirm(`确认删除策略「${targetName}」吗？`)) return

    const remainingRules = generatedStrategies.filter((s) => String(s?.id || '') !== targetID)
    const remainingGeneratedNames = remainingRules
      .map((s) => String(s?.name || '').trim())
      .filter(Boolean)
    const remainingBaseOptions = strategyOptions
      .map((x) => String(x || '').trim())
      .filter((x) => x && x !== targetName)
    const remainingExecution = Array.from(new Set([...remainingBaseOptions, ...remainingGeneratedNames]))
    const fallback = remainingExecution[0] || ''

    const normalizeSelection = (arr = []) => {
      const next = arr
        .map((x) => String(x || '').trim())
        .filter((x) => x && x !== targetName && remainingExecution.includes(x))
        .slice(0, 3)
      if (next.length) return next
      return fallback ? [fallback] : []
    }

    setGeneratedStrategies(remainingRules)
    setStrategyMetaMap((prev) => {
      const next = { ...(prev || {}) }
      delete next[targetName]
      return next
    })
    await syncGeneratedStrategiesToBackend(remainingRules, false)
    setStrategyOptions((arr) => arr.filter((x) => String(x || '').trim() !== targetName))
    setSelectedRuleId((prev) => (
      String(prev || '') === targetID
        ? String(remainingRules[0]?.id || '')
        : prev
    ))
    setRenameRuleName('')

    setEnabledStrategies((arr) => normalizeSelection(arr))
    setStrategyDraft((arr) => normalizeSelection(arr))
    setPaperStrategySelection((arr) => normalizeSelection(arr))
    setPaperStrategyDraft((arr) => normalizeSelection(arr))
    setBtStrategySelection((arr) => normalizeSelection(arr))
    setBtStrategyDraft((arr) => normalizeSelection(arr))

    setActiveStrategy((v) => {
      const cur = String(v || '').trim()
      return cur && cur !== targetName && remainingExecution.includes(cur) ? cur : fallback
    })
    setPaperStrategy((v) => {
      const cur = String(v || '').trim()
      return cur && cur !== targetName && remainingExecution.includes(cur) ? cur : fallback
    })
    setBtStrategy((v) => {
      const cur = String(v || '').trim()
      return cur && cur !== targetName && remainingExecution.includes(cur) ? cur : fallback
    })
    try {
      const envEnabled = executionStrategiesFromSettings(systemSettings)
      const nextEnvEnabled = envEnabled
        .filter((x) => x !== targetName && remainingExecution.includes(x))
        .slice(0, 3)
      await persistEnabledStrategiesEnv(nextEnvEnabled)
    } catch {
      // keep local state; env sync can be retried via manual save
    }

    setError('')
    showToast('success', '策略规则已删除')
  }

  async function runBacktest() {
    setBtRunning(true)
    setError('')
    try {
      if (btEnd < btStart) {
        throw new Error('结束时间不能早于开始时间')
      }
      const normalizedBtLeverage = normalizeLeverage(btLeverage)
      const normalizedBtHighAmount = normalizeDecimal(btHighConfidenceAmount, 0, 1000000)
      const normalizedBtLowAmount = normalizeDecimal(btLowConfidenceAmount, 0, 1000000)
      const normalizedBtHighPct = normalizeDecimal(btHighConfidenceMarginPct, 0, 100)
      const normalizedBtLowPct = normalizeDecimal(btLowConfidenceMarginPct, 0, 100)
      setBtLeverage(normalizedBtLeverage)
      setBtHighConfidenceAmount(normalizedBtHighAmount)
      setBtLowConfidenceAmount(normalizedBtLowAmount)
      setBtHighConfidenceMarginPct(normalizedBtHighPct)
      setBtLowConfidenceMarginPct(normalizedBtLowPct)
      const res = await runBacktestApi({
        pair: btPair,
        habit,
        start_month: btStart,
        end_month: btEnd,
        initial_margin: clamp(btInitialMargin, 1, 1000000000),
        leverage: normalizedBtLeverage,
        position_sizing_mode: String(btPositionSizingMode || 'contracts'),
        high_confidence_amount: normalizedBtHighAmount,
        low_confidence_amount: normalizedBtLowAmount,
        high_confidence_margin_pct: normalizedBtHighPct / 100,
        low_confidence_margin_pct: normalizedBtLowPct / 100,
        paper_margin: clamp(paperMargin, 0, 1000000),
        strategy_name: btStrategySelection[0] || btStrategy || activeStrategy || 'default_strategy',
      })
      const summary = res?.data?.summary || null
      const records = Array.isArray(res?.data?.records) ? res.data.records : []
      if (!summary) {
        setError('回测返回为空')
        return
      }
      const mappedSummary = mapBacktestSummary(summary, {
        btStrategy,
        btPair,
        habit,
        btStart,
        btEnd,
      })
      setBtSummary(mappedSummary)
      setBtRecords(records.slice().reverse())
      if (mappedSummary.historyId > 0) {
        setBtHistorySelectedId(mappedSummary.historyId)
      }
      await loadBacktestHistory(true)
      if (res?.data?.history_warning) {
        showToast('warning', String(res.data.history_warning))
      }
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '回测失败')
    } finally {
      setBtRunning(false)
    }
  }

  const viewBacktestHistoryDetail = async (id) => {
    const historyID = Number(id || 0)
    if (historyID <= 0) return
    setBtHistoryLoading(true)
    setError('')
    try {
      const res = await getBacktestHistoryDetail(historyID)
      const summary = res?.data?.summary || null
      const records = Array.isArray(res?.data?.records) ? res.data.records : []
      if (!summary) {
        setError('回测详情为空')
        return
      }
      setBtSummary(mapBacktestSummary(summary, {
        btStrategy,
        btPair,
        habit,
        btStart,
        btEnd,
      }))
      setBtRecords(records.slice().reverse())
      setBtHistorySelectedId(historyID)
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '加载回测详情失败')
      showToast('error', '加载回测详情失败')
    } finally {
      setBtHistoryLoading(false)
    }
  }

  const removeBacktestHistory = async (id) => {
    const historyID = Number(id || 0)
    if (historyID <= 0) return
    if (!window.confirm(`确认删除回测记录 #${historyID} 吗？`)) return
    setBtHistoryDeletingId(historyID)
    setError('')
    try {
      await deleteBacktestHistory(historyID)
      await loadBacktestHistory(true)
      setBtHistory((arr) => arr.filter((x) => Number(x?.id || 0) !== historyID))
      if (Number(btHistorySelectedId || 0) === historyID) {
        setBtSummary(null)
        setBtRecords([])
        setBtHistorySelectedId(0)
      }
      showToast('success', `回测记录 #${historyID} 已删除`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '删除失败'
      setError(reason)
      showToast('error', `删除失败：${reason}`)
    } finally {
      setBtHistoryDeletingId(0)
    }
  }

  return {
    menu,
    setMenu,
    loading,
    error,
    themeMode,
    setThemeMode,
    productName,
    sidebarMenuItems,
    toast,
    showLLMModal,
    setShowLLMModal,
    editingLLMId,
    setEditingLLMId,
    resetLLMModalDraft,
    setNewLLM,
    newLLM,
    llmProductCatalog,
    selectedLLMPreset,
    llmModelOptions,
    probingLLMModels,
    llmProbeMessage,
    probeLLMModelOptions,
    addingLLM,
    handleAddLLM,
    showExchangeModal,
    setShowExchangeModal,
    addingExchange,
    handleAddExchange,
    newExchange,
    setNewExchange,
    marketEmotion,
    totalPnL,
    status,
    account,
    strategyDurationText,
    pnlRatio,
    resolvedTheme,
    activePair,
    setActivePair,
    strategyPickerRef,
    strategyPickerOpen,
    setStrategyPickerOpen,
    enabledStrategies,
    strategyMetaMap,
    setStrategyDraft,
    selectedStrategyText,
    executionStrategyOptions,
    strategyDraft,
    toggleStrategyDraft,
    confirmStrategySelection,
    settings,
    setSettings: setLiveSettings,
    refreshCore,
    runningNow,
    runOneCycle,
    startLiveTrading,
    startingLive,
    toggleScheduler,
    schedulerRunning,
    savingSettings,
    saveLiveConfig,
    liveViewTab,
    setLiveViewTab,
    liveStrategyLabel,
    liveStrategyHistory,
    liveMarketSnapshot,
    tradeRecords,
    paperPair,
    setPaperPair,
    paperStrategyPickerRef,
    paperStrategyPickerOpen,
    setPaperStrategyPickerOpen,
    paperStrategySelection,
    setPaperStrategyDraft,
    paperSelectedStrategyText,
    paperStrategyDraft,
    togglePaperStrategyDraft,
    confirmPaperStrategySelection,
    paperMargin,
    setPaperMargin,
    paperSettings,
    setPaperSettings,
    startPaperSim,
    paperSimLoading,
    paperSimRunning,
    pausePaperSim,
    paperViewTab,
    setPaperViewTab,
    paperTradeRecords,
    paperLatestDecision: paperLatestDecisionMap[String(paperPair || '').toUpperCase()] || null,
    paperStrategyHistory,
    paperPnlBaselineMap,
    resetPaperCurrentPnL,
    assetOverview,
    assetDistribution,
    assetMonth,
    setAssetMonth,
    assetCalendar,
    assetRange,
    setAssetRange,
    assetTrend,
    builderTab,
    setBuilderTab,
    strategyGenMode,
    habit,
    setHabit,
    genPair,
    setGenPair,
    genStyle,
    setGenStyle,
    genMinRR,
    setGenMinRR,
    genLowConfAction,
    setGenLowConfAction,
    genDirectionBias,
    setGenDirectionBias,
    genAllowReversal,
    setGenAllowReversal,
    generateStrategy,
    generatingStrategy,
    skillWorkflow,
    loadingSkillWorkflow,
    savingSkillWorkflow,
    runningWorkflowUpgradeNow,
    aiWorkflowTab,
    setAiWorkflowTab,
    aiWorkflowLogs,
    aiWorkflowLogsLoading,
    aiWorkflowLogChannel,
    setAiWorkflowLogChannel,
    aiWorkflowLogLimit,
    setAiWorkflowLogLimit,
    coreRiskSettings,
    setCoreRiskField,
    savingCoreRiskSettings,
    coreRiskSaveHint,
    saveCoreRiskSettings,
    resettingRiskBaseline,
    resetRiskManually,
    autoReviewSaveHint,
    savingAutoReviewSettings,
    saveAutoReviewEnv,
    updateSkillStepField,
    updateSkillConstraintField,
    updateSkillPromptField,
    saveSkillWorkflowConfig,
    resetSkillWorkflowConfig,
    runWorkflowUpgradeNow,
    loadSkillWorkflowConfig,
    loadAIWorkflowLogs,
    generatedStrategies,
    selectedRuleId,
    setSelectedRuleId,
    selectedRule,
    renameRuleName,
    setRenameRuleName,
    renameGeneratedStrategy,
    deleteGeneratedStrategy,
    renameHanCount,
    btStrategyPickerRef,
    btStrategyPickerOpen,
    setBtStrategyPickerOpen,
    btStrategySelection,
    setBtStrategyDraft,
    btSelectedStrategyText,
    btStrategyDraft,
    toggleBtStrategyDraft,
    confirmBtStrategySelection,
    btPair,
    setBtPair,
    btInitialMargin,
    setBtInitialMargin,
    btLeverage,
    setBtLeverage,
    btPositionSizingMode,
    setBtPositionSizingMode,
    btHighConfidenceAmount,
    setBtHighConfidenceAmount,
    btLowConfidenceAmount,
    setBtLowConfidenceAmount,
    btHighConfidenceMarginPct,
    setBtHighConfidenceMarginPct,
    btLowConfidenceMarginPct,
    setBtLowConfidenceMarginPct,
    btStart,
    setBtStart,
    btEnd,
    setBtEnd,
    runBacktest,
    btRunning,
    btHistoryLoading,
    btHistory,
    btHistorySelectedId,
    viewBacktestHistoryDetail,
    selectedBacktestHistory,
    btSummary,
    btHistoryDeletingId,
    removeBacktestHistory,
    btRecords,
    systemSubTab,
    setSystemSubTab,
    loadingSystemRuntime,
    loadSystemRuntime,
    restartingBackend,
    restartBackend,
    runtimeComponents,
    systemRuntime,
    systemSettings,
    setSystemSettings,
    systemSaveHint,
    savingSystemSettings,
    saveSystemEnv,
    llmConfigs,
    testingLLMId,
    llmStatusMap,
    testLLMConfigReachability,
    openEditLLMModal,
    deletingLLMId,
    removeLLMConfig,
    exchangeBound,
    activeExchangeId,
    activeExchangeType,
    exchangeConfigs,
    activatingExchangeId,
    bindExchangeAccount,
    deletingExchangeId,
    removeExchangeAccount,
  }
}
