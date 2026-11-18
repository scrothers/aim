import { expect, test } from '@playwright/test'

// DOM-measured layout checks for page-level fixes: no screenshots, only bounding boxes.

test.use({ baseURL: process.env.AIM_E2E_BASE_URL, viewport: { width: 1440, height: 900 } })

test('task board: columns fit the board, or the board shows it scrolls', async ({ page }) => {
  await page.goto('/tasks')
  const board = page.getByTestId('task-board')
  await expect(board).toBeVisible()
  await expect(board.locator('section[data-status]')).toHaveCount(5)
  const { fits, overflows } = await board.evaluate((el) => {
    const view = el.getBoundingClientRect()
    const cols = [...el.querySelectorAll<HTMLElement>('section[data-status]')].map((c) => c.getBoundingClientRect())
    return {
      fits: cols.every((c) => c.left >= view.left - 1 && c.right <= view.right + 1),
      overflows: el.scrollWidth > el.clientWidth + 1,
    }
  })
  if (!fits) {
    expect(overflows).toBe(true)
    await expect(page.getByTestId('board-fade-right')).toBeVisible()
  } else {
    await expect(page.getByTestId('board-fade-right')).toHaveCount(0)
  }
  // Columns are narrow enough that five (minimum widths plus gaps) need at most 1010px.
  const minTotal = await board.evaluate((el) => {
    const cols = [...el.querySelectorAll<HTMLElement>('section[data-status]:not([data-collapsed])')]
    return cols.reduce((n, c) => n + parseFloat(getComputedStyle(c).minWidth), 0)
  })
  expect(minTotal).toBeLessThanOrEqual(5 * 192)
})

test('task board: scrolling sideways reveals the left fade', async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 800 })
  await page.goto('/tasks')
  const board = page.getByTestId('task-board')
  await expect(board).toBeVisible()
  const overflows = await board.evaluate((el) => el.scrollWidth > el.clientWidth + 1)
  test.skip(!overflows, 'board fits at this width')
  await expect(page.getByTestId('board-fade-right')).toBeVisible()
  await expect(page.getByTestId('board-fade-left')).toHaveCount(0)
  await board.evaluate((el) => el.scrollTo({ left: 80 }))
  await expect(page.getByTestId('board-fade-left')).toBeVisible()
})

test('task board: mobile filters sit behind a Filters button', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 800 })
  await page.goto('/tasks')
  const toggle = page.getByRole('button', { name: /^Filters/ })
  await expect(toggle).toBeVisible()
  await expect(page.getByRole('group', { name: 'Task filters' })).toBeHidden()
  const boardTop = (await page.getByTestId('task-board').boundingBox())!.y
  expect(boardTop).toBeLessThan(400)
  await toggle.click()
  await expect(page.getByRole('group', { name: 'Task filters' })).toBeVisible()
})

test('approvals at 390: Deny and Allow share a row with Allow as the primary', async ({ page }) => {
  const soon = new Date(Date.now() + 200_000).toISOString()
  await page.route('**/api/approvals', (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: 'appr-e2e',
            machine: 'devbox',
            tool: 'Bash',
            client: 'claude-code',
            created_at: new Date(Date.now() - 60_000).toISOString(),
            expires_at: soon,
            input: { command: 'ls -la' },
          },
        ],
      },
    }),
  )
  await page.setViewportSize({ width: 390, height: 800 })
  await page.goto('/approvals')
  const allow = page.getByRole('button', { name: 'Allow' })
  const deny = page.getByRole('button', { name: 'Deny' })
  await expect(allow).toBeVisible()
  const [a, d] = [(await allow.boundingBox())!, (await deny.boundingBox())!]
  expect(Math.abs(a.y - d.y)).toBeLessThanOrEqual(2)
  expect(d.x).toBeLessThan(a.x) // secondary then primary, like the dialogs
  expect(a.x + a.width).toBeLessThanOrEqual(390)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('org chart opens on the root node', async ({ page }) => {
  await page.goto('/org-chart')
  const root = page.locator('[data-node="operator"]')
  await expect(root).toBeVisible()
  const box = (await root.boundingBox())!
  const view = page.viewportSize()!
  expect(box.x).toBeGreaterThanOrEqual(0)
  expect(box.x + box.width).toBeLessThanOrEqual(view.width)
  expect(box.y).toBeGreaterThanOrEqual(0)
  expect(box.y + box.height).toBeLessThanOrEqual(view.height)
  // The operator is centred in the scroller, not parked at an arbitrary scroll offset.
  const off = await page.getByTestId('org-chart-scroll').evaluate((el, r) => {
    const v = el.getBoundingClientRect()
    return Math.abs(r.x + r.width / 2 - (v.left + v.width / 2))
  }, box)
  expect(off).toBeLessThan(40)
})

test('org chart zoom and fit controls resize the tree', async ({ page }) => {
  await page.goto('/org-chart')
  const root = page.locator('[data-node="operator"]')
  const w100 = (await root.boundingBox())!.width
  await page.getByRole('button', { name: 'Zoom out' }).click()
  await expect(page.getByLabel('Zoom level')).toHaveText('75%')
  expect((await root.boundingBox())!.width).toBeLessThan(w100)
  await page.getByRole('button', { name: 'Reset' }).click()
  await expect(page.getByLabel('Zoom level')).toHaveText('100%')
  await page.getByRole('button', { name: 'Fit width' }).click()
  const scroll = page.getByTestId('org-chart-scroll')
  expect(await scroll.evaluate((el) => el.scrollWidth <= el.clientWidth + 2)).toBe(true)
})
