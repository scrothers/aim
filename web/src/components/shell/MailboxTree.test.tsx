import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { fixtureNow, treeFixture } from '@/test/fixtures'
import { renderWithRouter } from '@/test/utils'
import { MailboxTree, filterTree, isQuiet } from './MailboxTree'

const ui = () => <MailboxTree orgs={treeFixture} now={fixtureNow} />

describe('MailboxTree', () => {
  it('shows active org expanded, repos with unread expanded, quiet org collapsed', async () => {
    await renderWithRouter(ui())
    expect(await screen.findByText('coordinator@')).toBeInTheDocument()
    expect(screen.getByText('reviewer@')).toBeInTheDocument() // api has unread -> open
    expect(screen.queryByText('triage@')).toBeNull() // web has none -> collapsed
    expect(screen.queryByText('scout@')).toBeNull() // labs quiet -> collapsed
    expect(screen.getByText(/quiet · 3d/)).toBeInTheDocument()
    const blocks = screen.getAllByTestId('org-block')
    expect(blocks[0]).toHaveAttribute('data-quiet', 'false')
    expect(blocks[1]).toHaveAttribute('data-quiet', 'true')
  })

  it('classifies quiet orgs', () => {
    expect(isQuiet(treeFixture[0]!, fixtureNow)).toBe(false)
    expect(isQuiet(treeFixture[1]!, fixtureNow)).toBe(true)
  })

  it('toggles repo collapse', async () => {
    const user = userEvent.setup()
    await renderWithRouter(ui())
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

  it('links mailboxes with search params', async () => {
    await renderWithRouter(ui())
    const link = await screen.findByTitle('reviewer@github.com/acme/api')
    expect(link).toHaveAttribute('href', expect.stringContaining('mailbox=reviewer%40github.com%2Facme%2Fapi'))
  })
})
