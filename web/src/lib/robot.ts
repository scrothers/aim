// Procedural robot avatars. Deterministic: the same canonical address always
// yields the same robot, forever (no salt). Subagents inherit palette, body
// and head from their parent identity ("family resemblance"). Ported from the
// approved prototype; do not change the hash, PRNG, draw order or constants,
// or every robot silently re-rolls (robot.test.ts guards this).

// cyrb53: small, well-distributed 53-bit string hash.
function cyrb53(str: string, seed = 0): number {
  let h1 = 0xdeadbeef ^ seed
  let h2 = 0x41c6ce57 ^ seed
  for (let i = 0; i < str.length; i++) {
    const ch = str.charCodeAt(i)
    h1 = Math.imul(h1 ^ ch, 2654435761)
    h2 = Math.imul(h2 ^ ch, 1597334677)
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909)
  return 4294967296 * (2097151 & h2) + (h1 >>> 0)
}

// mulberry32 PRNG seeded from the hash, so each layer draws its own values.
function rng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const pick = (r: () => number, n: number) => Math.floor(r() * n)
const hsl = (h: number, s: number, l: number) => `hsl(${((h % 360) + 360) % 360} ${s}% ${l}%)`
const INK = '#1b1b22'

function split(address: string): { full: string; parent: string } {
  const parts = address.trim().toLowerCase().split('@')
  if (parts.length === 3) return { full: parts.join('@'), parent: `${parts[1]}@${parts[2]}` }
  return { full: parts.join('@'), parent: parts.join('@') }
}

export interface RobotTraits {
  hue: number
  headHue: number
  bgHue: number
  body: number
  head: number
  bg: number
  eyes: number
  mouth: number
  accessory: number
}

export function robotTraits(address: string): RobotTraits {
  const { full, parent } = split(address)
  const fam = rng(cyrb53(parent, 1)) // palette, body, head: shared by a family
  const own = rng(cyrb53(full, 2)) // face, accessory, background pattern: per address
  const base = Math.floor(fam() * 360)
  return {
    hue: base,
    headHue: base + [0, 24, -24, 40, -40][pick(fam, 5)]!,
    bgHue: base + 150 + Math.floor(fam() * 60),
    body: pick(fam, 4),
    head: pick(fam, 5),
    bg: pick(own, 4),
    eyes: pick(own, 4),
    mouth: pick(own, 4),
    accessory: pick(own, 4),
  }
}

/** One SVG shape. `clip` shapes are clipped to the background tile. */
export interface RobotPart {
  tag: 'rect' | 'circle' | 'path' | 'line'
  attrs: Record<string, string | number>
  clip?: true
}

export interface RobotShape {
  viewBox: string
  parts: RobotPart[]
}

const rect = (attrs: RobotPart['attrs'], clip?: true): RobotPart => (clip ? { tag: 'rect', attrs, clip } : { tag: 'rect', attrs })
const circle = (cx: number, cy: number, r: number, fill: string): RobotPart => ({ tag: 'circle', attrs: { cx, cy, r, fill } })
const path = (attrs: RobotPart['attrs']): RobotPart => ({ tag: 'path', attrs })
const line = (x1: number, y1: number, x2: number, y2: number, stroke: string): RobotPart => ({
  tag: 'line',
  attrs: { x1, y1, x2, y2, stroke, strokeWidth: 2 },
})

/** Compact (small-size) crop: the head fills the viewBox. */
const COMPACT_VIEWBOX = '12 9 40 40'

/**
 * Pure shape descriptors for an address. The full robot is a 64x64 tile; the
 * compact variant crops to the head and drops pattern, body, accessory and mouth.
 */
export function robotParts(address: string, opts: { compact?: boolean } = {}): RobotShape {
  const compact = opts.compact ?? false
  const t = robotTraits(address)
  const bodyC = hsl(t.hue, 52, 46)
  const bodyDark = hsl(t.hue, 52, 36)
  const headC = hsl(t.headHue, 58, 60)
  const headDark = hsl(t.headHue, 50, 46)
  const glow = hsl(t.headHue + 180, 95, 62)
  const bgC = hsl(t.bgHue, 45, 86)
  const bgDark = hsl(t.bgHue, 40, 76)
  const o: RobotPart[] = []

  // Background tile.
  if (compact) {
    o.push(rect({ x: 12, y: 9, width: 40, height: 40, rx: 9, fill: bgC }))
  } else {
    o.push(rect({ width: 64, height: 64, rx: 14, fill: bgC }))
    if (t.bg === 1) o.push(path({ d: 'M0 64 L64 0 L64 50 Q64 64 50 64 Z', fill: bgDark }))
    if (t.bg === 2) o.push({ tag: 'circle', attrs: { cx: 32, cy: 30, r: 25, fill: 'none', stroke: bgDark, strokeWidth: 4 } })
    if (t.bg === 3) for (let y = 8; y < 64; y += 10) for (let x = 8; x < 64; x += 10) o.push(circle(x, y, 1.4, bgDark))

    // Body (clipped by the tile at the bottom).
    if (t.body === 0) o.push(rect({ x: 15, y: 47, width: 34, height: 22, rx: 3, fill: bodyC }, true))
    if (t.body === 1) o.push(rect({ x: 14, y: 46, width: 36, height: 24, rx: 11, fill: bodyC }, true))
    if (t.body === 2) o.push({ tag: 'path', attrs: { d: 'M18 47 H46 Q52 56 46 70 H18 Q12 56 18 47 Z', fill: bodyC }, clip: true })
    if (t.body === 3) {
      o.push(rect({ x: 8, y: 48, width: 48, height: 22, rx: 5, fill: bodyC }, true))
      o.push({ tag: 'circle', attrs: { cx: 14, cy: 54, r: 1.8, fill: bodyDark }, clip: true })
      o.push({ tag: 'circle', attrs: { cx: 50, cy: 54, r: 1.8, fill: bodyDark }, clip: true })
    }
    o.push(rect({ x: 27, y: 51, width: 10, height: 5, rx: 1.5, fill: bodyDark }, true)) // chest panel
    o.push(rect({ x: 28, y: 42, width: 8, height: 6, fill: bodyDark })) // neck

    // Accessory behind/around the head.
    if (t.accessory === 0) {
      o.push(line(32, 16, 32, 7, headDark), circle(32, 6, 2.8, glow))
    }
    if (t.accessory === 1) {
      o.push(line(25, 16, 21, 7, headDark), line(39, 16, 43, 7, headDark), circle(21, 6.5, 2.2, glow), circle(43, 6.5, 2.2, glow))
    }
    if (t.accessory === 2) {
      o.push(rect({ x: 13, y: 25, width: 5, height: 10, rx: 2, fill: headDark }), rect({ x: 46, y: 25, width: 5, height: 10, rx: 2, fill: headDark }))
    }
  }

  // Head.
  if (t.head === 0) o.push(rect({ x: 17, y: 15, width: 30, height: 28, rx: 4, fill: headC }))
  if (t.head === 1) o.push(path({ d: 'M17 43 V30 A15 15 0 0 1 47 30 V43 Z', fill: headC }))
  if (t.head === 2) o.push(rect({ x: 16, y: 15, width: 32, height: 28, rx: 12, fill: headC }))
  if (t.head === 3) o.push(path({ d: 'M21 15 H43 L48 43 H16 Z', fill: headC }))
  if (t.head === 4) {
    o.push(rect({ x: 15, y: 14, width: 34, height: 29, rx: 5, fill: headC }))
    o.push(rect({ x: 19, y: 18, width: 26, height: 21, rx: 3, fill: headDark }))
  }
  const screen = t.head === 4

  // Eyes.
  const eyeY = 27
  if (t.eyes === 0) {
    o.push(circle(26, eyeY, 2.8, screen ? glow : INK), circle(38, eyeY, 2.8, screen ? glow : INK))
  }
  if (t.eyes === 1) {
    o.push(rect({ x: 21, y: eyeY - 3.5, width: 22, height: 7, rx: 3.5, fill: INK }))
    o.push(rect({ x: 24, y: eyeY - 0.8, width: 16, height: 1.6, rx: 0.8, fill: glow }))
  }
  if (t.eyes === 2) {
    o.push(circle(25.5, eyeY, 4.6, '#ffffff'), circle(38.5, eyeY, 4.6, '#ffffff'))
    o.push(circle(26.5, eyeY + 0.6, 2, INK), circle(39.5, eyeY + 0.6, 2, INK))
  }
  if (t.eyes === 3) {
    for (const x of [21, 29.5, 38]) o.push(rect({ x, y: eyeY - 2, width: 5, height: 4, rx: 1, fill: glow }))
  }

  // Mouth.
  if (!compact) {
    const mY = 35.5
    const m = screen ? glow : INK
    if (t.mouth === 0) {
      o.push(rect({ x: 25, y: mY - 2, width: 14, height: 4, rx: 1, fill: m }))
      o.push(path({ d: `M28.5 ${mY - 2} V${mY + 2} M32 ${mY - 2} V${mY + 2} M35.5 ${mY - 2} V${mY + 2}`, stroke: headC, strokeWidth: 1 }))
    }
    if (t.mouth === 1) o.push(rect({ x: 26, y: mY - 0.8, width: 12, height: 1.8, rx: 0.9, fill: m }))
    if (t.mouth === 2) o.push(circle(28, mY, 1.3, m), circle(32, mY, 1.3, m), circle(36, mY, 1.3, m))
  }

  return { viewBox: compact ? COMPACT_VIEWBOX : '0 0 64 64', parts: o }
}
