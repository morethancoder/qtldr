import { describe, expect, it } from 'vitest'
import { acyclic, backbone, flowRows, primaryEntry, related, type Arrow } from './flow'

const rowsOf = (r: Map<string, number>): string[][] => {
  const out: string[][] = []
  for (const [id, row] of r) (out[row] ??= []).push(id)
  return out.map((ids) => ids.sort())
}

describe('acyclic', () => {
  it('drops the edge that closes a cycle and self-loops', () => {
    const e: Arrow[] = [['a', 'b'], ['b', 'c'], ['c', 'a'], ['c', 'c']]
    expect(acyclic(['a', 'b', 'c'], e)).toEqual([['a', 'b'], ['b', 'c']])
  })
})

describe('backbone', () => {
  it('hides an arrow a longer path already shows', () => {
    const e: Arrow[] = [['main', 'cli'], ['cli', 'server'], ['server', 'model'], ['cli', 'model'], ['main', 'model']]
    expect(backbone(['main', 'cli', 'server', 'model'], e)).toEqual([['main', 'cli'], ['cli', 'server'], ['server', 'model']])
  })
  it('keeps arrows with no other path and ignores duplicates', () => {
    const e: Arrow[] = [['a', 'b'], ['a', 'c'], ['a', 'b']]
    expect(backbone(['a', 'b', 'c'], e)).toEqual([['a', 'b'], ['a', 'c']])
  })
  it('keeps a cycle drawable', () => {
    const e: Arrow[] = [['a', 'b'], ['b', 'a']]
    expect(backbone(['a', 'b'], e)).toEqual([['a', 'b'], ['b', 'a']])
  })
})

describe('flowRows', () => {
  it('puts the entry on top and each node below everything that points to it', () => {
    const e: Arrow[] = [['main', 'cli'], ['cli', 'server'], ['server', 'model'], ['cli', 'model']]
    const { row } = flowRows(['model', 'server', 'cli', 'main'], e, 6)
    expect(rowsOf(row)).toEqual([['main'], ['cli'], ['server'], ['model']])
  })
  it('puts nodes with no arrows at all in the last rows', () => {
    const { row } = flowRows(['lonely', 'a', 'b'], [['a', 'b']], 6)
    expect(rowsOf(row)).toEqual([['a'], ['b'], ['lonely']])
  })
  it('wraps a wide row into balanced rows that each span the width', () => {
    const kids = ['k1', 'k2', 'k3', 'k4', 'k5']
    const { row, order } = flowRows(['root', ...kids], kids.map((k): Arrow => ['root', k]), 3)
    expect(rowsOf(row)).toEqual([['root'], ['k1', 'k3', 'k5'], ['k2', 'k4']])
    expect(order).toEqual(['root', 'k1', 'k3', 'k5', 'k2', 'k4'])
  })
  it('orders a row by where its parents sit', () => {
    const e: Arrow[] = [['r', 'left'], ['r', 'right'], ['left', 'x'], ['right', 'y']]
    const { order } = flowRows(['r', 'left', 'right', 'y', 'x'], e, 6)
    expect(order).toEqual(['r', 'left', 'right', 'x', 'y'])
  })
  it('survives cycles', () => {
    const { row } = flowRows(['a', 'b'], [['a', 'b'], ['b', 'a']], 6)
    expect(rowsOf(row)).toEqual([['a'], ['b']])
  })
})

describe('related', () => {
  it('lists every node with a real arrow to or from id', () => {
    const e: Arrow[] = [['a', 'b'], ['c', 'a'], ['b', 'c']]
    expect([...related('a', e)].sort()).toEqual(['b', 'c'])
  })
})

describe('primaryEntry', () => {
  it('is the top-row node that reaches the most nodes', () => {
    const e: Arrow[] = [['tool', 'shared'], ['app', 'api'], ['api', 'store'], ['api', 'shared']]
    const rows = flowRows(['tool', 'app', 'api', 'store', 'shared'], e, 4)
    expect(primaryEntry(rows, e)).toBe('app')
  })
  it('is undefined for an empty map', () => {
    expect(primaryEntry(flowRows([], [], 4), [])).toBeUndefined()
  })
})
