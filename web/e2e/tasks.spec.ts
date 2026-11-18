import { mkdirSync, mkdtempSync } from 'node:fs'
import { join } from 'node:path'
import { Agent } from './agent'
import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test'

const ORG = 'github.com/pwtasks'
const DEV = `dev@${ORG}`
const THREAD = 'Tasks e2e thread'

test.describe.configure({ mode: 'serial' })
test.use({ baseURL: process.env.AIM_E2E_BASE_URL, viewport: { width: 1440, height: 900 }, screenshot: 'only-on-failure' })
test.setTimeout(60_000)

// Set AIM_E2E_SHOTS to a directory to keep screenshots; otherwise they go to a temp dir.
const SHOTS =
  process.env.AIM_E2E_SHOTS ??
  (() => {
    const base = '/tmp'
    mkdirSync(base, { recursive: true })
    return mkdtempSync(join(base, 'aim-tasks-'))
  })()

let dev: Agent

test.beforeAll(async () => {
  dev = await Agent.start(process.env.AIM_E2E_BIN!, process.env.AIM_E2E_SOCKET!, 'pw-session-tasks-dev', 'claude-code')
  await dev.must('agent_register', { address: DEV, role: 'dev', description: 'does tasks', reports_to: 'operator' })
  await dev.must('agent_sign_on', { address: DEV, away: 'tasks spec' })
  await dev.must('mail_send', { to: ['operator'], subject: THREAD, body: 'Please plan the migration.' })
})

test.afterAll(async () => {
  await dev?.stop()
})

const HEADERS = { 'X-AIM-Request': '1', 'Content-Type': 'application/json' }

async function seed(request: APIRequestContext, body: Record<string, unknown>, actions: Array<Record<string, unknown>> = []): Promise<string> {
  const res = await request.post('/api/tasks', { data: body, headers: HEADERS })
  expect(res.ok(), await res.text()).toBe(true)
  const { id } = (await res.json()) as { id: string }
  for (const a of actions) {
    const r = await request.post(`/api/tasks/${id}/actions`, { data: a, headers: HEADERS })
    expect(r.ok(), await r.text()).toBe(true)
  }
  return id
}

const col = (page: Page, name: string) => page.getByRole('region', { name, exact: true })
const card = (page: Page, title: string) => page.getByTestId('task-card').filter({ hasText: title })
const drawer = (page: Page, title: string) => page.getByRole('dialog', { name: title })

test('create a task, assign it, comment, and move it across the board', async ({ page }) => {
  await page.goto('/tasks')
  for (const name of ['Not started', 'Claimed', 'Started', 'Abandoned', 'Finished']) await expect(col(page, name)).toBeVisible()
  await expect(col(page, 'Finished')).toHaveAttribute('data-collapsed', 'true')

  // Create with the identity type-ahead, then the drawer opens on the new task.
  await page.getByRole('button', { name: /^New task/ }).click()
  const sheet = page.getByRole('dialog', { name: 'New task' })
  await sheet.getByRole('textbox', { name: 'Title' }).fill('Cut the release')
  await sheet.getByRole('combobox', { name: 'Priority' }).selectOption('high')
  await sheet.getByRole('combobox', { name: 'Assignee' }).fill('dev@')
  // hasText, not name: the address chip's flex children get spaces in the accessible name.
  await sheet.getByRole('option').filter({ hasText: DEV }).click()
  await expect(sheet.getByRole('combobox', { name: 'Assignee' })).toHaveValue(DEV)
  await sheet.getByRole('button', { name: /Create task/ }).click()

  const dlg = drawer(page, 'Cut the release')
  await expect(dlg).toBeVisible()
  await expect(dlg.getByText('assigned it', { exact: false })).toBeVisible()
  await expect(dlg.getByRole('group', { name: 'Task actions' }).getByRole('button')).toHaveText(['Start', 'Claim'])

  // Assign someone else, unassign, then back.
  await dlg.getByRole('button', { name: 'Unassign' }).click()
  await expect(dlg.getByText('Nobody. Anyone may claim it.')).toBeVisible()
  await dlg.getByRole('combobox', { name: 'Assign to' }).fill(DEV)
  await dlg.getByRole('button', { name: 'Assign', exact: true }).click()
  await expect(dlg.getByRole('button', { name: 'Unassign' })).toBeVisible()

  await dlg.getByRole('textbox', { name: 'Comment' }).fill('Kicking this off')
  await dlg.getByRole('button', { name: 'Comment' }).click()
  const history = dlg.getByRole('list', { name: 'History' })
  await expect(history.getByText('Kicking this off')).toBeVisible()
  await expect(dlg.getByRole('textbox', { name: 'Comment' })).toHaveValue('')
  await page.keyboard.press('Escape')
  await expect(dlg).toHaveCount(0)

  const c = card(page, 'Cut the release')
  await expect(col(page, 'Not started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()
  await expect(c.getByLabel(`Assigned to ${DEV}`)).toBeVisible()
  await expect(c.getByText('high')).toBeVisible()

  // Drag to Started: runs start, the operator becomes the claimant.
  await c.dragTo(col(page, 'Started'))
  await expect(col(page, 'Started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()
  await expect(card(page, 'Cut the release').getByLabel('Claimed by operator')).toBeVisible()
  await expect(card(page, 'Cut the release').getByRole('img', { name: /^Health:/ })).toBeVisible()

  // Back to Not started needs confirmation; cancelling leaves it.
  await card(page, 'Cut the release').dragTo(col(page, 'Not started'))
  const confirm = page.getByRole('dialog', { name: /Move to Not started/ })
  await confirm.getByRole('button', { name: 'Cancel' }).click()
  await expect(col(page, 'Started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()
  await card(page, 'Cut the release').dragTo(col(page, 'Not started'))
  await confirm.getByRole('button', { name: 'Move task' }).click()
  await expect(col(page, 'Not started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()

  // Abandoned is not a drop target.
  await card(page, 'Cut the release').dragTo(col(page, 'Abandoned'))
  await expect(page.getByText('become abandoned on their own')).toBeVisible()
  await expect(col(page, 'Not started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()

  // Start, then drop on the collapsed Finished rail.
  await card(page, 'Cut the release').dragTo(col(page, 'Started'))
  await expect(col(page, 'Started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()
  await card(page, 'Cut the release').dragTo(col(page, 'Finished'))
  await expect(card(page, 'Cut the release')).toHaveCount(0)
  await page.getByRole('checkbox', { name: 'Show finished' }).check()
  await expect(col(page, 'Finished').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()

  // Reopen from the drawer.
  await card(page, 'Cut the release').getByRole('button', { name: 'Cut the release' }).click()
  await drawer(page, 'Cut the release').getByRole('button', { name: 'Reopen' }).click()
  await expect(drawer(page, 'Cut the release').getByRole('group', { name: 'Task actions' }).getByRole('button')).toHaveText(['Start', 'Claim'])
  await page.keyboard.press('Escape')
  await expect(col(page, 'Not started').getByTestId('task-card').filter({ hasText: 'Cut the release' })).toBeVisible()
})

test('edit title, priority, description and due date in the drawer', async ({ page, request }) => {
  await seed(request, { title: 'Edit me', body: 'First **draft**' })
  await page.goto('/tasks')
  await card(page, 'Edit me').getByRole('button', { name: 'Edit me' }).click()
  const dlg = page.getByRole('dialog') // not by name: the title changes below
  await expect(dlg.locator('.prose-aim strong')).toHaveText('draft')

  await dlg.getByRole('button', { name: 'Edit me', exact: true }).click()
  await dlg.getByRole('textbox', { name: 'Title' }).fill('Edited title')
  await page.keyboard.press('Enter')
  await expect(drawer(page, 'Edited title')).toBeVisible()

  await dlg.getByRole('combobox', { name: 'Priority' }).selectOption('urgent')
  await dlg.getByLabel('Due date').fill('2020-01-02')
  await expect(card(page, 'Edited title').getByText('urgent')).toBeVisible()
  await expect(card(page, 'Edited title').locator('[data-overdue]')).toBeVisible()
  await dlg.getByRole('button', { name: 'Clear due date' }).click()
  await expect(card(page, 'Edited title').locator('[data-overdue]')).toHaveCount(0)

  await dlg.getByRole('button', { name: 'Edit', exact: true }).click()
  const editor = dlg.getByRole('textbox', { name: 'Description' })
  await expect(editor).toBeVisible()
  await editor.click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type(' and more')
  await dlg.getByRole('button', { name: 'Save' }).click()
  await expect(dlg.locator('.prose-aim')).toContainText('and more')
})

test('create a task from a thread and see its chip on the thread', async ({ page }) => {
  await page.goto('/mail')
  await page.getByRole('region', { name: 'Message list' }).getByRole('button', { name: new RegExp(THREAD) }).click()
  const thread = page.getByRole('article', { name: 'Thread' })
  await expect(thread.getByRole('heading', { name: THREAD })).toBeVisible()
  await thread.getByRole('button', { name: 'Create task from thread' }).click()
  const sheet = page.getByRole('dialog', { name: 'New task' })
  await expect(sheet.getByRole('textbox', { name: 'Title' })).toHaveValue(THREAD)
  await sheet.getByRole('button', { name: /Create task/ }).click()
  await expect(sheet).toHaveCount(0)

  const chip = thread.getByRole('button', { name: `Task: ${THREAD} (Not started)` })
  await expect(chip).toBeVisible()
  // The nav badge counts open tasks.
  await expect(page.getByRole('link', { name: /^Tasks/ }).locator('[aria-label$=" open"]')).toBeVisible()

  await chip.click()
  const dlg = drawer(page, THREAD)
  await expect(dlg).toBeVisible()
  const link = dlg.getByRole('button', { name: /linked by/ })
  await expect(link).toBeVisible()
  await dlg.getByRole('button', { name: 'Start' }).click()
  await page.keyboard.press('Escape')
  await expect(thread.getByRole('button', { name: `Task: ${THREAD} (Started)` })).toBeVisible()

  // The drawer's linked thread leads back to the mail view.
  await thread.getByRole('button', { name: `Task: ${THREAD} (Started)` }).click()
  await drawer(page, THREAD).getByRole('button', { name: /linked by/ }).click()
  await expect(page).toHaveURL(/\/mail\?.*thread=/)
  await expect(drawer(page, THREAD)).toHaveCount(0)
})

test('keyboard: t opens the board and n starts a new task', async ({ page }) => {
  await page.goto('/mail')
  await expect(page.getByRole('link', { name: /^Tasks/ })).toBeVisible() // the app has mounted and bound its keys
  await page.keyboard.press('t')
  await expect(page).toHaveURL(/\/tasks$/)
  await expect(page.getByRole('heading', { name: 'Tasks', level: 1 })).toBeVisible() // the lazy route has bound `n`
  await page.keyboard.press('n')
  await expect(page.getByRole('dialog', { name: 'New task' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.keyboard.press('ControlOrMeta+k')
  const palette = page.getByRole('dialog', { name: 'Command palette' })
  await palette.getByPlaceholder(/Search mail/).fill('new task')
  await palette.getByRole('option', { name: 'New task', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'New task' })).toBeVisible()
})

// ---------------------------------------------------------------- visual checks

type Rgb = [number, number, number, number]

async function color(loc: Locator, prop: 'color' | 'backgroundColor'): Promise<Rgb> {
  return loc.evaluate((el, p) => {
    const raw = getComputedStyle(el)[p as 'color']
    const c = document.createElement('canvas').getContext('2d')!
    c.fillStyle = '#000'
    c.fillStyle = raw
    c.fillRect(0, 0, 1, 1)
    const d = Array.from(c.getImageData(0, 0, 1, 1).data)
    return [d[0]!, d[1]!, d[2]!, d[3]! / 255] as [number, number, number, number]
  }, prop)
}

async function effectiveBg(loc: Locator): Promise<Rgb> {
  return loc.evaluate((el) => {
    const c = document.createElement('canvas').getContext('2d')!
    let n: Element | null = el
    while (n) {
      const raw = getComputedStyle(n).backgroundColor
      c.clearRect(0, 0, 1, 1)
      c.fillStyle = raw
      c.fillRect(0, 0, 1, 1)
      const d = Array.from(c.getImageData(0, 0, 1, 1).data)
      if (d[3]! > 250) return [d[0]!, d[1]!, d[2]!, 1] as [number, number, number, number]
      n = n.parentElement
    }
    return [255, 255, 255, 1] as [number, number, number, number]
  })
}

function lum([r, g, b]: Rgb): number {
  const f = (v: number) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
}

async function expectReadable(loc: Locator, min = 4.5) {
  const fg = await color(loc, 'color')
  const bg = await effectiveBg(loc)
  const [x, y] = [lum(fg), lum(bg)]
  const ratio = (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
  expect(ratio, `contrast of ${fg} on ${bg}`).toBeGreaterThanOrEqual(min)
}

for (const theme of ['light', 'dark'] as const) {
  test.describe(`${theme} theme`, () => {
    test.beforeEach(async ({ page }) => {
      await page.emulateMedia({ colorScheme: theme })
      await page.addInitScript((t) => localStorage.setItem('aim-theme', t), theme)
    })

    test('board and drawer are readable', async ({ page, request }) => {
      const yesterday = new Date(Date.now() - 86_400_000).toISOString()
      await seed(request, { title: `Shot overdue ${theme}`, priority: 'urgent', scope: ORG, due_at: yesterday, assignee: DEV }, [{ action: 'start' }])
      await seed(request, { title: `Shot claimed ${theme}`, priority: 'low', scope: ORG }, [{ action: 'claim' }])
      await seed(request, { title: `Shot waiting ${theme}`, priority: 'normal', body: 'Needs a **look**.' })
      await seed(request, { title: `Shot done ${theme}` }, [{ action: 'start' }, { action: 'finish' }])

      await page.goto('/tasks')
      await expect(page.locator('html')).toHaveClass(theme === 'dark' ? /dark/ : /^(?!.*dark)/)
      for (const name of ['Not started', 'Claimed', 'Started', 'Abandoned', 'Finished']) await expect(col(page, name)).toBeVisible()
      await expect(card(page, `Shot overdue ${theme}`)).toBeVisible()
      await page.getByRole('checkbox', { name: 'Show finished' }).check()
      await expect(card(page, `Shot done ${theme}`)).toBeVisible()
      await page.screenshot({ path: join(SHOTS, `board-${theme}.png`), animations: 'disabled' })

      // Text on the board stays legible.
      const over = card(page, `Shot overdue ${theme}`)
      await expectReadable(over.getByRole('button', { name: `Shot overdue ${theme}` }))
      await expectReadable(over.locator('[data-priority]'))
      await expectReadable(over.locator('[data-overdue]'))
      await expectReadable(over.locator('[data-health]'))
      await expectReadable(over.getByLabel(`Assigned to ${DEV}`))
      await expectReadable(over.getByText(ORG))
      await expectReadable(card(page, `Shot claimed ${theme}`).locator('[data-priority]'))
      await expectReadable(col(page, 'Started').getByRole('heading', { name: 'Started' }))
      await expectReadable(page.getByRole('link', { name: /^Tasks/ }))

      await card(page, `Shot waiting ${theme}`).getByRole('button', { name: `Shot waiting ${theme}` }).click()
      const dlg = drawer(page, `Shot waiting ${theme}`)
      await expect(dlg.locator('.prose-aim strong')).toHaveText('look')
      await dlg.getByRole('textbox', { name: 'Comment' }).fill('A comment for the screenshot')
      await dlg.getByRole('button', { name: 'Comment' }).click()
      await expect(dlg.getByRole('list', { name: 'History' }).getByText('A comment for the screenshot')).toBeVisible()
      await dlg.getByRole('combobox', { name: 'Assign to' }).fill('dev@')
      await expect(dlg.getByRole('listbox', { name: 'Assign to suggestions' })).toBeVisible()
      await page.screenshot({ path: join(SHOTS, `drawer-${theme}.png`), animations: 'disabled' })
      await expectReadable(dlg.getByRole('button', { name: `Shot waiting ${theme}`, exact: true }))
      await expectReadable(dlg.getByRole('textbox', { name: 'Comment' }))
      await expectReadable(dlg.locator('.prose-aim'))
      await expectReadable(dlg.getByRole('option').filter({ hasText: DEV }))
      await expectReadable(dlg.getByRole('list', { name: 'History' }).getByText('A comment for the screenshot'))
    })

    test('narrow screens scroll the board, not the page', async ({ page }) => {
      await page.setViewportSize({ width: 390, height: 800 })
      await page.goto('/tasks')
      await expect(page.getByRole('heading', { name: 'Tasks', level: 1 })).toBeVisible()
      await expect(card(page, 'Shot waiting').first()).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.screenshot({ path: join(SHOTS, `board-narrow-${theme}.png`), animations: 'disabled' })
      await card(page, 'Shot waiting').first().getByRole('button').click()
      await expect(page.getByRole('dialog')).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.screenshot({ path: join(SHOTS, `drawer-narrow-${theme}.png`), animations: 'disabled' })
    })
  })
}
