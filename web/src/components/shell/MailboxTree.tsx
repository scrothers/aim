import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { components } from '@/api/schema'
import { ago, parseAddress } from '@/lib/identity'
import { cn } from '@/lib/utils'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { Sparkline } from '@/components/shared/Sparkline'

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
  return (
    <span className={cn('min-w-[18px] text-right text-xs', n > 0 ? 'font-semibold text-accent-text' : 'text-faint')}>
      {n}
    </span>
  )
}

function IdentityRow({ node, indent }: { node: MailboxNode; indent: string }) {
  const name = parseAddress(node.address).identity ?? node.address
  return (
    <Link
      to="/mail"
      search={{ mailbox: node.address }}
      title={node.address}
      className={cn(
        'flex h-[30px] items-center gap-2 rounded-md pr-1.5 hover:bg-hover data-[status=active]:bg-accent-soft',
        indent,
        node.retired && 'opacity-50',
      )}
    >
      <span
        role="img"
        aria-label={node.online > 0 ? 'online' : 'offline'}
        className={cn('size-2 shrink-0 rounded-full', node.online > 0 ? 'bg-online' : 'bg-border')}
      />
      <RobotAvatar address={node.address} size={18} />
      <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground">{name}@</span>
      <span className="text-[11px] text-faint">{ago(node.last_activity)}</span>
      <Unread n={node.unread} />
    </Link>
  )
}

export function MailboxTree({ orgs, now }: { orgs: ReadonlyArray<OrgNode>; now?: number }) {
  const [query, setQuery] = useState('')
  // Repo/org expansion: explicit user choices override the defaults.
  const [repoOpen, setRepoOpen] = useState<Record<string, boolean>>({})
  const [orgOpen, setOrgOpen] = useState<Record<string, boolean>>({})
  const filtering = query.trim() !== ''
  const filtered = useMemo(() => filterTree(orgs, query), [orgs, query])

  return (
    <div className="flex flex-col gap-1.5">
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
        className="mx-1 box-border h-[30px] rounded-md border bg-panel2 px-2.5 text-[13px] text-text outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      {filtered.length === 0 && <p className="m-0 px-2.5 text-[13px] text-faint">No mailboxes match.</p>}
      {filtered.map(({ org, identities, repos }) => {
        const quiet = isQuiet(org, now)
        const open = filtering || (orgOpen[org.address] ?? !quiet)
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
                className="flex h-9 min-w-0 flex-1 items-center gap-2 rounded-md px-1.5 hover:bg-hover data-[status=active]:bg-accent-soft"
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
                  <span
                    aria-label={`${org.unread} unread`}
                    className="h-[18px] min-w-5 rounded-[9px] bg-accent-soft text-center text-xs leading-[18px] font-semibold text-accent-text"
                  >
                    {org.unread}
                  </span>
                ) : (
                  quiet && <span className="text-xs text-faint">quiet · {ago(org.last_activity, now) || '–'}</span>
                )}
              </Link>
              {!filtering && (
                <button
                  type="button"
                  aria-expanded={open}
                  aria-label={`${open ? 'Collapse' : 'Expand'} ${label}`}
                  onClick={() => setOrgOpen((s) => ({ ...s, [org.address]: !open }))}
                  className="flex size-7 items-center justify-center rounded-md text-faint hover:bg-hover"
                >
                  {open ? <ChevronDown aria-hidden="true" className="size-3.5" /> : <ChevronRight aria-hidden="true" className="size-3.5" />}
                </button>
              )}
            </div>
            {open && (
              <>
                {identities.map((m) => (
                  <IdentityRow key={m.address} node={m} indent="pl-3.5" />
                ))}
                {repos.map(({ repo, identities: ids }) => {
                  const ropen = filtering || (repoOpen[repo.address] ?? repo.unread > 0)
                  return (
                    <div key={repo.address} data-testid="repo-block" className="flex flex-col gap-px">
                      <div className="flex h-[30px] items-center gap-2 rounded-md pr-1.5 pl-2.5 hover:bg-hover">
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
                          className="flex min-w-0 flex-1 items-center gap-2"
                        >
                          <span className="min-w-0 flex-1 truncate font-mono text-[13px] font-medium">{repo.name}</span>
                          <Sparkline values={repo.activity} />
                          <Unread n={repo.unread} />
                        </Link>
                      </div>
                      {ropen && ids.map((m) => <IdentityRow key={m.address} node={m} indent="pl-[34px]" />)}
                    </div>
                  )
                })}
              </>
            )}
          </div>
        )
      })}
    </div>
  )
}
