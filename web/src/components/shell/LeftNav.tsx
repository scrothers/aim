import { Link } from '@tanstack/react-router'
import { Archive, CalendarDays, Circle, Inbox, Network, PenLine, Send, Users, type LucideIcon } from 'lucide-react'
import { useTree, useViews } from '@/api/queries'
import { composeStore } from '@/lib/compose'
import { cn } from '@/lib/utils'
import { DaemonFooter } from './DaemonFooter'
import { MailboxTree } from './MailboxTree'

type ViewKey = 'operator' | 'unread' | 'today' | 'sent' | 'archived'

const rowCls =
  'flex h-8 items-center gap-2.5 rounded-md px-2.5 text-muted-foreground hover:bg-hover data-[status=active]:bg-accent-soft data-[status=active]:font-semibold data-[status=active]:text-accent-text'

function ViewLink({
  view,
  label,
  icon: Icon,
  count,
  pill,
}: {
  view: ViewKey
  label: string
  icon: LucideIcon
  count?: number
  pill?: boolean
}) {
  return (
    <Link to="/mail" search={{ view }} className={rowCls}>
      <Icon aria-hidden="true" className="size-4" />
      <span className="flex-1">{label}</span>
      {count !== undefined &&
        (pill && count > 0 ? (
          <span aria-label={`${count} unread`} className="h-[18px] min-w-5 rounded-[9px] bg-brand text-center text-xs leading-[18px] text-on-accent">
            {count}
          </span>
        ) : (
          <span className="text-xs text-faint">{count}</span>
        ))}
    </Link>
  )
}

export function LeftNav() {
  const views = useViews().data
  const tree = useTree()
  return (
    <nav aria-label="Mailboxes" className="box-border flex flex-[1_1_232px] flex-col gap-5 border-r bg-panel px-3 py-4">
      <button
        type="button"
        onClick={composeStore.open}
        className="flex h-10 items-center justify-center gap-2 rounded-lg bg-brand font-semibold text-on-accent"
      >
        <PenLine aria-hidden="true" className="size-4" />
        Compose
      </button>
      <div className="flex flex-col gap-px">
        <ViewLink view="operator" label="Operator inbox" icon={Inbox} count={views?.operator_unread} pill />
        <ViewLink view="unread" label="Unread everywhere" icon={Circle} count={views?.unread_everywhere} />
        <ViewLink view="today" label="Today" icon={CalendarDays} count={views?.today} />
        <ViewLink view="sent" label="Sent" icon={Send} />
        <ViewLink view="archived" label="Archived" icon={Archive} />
      </div>
      {tree.data ? (
        <MailboxTree orgs={tree.data.orgs ?? []} />
      ) : (
        <p className="m-0 px-2.5 text-[13px] text-faint">{tree.isError ? 'Mailboxes unavailable.' : 'Loading…'}</p>
      )}
      <div className="flex flex-col gap-0.5">
        <h3 className="m-0 px-2.5 pb-1.5 text-xs font-semibold tracking-[0.06em] text-faint uppercase">Manage</h3>
        <Link to="/roster" className={cn(rowCls, 'h-[34px]')}>
          <Users aria-hidden="true" className="size-4" />
          <span>Roster</span>
        </Link>
        <Link to="/org-chart" className={cn(rowCls, 'h-[34px]')}>
          <Network aria-hidden="true" className="size-4" />
          <span>Org chart</span>
        </Link>
      </div>
      <DaemonFooter />
    </nav>
  )
}
