import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { rosterQuery } from '@/api/mail'
import { treeQuery } from '@/api/queries'
import { OPERATOR } from '@/lib/address'

export type CandidateKind = 'operator' | 'identity' | 'subagent' | 'org' | 'repo'

export interface Candidate {
  address: string
  kind: CandidateKind
  role?: string
  online: boolean
}

export const KIND_LABEL: Record<CandidateKind, string> = {
  operator: 'operator',
  identity: 'identity',
  subagent: 'subagent',
  org: 'org',
  repo: 'repo',
}

const KIND_RANK: Record<CandidateKind, number> = { operator: 0, identity: 0, subagent: 1, org: 2, repo: 2 }
export const MAX_SUGGESTIONS = 8

/** Match quality: 0 exact, 1 prefix of the address or its name before `@`, 2 substring, -1 none. */
function tier(c: Candidate, q: string): number {
  const a = c.address.toLowerCase()
  if (a === q) return 0
  if (a.startsWith(q) || a.split('@')[0]!.startsWith(q)) return 1
  return a.includes(q) ? 2 : -1
}

/**
 * Filters and orders candidates for `query`: exact/prefix before substring,
 * then online before offline, then identities before subagents before scopes.
 */
export function rankCandidates(candidates: Candidate[], query: string, exclude: readonly string[] = []): Candidate[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const skip = new Set(exclude)
  return candidates
    .filter((c) => !skip.has(c.address))
    .map((c) => ({ c, t: tier(c, q) }))
    .filter((x) => x.t >= 0)
    .sort(
      (x, y) =>
        x.t - y.t ||
        Number(y.c.online) - Number(x.c.online) ||
        KIND_RANK[x.c.kind] - KIND_RANK[y.c.kind] ||
        x.c.address.localeCompare(y.c.address),
    )
    .slice(0, MAX_SUGGESTIONS)
    .map((x) => x.c)
}

/** Everything the operator can address: live identities, their subagents, orgs, repos and `operator`. */
export function useRecipientCandidates(): Candidate[] {
  const roster = useQuery(rosterQuery(true)).data
  const tree = useQuery(treeQuery()).data
  return useMemo(() => {
    const seen = new Set<string>()
    const out: Candidate[] = []
    const add = (c: Candidate) => {
      if (seen.has(c.address)) return
      seen.add(c.address)
      out.push(c)
    }
    add({ address: OPERATOR, kind: 'operator', role: 'you', online: true })
    for (const r of roster?.items ?? []) {
      if (r.retired_at) continue
      const online = (r.online ?? []).length > 0
      add({ address: r.address, kind: 'identity', role: r.role, online })
      // Subagent addresses are `sub@identity`: the subagent leads the identity address.
      for (const s of r.subagents ?? []) add({ address: s.includes('/') ? s : `${s}@${r.address}`, kind: 'subagent', online })
    }
    for (const o of tree?.orgs ?? []) {
      add({ address: o.address, kind: 'org', online: false })
      for (const r of o.repos ?? []) add({ address: r.address, kind: 'repo', online: false })
    }
    return out
  }, [roster, tree])
}
