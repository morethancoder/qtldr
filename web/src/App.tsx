import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  api, subscribe, type AgentStatus, type Config, type Detail, type Glossary, type Snapshot, type Source, type ThemeDef,
} from './api'
import { Footer, Search, Toasts, TopBar, type Crumb, type Toast } from './components/Chrome'
import { FunctionView } from './components/FunctionView'
import { Inspector, type InspectorActions } from './components/Inspector'
import { isTyping, MapCanvas, type Rect } from './components/MapCanvas'
import { useTips } from './components/Tooltip'
import { nearest, type Direction } from './layout'
import { index, metricsOf, packageOf, searchItems, short, worstPackage, type Index, type Mode } from './model'
import { formatRoute, parseRoute, type Route } from './route'
import { applyVars, derive, pickInitial } from './theme'
import bundled from './themes.json'

const builtin = (bundled as { themes: ThemeDef[] }).themes
const THEME_KEY = 'qtldr.theme'

function stored(): string | null {
  try {
    return localStorage.getItem(THEME_KEY)
  } catch {
    return null
  }
}

/** resolve fixes a route whose nodes vanished: up to the nearest parent. */
function resolve(ix: Index, r: Route): Route {
  if (r.level === 2 && !ix.byId.has(r.fn)) {
    const pkg = packageOfId(ix, r.fn)
    return pkg ? { level: 1, pkg } : { level: 0 }
  }
  if (r.level === 1 && ix.byId.get(r.pkg)?.kind !== 'package') return { level: 0 }
  return r
}

function packageOfId(ix: Index, id: string): string | undefined {
  for (let cut = id.lastIndexOf('.'); cut > 0; cut = id.lastIndexOf('.', cut - 1)) {
    const cand = id.slice(0, cut)
    if (ix.byId.get(cand)?.kind === 'package') return cand
  }
  return undefined
}

function defaultSelection(ix: Index, r: Route): string | undefined {
  if (r.level === 2) return r.fn
  if (r.level === 1) {
    const worst = metricsOf(ix, r.pkg).worst
    return worst && ix.byId.has(worst) ? worst : r.pkg
  }
  return worstPackage(ix)
}

const levelNames = ['module', 'package', 'function'] as const

export function App() {
  const [snap, setSnap] = useState<Snapshot | null>(null)
  const [cfg, setCfg] = useState<Config | null>(null)
  const [glossary, setGlossary] = useState<Glossary>({})
  const [themes, setThemes] = useState<ThemeDef[]>(builtin)
  const [themeId, setThemeId] = useState<string>(() => stored() ?? 'gruvbox-dark')
  const explicit = useRef<string | null>(null)
  const [mode, setMode] = useState<Mode>('combined')
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.hash))
  const [expanded, setExpanded] = useState(false)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [source, setSource] = useState<Source | null>(null)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [progress, setProgress] = useState<string | null>(null)
  const [searching, setSearching] = useState(false)
  const [agent, setAgent] = useState<AgentStatus>({ connected: false })
  const [error, setError] = useState<string | null>(null)
  const [notesTick, setNotesTick] = useState(0)
  const tips = useTips()
  const footerTips = useTips()
  const positions = useRef(new Map<string, Rect>())

  const theme = themes.find((t) => t.id === themeId) ?? themes[0]!
  const palette = useMemo(() => derive(theme), [theme])
  useEffect(() => applyVars(palette), [palette])

  const toast = useCallback((text: string) => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t.slice(-2), { id, text }])
    window.setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4500)
  }, [])

  // Initial load.
  useEffect(() => {
    Promise.all([api.snapshot(), api.config(), api.glossary(), api.themes()])
      .then(([s, c, g, t]) => {
        setSnap(s)
        setCfg(c)
        setGlossary(g)
        setThemes(t.themes)
        setAgent(c.agent)
        const dark = window.matchMedia('(prefers-color-scheme: dark)').matches
        setThemeId(pickInitial(t.themes, c, dark, explicit.current))
        for (const w of t.warnings ?? []) toast(`Theme skipped: ${w}`)
      })
      .catch((e: unknown) => setError(String(e)))
  }, [toast])

  // Mirror the theme for fast first paint; follow the OS pair when set.
  useEffect(() => {
    try {
      localStorage.setItem(THEME_KEY, themeId)
    } catch {
      // storage may be unavailable
    }
  }, [themeId])
  useEffect(() => {
    if (!cfg?.theme_light || !cfg.theme_dark) return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const on = () => explicit.current === null && setThemeId(pickInitial(themes, cfg, mq.matches, null))
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [cfg, themes])

  // Live updates.
  useEffect(() => subscribe((e) => {
    switch (e.name) {
      case 'snapshot': void api.snapshot().then(setSnap); break
      case 'progress': setProgress(e.data.done ? null : e.data.message); break
      case 'toast': toast(e.data.message); break
      case 'notes': setNotesTick((t) => t + 1); break
      case 'agent': setAgent(e.data); break
    }
  }), [toast])

  // Back button and links.
  useEffect(() => {
    const on = () => setRoute(parseRoute(window.location.hash))
    window.addEventListener('popstate', on)
    window.addEventListener('hashchange', on)
    return () => {
      window.removeEventListener('popstate', on)
      window.removeEventListener('hashchange', on)
    }
  }, [])

  const ix = useMemo(() => (snap ? index(snap) : null), [snap])
  const view = ix ? resolve(ix, route) : route
  const sel = ix && view.sel && ix.byId.has(view.sel) ? view.sel : ix ? defaultSelection(ix, view) : undefined
  const line = view.level === 2 ? view.line : undefined
  const context = view.level === 1 ? view.pkg : view.level === 2 ? view.fn : undefined

  const navigate = useCallback((r: Route, push: boolean) => {
    const h = formatRoute(r)
    if (push) window.history.pushState(null, '', h)
    else window.history.replaceState(null, '', h)
    setRoute(r)
  }, [])

  const select = useCallback((id: string) => navigate({ ...view, sel: id } as Route, false), [navigate, view])

  const open = useCallback((id: string) => {
    if (!ix) return
    if (id === 'more') return setExpanded(true)
    const n = ix.byId.get(id)
    if (!n) return
    if (n.kind === 'package') navigate({ level: 1, pkg: id }, true)
    else if (n.kind === 'func') navigate({ level: 2, fn: id }, true)
    else if (n.kind === 'external') toast('External modules are not analyzed; they are shown for context.')
    else if (n.kind === 'type') toast('The type view (fields, usages, implemented interfaces) comes in a later version.')
  }, [ix, navigate, toast])

  const up = useCallback(() => {
    if (!ix) return
    if (view.level === 2) {
      const pkg = packageOf(ix, view.fn)
      navigate(pkg ? { level: 1, pkg, sel: view.fn } : { level: 0 }, true)
    } else if (view.level === 1) {
      navigate({ level: 0, sel: view.pkg }, true)
    }
    setExpanded(false)
  }, [ix, view, navigate])

  // Selection → inspector detail, focus.json (debounced 300 ms).
  useEffect(() => {
    if (!sel) return
    let live = true
    api.node(sel).then((d) => live && setDetail(d)).catch(() => live && setDetail(null))
    return () => { live = false }
  }, [sel, snap, notesTick])
  useEffect(() => {
    if (!sel) return
    const t = window.setTimeout(() => {
      void api.focus({ id: sel, level: levelNames[view.level], selected_line: line, sent: false }).catch(() => undefined)
    }, 300)
    return () => window.clearTimeout(t)
  }, [sel, view.level, line])
  useEffect(() => {
    if (view.level !== 2) return setSource(null)
    let live = true
    api.source(view.fn).then((s) => live && setSource(s)).catch((e: unknown) => live && toast(String(e)))
    return () => { live = false }
  }, [view.level, view.level === 2 ? view.fn : '', snap, notesTick, toast])

  const chooseTheme = useCallback((id: string) => {
    explicit.current = id
    setThemeId(id)
    api.saveTheme(id).catch((e: unknown) => toast(`Theme not saved: ${String(e)}`))
  }, [toast])

  const actions: InspectorActions = {
    select,
    open,
    copyPrompt: () => {
      if (!sel) return
      api.prompt(sel)
        .then((r) => navigator.clipboard.writeText(r.prompt).then(
          () => toast(`Copied: “${r.prompt.slice(0, 130)}${r.prompt.length > 130 ? '…' : ''}”`),
          () => toast('The browser blocked the clipboard; use Send to agent instead.'),
        ))
        .catch((e: unknown) => toast(String(e)))
    },
    send: (message) => {
      if (!sel) return
      api.focus({ id: sel, level: levelNames[view.level], selected_line: line, message, sent: true })
        .then((r) => toast(`Sent. Ask your agent to fix what you're looking at.${r.tmux ? ' The prompt was also typed into tmux.' : ''}${r.tmux_error ? ` tmux: ${r.tmux_error}` : ''}`))
        .catch((e: unknown) => toast(String(e)))
    },
    openIn: (editor) => {
      if (!sel) return
      api.open(sel, line, editor).then((r) => toast(`Ran: ${r.command.join(' ')}`)).catch((e: unknown) => toast(String(e)))
    },
    refresh: (id, coverage, mutation = false) => {
      api.refresh(id, coverage, mutation)
        .then(() => toast(mutation ? 'Running mutation testing… only this package is re-tested.' : coverage ? 'Running coverage…' : 'Refreshing…'))
        .catch((e: unknown) => toast(String(e)))
    },
    addNote: async (text) => {
      if (view.level !== 2 || line === undefined) return false
      try {
        await api.addNote(view.fn, line, text)
        toast('Note added.')
        return true
      } catch (e) {
        toast(String(e))
        return false
      }
    },
  }

  // Keyboard (docs/UI.md §10).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (tips.close() || footerTips.close()) return
        if (searching) return setSearching(false)
        return up()
      }
      if (isTyping(e.target) || e.metaKey || e.ctrlKey || e.altKey) return
      const dirs: Record<string, Direction> = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }
      const dir = dirs[e.key]
      if (dir && sel) {
        const next = nearest(positions.current, sel, dir)
        if (next) {
          e.preventDefault()
          select(next)
          document.querySelector<HTMLElement>(`[data-qt-id="${CSS.escape(next)}"]`)?.focus()
        }
        return
      }
      const keys: Record<string, () => void> = {
        '/': () => setSearching(true),
        t: () => chooseTheme(themes[(themes.findIndex((x) => x.id === themeId) + 1) % themes.length]!.id),
        c: actions.copyPrompt,
        e: () => cfg?.editors[0] && actions.openIn(cfg.editors[0].id),
        o: () => sel && open(sel),
      }
      const run = keys[e.key]
      if (run && !(e.target instanceof HTMLButtonElement && e.key === 'o')) {
        e.preventDefault()
        run()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  if (error) return <div className="empty">Could not load qtldr data: {error}. Is `qtldr serve` still running?</div>
  if (!ix || !snap || !cfg) return <div className="empty">Loading…</div>

  const moduleName = ix.module?.name ?? snap.module
  const crumbs: Crumb[] = [{ label: moduleName, go: view.level > 0 ? () => navigate({ level: 0 }, true) : undefined }]
  const pkgId = view.level === 1 ? view.pkg : view.level === 2 ? packageOf(ix, view.fn) : undefined
  if (pkgId) crumbs.push({ label: ix.byId.get(pkgId)?.name ?? pkgId, go: view.level > 1 ? () => navigate({ level: 1, pkg: pkgId, sel: context }, true) : undefined })
  if (view.level === 2) crumbs.push({ label: ix.byId.get(view.fn)?.name ?? short(view.fn) })
  const caption = view.level === 0
    ? `${snap.module} · packages · arrows show imports`
    : view.level === 1 ? `Inside ${ix.byId.get(view.pkg)?.name} · arrows show calls` : 'Called by (left) and calls (right)'
  const onPositions = (r: Map<string, Rect>) => { positions.current = r }

  return (
    <div className="app">
      <TopBar crumbs={crumbs} mode={mode} onMode={setMode} themes={themes} themeId={theme.id} onTheme={chooseTheme} agent={agent} />
      {progress !== null && <div className="progress" role="progressbar" aria-label={progress}><div className="bar" /></div>}
      <div className="body">
        <div className="main">
          <div className="canvas">
            <div className="caption">{caption}</div>
            {view.level > 0 && <button type="button" className="up" onClick={up}>Up one level</button>}
            <div className="level" key={`${view.level}:${context ?? ''}`}>
              {view.level === 2 ? (
                <FunctionView
                  ix={ix} fn={view.fn} mode={mode} palette={palette} sel={sel} source={source} selectedLine={line} scrollTo={line}
                  onSelect={select} onOpen={open} onPositions={onPositions}
                  onSelectLine={(n) => navigate({ ...view, line: n === line ? undefined : n }, false)}
                  onResolveNote={(id) => void api.resolveNote(id).then(() => toast('Note resolved.')).catch((e: unknown) => toast(String(e)))}
                />
              ) : (
                <MapCanvas
                  ix={ix} level={view.level} pkg={view.level === 1 ? view.pkg : undefined} mode={mode} palette={palette}
                  th={cfg.thresholds} sel={sel} expanded={expanded} onSelect={select} onOpen={open} onPositions={onPositions}
                />
              )}
            </div>
            {progress && <div className="progress-label">{progress}</div>}
            {searching && (
              <Search
                items={searchItems(ix)}
                onClose={() => setSearching(false)}
                onPick={(it) => {
                  setSearching(false)
                  if (it.kind === 'package') navigate({ level: 1, pkg: it.id }, true)
                  else {
                    const pkg = packageOf(ix, it.id)
                    navigate(pkg ? { level: 1, pkg, sel: it.id } : { level: 2, fn: it.id }, true)
                  }
                }}
              />
            )}
            <Toasts toasts={toasts} />
          </div>
          <Footer palette={palette} glossary={glossary} tips={footerTips} />
        </div>
        <Inspector
          ix={ix} id={sel} detail={detail} level={view.level} context={context} palette={palette} glossary={glossary}
          config={cfg} selectedLine={line} busy={progress !== null} tips={tips} actions={actions}
        />
      </div>
    </div>
  )
}
