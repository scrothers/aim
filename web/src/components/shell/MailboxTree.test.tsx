import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fixtureNow, treeFixture } from '@/test/fixtures'
import { renderWithRouter } from '@/test/utils'
import { EXPANDED_ORGS_KEY, MailboxTree, filterTree, isQuiet } from './MailboxTree'

const ui = () => <MailboxTree orgs={treeFixture} now={fixtureNow} />

beforeEach(() => localStorage.clear())
afterEach(() => vi.restoreAllMocks())

describe('MailboxTree', () => {
  it('starts with every org collapsed, showing org badges', async () => {
    await renderWithRouter(<MailboxTree orgs={treeFixture} now={fixtureNow} attention={new Set(['reviewer@github.com/acme/api'])} />)
    expect(await screen.findByRole('button', { name: 'Expand acme' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('button', { name: 'Expand labs' })).toBeInTheDocument()
    expect(screen.queryByText('coordinator@')).toBeNull()
    expect(screen.queryByText('scout@')).toBeNull()
    const [acme, labs] = screen.getAllByTestId('org-block')
    // Agent mailbox unread is informational (neutral); blocked agents get the attention pill.
    expect(within(acme!).getByLabelText('9 unread')).toHaveAttribute('data-variant', 'count')
    expect(within(acme!).getByLabelText('1 needs attention')).toHaveAttribute('data-variant', 'attention')
    expect(within(labs!).queryByTestId('org-attention')).toBeNull()
    expect(within(labs!).getByText(/quiet · 3d/)).toBeInTheDocument()
    expect(acme).toHaveAttribute('data-quiet', 'false')
    expect(labs).toHaveAttribute('data-quiet', 'true')
  })

  it('auto-expands only the org and repo holding the selected mailbox', async () => {
    await renderWithRouter(ui(), '/mail?mailbox=triage%40github.com%2Facme%2Fweb')
    expect(await screen.findByText('triage@')).toBeInTheDocument() // web holds the selection
    expect(screen.getByText('coordinator@')).toBeInTheDocument()
    expect(screen.getByText('reviewer@')).toBeInTheDocument() // api has unread -> open
    expect(screen.queryByText('scout@')).toBeNull()
  })

  it('collapses a selected org on request without persisting it as expanded', async () => {
    const user = userEvent.setup()
    await renderWithRouter(ui(), '/mail?mailbox=github.com%2Facme')
    await user.click(await screen.findByRole('button', { name: 'Collapse acme' }))
    expect(screen.queryByText('coordinator@')).toBeNull()
    expect(localStorage.getItem(EXPANDED_ORGS_KEY)).toBe('[]')
  })

  it('remembers expanded orgs in localStorage', async () => {
    const user = userEvent.setup()
    const first = await renderWithRouter(ui())
    await user.click(await screen.findByRole('button', { name: 'Expand labs' }))
    expect(screen.getByText('scout@')).toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem(EXPANDED_ORGS_KEY)!)).toEqual(['github.com/labs'])
    first.unmount()
    await renderWithRouter(ui())
    expect(await screen.findByText('scout@')).toBeInTheDocument()
    expect(screen.queryByText('coordinator@')).toBeNull()
  })

  it('renders and toggles when storage is unavailable', async () => {
    const user = userEvent.setup()
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    await renderWithRouter(ui())
    await user.click(await screen.findByRole('button', { name: 'Expand acme' }))
    expect(screen.getByText('coordinator@')).toBeInTheDocument()
  })

  it('classifies quiet orgs', () => {
    expect(isQuiet(treeFixture[0]!, fixtureNow)).toBe(false)
    expect(isQuiet(treeFixture[1]!, fixtureNow)).toBe(true)
  })

  it('toggles repo collapse', async () => {
    const user = userEvent.setup()
    await renderWithRouter(ui())
    await user.click(await screen.findByRole('button', { name: 'Expand acme' }))
    await user.click(await screen.findByRole('button', { name: 'Collapse api' }))
    expect(screen.queryByText('reviewer@')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Expand api' }))
    expect(screen.getByText('reviewer@')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Expand web' }))
    expect(screen.getByText('triage@')).toBeInTheDocument()
  })

  it('toggles org collapse', async () => {
    const user = userEvent.setup()
    await renderWithRouter(ui())
    await user.click(await screen.findByRole('button', { name: 'Expand labs' }))
    expect(screen.getByText('scout@')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Expand acme' }))
    expect(screen.getByText('coordinator@')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Collapse acme' }))
    expect(screen.queryByText('coordinator@')).toBeNull()
  })

  it('filters by text and auto-expands matches', async () => {
    const user = userEvent.setup()
    await renderWithRouter(ui())
    await user.type(await screen.findByLabelText('Filter mailboxes'), 'triage')
    const blocks = screen.getAllByTestId('org-block')
    expect(blocks).toHaveLength(1)
    expect(within(blocks[0]!).getByText('triage@')).toBeInTheDocument()
    expect(screen.queryByText('reviewer@')).toBeNull()
    await user.clear(screen.getByLabelText('Filter mailboxes'))
    await user.type(screen.getByLabelText('Filter mailboxes'), 'nope')
    expect(screen.getByText('No mailboxes match.')).toBeInTheDocument()
  })

  it('filterTree keeps whole org when org name matches', () => {
    const r = filterTree(treeFixture, 'acme')
    expect(r).toHaveLength(1)
    expect(r[0]!.repos).toHaveLength(2)
  })

  it('filterTree does not match every mailbox on host substrings', () => {
    for (const q of ['co', 'git', 'hub', 'com']) {
      const r = filterTree(treeFixture, q)
      const all = r.flatMap((o) => [...o.identities, ...o.repos.flatMap((x) => x.identities)])
      expect(all.every((m) => (m.address.split('@')[0] ?? '').includes(q))).toBe(true)
    }
  })

  it('filterTree matches full addresses when the query looks like one', () => {
    const r = filterTree(treeFixture, 'acme/api')
    expect(r).toHaveLength(1)
    expect(r[0]!.repos.map((x) => x.repo.name)).toEqual(['api'])
    const host = filterTree(treeFixture, 'github.com')
    expect(host.length).toBeGreaterThan(0)
  })

  it('links mailboxes with search params', async () => {
    const user = userEvent.setup()
    await renderWithRouter(ui())
    await user.click(await screen.findByRole('button', { name: 'Expand acme' }))
    const link = await screen.findByTitle('reviewer@github.com/acme/api')
    expect(link).toHaveAttribute('href', expect.stringContaining('mailbox=reviewer%40github.com%2Facme%2Fapi'))
    const avatar = within(link).getByTestId('robot-avatar')
    expect(avatar).toHaveAttribute('data-compact', 'true')
  })
})
