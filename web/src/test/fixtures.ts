import type { components } from '@/api/schema'

type OrgNode = components['schemas']['OrgNode']
type PresenceBody = components['schemas']['PresenceBody']

const NOW = Date.parse('2026-11-17T12:00:00Z')
export const fixtureNow = NOW
const iso = (msAgo: number) => new Date(NOW - msAgo).toISOString()

export const treeFixture: OrgNode[] = [
  {
    address: 'github.com/acme',
    host: 'github.com',
    org: 'acme',
    unread: 9,
    last_activity: iso(120_000),
    activity: [1, 2, 3, 4, 5, 6, 7],
    identities: [
      { address: 'coordinator@github.com/acme', role: 'lead', online: 1, retired: false, subagents: [], unread: 1, activity: [], last_activity: iso(120_000) },
    ],
    repos: [
      {
        address: 'github.com/acme/api',
        name: 'api',
        unread: 5,
        activity: [2, 4, 3, 8, 6, 9, 12],
        last_activity: iso(540_000),
        identities: [
          { address: 'reviewer@github.com/acme/api', role: 'review', online: 1, retired: false, subagents: [], unread: 3, activity: [], last_activity: iso(540_000) },
          { address: 'builder@github.com/acme/api', role: 'build', online: 0, retired: false, subagents: [], unread: 0, activity: [], last_activity: iso(840_000) },
        ],
      },
      {
        address: 'github.com/acme/web',
        name: 'web',
        unread: 0,
        activity: [0, 0, 0, 0, 0, 0, 0],
        last_activity: iso(3 * 86_400_000),
        identities: [
          { address: 'triage@github.com/acme/web', role: 'triage', online: 0, retired: false, subagents: [], unread: 0, activity: [], last_activity: iso(3 * 86_400_000) },
        ],
      },
    ],
  },
  {
    address: 'github.com/labs',
    host: 'github.com',
    org: 'labs',
    unread: 0,
    last_activity: iso(3 * 86_400_000),
    activity: [0, 0, 0, 0, 0, 0, 0],
    identities: [
      { address: 'scout@github.com/labs', role: 'scout', online: 0, retired: false, subagents: [], unread: 0, activity: [], last_activity: iso(3 * 86_400_000) },
    ],
    repos: [],
  },
]

export const presenceFixture: PresenceBody = {
  groups: [
    {
      name: 'github.com/acme',
      offline: false,
      online: 1,
      total: 1,
      buddies: [
        {
          address: 'coordinator@github.com/acme',
          name: 'coordinator',
          role: 'lead',
          online: true,
          away: 'Watching issues',
          client: 'claude-code',
          machine: 'devbox',
          subagents: ['researcher', 'summarizer'],
          signed_on_at: iso(3600_000),
        },
      ],
    },
    {
      name: 'github.com/acme/api',
      offline: false,
      online: 1,
      total: 2,
      buddies: [
        {
          address: 'reviewer@github.com/acme/api',
          name: 'reviewer',
          role: 'review',
          online: true,
          away: 'Reviewing PR #212',
          client: 'codex-cli',
          machine: 'pi-03',
          subagents: [],
          signed_on_at: iso(1800_000),
        },
      ],
    },
    {
      name: 'Offline',
      offline: true,
      online: 0,
      total: 1,
      buddies: [
        { address: 'triage@github.com/acme/web', name: 'triage', role: 'triage', online: false, subagents: [] },
      ],
    },
  ],
  events: [
    { kind: 'signed_on', identity: 'reviewer@github.com/acme/api', machine: 'pi-03', client: 'codex-cli', at: iso(1800_000) },
    { kind: 'connected', identity: '', machine: 'pi-03', client: 'codex-cli', at: iso(1810_000) },
    { kind: 'signed_off', identity: 'triage@github.com/acme/web', machine: 'devbox', client: 'grok', at: iso(7200_000) },
  ],
}
