import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { Archive, ArchiveRestore, Bell, BellOff, Mail, MailOpen } from 'lucide-react'
import { ApiError } from '@/api/client'
import { mailFilterParams } from '@/api/filters'
import {
  ARCHIVE_DAYS,
  listParamsFor,
  messagesQuery,
  rowKey,
  useMark,
  useMute,
  type Header,
  type MailSearch,
  type MailState,
  type MarkState,
} from '@/api/mail'
import { useIsMobile } from '@/lib/viewport'
import { Button } from '@/components/ui/button'
import { SaveFilterButton } from './SaveFilter'
import { composeStore } from '@/lib/compose'
import { helpFlag, isTyping, paletteFlag } from '@/lib/flag'
import { MessageList } from './MessageList'
import { ThreadView } from './ThreadView'

const VIEW_TITLES: Record<NonNullable<MailSearch['view']>, string> = {
  operator: 'Operator inbox',
  unread: 'Unread everywhere',
  today: 'Today',
  sent: 'Sent',
  archived: 'Archived',
}

const day = (iso?: string) => (iso ? new Date(iso).toLocaleDateString([], { dateStyle: 'medium' }) : '')

export function MailView({ search, onSearch }: { search: MailSearch; onSearch: (patch: Partial<MailSearch>) => void }) {
  const view = search.mailbox ? undefined : (search.view ?? 'operator')
  // Range bounds are computed once per list identity so the query key stays stable between polls.
  const params = useMemo(
    () => listParamsFor({ view: search.view, mailbox: search.mailbox, state: search.state, muted: search.muted }),
    [search.view, search.mailbox, search.state, search.muted],
  )
  const q = useInfiniteQuery(messagesQuery(params))
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items ?? []) ?? [], [q.data])
  const mobile = useIsMobile()
  const mark = useMark()
  const mute = useMute()
  const [cursor, setCursor] = useState<string | undefined>()
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set())
  const [anchor, setAnchor] = useState<string | undefined>()
  // A different list (view, mailbox, state, muted) starts with nothing selected.
  const listKey = JSON.stringify(params)
  const [prevList, setPrevList] = useState(listKey)
  if (prevList !== listKey) {
    setPrevList(listKey)
    setPicked(new Set())
    setAnchor(undefined)
  }
  // Rows that disappeared (archived, polled away) fall out of the selection.
  const selected = useMemo(() => {
    const keys = new Set(items.map(rowKey))
    return new Set([...picked].filter((k) => keys.has(k)))
  }, [picked, items])
  const clearSelection = useCallback(() => {
    setPicked(new Set())
    setAnchor(undefined)
  }, [])

  const toggleRow = useCallback(
    (h: Header, shift: boolean) => {
      const k = rowKey(h)
      const checked = !selected.has(k)
      const idx = items.findIndex((x) => rowKey(x) === k)
      const from = shift && anchor ? items.findIndex((x) => rowKey(x) === anchor) : -1
      const [lo, hi] = from >= 0 ? [Math.min(from, idx), Math.max(from, idx)] : [idx, idx]
      const next = new Set(selected)
      for (const x of items.slice(lo, hi + 1)) {
        if (checked) next.add(rowKey(x))
        else next.delete(rowKey(x))
      }
      setAnchor(k)
      setPicked(next)
    },
    [items, selected, anchor],
  )
  const toggleAll = useCallback((checked: boolean) => setPicked(checked ? new Set(items.map(rowKey)) : new Set()), [items])
  const [replying, setReplying] = useState(false)
  const threadRef = useRef<HTMLElement>(null)

  // Cursor is a delivery key (message id + mailbox): one message can appear in several rows.
  const threadRow = search.thread ? items.find((h) => h.thread_id === search.thread) : undefined
  const cursorId = cursor ?? (threadRow ? rowKey(threadRow) : undefined)
  const state = params.state

  const open = useCallback(
    (h: Header) => {
      setCursor(rowKey(h))
      setReplying(false)
      onSearch({ thread: h.thread_id })
    },
    [onSearch],
  )

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      // Dialogs (help, palette, compose sheet) own the keyboard while open.
      if (paletteFlag.get() || helpFlag.get() || document.querySelector('[role="dialog"]')) return
      if (e.key === 'Escape') {
        setReplying(false)
        composeStore.close()
        clearSelection()
        return
      }
      if (isTyping(e.target) || composeStore.isOpen()) return
      const idx = items.findIndex((h) => rowKey(h) === cursorId)
      const cur = idx >= 0 ? items[idx] : undefined
      switch (e.key) {
        case 'j':
          if (items.length) setCursor(rowKey(items[Math.min(idx + 1, items.length - 1)]!))
          break
        case 'k':
          if (items.length) setCursor(rowKey(items[Math.max(idx - 1, 0)]!))
          break
        case 'Enter':
          // A focused row or link already handles Enter natively.
          if (e.target instanceof HTMLElement && ['BUTTON', 'A'].includes(e.target.tagName)) return
          if (cur) {
            open(cur)
            threadRef.current?.focus()
          }
          break
        case 'x':
          if (cur) toggleRow(cur, false)
          break
        case 'e':
        case 'u':
          if (cur?.mailbox === 'operator') {
            if (e.key === 'e') {
              const nxt = items[idx + 1] ?? items[idx - 1]
              setCursor(nxt ? rowKey(nxt) : undefined)
            }
            const next = e.key === 'u' ? 'unread' : state === 'archived' ? 'unarchived' : 'archived'
            mark.mutate({ ids: [cur.id], state: next })
          }
          break
        case 'r':
          if (cur) {
            e.preventDefault()
            setCursor(rowKey(cur))
            onSearch({ thread: cur.thread_id })
            setReplying(true)
          }
          break
        case 'c':
          e.preventDefault()
          composeStore.open()
          break
        default:
          return
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [items, cursorId, state, open, onSearch, mark, toggleRow, clearSelection])

  const chosen = items.filter((h) => selected.has(rowKey(h)))
  const operatorIds = [...new Set(chosen.filter((h) => h.mailbox === 'operator').map((h) => h.id))]
  const threadIds = [...new Set(chosen.map((h) => h.thread_id))]
  const allMuted = chosen.length > 0 && chosen.every((h) => h.muted)
  const bulk = (s: MarkState) => mark.mutate({ ids: operatorIds, state: s }, { onSuccess: clearSelection })
  const btn = 'max-md:h-11'
  const bulkActions = (
    <div className="flex flex-wrap items-center gap-1.5" role="toolbar" aria-label="Bulk actions">
      <Button variant="outline" size="sm" className={btn} disabled={!operatorIds.length || mark.isPending} onClick={() => bulk(state === 'archived' ? 'unarchived' : 'archived')}>
        {state === 'archived' ? <ArchiveRestore aria-hidden="true" /> : <Archive aria-hidden="true" />}
        {state === 'archived' ? 'Unarchive' : 'Archive'}
      </Button>
      <Button variant="outline" size="sm" className={btn} disabled={!operatorIds.length || mark.isPending} onClick={() => bulk('read')}>
        <MailOpen aria-hidden="true" />
        Mark read
      </Button>
      <Button variant="outline" size="sm" className={btn} disabled={!operatorIds.length || mark.isPending} onClick={() => bulk('unread')}>
        <Mail aria-hidden="true" />
        Mark unread
      </Button>
      <Button
        variant="outline"
        size="sm"
        className={btn}
        disabled={mute.isPending}
        onClick={() => mute.mutate({ threadIds, muted: !allMuted }, { onSuccess: clearSelection })}
      >
        {allMuted ? <Bell aria-hidden="true" /> : <BellOff aria-hidden="true" />}
        {allMuted ? 'Unmute' : 'Mute'}
      </Button>
      <Button variant="ghost" size="sm" className={btn} onClick={clearSelection}>
        Clear
      </Button>
    </div>
  )

  const rangeNote =
    state === 'archived'
      ? `Last ${ARCHIVE_DAYS} days · ${day(params.since)} – ${day(params.until)}`
      : undefined

  return (
    <>
      {!(mobile && search.thread) && (
      <MessageList
        title={VIEW_TITLES[view ?? 'operator']}
        titleAddress={search.mailbox}
        showMailbox={view !== 'operator' && view !== 'archived'}
        items={items}
        state={state as MailState}
        lockedState={view === 'unread' || view === 'archived'}
        showUnread={view !== 'sent'}
        selectedThread={search.thread}
        cursorId={cursorId}
        rangeNote={rangeNote}
        loading={q.isPending}
        error={q.isError ? (q.error instanceof ApiError ? q.error.message : 'Could not load messages.') : undefined}
        hasNext={!!q.hasNextPage}
        loadingMore={q.isFetchingNextPage}
        selected={selected}
        onToggle={toggleRow}
        onToggleAll={toggleAll}
        bulkActions={bulkActions}
        headerActions={<SaveFilterButton params={mailFilterParams(search)} suggestion={search.mailbox ?? VIEW_TITLES[view ?? 'operator']} />}
        mutedToggle={view === 'operator' ? { checked: !!search.muted, onChange: (v) => onSearch({ muted: v || undefined }) } : undefined}
        onSelect={open}
        onState={(s) => onSearch({ state: s })}
        onLoadMore={() => void q.fetchNextPage()}
      />
      )}
      {(!mobile || search.thread) && (
      <ThreadView
        ref={threadRef}
        onBack={mobile ? () => onSearch({ thread: undefined }) : undefined}
        threadId={search.thread}
        archived={state === 'archived'}
        replying={replying}
        onReplying={setReplying}
        onArchived={() => onSearch({ thread: undefined })}
      />
      )}
    </>
  )
}
