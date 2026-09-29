// Flow layout for the maps (docs/UI.md §4): entries on top, every arrow
// pointing down, wide rows wrapped. These pure functions choose the rows and
// the order; ELK then places the cards and routes the arrows (layout.ts).

export type Arrow = [string, string]

/** acyclic drops self-loops and the edges that close a cycle (depth-first
 * from the nodes in order), so rows can be ranked. */
export function acyclic(nodes: string[], edges: Arrow[]): Arrow[] {
  const out = outgoing(edges)
  const state = new Map<string, 'open' | 'done'>()
  const back = new Set<string>()
  const visit = (n: string) => {
    state.set(n, 'open')
    for (const m of out.get(n) ?? []) {
      if (state.get(m) === 'open') back.add(`${n}→${m}`)
      else if (!state.has(m)) visit(m)
    }
    state.set(n, 'done')
  }
  for (const n of nodes) if (!state.has(n)) visit(n)
  return unique(edges).filter(([a, b]) => a !== b && !back.has(`${a}→${b}`))
}

/** backbone hides an arrow a→b when b is still reachable from a without it:
 * a longer path already shows the dependency (transitive reduction). Arrows
 * inside a cycle are kept. */
export function backbone(nodes: string[], edges: Arrow[]): Arrow[] {
  const all = unique(edges)
  const dag = new Set(acyclic(nodes, all).map(key))
  const out = outgoing([...all].filter((e) => dag.has(key(e))))
  return all.filter((e) => !dag.has(key(e)) || !reachableWithout(out, e))
}

function reachableWithout(out: Map<string, string[]>, [from, to]: Arrow): boolean {
  const seen = new Set<string>()
  const stack = (out.get(from) ?? []).filter((m) => m !== to)
  while (stack.length > 0) {
    const n = stack.pop()!
    if (n === to) return true
    if (seen.has(n)) continue
    seen.add(n)
    stack.push(...(out.get(n) ?? []))
  }
  return false
}

export interface Rows {
  /** row index per node, 0 at the top */
  row: Map<string, number>
  /** nodes top to bottom, left to right */
  order: string[]
}

/** flowRows ranks nodes by the longest path from the entries (nodes nothing
 * points to), so each sits below everything that points to it; nodes with no
 * arrows go last. A rank is ordered by where its parents sit and wrapped
 * into balanced rows of at most max nodes. */
export function flowRows(nodes: string[], edges: Arrow[], max: number): Rows {
  const dag = acyclic(nodes, edges)
  const ranks = rankGroups(nodes, dag)
  const parents = incoming(dag)
  const x = new Map<string, number>()
  const row = new Map<string, number>()
  const order: string[] = []
  for (const group of ranks) {
    for (const line of wrap(byParents(group, parents, x, nodes), max)) {
      line.forEach((n, i) => x.set(n, i - (line.length - 1) / 2))
      for (const n of line) row.set(n, order.length === 0 ? 0 : nextRow(row, order))
      order.push(...line)
    }
  }
  return { row, order }
}

function nextRow(row: Map<string, number>, order: string[]): number {
  return row.get(order[order.length - 1]!)! + 1
}

/** rankGroups: longest-path ranks over a DAG; isolated nodes form the last
 * group. Groups keep the input order. */
function rankGroups(nodes: string[], dag: Arrow[]): string[][] {
  const out = outgoing(dag)
  const indeg = new Map(nodes.map((n) => [n, 0]))
  for (const [, b] of dag) indeg.set(b, (indeg.get(b) ?? 0) + 1)
  const touched = new Set(dag.flat())
  const rank = new Map<string, number>()
  const queue = nodes.filter((n) => indeg.get(n) === 0 && touched.has(n))
  for (const n of queue) rank.set(n, 0)
  for (let i = 0; i < queue.length; i++) {
    const n = queue[i]!
    for (const m of out.get(n) ?? []) {
      rank.set(m, Math.max(rank.get(m) ?? 0, rank.get(n)! + 1))
      indeg.set(m, indeg.get(m)! - 1)
      if (indeg.get(m) === 0) queue.push(m)
    }
  }
  const groups: string[][] = []
  for (const n of nodes) if (rank.has(n)) (groups[rank.get(n)!] ??= []).push(n)
  const lonely = nodes.filter((n) => !touched.has(n))
  return [...groups.filter(Boolean), ...(lonely.length > 0 ? [lonely] : [])]
}

/** byParents sorts a rank by the mean position of its parents (input order
 * breaks ties, and orders nodes without placed parents). */
function byParents(group: string[], parents: Map<string, string[]>, x: Map<string, number>, nodes: string[]): string[] {
  const pos = new Map(nodes.map((n, i) => [n, i]))
  const center = (n: string): number => {
    const xs = (parents.get(n) ?? []).filter((p) => x.has(p)).map((p) => x.get(p)!)
    return xs.length === 0 ? 0 : xs.reduce((a, b) => a + b, 0) / xs.length
  }
  return [...group].sort((a, b) => center(a) - center(b) || pos.get(a)! - pos.get(b)!)
}

/** wrap splits a sorted rank into balanced rows, dealing nodes out in turn
 * so each row spans the whole width. */
function wrap(sorted: string[], max: number): string[][] {
  const count = Math.max(1, Math.ceil(sorted.length / max))
  const lines: string[][] = Array.from({ length: count }, () => [])
  sorted.forEach((n, i) => lines[i % count]!.push(n))
  return lines
}

/** primaryEntry is the top-row node that reaches the most nodes: where a
 * reader starts (cmd/app rather than a small cmd/tool). Ties keep row order. */
export function primaryEntry(rows: Rows, edges: Arrow[]): string | undefined {
  const out = outgoing(edges)
  let best: string | undefined
  let most = -1
  for (const n of rows.order.filter((id) => rows.row.get(id) === 0)) {
    const seen = new Set<string>()
    const stack = [...(out.get(n) ?? [])]
    while (stack.length > 0) {
      const m = stack.pop()!
      if (seen.has(m)) continue
      seen.add(m)
      stack.push(...(out.get(m) ?? []))
    }
    if (seen.size > most) [best, most] = [n, seen.size]
  }
  return best
}

/** related lists every node with a real arrow to or from id. */
export function related(id: string, edges: Arrow[]): Set<string> {
  const out = new Set<string>()
  for (const [a, b] of edges) {
    if (a === id) out.add(b)
    if (b === id) out.add(a)
  }
  return out
}

function outgoing(edges: Arrow[]): Map<string, string[]> {
  const out = new Map<string, string[]>()
  for (const [a, b] of edges) out.set(a, [...(out.get(a) ?? []), b])
  return out
}

function incoming(edges: Arrow[]): Map<string, string[]> {
  const out = new Map<string, string[]>()
  for (const [a, b] of edges) out.set(b, [...(out.get(b) ?? []), a])
  return out
}

function unique(edges: Arrow[]): Arrow[] {
  const seen = new Set<string>()
  return edges.filter((e) => !seen.has(key(e)) && seen.add(key(e)) !== undefined)
}

const key = ([a, b]: Arrow): string => `${a}→${b}`
