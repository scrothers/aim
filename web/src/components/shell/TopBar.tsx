import { useEffect, useState } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Bell, BellOff, Keyboard, Menu, Moon, MoreHorizontal, PanelRightClose, PanelRightOpen, Search, Sun, Users } from 'lucide-react'
import { toast } from 'sonner'
import { buddiesFlag, helpFlag, navFlag, paletteFlag } from '@/lib/flag'
import { cn } from '@/lib/utils'
import { notifySupported, setNotificationsEnabled, useNotificationsEnabled } from '@/lib/notify'
import { useIsWide } from '@/lib/viewport'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/lib/theme'
import { ApprovalsMenuItem, ApprovalsSwitch, useRemoteApprovals } from './ApprovalsSwitch'
import { useBuddyRail } from './BuddyRail'
import { ConsoleMark } from '@/components/shared/RobotAvatar'
import { Logo } from './Logo'

/** Latest dataUpdatedAt across all cached queries (ms), or 0. */
function latestUpdate(qc: ReturnType<typeof useQueryClient>): number {
  let t = 0
  for (const q of qc.getQueryCache().getAll()) t = Math.max(t, q.state.dataUpdatedAt)
  return t
}

/** Fixed width and tabular digits: the ticking label never resizes its neighbours. */
export function UpdatedAgo({ className }: { className?: string }) {
  const qc = useQueryClient()
  useIsFetching() // re-render when any fetch settles
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  const t = latestUpdate(qc)
  const s = t === 0 ? null : Math.max(0, Math.round((now - t) / 1000))
  return (
    <span data-testid="updated-ago" className={cn('flex items-center gap-1.5 text-[13px] whitespace-nowrap text-faint tabular-nums', className)}>
      <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-online" />
      {s === null ? 'Connecting…' : `Updated ${s}s ago`}
    </span>
  )
}

function OperatorChip() {
  return (
    <span className="flex shrink-0 items-center gap-2 rounded-full border py-1 pr-2.5 pl-1">
      <ConsoleMark size={24} className="rounded-full" />
      <span className="font-mono text-[13px]">operator</span>
    </span>
  )
}

/**
 * Overflow menu. Below `xl` it also carries what the top bar has no room
 * for: the refresh age, who is signed in and the remote-approvals toggle.
 */
function MoreMenu({ compact }: { compact: boolean }) {
  const { dark, toggle } = useTheme()
  const notify = useNotificationsEnabled()
  const remote = useRemoteApprovals()
  const onToggle = async (on: boolean) => {
    const result = await setNotificationsEnabled(on)
    if (on && !result) toast.error('Notification permission was not granted')
  }
  const remoteOn = compact && remote.enabled
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="More options"
        aria-describedby={remoteOn ? 'more-remote-on' : undefined}
        className="relative flex size-8 shrink-0 items-center justify-center rounded-lg border text-muted-foreground hover:bg-hover"
      >
        <MoreHorizontal aria-hidden="true" className="size-4" />
        {remoteOn && (
          <>
            <span data-testid="remote-on-dot" aria-hidden="true" className="absolute top-1 right-1 size-2 rounded-full border border-brand-strong bg-brand ring-2 ring-panel" />
            <span id="more-remote-on" className="sr-only">
              Remote approvals on
            </span>
          </>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        {compact && (
          <>
            <DropdownMenuLabel className="flex flex-col gap-1.5 font-normal">
              <span className="flex items-center gap-2">
                <ConsoleMark size={24} className="rounded-full" />
                <span className="font-mono text-[13px]">operator</span>
              </span>
              <UpdatedAgo />
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <ApprovalsMenuItem />
          </>
        )}
        {notifySupported() && (
          <DropdownMenuCheckboxItem checked={notify} onCheckedChange={(v) => void onToggle(v === true)}>
            {notify ? <Bell aria-hidden="true" /> : <BellOff aria-hidden="true" />}
            Desktop notifications
          </DropdownMenuCheckboxItem>
        )}
        <DropdownMenuItem className="md:hidden" onSelect={toggle}>
          {dark ? <Sun aria-hidden="true" /> : <Moon aria-hidden="true" />}
          {dark ? 'Light theme' : 'Dark theme'}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => helpFlag.set(true)}>
          <Keyboard aria-hidden="true" />
          Keyboard shortcuts
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** One row at every width: anything that does not fit moves into the More menu. */
export function TopBar() {
  const { dark, toggle } = useTheme()
  const wide = useIsWide()
  const rail = useBuddyRail()
  const iconBtn = 'flex size-8 shrink-0 items-center justify-center rounded-lg border text-muted-foreground hover:bg-hover'
  return (
    <header className="flex h-14 shrink-0 flex-nowrap items-center gap-2 overflow-hidden border-b bg-panel px-2 md:gap-3 md:px-4">
      <button type="button" aria-label="Open navigation" onClick={() => navFlag.set(true)} className={cn(iconBtn, 'md:hidden')}>
        <Menu aria-hidden="true" className="size-4" />
      </button>
      <Link to="/" aria-label="AIM home" className="flex shrink-0 items-center gap-2.5 max-md:min-h-11 max-md:min-w-11 max-md:justify-center">
        <Logo />
        <span className="text-base font-bold tracking-[0.02em] max-md:hidden">AIM</span>
        <span className="hidden text-[13px] whitespace-nowrap text-faint lg:inline">Agent Instant Messaging</span>
      </Link>
      <button
        type="button"
        aria-label="Search mail, mailboxes and agents"
        onClick={() => paletteFlag.set(true)}
        className="flex h-9 min-w-0 flex-1 items-center gap-2 rounded-lg border bg-panel2 px-3 text-faint max-md:ml-auto max-md:size-8 max-md:flex-none max-md:justify-center max-md:px-0"
      >
        <Search aria-hidden="true" className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate text-left max-md:hidden">Search mail, mailboxes and agents</span>
        <kbd className="rounded border px-1.5 py-px font-mono text-xs text-muted-foreground max-md:hidden">⌘K</kbd>
      </button>
      <div className="flex shrink-0 items-center gap-2 md:gap-3">
        {wide && (
          <>
            <ApprovalsSwitch />
            <UpdatedAgo className="w-[8.5rem]" />
            <OperatorChip />
          </>
        )}
        {wide ? (
          <button
            type="button"
            aria-label={rail.open ? 'Hide buddy list' : 'Show buddy list'}
            aria-pressed={rail.open}
            title={rail.open ? 'Hide buddy list' : 'Show buddy list'}
            onClick={rail.toggle}
            className={iconBtn}
          >
            {rail.open ? <PanelRightClose aria-hidden="true" className="size-4" /> : <PanelRightOpen aria-hidden="true" className="size-4" />}
          </button>
        ) : (
          <button type="button" aria-label="Open buddy list" onClick={() => buddiesFlag.set(true)} className={iconBtn}>
            <Users aria-hidden="true" className="size-4" />
          </button>
        )}
        <button
          type="button"
          onClick={toggle}
          aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
          className={cn(iconBtn, 'max-md:hidden')}
        >
          {dark ? <Sun aria-hidden="true" className="size-4" /> : <Moon aria-hidden="true" className="size-4" />}
        </button>
        <MoreMenu compact={!wide} />
      </div>
    </header>
  )
}
