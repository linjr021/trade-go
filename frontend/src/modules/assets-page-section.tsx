// @ts-nocheck
import { Select } from '@/components/ui/dashboard-primitives'
import { fmtNum, fmtPct } from '@/modules/format'
import { MonthSelect } from '@/modules/month-select'
import { AssetDistributionChart, AssetTrendChart, PnLCalendar } from '@/modules/charts'

export function AssetsPageSection({
  assetOverview,
  assetDistribution,
  assetMonth,
  setAssetMonth,
  assetCalendar,
  assetRange,
  setAssetRange,
  assetTrend,
  minMonth,
  maxMonth,
}) {
  return (
    <section className="stack">
      <div className="asset-layout">
        <section className="card asset-total-card">
          <div className="asset-total-head">
            <h3>交易所账户总资金</h3>
            <span className="asset-status-pill">实时</span>
          </div>
          <div className="asset-convert-label">资产折合</div>
          <div className="asset-convert-value">
            <span className="asset-convert-prefix">≈</span>
            {fmtNum(assetOverview.total_funds, 2)}
            <span className="asset-convert-unit">USDT</span>
          </div>
          <div className="asset-kpi-grid">
            <article className="asset-kpi-item">
              <span>今日盈亏</span>
              <b className={Number(assetOverview.today_pnl_amount || 0) >= 0 ? 'up' : 'down'}>
                {Number(assetOverview.today_pnl_amount || 0) > 0 ? '+' : ''}{fmtNum(assetOverview.today_pnl_amount, 2)}
              </b>
              <em>{fmtPct(assetOverview.today_pnl_pct)}</em>
            </article>
            <article className="asset-kpi-item">
              <span>累计盈亏</span>
              <b className={Number(assetOverview.cumulative_pnl || 0) >= 0 ? 'up' : 'down'}>
                {Number(assetOverview.cumulative_pnl || 0) > 0 ? '+' : ''}{fmtNum(assetOverview.cumulative_pnl, 2)}
              </b>
              <em>{fmtPct(assetOverview.cumulative_pnl_pct)}</em>
            </article>
          </div>
        </section>
        <section className="card asset-distribution-card">
          <div className="card-head asset-card-head">
            <div>
              <h3>资产分布图</h3>
              <p>查看当前资金占比结构</p>
            </div>
          </div>
          <AssetDistributionChart items={assetDistribution} />
        </section>
        <section className="card asset-equal-card asset-calendar-card">
          <div className="card-head asset-card-head">
            <div>
              <h3>盈亏日历</h3>
              <p>按日查看盈亏金额</p>
            </div>
            <MonthSelect
              value={assetMonth}
              min={minMonth}
              max={maxMonth}
              onChange={setAssetMonth}
            />
          </div>
          <PnLCalendar month={assetMonth} days={assetCalendar} />
        </section>
        <section className="card asset-equal-card asset-trend-card">
          <div className="card-head asset-card-head">
            <div>
              <h3>资产趋势</h3>
              <p>区间净值变化走势</p>
            </div>
            <Select
              size="small"
              className="range-select"
              value={assetRange}
              options={['7D', '30D', '3M', '6M', '1Y'].map((r) => ({ value: r, label: r }))}
              onChange={setAssetRange}
            />
          </div>
          <AssetTrendChart points={assetTrend} range={assetRange} />
        </section>
      </div>
    </section>
  )
}
