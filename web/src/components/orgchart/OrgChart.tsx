import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Maximize2, Minus, Network, Plus } from 'lucide-react'
import { orgChartQuery, type OrgChartNode } from '@/api/roster'
import { OPERATOR } from '@/lib/address'
import { layoutOrgChart, OPERATOR_ID } from '@/lib/orgchart'
import { parseAddress } from '@/lib/identity'
import { EmptyState } from '@/components/shared/EmptyState'
import { PageHeader } from '@/components/shared/PageHeader'
import { PresenceDot } from '@/components/shared/PresenceDot'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { Button } from '@/components/ui/button'

const ZOOM_MIN = 0.25
const ZOOM_MAX = 1.5
const ZOOM_STEP = 0.25

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

  // The scroller only exists once the chart has loaded, so track it as state rather than a ref.
  const [box, setBox] = useState<HTMLDivElement | null>(null)
  const [boxW, setBoxW] = useState(0)
  const [scale, setScale] = useState(1)
  useEffect(() => {
    if (!box) return
    const measure = () => setBoxW(box.clientWidth)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(box)
    return () => ro.disconnect()
  }, [box])
  const scaledW = layout.width * scale
  // Center a narrow tree; a wide one scrolls.
  const offset = Math.max(0, Math.floor((boxW - scaledW) / 2))

  // Open on the root, not an arbitrary slice of a wide tree. Re-centre when the zoom or the tree changes.
  const measured = boxW > 0
  const root = layout.nodes.find((n) => n.id === OPERATOR_ID)
  const rootCenter = root ? (root.x + root.w / 2) * scale : 0
  useEffect(() => {
    if (!box || !measured) return
    box.scrollTo?.({ left: Math.max(0, Math.round(rootCenter + Math.max(0, Math.floor((box.clientWidth - scaledW) / 2)) - box.clientWidth / 2)), top: 0 })
  }, [box, measured, rootCenter, scaledW])

  const clamp = (s: number) => Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(s * 100) / 100))
  const fit = () => setScale(clamp(boxW > 0 ? Math.min(1, (boxW - 2) / layout.width) : 1))

  return (
    <section aria-label="Org chart" className="box-border flex min-w-0 flex-1 flex-col gap-4 p-6 max-md:p-4">
      <PageHeader
        title="Org chart"
        description="Lines follow reports_to. Escalations climb toward operator."
        actions={
          <div role="group" aria-label="Zoom" className="flex items-center gap-1">
            <Button variant="outline" size="icon" aria-label="Zoom out" disabled={scale <= ZOOM_MIN} onClick={() => setScale((s) => clamp(s - ZOOM_STEP))}>
              <Minus aria-hidden="true" />
            </Button>
            <output aria-label="Zoom level" className="min-w-11 text-center font-mono text-xs text-muted-foreground">
              {Math.round(scale * 100)}%
            </output>
            <Button variant="outline" size="icon" aria-label="Zoom in" disabled={scale >= ZOOM_MAX} onClick={() => setScale((s) => clamp(s + ZOOM_STEP))}>
              <Plus aria-hidden="true" />
            </Button>
            <Button variant="outline" onClick={fit}>
              <Maximize2 aria-hidden="true" />
              Fit width
            </Button>
            <Button variant="outline" disabled={scale === 1} onClick={() => setScale(1)}>
              Reset
            </Button>
          </div>
        }
      />
      {q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          Could not load the org chart.
        </p>
      ) : q.isPending ? (
        <p className="m-0 text-faint">Loading…</p>
      ) : (
        <>
        {nodes.length === 0 && (
          <EmptyState icon={Network}>No identities registered yet. The chart fills in as agents register and name who they report to.</EmptyState>
        )}
        <div ref={setBox} data-testid="org-chart-scroll" className="min-h-0 flex-1 overflow-auto rounded-xl border bg-panel2">
          <div className="relative" style={{ width: scaledW + offset, height: layout.height * scale }}>
            <div className="absolute top-0 origin-top-left" style={{ left: offset, width: layout.width, height: layout.height, transform: `scale(${scale})` }}>
            <svg aria-hidden="true" width={layout.width} height={layout.height} className="absolute top-0 fill-none stroke-faint" style={{ left: 0, strokeWidth: 2, strokeLinejoin: 'round' }}>
              {layout.edges.map((e) => (
                <path key={`${e.from}>${e.to}`} d={e.d} data-edge={`${e.from}>${e.to}`} />
              ))}
            </svg>
            {layout.nodes.map((p) => {
              const style = { left: p.x, top: p.y, width: p.w, height: p.h }
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
        </div>
        </>
      )}
    </section>
  )
}
