// The inspector (docs/UI.md §5): what the selected box is, its scores with
// explanations, the riskiest functions or surviving mutants, and actions.

import { useState, type ReactNode } from 'react'
import type { Config, Detail, Glossary, Metrics, QNode } from '../api'
import { fmt, funcsOf, metricsOf, riskiest, short, typesOf, type Index } from '../model'
import { gradeColor, type Palette } from '../theme'
import { Help, type TipState } from './Tooltip'

export interface InspectorActions {
  select: (id: string) => void
  open: (id: string) => void
  copyPrompt: () => void
  send: (message: string) => void
  openIn: (editor: string) => void
  refresh: (id: string, coverage: boolean) => void
  addNote: (text: string) => Promise<boolean>
}

interface Props {
  ix: Index
  id: string | undefined
  detail: Detail | null
  level: 0 | 1 | 2
  /** the package or function the canvas shows (for "other package" labels) */
  context?: string
  palette: Palette
  glossary: Glossary
  config: Config
  selectedLine?: number
  busy: boolean
  tips: TipState
  actions: InspectorActions
}

interface RowSpec { key: string; label: string; value: string; grade?: number | null; stale?: boolean }

export function Inspector(p: Props) {
  const node = p.id ? p.ix.byId.get(p.id) : undefined
  if (!node) {
    return <aside className="inspector" aria-label="Inspector"><span className="muted">Select a box to inspect it.</span></aside>
  }
  const m = metricsOf(p.ix, node.id)
  const spec = describe(p, node, m)
  return (
    <aside className="inspector" aria-label="Inspector">
      <div className="ins-head">
        <span className="kind">{spec.kind}</span>
        <div className="ins-title">
          <h2 title={node.id}>{spec.title}</h2>
          {spec.tag && <span className={spec.tag.pure ? 'tag pure' : 'tag'} title={spec.tag.hover}>{spec.tag.text}</span>}
        </div>
        <span className="sub">{spec.sub}</span>
        {node.signature && <div className="sig">{node.signature}</div>}
      </div>
      {spec.rows.length > 0 && (
        <div className="rows">
          {spec.rows.map((r) => <MetricRow key={r.key} row={r} {...p} />)}
        </div>
      )}
      {spec.list}
      <Actions {...p} node={node} primary={spec.primary} />
    </aside>
  )
}

function MetricRow({ row, palette, glossary, tips }: Props & { row: RowSpec }) {
  return (
    <div className="row" onMouseEnter={() => tips.enter(row.key)} onMouseLeave={() => tips.leave()}>
      <span className="l">{row.label}</span>
      <Help tipKey={row.key} term={glossary[row.key]} tips={tips} label={row.label} />
      <span className="grow" />
      <span className="v">{row.value}{row.stale && <span className="stale-mark" title={glossary.stale?.short}>stale</span>}</span>
      <span className="g" style={{ background: row.grade === undefined || row.grade === null ? 'transparent' : gradeColor(palette, row.grade) }} />
    </div>
  )
}

interface Spec {
  kind: string
  title: string
  sub: string
  tag?: { text: string; pure: boolean; hover?: string }
  rows: RowSpec[]
  list?: ReactNode
  primary?: { label: string; run: () => void }
}

function describe(p: Props, node: QNode, m: Metrics): Spec {
  switch (node.kind) {
    case 'package': return packageSpec(p, node, m)
    case 'func': return funcSpec(p, node, m)
    case 'type': return typeSpec(node)
    case 'external': return { kind: 'External module', title: node.name, sub: 'Not analyzed. Shown so you can see what the code depends on.', rows: [] }
  }
  return { kind: 'Module', title: node.name, sub: node.id, rows: [] }
}

function mutRows(m: Metrics): RowSpec[] {
  const mu = m.mutation
  return [
    { key: 'mutation', label: 'Mutation score', value: mu ? fmt.pct(mu.score) : 'not measured', grade: m.grades?.mutation ?? (mu ? null : 1), stale: mu?.stale },
    { key: 'survived', label: 'Surviving mutants', value: mu ? String(mu.survived) : '—' },
    { key: 'not_covered', label: 'Not-covered mutants', value: mu ? String(mu.not_covered) : '—' },
  ]
}

function coverageRow(m: Metrics, grade: number | undefined): RowSpec {
  const c = m.coverage
  const value = !c ? 'not measured' : c.percent === null ? 'no statements' : fmt.pct(c.percent)
  return { key: 'coverage', label: 'Coverage', value, grade: c ? grade : 1, stale: c?.stale }
}

function packageSpec(p: Props, node: QNode, m: Metrics): Spec {
  const funcs = funcsOf(p.ix, node.id)
  const effectful = funcs.filter((f) => f.pure === false).map((f) => `${f.name}: ${f.effects?.[0] ?? 'effectful'}`)
  const risky = riskiest(p.ix, node.id, 3)
  return {
    kind: 'Package',
    title: node.name,
    sub: `${node.id} · ${funcs.length} funcs · ${typesOf(p.ix, node.id).length} types`,
    tag: node.pure === undefined ? undefined : { text: node.pure ? 'λ pure core' : 'effectful shell', pure: node.pure, hover: effectful.slice(0, 6).join('\n') || undefined },
    rows: [
      { key: 'crap_max', label: 'CRAP max', value: fmt.crap(m.crap_max), grade: m.grades?.crap ?? 1, stale: m.coverage?.stale },
      { key: 'crap_avg', label: 'CRAP average', value: fmt.crap(m.crap_avg) },
      coverageRow(m, m.grades?.coverage),
      ...mutRows(m),
      { key: 'churn', label: 'Churn', value: commits(m.churn) },
    ],
    list: risky.length > 0 && (
      <List title="Riskiest functions" items={risky.map((r) => ({
        key: r.id, a: r.name, b: r.reason, c: fmt.crap(r.crap), color: gradeColor(p.palette, r.grade), go: () => p.actions.select(r.id),
      }))} />
    ),
    primary: p.level === 0
      ? { label: 'Open package', run: () => p.actions.open(node.id) }
      : { label: 'Run coverage for this package', run: () => p.actions.refresh(node.id, true) },
  }
}

function funcSpec(p: Props, node: QNode, m: Metrics): Spec {
  const other = p.context !== undefined && node.parent !== p.context && node.id !== p.context && p.ix.byId.get(p.context)?.kind === 'package'
  const loc = `${node.file}:${node.line}`
  const tag = node.pure === undefined ? undefined : { text: node.pure ? 'λ pure' : 'effectful', pure: node.pure, hover: node.effects?.join('\n') }
  const crapRow: RowSpec = { key: 'crap', label: 'CRAP', value: fmt.crap(m.crap), grade: m.grades?.crap ?? 1, stale: m.coverage?.stale }
  if (other) {
    return {
      kind: 'Function · other package', title: short(node.id), sub: loc, tag,
      rows: [crapRow, coverageRow(m, m.grades?.coverage), mutRows(m)[0]!],
      primary: { label: 'Show code', run: () => p.actions.open(node.id) },
    }
  }
  return {
    kind: node.exported ? 'Function · exported' : 'Function · unexported',
    title: node.name, sub: loc, tag,
    rows: [
      crapRow,
      { key: 'cc', label: 'Cyclomatic (CC)', value: String(m.cc ?? '—') },
      { key: 'cognitive', label: 'Cognitive', value: String(m.cognitive ?? '—') },
      coverageRow(m, m.grades?.coverage),
      ...mutRows(m),
      { key: 'churn', label: 'Churn (file)', value: commits(m.churn) },
    ],
    list: survivorList(p, node, m),
    primary: p.level === 2 && node.id === p.context
      ? { label: 'Re-run coverage for this function', run: () => p.actions.refresh(node.id, true) }
      : { label: 'Show code', run: () => p.actions.open(node.id) },
  }
}

function survivorList(p: Props, node: QNode, m: Metrics): ReactNode {
  const lived = (m.mutation?.mutants ?? []).filter((x) => x.status === 'LIVED')
  if (lived.length === 0) return null
  const byLine = new Map<number, string[]>()
  for (const x of lived) byLine.set(x.line, [...(byLine.get(x.line) ?? []), x.description])
  return (
    <List title="Surviving mutants" items={[...byLine].sort((a, b) => a[0] - b[0]).map(([line, changes]) => ({
      key: String(line), a: `L${line}`, b: changes.join('  ·  '), c: String(changes.length), color: p.palette.yellow,
      go: () => p.actions.open(node.id),
    }))} />
  )
}

function typeSpec(node: QNode): Spec {
  return {
    kind: `Type · ${node.type_kind ?? 'other'}`, title: node.name, sub: `${node.file}:${node.line}`, rows: [],
    list: node.fields && node.fields.length > 0 && (
      <List title="Fields" items={node.fields.map((f) => ({ key: f.name, a: f.name, b: f.type, c: '', color: '', go: undefined }))} />
    ),
  }
}

interface Item { key: string; a: string; b: string; c: string; color: string; go?: () => void }

function List({ title, items }: { title: string; items: Item[] }) {
  return (
    <div className="list">
      <span className="kind">{title}</span>
      {items.map((it) => (
        <button type="button" key={it.key} onClick={it.go} disabled={!it.go}>
          <span className="a">{it.a}</span>
          <span className="b">{it.b}</span>
          <span className="c" style={{ color: it.color || undefined }}>{it.c}</span>
        </button>
      ))}
    </div>
  )
}

function Actions(p: Props & { node: QNode; primary?: Spec['primary'] }) {
  const [message, setMessage] = useState('')
  const [note, setNote] = useState('')
  const hasFile = p.node.kind === 'func' || p.node.kind === 'type'
  return (
    <div className="actions">
      {p.primary && <button type="button" className="primary" disabled={p.busy} onClick={p.primary.run}>{p.primary.label}</button>}
      {hasFile && p.config.editors.length > 0 && (
        <div className="openin">
          <span className="label">Open in</span>
          {p.config.editors.map((e) => (
            <button type="button" key={e.id} onClick={() => p.actions.openIn(e.id)}>{e.label}</button>
          ))}
        </div>
      )}
      {p.level === 2 && p.selectedLine !== undefined && p.node.kind === 'func' && (
        <form onSubmit={(e) => { e.preventDefault(); void p.actions.addNote(note).then((ok) => ok && setNote('')) }} className="actions">
          <label className="kind" htmlFor="note-text">Add note on line {p.selectedLine}</label>
          <textarea id="note-text" className="field" value={note} onChange={(e) => setNote(e.target.value)} placeholder="What should change here?" />
          <button type="submit" className="small-btn" disabled={!note.trim()}>Add note</button>
        </form>
      )}
      <input className="field" style={{ minHeight: 34 }} aria-label="Message for the agent (optional)" placeholder="Message for the agent (optional)"
        value={message} onChange={(e) => setMessage(e.target.value)} />
      <div className="pair">
        <button type="button" onClick={p.actions.copyPrompt}>Copy fix prompt</button>
        <button type="button" className="send" onClick={() => { p.actions.send(message); setMessage('') }}>Send to agent</button>
      </div>
    </div>
  )
}

function commits(n: number | undefined): string {
  if (n === undefined) return 'not measured'
  return n === 1 ? '1 commit' : `${n} commits`
}
