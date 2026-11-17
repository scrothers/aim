export const OPERATOR_ID = 'operator'

export const CARD_W = 220
export const CARD_H = 72
export const OPERATOR_W = 200
export const OPERATOR_H = 64
export const GAP_X = 24
export const GAP_Y = 56
export const PAD = 32

export interface ChartInput {
  address: string
  reports_to?: string
}

export interface PlacedNode {
  id: string
  x: number
  y: number
  w: number
  h: number
  depth: number
  /** True when reports_to was missing, unknown or cyclic and the node was attached to the operator. */
  orphan: boolean
}

export interface PlacedEdge {
  from: string
  to: string
  /** SVG path: down from the parent, across the bus, down into the child. */
  d: string
}

export interface ChartLayout {
  nodes: PlacedNode[]
  edges: PlacedEdge[]
  width: number
  height: number
}

/**
 * Computes a top-down tree layout. The operator is the root; identities hang
 * off their reports_to. Nodes whose parent is missing, unknown, self-referential
 * or part of a cycle attach to the operator instead, so every node is placed
 * exactly once. Output is deterministic (siblings sorted by address).
 */
export function layoutOrgChart(input: ChartInput[]): ChartLayout {
  const ids = new Set(input.map((n) => n.address).filter((a) => a !== OPERATOR_ID))
  const parentOf = new Map<string, string>()
  for (const n of input) {
    if (n.address === OPERATOR_ID) continue
    const p = n.reports_to
    parentOf.set(n.address, p && p !== n.address && (p === OPERATOR_ID || ids.has(p)) ? p : OPERATOR_ID)
  }
  const orphan = new Set<string>()
  for (const [a, p] of parentOf) if (p === OPERATOR_ID && input.find((n) => n.address === a)?.reports_to !== OPERATOR_ID) orphan.add(a)

  const children = new Map<string, string[]>()
  const addChild = (p: string, c: string) => children.set(p, [...(children.get(p) ?? []), c])
  for (const a of [...ids].sort()) addChild(parentOf.get(a)!, a)

  // Anything not reachable from the operator sits in a cycle: break it by attaching one member per cycle.
  const reach = new Set<string>([OPERATOR_ID])
  const walk = (id: string) => {
    for (const c of children.get(id) ?? []) {
      if (reach.has(c)) continue
      reach.add(c)
      walk(c)
    }
  }
  walk(OPERATOR_ID)
  for (const a of [...ids].sort()) {
    if (reach.has(a)) continue
    const old = parentOf.get(a)!
    children.set(old, (children.get(old) ?? []).filter((c) => c !== a))
    parentOf.set(a, OPERATOR_ID)
    orphan.add(a)
    addChild(OPERATOR_ID, a)
    reach.add(a)
    walk(a)
  }

  // Subtree widths (leaf = one card slot), then centers.
  const slot = CARD_W + GAP_X
  const widthOf = new Map<string, number>()
  const measure = (id: string): number => {
    const kids = children.get(id) ?? []
    const w = kids.length ? kids.reduce((sum, c) => sum + measure(c), 0) : slot
    widthOf.set(id, Math.max(w, slot))
    return widthOf.get(id)!
  }
  measure(OPERATOR_ID)

  const nodes: PlacedNode[] = []
  const centers = new Map<string, { cx: number; y: number; h: number }>()
  const place = (id: string, left: number, depth: number) => {
    const w = id === OPERATOR_ID ? OPERATOR_W : CARD_W
    const h = id === OPERATOR_ID ? OPERATOR_H : CARD_H
    const y = PAD + depth * (CARD_H + GAP_Y)
    const cx = left + widthOf.get(id)! / 2
    nodes.push({ id, x: cx - w / 2, y, w, h, depth, orphan: orphan.has(id) })
    centers.set(id, { cx, y, h })
    let l = left
    for (const c of children.get(id) ?? []) {
      place(c, l, depth + 1)
      l += widthOf.get(c)!
    }
  }
  place(OPERATOR_ID, PAD, 0)

  const edges: PlacedEdge[] = []
  for (const [p, kids] of children) {
    const pc = centers.get(p)!
    for (const c of kids) {
      const cc = centers.get(c)!
      const startY = pc.y + pc.h
      const midY = startY + GAP_Y / 2
      edges.push({ from: p, to: c, d: `M ${pc.cx} ${startY} V ${midY} H ${cc.cx} V ${cc.y}` })
    }
  }
  const maxDepth = Math.max(...nodes.map((n) => n.depth))
  return {
    nodes,
    edges,
    width: widthOf.get(OPERATOR_ID)! + PAD * 2,
    height: PAD * 2 + (maxDepth + 1) * CARD_H + maxDepth * GAP_Y,
  }
}
