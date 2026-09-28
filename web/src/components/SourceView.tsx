// The annotated source of one function (docs/UI.md §6).

import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import type { Annotation, Source } from '../api'
import { tokenize } from '../highlight'
import { alpha, type Palette } from '../theme'

type Token = { content: string; color?: string }

interface Props {
  source: Source
  palette: Palette
  selectedLine?: number
  scrollTo?: number
  onSelectLine: (line: number) => void
  onResolveNote: (id: string) => void
}

export function SourceView({ source, palette, selectedLine, scrollTo, onSelectLine, onResolveNote }: Props) {
  const [tokens, setTokens] = useState<Token[][] | null>(null)
  const code = useMemo(() => source.lines.map((l) => l.text).join('\n'), [source])
  const box = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let live = true
    tokenize(code, palette).then((t) => live && setTokens(t)).catch(() => live && setTokens(null))
    return () => { live = false }
  }, [code, palette])

  // Open at the top of the function, or at the selected line.
  useEffect(() => {
    if (!box.current) return
    const el = scrollTo ? box.current.querySelector<HTMLElement>(`[data-line="${scrollTo}"]`) : null
    box.current.scrollTop = el ? Math.max(0, el.offsetTop - box.current.clientHeight / 3) : 0
  }, [scrollTo, source.id])

  return (
    <div className="code" ref={box} role="region" aria-label={`Source of ${source.id}`} tabIndex={0}>
      {source.lines.map((l, i) => (
        <div key={l.n} data-line={l.n} style={{ background: lineTint(palette, l.state, (l.survived ?? 0) > 0, l.n === selectedLine) }}>
          <div className="line">
            <button type="button" className="lnum" aria-label={`Select line ${l.n}`} aria-pressed={l.n === selectedLine} onClick={() => onSelectLine(l.n)}>{l.n}</button>
            <span className="gutter" style={{ background: gutter(palette, l.state) }} title={l.state ? `${l.state === 'uncovered' ? 'not covered' : l.state}` : undefined} />
            <span className="src">
              {(tokens?.[i] ?? [{ content: l.text }]).map((t, j) => (
                <span key={j} style={{ color: t.color ?? palette.text }}>{t.content}</span>
              ))}
            </span>
          </div>
          {l.annotations?.map((a, j) => <Note key={j} a={a} palette={palette} onResolve={onResolveNote} />)}
        </div>
      ))}
    </div>
  )
}

function lineTint(p: Palette, state: string | undefined, survived: boolean, selected: boolean): string {
  if (selected) return p.accentSoft
  if (state === 'uncovered') return alpha(p.red, 0.09)
  if (survived) return alpha(p.yellow, 0.08)
  return 'transparent'
}

function gutter(p: Palette, state: string | undefined): string {
  switch (state) {
    case 'covered': return p.green
    case 'uncovered': return p.red
    case 'partial': return `linear-gradient(${p.yellow} 50%, ${p.green} 50%)`
  }
  return 'transparent'
}

const badges: Record<Annotation['kind'], string> = { survived: 'SURVIVED', not_covered: 'NOT COVERED', note: 'NOTE' }

function Note({ a, palette, onResolve }: { a: Annotation; palette: Palette; onResolve: (id: string) => void }) {
  const c = a.kind === 'survived' ? palette.yellow : a.kind === 'not_covered' ? palette.red : palette.accent
  const style: CSSProperties = { background: alpha(c, palette.dark ? 0.1 : 0.08), border: `1px solid ${alpha(c, 0.35)}` }
  let title = a.title
  if (a.kind === 'note' && a.created) title += ` · ${ago(a.created)}`
  if (a.outdated) title += ' (code changed)'
  return (
    <div className={a.outdated ? 'note outdated' : 'note'} style={style}>
      <div className="note-head">
        <span className="badge" style={{ background: c }}>{badges[a.kind]}</span>
        <span className="note-title">{title}</span>
        {a.detail && <span className="note-detail">{a.detail}</span>}
        {a.note_id && (
          <button type="button" className="linkbtn" onClick={() => onResolve(a.note_id!)}>Resolve</button>
        )}
      </div>
      {a.why && <div className="note-why">{a.why}</div>}
    </div>
  )
}

/** ago formats a past time as "2 days ago". */
export function ago(iso: string, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000))
  const steps: [number, string][] = [[86400, 'day'], [3600, 'hour'], [60, 'minute']]
  for (const [size, unit] of steps) {
    const n = Math.floor(s / size)
    if (n >= 1) return `${n} ${unit}${n === 1 ? '' : 's'} ago`
  }
  return 'just now'
}
