import { useMemo, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { ChevronDown, ChevronRight, SearchX } from 'lucide-react'
import type { components } from '@/api/schema'
import { ago, parseAddress } from '@/lib/identity'
import { isStringArray, readPref, writePref } from '@/lib/storage'
import { cn } from '@/lib/utils'
import { EmptyState } from '@/components/shared/EmptyState'
import { PresenceDot } from '@/components/shared/PresenceDot'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { Sparkline } from '@/components/shared/Sparkline'
import { Badge, CountBadge } from '@/components/ui/badge'

type OrgNode = components['schemas']['OrgNode']
type RepoNode = components['schemas']['RepoNode']
type MailboxNode = components['schemas']['MailboxNode']

const DAY_MS = 86_400_000

export function isQuiet(org: OrgNode, now = Date.now()): boolean {
  if (org.unread > 0) return false
  const t = org.last_activity ? Date.parse(org.last_activity) : NaN
  return Number.isNaN(t) || now - t > DAY_MS
}

function matches(text: string, q: string) {
  return text.toLowerCase().includes(q)
}

// Every address shares a host like "github.com", so matching short queries
// against the full address hits everything ("co", "git", "hub"). Match names
// only, unless the query itself looks like an address fragment.
function looksLikeAddress(q: string) {
  return /[@/.]/.test(q)
}

function identityHit(m: MailboxNode, q: string) {
  if (looksLikeAddress(q)) return matches(m.address, q)
  return matches(parseAddress(m.address).identity ?? m.address, q)
}

interface FilteredOrg {
  org: OrgNode
  identities: MailboxNode[]
  repos: Array<{ repo: RepoNode; identities: MailboxNode[] }>
}

export function filterTree(orgs: ReadonlyArray<OrgNode>, query: string): FilteredOrg[] {
  const q = query.trim().toLowerCase()
  const out: FilteredOrg[] = []
  for (const org of orgs) {
    const addr = looksLikeAddress(q)
    const orgHit = q === '' || matches(org.org, q) || (addr && matches(org.address, q))
    const identities = (org.identities ?? []).filter((m) => orgHit || identityHit(m, q))
    const repos: FilteredOrg['repos'] = []
    for (const repo of org.repos ?? []) {
      const repoHit = orgHit || matches(repo.name, q) || (addr && matches(repo.address, q))
      const ids = (repo.identities ?? []).filter((m) => repoHit || identityHit(m, q))
      if (repoHit || ids.length > 0) repos.push({ repo, identities: ids })
    }
    if (orgHit || identities.length > 0 || repos.length > 0) out.push({ org, identities, repos })
  }
  return out
}

function Unread({ n }: { n: number }) {
  return <CountBadge count={n} label="unread" />
}

function IdentityRow({ node, indent, idle }: { node: MailboxNode; indent: string; idle?: ReadonlySet<string> }) {
  const name = parseAddress(node.address).identity ?? node.address
  return (
    <Link
      to="/mail"
      search={{ mailbox: node.address }}
      title={node.address}
      className={cn(
        'flex h-[30px] items-center gap-2 rounded-md pr-1.5 hover:bg-hover data-[status=active]:bg-accent-soft max-md:min-h-11',
        indent,
        node.retired && 'opacity-50',
      )}
    >
      <PresenceDot online={node.online > 0} idle={idle?.has(node.address)} />
      <RobotAvatar address={node.address} size={18} />
      <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground">{name}@</span>
      <span className="text-[11px] text-faint">{ago(node.last_activity)}</span>
      <Unread n={node.unread} />
    </Link>
  )
}

/** Every mailbox address inside an org: the org, its identities, repos and repo identities. */
export function orgAddresses(org: OrgNode): string[] {
  return [
    org.address,
    ...(org.identities ?? []).map((i) => i.address),
    ...(org.repos ?? []).flatMap((r) => [r.address, ...(r.identities ?? []).map((i) => i.address)]),
  ]
}

/** Orgs the operator expanded, remembered per browser. */
export const EXPANDED_ORGS_KEY = 'aim.nav.expandedOrgs'

function useExpandedOrgs() {
  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set(readPref(EXPANDED_ORGS_KEY, [], isStringArray)))
  const set = (address: string, v: boolean) =>
    setOpen((prev) => {
      const next = new Set(prev)
      if (v) next.add(address)
      else next.delete(address)
      writePref(EXPANDED_ORGS_KEY, [...next])
      return next
    })
  return [open, set] as const
}

/** The mailbox the console is showing, if any. */
function useSelectedMailbox(): string | undefined {
  return useRouterState({
    select: (s): string | undefined => {
      if (s.location.pathname !== '/mail') return undefined
      const m = (s.location.search as { mailbox?: unknown }).mailbox
      return typeof m === 'string' ? m : undefined
    },
  })
}

export function MailboxTree({
  orgs,
  now,
  idle,
  attention,
}: {
  orgs: ReadonlyArray<OrgNode>
  now?: number
  idle?: ReadonlySet<string>
  /** Addresses of online agents in an attention state (blocked, waiting, ...). */
  attention?: ReadonlySet<string>
}) {
  const [query, setQuery] = useState('')
  // Orgs start collapsed; an org opens when the operator expands it (remembered)
  // or while it holds the selected mailbox (unless collapsed for that selection).
  const [expanded, setExpanded] = useExpandedOrgs()
  const selected = useSelectedMailbox()
  const [dismissed, setDismissed] = useState<{ sel?: string; orgs: ReadonlySet<string> }>({ orgs: new Set() })
  const dismissedNow = dismissed.sel === selected ? dismissed.orgs : undefined
  const [repoOpen, setRepoOpen] = useState<Record<string, boolean>>({})
  const filtering = query.trim() !== ''
  const filtered = useMemo(() => filterTree(orgs, query), [orgs, query])

  return (
    <section aria-label="Mailbox list" className="flex min-h-0 flex-1 flex-col gap-1.5">
      <div className="flex items-center px-2.5">
        <h3 className="m-0 text-xs font-semibold tracking-[0.06em] text-faint uppercase">Mailboxes</h3>
        <span className="ml-auto text-xs text-faint">7d activity</span>
      </div>
      <label htmlFor="mbfilter" className="sr-only">
        Filter mailboxes
      </label>
      <input
        id="mbfilter"
        type="text"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Filter mailboxes"
        className="mx-1 box-border h-[30px] shrink-0 rounded-md border bg-panel2 px-2.5 text-[13px] text-text outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11"
      />
      <div data-testid="mailbox-scroll" className="-mx-1 flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto px-1 pb-1">
      {filtered.length === 0 && (
        <EmptyState compact icon={SearchX}>
          No mailboxes match.
        </EmptyState>
      )}
      {filtered.map(({ org, identities, repos }) => {
        const quiet = isQuiet(org, now)
        const addresses = orgAddresses(org)
        const holdsSelection = !!selected && addresses.includes(selected)
        const open = filtering || expanded.has(org.address) || (holdsSelection && !dismissedNow?.has(org.address))
        const needAttention = attention ? addresses.filter((a) => attention.has(a)).length : 0
        const toggleOrg = () => {
          setExpanded(org.address, !open)
          if (holdsSelection) {
            const next = new Set(dismissedNow)
            if (open) next.add(org.address)
            else next.delete(org.address)
            setDismissed({ sel: selected, orgs: next })
          }
        }
        const label = org.org
        return (
          <div
            key={org.address}
            data-testid="org-block"
            data-quiet={quiet}
            className={cn('flex flex-col gap-px rounded-[10px] border p-1.5', quiet && !open ? 'bg-transparent' : 'bg-panel2')}
          >
            <div className="flex items-center">
              <Link
                to="/mail"
                search={{ mailbox: org.address }}
                className="flex h-9 min-w-0 flex-1 items-center gap-2 rounded-md px-1.5 max-md:min-h-11 hover:bg-hover data-[status=active]:bg-accent-soft"
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'flex size-6 shrink-0 items-center justify-center rounded-md text-xs font-bold',
                    quiet ? 'bg-code text-muted-foreground' : 'bg-text text-panel',
                  )}
                >
                  {label.slice(0, 1).toUpperCase()}
                </span>
                <span className="flex min-w-0 flex-1 flex-col leading-4">
                  <span className="truncate font-semibold">{label}</span>
                  <span className="truncate font-mono text-[11px] text-faint">{org.host}</span>
                </span>
                {org.unread > 0 ? (
                  <Unread n={org.unread} />
                ) : (
                  quiet && needAttention === 0 && <span className="text-xs text-faint">quiet · {ago(org.last_activity, now) || '–'}</span>
                )}
                {needAttention > 0 && (
                  <Badge
                    variant="attention"
                    aria-label={`${needAttention} ${needAttention === 1 ? 'needs' : 'need'} attention`}
                    data-testid="org-attention"
                  >
                    <span aria-hidden="true" className="size-1.5 rounded-full bg-attention" />
                    {needAttention}
                  </Badge>
                )}
              </Link>
              {!filtering && (
                <button
                  type="button"
                  aria-expanded={open}
                  aria-label={`${open ? 'Collapse' : 'Expand'} ${label}`}
                  onClick={toggleOrg}
                  className="flex size-7 shrink-0 items-center justify-center rounded-md text-faint hover:bg-hover max-md:size-11"
                >
                  {open ? <ChevronDown aria-hidden="true" className="size-3.5" /> : <ChevronRight aria-hidden="true" className="size-3.5" />}
                </button>
              )}
            </div>
            {open && (
              <>
                {identities.map((m) => (
                  <IdentityRow key={m.address} node={m} indent="pl-3.5" idle={idle} />
                ))}
                {repos.map(({ repo, identities: ids }) => {
                  const holds = !!selected && (selected === repo.address || (repo.identities ?? []).some((m) => m.address === selected))
                  const ropen = filtering || (repoOpen[repo.address] ?? (repo.unread > 0 || holds))
                  return (
                    <div key={repo.address} data-testid="repo-block" className="flex flex-col gap-px">
                      <div className="flex h-[30px] items-center gap-2 rounded-md pr-1.5 pl-2.5 hover:bg-hover max-md:min-h-11">
                        <button
                          type="button"
                          aria-expanded={ropen}
                          aria-label={`${ropen ? 'Collapse' : 'Expand'} ${repo.name}`}
                          onClick={() => setRepoOpen((s) => ({ ...s, [repo.address]: !ropen }))}
                          className="flex size-3 shrink-0 items-center justify-center text-[11px] text-faint"
                        >
                          {ropen ? '▾' : '▸'}
                        </button>
                        <Link
                          to="/mail"
                          search={{ mailbox: repo.address }}
                          className="flex min-h-11 min-w-0 flex-1 items-center gap-2 md:min-h-0"
                        >
                          <span className="min-w-0 flex-1 truncate font-mono text-[13px] font-medium">{repo.name}</span>
                          <Sparkline values={repo.activity} />
                          <Unread n={repo.unread} />
                        </Link>
                      </div>
                      {ropen && ids.map((m) => <IdentityRow key={m.address} node={m} indent="pl-[34px]" idle={idle} />)}
                    </div>
                  )
                })}
              </>
            )}
          </div>
        )
      })}
      </div>
    </section>
  )
}
