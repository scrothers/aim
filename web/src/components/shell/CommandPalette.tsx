import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Archive, CalendarDays, Circle, FolderTree, Inbox, Keyboard, Mail, Moon, PenLine, Send, User } from 'lucide-react'
import { rosterQuery, searchQuery } from '@/api/mail'
import { useTree } from '@/api/queries'
import { composeStore } from '@/lib/compose'
import { helpFlag, paletteFlag } from '@/lib/flag'
import { useTheme } from '@/lib/theme'
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'

const VIEWS = [
  { view: 'operator', label: 'Operator inbox', icon: Inbox },
  { view: 'unread', label: 'Unread everywhere', icon: Circle },
  { view: 'today', label: 'Today', icon: CalendarDays },
  { view: 'sent', label: 'Sent', icon: Send },
  { view: 'archived', label: 'Archived', icon: Archive },
] as const

function useDebounced<T>(v: T, ms: number): T {
  const [d, setD] = useState(v)
  useEffect(() => {
    const id = setTimeout(() => setD(v), ms)
    return () => clearTimeout(id)
  }, [v, ms])
  return d
}

/** ⌘K palette: mailboxes, identities, views, actions and global message search. */
export function CommandPalette() {
  const open = paletteFlag.use()
  const navigate = useNavigate()
  const { toggle, dark } = useTheme()
  const [text, setText] = useState('')
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
      <CommandInput placeholder="Search mail, mailboxes and agents" value={text} onValueChange={setText} />
      <CommandList>
        {!(term.length >= 2 && hits.length > 0) && <CommandEmpty>No results.</CommandEmpty>}
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
        <CommandGroup heading="Mailboxes">
          {mailboxes.map((a) => (
            <CommandItem key={a} value={`mailbox ${a}`} onSelect={go(() => void navigate({ to: '/mail', search: { mailbox: a } }))}>
              <FolderTree aria-hidden="true" />
              <span className="font-mono text-[13px]">{a}</span>
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Identities">
          {(roster?.items ?? []).map((r) => (
            <CommandItem key={r.address} value={`identity ${r.address} ${r.role}`} onSelect={go(() => void navigate({ to: '/roster', search: { edit: r.address } }))}>
              <User aria-hidden="true" />
              <span className="font-mono text-[13px]">{r.address}</span>
              {r.role && <span className="text-xs text-faint">{r.role}</span>}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Views">
          {VIEWS.map(({ view, label, icon: Icon }) => (
            <CommandItem key={view} value={`view ${label}`} onSelect={go(() => void navigate({ to: '/mail', search: { view } }))}>
              <Icon aria-hidden="true" />
              {label}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Actions">
          <CommandItem value="action compose new message" onSelect={go(composeStore.open)}>
            <PenLine aria-hidden="true" />
            Compose
          </CommandItem>
          <CommandItem value="action toggle theme dark light" onSelect={go(toggle)}>
            <Moon aria-hidden="true" />
            {dark ? 'Switch to light theme' : 'Switch to dark theme'}
          </CommandItem>
          <CommandItem value="action keyboard shortcuts help" onSelect={go(() => helpFlag.set(true))}>
            <Keyboard aria-hidden="true" />
            Keyboard shortcuts
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
