import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'
import { ago, parseAddress } from '@/lib/identity'
import { rowKey, type Header, type MailState } from '@/api/mail'
import { AddressChip } from '@/components/shared/AddressChip'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export interface MessageListProps {
  title: string
  /** Mailbox view: the title is shown as an address chip. */
  titleAddress?: string
  /** The list spans several mailboxes: label each row with the mailbox it was delivered to. */
  showMailbox?: boolean
  items: Header[]
  state: MailState
  /** View forces its own state filter (e.g. "Unread everywhere"). */
  lockedState?: boolean
  showUnread: boolean
  selectedThread?: string
  cursorId?: string
  rangeNote?: string
  loading: boolean
  error?: string
  hasNext: boolean
  loadingMore: boolean
  now?: number
  onSelect: (h: Header) => void
  onState: (s: MailState) => void
  onLoadMore: () => void
}

export function MessageList(p: MessageListProps) {
  const cursorRef = useRef<HTMLLIElement | null>(null)
  useEffect(() => {
    cursorRef.current?.scrollIntoView?.({ block: 'nearest' })
  }, [p.cursorId])

  return (
    <section
      aria-label="Message list"
      className="box-border flex min-w-0 flex-[1_1_340px] flex-col border-r bg-panel text-[13px]"
    >
      <header className="flex flex-col gap-2 border-b px-4 pt-3 pb-2">
        <h2 className="m-0 min-w-0 truncate text-sm font-semibold">
          {p.titleAddress ? <AddressChip address={p.titleAddress} /> : p.title}
        </h2>
        {!p.lockedState && (
          <Tabs value={p.state} onValueChange={(v) => p.onState(v as MailState)}>
            <TabsList>
              <TabsTrigger value="inbox">Inbox</TabsTrigger>
              <TabsTrigger value="unread">Unread</TabsTrigger>
              <TabsTrigger value="archived">Archived</TabsTrigger>
              <TabsTrigger value="all">All</TabsTrigger>
            </TabsList>
          </Tabs>
        )}
        {p.rangeNote && <p className="m-0 text-xs text-faint">{p.rangeNote}</p>}
      </header>
      {p.error ? (
        <p role="alert" className="m-0 p-4 text-destructive">
          {p.error}
        </p>
      ) : p.loading ? (
        <p className="m-0 p-4 text-faint">Loading…</p>
      ) : p.items.length === 0 ? (
        <p className="m-0 p-4 text-faint">No messages.</p>
      ) : (
        <ul className="m-0 min-h-0 flex-1 list-none overflow-y-auto p-0">
          {p.items.map((h) => {
            const unread = p.showUnread && !h.read_at && !h.archived_at
            const selected = h.thread_id === p.selectedThread
            const key = rowKey(h)
            const isCursor = key === p.cursorId
            const n = h.thread_count
            const who = parseAddress(h.from)
            return (
              <li key={key} ref={isCursor ? cursorRef : undefined}>
                <button
                  type="button"
                  data-unread={unread}
                  aria-current={selected ? 'true' : undefined}
                  data-cursor={isCursor || undefined}
                  onClick={() => p.onSelect(h)}
                  className={cn(
                    'flex w-full items-start gap-3 border-b px-4 py-3 text-left hover:bg-hover',
                    selected && 'bg-accent-soft',
                    isCursor && 'ring-2 ring-ring/50 ring-inset',
                  )}
                >
                  <span className="flex w-2 shrink-0 justify-center pt-3.5">
                    {unread && <span aria-label="Unread" role="img" className="size-2 rounded-full bg-brand" />}
                  </span>
                  <AgentAvatar name={who.identity ?? who.path} client={h.from_client} />
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="flex items-baseline gap-2">
                      <AddressChip address={h.from} className={cn('min-w-0 truncate', unread && 'font-bold')} />
                      <span className="flex-1" />
                      <span className="shrink-0 text-xs text-faint">{ago(h.created_at, p.now)}</span>
                    </span>
                    <span className="flex items-center gap-2">
                      <span className={cn('min-w-0 flex-1 truncate', unread ? 'font-bold text-text' : 'text-muted-foreground')}>
                        {h.subject}
                      </span>
                      {n > 1 && (
                        <span aria-label={`${n} messages in thread`} className="rounded-full bg-code px-1.5 text-xs text-muted-foreground">
                          {n}
                        </span>
                      )}
                    </span>
                    {h.snippet && <span className="truncate text-xs text-faint">{h.snippet}</span>}
                    {p.showMailbox && (
                      <span className="flex items-center gap-1 text-xs text-faint">
                        <span>to</span>
                        <AddressChip address={h.mailbox} className="min-w-0 truncate" />
                      </span>
                    )}
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
      {p.hasNext && (
        <div className="border-t p-2">
          <Button variant="ghost" size="sm" className="w-full" disabled={p.loadingMore} onClick={p.onLoadMore}>
            {p.loadingMore ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}
    </section>
  )
}
