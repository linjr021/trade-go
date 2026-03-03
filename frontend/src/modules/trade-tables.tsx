import { Table } from '@/components/ui/dashboard-primitives'
import { fmtNum, fmtTime } from '@/modules/format'

export function StrategyBacktestTable({ records }) {
  if (!records.length) return <p className="muted">暂无模拟记录</p>
  const getOrderBasis = (record) => String(record?.order_basis || record?.orderBasis || '').trim() || '-'
  const getStopLoss = (record) => (record?.stop_loss ?? record?.stopLoss)
  const getTakeProfit = (record) => (record?.take_profit ?? record?.takeProfit)
  const fmtDetailPrice = (value) => {
    const n = Number(value)
    if (!Number.isFinite(n) || n <= 0) return '-'
    return fmtNum(n, 6)
  }
  const columns = [
    { title: '时间', dataIndex: 'ts', key: 'ts', render: (v) => fmtTime(v) },
    { title: '方向', dataIndex: 'side', key: 'side' },
    { title: '信心', dataIndex: 'confidence', key: 'confidence', render: (v) => v || '-' },
    {
      title: '张数',
      dataIndex: 'size',
      key: 'size',
      render: (v) => (v === undefined || v === null ? '-' : fmtNum(v, 2)),
    },
    {
      title: '杠杆',
      dataIndex: 'leverage',
      key: 'leverage',
      render: (v) => (v ? `${v}x` : '-'),
    },
    { title: '入场', dataIndex: 'entry', key: 'entry', render: (v) => fmtNum(v, 2) },
    { title: '出场', dataIndex: 'exit', key: 'exit', render: (v) => fmtNum(v, 2) },
    {
      title: '盈亏',
      dataIndex: 'pnl',
      key: 'pnl',
      render: (v) => <span className={Number(v || 0) >= 0 ? 'up' : 'down'}>{fmtNum(v, 2)}</span>,
    },
  ]
  const dataSource = records.map((r, i) => ({ ...r, key: r.id || `${r.ts || 'ts'}-${i}` }))
  return (
    <div className="table-wrap">
      <Table
        className="dashboard-data-table"
        size="small"
        columns={columns}
        dataSource={dataSource}
        pagination={{ pageSize: 200, showSizeChanger: false }}
        scroll={{ x: 900, y: 560 }}
        expandable={{
          expandRowByClick: true,
          showExpandColumn: false,
          expandedRowRender: (record) => (
            <div className="bt-order-detail">
              <div className="bt-order-detail-grid">
                <div className="bt-order-item">
                  <span>下单依据</span>
                  <b>{getOrderBasis(record)}</b>
                </div>
                <div className="bt-order-item">
                  <span>下单价</span>
                  <b>{fmtDetailPrice(record?.entry)}</b>
                </div>
                <div className="bt-order-item">
                  <span>止损价</span>
                  <b>{fmtDetailPrice(getStopLoss(record))}</b>
                </div>
                <div className="bt-order-item">
                  <span>止盈价</span>
                  <b>{fmtDetailPrice(getTakeProfit(record))}</b>
                </div>
              </div>
            </div>
          ),
        }}
      />
    </div>
  )
}

export function TradeRecordsTable({ records }) {
  if (!records.length) return <p className="muted">暂无交易记录</p>
  const fmtDetailPrice = (value) => {
    const n = Number(value)
    if (!Number.isFinite(n) || n <= 0) return '-'
    return fmtNum(n, 6)
  }
  const normalizedRows = records.map((r, i) => ({
    ...r,
    __idx: i,
    __tsMs: Date.parse(String(r?.ts || '')) || 0,
  }))
  const rowsByTimeAsc = [...normalizedRows].sort((a, b) => a.__tsMs - b.__tsMs)
  const inferredCloseMap = new Map()
  for (let i = 0; i < rowsByTimeAsc.length; i += 1) {
    const row = rowsByTimeAsc[i]
    const signal = String(row?.signal || '').toUpperCase()
    const approved = row?.approved === undefined ? signal !== 'HOLD' : Boolean(row?.approved)
    if (!approved || (signal !== 'BUY' && signal !== 'SELL')) continue
    const opposite = signal === 'BUY' ? 'SELL' : 'BUY'
    for (let j = i + 1; j < rowsByTimeAsc.length; j += 1) {
      const next = rowsByTimeAsc[j]
      const nextSignal = String(next?.signal || '').toUpperCase()
      const nextApproved = next?.approved === undefined ? nextSignal !== 'HOLD' : Boolean(next?.approved)
      if (!nextApproved || nextSignal !== opposite) continue
      const px = Number(next?.close_price ?? next?.exit_price ?? next?.exit ?? next?.price ?? 0)
      if (Number.isFinite(px) && px > 0) {
        inferredCloseMap.set(row.__idx, px)
      }
      break
    }
  }
  const pickOpenPrice = (row) => {
    const px = Number(row?.open_price ?? row?.entry_price ?? row?.entry ?? row?.price ?? 0)
    return Number.isFinite(px) && px > 0 ? px : 0
  }
  const pickClosePrice = (row) => {
    const direct = Number(row?.close_price ?? row?.exit_price ?? row?.exit ?? 0)
    if (Number.isFinite(direct) && direct > 0) return direct
    const inferred = Number(inferredCloseMap.get(row.__idx) || 0)
    return Number.isFinite(inferred) && inferred > 0 ? inferred : 0
  }
  const columns = [
    { title: '时间', dataIndex: 'ts', key: 'ts', render: (v) => fmtTime(v) },
    { title: '交易对', dataIndex: 'symbol', key: 'symbol', render: (v) => v || '-' },
    {
      title: '方向',
      dataIndex: 'signal',
      key: 'signal',
      render: (v) => String(v || '-').toUpperCase(),
    },
    { title: '开单', dataIndex: 'approved', key: 'approved', render: (v) => (v ? '已开单' : '未开单') },
    { title: '数量', dataIndex: 'approved_size', key: 'approved_size', render: (v) => fmtNum(v, 2) },
    { title: '价格', dataIndex: 'price', key: 'price', render: (v) => fmtNum(v, 2) },
    {
      title: '盈亏',
      dataIndex: 'unrealized_pnl',
      key: 'unrealized_pnl',
      render: (v) => <span className={Number(v || 0) >= 0 ? 'up' : 'down'}>{fmtNum(v, 2)}</span>,
    },
  ]
  const dataSource = normalizedRows.map((r) => ({
    ...r,
    key: r.id || `${r.ts || 'ts'}-${r.symbol || 'sym'}-${r.signal || 'sig'}`,
  }))
  return (
    <div className="table-wrap">
      <Table
        className="dashboard-data-table"
        size="small"
        columns={columns}
        dataSource={dataSource}
        pagination={{ pageSize: 12, showSizeChanger: false }}
        scroll={{ x: 860 }}
        expandable={{
          expandRowByClick: true,
          showExpandColumn: false,
          expandedRowRender: (record) => (
            <div className="bt-order-detail">
              <div className="bt-order-detail-grid">
                <div className="bt-order-item">
                  <span>下单策略</span>
                  <b>{String(record?.strategy_combo || record?.strategyCombo || '-')}</b>
                </div>
                <div className="bt-order-item">
                  <span>开仓价</span>
                  <b>{fmtDetailPrice(pickOpenPrice(record))}</b>
                </div>
                <div className="bt-order-item">
                  <span>平仓价</span>
                  <b>{fmtDetailPrice(pickClosePrice(record))}</b>
                </div>
                <div className="bt-order-item">
                  <span>止损价</span>
                  <b>{fmtDetailPrice(record?.stop_loss ?? record?.stopLoss)}</b>
                </div>
                <div className="bt-order-item">
                  <span>止盈价</span>
                  <b>{fmtDetailPrice(record?.take_profit ?? record?.takeProfit)}</b>
                </div>
              </div>
            </div>
          ),
        }}
      />
    </div>
  )
}
