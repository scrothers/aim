import { useHealth, useStats } from '@/api/queries'
import { formatBytes } from '@/lib/identity'
import { cn } from '@/lib/utils'

export function DaemonFooter() {
  const health = useHealth()
  const stats = useStats()
  const healthy = health.data?.status === 'ok'
  const s = stats.data
  return (
    <div className="mt-auto flex flex-col gap-1 border-t px-2.5 pt-3 text-xs text-faint">
      <span className="flex items-center gap-1.5 text-muted-foreground">
        <span aria-hidden="true" className={cn('size-1.5 rounded-full', healthy ? 'bg-online' : 'bg-destructive')} />
        {healthy ? 'Daemon healthy' : health.isPending ? 'Checking daemon…' : 'Daemon unreachable'}
      </span>
      {s && (
        <>
          <span>
            {s.sessions} {s.sessions === 1 ? 'session' : 'sessions'} connected · {s.machines} {s.machines === 1 ? 'machine' : 'machines'}
          </span>
          <span>
            {s.messages.toLocaleString('en-US')} messages · {formatBytes(s.db_size_bytes)}
          </span>
        </>
      )}
    </div>
  )
}
