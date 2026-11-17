import { describe, expect, it } from 'vitest'
import { CARD_H, CARD_W, GAP_Y, OPERATOR_ID, layoutOrgChart } from './orgchart'

const node = (layout: ReturnType<typeof layoutOrgChart>, id: string) => layout.nodes.find((n) => n.id === id)!

describe('layoutOrgChart', () => {
  it('places the operator alone when the roster is empty', () => {
    const l = layoutOrgChart([])
    expect(l.nodes.map((n) => n.id)).toEqual([OPERATOR_ID])
    expect(l.edges).toEqual([])
  })

  it('lays out depth rows, centers parents over children and emits one edge per node', () => {
    const l = layoutOrgChart([
      { address: 'lead', reports_to: 'operator' },
      { address: 'a', reports_to: 'lead' },
      { address: 'b', reports_to: 'lead' },
    ])
    const lead = node(l, 'lead')
    const a = node(l, 'a')
    const b = node(l, 'b')
    expect(lead.depth).toBe(1)
    expect(a.depth).toBe(2)
    expect(a.y).toBe(lead.y + CARD_H + GAP_Y)
    expect(b.y).toBe(a.y)
    expect(b.x).toBeGreaterThan(a.x + CARD_W - 1) // siblings never overlap
    expect(lead.x + lead.w / 2).toBeCloseTo((a.x + b.x + CARD_W) / 2)
    expect(l.edges.map((e) => `${e.from}>${e.to}`).sort()).toEqual(['lead>a', 'lead>b', 'operator>lead'])
    expect(l.width).toBeGreaterThanOrEqual(2 * CARD_W)
    expect(l.height).toBeGreaterThan(a.y + CARD_H)
    expect(lead.orphan).toBe(false)
  })

  it('attaches missing, unknown and self parents to the operator', () => {
    const l = layoutOrgChart([{ address: 'none' }, { address: 'ghost', reports_to: 'nobody' }, { address: 'me', reports_to: 'me' }])
    for (const id of ['none', 'ghost', 'me']) {
      expect(node(l, id).depth).toBe(1)
      expect(node(l, id).orphan).toBe(true)
      expect(l.edges.some((e) => e.from === OPERATOR_ID && e.to === id)).toBe(true)
    }
  })

  it('is cycle-safe: every node is placed once', () => {
    const l = layoutOrgChart([
      { address: 'a', reports_to: 'b' },
      { address: 'b', reports_to: 'c' },
      { address: 'c', reports_to: 'a' },
      { address: 'd', reports_to: 'c' },
    ])
    expect(l.nodes).toHaveLength(5)
    expect(new Set(l.nodes.map((n) => n.id)).size).toBe(5)
    expect(l.edges).toHaveLength(4)
    expect(node(l, 'a').orphan).toBe(true)
  })

  it('is deterministic regardless of input order', () => {
    const input = [
      { address: 'x', reports_to: 'operator' },
      { address: 'y', reports_to: 'x' },
      { address: 'z', reports_to: 'operator' },
    ]
    expect(layoutOrgChart([...input].reverse())).toEqual(layoutOrgChart(input))
  })
})
