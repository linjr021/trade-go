// @ts-nocheck
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Bot,
  CandlestickChart,
  FlaskConical,
  History,
  Settings2,
  Wallet,
} from 'lucide-react'
import { fmtTime } from '@/modules/format'
import { useCloseOnOutside } from '@/modules/use-close-on-outside'
import {
  envFieldDefs,
  HISTORY_MAX_MONTH,
  LLM_PRODUCT_CATALOG as DEFAULT_LLM_PRODUCT_CATALOG,
  promptSettingDefaults,
  strategyTemplateFallback,
  systemSettingDefaults,
} from '@/modules/constants'
import {
  calcPaperPnL,
  clamp,
  countHanChars,
  joinFieldMessages,
  loadPaperLocalRecords,
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
  getSignals,
  getTradeRecords,
  getStatus,
  getStrategyTemplate,
  getStrategies,
  getStrategyScores,
  getPromptSettings,
  getSystemSettings,
  generateStrategyPreference,
  resetPromptSettings,
  runBacktestApi,
  getBacktestHistory,
  getBacktestHistoryDetail,
  deleteBacktestHistory,
  runNow,
  savePromptSettings,
  saveSystemSettings,
  getSystemRuntimeStatus,
  restartSystemRuntime,
  startScheduler,
  stopScheduler,
  uploadStrategyFile,
  updateSettings,
} from '../api'

export function useDashboardController() {
  const [menu, setMenu] = useState('live')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [themeMode, setThemeMode] = useState(() => localStorage.getItem('ui-theme-mode') || 'system')
  const [prefersDark, setPrefersDark] = useState(
    () => window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
  )

  const [status, setStatus] = useState({})
  const [account, setAccount] = useState({})
  const [signals, setSignals] = useState([])
  const [tradeRecords, setTradeRecords] = useState([])
  const [strategyScores, setStrategyScores] = useState([])

  const [strategyOptions, setStrategyOptions] = useState(['ai_assisted', 'trend_following', 'mean_reversion', 'breakout'])
  const [enabledStrategies, setEnabledStrategies] = useState(['ai_assisted'])
  const [activeStrategy, setActiveStrategy] = useState('ai_assisted')
  const [paperStrategy, setPaperStrategy] = useState('ai_assisted')
  const [activePair, setActivePairState] = useState('BTCUSDT')
  const [paperPair, setPaperPair] = useState('BTCUSDT')
  const activePairHydratedRef = useRef(false)
  const activePairUserOverrideRef = useRef(false)
  const [liveViewTab, setLiveViewTab] = useState('overview')
  const [paperViewTab, setPaperViewTab] = useState('overview')
  const [strategyPickerOpen, setStrategyPickerOpen] = useState(false)
  const [strategyDraft, setStrategyDraft] = useState([])
  const strategyPickerRef = useRef(null)
  const [paperStrategySelection, setPaperStrategySelection] = useState(['ai_assisted'])
  const [paperStrategyPickerOpen, setPaperStrategyPickerOpen] = useState(false)
  const [paperStrategyDraft, setPaperStrategyDraft] = useState(['ai_assisted'])
  const paperStrategyPickerRef = useRef(null)
  const [btStrategySelection, setBtStrategySelection] = useState([])
  const [btStrategyPickerOpen, setBtStrategyPickerOpen] = useState(false)
  const [btStrategyDraft, setBtStrategyDraft] = useState([])
  const btStrategyPickerRef = useRef(null)

  const [settings, setSettings] = useState({
    positionSizingMode: 'contracts',
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    highConfidenceMarginPct: 10,
    lowConfidenceMarginPct: 5,
    leverage: 10,
  })
  const [paperSettings, setPaperSettings] = useState({
    positionSizingMode: 'contracts',
    highConfidenceAmount: 0.01,
    lowConfidenceAmount: 0.005,
    highConfidenceMarginPct: 10,
    lowConfidenceMarginPct: 5,
    leverage: 10,
  })
  const [systemSettings, setSystemSettings] = useState({ ...systemSettingDefaults })
  const [systemSubTab, setSystemSubTab] = useState('env')
  const [systemRuntime, setSystemRuntime] = useState(null)
  const [backendReachability, setBackendReachability] = useState({
    status: 'unconfigured',
    message: '未检测',
    checkedAt: '',
  })
  const [loadingSystemRuntime, setLoadingSystemRuntime] = useState(false)
  const [restartingBackend, setRestartingBackend] = useState(false)
  const [llmConfigs, setLlmConfigs] = useState([])
  const [llmProductCatalog, setLlmProductCatalog] = useState(() => DEFAULT_LLM_PRODUCT_CATALOG)
  const [exchangeConfigs, setExchangeConfigs] = useState([])
  const [activeExchangeId, setActiveExchangeId] = useState('')
  const [exchangeBound, setExchangeBound] = useState(false)
  const [activatingExchangeId, setActivatingExchangeId] = useState('')
  const [deletingExchangeId, setDeletingExchangeId] = useState('')
  const [addingLLM, setAddingLLM] = useState(false)
  const [editingLLMId, setEditingLLMId] = useState('')
  const [deletingLLMId, setDeletingLLMId] = useState('')
  const [testingLLMId, setTestingLLMId] = useState('')
  const [llmStatusMap, setLlmStatusMap] = useState({})
  const [addingExchange, setAddingExchange] = useState(false)
  const [showLLMModal, setShowLLMModal] = useState(false)
  const [showExchangeModal, setShowExchangeModal] = useState(false)
  const [newLLM, setNewLLM] = useState({
    name: '',
    product: 'chatgpt',
    base_url: 'https://api.openai.com/v1',
    api_key: '',
    model: '',
  })
  const [llmModelOptions, setLlmModelOptions] = useState([])
  const [probingLLMModels, setProbingLLMModels] = useState(false)
  const [llmProbeMessage, setLlmProbeMessage] = useState('')
  const llmProbeTimerRef = useRef(null)
  const llmProbeSeqRef = useRef(0)
  const llmProbeKeyRef = useRef('')
  const [newExchange, setNewExchange] = useState({
    name: '',
    exchange: 'binance',
    api_key: '',
    secret: '',
    passphrase: '',
  })
  const [promptSettings, setPromptSettings] = useState({
    trading_ai_system_prompt: '',
    trading_ai_policy_prompt: '',
    strategy_generator_prompt_template: '',
  })
  const [paperMargin, setPaperMargin] = useState(200)
  const [paperLocalRecords, setPaperLocalRecords] = useState(() => loadPaperLocalRecords())
  const [paperSimRunning, setPaperSimRunning] = useState(false)
  const [paperSimLoading, setPaperSimLoading] = useState(false)
  const paperSimTimerRef = useRef(null)
  const paperLastPriceRef = useRef(0)
  const paperConfigRef = useRef({
    pair: 'BTCUSDT',
    margin: 200,
    settings: {
      positionSizingMode: 'contracts',
      highConfidenceAmount: 0.01,
      lowConfidenceAmount: 0.005,
      highConfidenceMarginPct: 10,
      lowConfidenceMarginPct: 5,
      leverage: 10,
    },
  })

  const [runningNow, setRunningNow] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const [savingSystemSettings, setSavingSystemSettings] = useState(false)
  const [systemSaveHint, setSystemSaveHint] = useState('')
  const [toast, setToast] = useState({ visible: false, type: 'success', message: '' })
  const [savingPrompts, setSavingPrompts] = useState(false)
  const [resettingPrompts, setResettingPrompts] = useState(false)

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
  const [uploadFile, setUploadFile] = useState(null)
  const [uploadingStrategy, setUploadingStrategy] = useState(false)
  const [strategyTemplate, setStrategyTemplate] = useState('')
  const [loadingTemplate, setLoadingTemplate] = useState(false)
  const [generatingStrategy, setGeneratingStrategy] = useState(false)

  const [btStrategy, setBtStrategy] = useState('')
  const [btPair, setBtPair] = useState('BTCUSDT')
  const [btInitialMargin, setBtInitialMargin] = useState(1000)
  const [btLeverage, setBtLeverage] = useState(10)
  const [btPositionSizingMode, setBtPositionSizingMode] = useState('contracts')
  const [btHighConfidenceAmount, setBtHighConfidenceAmount] = useState(0.01)
  const [btLowConfidenceAmount, setBtLowConfidenceAmount] = useState(0.005)
  const [btHighConfidenceMarginPct, setBtHighConfidenceMarginPct] = useState(10)
  const [btLowConfidenceMarginPct, setBtLowConfidenceMarginPct] = useState(5)
  const [btStart, setBtStart] = useState('2021-01')
  const [btEnd, setBtEnd] = useState('2024-12')
  const [btRunning, setBtRunning] = useState(false)
  const [btSummary, setBtSummary] = useState(null)
  const [btRecords, setBtRecords] = useState([])
  const [btHistory, setBtHistory] = useState([])
  const [btHistoryLoading, setBtHistoryLoading] = useState(false)
  const [btHistoryDeletingId, setBtHistoryDeletingId] = useState(0)
  const [btHistorySelectedId, setBtHistorySelectedId] = useState(0)

  const [liveStrategyStartedAt, setLiveStrategyStartedAt] = useState(Date.now())
  const [assetRange, setAssetRange] = useState('30D')
  const [assetMonth, setAssetMonth] = useState(HISTORY_MAX_MONTH)
  const [assetOverview, setAssetOverview] = useState({})
  const [assetTrend, setAssetTrend] = useState([])
  const [assetCalendar, setAssetCalendar] = useState([])
  const [assetDistribution, setAssetDistribution] = useState([])

  const schedulerRunning = Boolean(status?.scheduler_running)
  const resolvedTheme = useMemo(() => {
    if (themeMode === 'system') return prefersDark ? 'dark' : 'light'
    return themeMode === 'dark' ? 'dark' : 'light'
  }, [themeMode, prefersDark])
  const rawProductName = String(systemSettings?.PRODUCT_NAME || '').trim()
  const productName = !rawProductName || rawProductName === 'AI 交易看板'
    ? '21xG交易'
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
    const id = String(activeExchangeId || '').trim()
    const matched = exchangeConfigs.find((x) => String(x?.id || '').trim() === id)
    return String(matched?.exchange || 'binance').trim().toLowerCase() || 'binance'
  }, [activeExchangeId, exchangeConfigs])
  const selectedLLMPreset = useMemo(
    () => llmProductCatalog.find((p) => String(p?.product || '') === String(newLLM?.product || '')) || llmProductCatalog[0],
    [newLLM?.product, llmProductCatalog],
  )
  const setActivePair = useCallback((nextPair) => {
    const symbol = String(nextPair || '').toUpperCase()
    activePairUserOverrideRef.current = true
    setActivePairState(symbol || 'BTCUSDT')
  }, [])
  const sidebarMenuItems = useMemo(
    () => [
      { key: 'assets', label: '资产详情', icon: <Wallet size={16} /> },
      { key: 'live', label: '实盘交易', icon: <CandlestickChart size={16} /> },
      { key: 'paper', label: '模拟交易', icon: <FlaskConical size={16} /> },
      { key: 'builder', label: '策略生成', icon: <Bot size={16} /> },
      { key: 'backtest', label: '历史回测', icon: <History size={16} /> },
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
      const [tradeRes, statusRes, accountRes, signalRes, scoreRes] = await Promise.all([
        getTradeRecords(60),
        getStatus(),
        getAccount(),
        getSignals(40),
        getStrategyScores(40),
      ])
      setTradeRecords(tradeRes?.data?.records || [])
      const st = statusRes.data || {}
      setStatus(st)
      setAccount(accountRes.data || {})
      setSignals([...(signalRes?.data?.signals || [])].reverse())
      setStrategyScores(scoreRes?.data?.scores || st?.strategy_scores || [])

      const cfg = st?.trade_config || {}
      setSettings((old) => ({
        ...old,
        ...normalizeTradeSettings({
          positionSizingMode: String(cfg.position_sizing_mode || old.positionSizingMode || 'contracts'),
          highConfidenceAmount: Number(cfg.high_confidence_amount || old.highConfidenceAmount || 0.01),
          lowConfidenceAmount: Number(cfg.low_confidence_amount || old.lowConfidenceAmount || 0.005),
          highConfidenceMarginPct: Number(cfg.high_confidence_margin_pct || 0.1) * 100,
          lowConfidenceMarginPct: Number(cfg.low_confidence_margin_pct || 0.05) * 100,
          leverage: Number(cfg.leverage || old.leverage || 10),
        }),
      }))
      setPaperSettings((old) => ({
        ...old,
        ...normalizeTradeSettings({
          positionSizingMode: String(cfg.position_sizing_mode || old.positionSizingMode || 'contracts'),
          highConfidenceAmount: Number(cfg.high_confidence_amount || old.highConfidenceAmount || 0.01),
          lowConfidenceAmount: Number(cfg.low_confidence_amount || old.lowConfidenceAmount || 0.005),
          highConfidenceMarginPct: Number(cfg.high_confidence_margin_pct || 0.1) * 100,
          lowConfidenceMarginPct: Number(cfg.low_confidence_margin_pct || 0.05) * 100,
          leverage: Number(cfg.leverage || old.leverage || 10),
        }),
      }))
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
    } catch (e) {
      if (!silent) setError(e?.response?.data?.error || e?.message || '请求失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  const paperTradeRecords = useMemo(
    () => paperLocalRecords.filter((r) => !r.symbol || r.symbol === paperPair),
    [paperLocalRecords, paperPair],
  )

  const computePaperOrderSize = (price, confidence, simCfg) => {
    const px = Number(price || 0)
    if (!Number.isFinite(px) || px <= 0) return 0
    const settings = simCfg?.settings || paperSettings
    const margin = Number(simCfg?.margin ?? paperMargin)
    const mode = String(settings.positionSizingMode || 'contracts')
    const isHigh = String(confidence || '').toUpperCase() === 'HIGH'
    if (mode === 'margin_pct') {
      const pct = clamp(
        isHigh ? settings.highConfidenceMarginPct : settings.lowConfidenceMarginPct,
        0,
        100,
      ) / 100
      if (pct <= 0) return 0
      const leverage = normalizeLeverage(settings.leverage)
      return (clamp(margin, 0, 1_000_000_000) * pct * leverage) / px
    }
    return clamp(
      isHigh ? settings.highConfidenceAmount : settings.lowConfidenceAmount,
      0,
      1_000_000,
    )
  }

  const fetchPaperPrice = async (symbol) => {
    const sym = String(symbol || 'BTCUSDT').toUpperCase()
    const url = `https://api.binance.com/api/v3/ticker/price?symbol=${encodeURIComponent(sym)}`
    const resp = await fetch(url, { method: 'GET' })
    if (!resp.ok) throw new Error(`行情请求失败(${resp.status})`)
    const json = await resp.json()
    const price = Number(json?.price || 0)
    if (!Number.isFinite(price) || price <= 0) throw new Error('行情价格无效')
    return price
  }

  const runPaperSimStep = async () => {
    const simCfg = paperConfigRef.current || {}
    const simPair = String(simCfg.pair || paperPair || 'BTCUSDT').toUpperCase()
    const nowISO = new Date().toISOString()
    let price = 0
    try {
      price = await fetchPaperPrice(simPair)
    } catch {
      const prev = Number(paperLastPriceRef.current || 0)
      if (prev > 0) {
        const noise = 1 + (Math.random() - 0.5) * 0.004
        price = Math.max(0.0001, prev * noise)
      } else {
        price = 0
      }
    }
    if (!(price > 0)) return

    const prev = Number(paperLastPriceRef.current || 0) || price
    const deltaPct = prev > 0 ? ((price - prev) / prev) * 100 : 0
    const absDelta = Math.abs(deltaPct)
    let signal = 'HOLD'
    if (deltaPct >= 0.05) signal = 'BUY'
    if (deltaPct <= -0.05) signal = 'SELL'
    const confidence = absDelta >= 0.2 ? 'HIGH' : 'LOW'
    const size = signal === 'HOLD' ? 0 : computePaperOrderSize(price, confidence, simCfg)
    const pnl = calcPaperPnL(signal, prev, price, size)

    const rec = {
      id: `paper-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      ts: nowISO,
      symbol: simPair,
      signal,
      confidence,
      approved: signal !== 'HOLD' && size > 0,
      approved_size: Number(size || 0),
      price: Number(price || 0),
      unrealized_pnl: Number(pnl || 0),
      mode: String(simCfg?.settings?.positionSizingMode || 'contracts'),
      leverage: normalizeLeverage(simCfg?.settings?.leverage),
      source: 'paper_local',
    }

    paperLastPriceRef.current = price
    setPaperLocalRecords((arr) => [rec, ...arr].slice(0, 2000))
  }

  const startPaperSim = async () => {
    if (paperSimRunning) return
    setPaperSimLoading(true)
    try {
      await runPaperSimStep()
      paperSimTimerRef.current = setInterval(() => {
        runPaperSimStep()
      }, 8000)
      setPaperSimRunning(true)
      showToast('success', '模拟交易已开始（仅本地记录，不会下交易所订单）')
    } finally {
      setPaperSimLoading(false)
    }
  }

  const pausePaperSim = () => {
    if (paperSimTimerRef.current) {
      clearInterval(paperSimTimerRef.current)
      paperSimTimerRef.current = null
    }
    setPaperSimRunning(false)
    showToast('warning', '模拟交易已暂停')
  }

  useEffect(() => {
    localStorage.setItem('paper-local-records', JSON.stringify(paperLocalRecords))
  }, [paperLocalRecords])

  useEffect(() => {
    paperConfigRef.current = {
      pair: paperPair,
      margin: paperMargin,
      settings: { ...paperSettings },
    }
  }, [paperPair, paperMargin, paperSettings])

  useEffect(() => () => {
    if (paperSimTimerRef.current) {
      clearInterval(paperSimTimerRef.current)
      paperSimTimerRef.current = null
    }
  }, [])

  useEffect(() => {
    setStrategyDraft(enabledStrategies)
  }, [enabledStrategies])

  useEffect(() => {
    setPaperStrategyDraft(paperStrategySelection)
  }, [paperStrategySelection])

  useEffect(() => {
    setBtStrategyDraft(btStrategySelection)
  }, [btStrategySelection])

  const closeStrategyPicker = useCallback(() => setStrategyPickerOpen(false), [])
  const closePaperStrategyPicker = useCallback(() => setPaperStrategyPickerOpen(false), [])
  const closeBtStrategyPicker = useCallback(() => setBtStrategyPickerOpen(false), [])

  useCloseOnOutside(strategyPickerOpen, strategyPickerRef, closeStrategyPicker)
  useCloseOnOutside(paperStrategyPickerOpen, paperStrategyPickerRef, closePaperStrategyPicker)
  useCloseOnOutside(btStrategyPickerOpen, btStrategyPickerRef, closeBtStrategyPicker)

  const loadSystemAndStrategies = async () => {
    const [sysRes, strategyRes, promptRes, integrationRes] = await Promise.allSettled([
      getSystemSettings(),
      getStrategies(),
      getPromptSettings(),
      getIntegrations(),
    ])

    let merged = mergeSystemDefaults(systemSettings)
    if (sysRes.status === 'fulfilled') {
      merged = mergeSystemDefaults(sysRes.value?.data?.settings || {})
    }
    setSystemSettings(merged)

    if (promptRes.status === 'fulfilled') {
      const prompts = promptRes.value?.data?.prompts || {}
      setPromptSettings({
        trading_ai_system_prompt: String(
          prompts.trading_ai_system_prompt || promptSettingDefaults.trading_ai_system_prompt
        ),
        trading_ai_policy_prompt: String(
          prompts.trading_ai_policy_prompt || promptSettingDefaults.trading_ai_policy_prompt
        ),
        strategy_generator_prompt_template: String(
          prompts.strategy_generator_prompt_template || promptSettingDefaults.strategy_generator_prompt_template
        ),
      })
    } else {
      setPromptSettings((v) => ({
        trading_ai_system_prompt:
          String(v.trading_ai_system_prompt || '').trim() || promptSettingDefaults.trading_ai_system_prompt,
        trading_ai_policy_prompt:
          String(v.trading_ai_policy_prompt || '').trim() || promptSettingDefaults.trading_ai_policy_prompt,
        strategy_generator_prompt_template:
          String(v.strategy_generator_prompt_template || '').trim() ||
          promptSettingDefaults.strategy_generator_prompt_template,
      }))
    }

    if (strategyRes.status === 'fulfilled') {
      const available = parseStrategies(strategyRes.value?.data?.available)
      const enabled = parseStrategies(strategyRes.value?.data?.enabled)
      const byEnv = parseStrategies(String(merged.PY_STRATEGY_ENABLED || '').split(','))
      const mergedSet = Array.from(new Set([...available, ...enabled, ...byEnv]))
      const mergedExecution = Array.from(new Set([...mergedSet, ...generatedStrategyNames]))
      const enabledMerged = Array.from(new Set([...enabled, ...byEnv])).filter((x) => mergedExecution.includes(x))
      if (mergedSet.length) {
        setStrategyOptions(mergedSet)
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
  }

  useEffect(() => {
    refreshCore(false)
    loadSystemAndStrategies()
    loadStrategyTemplate()
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

  const probeLLMModelOptions = async (input = {}) => {
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
    const diff = Math.max(0, Date.now() - liveStrategyStartedAt)
    const mins = Math.floor(diff / 60000)
    const hours = Math.floor(mins / 60)
    const rem = mins % 60
    if (hours > 0) return `${hours}h ${rem}m`
    return `${mins}m`
  }, [liveStrategyStartedAt, status?.runtime?.last_run_at])

  const saveLiveConfig = async () => {
    setSavingSettings(true)
    setError('')
    try {
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
      const merged = { ...systemSettings, PY_STRATEGY_ENABLED: nextEnabled.join(',') }
      await saveSystemSettings(merged)
      setSystemSettings(merged)
      setLiveStrategyStartedAt(Date.now())
      activePairHydratedRef.current = true
      activePairUserOverrideRef.current = false
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

  const savePromptConfig = async () => {
    setSavingPrompts(true)
    setError('')
    try {
      const payload = {
        trading_ai_system_prompt: String(promptSettings.trading_ai_system_prompt || ''),
        trading_ai_policy_prompt: String(promptSettings.trading_ai_policy_prompt || ''),
        strategy_generator_prompt_template: String(promptSettings.strategy_generator_prompt_template || ''),
      }
      const res = await savePromptSettings(payload)
      const prompts = res?.data?.prompts || payload
      setPromptSettings({
        trading_ai_system_prompt: String(prompts.trading_ai_system_prompt || ''),
        trading_ai_policy_prompt: String(prompts.trading_ai_policy_prompt || ''),
        strategy_generator_prompt_template: String(prompts.strategy_generator_prompt_template || ''),
      })
      showToast('success', '提示词保存成功')
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '提示词保存失败'
      setError(reason)
      showToast('error', `保存失败：${reason}`)
    } finally {
      setSavingPrompts(false)
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
      setNewExchange({
        name: '',
        exchange: 'binance',
        api_key: '',
        secret: '',
        passphrase: '',
      })
      setShowExchangeModal(false)
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
      await loadSystemAndStrategies()
      await refreshCore(true)
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
      await refreshCore(true)
      showToast('success', `账号已删除（ID=${exchangeID}）`)
    } catch (e) {
      const reason = e?.response?.data?.error || e?.message || '删除失败'
      setError(reason)
      showToast('error', `删除失败：${reason}`)
    } finally {
      setDeletingExchangeId('')
    }
  }

  const resetPromptConfig = async () => {
    setResettingPrompts(true)
    setError('')
    try {
      const res = await resetPromptSettings()
      const prompts = res?.data?.prompts || {}
      setPromptSettings({
        trading_ai_system_prompt: String(prompts.trading_ai_system_prompt || ''),
        trading_ai_policy_prompt: String(prompts.trading_ai_policy_prompt || ''),
        strategy_generator_prompt_template: String(prompts.strategy_generator_prompt_template || ''),
      })
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '恢复默认失败')
    } finally {
      setResettingPrompts(false)
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
    const buildLocalFallbackRule = (reason) => {
      const fallbackName = `AI_${genPair}_${habit}_${new Date().toISOString().slice(0, 10)}`
      return {
        id,
        name: fallbackName,
        habit,
        symbol: genPair,
        style: genStyle,
        minRR: clamp(genMinRR, 1.0, 10),
        allowReversal: Boolean(genAllowReversal),
        lowConfAction: genLowConfAction,
        directionBias: genDirectionBias,
        createdAt: new Date().toISOString(),
        prompt: promptSettings.strategy_generator_prompt_template,
        preferencePrompt:
          `交易风格=${habit}；策略样式=${genStyle}；方向偏好=${genDirectionBias}；` +
          `低信心处理=${genLowConfAction}；允许反转=${genAllowReversal ? '是' : '否'}。`,
        logic: '前端本地回退：按当前选项生成默认规则，待后端恢复后可重新生成。',
        basis: `回退原因：${reason}`,
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
      const baseName =
        String(generated.strategy_name || '').trim() ||
        `AI_${genPair}_${habit}_${new Date().toISOString().slice(0, 10)}`
      const name = resolveUniqueGeneratedName(baseName)
      const preferencePrompt = String(generated.preference_prompt || '').trim()
      const generatorPrompt = String(generated.generator_prompt || '').trim()
      const logic = String(generated.logic || '').trim()
      const basis = String(generated.basis || '').trim()

      const rule = {
        id,
        name,
        habit,
        symbol: genPair,
        style: genStyle,
        minRR: clamp(genMinRR, 1.0, 10),
        allowReversal: Boolean(genAllowReversal),
        lowConfAction: genLowConfAction,
        directionBias: genDirectionBias,
        createdAt: new Date().toISOString(),
        prompt: generatorPrompt || promptSettings.strategy_generator_prompt_template,
        preferencePrompt: preferencePrompt || '',
        logic: logic || '按市场状态识别 -> 多因子确认 -> 风控过滤 -> 执行建议生成。',
        basis: basis || '基于实时行情与技术指标综合生成。',
      }
      setGeneratedStrategies((arr) => [rule, ...arr])
      setSelectedRuleId(id)
      setBuilderTab('rules')
      setStrategyGenMode(usedFallback ? 'fallback' : 'llm')
      if (!btStrategy) setBtStrategy(name)
      if (usedFallback) {
        showToast('warning', '策略已生成（智能体未接入或调用失败，使用模板回退）')
      } else {
        showToast('success', baseName !== name ? `策略生成成功（已自动命名为 ${name}）` : '策略生成成功')
      }

    } catch (e) {
      const reason = resolveRequestError(e, '策略生成失败')
      const fallbackRule = buildLocalFallbackRule(reason)
      const uniqueName = resolveUniqueGeneratedName(fallbackRule.name)
      const rule = { ...fallbackRule, name: uniqueName }
      setGeneratedStrategies((arr) => [rule, ...arr])
      setSelectedRuleId(id)
      setBuilderTab('rules')
      setStrategyGenMode('fallback')
      if (!btStrategy) setBtStrategy(uniqueName)
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

  const confirmStrategySelection = () => {
    const normalized = strategyDraft.filter((x) => executionStrategyOptions.includes(x)).slice(0, 3)
    const next = normalized.length ? normalized : (executionStrategyOptions[0] ? [executionStrategyOptions[0]] : [])
    setEnabledStrategies(next)
    if (next.length && !next.includes(activeStrategy)) {
      setActiveStrategy(next[0])
      setLiveStrategyStartedAt(Date.now())
    }
    setStrategyPickerOpen(false)
  }

  const confirmPaperStrategySelection = () => {
    const normalized = paperStrategyDraft.filter((x) => executionStrategyOptions.includes(x)).slice(0, 3)
    const next = normalized.length ? normalized : (executionStrategyOptions[0] ? [executionStrategyOptions[0]] : [])
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

  const renameGeneratedStrategy = () => {
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

    setGeneratedStrategies((arr) => arr.map((s) => (s.id === selectedRule.id ? { ...s, name: uniqueName } : s)))
    setEnabledStrategies((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setStrategyDraft((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setPaperStrategySelection((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setPaperStrategyDraft((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setBtStrategySelection((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setBtStrategyDraft((arr) => arr.map((x) => (x === oldName ? uniqueName : x)))
    setActiveStrategy((v) => (v === oldName ? uniqueName : v))
    setPaperStrategy((v) => (v === oldName ? uniqueName : v))
    setBtStrategy((v) => (v === oldName ? uniqueName : v))
    setRenameRuleName(uniqueName)
    setError('')
    showToast('success', uniqueName !== nextName ? `策略重命名成功（已自动命名为 ${uniqueName}）` : '策略重命名成功')
  }

  const deleteGeneratedStrategy = (ruleID) => {
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
    const remainingExecution = Array.from(new Set([...strategyOptions, ...remainingGeneratedNames]))
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

    setError('')
    showToast('success', '策略规则已删除')
  }

  const uploadStrategy = async () => {
    if (!uploadFile) {
      setError('请先选择 .py 策略文件')
      return
    }
    if (!uploadFile.name.toLowerCase().endsWith('.py')) {
      setError('仅支持上传 .py 文件')
      return
    }
    setUploadingStrategy(true)
    setError('')
    try {
      const res = await uploadStrategyFile(uploadFile)
      setUploadFile(null)
      const available = parseStrategies(res?.data?.available || [])
      if (available.length) {
        setStrategyOptions(available)
      }
      await loadSystemAndStrategies()
    } catch (e) {
      setError(e?.response?.data?.error || e?.message || '上传策略失败')
    } finally {
      setUploadingStrategy(false)
    }
  }

  const loadStrategyTemplate = async () => {
    setLoadingTemplate(true)
    setError('')
    try {
      const res = await getStrategyTemplate()
      const content = String(res?.data?.content || '').trim()
      if (!content) {
        setStrategyTemplate(strategyTemplateFallback)
        setError('模板接口未返回内容，已加载内置模板。请重启 Go 后端后重试。')
        return
      }
      setStrategyTemplate(content)
    } catch (e) {
      setStrategyTemplate(strategyTemplateFallback)
      setError((e?.response?.data?.error || e?.message || '加载模板失败') + '，已回退内置模板')
    } finally {
      setLoadingTemplate(false)
    }
  }

  const copyStrategyTemplate = async () => {
    if (!strategyTemplate.trim()) {
      setError('模板为空，请先加载模板')
      return
    }
    try {
      await navigator.clipboard.writeText(strategyTemplate)
    } catch {
      setError('复制失败，请手动复制')
    }
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
    setStrategyDraft,
    selectedStrategyText,
    executionStrategyOptions,
    strategyDraft,
    toggleStrategyDraft,
    confirmStrategySelection,
    settings,
    setSettings,
    refreshCore,
    runningNow,
    runOneCycle,
    toggleScheduler,
    schedulerRunning,
    savingSettings,
    saveLiveConfig,
    liveViewTab,
    setLiveViewTab,
    liveStrategyLabel,
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
    setUploadFile,
    loadStrategyTemplate,
    loadingTemplate,
    copyStrategyTemplate,
    uploadStrategy,
    uploadingStrategy,
    strategyTemplate,
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
    promptSettings,
    setPromptSettings,
    resetPromptConfig,
    resettingPrompts,
    savePromptConfig,
    savingPrompts,
  }
}
