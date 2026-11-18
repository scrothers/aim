import { lazy, Suspense, useState, type KeyboardEvent } from 'react'
import { ApiError } from '@/api/client'
import { useCreateTask, type TaskPriority } from '@/api/tasks'
import { isValidAddress } from '@/lib/address'
import { taskStore, useTaskStore, type CreateDraft } from '@/lib/tasks'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { AssigneeInput } from './AssigneeInput'
import { fromDateInput, PRIORITIES, selectCls } from './model'
import { useScopes } from './useScopes'

const MarkdownEditor = lazy(() => import('@/components/mail/MarkdownEditor'))

function CreateForm({ draft }: { draft: CreateDraft }) {
  const create = useCreateTask()
  const scopes = useScopes()
  const [title, setTitle] = useState(draft.title ?? '')
  const [body, setBody] = useState('')
  const [priority, setPriority] = useState<TaskPriority>('normal')
  const [scope, setScope] = useState(draft.scope ?? '')
  const [assignee, setAssignee] = useState('')
  const [due, setDue] = useState('')
  const [error, setError] = useState('')
  const [suggestions, setSuggestions] = useState<string[]>([])

  const submit = () => {
    if (create.isPending) return
    const t = title.trim()
    const s = scope.trim()
    const a = assignee.trim()
    if (!t) return setError('Title is required.')
    if (s && !isValidAddress(s)) return setError(`Invalid scope: ${s}`)
    setError('')
    setSuggestions([])
    const dueAt = fromDateInput(due)
    create.mutate(
      {
        title: t,
        priority,
        ...(body.trim() ? { body } : {}),
        ...(s ? { scope: s } : {}),
        ...(a ? { assignee: a } : {}),
        ...(dueAt ? { due_at: dueAt } : {}),
        ...(draft.messageId ? { message_id: draft.messageId } : {}),
      },
      {
        onSuccess: (task) => {
          taskStore.closeCreate()
          // From the board the new card is the point; from a thread, stay put and let the chip appear.
          if (!draft.messageId) taskStore.select(task.id)
        },
        onError: (e) => {
          setError(e instanceof ApiError ? e.message : 'Could not create the task.')
          if (e instanceof ApiError) setSuggestions(e.suggestions)
        },
      },
    )
  }

  // Capture phase, so the editor never sees the chord and inserts a newline.
  const onKeyDownCapture = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      e.stopPropagation()
      submit()
    }
  }

  return (
    <form
      aria-label="New task"
      onKeyDownCapture={onKeyDownCapture}
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
      className="flex min-h-0 flex-1 flex-col gap-3 px-4 pb-4"
    >
      <label className="flex flex-col gap-1 text-xs font-semibold text-faint">
        Title
        <Input aria-label="Title" autoFocus value={title} onChange={(e) => setTitle(e.target.value)} placeholder="One-line summary" className="text-sm font-normal text-text" />
      </label>
      <div className="flex flex-col gap-1 text-xs font-semibold text-faint">
        Description
        <Suspense fallback={<Textarea aria-label="Description" disabled rows={6} placeholder="Loading editor…" className="font-mono text-[13px]" />}>
          <MarkdownEditor value={body} onChange={setBody} label="Description" placeholder="Details, acceptance criteria, links…" />
        </Suspense>
      </div>
      <div className="grid grid-cols-2 gap-3 max-sm:grid-cols-1">
        <label className="flex flex-col gap-1 text-xs font-semibold text-faint">
          Priority
          <select aria-label="Priority" value={priority} onChange={(e) => setPriority(e.target.value as TaskPriority)} className={`${selectCls} h-9 text-sm font-normal`}>
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-semibold text-faint">
          Due
          <Input type="date" aria-label="Due date" value={due} onChange={(e) => setDue(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1 text-xs font-semibold text-faint">
          Scope
          <Input aria-label="Scope" list="create-scope-options" value={scope} onChange={(e) => setScope(e.target.value)} placeholder="github.com/org/repo" className="font-mono text-[13px] font-normal" />
          <datalist id="create-scope-options">
            {scopes.map((s) => (
              <option key={s} value={s} />
            ))}
          </datalist>
        </label>
        <div className="flex flex-col gap-1 text-xs font-semibold text-faint">
          Assignee
          <AssigneeInput label="Assignee" value={assignee} onChange={setAssignee} />
        </div>
      </div>
      {draft.messageId && <p className="m-0 text-xs text-muted-foreground">The latest message in this thread will be linked to the task.</p>}
      {error && (
        <div role="alert" className="text-[13px] text-destructive">
          <p className="m-0">{error}</p>
          {suggestions.length > 0 && (
            <p className="m-0 mt-1 flex flex-wrap items-center gap-1.5 text-text">
              Did you mean
              {suggestions.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => {
                    setAssignee(s)
                    setError('')
                    setSuggestions([])
                  }}
                  className="rounded-full bg-accent-soft px-2 py-0.5 font-mono text-xs text-accent-text hover:underline"
                >
                  {s}
                </button>
              ))}
              ?
            </p>
          )}
        </div>
      )}
      <div className="flex items-center gap-2">
        <span className="text-xs text-faint">Created as operator</span>
        <span className="flex-1" />
        <Button type="button" variant="ghost" size="sm" onClick={taskStore.closeCreate}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={create.isPending}>
          Create task <kbd className="font-sans text-xs opacity-80">⌘↵</kbd>
        </Button>
      </div>
    </form>
  )
}

/** New-task sheet, opened from the board, the palette or a thread. */
export function CreateTaskSheet() {
  const { create } = useTaskStore()
  return (
    <Sheet open={!!create} onOpenChange={(o) => !o && taskStore.closeCreate()}>
      <SheetContent side="right" className="w-full gap-3 overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>New task</SheetTitle>
          <SheetDescription>Create a work item for an agent or yourself.</SheetDescription>
        </SheetHeader>
        {create && <CreateForm draft={create} />}
      </SheetContent>
    </Sheet>
  )
}
