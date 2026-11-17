import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { ApiError } from '@/api/client'
import { ARCHIVE_DAYS, listParamsFor, messagesQuery, rowKey, useMark, type Header, type MailSearch, type MailState } from '@/api/mail'
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
    () => listParamsFor({ view: search.view, mailbox: search.mailbox, state: search.state }),
    [search.view, search.mailbox, search.state],
  )
  const q = useInfiniteQuery(messagesQuery(params))
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items ?? []) ?? [], [q.data])
  const mark = useMark()
  const [cursor, setCursor] = useState<string | undefined>()
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
  }, [items, cursorId, state, open, onSearch, mark])

  const rangeNote =
    state === 'archived'
      ? `Last ${ARCHIVE_DAYS} days · ${day(params.since)} – ${day(params.until)}`
      : undefined

  return (
    <>
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
        onSelect={open}
        onState={(s) => onSearch({ state: s })}
        onLoadMore={() => void q.fetchNextPage()}
      />
      <ThreadView
        ref={threadRef}
        threadId={search.thread}
        archived={state === 'archived'}
        replying={replying}
        onReplying={setReplying}
        onArchived={() => onSearch({ thread: undefined })}
      />
    </>
  )
}
