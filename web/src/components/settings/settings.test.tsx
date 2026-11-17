import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Settings } from '@/api/settings'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { SettingsPage } from './SettingsPage'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })

const base = (): Settings => ({
  retention: { prune_archived_after_days: 30 },
  backups: { every_hours: 24, keep: 7, dir: '', effective_dir: '/state/aim/backups' },
  backup_status: { last_success_at: '2026-11-17T10:00:00Z', last_size: 2_097_152 },
  notify: {
    public_url: '',
    default_public_url: 'http://devbox:7172',
    webhooks: [
      { id: 'w1', name: 'ops', kind: 'discord', url: 'https://discord.com/api/webhooks/123/****', enabled: true },
    ],
  },
})

let settings: Settings
function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <SettingsPage />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  settings = base()
  mocks.GET.mockImplementation(async () => ok(settings))
  mocks.PUT.mockImplementation(async () => ok({}))
  mocks.POST.mockImplementation(async () => ok({}))
})

describe('SettingsPage', () => {
  it('shows masked webhook URLs and never a full secret', async () => {
    mount()
    expect(await screen.findByText('https://discord.com/api/webhooks/123/****')).toBeInTheDocument()
    expect(screen.getByText('Never sent')).toBeInTheDocument()
    expect(screen.getByText(/2\.0 MB/)).toBeInTheDocument()
  })

  it('saves retention days', async () => {
    const user = userEvent.setup()
    mount()
    const input = await screen.findByLabelText(/Prune archived mail after/)
    await user.clear(input)
    await user.type(input, '14')
    await user.click(within(screen.getByRole('region', { name: 'Retention' })).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/retention', { body: { prune_archived_after_days: 14 } }))
  })

  it('previews a prune with a dry run before deleting', async () => {
    const user = userEvent.setup()
    mocks.POST.mockImplementation(async (_p: string, o: { params: { query: { dry_run: boolean } } }) =>
      ok({ enabled: true, dry_run: o.params.query.dry_run, messages: 3, cutoff: '2026-10-01T00:00:00Z' }),
    )
    mount()
    await user.click(await screen.findByRole('button', { name: 'Prune now' }))
    const dlg = await screen.findByRole('dialog', { name: 'Prune archived mail?' })
    expect(mocks.POST).toHaveBeenCalledWith('/api/settings/prune', { params: { query: { dry_run: true } } })
    expect(dlg).toHaveTextContent('3 messages')
    await user.click(within(dlg).getByRole('button', { name: 'Delete 3' }))
    await waitFor(() => expect(mocks.POST).toHaveBeenCalledWith('/api/settings/prune', { params: { query: { dry_run: false } } }))
  })

  it('adds a webhook with a write-only URL and keeps the stored URL when editing with an empty field', async () => {
    const user = userEvent.setup()
    mount()
    await user.click(await screen.findByRole('button', { name: 'Add webhook' }))
    const dlg = await screen.findByRole('dialog', { name: 'Add webhook' })
    await user.type(within(dlg).getByLabelText('Name'), 'alerts')
    await user.type(within(dlg).getByLabelText(/Webhook URL/), 'https://hooks.slack.com/services/T/B/x')
    await user.click(within(dlg).getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/notify', {
        body: {
          public_url: '',
          webhooks: [
            { id: 'w1', name: 'ops', enabled: true },
            { name: 'alerts', enabled: true, url: 'https://hooks.slack.com/services/T/B/x' },
          ],
        },
      }),
    )

    mocks.PUT.mockClear()
    await user.click(await screen.findByRole('button', { name: 'Edit ops' }))
    const edit = await screen.findByRole('dialog', { name: 'Edit webhook' })
    const url = within(edit).getByLabelText(/Webhook URL/)
    expect(url).toHaveValue('')
    expect(url).toHaveAttribute('placeholder', 'https://discord.com/api/webhooks/123/****')
    await user.clear(within(edit).getByLabelText('Name'))
    await user.type(within(edit).getByLabelText('Name'), 'ops2')
    await user.click(within(edit).getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/notify', {
        body: { public_url: '', webhooks: [{ id: 'w1', name: 'ops2', enabled: true }] },
      }),
    )
  })

  it('toggles enabled and deletes after confirmation', async () => {
    const user = userEvent.setup()
    mount()
    await user.click(await screen.findByRole('checkbox', { name: 'Enable ops' }))
    await waitFor(() =>
      expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/notify', { body: { public_url: '', webhooks: [{ id: 'w1', name: 'ops', enabled: false }] } }),
    )
    mocks.PUT.mockClear()
    await user.click(screen.getByRole('button', { name: 'Delete ops' }))
    expect(mocks.PUT).not.toHaveBeenCalled()
    await user.click(within(await screen.findByRole('dialog', { name: 'Delete webhook?' })).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/notify', { body: { public_url: '', webhooks: [] } }))
  })

  it('shows the result of Send test', async () => {
    const user = userEvent.setup()
    mocks.POST.mockImplementation(async () => ok({ status: 0, error: 'dial tcp: no route' }))
    mount()
    await user.click(await screen.findByRole('button', { name: 'Send test' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Test failed: dial tcp: no route')
    expect(mocks.POST).toHaveBeenCalledWith('/api/settings/webhooks/test', { params: { query: { id: 'w1' } } })
  })

  it('backs up now', async () => {
    const user = userEvent.setup()
    mocks.POST.mockImplementation(async () => ok({}))
    mount()
    await user.click(await screen.findByRole('button', { name: 'Back up now' }))
    await waitFor(() => expect(mocks.POST).toHaveBeenCalledWith('/api/settings/backup'))
  })
})
