// Automatic layout with ELK "layered", direction DOWN (importers above
// importees, callers above callees). Nodes are measured by React Flow first,
// then laid out here (docs/decisions.md #8).

import type { ELK as ElkInstance, ElkNode } from 'elkjs/lib/elk.bundled.js'

export interface Sized { id: string; width: number; height: number; last?: boolean }
export interface Point { x: number; y: number }

// ELK is large; load it on first layout so the first paint does not wait.
let elk: Promise<ElkInstance> | null = null
function getElk(): Promise<ElkInstance> {
  elk ??= import('elkjs/lib/elk.bundled.js').then((m) => new m.default())
  return elk
}

export const layeredOptions: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'DOWN',
  'elk.spacing.nodeNode': '48',
  'elk.layered.spacing.nodeNodeBetweenLayers': '64',
  'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
  'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
  'elk.edgeRouting': 'ORTHOGONAL',
  'elk.spacing.edgeNode': '16',
  'elk.spacing.edgeEdge': '10',
  'elk.layered.spacing.edgeNodeBetweenLayers': '20',
  'elk.padding': '[top=0,left=0,bottom=0,right=0]',
}

/** toElk builds the ELK input; duplicate edges and self-loops are dropped. */
export function toElk(nodes: Sized[], edges: [string, string][]): ElkNode {
  const ids = new Set(nodes.map((n) => n.id))
  const seen = new Set<string>()
  const elkEdges = []
  for (const [from, to] of edges) {
    const key = `${from}→${to}`
    if (from === to || seen.has(key) || !ids.has(from) || !ids.has(to)) continue
    seen.add(key)
    elkEdges.push({ id: `e${elkEdges.length}`, sources: [from], targets: [to] })
  }
  return {
    id: 'root',
    layoutOptions: layeredOptions,
    children: nodes.map((n) => ({
      id: n.id, width: n.width, height: n.height,
      ...(n.last ? { layoutOptions: { 'elk.layered.layering.layerConstraint': 'LAST' } } : {}),
    })),
    edges: elkEdges,
  }
}

export function positions(result: ElkNode): Map<string, Point> {
  const out = new Map<string, Point>()
  for (const c of result.children ?? []) out.set(c.id, { x: c.x ?? 0, y: c.y ?? 0 })
  return out
}

/** routes maps "from→to" to the edge's routed points (start, bends, end). */
export function routes(result: ElkNode): Map<string, Point[]> {
  const out = new Map<string, Point[]>()
  for (const e of result.edges ?? []) {
    const s = e.sections?.[0]
    if (!s) continue
    out.set(`${e.sources[0]}→${e.targets[0]}`, [s.startPoint, ...(s.bendPoints ?? []), s.endPoint])
  }
  return out
}

export interface Layout { at: Map<string, Point>; routes: Map<string, Point[]> }

export async function layered(nodes: Sized[], edges: [string, string][]): Promise<Layout> {
  const result = await (await getElk()).layout(toElk(nodes, edges))
  return { at: positions(result), routes: routes(result) }
}

/** roundedPath draws an orthogonal route with rounded corners. */
export function roundedPath(points: Point[], radius = 8): string {
  if (points.length === 0) return ''
  const [first, ...rest] = points
  let d = `M${first!.x},${first!.y}`
  for (let i = 0; i < rest.length; i++) {
    const cur = rest[i]!
    const next = rest[i + 1]
    const prev = i === 0 ? first! : rest[i - 1]!
    if (!next) {
      d += ` L${cur.x},${cur.y}`
      break
    }
    const r = Math.min(radius, dist(prev, cur) / 2, dist(cur, next) / 2)
    const a = toward(cur, prev, r)
    const b = toward(cur, next, r)
    d += ` L${a.x},${a.y} Q${cur.x},${cur.y} ${b.x},${b.y}`
  }
  return d
}

function dist(a: Point, b: Point): number {
  return Math.hypot(a.x - b.x, a.y - b.y)
}

function toward(from: Point, to: Point, r: number): Point {
  const d = dist(from, to) || 1
  return { x: from.x + ((to.x - from.x) * r) / d, y: from.y + ((to.y - from.y) * r) / d }
}

export function shift(points: Point[], dx: number, dy: number): Point[] {
  return points.map((p) => ({ x: p.x + dx, y: p.y + dy }))
}

/** bounds of positioned nodes. */
export function bounds(nodes: Sized[], at: Map<string, Point>): { x: number; y: number; width: number; height: number } {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const n of nodes) {
    const p = at.get(n.id)
    if (!p) continue
    minX = Math.min(minX, p.x)
    minY = Math.min(minY, p.y)
    maxX = Math.max(maxX, p.x + n.width)
    maxY = Math.max(maxY, p.y + n.height)
  }
  if (minX === Infinity) return { x: 0, y: 0, width: 0, height: 0 }
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY }
}

export type Direction = 'up' | 'down' | 'left' | 'right'

/** nearest finds the box closest to from in a direction (for arrow keys):
 * candidates must lie in that half-plane; distance along the axis counts
 * once, sideways drift counts double. */
export function nearest(rects: Map<string, { x: number; y: number; w: number; h: number }>, from: string, dir: Direction): string | undefined {
  const a = rects.get(from)
  if (!a) return rects.keys().next().value
  const ax = a.x + a.w / 2
  const ay = a.y + a.h / 2
  let best: string | undefined
  let bestScore = Infinity
  for (const [id, r] of rects) {
    if (id === from) continue
    const dx = r.x + r.w / 2 - ax
    const dy = r.y + r.h / 2 - ay
    const along = dir === 'up' ? -dy : dir === 'down' ? dy : dir === 'left' ? -dx : dx
    const side = dir === 'up' || dir === 'down' ? Math.abs(dx) : Math.abs(dy)
    if (along <= 1) continue
    const score = along + 2 * side
    if (score < bestScore) {
      bestScore = score
      best = id
    }
  }
  return best
}
