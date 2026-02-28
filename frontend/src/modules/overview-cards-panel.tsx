// @ts-nocheck
import { fmtNum } from '@/modules/format'
import { BinanceAdvancedChart } from '@/modules/charts'

export function OverviewCardsPanel({
  pair,
  strategyName,
  marketEmotion,
  totalPnL,
  account,
  strategyDurationText,
  pnlRatio,
  extra = null,
  resolvedTheme,
  activeExchangeType = 'binance',
}) {
  const exchangeLabel = String(activeExchangeType || '').toLowerCase() === 'okx' ? 'OKX' : 'Binance'
  return (
    <div className="builder-pane">
      <div className="overview-grid">
        <article className="metric-card"><h4>今日市场情绪</h4><p>{marketEmotion}</p></article>
        <article className="metric-card"><h4>总盈亏</h4><p className={totalPnL >= 0 ? 'up' : 'down'}>{fmtNum(totalPnL, 2)} USDT</p></article>
        <article className="metric-card"><h4>账户信息</h4><p>余额 {fmtNum(account?.balance, 2)} / 持仓 {account?.position?.side || '无'}</p></article>
        <article className="metric-card"><h4>当前策略交易时长</h4><p>{strategyDurationText}</p></article>
        <article className="metric-card"><h4>盈亏比</h4><p>{pnlRatio}</p></article>
        <article className="metric-card"><h4>当前策略</h4><p>{strategyName || '-'}</p></article>
      </div>
      {extra}
      <section className="sub-window kline-card">
        <div className="card-head">
          <h3>K 线图</h3>
          <span>{pair} · {exchangeLabel} 实时K线 · EMA(7/25/99)</span>
        </div>
        <BinanceAdvancedChart key={`${pair}-${resolvedTheme}-${exchangeLabel}`} symbol={pair} theme={resolvedTheme} exchange={activeExchangeType} />
      </section>
    </div>
  )
}
