import { describe, expect, it } from 'vitest'
import { robotParts, robotTraits } from './robot'

const ADDRS = ['test@github.com/testing', 'coordinator@github.com/acme', 'researcher@coordinator@github.com/acme']

// Computed by running the approved prototype (robot.mjs). If these change,
// every existing robot has silently re-rolled.
const PROTOTYPE_TRAITS = [
  { hue: 171, headHue: 211, bgHue: 323, body: 2, head: 2, bg: 1, eyes: 2, mouth: 2, accessory: 3 },
  { hue: 171, headHue: 147, bgHue: 323, body: 0, head: 4, bg: 3, eyes: 2, mouth: 0, accessory: 2 },
  { hue: 171, headHue: 147, bgHue: 323, body: 0, head: 4, bg: 2, eyes: 2, mouth: 2, accessory: 3 },
]

describe('robotTraits', () => {
  it('matches the approved prototype', () => {
    expect(ADDRS.map(robotTraits)).toEqual(PROTOTYPE_TRAITS)
  })

  it('is deterministic and case/whitespace-insensitive', () => {
    const a = 'Researcher@Coordinator@GitHub.com/Acme'
    expect(robotTraits(a)).toEqual(robotTraits(`  ${a.toLowerCase()} `))
    expect(robotParts(a)).toEqual(robotParts(a.toLowerCase()))
    expect(robotParts(a, { compact: true })).toEqual(robotParts(a.toLowerCase(), { compact: true }))
  })

  it('gives subagents their parent identity palette, body and head', () => {
    const parent = robotTraits('coordinator@github.com/acme')
    const sub = robotTraits('researcher@coordinator@github.com/acme')
    expect(sub).toMatchObject({ hue: parent.hue, headHue: parent.headHue, bgHue: parent.bgHue, body: parent.body, head: parent.head })
    const other = robotTraits('writer@coordinator@github.com/acme')
    expect(other).toMatchObject({ hue: parent.hue, body: parent.body, head: parent.head })
  })
})

describe('robotParts', () => {
  it('golden snapshot guards against silent re-rolls', () => {
    const golden = Object.fromEntries(
      ADDRS.map((a) => [a, { traits: robotTraits(a), full: robotParts(a), compact: robotParts(a, { compact: true }) }]),
    )
    expect(golden).toMatchSnapshot()
  })

  it('compact variant crops to the head and omits accessory, mouth, pattern and body', () => {
    // accessory 2 (ear pods) and mouth 0 (grille) for coordinator.
    const addr = 'coordinator@github.com/acme'
    const full = robotParts(addr)
    const compact = robotParts(addr, { compact: true })
    expect(full.viewBox).toBe('0 0 64 64')
    expect(compact.viewBox).toBe('12 9 40 40')
    expect(compact.parts.length).toBeLessThan(full.parts.length)
    expect(compact.parts.some((p) => p.clip)).toBe(false)
    // tile + head (2 rects for screen head) + eyes (2 circles)
    expect(compact.parts.map((p) => p.tag)).toEqual(['rect', 'rect', 'rect', 'circle', 'circle', 'circle', 'circle'])
    expect(JSON.stringify(compact.parts)).not.toContain('"strokeWidth"')
  })
})
