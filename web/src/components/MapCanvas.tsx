// Levels 0 (module) and 1 (package) on a React Flow canvas. Nodes render
// invisibly first so React Flow measures them; ELK then lays them out and the
// view fits (docs/decisions.md #8).

import {
  applyNodeChanges, Background, BackgroundVariant, MarkerType, ReactFlow, ReactFlowProvider,
  useNodesInitialized, useReactFlow, type Edge, type Node, type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { Thresholds } from '../api'
import { bounds, layered, shift as shiftPoints, type Point, type Sized } from '../layout'
import {
  functionChips, isStale, metricsOf, moduleView, packageProblems, packageView, short,
  type Index, type Mode,
} from '../model'
import { gradeColor, type Palette } from '../theme'
import { edgeTypes, nodeTypes, type BoxData, type CardData, type PillData } from './nodes'
import { boxStyle, cardStyle, chipStyle, dots, modeGrade, toneColor } from './visual'

export interface Rect { x: number; y: number; w: number; h: number }

interface Props {
  ix: Index
  level: 0 | 1
  pkg?: string
  mode: Mode
  palette: Palette
  th: Thresholds
  sel?: string
  expanded: boolean
  onSelect: (id: string) => void
  onOpen: (id: string) => void
  onPositions: (rects: Map<string, Rect>) => void
}

const PKG_W = 236
const FN_W = 200
const PAD = 32
const HEADER = 64

interface Built {
  nodes: Node[]
  edges: Edge[]
  key: string
  boxId?: string
  outside: string[]
}

function build(p: Props): Built {
  return p.level === 0 ? buildModule(p) : buildPackage(p)
}

function cardData(p: Props, id: string, width: number): CardData {
  const n = p.ix.byId.get(id)
  const m = metricsOf(p.ix, id)
  const d = dots(p.palette, m)
  const isPkg = n?.kind === 'package'
  const chips = isPkg ? [] : functionChips(m, p.th)
  const problems = isPkg ? packageProblems(p.ix, id, p.th) : []
  const key = isPkg ? `CRAP max ${m.crap_max?.toFixed(1) ?? 'not measured'}` : `CRAP ${m.crap?.toFixed(1) ?? 'not measured'}`
  return {
    id, width, title: n?.name ?? short(id), pure: n?.pure === true, vis: isPkg ? '' : n?.exported ? '+' : '−',
    aria: `${n?.name ?? id}, ${isPkg ? 'package' : 'function'}, ${key}. Double-click to open.`,
    cDot: d.c, mDot: d.m,
    problems: problems.map((pr) => ({ text: pr.text, color: toneColor(p.palette, pr.tone) })),
    chips: chips.map((c) => ({ text: c.text, style: chipStyle(p.palette, c.tone) })),
    style: cardStyle(p.palette, modeGrade(m, p.mode), p.sel === id, isStale(m)),
    onSelect: p.onSelect, onOpen: p.onOpen,
  }
}

function pillData(p: Props, id: string): PillData {
  const n = p.ix.byId.get(id)
  const m = metricsOf(p.ix, id)
  const external = n?.kind === 'external'
  const grade = modeGrade(m, p.mode)
  const selected = p.sel === id
  return {
    id, title: external ? (n?.name ?? id) : short(id), variant: external ? 'external' : 'func',
    aria: external ? `${n?.name}, external module` : `${short(id)}, function in another package`,
    dot: external ? undefined : grade === null ? p.palette.border2 : gradeColor(p.palette, grade),
    style: selected ? { border: `2px solid ${p.palette.accent}`, boxShadow: `0 0 0 4px ${p.palette.accentSoft}` } : undefined,
    onSelect: p.onSelect, onOpen: p.onOpen,
  }
}

function edge(p: Props, from: string, to: string): Edge {
  const hi = p.sel === from || p.sel === to
  const color = hi ? p.palette.accent : p.palette.border2
  return {
    id: `${from}→${to}`, source: from, target: to, type: 'default', focusable: false,
    style: { stroke: color, strokeWidth: hi ? 2 : 1.4 },
    markerEnd: { type: MarkerType.ArrowClosed, color, width: 16, height: 16 },
  }
}

function buildModule(p: Props): Built {
  const v = moduleView(p.ix)
  const nodes: Node[] = [
    ...v.packages.map((id): Node => ({ id, type: 'card', position: { x: 0, y: 0 }, data: cardData(p, id, PKG_W) })),
    ...v.externals.map((id): Node => ({ id, type: 'pill', position: { x: 0, y: 0 }, data: pillData(p, id) })),
  ]
  const key = ['L0', ...nodes.map((n) => sizeKey(n))].join('|')
  return { nodes, edges: v.edges.map(([a, b]) => edge(p, a, b)), key, outside: [] }
}

function buildPackage(p: Props): Built {
  const pkg = p.pkg!
  const v = packageView(p.ix, pkg, p.expanded)
  const pm = metricsOf(p.ix, pkg)
  const pd = dots(p.palette, pm)
  const boxId = `box:${pkg}`
  const box: Node = {
    id: boxId, type: 'box', position: { x: 0, y: 0 }, selectable: false,
    data: {
      id: pkg, title: p.ix.byId.get(pkg)?.name ?? pkg, pure: p.ix.byId.get(pkg)?.pure === true, cDot: pd.c, mDot: pd.m,
      style: boxStyle(p.palette, modeGrade(pm, p.mode), p.sel === pkg),
      types: v.types.map((t) => ({
        id: t, name: p.ix.byId.get(t)?.name ?? short(t), selected: p.sel === t,
        style: p.sel === t ? { border: `2px solid ${p.palette.accent}` } : undefined,
      })),
      onSelect: p.onSelect,
    } satisfies BoxData,
  }
  const cards: Node[] = v.funcs.map((id) => ({ id, type: 'card', parentId: boxId, position: { x: 0, y: 0 }, data: cardData(p, id, FN_W) }))
  if (v.more > 0) {
    cards.push({
      id: 'more', type: 'card', parentId: boxId, position: { x: 0, y: 0 },
      data: { ...cardData(p, 'more', FN_W), title: `${v.more} more`, aria: `${v.more} more functions. Open to show all.`, more: true, style: {}, onOpen: p.onOpen, onSelect: p.onOpen },
    })
  }
  const pills: Node[] = v.outside.map((id) => ({ id, type: 'pill', position: { x: 0, y: 0 }, data: pillData(p, id) }))
  const nodes = [box, ...cards, ...pills]
  const key = ['L1', pkg, String(p.expanded), ...nodes.map((n) => sizeKey(n))].join('|')
  return { nodes, edges: v.edges.map(([a, b]) => edge(p, a, b)), key, boxId, outside: v.outside }
}

/** sizeKey changes when a node's size could change. */
function sizeKey(n: Node): string {
  const d = n.data as Partial<CardData & BoxData>
  return [n.id, d.title, d.problems?.map((x) => x.text).join(','), d.chips?.map((x) => x.text).join(','), d.types?.length].join(':')
}

function sized(n: Node): Sized {
  return { id: n.id, width: n.measured?.width ?? 200, height: n.measured?.height ?? 44 }
}

type Placed = Map<string, { pos: Point; size?: { width: number; height: number } }>

interface Laid { nodes: Placed; routes: Map<string, Point[]> }

async function layout(b: Built, measured: Node[]): Promise<Laid> {
  const out: Placed = new Map()
  if (!b.boxId) {
    const all = measured.map(sized)
    const l = await layered(all, b.edges.map((e) => [e.source, e.target]))
    for (const [id, pos] of l.at) out.set(id, { pos })
    return { nodes: out, routes: l.routes }
  }
  const boxNode = measured.find((n) => n.id === b.boxId)
  const kids = measured.filter((n) => n.parentId === b.boxId).map(sized)
  const pills = measured.filter((n) => b.outside.includes(n.id)).map((n) => ({ ...sized(n), last: true }))
  const l = await layered([...kids, ...pills], b.edges.map((e) => [e.source, e.target]))
  const bb = bounds(kids, l.at)
  const width = Math.max(bb.width + 2 * PAD, (boxNode?.measured?.width ?? 0) + 24, 560)
  const height = HEADER + bb.height + 28
  const dx = PAD + (width - 2 * PAD - bb.width) / 2 - bb.x
  const dy = HEADER - bb.y
  out.set(b.boxId, { pos: { x: 0, y: 0 }, size: { width, height } })
  for (const n of [...kids, ...pills]) {
    const p = l.at.get(n.id) ?? { x: 0, y: 0 }
    out.set(n.id, { pos: { x: p.x + dx, y: p.y + dy } })
  }
  const routes = new Map([...l.routes].map(([k, pts]) => [k, shiftPoints(pts, dx, dy)]))
  return { nodes: out, routes }
}

function Inner(p: Props) {
  const built = useMemo(() => build(p), [p])
  const [nodes, setNodes] = useState<Node[]>([])
  const [ready, setReady] = useState(false)
  const [paths, setPaths] = useState<Map<string, Point[]>>(new Map())
  const keyRef = useRef('')
  const initialized = useNodesInitialized()
  const rf = useReactFlow()
  const reduced = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  // New structure: render hidden for measuring. Same structure: refresh data.
  useEffect(() => {
    if (built.key !== keyRef.current) {
      keyRef.current = built.key
      setReady(false)
      setNodes(built.nodes.map((n) => ({ ...n, style: { ...n.style, opacity: 0 } })))
      return
    }
    setNodes((cur) => cur.map((n) => {
      const next = built.nodes.find((b) => b.id === n.id)
      return next ? { ...n, data: next.data } : n
    }))
  }, [built])

  useEffect(() => {
    if (ready || !initialized || nodes.length === 0) return
    let cancelled = false
    const measured = rf.getNodes()
    layout(built, measured).then(({ nodes: at, routes }) => {
      if (cancelled) return
      setPaths(routes)
      setNodes((cur) => cur.map((n) => {
        const l = at.get(n.id)
        const style = { ...n.style, opacity: 1, ...(l?.size ?? {}) }
        return { ...n, position: l?.pos ?? n.position, style, ...(l?.size ?? {}) }
      }))
      setReady(true)
      p.onPositions(rects(measured, at))
      requestAnimationFrame(() => void rf.fitView({ padding: 0.12, duration: reduced ? 0 : 200 }))
    })
    return () => { cancelled = true }
  }, [initialized, ready, nodes.length, built, rf, reduced, p])

  // + / − / 0 zoom keys.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e.target)) return
      if (e.key === '+' || e.key === '=') void rf.zoomIn({ duration: 120 })
      else if (e.key === '-') void rf.zoomOut({ duration: 120 })
      else if (e.key === '0') void rf.fitView({ padding: 0.12, duration: 120 })
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [rf])

  return (
    <ReactFlow
      nodes={nodes}
      edges={ready ? built.edges.map((e) => routed(e, paths)) : []}
      edgeTypes={edgeTypes}
      nodeTypes={nodeTypes}
      onNodesChange={(changes: NodeChange[]) => setNodes((ns) => applyNodeChanges(changes, ns))}
      nodesDraggable={false}
      nodesConnectable={false}
      nodesFocusable={false}
      edgesFocusable={false}
      elementsSelectable={false}
      zoomOnScroll={false}
      panOnScroll
      zoomActivationKeyCode={['Meta', 'Control']}
      zoomOnDoubleClick={false}
      minZoom={0.2}
      maxZoom={2}
      proOptions={{ hideAttribution: true }}
    >
      <Background variant={BackgroundVariant.Dots} gap={22} size={1.2} color={p.palette.border} bgColor={p.palette.surface} />
    </ReactFlow>
  )
}

/** routed uses ELK's route for an edge when there is one. */
function routed(e: Edge, paths: Map<string, Point[]>): Edge {
  const pts = paths.get(`${e.source}→${e.target}`)
  return pts ? { ...e, type: 'routed', data: { points: pts } } : e
}

/** rects turns layout positions into absolute rectangles for keyboard moves. */
function rects(measured: Node[], at: Placed): Map<string, Rect> {
  const out = new Map<string, Rect>()
  for (const n of measured) {
    const l = at.get(n.id)
    if (!l || n.type === 'box') continue
    const parent = n.parentId ? at.get(n.parentId)?.pos ?? { x: 0, y: 0 } : { x: 0, y: 0 }
    out.set(n.id, { x: parent.x + l.pos.x, y: parent.y + l.pos.y, w: n.measured?.width ?? 0, h: n.measured?.height ?? 0 })
  }
  return out
}

export function isTyping(t: EventTarget | null): boolean {
  return t instanceof HTMLElement && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)
}

export function MapCanvas(p: Props) {
  return (
    <ReactFlowProvider>
      <Inner {...p} />
    </ReactFlowProvider>
  )
}
