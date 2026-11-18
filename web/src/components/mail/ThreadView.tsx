import { forwardRef, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Archive, ArchiveRestore, ArrowLeft, Bell, BellOff, ChevronDown, ChevronRight, ListPlus, MailOpen, Reply } from 'lucide-react'
import { threadQuery, useMark, useMute, useReadMessage, type MessageView } from '@/api/mail'
import { OPERATOR } from '@/lib/address'
import { runtimeFor } from '@/lib/identity'
import { taskStore } from '@/lib/tasks'
import { ThreadTaskChips } from '@/components/tasks/ThreadTaskChips'
import { AddressChip } from '@/components/shared/AddressChip'
import { EmptyState } from '@/components/shared/EmptyState'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { RuntimeBadge } from '@/components/shared/RuntimeBadge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Composer } from './Composer'

const toOperator = (m: MessageView) => (m.to ?? []).includes(OPERATOR) || (m.cc ?? []).includes(OPERATOR)

function stamp(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}

function senderLine(m: MessageView): string {
  const rt = runtimeFor(m.from_client)
  return [m.from_client ? rt.label : '', m.from_machine ? `on ${m.from_machine}` : ''].filter(Boolean).join(' ')
}

function MessageHeader({ m }: { m: MessageView }) {
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      {/* A long sender wraps inside the column, so the date drops below it instead of under the chevron. */}
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
        <AddressChip address={m.from} className="max-w-full min-w-0 flex-wrap text-sm break-all" />
        <span className="shrink-0 text-xs text-faint">{stamp(m.created_at)}</span>
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
        <span>to</span>
        {(m.to ?? []).map((a) => (
          <AddressChip key={a} address={a} className="max-w-full min-w-0 flex-wrap break-all" />
        ))}
        {(m.cc ?? []).length > 0 && <span>· cc</span>}
        {(m.cc ?? []).map((a) => (
          <AddressChip key={a} address={a} className="max-w-full min-w-0 flex-wrap break-all" />
        ))}
      </div>
      {senderLine(m) && (
        <div className="flex items-center gap-1.5 text-xs text-faint">
          <RuntimeBadge client={m.from_client} size={14} />
          <span>{senderLine(m)}</span>
        </div>
      )}
    </div>
  )
}

export interface ThreadViewProps {
  threadId?: string
  /** The list is showing archived mail: offer Unarchive instead of Archive. */
  archived?: boolean
  replying: boolean
  onReplying: (v: boolean) => void
  /** Called after the thread's operator deliveries were archived. */
  onArchived: () => void
  /** Single-pane layout (phones, narrow windows): shows a back button that returns to the list. */
  onBack?: () => void
}

export const ThreadView = forwardRef<HTMLElement, ThreadViewProps>(function ThreadView(
  { threadId, archived, replying, onReplying, onArchived, onBack },
  ref,
) {
  const q = useQuery({ ...threadQuery(threadId ?? ''), enabled: !!threadId })
  const mark = useMark()
  const mute = useMute()
  const read = useReadMessage()
  const readRef = useRef(new Set<string>())
  const [toggled, setToggled] = useState<Record<string, boolean>>({})
  const [prevThread, setPrevThread] = useState(threadId)
  if (prevThread !== threadId) {
    setPrevThread(threadId)
    setToggled({})
  }

  const messages = [...(q.data?.messages ?? [])].sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))
  const latest = messages[messages.length - 1]
  const isOpen = (m: MessageView) => toggled[m.id] ?? m.id === latest?.id
  const mine = messages.filter(toOperator)

  // Viewing an operator-addressed message marks it read, once per message.
  const openOperatorIds = messages.filter((m) => toOperator(m) && isOpen(m)).map((m) => m.id)
  const key = openOperatorIds.join(',')
  const { mutate: markRead } = read
  useEffect(() => {
    for (const id of key ? key.split(',') : []) {
      if (readRef.current.has(id)) continue
      readRef.current.add(id)
      // Forget the id on failure so a later open retries.
      markRead(id, { onError: () => readRef.current.delete(id) })
    }
  }, [key, markRead])

  if (!threadId) {
    return (
      <article ref={ref} tabIndex={-1} aria-label="Thread" className="box-border flex min-h-0 min-w-0 flex-1 flex-col justify-center bg-bg p-5 outline-none">
        <EmptyState icon={MailOpen}>Select a message to read it.</EmptyState>
      </article>
    )
  }

  return (
    <article ref={ref} tabIndex={-1} aria-label="Thread" className="box-border flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-y-auto bg-bg p-5 outline-none max-md:p-3">
      {onBack && (
        <Button variant="ghost" size="sm" className="-ml-1 self-start" onClick={onBack}>
          <ArrowLeft aria-hidden="true" />
          Back
        </Button>
      )}
      {q.isError ? (
        <p role="alert" className="m-0 text-[13px] text-destructive">
          Could not load this thread.
        </p>
      ) : !latest ? (
        <p className="m-0 text-[13px] text-faint">Loading…</p>
      ) : (
        <>
          <header className="flex flex-wrap items-center gap-3">
            <h1 className="m-0 min-w-0 basis-full text-lg font-semibold">{latest.subject}</h1>
            <span className="text-xs text-faint">
              {messages.length} {messages.length === 1 ? 'message' : 'messages'}
            </span>
            {latest.muted && (
              <Badge variant="count">
                <BellOff aria-hidden="true" />
                Muted
              </Badge>
            )}
            <Button variant="outline" size="sm" onClick={() => onReplying(true)}>
              <Reply aria-hidden="true" />
              Reply
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => taskStore.openCreate({ title: latest.subject.replace(/^(re|fwd?):\s*/i, ''), messageId: latest.id })}
            >
              <ListPlus aria-hidden="true" />
              Create task from thread
            </Button>
            <ThreadTaskChips threadId={threadId} />
            {mine.length > 0 && (
              <>
                {archived ? (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={mark.isPending}
                    onClick={() => mark.mutate({ ids: mine.map((m) => m.id), state: 'unarchived' }, { onSuccess: onArchived })}
                  >
                    <ArchiveRestore aria-hidden="true" />
                    Unarchive
                  </Button>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={mark.isPending}
                    onClick={() => mark.mutate({ ids: mine.map((m) => m.id), state: 'archived' }, { onSuccess: onArchived })}
                  >
                    <Archive aria-hidden="true" />
                    Archive
                  </Button>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  disabled={mute.isPending}
                  onClick={() => mute.mutate({ threadIds: [threadId], muted: !latest.muted })}
                >
                  {latest.muted ? <Bell aria-hidden="true" /> : <BellOff aria-hidden="true" />}
                  {latest.muted ? 'Unmute' : 'Mute'}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={mark.isPending}
                  onClick={() => {
                    const id = mine[mine.length - 1]!.id
                    // Reopening the message after an explicit "unread" should mark it read again.
                    readRef.current.delete(id)
                    mark.mutate({ ids: [id], state: 'unread' })
                  }}
                >
                  <MailOpen aria-hidden="true" />
                  Mark unread
                </Button>
              </>
            )}
          </header>
          <ol className="m-0 flex list-none flex-col gap-2 p-0">
            {messages.map((m) => {
              const open = isOpen(m)
              return (
                <li key={m.id} data-open={open} className="rounded-xl border bg-panel">
                  <div className="flex items-start gap-3 p-3">
                    <RobotAvatar address={m.from} client={m.from_client} />
                    {open ? (
                      <MessageHeader m={m} />
                    ) : (
                      <span className="flex min-w-0 flex-1 items-baseline gap-2 pt-1">
                        <AddressChip address={m.from} className="min-w-0 shrink overflow-hidden whitespace-nowrap md:shrink-0" />
                        <span className="min-w-0 flex-1 truncate text-[13px] text-faint max-md:hidden">{m.snippet}</span>
                        <span className="shrink-0 text-xs text-faint max-md:ml-auto">{stamp(m.created_at)}</span>
                      </span>
                    )}
                    <button
                      type="button"
                      aria-expanded={open}
                      aria-label={`${open ? 'Collapse' : 'Expand'} message from ${m.from}`}
                      onClick={() => setToggled((t) => ({ ...t, [m.id]: !open }))}
                      className="flex shrink-0 items-center justify-center rounded p-1 text-faint hover:bg-hover"
                    >
                      {open ? <ChevronDown aria-hidden="true" className="size-4" /> : <ChevronRight aria-hidden="true" className="size-4" />}
                    </button>
                  </div>
                  {open && (
                    <div
                      className="prose-aim border-t px-4 py-3"
                      // body_html is sanitized server-side (bluemonday).
                      dangerouslySetInnerHTML={{ __html: m.body_html }}
                    />
                  )}
                </li>
              )
            })}
          </ol>
          {replying && <Composer key={latest.id} mode={{ kind: 'reply', original: latest }} onClose={() => onReplying(false)} />}
        </>
      )}
    </article>
  )
})
