// Levels 0 (module) and 1 (package) on a React Flow canvas. Nodes render
// invisibly first so React Flow measures them; flow.ts picks the rows (entry
// on top), ELK lays them out (docs/decisions.md #8), pills go in a strip
// underneath, and the view opens readable at the top.

import {
  applyNodeChanges, Background, BackgroundVariant, MarkerType, ReactFlow, ReactFlowProvider,
  useNodesInitialized, useReactFlow, useStoreApi, type Edge, type Node, type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { Thresholds } from '../api'
import { acyclic, backbone, flowRows, primaryEntry, related, type Arrow, type Rows } from '../flow'
import { bounds, layered, openingView, shift as shiftPoints, strip, type Point, type Sized } from '../layout'
import {
  functionChips, GROUP, groupMetrics, isStale, metricsOf, moduleView, packageProblems, packageView, short,
  type Group, type Index, type Mode, type Outside,
} from '../model'
import { gradeColor, type Palette } from '../theme'
import { edgeTypes, nodeTypes, type BoxData, type CardData, type PillData } from './nodes'
import { boxStyle, cardStyle, chipStyle, dots, modeGrade, toneColor } from './visual'

export interface Rect { x: number; y: number; w: number; h: number }

interface Props {
  ix: Index
  level: 0 | 1
  pkg?: string
  group?: string
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
const FN_W = 240
const PAD = 32
const HEADER = 64
/** ROW_MAX cards per row; wider ranks wrap (flow.ts). */
const ROW_MAX = 4
/** STRIP_GAP is the space between the map and the pill strip under it. */
const STRIP_GAP = 56

interface Built {
  nodes: Node[]
  /** drawn: the backbone, plus every real arrow of the selected node */
  edges: Edge[]
  /** laid out and routed by ELK: the backbone without cycles */
  layoutEdges: Arrow[]
  rows: Rows
  /** where reading starts: the view opens centered on it */
  entry?: string
  key: string
  boxId?: string
  /** pills under the map, no arrows (external modules, other packages) */
  strip: string[]
}

function build(p: Props): Built {
  return p.level === 0 ? buildModule(p) : buildPackage(p)
}

function cardData(p: Props, id: string, width: number, isRelated = false): CardData {
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
    style: cardStyle(p.palette, modeGrade(m, p.mode), p.sel === id, isStale(m), isRelated),
    onSelect: p.onSelect, onOpen: p.onOpen,
  }
}

/** groupData is a card for a directory of packages; it looks and rolls up
 * like a package card and opens into its packages. */
function groupData(p: Props, g: Group, isRelated = false): CardData {
  const m = groupMetrics(p.ix, g.packages)
  const d = dots(p.palette, m)
  const pure = g.packages.every((id) => p.ix.byId.get(id)?.pure === true)
  const worst = m.worst ? `${p.ix.byId.get(m.worst)?.name ?? short(m.worst)} · CRAP ${m.crap_max?.toFixed(1)}` : ''
  return {
    id: g.id, width: PKG_W, title: `${g.prefix}/`, pure, vis: '',
    aria: `${g.prefix}, group of ${g.packages.length} packages. Double-click to open.`,
    cDot: d.c, mDot: d.m,
    problems: [
      { text: `${g.packages.length} packages`, color: p.palette.faint },
      ...(worst && m.crap_max !== undefined && m.crap_max > p.th.crap_max ? [{ text: worst, color: p.palette.orange }] : []),
    ],
    chips: [],
    style: cardStyle(p.palette, modeGrade(m, p.mode), p.sel === g.id, false, isRelated),
    onSelect: p.onSelect, onOpen: p.onOpen,
  }
}

export const isGroup = (id: string | undefined): boolean => id?.startsWith(GROUP) ?? false

/** pillStyle marks the selected pill, and pills related to the selection. */
function pillStyle(p: Props, id: string, isRelated: boolean): PillData['style'] {
  if (p.sel === id) return { border: `2px solid ${p.palette.accent}`, boxShadow: `0 0 0 4px ${p.palette.accentSoft}` }
  return isRelated ? { border: `1.5px solid ${p.palette.accent}` } : undefined
}

/** externalPill is an external module under the module map. */
function externalPill(p: Props, id: string, isRelated: boolean): PillData {
  const n = p.ix.byId.get(id)
  return {
    id, title: n?.name ?? id, variant: 'external', aria: `${n?.name ?? id}, external module`,
    style: pillStyle(p, id, isRelated), onSelect: p.onSelect, onOpen: p.onOpen,
  }
}

/** packagePill is another package this one calls, under the package box. */
function packagePill(p: Props, o: Outside, isRelated: boolean): PillData {
  const name = p.ix.byId.get(o.pkg)?.name ?? short(o.pkg)
  const grade = modeGrade(metricsOf(p.ix, o.pkg), p.mode)
  const n = o.funcs.length
  return {
    id: o.pkg, title: `${name} · ${n}`, variant: 'func',
    aria: `${name}, another package; ${n} of its functions ${n === 1 ? 'is' : 'are'} called from here. Double-click to open.`,
    dot: grade === null ? p.palette.border2 : gradeColor(p.palette, grade),
    style: pillStyle(p, o.pkg, isRelated), onSelect: p.onSelect, onOpen: p.onOpen,
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

/** flowBuilt adds rows and arrows for the flow nodes (cards) of a level.
 * Only the backbone is drawn; the selection's other real arrows show as
 * outlined cards (see near in the builders). */
function flowBuilt(p: Props, nodes: Node[], flow: string[], arrows: Arrow[], key: string, rest: { boxId?: string; strip: string[] }): Built {
  const line = backbone(flow, arrows)
  const rows = flowRows(flow, line, ROW_MAX)
  const layoutEdges = acyclic(flow, line)
  return {
    nodes, rows, layoutEdges, ...rest, entry: primaryEntry(rows, line),
    edges: line.map(([a, b]) => edge(p, a, b)),
    key: `${key}|${layoutEdges.length}`,
  }
}

function buildModule(p: Props): Built {
  const v = moduleView(p.ix, p.group ?? '')
  const flow = [...v.groups.map((g) => g.id), ...v.packages]
  const inFlow = new Set(flow)
  const near = p.sel ? related(p.sel, v.edges) : new Set<string>()
  const nodes: Node[] = [
    ...v.groups.map((g): Node => ({ id: g.id, type: 'card', position: { x: 0, y: 0 }, data: groupData(p, g, near.has(g.id)) })),
    ...v.packages.map((id): Node => ({ id, type: 'card', position: { x: 0, y: 0 }, data: cardData(p, id, PKG_W, near.has(id)) })),
    ...v.externals.map((id): Node => ({ id, type: 'pill', position: { x: 0, y: 0 }, data: externalPill(p, id, near.has(id)) })),
  ]
  const key = ['L0', p.group ?? '', ...nodes.map((n) => sizeKey(n))].join('|')
  return flowBuilt(p, nodes, flow, v.edges.filter(([, b]) => inFlow.has(b)), key, { strip: v.externals })
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
  const near = p.sel ? related(p.sel, [...v.edges, ...v.calls]) : new Set<string>()
  const cards: Node[] = v.funcs.map((id) => ({ id, type: 'card', parentId: boxId, position: { x: 0, y: 0 }, data: cardData(p, id, FN_W, near.has(id)) }))
  if (v.more > 0) {
    cards.push({
      id: 'more', type: 'card', parentId: boxId, position: { x: 0, y: 0 },
      data: { ...cardData(p, 'more', FN_W), title: `${v.more} more`, aria: `${v.more} more functions. Open to show all.`, more: true, style: {}, onOpen: p.onOpen, onSelect: p.onOpen },
    })
  }
  const pills: Node[] = v.outside.map((o) => ({ id: o.pkg, type: 'pill', position: { x: 0, y: 0 }, data: packagePill(p, o, near.has(o.pkg)) }))
  const nodes = [box, ...cards, ...pills]
  const key = ['L1', pkg, String(p.expanded), ...nodes.map((n) => sizeKey(n))].join('|')
  return flowBuilt(p, nodes, cards.map((c) => c.id), v.edges, key, { boxId, strip: v.outside.map((o) => o.pkg) })
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

interface Area { x: number; y: number; width: number; height: number }

interface Laid {
  nodes: Placed
  routes: Map<string, Point[]>
  /** everything drawn, for the opening view */
  content: Area
  /** the middle of the entry card: where reading starts */
  focusX: number
}

async function layout(b: Built, measured: Node[]): Promise<Laid> {
  const size = new Map(measured.map((n) => [n.id, sized(n)]))
  const flow = b.rows.order.flatMap((id) => size.get(id) ?? [])
  const l = await layered(flow, b.layoutEdges, b.rows)
  const out: Placed = new Map()
  let area: Area
  let routes = l.routes
  let dx = 0
  let dy = 0
  if (!b.boxId) {
    area = bounds(flow, l.at)
  } else {
    const boxNode = measured.find((n) => n.id === b.boxId)
    const bb = bounds(flow, l.at)
    const width = Math.max(bb.width + 2 * PAD, (boxNode?.measured?.width ?? 0) + 24, 560)
    dx = PAD + (width - 2 * PAD - bb.width) / 2 - bb.x
    dy = HEADER - bb.y
    area = { x: 0, y: 0, width, height: HEADER + bb.height + 28 }
    out.set(b.boxId, { pos: { x: 0, y: 0 }, size: { width: area.width, height: area.height } })
    routes = new Map([...l.routes].map(([k, pts]) => [k, shiftPoints(pts, dx, dy)]))
  }
  for (const n of flow) {
    const at = l.at.get(n.id) ?? { x: 0, y: 0 }
    out.set(n.id, { pos: { x: at.x + dx, y: at.y + dy } })
  }
  const pills = b.strip.flatMap((id) => size.get(id) ?? [])
  const under = strip(pills, area.x + area.width / 2, area.y + area.height + STRIP_GAP, Math.max(area.width, 720))
  for (const [id, pos] of under) out.set(id, { pos })
  const first = flow.filter((n) => n.id === b.entry)
  const fb = bounds(first, new Map(first.map((n) => [n.id, out.get(n.id)!.pos])))
  const content = pills.length > 0 ? union(area, bounds(pills, under)) : area
  return { nodes: out, routes, content, focusX: fb.x + fb.width / 2 }
}

function union(a: Area, b: Area): Area {
  const x = Math.min(a.x, b.x)
  const y = Math.min(a.y, b.y)
  return { x, y, width: Math.max(a.x + a.width, b.x + b.width) - x, height: Math.max(a.y + a.height, b.y + b.height) - y }
}

function Inner(p: Props) {
  const built = useMemo(() => build(p), [p])
  const [nodes, setNodes] = useState<Node[]>([])
  const [ready, setReady] = useState(false)
  const [paths, setPaths] = useState<Map<string, Point[]>>(new Map())
  const keyRef = useRef('')
  const initialized = useNodesInitialized()
  const rf = useReactFlow()
  const store = useStoreApi()
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
    layout(built, measured).then(({ nodes: at, routes, content, focusX }) => {
      if (cancelled) return
      setPaths(routes)
      setNodes((cur) => cur.map((n) => {
        const l = at.get(n.id)
        const style = { ...n.style, opacity: 1, ...(l?.size ?? {}) }
        return { ...n, position: l?.pos ?? n.position, style, ...(l?.size ?? {}) }
      }))
      setReady(true)
      p.onPositions(rects(measured, at))
      const { width, height } = store.getState()
      const view = openingView(content, focusX, { width, height }, built.boxId !== undefined)
      requestAnimationFrame(() => void rf.setViewport(view, { duration: reduced ? 0 : 200 }))
    })
    return () => { cancelled = true }
  }, [initialized, ready, nodes.length, built, rf, store, reduced, p])

  // + / − / 0 zoom keys.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e.target)) return
      if (e.key === '+' || e.key === '=') void rf.zoomIn({ duration: 120 })
      else if (e.key === '-') void rf.zoomOut({ duration: 120 })
      else if (e.key === '0') void rf.fitView({ padding: 0.12, maxZoom: 1, duration: 120 })
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
  return pts ? { ...e, type: 'routed', data: { points: pts, spline: true } } : e
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
