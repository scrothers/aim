import { useId, useState, type FormEvent } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Search } from 'lucide-react'
import { ApiError } from '@/api/client'
import { searchPageQuery, type SearchPageParams, type SearchState } from '@/api/mail'
import { useMailboxAddresses } from '@/api/queries'
import { ago } from '@/lib/identity'
import { stripMarkdown } from '@/lib/markdown'
import { AddressChip } from '@/components/shared/AddressChip'
import { PageHeader } from '@/components/shared/PageHeader'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { SaveFilterButton } from '@/components/mail/SaveFilter'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Highlight } from './Highlight'

const STATES: Array<{ value: SearchState | ''; label: string }> = [
  { value: '', label: 'Any state' },
  { value: 'inbox', label: 'Inbox' },
  { value: 'unread', label: 'Unread' },
  { value: 'archived', label: 'Archived' },
]

/** URL params for a saved search; /search?q=… reproduces it. */
export function searchFilterParams(p: SearchPageParams): Record<string, string> {
  const out: Record<string, string> = { q: p.q }
  for (const k of ['scope', 'from', 'since', 'until', 'state'] as const) if (p[k]) out[k] = p[k]
  return out
}

const fieldCls = 'flex min-w-0 flex-col gap-1.5 text-[13px] font-semibold'

function Form({ value, onSubmit }: { value: SearchPageParams; onSubmit: (p: SearchPageParams) => void }) {
  const list = useId()
  const addresses = useMailboxAddresses()
  const [d, setD] = useState({ q: value.q, scope: value.scope ?? '', from: value.from ?? '', since: value.since ?? '', until: value.until ?? '', state: value.state ?? '' })
  const set = (k: keyof typeof d) => (e: { target: { value: string } }) => setD((s) => ({ ...s, [k]: e.target.value }))
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const p: SearchPageParams = { q: d.q.trim() }
    if (d.scope.trim()) p.scope = d.scope.trim()
    if (d.from.trim()) p.from = d.from.trim()
    if (d.since) p.since = d.since
    if (d.until) p.until = d.until
    if (d.state) p.state = d.state as SearchState
    onSubmit(p)
  }
  return (
    <form onSubmit={submit} role="search" aria-label="Search mail" className="flex flex-col gap-3 rounded-xl border bg-panel p-4">
      <div className="flex flex-wrap items-end gap-3">
        <label className={`${fieldCls} flex-[2_1_320px]`}>
          Search
          <Input value={d.q} onChange={set('q')} placeholder="Words in subject or body" autoFocus className="max-md:h-11" />
        </label>
        <Button type="submit" disabled={!d.q.trim()} className="max-md:h-11">
          <Search aria-hidden="true" />
          Search
        </Button>
      </div>
      <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-3">
        <label className={fieldCls}>
          Mailbox
          <Input list={list} value={d.scope} onChange={set('scope')} placeholder="Any mailbox" className="font-mono max-md:h-11" />
          <datalist id={list}>
            {addresses.map((a) => (
              <option key={a} value={a} />
            ))}
          </datalist>
        </label>
        <label className={fieldCls}>
          From
          <Input value={d.from} onChange={set('from')} placeholder="Any sender" className="font-mono max-md:h-11" />
        </label>
        <label className={fieldCls}>
          Since
          <Input type="date" value={d.since} onChange={set('since')} className="max-md:h-11" />
        </label>
        <label className={fieldCls}>
          Until
          <Input type="date" value={d.until} onChange={set('until')} className="max-md:h-11" />
        </label>
        <label className={fieldCls}>
          State
          <select
            value={d.state}
            onChange={set('state')}
            className="h-9 rounded-md border border-input bg-transparent px-2 text-sm max-md:h-11"
          >
            {STATES.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
      </div>
    </form>
  )
}

export function SearchPage({ params, onSearch }: { params: SearchPageParams; onSearch: (p: SearchPageParams) => void }) {
  const q = useInfiniteQuery(searchPageQuery(params))
  const hits = q.data?.pages.flatMap((p) => p.hits ?? []) ?? []
  const active = params.q.trim() !== ''
  // Remount the form when the URL changes elsewhere (saved view, back button).
  const formKey = JSON.stringify(params)
  return (
    <section aria-label="Search" className="box-border flex min-w-0 flex-1 flex-col gap-4 p-6 max-md:p-4">
      <PageHeader
        title="Search all mail"
        actions={active && <SaveFilterButton label="Save search" params={searchFilterParams(params)} suggestion={params.q} size="default" />}
      />
      <Form key={formKey} value={params} onSubmit={onSearch} />
      {!active ? (
        <p className="m-0 text-[13px] text-faint">Enter words to search every mailbox, including archived mail.</p>
      ) : q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          {q.error instanceof ApiError ? q.error.message : 'Search failed.'}
        </p>
      ) : q.isPending ? (
        <p className="m-0 text-faint">Searching…</p>
      ) : hits.length === 0 ? (
        <p className="m-0 text-[13px] text-faint">No messages match.</p>
      ) : (
        <>
          <ul aria-label="Search results" className="m-0 flex list-none flex-col overflow-hidden rounded-xl border bg-panel p-0">
            {hits.map((h) => (
              <li key={`${h.message_id}/${h.mailbox}`} data-testid="search-hit" className="border-b last:border-b-0">
                <Link
                  to="/mail"
                  search={{ mailbox: h.mailbox, state: 'all', thread: h.thread_id }}
                  className="flex items-start gap-3 px-4 py-3 hover:bg-hover max-md:min-h-11"
                >
                  <RobotAvatar address={h.from} />
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="flex flex-wrap items-baseline gap-x-2">
                      <AddressChip address={h.from} className="min-w-0 truncate" />
                      <span className="text-xs text-faint">to</span>
                      <span data-testid="hit-mailbox" className="min-w-0 truncate rounded bg-code px-1.5">
                        <AddressChip address={h.mailbox} />
                      </span>
                      <span className="flex-1" />
                      <span className="shrink-0 text-xs text-faint">{ago(h.created_at)}</span>
                    </span>
                    <span className="truncate font-semibold">{h.subject}</span>
                    <span data-testid="hit-snippet" className="text-[13px] break-words text-muted-foreground">
                      {(h.segments ?? []).length > 0 ? <Highlight segments={h.segments ?? []} /> : stripMarkdown(h.snippet)}
                    </span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
          {q.hasNextPage && (
            <Button variant="outline" className="self-start max-md:h-11" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
              {q.isFetchingNextPage ? 'Loading…' : 'Load more'}
            </Button>
          )}
        </>
      )}
    </section>
  )
}
