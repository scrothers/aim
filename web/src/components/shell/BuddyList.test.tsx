import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { presenceFixture } from '@/test/fixtures'
import { BuddyList, groupLabel, onlineTotal } from './BuddyList'

const buddyNames = () => screen.queryAllByTestId('buddy').map((r) => r.querySelector('span.font-semibold')?.textContent)

describe('BuddyList', () => {
  it('groups buddies with (online/total) and strips the host', () => {
    render(<BuddyList data={presenceFixture} />)
    expect(screen.getByRole('button', { name: /^acme\s*\(1\/1\)$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^acme\/api\s*\(1\/2\)$/ })).toBeInTheDocument()
    expect(groupLabel('github.com/acme/api')).toBe('acme/api')
    expect(onlineTotal(presenceFixture.groups ?? [])).toBe(2)
    expect(screen.getByText('2 online')).toBeInTheDocument()
  })

  it('renders a robot avatar per buddy row', () => {
    render(<BuddyList data={presenceFixture} />)
    for (const row of screen.getAllByTestId('buddy')) {
      expect(within(row).getByTestId('robot-avatar')).toBeInTheDocument()
    }
  })

  it('shows away bubble, machine · runtime, runtime icon and subagent chips', () => {
    render(<BuddyList data={presenceFixture} />)
    expect(screen.getByText('Watching issues')).toBeInTheDocument()
    expect(screen.getByText('devbox')).toBeInTheDocument()
    expect(screen.getByText('· Claude Code')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Codex' })).toBeInTheDocument()
    expect(screen.getByText('↳ researcher')).toBeInTheDocument()
    expect(screen.getByText('↳ summarizer')).toBeInTheDocument()
  })

  it('keeps the machine · runtime line on one truncating row with the full host in a title', () => {
    const long = 'devbox.example.com'
    const data = structuredClone(presenceFixture)
    data.groups![0]!.buddies![0]!.machine = long
    render(<BuddyList data={data} />)
    const host = screen.getByText(long)
    expect(host).toHaveAttribute('title', long)
    expect(host.className).toContain('truncate')
    const line = host.parentElement!
    expect(line.className).toContain('whitespace-nowrap')
    expect(screen.getByText('· Claude Code').className).toContain('shrink-0')
  })

  it('collapses the offline group by default and expands it on click', async () => {
    const user = userEvent.setup()
    render(<BuddyList data={presenceFixture} />)
    expect(buddyNames()).not.toContain('triage')
    const btn = screen.getByRole('button', { name: /^Offline\s*\(1\)$/ })
    expect(btn).toHaveAttribute('aria-expanded', 'false')
    await user.click(btn)
    const row = screen.getAllByTestId('buddy').find((r) => r.textContent?.includes('triage'))!
    expect(row).toHaveAttribute('data-online', 'false')
    expect(within(row as HTMLElement).queryByText('Watching issues')).toBeNull()
  })

  it('collapses an online group on click', async () => {
    const user = userEvent.setup()
    render(<BuddyList data={presenceFixture} />)
    await user.click(screen.getByRole('button', { name: /^acme\/api/ }))
    expect(buddyNames()).not.toContain('reviewer')
  })

  it('marks idle buddies with an orange ring, idle age and group idle counts', () => {
    const data = structuredClone(presenceFixture)
    const g = data.groups![1]!
    g.idle = 1
    g.buddies![0]!.state = 'idle'
    g.buddies![0]!.last_active = new Date(Date.now() - 25 * 60_000).toISOString()
    render(<BuddyList data={data} />)
    const row = screen.getAllByTestId('buddy').find((r) => r.textContent?.includes('reviewer'))!
    expect(row).toHaveAttribute('data-state', 'idle')
    expect(within(row as HTMLElement).getByRole('img', { name: 'idle' }).className).toContain('border-idle')
    expect(within(row as HTMLElement).getByText('idle 25m')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^acme\/api\s*\(1\/2\s*·\s*1 idle\s*\)$/ })).toBeInTheDocument()
    expect(screen.getByText('2 online · 1 idle')).toBeInTheDocument()
    expect(screen.getByText('coordinator').closest('li')).toHaveAttribute('data-state', 'online')
  })

  it('renders only sign-on/off events in the feed, newest first', () => {
    render(<BuddyList data={presenceFixture} />)
    const feed = screen.getAllByTestId('feed-event')
    expect(feed).toHaveLength(2)
    expect(feed[0]).toHaveTextContent('reviewer signed on')
    expect(feed[1]).toHaveTextContent('triage signed off')
    expect(screen.getByText('Refreshes every 5 seconds')).toBeInTheDocument()
  })

  it('flags blocked buddies with a labelled badge, tooltip detail and group count', () => {
    const data = structuredClone(presenceFixture)
    const g = data.groups![1]!
    g.attention = 1
    Object.assign(g.buddies![0]!, {
      attention: 'blocked',
      attention_detail: 'Claude needs your permission to use Bash',
      attention_since: new Date(Date.now() - 2 * 60_000).toISOString(),
    })
    render(<BuddyList data={data} />)
    const row = screen.getAllByTestId('buddy').find((r) => r.textContent?.includes('Reviewing PR'))!
    expect(row).toHaveAttribute('data-attention', 'blocked')
    const badge = within(row).getByRole('status', { name: /blocked: Claude needs your permission to use Bash/ })
    expect(badge).toHaveAttribute('title', 'Claude needs your permission to use Bash · for 2m')
    expect(badge.className).toContain('bg-attention-soft')
    expect(screen.getByText(/1 needs attention/)).toBeInTheDocument()
    expect(screen.getByText('coordinator').closest('li')).not.toHaveAttribute('data-attention')
  })

  it('shows waiting as a subtle "needs input" label without the alert style', () => {
    const data = structuredClone(presenceFixture)
    data.groups![0]!.buddies![0]!.attention = 'waiting'
    render(<BuddyList data={data} />)
    const badge = screen.getByTestId('attention')
    expect(badge).toHaveTextContent('needs input')
    expect(badge.className).not.toContain('bg-attention-soft')
    expect(screen.queryByText(/needs? attention/)).not.toBeInTheDocument()
  })

  it('lists attention events in the feed with readable labels', () => {
    const data = structuredClone(presenceFixture)
    data.events = [
      { kind: 'attention', attention: 'waiting', identity: 'reviewer@github.com/acme/api', at: new Date().toISOString() },
      { kind: 'attention', attention: 'failed', identity: 'coordinator@github.com/acme', at: new Date().toISOString() },
      ...(data.events ?? []),
    ]
    render(<BuddyList data={data} />)
    const feed = screen.getAllByTestId('feed-event')
    expect(feed[0]).toHaveTextContent('reviewer needs input')
    expect(feed[1]).toHaveTextContent('coordinator failed')
    expect(feed).toHaveLength(4)
  })
})
