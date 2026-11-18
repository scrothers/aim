import { Agent } from './agent'
import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test'

// DOM-measured layout checks against the shared daemon. They seed their own
// mail (operator -> several org/repo/identity addresses) and read it from the
// Sent view, so they need no other spec's state.
test.describe.configure({ mode: 'serial' })
test.use({ baseURL: process.env.AIM_E2E_BASE_URL })

const HEADERS = { 'X-AIM-Request': '1', 'Content-Type': 'application/json' }
const ORGS = ['layalpha', 'laybravo', 'laycharlie', 'laydelta', 'layecho']
const SUBJECT = 'Layout probe'

async function post(request: APIRequestContext, path: string, data: unknown) {
  const res = await request.post(path, { data, headers: HEADERS })
  expect(res.ok(), await res.text()).toBe(true)
  return res
}

test.beforeAll(async ({ request }) => {
  // 5 orgs x 2 repos x 2 identities: a mailbox tree far taller than 900px once expanded.
  const addresses = ORGS.flatMap((org) => ['api', 'web'].flatMap((repo) => ['builder', 'reviewer'].map((who) => `${who}@github.com/${org}/${repo}`)))
  const agent = await Agent.start(process.env.AIM_E2E_BIN!, process.env.AIM_E2E_SOCKET!, 'pw-session-layout', 'claude-code')
  try {
    for (const address of addresses) await agent.must('agent_register', { address, role: 'probe', description: 'layout probe', reports_to: 'operator' })
  } finally {
    await agent.stop()
  }
  for (const [i, address] of addresses.entries()) {
    await post(request, '/api/messages', { to: [address], subject: `${SUBJECT} ${i + 1}`, body: `Probe ${i + 1} for ${address}.` })
  }
})

const header = (page: Page) => page.locator('header').first()
const list = (page: Page) => page.getByRole('region', { name: 'Message list' })
const thread = (page: Page) => page.getByRole('article', { name: 'Thread' })

const noHScroll = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)

async function box(loc: Locator) {
  const b = await loc.boundingBox()
  expect(b, 'element has a box').not.toBeNull()
  return b!
}

for (const size of [
  { width: 1024, height: 768 },
  { width: 1280, height: 800 },
]) {
  test(`opening a thread at ${size.width}x${size.height} shows it in the viewport`, async ({ page }) => {
    await page.setViewportSize(size)
    await page.goto('/mail?view=sent')
    await list(page).getByRole('button', { name: new RegExp(SUBJECT) }).first().click()
    await expect(thread(page).getByRole('heading', { level: 1 })).toContainText(SUBJECT)
    const b = await box(thread(page))
    expect(b.y).toBeGreaterThanOrEqual(0)
    expect(b.y).toBeLessThan(size.height / 2)
    const h = await box(thread(page).getByRole('heading', { level: 1 }))
    expect(h.y + h.height).toBeLessThanOrEqual(size.height)
    expect(await noHScroll(page)).toBe(true)
  })
}

test('at 1024 the header stays one 56px row and overlaps nothing', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 })
  for (const path of ['/mail?view=sent', '/tasks', '/search?q=probe', '/org-chart']) {
    await page.goto(path)
    await expect(page.locator('main')).toBeVisible()
    const hb = await box(header(page))
    expect(hb.height, path).toBe(56)
    const escaped = await header(page).evaluate((h) => {
      const r = h.getBoundingClientRect()
      const out: string[] = []
      for (const el of h.querySelectorAll('*')) {
        const e = el.getBoundingClientRect()
        if (e.width === 0 || e.height === 0) continue
        if (e.top < r.top - 0.5 || e.bottom > r.bottom + 0.5 || e.right > r.right + 0.5) out.push(el.outerHTML.slice(0, 80))
      }
      return out
    })
    expect(escaped, `${path}: header children outside the header`).toEqual([])
    const mb = await box(page.locator('main'))
    expect(mb.y, path).toBeGreaterThanOrEqual(hb.y + hb.height - 0.5)
    expect(await noHScroll(page), path).toBe(true)
  }
})

test('at 1440x900 Tasks and Approvals stay on screen with every org expanded', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/mail')
  const nav = page.getByRole('navigation', { name: 'Mailboxes' })
  const orgs = nav.getByTestId('org-block')
  await expect(orgs.filter({ hasText: 'layecho' })).toBeVisible()
  expect(await orgs.count()).toBeGreaterThanOrEqual(ORGS.length)
  // Expand everything: the tree scrolls in its own region, the work links don't move.
  for (const org of ORGS) await nav.getByRole('button', { name: `Expand ${org}` }).click()
  const scroller = nav.getByTestId('mailbox-scroll')
  expect(await scroller.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true)
  for (const name of [/^Tasks/, /^Approvals/, /^Operator inbox/]) {
    const b = await box(nav.getByRole('link', { name }))
    expect(b.y).toBeGreaterThanOrEqual(0)
    expect(b.y + b.height).toBeLessThanOrEqual(900)
  }
  for (const name of ['Roster', 'Org chart', 'Settings']) {
    const b = await box(nav.getByRole('link', { name, exact: true }))
    expect(b.y + b.height, name).toBeLessThanOrEqual(900)
  }
  expect(await nav.evaluate((el) => el.scrollTop)).toBe(0)
  expect(await page.evaluate(() => document.scrollingElement!.scrollTop)).toBe(0)
  expect(await noHScroll(page)).toBe(true)
})

test.describe('phone 390x844', () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true })

  /** Interactive elements in `dialog` whose box intersects its Close button. */
  async function closeOverlaps(dialog: Locator): Promise<string[]> {
    return dialog.evaluate((d) => {
      const close = [...d.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Close' || b.getAttribute('aria-label') === 'Close')
      if (!close) return ['no close button']
      const c = close.getBoundingClientRect()
      const out: string[] = []
      const sel = 'button, a[href], input, select, textarea, [role=switch], [role=tab], [contenteditable=true]'
      for (const el of d.querySelectorAll(sel)) {
        if (el === close || el.contains(close) || close.contains(el)) continue
        const r = el.getBoundingClientRect()
        if (r.width === 0 || r.height === 0 || getComputedStyle(el).visibility === 'hidden') continue
        const hit = r.left < c.right && r.right > c.left && r.top < c.bottom && r.bottom > c.top
        if (hit) out.push((el.getAttribute('aria-label') || el.textContent || el.tagName).trim().slice(0, 40))
      }
      return out
    })
  }

  test('sheet close buttons never sit on other controls', async ({ page, request }) => {
    const res = await post(request, '/api/tasks', { title: 'Layout drawer probe', body: 'Check the close button.' })
    const { id } = (await res.json()) as { id: string }
    expect(id).toBeTruthy()

    await page.goto('/mail?view=sent')
    await expect(list(page)).toBeVisible()
    expect(await noHScroll(page)).toBe(true)

    await page.getByRole('button', { name: 'Open navigation' }).tap()
    const nav = page.getByRole('dialog', { name: 'Navigation' })
    await expect(nav.getByRole('button', { name: 'Compose' })).toBeVisible()
    expect(await closeOverlaps(nav)).toEqual([])
    await page.keyboard.press('Escape')
    await expect(nav).toHaveCount(0)

    await page.getByRole('button', { name: 'Open buddy list' }).tap()
    const buddies = page.getByRole('dialog', { name: 'Buddy list' })
    await expect(buddies).toBeVisible()
    expect(await closeOverlaps(buddies)).toEqual([])
    await page.keyboard.press('Escape')
    await expect(buddies).toHaveCount(0)

    // The floating Compose button opens the composer.
    const fab = page.getByRole('button', { name: 'Compose' })
    const fb = await box(fab)
    expect(fb.x + fb.width).toBeLessThanOrEqual(390)
    expect(fb.y + fb.height).toBeLessThanOrEqual(844)
    await fab.tap()
    const compose = page.getByRole('dialog', { name: 'New message' })
    await expect(compose).toBeVisible()
    expect(await closeOverlaps(compose)).toEqual([])
    await page.keyboard.press('Escape')
    await expect(compose).toHaveCount(0)

    await page.goto('/tasks')
    await page.getByTestId('task-card').filter({ hasText: 'Layout drawer probe' }).getByRole('button', { name: 'Layout drawer probe' }).tap()
    const drawer = page.getByRole('dialog', { name: 'Layout drawer probe' })
    await expect(drawer).toBeVisible()
    expect(await closeOverlaps(drawer)).toEqual([])
    expect(await noHScroll(page)).toBe(true)
  })

  test('the top bar fits: search is an icon and nothing wraps', async ({ page }) => {
    await page.goto('/mail')
    const hb = await box(header(page))
    expect(hb.height).toBe(56)
    const search = page.getByRole('button', { name: 'Search mail, mailboxes and agents' })
    const sb = await box(search)
    expect(sb.width).toBeLessThanOrEqual(48)
    await search.tap()
    await expect(page.getByRole('dialog', { name: 'Command palette' })).toBeVisible()
    expect(await noHScroll(page)).toBe(true)
  })
})
