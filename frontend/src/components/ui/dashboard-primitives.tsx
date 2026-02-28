import { Fragment, useEffect, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { cn } from '@/lib/utils'
import { Button as ShadButton } from '@/components/ui/button'
import {
  Select as ShadSelect,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tabs as ShadTabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export function ConfigProvider({ children }) {
  return <>{children}</>
}

export function AntdApp({ children }) {
  return <>{children}</>
}

export const Layout = Object.assign(
  function LayoutRoot({ className, children }) {
    return <div className={cn('min-h-screen w-full', className)}>{children}</div>
  },
  {
    Sider: function LayoutSider({ className, width, children }) {
      const siderStyle = width ? { width, minWidth: width, flex: `0 0 ${width}px` } : undefined
      return (
        <aside className={cn('shad-layout-sider', className)} style={siderStyle}>
          {children}
        </aside>
      )
    },
    Header: function LayoutHeader({ className, children }) {
      return <header className={cn('shad-layout-header', className)}>{children}</header>
    },
    Content: function LayoutContent({ className, children }) {
      return <main className={cn('shad-layout-content', className)}>{children}</main>
    },
  },
)

export function Menu({ className, selectedKeys = [], items = [], onClick }) {
  const selectedSet = new Set((selectedKeys || []).map((x) => String(x)))
  return (
    <ul className={cn('space-y-1 dashboard-dir-menu', className)}>
      {(items || []).map((item) => {
        const key = String(item?.key || '')
        const selected = selectedSet.has(key)
        return (
          <li key={key}>
            <button
              type="button"
              className={cn(
                'dashboard-dir-item flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors',
                selected
                  ? 'dashboard-dir-item-selected bg-primary/15 text-primary'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
              onClick={() => onClick?.({ key })}
            >
              {item?.icon ? <span className="dashboard-dir-item-icon inline-flex items-center">{item.icon}</span> : null}
              <span className="dashboard-dir-item-label">{item?.label}</span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}

export function Select({ className, value, options = [], onChange, size = 'middle', disabled = false }) {
  const triggerSize = size === 'small' ? 'h-9' : 'h-10'
  return (
    <ShadSelect value={String(value ?? '')} onValueChange={(v) => onChange?.(v)} disabled={disabled}>
      <SelectTrigger className={cn(triggerSize, className)}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {(options || []).map((opt) => (
          <SelectItem key={String(opt?.value)} value={String(opt?.value)}>
            {opt?.label}
          </SelectItem>
        ))}
      </SelectContent>
    </ShadSelect>
  )
}

export function Space({ children, wrap = false }) {
  return (
    <div className={cn('flex items-center gap-2', wrap && 'flex-wrap')}>
      {children}
    </div>
  )
}

export function Tabs({ className, activeKey, onChange, items = [] }) {
  const safeItems = Array.isArray(items) ? items : []
  const resolvedKey = String(activeKey ?? safeItems[0]?.key ?? '')
  return (
    <ShadTabs value={resolvedKey} onValueChange={(v) => onChange?.(v)} className={cn('dashboard-tabs', className)}>
      <TabsList className="dashboard-tabs-list">
        {safeItems.map((item) => (
          <TabsTrigger key={String(item?.key)} value={String(item?.key)} className="dashboard-tabs-trigger">
            {item?.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {safeItems.map((item) => (
        <TabsContent key={String(item?.key)} value={String(item?.key)} className="dashboard-tabs-content">
          {item?.children}
        </TabsContent>
      ))}
    </ShadTabs>
  )
}

export function Table({ className, columns = [], dataSource = [], pagination, scroll, expandable }) {
  const pageSize = Number(pagination?.pageSize || 0)
  const paged = pageSize > 0
  const [page, setPage] = useState(1)
  const [expanded, setExpanded] = useState({})

  useEffect(() => {
    setPage(1)
    setExpanded({})
  }, [dataSource, pageSize])

  const totalPages = paged ? Math.max(1, Math.ceil(dataSource.length / pageSize)) : 1
  const safePage = Math.min(page, totalPages)
  const rows = useMemo(
    () => (paged ? dataSource.slice((safePage - 1) * pageSize, safePage * pageSize) : dataSource),
    [paged, safePage, pageSize, dataSource],
  )
  const minWidth = typeof scroll?.x === 'number' ? `${scroll.x}px` : undefined
  const bodyStyle: CSSProperties | undefined = scroll?.y
    ? { maxHeight: Number(scroll.y), overflowY: 'auto' }
    : undefined

  const toggleExpand = (rowKey) => {
    setExpanded((prev) => ({ ...prev, [rowKey]: !prev[rowKey] }))
  }

  return (
    <div className={cn('dashboard-table-wrap rounded-lg border bg-card', className)}>
      <div className="overflow-x-auto">
        <div style={bodyStyle}>
          <table className="dashboard-table w-full text-sm" style={minWidth ? { minWidth } : undefined}>
            <thead className="dashboard-table-head">
              <tr className="border-b bg-muted/30">
                {columns.map((col, colIdx) => (
                  <th key={String(col?.key || col?.dataIndex || colIdx)} className="px-3 py-2 text-left font-semibold">
                    {col?.title}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="dashboard-table-body">
              {rows.map((row, rowIdx) => {
                const rowKey = String(row?.key || `row-${rowIdx}`)
                const canExpand = Boolean(expandable?.expandedRowRender)
                const rowExpanded = Boolean(expanded[rowKey])
                return (
                  <Fragment key={rowKey}>
                    <tr
                      className={cn(
                        'dashboard-table-row border-b transition-colors',
                        canExpand && expandable?.expandRowByClick && 'cursor-pointer hover:bg-muted/40',
                      )}
                      onClick={() => {
                        if (canExpand && expandable?.expandRowByClick) toggleExpand(rowKey)
                      }}
                    >
                      {columns.map((col, colIdx) => {
                        const value = col?.dataIndex ? row?.[col.dataIndex] : undefined
                        const content = col?.render ? col.render(value, row, rowIdx) : value
                        return (
                          <td key={`${rowKey}-${String(col?.key || col?.dataIndex || colIdx)}`} className="px-3 py-2 align-top">
                            {content}
                          </td>
                        )
                      })}
                    </tr>
                    {canExpand && rowExpanded ? (
                      <tr className="dashboard-table-expanded-row border-b bg-muted/20">
                        <td colSpan={columns.length} className="px-3 py-3">
                          {expandable.expandedRowRender(row)}
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
      {paged && totalPages > 1 ? (
        <div className="flex items-center justify-end gap-2 border-t px-3 py-2">
          <ShadButton
            size="sm"
            variant="outline"
            disabled={safePage <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            上一页
          </ShadButton>
          <span className="text-sm text-muted-foreground">{safePage} / {totalPages}</span>
          <ShadButton
            size="sm"
            variant="outline"
            disabled={safePage >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            下一页
          </ShadButton>
        </div>
      ) : null}
    </div>
  )
}

export function Modal({ open, title, onCancel, footer, children }) {
  return (
    <Dialog open={Boolean(open)} onOpenChange={(next) => { if (!next) onCancel?.() }}>
      <DialogContent className="sm:max-w-2xl">
        {title ? (
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
        ) : null}
        {children}
        {footer ? <div className="flex justify-end gap-2 pt-2">{footer}</div> : null}
      </DialogContent>
    </Dialog>
  )
}
