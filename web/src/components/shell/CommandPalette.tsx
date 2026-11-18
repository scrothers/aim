import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  Archive,
  CalendarDays,
  Circle,
  FolderTree,
  Inbox,
  Keyboard,
  ListChecks,
  ListPlus,
  Mail,
  Moon,
  Network,
  PenLine,
  Search,
  Send,
  Settings,
  ShieldCheck,
  Users,
} from 'lucide-react'
import { rosterQuery, searchQuery } from '@/api/mail'
import { useTree } from '@/api/queries'
import { composeStore } from '@/lib/compose'
import { helpFlag, paletteFlag } from '@/lib/flag'
import { taskStore } from '@/lib/tasks'
import { useTheme } from '@/lib/theme'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandShortcut } from '@/components/ui/command'

const VIEWS = [
  { view: 'operator', label: 'Operator inbox', icon: Inbox },
  { view: 'unread', label: 'Unread everywhere', icon: Circle },
  { view: 'today', label: 'Today', icon: CalendarDays },
  { view: 'sent', label: 'Sent', icon: Send },
  { view: 'archived', label: 'Archived', icon: Archive },
] as const

const PAGES = [
  { to: '/tasks', label: 'Tasks', keywords: 'task board work items', shortcut: 't', icon: ListChecks },
  { to: '/approvals', label: 'Approvals', keywords: 'remote permission prompts', shortcut: 'a', icon: ShieldCheck },
  { to: '/roster', label: 'Roster', keywords: 'identities agents roles', shortcut: undefined, icon: Users },
  { to: '/org-chart', label: 'Org chart', keywords: 'reports_to hierarchy', shortcut: undefined, icon: Network },
  { to: '/settings', label: 'Settings', keywords: 'preferences webhooks backup prune', shortcut: undefined, icon: Settings },
] as const

/** Headings stay visible while their group scrolls, so every hit shows its section. */
const groupCls = '[&_[cmdk-group-heading]]:sticky [&_[cmdk-group-heading]]:top-0 [&_[cmdk-group-heading]]:z-10 [&_[cmdk-group-heading]]:bg-popover'

function useDebounced<T>(v: T, ms: number): T {
  const [d, setD] = useState(v)
  useEffect(() => {
    const id = setTimeout(() => setD(v), ms)
    return () => clearTimeout(id)
  }, [v, ms])
  return d
}

/** ⌘K palette: pages, actions, views, mailboxes, identities and global message search. */
export function CommandPalette() {
  const open = paletteFlag.use()
  const navigate = useNavigate()
  const { toggle, dark } = useTheme()
  const [text, setText] = useState('')
  const listRef = useRef<HTMLDivElement>(null)
  const term = useDebounced(text.trim(), 250)
  const tree = useTree().data
  const roster = useQuery({ ...rosterQuery(), enabled: open }).data
  const hits = useQuery({ ...searchQuery(term), enabled: open && term.length >= 2 }).data?.hits ?? []

  const mailboxes = useMemo(
    () =>
      (tree?.orgs ?? []).flatMap((o) => [
        o.address,
        ...(o.identities ?? []).map((i) => i.address),
        ...(o.repos ?? []).flatMap((r) => [r.address, ...(r.identities ?? []).map((i) => i.address)]),
      ]),
    [tree],
  )

  const go = (fn: () => void) => () => {
    paletteFlag.set(false)
    fn()
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={(o) => {
        paletteFlag.set(o)
        if (!o) setText('')
      }}
      title="Command palette"
      description="Search mailboxes, identities, views and messages"
    >
      <CommandInput
        placeholder="Search mail, mailboxes and agents"
        value={text}
        onValueChange={(v) => {
          setText(v)
          // New text, new results: start from the top so the first section's heading shows.
          if (listRef.current) listRef.current.scrollTop = 0
        }}
      />
      <CommandList ref={listRef}>
        {!(term.length >= 2 && hits.length > 0) && <CommandEmpty>No results.</CommandEmpty>}
        {text.trim() !== '' && (
          <CommandGroup heading="Search" forceMount>
            <CommandItem forceMount value="search all mail" onSelect={go(() => void navigate({ to: '/search', search: { q: text.trim() } }))}>
              <Search aria-hidden="true" />
              <span className="truncate">Search all mail for “{text.trim()}”</span>
            </CommandItem>
          </CommandGroup>
        )}
        {term.length >= 2 && hits.length > 0 && (
          // forceMount on the group too: cmdk hides a group whose items all
          // fail its text filter, even when the items are force-mounted.
          <CommandGroup heading="Messages" forceMount>
            {hits.map((h) => (
              <CommandItem
                key={h.message_id}
                forceMount
                value={`message ${h.message_id}`}
                onSelect={go(() => void navigate({ to: '/mail', search: { mailbox: h.mailbox, state: 'all', thread: h.thread_id } }))}
              >
                <Mail aria-hidden="true" />
                <span className="flex min-w-0 flex-col">
                  <span className="truncate">{h.subject}</span>
                  <span className="truncate font-mono text-xs text-faint">
                    {h.from} → {h.mailbox}
                  </span>
                </span>
              </CommandItem>
            ))}
          </CommandGroup>
        )}
        <CommandGroup heading="Pages" className={groupCls}>
          {PAGES.map(({ to, label, keywords, shortcut, icon: Icon }) => (
            <CommandItem key={to} value={`page ${label} ${keywords}`} onSelect={go(() => void navigate({ to }))}>
              <Icon aria-hidden="true" />
              {label}
              {shortcut && <CommandShortcut>{shortcut}</CommandShortcut>}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Actions" className={groupCls}>
          <CommandItem value="action new message compose mail" onSelect={go(composeStore.open)}>
            <PenLine aria-hidden="true" />
            New message
            <CommandShortcut>c</CommandShortcut>
          </CommandItem>
          <CommandItem value="action new task create" onSelect={go(() => taskStore.openCreate())}>
            <ListPlus aria-hidden="true" />
            New task
          </CommandItem>
          <CommandItem value="action toggle theme dark light" onSelect={go(toggle)}>
            <Moon aria-hidden="true" />
            {dark ? 'Switch to light theme' : 'Switch to dark theme'}
          </CommandItem>
          <CommandItem value="action keyboard shortcuts help" onSelect={go(() => helpFlag.set(true))}>
            <Keyboard aria-hidden="true" />
            Keyboard shortcuts
            <CommandShortcut>?</CommandShortcut>
          </CommandItem>
        </CommandGroup>
        <CommandGroup heading="Views" className={groupCls}>
          {VIEWS.map(({ view, label, icon: Icon }) => (
            <CommandItem key={view} value={`view ${label}`} onSelect={go(() => void navigate({ to: '/mail', search: { view } }))}>
              <Icon aria-hidden="true" />
              {label}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Mailboxes" className={groupCls}>
          {mailboxes.map((a) => (
            <CommandItem key={a} value={`mailbox ${a}`} onSelect={go(() => void navigate({ to: '/mail', search: { mailbox: a } }))}>
              <FolderTree aria-hidden="true" />
              <span className="font-mono text-[13px]">{a}</span>
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Identities" className={groupCls}>
          {(roster?.items ?? []).map((r) => (
            <CommandItem key={r.address} value={`identity ${r.address} ${r.role}`} onSelect={go(() => void navigate({ to: '/roster', search: { edit: r.address } }))}>
              <RobotAvatar address={r.address} size={20} />
              <span className="font-mono text-[13px]">{r.address}</span>
              {r.role && <span className="text-xs text-faint">{r.role}</span>}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
