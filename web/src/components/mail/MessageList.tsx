import { useEffect, useRef, type ReactNode } from 'react'
import { BellOff, Inbox } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ago } from '@/lib/identity'
import { stripMarkdown } from '@/lib/markdown'
import { rowKey, type Header, type MailState } from '@/api/mail'
import { AddressChip } from '@/components/shared/AddressChip'
import { EmptyState } from '@/components/shared/EmptyState'
import { PageHeader } from '@/components/shared/PageHeader'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { Badge, CountBadge } from '@/components/ui/badge'
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
  /** Row keys (see rowKey) ticked for a bulk action. */
  selected: ReadonlySet<string>
  onToggle: (h: Header, shift: boolean) => void
  onToggleAll: (checked: boolean) => void
  /** Bulk action buttons, shown while rows are selected. */
  bulkActions?: ReactNode
  /** Extra header controls, e.g. "Save view". */
  headerActions?: ReactNode
  /** "Show muted" toggle (operator inbox only). */
  mutedToggle?: { checked: boolean; onChange: (v: boolean) => void }
  onSelect: (h: Header) => void
  onState: (s: MailState) => void
  onLoadMore: () => void
  /** Side-by-side layout: the list's fixed width in px. Unset, it fills the pane. */
  width?: number
}

export function MessageList(p: MessageListProps) {
  const cursorRef = useRef<HTMLLIElement | null>(null)
  useEffect(() => {
    cursorRef.current?.scrollIntoView?.({ block: 'nearest' })
  }, [p.cursorId])

  return (
    <section
      aria-label="Message list"
      style={p.width ? { width: p.width } : undefined}
      className={cn('box-border flex min-h-0 min-w-0 flex-col bg-panel text-[13px]', p.width ? 'shrink-0 border-r' : 'flex-1')}
    >
      <div className="flex flex-col gap-2 border-b px-4 pt-3 pb-2">
        <PageHeader
          as="h2"
          size="column"
          title={p.titleAddress ? <AddressChip address={p.titleAddress} /> : p.title}
          actions={
            (p.mutedToggle || p.headerActions) && (
              <>
                {p.mutedToggle && (
                  <label className="flex items-center gap-1.5 text-xs text-muted-foreground max-md:min-h-11">
                    <input type="checkbox" checked={p.mutedToggle.checked} onChange={(e) => p.mutedToggle?.onChange(e.target.checked)} />
                    Show muted
                  </label>
                )}
                {p.headerActions}
              </>
            )
          }
        />
        {!p.lockedState && (
          <Tabs value={p.state} onValueChange={(v) => p.onState(v as MailState)}>
            {/* Equal columns on phones, where 44px minimum widths would otherwise space the tabs unevenly. */}
            <TabsList className="max-md:grid max-md:w-full max-md:grid-cols-4">
              <TabsTrigger value="inbox">Inbox</TabsTrigger>
              <TabsTrigger value="unread">Unread</TabsTrigger>
              <TabsTrigger value="archived">Archived</TabsTrigger>
              <TabsTrigger value="all">All</TabsTrigger>
            </TabsList>
          </Tabs>
        )}
        {p.rangeNote && <p className="m-0 text-xs text-faint">{p.rangeNote}</p>}
        {p.items.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground" data-testid="bulk-bar">
            <label className="flex items-center gap-2 max-md:min-h-11">
              <input
                type="checkbox"
                ref={(el) => {
                  if (el) el.indeterminate = p.selected.size > 0 && p.selected.size < p.items.length
                }}
                checked={p.selected.size > 0 && p.selected.size === p.items.length}
                onChange={(e) => p.onToggleAll(e.target.checked)}
              />
              {p.selected.size > 0 ? `${p.selected.size} selected` : 'Select all in view'}
            </label>
            {p.selected.size > 0 && p.bulkActions}
          </div>
        )}
      </div>
      {p.error ? (
        <p role="alert" className="m-0 p-4 text-destructive">
          {p.error}
        </p>
      ) : p.loading ? (
        <p className="m-0 p-4 text-faint">Loading…</p>
      ) : p.items.length === 0 ? (
        <EmptyState icon={Inbox}>No messages.</EmptyState>
      ) : (
        <ul className="m-0 min-h-0 flex-1 list-none overflow-y-auto p-0 max-md:pb-24">
          {p.items.map((h) => {
            const unread = p.showUnread && !h.read_at && !h.archived_at
            const selected = h.thread_id === p.selectedThread
            const key = rowKey(h)
            const isCursor = key === p.cursorId
            const n = h.thread_count
            return (
              <li key={key} ref={isCursor ? cursorRef : undefined} data-selected={p.selected.has(key) || undefined} className="flex border-b">
                <label className="flex w-10 shrink-0 cursor-pointer items-start justify-center pt-4 hover:bg-hover max-md:w-11">
                  <input
                    type="checkbox"
                    aria-label={`Select ${h.subject}`}
                    checked={p.selected.has(key)}
                    onChange={(e) => p.onToggle(h, (e.nativeEvent as MouseEvent).shiftKey === true)}
                  />
                </label>
                <button
                  type="button"
                  data-unread={unread}
                  aria-current={selected ? 'true' : undefined}
                  data-cursor={isCursor || undefined}
                  onClick={() => p.onSelect(h)}
                  className={cn(
                    'flex min-w-0 flex-1 items-start gap-3 py-3 pr-4 pl-1 text-left hover:bg-hover max-md:min-h-11',
                    (selected || p.selected.has(key)) && 'bg-accent-soft',
                    h.muted && 'opacity-70',
                    isCursor && 'ring-2 ring-ring/50 ring-inset',
                  )}
                >
                  <span className="flex w-2 shrink-0 justify-center pt-3.5">
                    {unread && <span aria-label="Unread" role="img" className="size-2 rounded-full bg-brand" />}
                  </span>
                  <RobotAvatar address={h.from} client={h.from_client} />
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
                      {h.muted && (
                        <Badge variant="count" aria-label="Muted" role="img" title="Muted thread">
                          <BellOff aria-hidden="true" />
                          muted
                        </Badge>
                      )}
                      {n > 1 && <CountBadge count={n} label="messages in thread" />}
                    </span>
                    {h.snippet && <span className="truncate text-xs text-faint">{stripMarkdown(h.snippet, { trim: true })}</span>}
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
