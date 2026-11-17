import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { orgChartQuery, type OrgChartNode } from '@/api/roster'
import { OPERATOR } from '@/lib/address'
import { layoutOrgChart } from '@/lib/orgchart'
import { parseAddress } from '@/lib/identity'
import { PresenceDot } from '@/components/shared/PresenceDot'
import { RobotAvatar } from '@/components/shared/RobotAvatar'

function NodeCard({ node }: { node: OrgChartNode }) {
  const who = parseAddress(node.address)
  const online = node.online ?? []
  return (
    <span className="flex min-w-0 items-center gap-2 text-center">
      <RobotAvatar address={node.address} size={32} />
      <span className="flex min-w-0 flex-col items-center gap-0.5">
        <span className="flex items-center gap-1.5">
          <PresenceDot online={online.length > 0} idle={online.length > 0 && online.every((o) => o.state === 'idle')} />
          <b className="truncate">{who.identity ?? node.address}</b>
        </span>
        <span className="max-w-full truncate font-mono text-xs text-muted-foreground">
          {who.path}
          {node.role ? ` · ${node.role}` : ''}
        </span>
      </span>
    </span>
  )
}

export function OrgChart() {
  const q = useQuery(orgChartQuery())
  const nodes = useMemo(() => (q.data?.nodes ?? []).filter((n) => !n.operator && n.address !== OPERATOR), [q.data])
  const byAddress = useMemo(() => new Map(nodes.map((n) => [n.address, n])), [nodes])
  const layout = useMemo(() => layoutOrgChart(nodes), [nodes])

  const box = useRef<HTMLDivElement>(null)
  const [boxW, setBoxW] = useState(0)
  useEffect(() => {
    const el = box.current
    if (!el) return
    const measure = () => setBoxW(el.clientWidth)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  // Center a narrow tree; a wide one scrolls.
  const offset = Math.max(0, Math.floor((boxW - layout.width) / 2))

  return (
    <section aria-label="Org chart" className="box-border flex min-w-0 flex-1 flex-col gap-2 p-6">
      <h1 className="m-0 text-[22px] leading-[30px] font-semibold">Org chart</h1>
      <p className="m-0 text-[13px] text-muted-foreground">Lines follow reports_to. Escalations climb toward operator.</p>
      {q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          Could not load the org chart.
        </p>
      ) : q.isPending ? (
        <p className="m-0 text-faint">Loading…</p>
      ) : (
        <div ref={box} data-testid="org-chart-scroll" className="min-h-0 flex-1 overflow-auto rounded-xl border bg-panel2">
          <div className="relative" style={{ width: layout.width + offset, height: layout.height }}>
            <svg aria-hidden="true" width={layout.width} height={layout.height} className="absolute top-0 fill-none stroke-faint" style={{ left: offset, strokeWidth: 2, strokeLinejoin: 'round' }}>
              {layout.edges.map((e) => (
                <path key={`${e.from}>${e.to}`} d={e.d} data-edge={`${e.from}>${e.to}`} />
              ))}
            </svg>
            {layout.nodes.map((p) => {
              const style = { left: p.x + offset, top: p.y, width: p.w, height: p.h }
              const node = byAddress.get(p.id)
              if (!node) {
                return (
                  <div
                    key={p.id}
                    data-node={p.id}
                    style={style}
                    className="absolute box-border flex items-center justify-center rounded-xl bg-text p-2 font-mono font-medium text-panel"
                  >
                    operator
                  </div>
                )
              }
              return (
                <Link
                  key={p.id}
                  to="/roster"
                  search={{ edit: p.id }}
                  data-node={p.id}
                  data-orphan={p.orphan || undefined}
                  aria-label={`Edit ${p.id}`}
                  style={style}
                  className="absolute box-border flex items-center justify-center rounded-xl border bg-panel px-3 py-2 text-text no-underline hover:bg-hover"
                >
                  <NodeCard node={node} />
                </Link>
              )
            })}
          </div>
        </div>
      )}
    </section>
  )
}
