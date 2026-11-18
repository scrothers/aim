import { lazy, Suspense, useState, type KeyboardEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { CheckCheck, Hand, MessageSquare, Pencil, Play, RotateCcw, Undo2, type LucideIcon } from 'lucide-react'
import { ApiError } from '@/api/client'
import { useTask, useTaskAction, type TaskActionRequest, type TaskDetail, type TaskPriority } from '@/api/tasks'
import { OPERATOR } from '@/lib/address'
import { ago } from '@/lib/identity'
import { taskStore, useTaskStore } from '@/lib/tasks'
import { AddressChip } from '@/components/shared/AddressChip'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { AssigneeInput } from './AssigneeInput'
import { actionsFor, eventText, fromDateInput, isOverdue, PRIORITIES, selectCls, toDateInput, type ActionName } from './model'
import { HealthIndicator, StatusLabel } from './TaskBadges'
import { useScopes } from './useScopes'

// The editor (Lexical + CodeMirror) is a separate chunk, fetched on first edit.
const MarkdownEditor = lazy(() => import('@/components/mail/MarkdownEditor'))

const ACTION: Record<ActionName, { label: string; icon: LucideIcon; primary?: boolean }> = {
  start: { label: 'Start', icon: Play, primary: true },
  claim: { label: 'Claim', icon: Hand },
  finish: { label: 'Finish', icon: CheckCheck, primary: true },
  release: { label: 'Release', icon: Undo2 },
  reopen: { label: 'Reopen', icon: RotateCcw },
}

const stamp = (iso: string) => {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}

function Section({ title, children, action }: { title: string; children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col gap-2 border-t px-4 pt-3 pb-4">
      <div className="flex items-center gap-2">
        <h3 className="m-0 flex-1 text-xs font-semibold tracking-[0.06em] text-faint uppercase">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex min-w-0 flex-col gap-1 text-xs font-semibold text-faint">
      {label}
      {children}
    </label>
  )
}

/** Text input that saves on Enter or blur when it changed, and restores on Escape. */
function CommitInput({ value, onCommit, ...rest }: { value: string; onCommit: (v: string) => void } & Omit<React.ComponentProps<typeof Input>, 'value' | 'onChange'>) {
  const [text, setText] = useState(value)
  const commit = () => {
    const v = text.trim()
    if (v !== value) onCommit(v)
  }
  return (
    <Input
      {...rest}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          commit()
        } else if (e.key === 'Escape' && text !== value) {
          e.stopPropagation()
          setText(value)
        }
      }}
    />
  )
}

function Title({ task, onSave }: { task: TaskDetail; onSave: (title: string) => void }) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(task.title)
  const save = () => {
    setEditing(false)
    const t = text.trim()
    if (t && t !== task.title) onSave(t)
    else setText(task.title)
  }
  if (editing) {
    return (
      <Input
        autoFocus
        aria-label="Title"
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={save}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            save()
          } else if (e.key === 'Escape') {
            e.stopPropagation()
            setText(task.title)
            setEditing(false)
          }
        }}
        className="h-9 text-base font-semibold"
      />
    )
  }
  return (
    <button
      type="button"
      // No aria-label: the dialog is named after this button's text, the task title.
      title="Edit title"
      onClick={() => {
        setText(task.title)
        setEditing(true)
      }}
      className="group -mx-1 flex items-start gap-2 rounded px-1 text-left text-lg leading-6 font-semibold break-words hover:bg-hover max-md:min-h-11"
    >
      <span className="min-w-0 flex-1">{task.title}</span>
      <Pencil aria-hidden="true" className="mt-1 size-3.5 shrink-0 text-faint opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 max-md:opacity-100" />
    </button>
  )
}

function Body({ task, onSave, saving }: { task: TaskDetail; onSave: (md: string, done: () => void) => void; saving: boolean }) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(task.body)
  return (
    <Section
      title="Description"
      action={
        !editing && (
          <Button
            variant="ghost"
            size="xs"
            onClick={() => {
              setDraft(task.body)
              setEditing(true)
            }}
          >
            <Pencil aria-hidden="true" />
            Edit
          </Button>
        )
      }
    >
      {editing ? (
        <div className="flex flex-col gap-2">
          <Suspense fallback={<Textarea aria-label="Description" disabled rows={6} placeholder="Loading editor…" className="font-mono text-[13px]" />}>
            <MarkdownEditor value={draft} onChange={setDraft} label="Description" placeholder="Describe the work…" autoFocus />
          </Suspense>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setEditing(false)}>
              Cancel
            </Button>
            <Button size="sm" disabled={saving} onClick={() => onSave(draft, () => setEditing(false))}>
              Save
            </Button>
          </div>
        </div>
      ) : task.body.trim() ? (
        <div
          className="prose-aim text-[13px]"
          // body_html is sanitized server-side.
          dangerouslySetInnerHTML={{ __html: task.body_html }}
        />
      ) : (
        <p className="m-0 text-[13px] text-faint">No description.</p>
      )}
    </Section>
  )
}

function Details({ task }: { task: TaskDetail }) {
  const act = useTaskAction()
  const scopes = useScopes()
  const [assignee, setAssignee] = useState('')
  const [suggestions, setSuggestions] = useState<string[]>([])
  const [error, setError] = useState('')
  const edit = (patch: Partial<TaskActionRequest>) => act.mutate({ id: task.id, body: { action: 'edit', ...patch } })

  const assign = (who: string) => {
    const a = who.trim()
    if (!a) return
    setError('')
    setSuggestions([])
    act.mutate(
      { id: task.id, body: { action: 'assign', assignee: a } },
      {
        onSuccess: () => setAssignee(''),
        onError: (e) => {
          setError(e instanceof ApiError ? e.message : 'Could not assign the task')
          if (e instanceof ApiError) setSuggestions(e.suggestions)
        },
      },
    )
  }

  return (
    <Section title="Details">
      <div className="grid grid-cols-2 gap-3 max-sm:grid-cols-1">
        <Field label="Priority">
          <select
            aria-label="Priority"
            value={task.priority}
            onChange={(e) => edit({ priority: e.target.value as TaskPriority })}
            className={`${selectCls} h-9 text-sm font-normal text-text`}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Due">
          <span className="flex items-center gap-1.5">
            <Input
              type="date"
              aria-label="Due date"
              value={toDateInput(task.due_at)}
              onChange={(e) => {
                const due = fromDateInput(e.target.value)
                if (due) edit({ due_at: due })
              }}
              className={isOverdue(task) ? 'border-attention text-attention-text' : ''}
            />
            {task.due_at && (
              <Button type="button" variant="ghost" size="sm" aria-label="Clear due date" onClick={() => edit({ clear_due: true })}>
                Clear
              </Button>
            )}
          </span>
        </Field>
        <Field label="Scope">
          <CommitInput
            key={task.scope ?? ''}
            aria-label="Scope"
            list="task-scope-options"
            placeholder="github.com/org/repo"
            value={task.scope ?? ''}
            onCommit={(v) => edit({ scope: v })}
            className="font-mono text-[13px]"
          />
          <datalist id="task-scope-options">
            {scopes.map((s) => (
              <option key={s} value={s} />
            ))}
          </datalist>
        </Field>
        <div className="flex flex-col gap-1 text-xs font-semibold text-faint">
          <span>Claimed by</span>
          <span className="flex h-9 items-center text-[13px] font-normal text-text">
            {task.claimant ? <AddressChip address={task.claimant} /> : <span className="text-faint">Nobody</span>}
          </span>
        </div>
      </div>

      <div className="flex flex-col gap-1.5 text-xs font-semibold text-faint">
        <span>Assigned to</span>
        <div className="flex flex-wrap items-center gap-2 text-[13px] font-normal text-text">
          {task.assignee ? (
            <>
              <AddressChip address={task.assignee} />
              <Button variant="outline" size="xs" disabled={act.isPending} onClick={() => act.mutate({ id: task.id, body: { action: 'unassign' } })}>
                Unassign
              </Button>
            </>
          ) : (
            <span className="text-faint">Nobody. Anyone may claim it.</span>
          )}
        </div>
        <div className="flex items-start gap-2">
          <AssigneeInput
            label="Assign to"
            value={assignee}
            onChange={(v) => {
              setAssignee(v)
              setError('')
              setSuggestions([])
            }}
            onSubmit={() => assign(assignee)}
            className="flex-1"
          />
          <Button variant="outline" disabled={!assignee.trim() || act.isPending} onClick={() => assign(assignee)}>
            {task.assignee ? 'Reassign' : 'Assign'}
          </Button>
        </div>
        {error && (
          <div role="alert" className="text-xs font-normal text-destructive">
            <p className="m-0">{error}</p>
            {suggestions.length > 0 && (
              <p className="m-0 mt-1 flex flex-wrap items-center gap-1.5 text-text">
                Did you mean
                {suggestions.map((s) => (
                  <button key={s} type="button" onClick={() => assign(s)} className="rounded-full bg-accent-soft px-2 py-0.5 font-mono text-xs text-accent-text hover:underline">
                    {s}
                  </button>
                ))}
                ?
              </p>
            )}
          </div>
        )}
      </div>
    </Section>
  )
}

function Comment({ id }: { id: string }) {
  const act = useTaskAction()
  const [text, setText] = useState('')
  const post = () => {
    const note = text.trim()
    if (!note || act.isPending) return
    act.mutate({ id, body: { action: 'comment', note } }, { onSuccess: () => setText('') })
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      post()
    }
  }
  return (
    <Section title="Comment">
      <Textarea aria-label="Comment" rows={2} placeholder="Add a comment…" value={text} onChange={(e) => setText(e.target.value)} onKeyDown={onKeyDown} className="text-[13px]" />
      <div className="flex justify-end">
        <Button size="sm" disabled={!text.trim() || act.isPending} onClick={post}>
          <MessageSquare aria-hidden="true" />
          Comment
        </Button>
      </div>
    </Section>
  )
}

function DrawerContent({ task }: { task: TaskDetail }) {
  const act = useTaskAction()
  const navigate = useNavigate()
  const [now] = useState(() => Date.now())
  const events = [...(task.events ?? [])].reverse()
  const links = task.links ?? []
  const run = (action: ActionName) => act.mutate({ id: task.id, body: { action } })

  return (
    <>
      <SheetHeader className="gap-2 pr-12">
        <SheetTitle asChild>
          <h2 className="m-0">
            <Title key={task.title} task={task} onSave={(title) => act.mutate({ id: task.id, body: { action: 'edit', title } })} />
          </h2>
        </SheetTitle>
        <SheetDescription asChild>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <StatusLabel status={task.status} />
            <HealthIndicator task={task} now={now} showLabel />
            <span className="font-mono text-faint">{task.id}</span>
            <span>
              created by <b className="font-semibold">{task.created_by}</b> {ago(task.created_at, now)} ago
            </span>
          </div>
        </SheetDescription>
      </SheetHeader>

      <div role="group" aria-label="Task actions" className="flex flex-wrap gap-2 px-4 pb-4">
        {actionsFor(task.status).map((a) => {
          const { label, icon: Icon, primary } = ACTION[a]
          return (
            <Button key={a} variant={primary ? 'default' : 'outline'} disabled={act.isPending} onClick={() => run(a)} data-action={a}>
              <Icon aria-hidden="true" />
              {label}
            </Button>
          )
        })}
      </div>

      <Body
        key={task.body}
        task={task}
        saving={act.isPending}
        onSave={(md, done) => act.mutate({ id: task.id, body: { action: 'edit', body: md } }, { onSuccess: done })}
      />
      <Details key={task.id} task={task} />
      <Comment id={task.id} />

      <Section title="Linked threads">
        {links.length === 0 ? (
          <p className="m-0 text-[13px] text-faint">No linked messages.</p>
        ) : (
          <ul className="m-0 flex list-none flex-col gap-1 p-0">
            {links.map((l) => (
              <li key={l.message_id}>
                <button
                  type="button"
                  onClick={() => {
                    taskStore.select(undefined)
                    void navigate({ to: '/mail', search: { mailbox: OPERATOR, state: 'all', thread: l.thread_id } })
                  }}
                  className="flex w-full min-w-0 flex-col rounded-md border px-3 py-2 text-left hover:bg-hover max-md:min-h-11"
                >
                  <span className="truncate text-[13px] font-semibold">{l.subject}</span>
                  <span className="text-xs text-faint">
                    linked by {l.linked_by} {ago(l.linked_at, now)} ago
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title="History">
        <ol aria-label="History" className="m-0 flex list-none flex-col gap-3 p-0">
          {events.map((e, i) => (
            <li key={`${e.at}-${i}`} data-kind={e.kind} className="flex flex-col gap-0.5 border-l-2 pl-3 text-[13px]">
              <span>
                <b className="font-semibold">{e.actor}</b> <span className="text-muted-foreground">{eventText(e)}</span>
              </span>
              {e.note && e.kind !== 'assign' && e.kind !== 'link' && (
                <span className={e.kind === 'comment' ? 'rounded-md bg-code px-2 py-1.5 break-words whitespace-pre-wrap' : 'text-xs break-words text-muted-foreground'}>{e.note}</span>
              )}
              <time dateTime={e.at} title={stamp(e.at)} className="text-xs text-faint">
                {stamp(e.at)}
              </time>
            </li>
          ))}
        </ol>
      </Section>
    </>
  )
}

function Drawer({ id }: { id: string }) {
  const q = useTask(id)
  if (q.data) return <DrawerContent task={q.data} />
  return (
    <SheetHeader>
      <SheetTitle>Task</SheetTitle>
      <SheetDescription role={q.isError ? 'alert' : undefined} className={q.isError ? 'text-destructive' : ''}>
        {q.isError ? (q.error instanceof ApiError && q.error.status === 404 ? 'This task no longer exists.' : 'Could not load this task.') : 'Loading…'}
      </SheetDescription>
    </SheetHeader>
  )
}

/** Task detail drawer, opened from the board, a thread chip or the palette. */
export function TaskDrawer() {
  const { selected } = useTaskStore()
  return (
    <Sheet open={!!selected} onOpenChange={(o) => !o && taskStore.select(undefined)}>
      <SheetContent side="right" className="w-full gap-0 overflow-y-auto sm:max-w-xl">
        {selected && <Drawer key={selected} id={selected} />}
      </SheetContent>
    </Sheet>
  )
}
