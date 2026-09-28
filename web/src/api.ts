// Types mirror the Go JSON (internal/model, internal/server). Keep in sync.

export type Kind = 'module' | 'package' | 'func' | 'type' | 'external'
export type EdgeKind = 'imports' | 'calls' | 'calls_dynamic' | 'implements'

export interface Field { name: string; type: string }

export interface QNode {
  id: string
  kind: Kind
  name: string
  parent?: string
  dir?: string
  file?: string
  line?: number
  end_line?: number
  doc_line?: number
  exported?: boolean
  recv?: string
  signature?: string
  type_kind?: string
  fields?: Field[]
  pure?: boolean
  effects?: string[]
  errors?: string[]
}

export interface QEdge { from: string; to: string; kind: EdgeKind }

export interface LineStates { covered: number[]; uncovered: number[]; partial: number[] }

export interface Coverage {
  stmts: number
  covered: number
  percent: number | null
  stale: boolean
  lines?: LineStates
}

export interface Mutant { line: number; col: number; type: string; status: string; description: string }

export interface Mutation {
  killed: number
  survived: number
  not_covered: number
  timed_out: number
  score: number | null
  stale: boolean
  mutants?: Mutant[]
}

export interface Grades { crap: number; mutation: number | null; coverage: number; combined: number }

export interface Metrics {
  cc?: number
  cognitive?: number
  loc?: number
  churn?: number
  churn_scope?: string
  coverage?: Coverage
  crap?: number
  mutation?: Mutation
  grades?: Grades
  crap_max?: number
  crap_avg?: number
  worst?: string
  coverage_error?: string
  mutation_error?: string
}

export interface Snapshot {
  schema: number
  module: string
  generated: string
  runs: { structure: string | null; coverage: string | null; mutation: string | null }
  nodes: QNode[]
  edges: QEdge[]
  metrics: Record<string, Metrics>
}

export interface PlacedNote {
  id: string
  target: string
  author: string
  text: string
  created: string
  resolved: boolean
  line?: number
  outdated: boolean
}

export interface Detail {
  node: QNode
  metrics?: Metrics
  children?: string[]
  callers?: string[]
  callees?: string[]
  imports?: string[]
  imported_by?: string[]
  notes: PlacedNote[]
}

export interface Annotation {
  kind: 'survived' | 'not_covered' | 'note'
  title: string
  detail?: string
  why?: string
  note_id?: string
  author?: string
  created?: string
  outdated?: boolean
}

export interface SourceLine { n: number; text: string; state?: 'covered' | 'uncovered' | 'partial'; survived?: number; annotations?: Annotation[] }

export interface Source {
  id: string
  file: string
  from: number
  to: number
  coverage_stale: boolean
  mutation_stale: boolean
  lines: SourceLine[]
}

export interface ThemeDef {
  id: string
  name: string
  dark: boolean
  shiki?: string
  ui: Record<string, string>
  grade: Record<string, string>
  syntax: Record<string, string>
  custom?: boolean
}

export interface Term { key: string; title: string; short: string; body: string; good: string; links?: { text: string; url: string }[] }
export type Glossary = Record<string, Term>

export interface EditorPreset { id: string; label: string; bin: string }

export interface Thresholds { crap_max: number; cognitive_max: number; coverage_min: number; mutation_min: number }

export interface AgentStatus { connected: boolean; client?: string; last_seen?: string }

export interface Config {
  theme: string
  theme_light: string
  theme_dark: string
  editors: EditorPreset[]
  thresholds: Thresholds
  tmux: boolean
  agent: AgentStatus
}

function token(): string {
  return document.querySelector<HTMLMetaElement>('meta[name="qtldr-token"]')?.content ?? ''
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: { 'Content-Type': 'application/json', 'X-Qtldr-Token': token() } }
  if (body !== undefined) init.body = JSON.stringify(body)
  const resp = await fetch(path, init)
  const data: unknown = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    const msg = typeof data === 'object' && data !== null && 'error' in data ? String((data as { error: unknown }).error) : resp.statusText
    throw new Error(msg)
  }
  return data as T
}

const q = encodeURIComponent

export const api = {
  snapshot: () => request<Snapshot>('GET', '/api/snapshot'),
  config: () => request<Config>('GET', '/api/config'),
  glossary: () => request<Glossary>('GET', '/api/glossary'),
  themes: () => request<{ themes: ThemeDef[]; warnings: string[] | null }>('GET', '/api/themes'),
  node: (id: string) => request<Detail>('GET', `/api/node?id=${q(id)}`),
  source: (id: string) => request<Source>('GET', `/api/source?id=${q(id)}`),
  prompt: (id: string, message = '') => request<{ prompt: string }>('GET', `/api/prompt?id=${q(id)}&message=${q(message)}`),
  focus: (body: { id: string; level: string; selected_line?: number; message?: string; sent: boolean }) =>
    request<{ tmux?: boolean; tmux_error?: string }>('POST', '/api/focus', body),
  open: (id: string, line?: number, editor?: string) => request<{ command: string[] }>('POST', '/api/open', { id, line, editor }),
  addNote: (target: string, line: number, text: string) => request<unknown>('POST', '/api/notes', { target, line, text }),
  resolveNote: (id: string) => request<unknown>('PATCH', `/api/notes/${q(id)}`, { resolved: true }),
  refresh: (id: string, coverage: boolean, mutation = false) => request<unknown>('POST', '/api/refresh', { id, coverage, mutation }),
  saveTheme: (theme: string) => request<unknown>('POST', '/api/theme', { theme }),
}

export type ServerEvent =
  | { name: 'snapshot'; data: { generated: string } }
  | { name: 'stale'; data: { ids: string[] } }
  | { name: 'progress'; data: { message: string; done: boolean } }
  | { name: 'toast'; data: { message: string } }
  | { name: 'notes'; data: { target: string } }
  | { name: 'agent'; data: AgentStatus }

/** subscribe opens /api/events; it reconnects on its own (EventSource). */
export function subscribe(on: (e: ServerEvent) => void): () => void {
  const es = new EventSource('/api/events')
  const names: ServerEvent['name'][] = ['snapshot', 'stale', 'progress', 'toast', 'notes', 'agent']
  for (const name of names) {
    es.addEventListener(name, (ev) => {
      try {
        on({ name, data: JSON.parse((ev as MessageEvent<string>).data) } as ServerEvent)
      } catch {
        // ignore malformed events
      }
    })
  }
  return () => es.close()
}
