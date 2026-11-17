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

  it('renders only sign-on/off events in the feed, newest first', () => {
    render(<BuddyList data={presenceFixture} />)
    const feed = screen.getAllByTestId('feed-event')
    expect(feed).toHaveLength(2)
    expect(feed[0]).toHaveTextContent('reviewer signed on')
    expect(feed[1]).toHaveTextContent('triage signed off')
    expect(screen.getByText('Refreshes every 5 seconds')).toBeInTheDocument()
  })
})
