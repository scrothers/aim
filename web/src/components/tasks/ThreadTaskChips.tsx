import { useThreadTasks } from '@/api/tasks'
import { taskStore } from '@/lib/tasks'
import { STATUS_LABEL } from './model'
import { StatusDot } from './TaskBadges'

/** Tasks linked to a thread, as chips that open the task drawer. */
export function ThreadTaskChips({ threadId }: { threadId: string }) {
  const tasks = useThreadTasks(threadId).data ?? []
  if (tasks.length === 0) return null
  return (
    <ul aria-label="Linked tasks" className="m-0 flex basis-full list-none flex-wrap items-center gap-1.5 p-0">
      {tasks.map((t) => (
        <li key={t.task_id}>
          <button
            type="button"
            data-testid="task-chip"
            data-status={t.status}
            aria-label={`Task: ${t.title} (${STATUS_LABEL[t.status]})`}
            onClick={() => taskStore.select(t.task_id)}
            className="inline-flex max-w-72 items-center gap-1.5 rounded-full border bg-panel px-2.5 py-0.5 text-xs hover:bg-hover max-md:min-h-11"
          >
            <StatusDot status={t.status} />
            <span className="truncate font-medium">{t.title}</span>
            <span className="shrink-0 text-faint">{STATUS_LABEL[t.status]}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}
