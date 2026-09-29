// React Flow node types. Every box is a real <button> (docs/UI.md §3):
// click selects, double-click / Enter twice / "o" opens.

import { BaseEdge, Handle, Position, type Edge, type EdgeProps, type Node, type NodeProps } from '@xyflow/react'
import { roundedPath, splinePath, type Point } from '../layout'
import { useRef, type CSSProperties, type KeyboardEvent } from 'react'

export interface CardData extends Record<string, unknown> {
  id: string
  title: string
  aria: string
  width: number
  pure: boolean
  vis: string
  cDot: string
  mDot: string
  problems: { text: string; color: string }[]
  chips: { text: string; style: CSSProperties }[]
  style: CSSProperties
  more?: boolean
  onSelect: (id: string) => void
  onOpen: (id: string) => void
}

export interface PillData extends Record<string, unknown> {
  id: string
  title: string
  aria: string
  variant: 'func' | 'external' | 'type'
  dot?: string
  style?: CSSProperties
  onSelect: (id: string) => void
  onOpen: (id: string) => void
}

export interface BoxData extends Record<string, unknown> {
  id: string
  title: string
  pure: boolean
  cDot: string
  mDot: string
  style: CSSProperties
  types: { id: string; name: string; selected: boolean; style?: CSSProperties }[]
  onSelect: (id: string) => void
}

export type CardNode = Node<CardData, 'card'>
export type PillNode = Node<PillData, 'pill'>
export type BoxNode = Node<BoxData, 'box'>

/** useOpenKeys: Enter twice quickly or "o" opens; Enter/Space select (native). */
function useOpenKeys(id: string, onOpen: (id: string) => void) {
  const last = useRef(0)
  return (e: KeyboardEvent) => {
    if (e.key === 'o') {
      e.preventDefault()
      onOpen(id)
    } else if (e.key === 'Enter') {
      const now = Date.now()
      if (now - last.current < 400) onOpen(id)
      last.current = now
    }
  }
}

function Handles() {
  return (
    <>
      <Handle type="target" position={Position.Top} isConnectable={false} />
      <Handle type="source" position={Position.Bottom} isConnectable={false} />
    </>
  )
}

export function Card({ data }: NodeProps<CardNode>) {
  const keys = useOpenKeys(data.id, data.onOpen)
  return (
    <div style={{ width: data.width }}>
      <Handles />
      <button
        type="button"
        className={data.more ? 'card more' : 'card'}
        style={data.style}
        aria-label={data.aria}
        data-qt-id={data.id}
        onClick={() => data.onSelect(data.id)}
        onDoubleClick={() => data.onOpen(data.id)}
        onKeyDown={keys}
      >
        {data.more ? (
          <span>{data.title}</span>
        ) : (
          <>
            <div className="card-head">
              {data.pure && <span className="lambda" title="pure (heuristic)">λ</span>}
              {data.vis && <span className="vis" title={data.vis === '+' ? 'exported' : 'unexported'}>{data.vis}</span>}
              <span className="title">{data.title}</span>
              <span className="dotc" style={{ background: data.cDot }} title="CRAP grade">C</span>
              <span className="dotc" style={{ background: data.mDot }} title="Mutation grade">M</span>
            </div>
            {data.problems.map((pr) => (
              <div className="problem" key={pr.text}>
                <i style={{ background: pr.color }} />
                <span>{pr.text}</span>
              </div>
            ))}
            {data.chips.length > 0 && (
              <div className="chips">
                {data.chips.map((c) => (
                  <span className="chip" style={c.style} key={c.text}>{c.text}</span>
                ))}
              </div>
            )}
          </>
        )}
      </button>
    </div>
  )
}

export function Pill({ data }: NodeProps<PillNode>) {
  const keys = useOpenKeys(data.id, data.onOpen)
  return (
    <div>
      <Handles />
      <button
        type="button"
        className={`pill ${data.variant}`}
        style={data.style}
        aria-label={data.aria}
        data-qt-id={data.id}
        onClick={() => data.onSelect(data.id)}
        onDoubleClick={() => data.onOpen(data.id)}
        onKeyDown={keys}
      >
        {data.dot && <span className="pdot" style={{ background: data.dot }} />}
        {data.title}
      </button>
    </div>
  )
}

export function Box({ data }: NodeProps<BoxNode>) {
  return (
    <div className="box" style={data.style}>
      <div className="box-head">
        <button type="button" className="box-title" data-qt-id={data.id} aria-label={`${data.title} package`} onClick={() => data.onSelect(data.id)}>
          {data.pure && <span className="lambda">λ</span>}
          <span className="name">{data.title}</span>
          <span className="dotc" style={{ background: data.cDot }}>C</span>
          <span className="dotc" style={{ background: data.mDot }}>M</span>
        </button>
        {data.types.map((t) => (
          <button
            type="button"
            key={t.id}
            className="pill type"
            style={t.style}
            data-qt-id={t.id}
            aria-label={`${t.name} type`}
            onClick={() => data.onSelect(t.id)}
          >
            {t.name}
          </button>
        ))}
      </div>
    </div>
  )
}

export const nodeTypes = { card: Card, pill: Pill, box: Box }

export type RoutedEdge = Edge<{ points: Point[]; spline?: boolean }, 'routed'>

/** Routed draws the route ELK computed: a spline, or an orthogonal route with
 * rounded corners. */
export function Routed({ id, data, style, markerEnd }: EdgeProps<RoutedEdge>) {
  const pts = data?.points ?? []
  return <BaseEdge id={id} path={data?.spline ? splinePath(pts) : roundedPath(pts)} style={style} markerEnd={markerEnd} />
}

export const edgeTypes = { routed: Routed }
