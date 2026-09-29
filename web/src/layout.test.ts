import { describe, expect, it } from 'vitest'
import { flowRows } from './flow'
import { openingView, splinePath, strip, toElk } from './layout'

describe('toElk with rows', () => {
  it('lists children in flow order on their rows and keeps the rows', () => {
    const rows = flowRows(['b', 'a'], [['a', 'b']], 4)
    const g = toElk([{ id: 'b', width: 10, height: 10 }, { id: 'a', width: 10, height: 10 }], [['a', 'b']], rows)
    expect(g.children?.map((c) => [c.id, c.y])).toEqual([['a', 0], ['b', 1000]])
    expect(g.layoutOptions?.['elk.layered.layering.strategy']).toBe('INTERACTIVE')
    expect(g.layoutOptions?.['elk.edgeRouting']).toBe('SPLINES')
    expect(g.layoutOptions?.['elk.separateConnectedComponents']).toBe('false')
  })
})

describe('splinePath', () => {
  it('draws ELK spline control points as cubic segments', () => {
    const pts = [{ x: 0, y: 0 }, { x: 0, y: 10 }, { x: 5, y: 20 }, { x: 5, y: 30 }]
    expect(splinePath(pts)).toBe('M0,0 C0,10 5,20 5,30')
  })
  it('falls back to straight segments when the points are not a spline', () => {
    expect(splinePath([{ x: 0, y: 0 }, { x: 0, y: 9 }, { x: 4, y: 9 }])).toBe('M0,0 L0,9 L4,9')
  })
})

describe('strip', () => {
  it('centers pills in rows no wider than width', () => {
    const at = strip([{ id: 'a', width: 40, height: 20 }, { id: 'b', width: 40, height: 20 }, { id: 'c', width: 40, height: 20 }], 100, 0, 100, 10)
    expect([...at]).toEqual([['a', { x: 55, y: 0 }], ['b', { x: 105, y: 0 }], ['c', { x: 80, y: 30 }]])
  })
})

describe('openingView', () => {
  const pane = { width: 1000, height: 800 }
  it('shows narrow content whole, centered, at most at 100%', () => {
    expect(openingView({ x: 0, y: 0, width: 500, height: 900 }, 250, pane)).toEqual({ x: 250, y: 24, zoom: 1 })
  })
  it('zooms out to fit width, but not below readable, then centers on the entry', () => {
    const v = openingView({ x: 0, y: 0, width: 3000, height: 900 }, 1500, pane)
    expect(v.zoom).toBe(0.7)
    expect(v.x).toBe(500 - 1500 * 0.7)
  })
  it('keeps the entry centered even near an edge', () => {
    expect(openingView({ x: 0, y: 0, width: 3000, height: 900 }, 100, pane).x).toBe(500 - 100 * 0.7)
  })
  it('with edges, does not scroll past the edges of the content', () => {
    expect(openingView({ x: 0, y: 0, width: 3000, height: 900 }, 100, pane, true).x).toBe(24)
    expect(openingView({ x: 0, y: 0, width: 3000, height: 900 }, 2950, pane, true).x).toBeCloseTo(1000 - 24 - 3000 * 0.7)
  })
})
