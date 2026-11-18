import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import { ChevronsLeftRight, ListFilter, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { useTaskAction, useTasks, type TaskPriority, type TaskStatus, type TaskView } from '@/api/tasks'
import { OPERATOR } from '@/lib/address'
import { helpFlag, isTyping, paletteFlag } from '@/lib/flag'
import { taskStore } from '@/lib/tasks'
import { useIsMobile } from '@/lib/viewport'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/shared/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAssigneeCandidates } from './AssigneeInput'
import { ConfirmDialog } from './ConfirmDialog'
import { groupByStatus, PRIORITIES, planMove, resultStatus, selectCls, STATUS_LABEL, STATUSES, type MovePlan } from './model'
import { TaskCard } from './TaskCard'
import { StatusDot } from './TaskBadges'
import { useScopes } from './useScopes'

const EMPTY_HINT: Record<TaskStatus, string> = {
  not_started: 'Nothing waiting',
  claimed: 'Nothing claimed',
  started: 'Nothing in progress',
  abandoned: 'No lapsed claims',
  finished: 'Nothing finished',
}

function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 30_000)
    return () => clearInterval(id)
  }, [])
  return now
}

interface ColumnProps {
  status: TaskStatus
  tasks: TaskView[]
  now: number
  dragging?: string
  collapsed: boolean
  onToggle?: () => void
  onOpen: (id: string) => void
  onDragStart: (id: string) => void
  onDragEnd: () => void
  onDropTask: (status: TaskStatus) => void
}

function Column({ status, tasks, now, dragging, collapsed, onToggle, onOpen, onDragStart, onDragEnd, onDropTask }: ColumnProps) {
  const [over, setOver] = useState(false)
  const label = STATUS_LABEL[status]
  const dnd = {
    onDragOver: (e: DragEvent) => {
      if (!dragging) return
      e.preventDefault()
      // Even Abandoned accepts the drop, so refusing it can say why.
      e.dataTransfer.dropEffect = 'move'
      setOver(true)
    },
    onDragLeave: (e: DragEvent) => {
      if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOver(false)
    },
    onDrop: (e: DragEvent) => {
      e.preventDefault()
      setOver(false)
      onDropTask(status)
    },
  }
  const frame = cn('rounded-xl border bg-panel2 transition-colors', over && 'border-brand bg-accent-soft')

  if (collapsed) {
    return (
      <section aria-label={label} data-status={status} data-collapsed="true" className={cn(frame, 'flex w-12 shrink-0 flex-col items-center gap-2 py-3')} {...dnd}>
        {onToggle ? (
          <button
            type="button"
            onClick={onToggle}
            aria-label={`Expand ${label.toLowerCase()} column`}
            aria-expanded={false}
            className="flex flex-col items-center gap-2 rounded text-faint hover:text-text max-md:min-h-11"
          >
            <ChevronsLeftRight aria-hidden="true" className="size-4" />
            <span className="text-xs font-semibold [writing-mode:vertical-rl]">{label}</span>
          </button>
        ) : (
          // Empty columns with nothing to expand (abandoned): a slim label that still takes drops.
          <span className="flex flex-col items-center gap-2 text-faint">
            <StatusDot status={status} />
            <span className="text-xs font-semibold [writing-mode:vertical-rl]">{label}</span>
          </span>
        )}
      </section>
    )
  }

  return (
    <section aria-label={label} data-status={status} className={cn(frame, 'flex min-w-48 flex-1 basis-48 flex-col gap-2 p-2.5')} {...dnd}>
      <header className="flex items-center gap-2 px-1 pt-0.5">
        <StatusDot status={status} />
        <h2 className="m-0 flex-1 text-[13px] font-semibold">{label}</h2>
        <Badge variant="count" aria-label={`${tasks.length} ${tasks.length === 1 ? 'task' : 'tasks'}`}>
          {tasks.length}
        </Badge>
        {onToggle && (
          <button type="button" onClick={onToggle} aria-label={`Collapse ${label.toLowerCase()} column`} aria-expanded className="rounded p-0.5 text-faint hover:bg-hover max-md:min-h-11 max-md:min-w-11">
            <ChevronsLeftRight aria-hidden="true" className="mx-auto size-4" />
          </button>
        )}
      </header>
      {tasks.length === 0 ? (
        <p className="m-0 rounded-lg border border-dashed px-3 py-6 text-center text-xs text-faint">{EMPTY_HINT[status]}</p>
      ) : (
        <ul className="m-0 flex list-none flex-col gap-2 p-0">
          {tasks.map((t) => (
            <TaskCard key={t.id} task={t} now={now} dragging={dragging === t.id} onOpen={onOpen} onDragStart={onDragStart} onDragEnd={onDragEnd} />
          ))}
        </ul>
      )}
    </section>
  )
}

/** Tracks whether the board has more to scroll to on either side, so edge fades can say so. */
function useScrollEdges() {
  const ref = useRef<HTMLDivElement>(null)
  const [edges, setEdges] = useState({ left: false, right: false })
  const measure = useCallback(() => {
    const el = ref.current
    if (!el) return
    const left = el.scrollLeft > 1
    const right = el.scrollLeft + el.clientWidth < el.scrollWidth - 1
    setEdges((e) => (e.left === left && e.right === right ? e : { left, right }))
  }, [])
  // Re-measure after every render: columns come and go with the data.
  useEffect(() => {
    measure()
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  })
  return { ref, edges, measure }
}

export function TasksPage() {
  const { ref: boardRef, edges, measure } = useScrollEdges()
  const mobile = useIsMobile()
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [scope, setScope] = useState('')
  const [assignee, setAssignee] = useState('')
  const [priority, setPriority] = useState<TaskPriority | ''>('')
  const [mine, setMine] = useState(false)
  const [showFinished, setShowFinished] = useState(false)
  const [finishedOpen, setFinishedOpen] = useState(false)
  const [dragging, setDragging] = useState<string>()
  const [pending, setPending] = useState<{ task: TaskView; plan: MovePlan; to: TaskStatus }>()
  const now = useNow()
  const scopes = useScopes()
  const people = useAssigneeCandidates()
  const act = useTaskAction()

  const q = useTasks({
    showFinished,
    ...(scope ? { scope } : {}),
    ...(assignee ? { assignee } : {}),
    ...(mine ? { involves: OPERATOR } : {}),
  })
  const items = useMemo(() => (q.data?.items ?? []).filter((t) => !priority || t.priority === priority), [q.data, priority])
  const grouped = useMemo(() => groupByStatus(items), [items])
  const filtered = !!(scope || assignee || priority || mine)

  const run = async (t: TaskView, plan: MovePlan) => {
    const optimistic = resultStatus(plan.actions)
    try {
      for (const [i, action] of plan.actions.entries()) {
        await act.mutateAsync({ id: t.id, body: { action }, ...(i === 0 ? { optimistic } : {}) })
      }
    } catch {
      /* the mutation already toasted the server's reason and rolled the card back */
    }
  }

  const drop = (to: TaskStatus) => {
    const t = items.find((x) => x.id === dragging)
    setDragging(undefined)
    if (!t) return
    const plan = planMove(t.status, to)
    if (!plan) return
    if ('reject' in plan) {
      toast.error(plan.reject)
      return
    }
    if (plan.confirm) setPending({ task: t, plan, to })
    else void run(t, plan)
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'n' || e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target)) return
      if (paletteFlag.get() || helpFlag.get() || document.querySelector('[role="dialog"]')) return
      e.preventDefault()
      taskStore.openCreate()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const finishedCollapsed =!(showFinished && finishedOpen)
  const toggleFinished = () => {
    if (showFinished) setFinishedOpen(!finishedOpen)
    else {
      setShowFinished(true)
      setFinishedOpen(true)
    }
  }

  return (
    <section aria-label="Tasks" className="box-border flex min-w-0 flex-1 flex-col gap-4 p-6 max-md:p-4">
      <PageHeader
        title="Tasks"
        actions={
          <>
            {mobile && (
              <Button variant="outline" aria-expanded={filtersOpen} aria-controls="task-filters" onClick={() => setFiltersOpen((o) => !o)}>
                <ListFilter aria-hidden="true" />
                Filters{filtered ? ' (active)' : ''}
              </Button>
            )}
            <Button onClick={() => taskStore.openCreate()}>
              <Plus aria-hidden="true" />
              New task
            </Button>
          </>
        }
      />

      <div
        id="task-filters"
        role="group"
        aria-label="Task filters"
        hidden={mobile && !filtersOpen}
        className="flex flex-wrap items-center gap-x-3 gap-y-2 text-[13px] max-md:flex-col max-md:items-stretch"
      >
        <label className="flex items-center gap-1.5 text-muted-foreground">
          Scope
          <select aria-label="Scope" value={scope} onChange={(e) => setScope(e.target.value)} className={cn(selectCls, 'max-w-56')}>
            <option value="">All scopes</option>
            {scope && !scopes.includes(scope) && <option value={scope}>{scope}</option>}
            {scopes.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-1.5 text-muted-foreground">
          Assignee
          <select aria-label="Assignee" value={assignee} onChange={(e) => setAssignee(e.target.value)} className={cn(selectCls, 'max-w-56')}>
            <option value="">Anyone</option>
            {people.map((p) => (
              <option key={p.address} value={p.address}>
                {p.address}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-1.5 text-muted-foreground">
          Priority
          <select aria-label="Priority" value={priority} onChange={(e) => setPriority(e.target.value as TaskPriority | '')} className={selectCls}>
            <option value="">Any</option>
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-1.5 text-muted-foreground max-md:min-h-11">
          <input type="checkbox" checked={mine} onChange={(e) => setMine(e.target.checked)} />
          Involves operator
        </label>
        <label className="flex items-center gap-1.5 text-muted-foreground max-md:min-h-11">
          <input
            type="checkbox"
            checked={showFinished}
            onChange={(e) => {
              setShowFinished(e.target.checked)
              setFinishedOpen(e.target.checked)
            }}
          />
          Show finished
        </label>
        {filtered && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setScope('')
              setAssignee('')
              setPriority('')
              setMine(false)
            }}
          >
            Clear filters
          </Button>
        )}
      </div>

      {q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          Could not load tasks.
        </p>
      ) : !q.data ? (
        <p className="m-0 text-faint">Loading…</p>
      ) : (
        <>
          {q.data.truncated && <p className="m-0 text-xs text-faint">Showing the first {q.data.items.length} tasks. Narrow the filters to see the rest.</p>}
          {items.length === 0 && (
            <p className="m-0 text-muted-foreground">
              {filtered ? 'No tasks match these filters.' : 'No open tasks yet. Create one, or ask an agent to.'}
            </p>
          )}
          {/* relative: absolutely positioned sr-only text in cards must be clipped by this scroller, not widen the page. */}
          <div className="relative flex min-h-0 min-w-0 flex-col">
            {edges.left && (
              <div aria-hidden="true" data-testid="board-fade-left" className="pointer-events-none absolute inset-y-0 left-0 z-10 w-8 bg-gradient-to-r from-background to-transparent" />
            )}
            {edges.right && (
              <div aria-hidden="true" data-testid="board-fade-right" className="pointer-events-none absolute inset-y-0 right-0 z-10 w-8 bg-gradient-to-l from-background to-transparent" />
            )}
          <div ref={boardRef} onScroll={measure} className="relative flex min-h-64 items-stretch gap-3 overflow-x-auto pb-2" data-testid="task-board">
            {STATUSES.map((s) => (
              <Column
                key={s}
                status={s}
                tasks={grouped[s]}
                now={now}
                {...(dragging ? { dragging } : {})}
                collapsed={s === 'finished' ? finishedCollapsed : s === 'abandoned' && grouped.abandoned.length === 0}
                {...(s === 'finished' ? { onToggle: toggleFinished } : {})}
                onOpen={taskStore.select}
                onDragStart={setDragging}
                onDragEnd={() => setDragging(undefined)}
                onDropTask={drop}
              />
            ))}
          </div>
          </div>
        </>
      )}

      <ConfirmDialog
        open={!!pending}
        title={pending ? `Move to ${STATUS_LABEL[pending.to]}` : ''}
        description={pending ? `${pending.task.title}: ${pending.plan.confirm ?? ''}` : ''}
        confirmLabel="Move task"
        onCancel={() => setPending(undefined)}
        onConfirm={() => {
          const p = pending
          setPending(undefined)
          if (p) void run(p.task, p.plan)
        }}
      />
    </section>
  )
}
