import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Pencil } from 'lucide-react'
import { rosterQuery } from '@/api/mail'
import { useRetire, type RosterEntry } from '@/api/roster'
import { ago, parseAddress, runtimeFor } from '@/lib/identity'
import { AddressChip } from '@/components/shared/AddressChip'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { PresenceDot } from '@/components/shared/PresenceDot'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { EditSheet } from './EditSheet'

/** Groups entries by their org/repo path, sorted. */
export function groupByPath(items: RosterEntry[]): Array<[string, RosterEntry[]]> {
  const m = new Map<string, RosterEntry[]>()
  for (const e of items) {
    const p = parseAddress(e.address).path
    m.set(p, [...(m.get(p) ?? []), e])
  }
  return [...m.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([p, es]) => [p, es.sort((a, b) => a.address.localeCompare(b.address))])
}

function RosterCard({
  entry,
  onEdit,
  onRetire,
}: {
  entry: RosterEntry
  onEdit: () => void
  onRetire: () => void
}) {
  const online = entry.online ?? []
  const subagents = entry.subagents ?? []
  const retired = !!entry.retired_at
  return (
    <li
      data-testid="roster-card"
      data-retired={retired || undefined}
      className="flex min-w-0 flex-col gap-3 rounded-xl border bg-panel p-4 data-[retired]:opacity-70"
    >
      <div className="flex items-start gap-3">
        <RobotAvatar address={entry.address} client={online[0]?.client} size={40} />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <AddressChip address={entry.address} className="min-w-0 flex-wrap break-all" />
          <div className="flex flex-wrap items-center gap-2">
            {entry.role && <Badge variant="secondary">{entry.role}</Badge>}
            {retired && <Badge variant="outline">Retired {ago(entry.retired_at)} ago</Badge>}
          </div>
        </div>
      </div>
      {entry.description && <p className="m-0 text-[13px] leading-5 whitespace-pre-wrap text-muted-foreground">{entry.description}</p>}
      {entry.reports_to && (
        <div className="flex flex-wrap items-center gap-1.5 text-xs text-faint">
          <span>reports to</span>
          <AddressChip address={entry.reports_to} className="rounded-full bg-accent-soft px-2 py-0.5" />
        </div>
      )}
      <ul className="m-0 flex list-none flex-col gap-1 p-0 text-xs text-muted-foreground" aria-label="Presence">
        {online.length === 0 ? (
          <li className="flex items-center gap-2 text-faint">
            <PresenceDot online={false} />
            Offline
          </li>
        ) : (
          online.map((o) => (
            <li key={o.session_id} className="flex items-center gap-2">
              <PresenceDot online />
              <span className="font-mono">
                {o.machine} · {runtimeFor(o.client).label}
              </span>
              {o.away && <span className="truncate text-faint">“{o.away}”</span>}
            </li>
          ))
        )}
        {subagents.length > 0 && (
          <li className="text-faint">
            Subagents: <span className="font-mono">{subagents.join(', ')}</span>
          </li>
        )}
      </ul>
      <div className="mt-auto flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil aria-hidden="true" />
          Edit
        </Button>
        <Button variant="ghost" size="sm" onClick={onRetire}>
          {retired ? 'Unretire' : 'Retire'}
        </Button>
      </div>
    </li>
  )
}

export function RosterGrid({ editing, onEdit }: { editing?: string; onEdit: (address?: string) => void }) {
  const [showRetired, setShowRetired] = useState(false)
  const [confirm, setConfirm] = useState<RosterEntry>()
  const q = useQuery(rosterQuery(showRetired))
  const retire = useRetire()
  const items = q.data?.items ?? []
  const editEntry = editing ? items.find((e) => e.address === editing) : undefined

  return (
    <section aria-label="Roster" className="box-border flex min-w-0 flex-1 flex-col gap-4 p-6">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="m-0 flex-1 text-[22px] leading-[30px] font-semibold">Roster</h1>
        <label className="flex items-center gap-2 text-[13px] text-muted-foreground">
          <input type="checkbox" checked={showRetired} onChange={(e) => setShowRetired(e.target.checked)} />
          Show retired
        </label>
      </header>
      {q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          Could not load the roster.
        </p>
      ) : q.isPending ? (
        <p className="m-0 text-faint">Loading…</p>
      ) : items.length === 0 ? (
        <p className="m-0 text-faint">No identities registered yet.</p>
      ) : (
        groupByPath(items).map(([path, entries]) => (
          <div key={path} className="flex flex-col gap-2">
            <h2 className="m-0 font-mono text-[13px] font-semibold text-muted-foreground">{path || 'global'}</h2>
            <ul className="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-3 p-0">
              {entries.map((e) => (
                <RosterCard
                  key={e.address}
                  entry={e}
                  onEdit={() => onEdit(e.address)}
                  onRetire={() => (e.retired_at ? retire.mutate({ address: e.address, retire: false }) : setConfirm(e))}
                />
              ))}
            </ul>
          </div>
        ))
      )}
      <EditSheet entry={editEntry} roster={items} onClose={() => onEdit(undefined)} />
      <Dialog open={!!confirm} onOpenChange={(o) => !o && setConfirm(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Retire identity?</DialogTitle>
            <DialogDescription>
              {confirm?.address} will stop appearing in the roster and cannot be signed on until it is unretired. Its mail is kept.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirm(undefined)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (confirm) retire.mutate({ address: confirm.address, retire: true })
                setConfirm(undefined)
              }}
            >
              Retire
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
