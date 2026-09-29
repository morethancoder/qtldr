import { describe, expect, it } from 'vitest'
import type { Snapshot, Thresholds } from './api'
import {
  fuzzy, functionChips, functionView, gradeOf, index, moduleView, packageOf, packageProblems,
  greenShare, packageView, riskiest, searchItems, short, typeView, worstPackage,
} from './model'
import { formatRoute, parseRoute } from './route'
import { bounds, layered, toElk } from './layout'

const th: Thresholds = { crap_max: 8, cognitive_max: 15, coverage_min: 80, mutation_min: 70 }
const M = 'github.com/acme/ledger'
const P = `${M}/internal/pricing`
const MONEY = `${M}/internal/money`

// A trimmed snapshot shaped like the fixture's (hand-written UI input).
const snap: Snapshot = {
  schema: 1,
  module: M,
  generated: '2026-09-28T10:00:00Z',
  runs: { structure: null, coverage: null, mutation: null },
  nodes: [
    { id: M, kind: 'module', name: 'ledger' },
    { id: P, kind: 'package', name: 'internal/pricing', parent: M, pure: true },
    { id: MONEY, kind: 'package', name: 'internal/money', parent: M, pure: true },
    { id: `${P}.BestRule`, kind: 'func', name: 'BestRule', parent: P, exported: true },
    { id: `${P}.applyTiered`, kind: 'func', name: 'applyTiered', parent: P, exported: false },
    { id: `${P}.price`, kind: 'func', name: 'price', parent: P },
    { id: `${P}.Rule`, kind: 'type', name: 'Rule', parent: P, type_kind: 'struct' },
    { id: `${P}.Rule.Price`, kind: 'func', name: 'Rule.Price', parent: P, recv: 'Rule' },
    { id: `${P}.Pricer`, kind: 'type', name: 'Pricer', parent: P, type_kind: 'interface' },
    { id: `${MONEY}.Mul`, kind: 'func', name: 'Mul', parent: MONEY },
    { id: 'github.com/go-chi/chi/v5', kind: 'external', name: 'go-chi/chi' },
  ],
  edges: [
    { from: P, to: MONEY, kind: 'imports' },
    { from: P, to: MONEY, kind: 'imports' },
    { from: P, to: 'github.com/go-chi/chi/v5', kind: 'imports' },
    { from: `${P}.price`, to: `${P}.applyTiered`, kind: 'calls' },
    { from: `${P}.applyTiered`, to: `${MONEY}.Mul`, kind: 'calls' },
    { from: `${P}.price`, to: `${P}.Iface.M`, kind: 'calls_dynamic' },
    { from: `${P}.Rule`, to: `${P}.Pricer`, kind: 'implements' },
  ],
  metrics: {
    [M]: { worst: `${P}.BestRule` },
    [P]: { crap_max: 30, worst: `${P}.BestRule`, grades: { crap: 2, mutation: 1, coverage: 1, combined: 2 } },
    [`${P}.BestRule`]: { cc: 5, crap: 30, coverage: { stmts: 6, covered: 0, percent: 0, stale: false }, grades: { crap: 2, mutation: 1, coverage: 1, combined: 2 } },
    [`${P}.applyTiered`]: {
      cc: 11, crap: 14.08, coverage: { stmts: 17, covered: 12, percent: 70.6, stale: true },
      mutation: { killed: 9, survived: 7, not_covered: 3, timed_out: 0, score: 56.3, stale: false },
      grades: { crap: 4, mutation: 4, coverage: 6, combined: 4 },
    },
    [`${P}.price`]: { cc: 2, crap: 2, grades: { crap: 10, mutation: null, coverage: 10, combined: 10 } },
    [`${P}.Rule.Price`]: { cc: 1, crap: 1, grades: { crap: 10, mutation: null, coverage: 10, combined: 10 } },
  },
}
const ix = index(snap)

describe('view model', () => {
  it('names and grades', () => {
    expect(short(`${P}.applyTiered`)).toBe('pricing.applyTiered')
    expect(gradeOf(snap.metrics[`${P}.price`], 'mutation')).toBeNull()
    expect(gradeOf(snap.metrics[`${P}.applyTiered`], 'combined')).toBe(4)
    expect(gradeOf(undefined, 'crap')).toBeNull()
  })

  it('function chips', () => {
    expect(functionChips(snap.metrics[`${P}.BestRule`]!, th).map((c) => c.text)).toEqual(['untested', 'CRAP 30.0'])
    expect(functionChips(snap.metrics[`${P}.applyTiered`]!, th)).toEqual([
      { text: 'CRAP 14.1', tone: { grade: 4 } },
      { text: '7 survived', tone: 'red' },
    ])
    expect(functionChips(snap.metrics[`${P}.price`]!, th)).toEqual([])
  })

  it('package problem lines', () => {
    expect(packageProblems(ix, P, th).map((p) => p.text)).toEqual(['BestRule has no tests', 'mutation not measured yet'])
  })

  it('riskiest functions', () => {
    expect(riskiest(ix, P, 3).map((r) => [r.name, r.reason])).toEqual([
      ['BestRule', 'no tests run it'], ['applyTiered', '7 mutants survived'], ['price', 'CC 2'],
    ])
  })

  it('module level collapses duplicate imports and lists externals', () => {
    const v = moduleView(ix)
    expect(v.packages).toEqual([P, MONEY])
    expect(v.externals).toEqual(['github.com/go-chi/chi/v5'])
    expect(v.edges).toEqual([[P, MONEY], [P, 'github.com/go-chi/chi/v5']])
    expect(v.groups).toEqual([])
  })

  it('package level: calls inside, other packages grouped, no interface targets', () => {
    const v = packageView(ix, P, false)
    expect(v.funcs).toHaveLength(4)
    expect(v.outside).toEqual([{ pkg: MONEY, funcs: [`${MONEY}.Mul`] }])
    expect(v.types).toEqual([`${P}.Rule`, `${P}.Pricer`])
    expect(v.edges).toEqual([[`${P}.price`, `${P}.applyTiered`]])
    expect(v.calls).toEqual([[`${P}.applyTiered`, MONEY]])
  })

  it('green share: functions with a combined grade of 9 or 10', () => {
    const all = ix.snap.nodes.filter((n) => n.kind === 'func')
    const green = all.filter((f) => (ix.snap.metrics[f.id]?.grades?.combined ?? 0) >= 9)
    expect(greenShare(ix)).toEqual({ green: green.length, total: all.length })
    expect(greenShare(ix, P).total).toBe(4)
    expect(greenShare(ix, 'nope')).toEqual({ green: 0, total: 0 })
  })

  it('function level: callers and callees', () => {
    expect(functionView(ix, `${P}.applyTiered`)).toEqual({ callers: [`${P}.price`], callees: [`${MONEY}.Mul`] })
  })

  it('type view', () => {
    expect(typeView(ix, `${P}.Rule`)).toEqual({ methods: [`${P}.Rule.Price`], implements: [`${P}.Pricer`], implementedBy: [] })
    expect(typeView(ix, `${P}.Pricer`).implementedBy).toEqual([`${P}.Rule`])
  })

  it('navigation helpers', () => {
    expect(packageOf(ix, `${P}.Rule`)).toBe(P)
    expect(packageOf(ix, P)).toBe(P)
    expect(worstPackage(ix)).toBe(P)
  })

  it('fuzzy search', () => {
    const items = searchItems(ix)
    expect(fuzzy('tiered', items).map((i) => i.label)).toEqual(['pricing.applyTiered'])
    expect(fuzzy('prc', items).map((i) => i.label)).toContain('internal/pricing')
    expect(fuzzy('', items)).toEqual([])
    expect(fuzzy('zzz', items)).toEqual([])
  })
})

describe('route', () => {
  it('round-trips', () => {
    const routes = [
      { level: 0 as const },
      { level: 0 as const, sel: P },
      { level: 1 as const, pkg: P, sel: `${P}.applyTiered` },
      { level: 2 as const, fn: `${P}.applyTiered`, line: 29 },
    ]
    for (const r of routes) expect(parseRoute(formatRoute(r))).toEqual({ sel: undefined, ...r, ...(r.level === 2 ? { line: r.line } : {}) })
    expect(parseRoute('')).toEqual({ level: 0, sel: undefined })
    expect(parseRoute('#/func/x?line=abc')).toEqual({ level: 2, fn: 'x', sel: undefined, line: undefined })
  })
})

describe('layout', () => {
  it('builds an ELK graph without duplicate edges or self-loops', () => {
    const g = toElk([{ id: 'a', width: 10, height: 10 }, { id: 'b', width: 10, height: 10 }], [['a', 'b'], ['a', 'b'], ['a', 'a'], ['a', 'z']])
    expect(g.edges).toHaveLength(1)
    expect(g.layoutOptions?.['elk.direction']).toBe('DOWN')
  })

  it('places importers above importees', async () => {
    const nodes = [{ id: 'top', width: 236, height: 44 }, { id: 'mid', width: 236, height: 68 }, { id: 'low', width: 236, height: 44 }]
    const { at, routes } = await layered(nodes, [['top', 'mid'], ['mid', 'low'], ['top', 'low']])
    expect(routes.get('top→low')!.length).toBeGreaterThanOrEqual(2)
    expect(at.get('top')!.y).toBeLessThan(at.get('mid')!.y)
    expect(at.get('mid')!.y + 68).toBeLessThanOrEqual(at.get('low')!.y)
    expect(bounds(nodes, at).width).toBeGreaterThanOrEqual(236)
  })

})

describe('arrow-key navigation', () => {
  it('moves to the closest box in the direction', async () => {
    const { nearest } = await import('./layout')
    const rects = new Map([
      ['a', { x: 0, y: 0, w: 100, h: 40 }],
      ['b', { x: 0, y: 100, w: 100, h: 40 }],
      ['c', { x: 200, y: 0, w: 100, h: 40 }],
      ['d', { x: 220, y: 110, w: 100, h: 40 }],
    ])
    expect(nearest(rects, 'a', 'down')).toBe('b')
    expect(nearest(rects, 'a', 'right')).toBe('c')
    expect(nearest(rects, 'c', 'down')).toBe('d')
    expect(nearest(rects, 'a', 'up')).toBeUndefined()
    expect(nearest(rects, 'zzz', 'up')).toBe('a')
  })
})

describe('roundedPath', () => {
  it('rounds corners of an orthogonal route', async () => {
    const { roundedPath } = await import('./layout')
    expect(roundedPath([{ x: 0, y: 0 }, { x: 0, y: 20 }, { x: 30, y: 20 }])).toBe('M0,0 L0,12 Q0,20 8,20 L30,20')
    expect(roundedPath([{ x: 0, y: 0 }, { x: 0, y: 10 }])).toBe('M0,0 L0,10')
  })
})

describe('directory grouping', () => {
  it('groups more than 25 packages by path and rolls them up', async () => {
    const { groupMetrics, parentGroup } = await import('./model')
    const nodes: Snapshot['nodes'] = [{ id: 'm', kind: 'module', name: 'm' }]
    const edges: Snapshot['edges'] = []
    const metrics: Snapshot['metrics'] = {}
    for (let i = 0; i < 20; i++) nodes.push({ id: `m/internal/a${i}`, kind: 'package', name: `internal/a${i}`, parent: 'm' })
    for (let i = 0; i < 6; i++) nodes.push({ id: `m/cmd/c${i}`, kind: 'package', name: `cmd/c${i}`, parent: 'm' })
    nodes.push({ id: 'm/tools', kind: 'package', name: 'tools', parent: 'm' })
    edges.push({ from: 'm/cmd/c0', to: 'm/internal/a1', kind: 'imports' }, { from: 'm/cmd/c1', to: 'm/internal/a2', kind: 'imports' })
    metrics['m/internal/a3'] = { crap_max: 30, worst: 'm/internal/a3.F', grades: { crap: 2, mutation: null, coverage: 4, combined: 2 } }
    metrics['m/internal/a4'] = { crap_max: 9, grades: { crap: 6, mutation: 3, coverage: 8, combined: 4 } }
    const big = index({ ...snap, nodes, edges, metrics })
    const top = moduleView(big)
    expect(top.groups.map((g) => [g.prefix, g.packages.length])).toEqual([['internal', 20], ['cmd', 6]])
    expect(top.packages).toEqual(['m/tools'])
    expect(top.edges).toEqual([['group:cmd', 'group:internal']])
    const internal = moduleView(big, 'internal')
    expect(internal.groups).toEqual([])
    expect(internal.packages).toHaveLength(20)
    expect(groupMetrics(big, top.groups[0]!.packages)).toEqual({
      crap_max: 30, worst: 'm/internal/a3.F', grades: { crap: 2, mutation: 3, coverage: 4, combined: 2 },
    })
    const { groupOf } = await import('./model')
    expect(groupOf(big, 'm/internal/a3')).toBe('internal')
    expect(groupOf(big, 'm/tools')).toBeUndefined()
    expect(groupOf(ix, P)).toBeUndefined()
    expect(parentGroup('internal/x')).toBe('internal')
    expect(parentGroup('internal')).toBe('')
  })
})
