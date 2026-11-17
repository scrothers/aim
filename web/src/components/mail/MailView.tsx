import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { ApiError } from '@/api/client'
import { ARCHIVE_DAYS, listParamsFor, messagesQuery, useMark, type Header, type MailSearch, type MailState } from '@/api/mail'
import { composeStore } from '@/lib/compose'
import { MessageList } from './MessageList'
import { ThreadView } from './ThreadView'

const VIEW_TITLES: Record<NonNullable<MailSearch['view']>, string> = {
  operator: 'operator',
  unread: 'Unread everywhere',
  today: 'Today',
  sent: 'Sent',
  archived: 'Archived',
}

function isTyping(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false
  return t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName)
}

const day = (iso?: string) => (iso ? new Date(iso).toLocaleDateString([], { dateStyle: 'medium' }) : '')

export function MailView({ search, onSearch }: { search: MailSearch; onSearch: (patch: Partial<MailSearch>) => void }) {
  const view = search.mailbox ? undefined : (search.view ?? 'operator')
  // Range bounds are computed once per list identity so the query key stays stable between polls.
  const params = useMemo(
    () => listParamsFor({ view: search.view, mailbox: search.mailbox, state: search.state }),
    [search.view, search.mailbox, search.state],
  )
  const q = useInfiniteQuery(messagesQuery(params))
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items ?? []) ?? [], [q.data])
  const mark = useMark()
  const [cursor, setCursor] = useState<string | undefined>()
  const [replying, setReplying] = useState(false)
  const threadRef = useRef<HTMLElement>(null)

  const cursorId = cursor ?? items.find((h) => h.thread_id === search.thread)?.id
  const state = params.state

  const open = useCallback(
    (h: Header) => {
      setCursor(h.id)
      setReplying(false)
      onSearch({ thread: h.thread_id })
    },
    [onSearch],
  )

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (e.key === 'Escape') {
        setReplying(false)
        composeStore.close()
        return
      }
      if (isTyping(e.target) || composeStore.isOpen()) return
      const idx = items.findIndex((h) => h.id === cursorId)
      const cur = idx >= 0 ? items[idx] : undefined
      switch (e.key) {
        case 'j':
          if (items.length) setCursor(items[Math.min(idx + 1, items.length - 1)]!.id)
          break
        case 'k':
          if (items.length) setCursor(items[Math.max(idx - 1, 0)]!.id)
          break
        case 'Enter':
          if (cur) {
            open(cur)
            threadRef.current?.focus()
          }
          break
        case 'e':
        case 'u':
          if (cur?.mailbox === 'operator') {
            if (e.key === 'e') setCursor((items[idx + 1] ?? items[idx - 1])?.id)
            mark.mutate({ ids: [cur.id], state: e.key === 'e' ? 'archived' : 'unread' })
          }
          break
        case 'r':
          if (cur) {
            e.preventDefault()
            setCursor(cur.id)
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
  }, [items, cursorId, open, onSearch, mark])

  const rangeNote =
    state === 'archived'
      ? `Last ${ARCHIVE_DAYS} days · ${day(params.since)} – ${day(params.until)}`
      : undefined

  return (
    <>
      <MessageList
        title={search.mailbox ?? VIEW_TITLES[view ?? 'operator']}
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
        onSelect={open}
        onState={(s) => onSearch({ state: s })}
        onLoadMore={() => void q.fetchNextPage()}
      />
      <ThreadView
        ref={threadRef}
        threadId={search.thread}
        replying={replying}
        onReplying={setReplying}
        onArchived={() => onSearch({ thread: undefined })}
      />
    </>
  )
}
