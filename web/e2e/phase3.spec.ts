import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { Agent } from './agent'
import { startDaemon } from './daemon'
import { expect, test, type Page } from '@playwright/test'

const ORG = 'github.com/acme'
const PLANNER = `planner@${ORG}`
const ARTIFACTS = join(import.meta.dirname, 'artifacts')

// Shares the daemon with console.spec.ts but needs none of its state. Serial:
// later tests use earlier state.
test.describe.configure({ mode: 'serial' })
test.use({ baseURL: process.env.AIM_E2E_BASE_URL })

let planner: Agent

test.beforeAll(async () => {
  planner = await Agent.start(process.env.AIM_E2E_BIN!, process.env.AIM_E2E_SOCKET!, 'pw-session-planner', 'claude-code')
  await planner.must('register', { address: PLANNER, role: 'planner', description: 'plans', reports_to: 'operator' })
  await planner.must('sign_on', { address: PLANNER, away: 'planning' })
  await planner.must('send', { to: ['operator'], subject: 'Quokka rollout plan', body: 'The quokka launch is on track.' })
})

test.afterAll(async () => {
  await planner?.stop()
})

const list = (page: Page) => page.getByRole('region', { name: 'Message list' })
const thread = (page: Page) => page.getByRole('article', { name: 'Thread' })
const row = (page: Page, subject: string) => list(page).getByRole('button', { name: new RegExp(subject) })
const send = (subject: string) => planner.must('send', { to: ['operator'], subject, body: `Body of ${subject}.` })

/** Interactive elements under 44px (height; width too for buttons) on a phone. */
async function smallTargets(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const out: string[] = []
    const sel = 'button, a[href], [role=tab], [role=menuitem], select, input:not([type=checkbox]):not([type=radio]):not([type=hidden]), label:has(> input[type=checkbox])'
    for (const el of document.querySelectorAll(sel)) {
      if (el.closest('.prose-aim') || el.closest('[aria-hidden=true]')) continue
      const r = el.getBoundingClientRect()
      const cs = getComputedStyle(el)
      if (r.width === 0 || r.height === 0 || cs.visibility === 'hidden') continue
      const tooShort = r.height < 43.5
      const tooNarrow = el.tagName === 'BUTTON' && r.width < 43.5
      if (tooShort || tooNarrow) {
        const name = el.getAttribute('aria-label') || (el.textContent ?? '').trim().slice(0, 30)
        out.push(`${el.tagName.toLowerCase()} "${name}" ${Math.round(r.width)}x${Math.round(r.height)}`)
      }
    }
    return out
  })
}

const noHScroll = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)

test('idle buddies get a hollow ring and an idle group count', async ({ page }) => {
  const d = await startDaemon(['--idle-after', '2s'])
  const agent = await Agent.start(process.env.AIM_E2E_BIN!, d.socket, 'pw-idle-session', 'codex')
  try {
    const addr = `sleepy@${ORG}`
    await agent.must('register', { address: addr, role: 'napper', description: 'naps', reports_to: 'operator' })
    await agent.must('sign_on', { address: addr, away: 'napping' })
    await page.goto(`${d.base}/mail`)
    const buddy = page.getByTestId('buddy').filter({ hasText: 'sleepy' })
    await expect(buddy).toHaveAttribute('data-state', 'idle', { timeout: 20_000 })
    await expect(buddy.getByRole('img', { name: 'idle' })).toBeVisible()
    await expect(buddy).toContainText(/idle \d+s|idle <5s|idle now/)
    await expect(page.getByRole('button', { name: /acme.*1 idle/ })).toBeVisible()
    await expect(page.getByText('1 online · 1 idle')).toBeVisible()
    // The mailbox tree dot follows.
    await page.getByRole('navigation', { name: 'Mailboxes' }).getByRole('button', { name: 'Expand acme' }).click().catch(() => {})
    await expect(page.getByRole('navigation', { name: 'Mailboxes' }).getByRole('img', { name: 'idle' })).toBeVisible()
  } finally {
    await agent.stop()
    await d.stop()
  }
})

test('bulk archive with shift-select', async ({ page }) => {
  for (const s of ['Bulk alpha', 'Bulk bravo', 'Bulk charlie', 'Bulk delta']) await send(s)
  await page.goto('/mail')
  await expect(row(page, 'Bulk delta')).toBeVisible()
  // Newest first: delta, charlie, bravo, alpha. Select delta..bravo with shift.
  await list(page).getByRole('checkbox', { name: 'Select Bulk delta' }).check()
  await list(page).getByRole('checkbox', { name: 'Select Bulk bravo' }).click({ modifiers: ['Shift'] })
  await expect(list(page).getByRole('checkbox', { name: 'Select Bulk charlie' })).toBeChecked()
  await expect(list(page).getByRole('checkbox', { name: 'Select Bulk alpha' })).not.toBeChecked()
  await expect(list(page).getByText('3 selected')).toBeVisible()
  await list(page).getByRole('button', { name: 'Archive' }).click()
  await expect(row(page, 'Bulk delta')).toHaveCount(0)
  await expect(row(page, 'Bulk charlie')).toHaveCount(0)
  await expect(row(page, 'Bulk bravo')).toHaveCount(0)
  await expect(row(page, 'Bulk alpha')).toBeVisible()
  // The archived mail is in the archive view.
  await page.getByRole('link', { name: /^Archived/ }).click()
  await expect(row(page, 'Bulk bravo')).toBeVisible()
  await expect(row(page, 'Bulk delta')).toBeVisible()
})

test('x toggles selection and bulk mark unread works', async ({ page }) => {
  await page.goto('/mail')
  await expect(row(page, 'Bulk alpha')).toBeVisible()
  await page.keyboard.press('j')
  await page.keyboard.press('x')
  await expect(list(page).getByText('1 selected')).toBeVisible()
  await list(page).getByRole('button', { name: 'Mark read' }).click()
  await expect(row(page, 'Bulk alpha').getByRole('img', { name: 'Unread' })).toHaveCount(0)
  await list(page).getByLabel('Select all in view').check()
  await list(page).getByRole('button', { name: 'Mark unread' }).click()
  await expect(row(page, 'Bulk alpha').getByRole('img', { name: 'Unread' })).toBeVisible()
})

test('muting a thread hides it from the operator inbox and its unread count unless Show muted', async ({ page }) => {
  await page.goto('/mail')
  const pill = page.getByRole('link', { name: /Operator inbox/ }).locator('[aria-label$=" unread"]')
  const unread = async () => ((await pill.count()) === 0 ? 0 : parseInt((await pill.first().getAttribute('aria-label')) ?? '0', 10))
  await expect(pill).toBeVisible() // earlier tests left unread mail; wait for the views query
  const before = await unread()

  await send('Noisy build chatter')
  await expect.poll(unread).toBe(before + 1)
  // Mute without opening it (opening would mark it read): bulk bar.
  await list(page).getByRole('checkbox', { name: 'Select Noisy build chatter' }).check()
  await list(page).getByRole('button', { name: 'Mute', exact: true }).click()
  await expect(row(page, 'Noisy build chatter')).toHaveCount(0)
  await expect.poll(unread).toBe(before)

  await list(page).getByLabel('Show muted').check()
  const r = row(page, 'Noisy build chatter')
  await expect(r).toBeVisible()
  await expect(r.getByRole('img', { name: 'Muted' })).toBeVisible()

  await r.click()
  await expect(thread(page).getByRole('button', { name: 'Unmute' })).toBeVisible()
  await thread(page).getByRole('button', { name: 'Unmute' }).click()
  await list(page).getByLabel('Show muted').uncheck()
  await expect(row(page, 'Noisy build chatter')).toBeVisible()
  await expect(row(page, 'Noisy build chatter').getByRole('img', { name: 'Muted' })).toHaveCount(0)
})

test('saved filter: create, apply, rename and delete', async ({ page }) => {
  await page.goto(`/mail?mailbox=${encodeURIComponent(PLANNER)}&state=all`)
  await list(page).getByRole('button', { name: 'Save view' }).click()
  const dlg = page.getByRole('dialog', { name: 'Save view' })
  await dlg.getByLabel('Name').fill('Planner everything')
  await dlg.getByRole('button', { name: 'Save' }).click()
  const nav = page.getByRole('navigation', { name: 'Mailboxes' })
  const link = nav.getByRole('link', { name: 'Planner everything' })
  await expect(link).toBeVisible()

  await page.getByRole('link', { name: /^Sent/ }).click()
  await expect(page).toHaveURL(/view=sent/)
  await link.click()
  await expect(page).toHaveURL(/mailbox=planner/)
  await expect(page).toHaveURL(/state=all/)

  // Shared server-side: a fresh page sees it.
  const other = await page.context().newPage()
  await other.goto('/mail')
  await expect(other.getByRole('link', { name: 'Planner everything' })).toBeVisible()
  await other.close()

  await nav.getByRole('button', { name: 'Actions for Planner everything' }).click()
  await page.getByRole('menuitem', { name: 'Rename' }).click()
  const rename = page.getByRole('dialog', { name: 'Rename view' })
  await rename.getByLabel('Name').fill('Planner all')
  await rename.getByRole('button', { name: 'Save' }).click()
  await expect(nav.getByRole('link', { name: 'Planner all' })).toBeVisible()

  await nav.getByRole('button', { name: 'Actions for Planner all' }).click()
  await page.getByRole('menuitem', { name: 'Delete' }).click()
  await expect(nav.getByRole('link', { name: 'Planner all' })).toHaveCount(0)
})

test('search page highlights matches, opens the thread and saves the search', async ({ page }) => {
  await page.goto('/search')
  await page.getByRole('textbox', { name: 'Search' }).fill('quokka')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(page).toHaveURL(/q=quokka/)
  const hit = page.getByTestId('search-hit').filter({ hasText: 'Quokka rollout plan' }).first()
  await expect(hit).toBeVisible()
  await expect(hit.locator('mark').first()).toHaveText(/quokka/i)
  await expect(hit.getByTestId('hit-mailbox')).toBeVisible()

  // Scope to a mailbox that has no match: empty.
  await page.getByRole('combobox', { name: 'Mailbox' }).fill(PLANNER)
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(page.getByText('No messages match.')).toBeVisible()

  await page.getByRole('combobox', { name: 'Mailbox' }).fill('')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await page.getByRole('button', { name: 'Save search' }).click()
  await page.getByRole('dialog', { name: 'Save search' }).getByRole('button', { name: 'Save' }).click()
  const saved = page.getByRole('navigation', { name: 'Mailboxes' }).getByRole('link', { name: 'quokka' })
  await expect(saved).toBeVisible()
  await saved.click()
  await expect(page).toHaveURL(/\/search\?.*q=quokka/)
  await page.getByRole('navigation', { name: 'Mailboxes' }).getByRole('button', { name: 'Actions for quokka' }).click()
  await page.getByRole('menuitem', { name: 'Delete' }).click()
  await expect(saved).toHaveCount(0)

  await page.getByTestId('search-hit').filter({ hasText: 'Quokka rollout plan' }).first().click()
  await expect(page).toHaveURL(/\/mail\?/)
  await expect(thread(page).getByRole('heading', { name: 'Quokka rollout plan' })).toBeVisible()
})

test('command palette offers Search all mail', async ({ page }) => {
  await page.goto('/roster')
  await expect(page.getByTestId('roster-card').first()).toBeVisible()
  await page.keyboard.press('Control+k')
  const dialog = page.getByRole('dialog', { name: 'Command palette' })
  await dialog.getByRole('combobox').fill('quokka')
  await dialog.getByRole('option', { name: /Search all mail for/ }).click()
  await expect(page).toHaveURL(/\/search\?q=quokka/)
})

test('settings: webhook CRUD with masked URLs, send test, retention and backups', async ({ page }) => {
  mkdirSync(ARTIFACTS, { recursive: true })
  const secret = 'https://discord.com/api/webhooks/123456789/SUPERSECRETTOKEN'
  // The daemon only posts to discord.com / hooks.slack.com and has no host
  // override, so the test result is stubbed; everything else is real.
  await page.route('**/api/settings/webhooks/test*', (route) => route.fulfill({ json: { status: 0, error: 'stubbed network failure' } }))
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/settings')

  const hooks = page.getByTestId('webhook')
  await page.getByRole('button', { name: 'Add webhook' }).click()
  const dlg = page.getByRole('dialog', { name: 'Add webhook' })
  await dlg.getByLabel('Name').fill('ops-channel')
  await dlg.getByLabel(/Webhook URL/).fill(secret)
  await dlg.getByRole('button', { name: 'Save' }).click()
  const hook = hooks.filter({ hasText: 'ops-channel' })
  await expect(hook).toBeVisible()
  await expect(hook).toContainText('discord')
  await expect(hook).toContainText('****')
  await expect(page.locator('body')).not.toContainText('SUPERSECRETTOKEN')
  // Disabled straight away so nothing posts to the real host while the suite sends mail.
  await hook.getByRole('checkbox', { name: 'Enable ops-channel' }).click() // controlled: flips once the server answers
  await expect(hook.getByRole('checkbox', { name: 'Enable ops-channel' })).not.toBeChecked()

  await hook.getByRole('button', { name: 'Send test' }).click()
  await expect(hook.getByRole('alert')).toContainText('stubbed network failure')

  // Editing with an empty URL keeps the stored one (still masked, still present).
  await hook.getByRole('button', { name: 'Edit ops-channel' }).click()
  const edit = page.getByRole('dialog', { name: 'Edit webhook' })
  await expect(edit.getByLabel(/Webhook URL/)).toHaveValue('')
  await edit.getByLabel('Name').fill('ops-renamed')
  await edit.getByRole('button', { name: 'Save' }).click()
  const renamed = hooks.filter({ hasText: 'ops-renamed' })
  await expect(renamed).toContainText('****')

  // Rejects hosts other than Discord/Slack.
  await page.getByRole('button', { name: 'Add webhook' }).click()
  const bad = page.getByRole('dialog', { name: 'Add webhook' })
  await bad.getByLabel('Name').fill('evil')
  await bad.getByLabel(/Webhook URL/).fill('https://example.com/hook')
  await bad.getByRole('button', { name: 'Save' }).click()
  await expect(bad.getByRole('alert')).toBeVisible()
  await bad.getByRole('button', { name: 'Cancel' }).click()

  // Retention round-trips.
  await page.getByLabel(/Prune archived mail after/).fill('45')
  await page.getByRole('region', { name: 'Retention' }).getByRole('button', { name: 'Save' }).click()
  await page.reload()
  await expect(page.getByLabel(/Prune archived mail after/)).toHaveValue('45')
  await page.getByLabel(/Prune archived mail after/).fill('0')
  await page.getByRole('region', { name: 'Retention' }).getByRole('button', { name: 'Save' }).click()
  await page.getByRole('button', { name: 'Prune now' }).click()
  await expect(page.getByRole('dialog', { name: 'Prune archived mail?' })).toContainText('Retention is off')
  await page.getByRole('dialog', { name: 'Prune archived mail?' }).getByRole('button', { name: 'Cancel' }).click()

  // Back up now writes a backup and shows its time and size.
  await page.getByRole('button', { name: 'Back up now' }).click()
  await expect(page.getByRole('list', { name: 'Last backup' }).or(page.getByLabel('Last backup'))).not.toContainText('never', { timeout: 15_000 })

  await page.screenshot({ path: join(ARTIFACTS, 'settings.png'), animations: 'disabled' })

  await renamed.getByRole('button', { name: 'Delete ops-renamed' }).click()
  await page.getByRole('dialog', { name: 'Delete webhook?' }).getByRole('button', { name: 'Delete' }).click()
  await expect(hooks).toHaveCount(0)
})

test.describe('mobile 390x844', () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true })

  test('list to thread and back, drawer and buddy sheet, no overflow, 44px targets', async ({ page }) => {
    mkdirSync(ARTIFACTS, { recursive: true })
    await send('Mobile check')
    await page.goto('/mail')
    await expect(page.getByRole('navigation', { name: 'Mailboxes' })).toHaveCount(0)
    await expect(page.getByRole('complementary', { name: 'Buddy list' })).toHaveCount(0)
    await expect(list(page)).toBeVisible()
    expect(await noHScroll(page)).toBe(true)
    expect(await smallTargets(page)).toEqual([])

    // Single pane: list -> thread -> back.
    await row(page, 'Mobile check').tap()
    await expect(thread(page).getByRole('heading', { name: 'Mobile check' })).toBeVisible()
    await expect(list(page)).toHaveCount(0)
    expect(await noHScroll(page)).toBe(true)
    expect(await smallTargets(page)).toEqual([])
    await page.screenshot({ path: join(ARTIFACTS, 'mobile-thread.png'), animations: 'disabled' })
    await thread(page).getByRole('button', { name: 'Back' }).tap()
    await expect(list(page)).toBeVisible()
    await expect(thread(page)).toHaveCount(0)

    // Nav drawer: navigating closes it.
    await page.getByRole('button', { name: 'Open navigation' }).tap()
    const drawer = page.getByRole('dialog', { name: 'Navigation' })
    await expect(drawer).toBeVisible()
    expect(await smallTargets(page)).toEqual([])
    await drawer.getByRole('link', { name: 'Settings' }).tap()
    await expect(drawer).toHaveCount(0)
    await expect(page).toHaveURL(/\/settings/)
    expect(await noHScroll(page)).toBe(true)
    expect(await smallTargets(page)).toEqual([])

    // Buddy list sheet.
    await page.getByRole('button', { name: 'Open buddy list' }).tap()
    const buddies = page.getByRole('dialog', { name: 'Buddy list' })
    await expect(buddies.getByTestId('buddy').first()).toBeVisible()
    await page.keyboard.press('Escape')

    for (const path of ['/search?q=quokka', '/roster', '/org-chart']) {
      await page.goto(path)
      await expect(page.locator('main')).toBeVisible()
      expect(await noHScroll(page), `${path} overflows`).toBe(true)
      expect(await smallTargets(page), `${path} small targets`).toEqual([])
    }
  })
})
