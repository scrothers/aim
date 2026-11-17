import { mkdirSync } from 'node:fs'
import { hostname } from 'node:os'
import { join } from 'node:path'
import { Agent } from './agent'
import { expect, test } from '@playwright/test'

const ORG = 'github.com/acme'
const REPO = 'github.com/acme/api'
const COORD = `coordinator@${ORG}`
const REVIEWER = `reviewer@${REPO}`

const ARTIFACTS = join(import.meta.dirname, 'artifacts')

// The specs share one daemon and build on each other's state, so they run in
// order in a single worker.
test.describe.configure({ mode: 'serial' })

// globalSetup picks free ports and publishes the address through the
// environment, so baseURL cannot live in playwright.config.ts.
test.use({ baseURL: process.env.AIM_E2E_BASE_URL })

let coord: Agent
let reviewer: Agent

test.beforeAll(async () => {
  const bin = process.env.AIM_E2E_BIN!
  const addr = process.env.AIM_E2E_SOCKET!
  coord = await Agent.start(bin, addr, 'pw-session-coordinator', 'claude-code')
  reviewer = await Agent.start(bin, addr, 'pw-session-reviewer', 'codex')

  await coord.must('register', { address: COORD, role: 'lead', description: 'coordinates work', reports_to: 'operator' })
  await reviewer.must('register', { address: REVIEWER, role: 'reviewer', description: 'reviews code', reports_to: COORD })
  await coord.must('sign_on', { address: COORD, away: 'planning the sprint' })
  await reviewer.must('sign_on', { address: REVIEWER, away: 'reviewing PR 42' })
  await coord.must('send', { to: [REVIEWER], subject: 'Please review PR 42', body: 'Take a look when free.' })
})

test.afterAll(async () => {
  await Promise.all([coord?.stop(), reviewer?.stop()])
})

test('buddy list and mailbox tree show the signed-on agents', async ({ page }) => {
  await page.goto('/mail')

  const buddies = page.getByTestId('buddy')
  const c = buddies.filter({ hasText: 'coordinator' })
  await expect(c).toContainText('planning the sprint')
  await expect(c).toContainText('Claude Code')
  await expect(c).toContainText(hostname()) // the shim reports its machine
  const r = buddies.filter({ hasText: 'reviewer' })
  await expect(r).toContainText('reviewing PR 42')
  await expect(r).toContainText('Codex')

  const nav = page.getByRole('navigation', { name: 'Mailboxes' })
  const acme = nav.getByTestId('org-block').filter({ hasText: 'acme' })
  await expect(acme).toBeVisible()
  const repo = acme.getByTestId('repo-block').filter({ hasText: 'api' })
  await expect(repo).toBeVisible()
  await expect(repo).toContainText('1') // unread count from the review request
})

test('operator inbox: markdown message is rendered, sanitized and marked read', async ({ page }) => {
  await coord.must('send', {
    to: ['operator'],
    subject: 'Release plan zebrafish',
    body: [
      '## Plan',
      '',
      '| step | owner |',
      '|---|---|',
      '| build | coordinator |',
      '| review | reviewer |',
      '',
      '```go',
      'fmt.Println("hi")',
      '```',
      '',
      '<script>window.__pwned = true</script>',
      '<img src=x onerror="window.__pwned = true">',
      '[bad](javascript:alert(1))',
    ].join('\n'),
  })

  await page.goto('/mail')
  const list = page.getByRole('region', { name: 'Message list' })
  const item = list.getByRole('button', { name: /Release plan zebrafish/ })
  await expect(item).toBeVisible()
  await expect(item.getByRole('img', { name: 'Unread' })).toBeVisible()
  const pill = page.getByRole('link', { name: /Operator inbox/ }).getByLabel(/ unread$/)
  await expect(pill).toHaveAccessibleName('1 unread')

  await item.click()
  const thread = page.getByRole('article', { name: 'Thread' })
  await expect(thread.getByRole('heading', { name: 'Release plan zebrafish' })).toBeVisible()
  await expect(thread.locator('table')).toBeVisible()
  await expect(thread.getByRole('cell', { name: 'coordinator' })).toBeVisible()
  await expect(thread.locator('pre code')).toContainText('fmt.Println')
  await expect(thread.locator('script')).toHaveCount(0)
  await expect(thread.locator('[onerror]')).toHaveCount(0)
  await expect(thread.locator('a[href^="javascript:"]')).toHaveCount(0)
  expect(await page.evaluate(() => (window as unknown as { __pwned?: boolean }).__pwned)).toBeUndefined()

  // Opening marks it read: the unread pill and dot go away.
  await expect(pill).toHaveCount(0)
  await expect(item.getByRole('img', { name: 'Unread' })).toHaveCount(0)

  // Reply from the composer with Ctrl+Enter.
  await thread.getByRole('button', { name: 'Reply' }).click()
  await page.getByRole('textbox', { name: 'Message body' }).fill('Approved, ship it.')
  await page.getByRole('textbox', { name: 'Message body' }).press('Control+Enter')
  await expect(page.getByRole('form', { name: 'Reply' })).toHaveCount(0)

  const got = await coord.must('list', { address: COORD, state: 'all' })
  const subjects = (got.data.items as Array<{ from: string; subject: string }>).filter((m) => m.from === 'operator')
  expect(subjects.map((m) => m.subject)).toContain('Re: Release plan zebrafish')
})

test('roster: edit role, reject cycles, retire and unretire', async ({ page }) => {
  await page.goto('/roster')
  const card = (addr: string) => page.getByTestId('roster-card').filter({ hasText: addr })

  // Edit the reviewer's role.
  await card(REVIEWER).getByRole('button', { name: 'Edit' }).click()
  const sheet = page.getByRole('dialog', { name: 'Edit identity' })
  await sheet.getByLabel('Role').fill('senior reviewer')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toHaveCount(0)
  await expect(card(REVIEWER)).toContainText('senior reviewer')

  // coordinator -> reviewer would close the loop reviewer -> coordinator.
  // Cards are sorted by path, so the coordinator's is first; the reviewer's
  // card also mentions the coordinator in "reports to".
  await card(COORD).first().getByRole('button', { name: 'Edit' }).click()
  await sheet.getByLabel('Reports to').fill(REVIEWER)
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet.getByRole('alert')).toContainText(/cycle|loop|circular/i)
  await sheet.getByRole('button', { name: 'Cancel' }).click()
  await expect(sheet).toHaveCount(0)

  // Retire the reviewer: hidden until "Show retired".
  await card(REVIEWER).getByRole('button', { name: 'Retire' }).click()
  await page.getByRole('dialog', { name: 'Retire identity?' }).getByRole('button', { name: 'Retire' }).click()
  await expect(card(REVIEWER)).toHaveCount(0)
  await page.getByLabel('Show retired').check()
  await expect(card(REVIEWER)).toContainText(/Retired/)

  // Agents cannot mail a retired identity.
  const res = await coord.call('send', { to: [REVIEWER], subject: 'still there?', body: 'ping' })
  expect(res.isError).toBe(true)

  // Restore for the following specs.
  await card(REVIEWER).getByRole('button', { name: 'Unretire' }).click()
  await page.getByLabel('Show retired').uncheck()
  await expect(card(REVIEWER)).toBeVisible()
})

test('org chart shows operator, coordinator and reviewer', async ({ page }) => {
  await page.goto('/org-chart')
  const chart = page.getByRole('region', { name: 'Org chart' })
  await expect(chart.locator('[data-node="operator"]')).toBeVisible()
  await expect(chart.locator(`[data-node="${COORD}"]`)).toContainText('coordinator')
  await expect(chart.locator(`[data-node="${REVIEWER}"]`)).toContainText('reviewer')
  await expect(chart.locator(`[data-edge="operator>${COORD}"]`)).toHaveCount(1)
  await expect(chart.locator(`[data-edge="${COORD}>${REVIEWER}"]`)).toHaveCount(1)
})

test('command palette jumps to a mailbox and finds a message', async ({ page }) => {
  await page.goto('/roster')
  await expect(page.getByTestId('roster-card').first()).toBeVisible() // app mounted, key handler attached
  await page.keyboard.press('Control+k')
  const dialog = page.getByRole('dialog', { name: 'Command palette' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('combobox').fill('acme/api')
  await dialog.getByRole('option', { name: REPO, exact: true }).click()
  await expect(page).toHaveURL(/\/mail\?.*mailbox=github\.com(%2F|\/)acme(%2F|\/)api/)
  await expect(page.getByRole('region', { name: 'Message list' })).toContainText(REPO)

  await expect(dialog).toBeHidden()
  await page.keyboard.press('Control+k')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('combobox').fill('zebrafish')
  await dialog.getByRole('option', { name: /Release plan zebrafish/ }).first().click()
  await expect(page.getByRole('article', { name: 'Thread' }).getByRole('heading', { name: 'Release plan zebrafish' })).toBeVisible()
})

test('theme toggle switches the dark class', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/mail')
  const html = page.locator('html')
  await expect(html).not.toHaveClass(/dark/)
  await page.getByRole('button', { name: 'Switch to dark theme' }).click()
  await expect(html).toHaveClass(/dark/)
  await page.getByRole('button', { name: 'Switch to light theme' }).click()
  await expect(html).not.toHaveClass(/dark/)
})

test('screenshots of the main screen, light and dark', async ({ page }) => {
  mkdirSync(ARTIFACTS, { recursive: true })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/mail')
  await page.getByRole('region', { name: 'Message list' }).getByRole('button').first().click()
  await expect(page.getByTestId('buddy').first()).toBeVisible()
  await expect(page.locator('html')).not.toHaveClass(/dark/)
  await page.screenshot({ path: join(ARTIFACTS, 'mail-light.png') })

  await page.getByRole('button', { name: 'Switch to dark theme' }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.screenshot({ path: join(ARTIFACTS, 'mail-dark.png') })
})
