// Level 2: callers above left, callees above right, the function box with
// its annotated source (docs/UI.md §4 Level 2).

import { useEffect, useRef } from 'react'
import type { Source } from '../api'
import { functionView, metricsOf, short, type Index, type Mode } from '../model'
import { gradeColor, type Palette } from '../theme'
import type { Rect } from './MapCanvas'
import { SourceView } from './SourceView'
import { boxStyle, dots, modeGrade } from './visual'

interface Props {
  ix: Index
  fn: string
  mode: Mode
  palette: Palette
  sel?: string
  source: Source | null
  selectedLine?: number
  scrollTo?: number
  onSelect: (id: string) => void
  onOpen: (id: string) => void
  onSelectLine: (line: number) => void
  onResolveNote: (id: string) => void
  onPositions: (rects: Map<string, Rect>) => void
}

export function FunctionView(p: Props) {
  const node = p.ix.byId.get(p.fn)
  const m = metricsOf(p.ix, p.fn)
  const d = dots(p.palette, m)
  const { callers, callees } = functionView(p.ix, p.fn)
  const root = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const rects = new Map<string, Rect>()
    root.current?.querySelectorAll<HTMLElement>('[data-qt-id]').forEach((el) => {
      const r = el.getBoundingClientRect()
      rects.set(el.dataset.qtId!, { x: r.x, y: r.y, w: r.width, h: r.height })
    })
    p.onPositions(rects)
  })

  const lines = node?.doc_line ? `${node.doc_line}–${node.end_line}` : `${node?.line}–${node?.end_line}`
  const legend: [string, string][] = [[p.palette.green, 'covered'], [p.palette.red, 'not covered'], [p.palette.yellow, 'survived'], [p.palette.accent, 'note']]
  return (
    <div className="fnview" ref={root}>
      <div className="fn-pills">
        <div className="side" aria-label="Called by">
          {callers.map((id) => <FnPill key={id} id={id} dir="down" {...p} />)}
        </div>
        <div className="side" aria-label="Calls">
          {callees.map((id) => <FnPill key={id} id={id} dir="up" {...p} />)}
        </div>
      </div>
      <section className="fn-box" style={boxStyle(p.palette, modeGrade(m, p.mode), p.sel === p.fn)}>
        <div className="box-head">
          <button type="button" className="box-title" data-qt-id={p.fn} aria-label={`${node?.name} function`} onClick={() => p.onSelect(p.fn)}>
            {node?.pure && <span className="lambda">λ</span>}
            <span className="name">{node?.name}</span>
            <span className="dotc" style={{ background: d.c }}>C</span>
            <span className="dotc" style={{ background: d.m }}>M</span>
          </button>
          <span className="box-sub">{node?.file} · lines {lines}</span>
          <span className="grow" />
          <div className="legend">
            {legend.map(([c, t]) => <span key={t}><i style={{ background: c }} />{t}</span>)}
          </div>
        </div>
        {p.source ? (
          <SourceView
            source={p.source}
            palette={p.palette}
            selectedLine={p.selectedLine}
            scrollTo={p.scrollTo}
            onSelectLine={p.onSelectLine}
            onResolveNote={p.onResolveNote}
          />
        ) : (
          <div className="empty">Loading source…</div>
        )}
      </section>
    </div>
  )
}

function FnPill(p: Props & { id: string; dir: 'up' | 'down' }) {
  const m = metricsOf(p.ix, p.id)
  const g = modeGrade(m, p.mode)
  const hi = p.sel === p.id || p.sel === p.fn
  const color = hi ? p.palette.accent : p.palette.border2
  const selected = p.sel === p.id
  return (
    <div className="fn-pill-wrap">
      <button
        type="button"
        className="pill func"
        data-qt-id={p.id}
        aria-label={`${short(p.id)}, ${p.dir === 'down' ? 'caller' : 'callee'}`}
        style={selected ? { border: `2px solid ${p.palette.accent}`, boxShadow: `0 0 0 4px ${p.palette.accentSoft}` } : undefined}
        onClick={() => p.onSelect(p.id)}
        onDoubleClick={() => p.onOpen(p.id)}
        onKeyDown={(e) => e.key === 'o' && p.onOpen(p.id)}
      >
        <span className="pdot" style={{ background: g === null ? p.palette.border2 : gradeColor(p.palette, g) }} />
        {short(p.id)}
      </button>
      <svg width="12" height="26" aria-hidden="true">
        {p.dir === 'down' ? (
          <>
            <path d="M6 0 L6 18" stroke={color} strokeWidth={hi ? 2 : 1.4} />
            <polygon points="6,26 1,17 11,17" fill={color} />
          </>
        ) : (
          <>
            <path d="M6 26 L6 8" stroke={color} strokeWidth={hi ? 2 : 1.4} />
            <polygon points="6,0 1,9 11,9" fill={color} />
          </>
        )}
      </svg>
    </div>
  )
}
