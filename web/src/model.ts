// Pure view-model: what each level shows, card chips, package problem lines,
// riskiest lists. No React, no DOM; tested in model.test.ts.

import type { Metrics, QEdge, QNode, Snapshot, Thresholds } from './api'

export type Mode = 'combined' | 'crap' | 'coverage' | 'mutation'

export const MAX_FUNCS = 40

export interface Index {
  snap: Snapshot
  byId: Map<string, QNode>
  kids: Map<string, QNode[]>
  module: QNode | undefined
}

export function index(snap: Snapshot): Index {
  const byId = new Map<string, QNode>()
  const kids = new Map<string, QNode[]>()
  let module: QNode | undefined
  for (const n of snap.nodes) {
    byId.set(n.id, n)
    if (n.kind === 'module') module = n
    if (n.parent) {
      const list = kids.get(n.parent) ?? []
      list.push(n)
      kids.set(n.parent, list)
    }
  }
  return { snap, byId, kids, module }
}

export function metricsOf(ix: Index, id: string): Metrics {
  return ix.snap.metrics[id] ?? {}
}

/** short is the last path element: ".../internal/pricing.applyTiered" → "pricing.applyTiered". */
export function short(id: string): string {
  const i = id.lastIndexOf('/')
  return i >= 0 ? id.slice(i + 1) : id
}

/** gradeOf returns the grade that colors a box, or null when it does not apply. */
export function gradeOf(m: Metrics | undefined, mode: Mode): number | null {
  const g = m?.grades
  if (!g) return null
  if (mode === 'mutation') return g.mutation
  return g[mode]
}

export function isStale(m: Metrics | undefined): boolean {
  return Boolean(m?.coverage?.stale || m?.mutation?.stale)
}

export const fmt = {
  crap: (v: number | undefined): string => (v === undefined ? '—' : v.toFixed(1)),
  pct: (v: number | null | undefined): string => (v === null || v === undefined ? '—' : `${Math.round(v * 10) / 10}%`),
}

export function funcsOf(ix: Index, pkg: string): QNode[] {
  return (ix.kids.get(pkg) ?? []).filter((n) => n.kind === 'func')
}

export function typesOf(ix: Index, pkg: string): QNode[] {
  return (ix.kids.get(pkg) ?? []).filter((n) => n.kind === 'type')
}

export function packages(ix: Index): QNode[] {
  return ix.snap.nodes.filter((n) => n.kind === 'package')
}

/** risk orders by CRAP (highest first; missing counts as worst), then name. */
export function byRisk(ix: Index): (a: QNode, b: QNode) => number {
  const crap = (n: QNode) => metricsOf(ix, n.id).crap ?? Number.POSITIVE_INFINITY
  return (a, b) => crap(b) - crap(a) || a.name.localeCompare(b.name)
}

export type Tone = { grade: number } | 'red' | 'yellow' | 'orange'

export interface Chip { text: string; tone: Tone }

function untested(m: Metrics): boolean {
  return m.coverage?.percent === 0 && m.coverage.stmts > 0
}

/** functionChips: up to 3 problem chips (docs/UI.md §3). */
export function functionChips(m: Metrics, th: Thresholds): Chip[] {
  const chips: Chip[] = []
  if (untested(m)) chips.push({ text: 'untested', tone: 'red' })
  if (m.crap !== undefined && m.crap > th.crap_max) chips.push({ text: `CRAP ${m.crap.toFixed(1)}`, tone: { grade: m.grades?.crap ?? 1 } })
  const survived = m.mutation?.survived ?? 0
  if (survived > 0) chips.push({ text: `${survived} survived`, tone: survived > 2 ? 'red' : 'yellow' })
  return chips.slice(0, 3)
}

export interface Problem { text: string; tone: 'red' | 'orange' | 'yellow' }

/** packageProblems: up to 2 lines, only when something is wrong. */
export function packageProblems(ix: Index, pkg: string, th: Thresholds): Problem[] {
  const m = metricsOf(ix, pkg)
  const out: Problem[] = []
  const bare = funcsOf(ix, pkg).filter((f) => untested(metricsOf(ix, f.id))).sort(byRisk(ix))[0]
  if (bare) out.push({ text: `${bare.name} has no tests`, tone: 'red' })
  const mu = m.mutation
  if (mu && mu.score !== null && mu.score < th.mutation_min && mu.survived > 0) out.push({ text: `${mu.survived} mutants survived`, tone: 'orange' })
  if (m.crap_max !== undefined && m.crap_max > th.crap_max && m.worst && (!bare || bare.id !== m.worst)) {
    out.push({ text: `${ix.byId.get(m.worst)?.name ?? short(m.worst)} · CRAP ${m.crap_max.toFixed(1)}`, tone: 'orange' })
  }
  if (!mu && funcsOf(ix, pkg).length > 0) out.push({ text: 'mutation not measured yet', tone: 'red' })
  if (m.mutation_error) out.unshift({ text: 'tests fail: mutation not run', tone: 'red' })
  if (m.coverage_error) out.unshift({ text: 'tests failed: coverage not measured', tone: 'red' })
  return out.slice(0, 2)
}

export interface Riskiest { id: string; name: string; reason: string; crap: number | undefined; grade: number }

/** riskiest lists the top n functions of a package with a one-line reason. */
export function riskiest(ix: Index, pkg: string, n: number): Riskiest[] {
  return funcsOf(ix, pkg).sort(byRisk(ix)).slice(0, n).map((f) => {
    const m = metricsOf(ix, f.id)
    let reason = `CC ${m.cc ?? '—'}`
    if (m.crap === undefined) reason = 'coverage not measured'
    else if (untested(m)) reason = 'no tests run it'
    else if ((m.mutation?.survived ?? 0) > 0) reason = `${m.mutation?.survived} mutants survived`
    return { id: f.id, name: f.name, reason, crap: m.crap, grade: m.grades?.crap ?? 1 }
  })
}

/** moduleView: packages, the external modules they import, and import
 * arrows (one per pair). */
export function moduleView(ix: Index): { packages: string[]; externals: string[]; edges: [string, string][] } {
  const pkgs = packages(ix).map((p) => p.id)
  const inView = new Set(pkgs)
  const externals = new Set<string>()
  const edges = new Map<string, [string, string]>()
  for (const e of ix.snap.edges) {
    if (e.kind !== 'imports' || !inView.has(e.from)) continue
    const to = ix.byId.get(e.to)
    if (!to) continue
    if (to.kind === 'external') externals.add(e.to)
    else if (!inView.has(e.to)) continue
    edges.set(`${e.from}→${e.to}`, [e.from, e.to])
  }
  return { packages: pkgs, externals: [...externals].sort(), edges: [...edges.values()] }
}

export interface PackageView {
  funcs: string[]
  more: number
  types: string[]
  /** functions of other module packages called from this one */
  outside: string[]
  edges: [string, string][]
}

/** packageView: the package's functions (40 riskiest unless expanded), its
 * types, called functions of other packages, and call arrows. */
export function packageView(ix: Index, pkg: string, expanded: boolean): PackageView {
  const all = funcsOf(ix, pkg)
  const shown = expanded || all.length <= MAX_FUNCS ? all : [...all].sort(byRisk(ix)).slice(0, MAX_FUNCS)
  const inBox = new Set(shown.map((f) => f.id))
  const outside = new Set<string>()
  const edges = new Map<string, [string, string]>()
  for (const e of callEdges(ix.snap.edges)) {
    if (!inBox.has(e.from)) continue
    const to = ix.byId.get(e.to)
    if (!to || to.kind !== 'func') continue
    if (to.parent !== pkg) outside.add(e.to)
    else if (!inBox.has(e.to)) continue
    edges.set(`${e.from}→${e.to}`, [e.from, e.to])
  }
  return {
    funcs: shown.map((f) => f.id),
    more: all.length - shown.length,
    types: typesOf(ix, pkg).map((t) => t.id),
    outside: [...outside].sort(),
    edges: [...edges.values()],
  }
}

function callEdges(edges: QEdge[]): QEdge[] {
  return edges.filter((e) => e.kind === 'calls' || e.kind === 'calls_dynamic')
}

/** functionView: callers and callees that are functions in the module. */
export function functionView(ix: Index, fn: string): { callers: string[]; callees: string[] } {
  const callers = new Set<string>()
  const callees = new Set<string>()
  for (const e of callEdges(ix.snap.edges)) {
    if (e.to === fn && ix.byId.get(e.from)?.kind === 'func') callers.add(e.from)
    if (e.from === fn && ix.byId.get(e.to)?.kind === 'func') callees.add(e.to)
  }
  return { callers: [...callers].sort(), callees: [...callees].sort() }
}

/** packageOf returns the package containing id (a package is its own). */
export function packageOf(ix: Index, id: string): string | undefined {
  const n = ix.byId.get(id)
  if (!n) return undefined
  if (n.kind === 'package') return n.id
  return n.parent && ix.byId.get(n.parent)?.kind === 'package' ? n.parent : undefined
}

/** worstPackage is the package holding the module's riskiest function. */
export function worstPackage(ix: Index): string | undefined {
  const worst = ix.module ? metricsOf(ix, ix.module.id).worst : undefined
  return (worst && packageOf(ix, worst)) ?? packages(ix)[0]?.id
}

export interface SearchItem { id: string; label: string; kind: 'package' | 'func' }

export function searchItems(ix: Index): SearchItem[] {
  return ix.snap.nodes
    .filter((n) => n.kind === 'package' || n.kind === 'func')
    .map((n) => ({ id: n.id, label: n.kind === 'package' ? n.name : short(n.id), kind: n.kind as 'package' | 'func' }))
}

/** fuzzy ranks items whose label contains the query's letters in order;
 * tighter and earlier matches first. */
export function fuzzy(query: string, items: SearchItem[], limit = 8): SearchItem[] {
  const q = query.toLowerCase().replace(/\s+/g, '')
  if (!q) return []
  const scored: { item: SearchItem; score: number }[] = []
  for (const item of items) {
    const s = matchScore(q, item.label.toLowerCase())
    if (s !== null) scored.push({ item, score: s })
  }
  return scored.sort((a, b) => a.score - b.score || a.item.label.length - b.item.label.length).slice(0, limit).map((x) => x.item)
}

function matchScore(q: string, label: string): number | null {
  const direct = label.indexOf(q)
  if (direct >= 0) return direct
  let pos = -1
  let gaps = 0
  for (const ch of q) {
    const next = label.indexOf(ch, pos + 1)
    if (next < 0) return null
    if (pos >= 0) gaps += next - pos - 1
    pos = next
  }
  return 100 + gaps
}
