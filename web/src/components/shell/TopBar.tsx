import { useEffect, useState } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Bell, BellOff, Keyboard, Moon, MoreHorizontal, Search, Sun } from 'lucide-react'
import { toast } from 'sonner'
import { helpFlag, paletteFlag } from '@/lib/flag'
import { notifySupported, setNotificationsEnabled, useNotificationsEnabled } from '@/lib/notify'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useTheme } from '@/lib/theme'
import { Logo } from './Logo'

/** Latest dataUpdatedAt across all cached queries (ms), or 0. */
function latestUpdate(qc: ReturnType<typeof useQueryClient>): number {
  let t = 0
  for (const q of qc.getQueryCache().getAll()) t = Math.max(t, q.state.dataUpdatedAt)
  return t
}

export function UpdatedAgo() {
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
    <span className="flex items-center gap-1.5 text-[13px] text-faint">
      <span aria-hidden="true" className="size-1.5 rounded-full bg-online" />
      {s === null ? 'Connecting…' : `Updated ${s}s ago`}
    </span>
  )
}

function MoreMenu() {
  const notify = useNotificationsEnabled()
  const onToggle = async (on: boolean) => {
    const result = await setNotificationsEnabled(on)
    if (on && !result) toast.error('Notification permission was not granted')
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="More options"
        className="flex size-8 items-center justify-center rounded-lg border text-muted-foreground hover:bg-hover"
      >
        <MoreHorizontal aria-hidden="true" className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {notifySupported() && (
          <DropdownMenuCheckboxItem checked={notify} onCheckedChange={(v) => void onToggle(v === true)}>
            {notify ? <Bell aria-hidden="true" /> : <BellOff aria-hidden="true" />}
            Desktop notifications
          </DropdownMenuCheckboxItem>
        )}
        <DropdownMenuItem onSelect={() => helpFlag.set(true)}>
          <Keyboard aria-hidden="true" />
          Keyboard shortcuts
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function TopBar() {
  const { dark, toggle } = useTheme()
  return (
    <header className="flex h-14 flex-wrap items-center gap-4 border-b bg-panel px-4">
      <Link to="/" className="flex items-center gap-2.5">
        <Logo />
        <span className="text-base font-bold tracking-[0.02em]">AIM</span>
        <span className="hidden text-[13px] text-faint sm:inline">Agent Instant Messaging</span>
      </Link>
      <button
        type="button"
        aria-label="Search mail, mailboxes and agents"
        onClick={() => paletteFlag.set(true)}
        className="flex h-9 flex-[1_1_280px] items-center gap-2 rounded-lg border bg-panel2 px-3 text-faint"
      >
        <Search aria-hidden="true" className="size-4" />
        <span className="flex-1 text-left">Search mail, mailboxes and agents</span>
        <kbd className="rounded border px-1.5 py-px font-mono text-xs text-muted-foreground">⌘K</kbd>
      </button>
      <div className="ml-auto flex items-center gap-4">
        <UpdatedAgo />
        <span className="flex items-center gap-2 rounded-full border py-1 pr-2.5 pl-1">
          <span aria-hidden="true" className="flex size-6 items-center justify-center rounded-full bg-text text-[11px] font-bold text-panel">
            OP
          </span>
          <span className="font-mono text-[13px]">operator</span>
        </span>
        <button
          type="button"
          onClick={toggle}
          aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
          className="flex size-8 items-center justify-center rounded-lg border text-muted-foreground hover:bg-hover"
        >
          {dark ? <Sun aria-hidden="true" className="size-4" /> : <Moon aria-hidden="true" className="size-4" />}
        </button>
        <MoreMenu />
      </div>
    </header>
  )
}
