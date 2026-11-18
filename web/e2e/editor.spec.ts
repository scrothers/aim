import { mkdirSync, mkdtempSync } from 'node:fs'
import { join } from 'node:path'
import { Agent } from './agent'
import { expect, test, type Locator, type Page } from '@playwright/test'

const COORD = 'styler@github.com/pwedit'

test.describe.configure({ mode: 'serial' })
test.use({ baseURL: process.env.AIM_E2E_BASE_URL, viewport: { width: 1280, height: 800 }, screenshot: 'only-on-failure' })
test.setTimeout(60_000)

// Set AIM_E2E_SHOTS to a directory to keep screenshots; otherwise they go to a temp dir.
const SHOTS = process.env.AIM_E2E_SHOTS ?? (() => {
  const base = '/tmp'
  mkdirSync(base, { recursive: true })
  return mkdtempSync(join(base, 'aim-editor-'))
})()

let coord: Agent

test.beforeAll(async () => {
  coord = await Agent.start(process.env.AIM_E2E_BIN!, process.env.AIM_E2E_SOCKET!, 'pw-editor-coordinator', 'claude-code')
  await coord.must('agent_register', { address: COORD, role: 'lead', description: 'coordinates work', reports_to: 'operator' })
  await coord.must('agent_sign_on', { address: COORD, away: 'editor spec' })
  await coord.must('mail_send', { to: ['operator'], subject: 'Editor styling thread', body: 'Please reply with the plan.' })
})

test.afterAll(async () => {
  await coord?.stop()
})

type Rgb = [number, number, number, number]

/** Computed colour of an element, resolved to rgb(a) through a canvas. */
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

/** Background colour, walking up past transparent ancestors. */
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
function contrast(a: Rgb, b: Rgb): number {
  const [x, y] = [lum(a), lum(b)]
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}

async function expectReadable(loc: Locator, min = 4.5) {
  const fg = await color(loc, 'color')
  const bg = await effectiveBg(loc)
  expect(contrast(fg, bg), `contrast of ${fg} on ${bg}`).toBeGreaterThanOrEqual(min)
}

async function shoot(page: Page, name: string) {
  await page.screenshot({ path: join(SHOTS, `${name}.png`) })
}

for (const theme of ['light', 'dark'] as const) {
  test.describe(`${theme} theme`, () => {
    test.beforeEach(async ({ page }) => {
      await page.addInitScript((t) => localStorage.setItem('aim-theme', t), theme)
    })

    test('compose sheet: editor is tall, readable and its popups follow the theme', async ({ page }) => {
      await page.goto('/mail')
      await page.getByRole('button', { name: 'Compose' }).click()
      const sheet = page.getByRole('dialog')
      const editor = sheet.getByRole('textbox', { name: 'Message body' })
      await expect(editor).toBeVisible()
      await expect(page.locator('html')).toHaveClass(theme === 'dark' ? /dark/ : /^(?!.*dark)/)

      // Empty editor already fills the sheet: at least ~16 lines of 20px.
      const root = sheet.locator('.aim-editor.mdxeditor')
      expect((await root.boundingBox())!.height).toBeGreaterThanOrEqual(400)
      expect((await editor.boundingBox())!.height).toBeGreaterThanOrEqual(300)
      await shoot(page, `${theme}-sheet-empty`)

      await editor.click()
      await page.keyboard.type('## Release notes')
      await page.keyboard.press('Enter')
      await page.keyboard.type('Some **bold** text and `inline code` here.')
      await page.keyboard.press('Enter')
      await page.keyboard.type('- first item')
      await page.keyboard.press('Enter')
      await page.keyboard.type('second item')
      await page.keyboard.press('Enter')
      await page.keyboard.press('Enter')
      await page.keyboard.type('> a quoted line')
      await page.keyboard.press('Enter')
      await page.keyboard.press('Enter')

      await expectReadable(editor.locator('h2'))
      await expectReadable(editor.locator('li').first())
      await expectReadable(editor.locator('blockquote'), 4.5)
      await expectReadable(editor.locator('code').first())
      await expectReadable(sheet.locator('.mdxeditor-toolbar button[data-state]').first(), 4.5)

      // Table and code block.
      await sheet.getByRole('button', { name: 'Insert Table' }).click()
      await expect(editor.locator('table')).toBeVisible()
      const head = editor.locator('table th:has(p)').first()
      const body = editor.locator('table td:has(p)').first()
      await head.click()
      await page.keyboard.type('head')
      await body.click()
      await page.keyboard.type('cell')
      await expect(head).toContainText('head')
      await expect(body).toContainText('cell')
      await expectReadable(head.locator('p'))
      await expectReadable(body.locator('p'))
      // Click the empty area under the table: the whole editor surface is a click target.
      await editor.click({ position: { x: 20, y: (await editor.boundingBox())!.height - 12 } })
      await sheet.getByRole('button', { name: 'Insert Code Block' }).click()
      const cm = editor.locator('.cm-editor')
      await expect(cm).toBeVisible()
      await cm.locator('.cm-content').click({ position: { x: 20, y: 8 } })
      await page.keyboard.type('func main() {\n\treturn 1\n}')
      await expectReadable(cm.locator('.cm-content'))
      await expectReadable(cm.locator('.cm-line').first())

      // Code-block language picker (portal).
      await editor.locator('[class*="codeMirrorToolbar"] [role="combobox"]').click()
      const opt = page.getByRole('option', { name: 'Go' })
      await expect(opt).toBeVisible()
      await expectReadable(opt, 4.5)
      await shoot(page, `${theme}-sheet-language`)
      await opt.click()
      await expect(cm.locator('.cm-line').first()).toContainText('func')
      // Syntax colours are brightness-filtered per theme (not visible to getComputedStyle); checked in screenshots.
      await expect(cm.locator('.cm-line span').first()).toHaveCSS('filter', theme === 'dark' ? 'brightness(1.45)' : 'brightness(0.72)')

      // Link dialog (portal).
      await editor.locator('h2').click()
      await sheet.getByRole('button', { name: 'Create link' }).click()
      const url = page.locator('[class*="linkDialogPopoverContent"] input').first()
      await expect(url).toBeVisible()
      await url.fill('https://example.com')
      await expectReadable(url, 4.5)
      await shoot(page, `${theme}-sheet-link`)
      await page.keyboard.press('Escape')

      // Block type dropdown.
      await sheet.getByRole('combobox', { name: /block type/i }).click()
      await expectReadable(page.getByRole('option').first(), 4.5)
      await shoot(page, `${theme}-sheet-blocktype`)
      await page.keyboard.press('Escape')

      await shoot(page, `${theme}-sheet-filled`)

      // Source mode keeps its size and theme.
      await sheet.locator('.mdxeditor-toolbar button').last().click()
      await expect(sheet.locator('.mdxeditor-source-editor .cm-editor')).toBeVisible()
      expect((await sheet.locator('.aim-editor.mdxeditor').boundingBox())!.height).toBeGreaterThanOrEqual(400)
      await expect.poll(async () => (await sheet.locator('.mdxeditor-source-editor .cm-editor').boundingBox())!.height).toBeGreaterThanOrEqual(300)
      await expectReadable(sheet.locator('.mdxeditor-source-editor .cm-content'))
      await shoot(page, `${theme}-sheet-source`)
    })

    test('inline reply: at least ~8 lines, grows, readable', async ({ page }) => {
      await page.goto('/mail')
      await page.getByRole('button', { name: /Editor styling thread/ }).click()
      const thread = page.getByRole('article', { name: 'Thread' })
      await thread.getByRole('button', { name: 'Reply' }).click()
      const form = page.getByRole('form', { name: 'Reply' })
      const editor = form.getByRole('textbox', { name: 'Message body' })
      await expect(editor).toBeVisible()
      const h0 = (await editor.boundingBox())!.height
      expect(h0).toBeGreaterThanOrEqual(160)
      await editor.click()
      await page.keyboard.type('Reply text that must be readable.')
      await expectReadable(editor.locator('p').first())
      await shoot(page, `${theme}-reply-short`)

      for (let i = 0; i < 25; i++) {
        await page.keyboard.press('Enter')
        await page.keyboard.type(`line ${i}`)
      }
      const h1 = (await form.locator('.aim-editor.mdxeditor').boundingBox())!.height
      expect(h1).toBeLessThanOrEqual(800 * 0.7)
      expect((await editor.boundingBox())!.height).toBeGreaterThan(h0)
      await shoot(page, `${theme}-reply-long`)
    })
  })
}

test('editor follows a runtime theme toggle', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('aim-theme', 'light'))
  await page.goto('/mail')
  await page.getByRole('button', { name: 'Compose' }).click()
  const editor = page.getByRole('dialog').getByRole('textbox', { name: 'Message body' })
  await editor.click()
  await page.keyboard.type('toggle me')
  const root = page.locator('.aim-editor.mdxeditor')
  const light = await effectiveBg(root)
  await page.evaluate(() => document.documentElement.classList.add('dark'))
  await expect.poll(async () => lum(await effectiveBg(root))).toBeLessThan(lum(light) / 2)
  await expectReadable(editor.locator('p').first())
})
