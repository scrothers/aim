import { useMemo, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { Archive, CalendarDays, Circle, Inbox, ListChecks, Network, PenLine, Send, Settings, ShieldCheck, Users, type LucideIcon } from 'lucide-react'
import { useApprovals } from '@/api/approvals'
import { usePresence, useTree, useViews } from '@/api/queries'
import { useOpenTaskCount } from '@/api/tasks'
import { composeStore } from '@/lib/compose'
import { cn } from '@/lib/utils'
import { DaemonFooter } from './DaemonFooter'
import { MailboxTree } from './MailboxTree'
import { SavedViews } from './SavedViews'

type ViewKey = 'operator' | 'unread' | 'today' | 'sent' | 'archived'

const rowCls =
  'flex h-8 items-center max-md:min-h-11 gap-2.5 rounded-md px-2.5 text-muted-foreground hover:bg-hover data-[status=active]:bg-accent-soft data-[status=active]:font-semibold data-[status=active]:text-accent-text'

const pillCls = 'h-[18px] min-w-5 rounded-[9px] bg-brand text-center text-xs leading-[18px] text-on-accent'

function Count({ count, pill, label }: { count?: number; pill?: boolean; label: string }) {
  if (count === undefined) return null
  if (pill) {
    return count > 0 ? (
      <span aria-label={`${count} ${label}`} className={pillCls}>
        {count}
      </span>
    ) : null
  }
  return <span className="text-xs text-faint">{count}</span>
}

function ViewLink({ view, label, icon: Icon, children }: { view: ViewKey; label: string; icon: LucideIcon; children?: ReactNode }) {
  return (
    <Link to="/mail" search={{ view }} className={rowCls}>
      <Icon aria-hidden="true" className="size-4" />
      <span className="flex-1">{label}</span>
      {children}
    </Link>
  )
}

const footLinkCls =
  'flex h-11 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 rounded-md text-[11px] text-muted-foreground hover:bg-hover data-[status=active]:bg-accent-soft data-[status=active]:font-semibold data-[status=active]:text-accent-text'

/**
 * Left navigation: work views on top (always visible), the mailbox tree in
 * its own scroll region, and the management pages in a compact footer row.
 */
export function LeftNav({ drawer = false, onNavigate }: { drawer?: boolean; onNavigate?: () => void }) {
  const views = useViews().data
  const tree = useTree()
  const presence = usePresence().data
  const pending = useApprovals().data?.items?.length ?? 0
  const openTasks = useOpenTaskCount()
  const { idle, attention } = useMemo(() => {
    const buddies = (presence?.groups ?? []).flatMap((g) => g.buddies ?? []).filter((b) => b.online)
    return {
      idle: new Set(buddies.filter((b) => b.state === 'idle').map((b) => b.address)),
      attention: new Set(buddies.filter((b) => b.attention).map((b) => b.address)),
    }
  }, [presence])
  return (
    <nav
      aria-label="Mailboxes"
      className={cn('box-border flex min-h-0 flex-col gap-4 bg-panel px-3 pt-4 pb-3', drawer ? 'w-full flex-1' : 'w-[248px] shrink-0 border-r')}
    >
      <button
        type="button"
        onClick={() => {
          onNavigate?.()
          composeStore.open()
        }}
        className="flex h-10 shrink-0 items-center justify-center gap-2 rounded-lg bg-brand font-semibold text-on-accent max-md:h-11"
      >
        <PenLine aria-hidden="true" className="size-4" />
        Compose
      </button>
      <div className="flex shrink-0 flex-col gap-3">
        <div className="flex flex-col gap-px" role="group" aria-label="Work">
          <ViewLink view="operator" label="Operator inbox" icon={Inbox}>
            <Count count={views?.operator_unread} pill label="unread" />
          </ViewLink>
          <ViewLink view="unread" label="Unread everywhere" icon={Circle}>
            <Count count={views?.unread_everywhere} label="unread" />
          </ViewLink>
          <ViewLink view="today" label="Today" icon={CalendarDays}>
            <Count count={views?.today} label="today" />
          </ViewLink>
          <Link to="/tasks" className={rowCls}>
            <ListChecks aria-hidden="true" className="size-4" />
            <span className="flex-1">Tasks</span>
            {openTasks > 0 && (
              <span aria-label={`${openTasks} open`} className="h-[18px] min-w-5 rounded-[9px] bg-code px-1 text-center text-xs leading-[18px] text-muted-foreground">
                {openTasks}
              </span>
            )}
          </Link>
          <Link to="/approvals" className={rowCls}>
            <ShieldCheck aria-hidden="true" className="size-4" />
            <span className="flex-1">Approvals</span>
            <Count count={pending} pill label="pending" />
          </Link>
        </div>
        <div className="flex flex-col gap-px" role="group" aria-label="Folders">
          <ViewLink view="sent" label="Sent" icon={Send} />
          <ViewLink view="archived" label="Archived" icon={Archive} />
          <SavedViews />
        </div>
      </div>
      {tree.data ? (
        <MailboxTree orgs={tree.data.orgs ?? []} idle={idle} attention={attention} />
      ) : (
        <p className="m-0 min-h-0 flex-1 px-2.5 text-[13px] text-faint">{tree.isError ? 'Mailboxes unavailable.' : 'Loading…'}</p>
      )}
      <div className="flex shrink-0 flex-col gap-2 border-t pt-2">
        <div className="flex gap-1" role="group" aria-label="Manage">
          <Link to="/roster" className={footLinkCls}>
            <Users aria-hidden="true" className="size-4" />
            <span>Roster</span>
          </Link>
          <Link to="/org-chart" className={footLinkCls}>
            <Network aria-hidden="true" className="size-4" />
            <span>Org chart</span>
          </Link>
          <Link to="/settings" className={footLinkCls}>
            <Settings aria-hidden="true" className="size-4" />
            <span>Settings</span>
          </Link>
        </div>
        <DaemonFooter />
      </div>
    </nav>
  )
}
