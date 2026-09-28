// Top bar, footer, toasts and search (docs/UI.md §2, §9, §10).

import { useEffect, useRef, useState } from 'react'
import type { AgentStatus, Glossary, ThemeDef } from '../api'
import { fuzzy, type Mode, type SearchItem } from '../model'
import type { Palette } from '../theme'
import { Help, type TipState } from './Tooltip'

export interface Crumb { label: string; go?: () => void }

const modes: [Mode, string][] = [['combined', 'Combined'], ['crap', 'CRAP'], ['coverage', 'Coverage'], ['mutation', 'Mutation']]

interface TopBarProps {
  crumbs: Crumb[]
  mode: Mode
  onMode: (m: Mode) => void
  themes: ThemeDef[]
  themeId: string
  onTheme: (id: string) => void
  agent: AgentStatus
}

export function TopBar(p: TopBarProps) {
  const groups: [string, ThemeDef[]][] = [
    ['Dark', [...p.themes.filter((t) => t.dark && !t.custom), ...p.themes.filter((t) => t.dark && t.custom)]],
    ['Light', [...p.themes.filter((t) => !t.dark && !t.custom), ...p.themes.filter((t) => !t.dark && t.custom)]],
  ]
  return (
    <header className="topbar">
      <div className="logo">
        <svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M5 6 L12 12 L19 7 M12 12 L12 19" />
          <circle cx="5" cy="6" r="2.6" /><circle cx="19" cy="7" r="2.6" /><circle cx="12" cy="12" r="2.6" /><circle cx="12" cy="19" r="2.6" />
        </svg>
        <span>qtldr</span>
      </div>
      <span className="vsep" />
      <nav className="crumbs" aria-label="Location">
        {p.crumbs.map((c, i) => (
          <span key={i} style={{ display: 'contents' }}>
            {i > 0 && <span className="faint">/</span>}
            {c.go ? <button type="button" onClick={c.go}>{c.label}</button> : <span className="here" aria-current="location">{c.label}</span>}
          </span>
        ))}
      </nav>
      <span className="grow" />
      <span className="label">Color by</span>
      <div className="segmented" role="group" aria-label="Color boxes by">
        {modes.map(([m, label]) => (
          <button type="button" key={m} aria-pressed={p.mode === m} onClick={() => p.onMode(m)}>{label}</button>
        ))}
      </div>
      <label className="label" htmlFor="theme-pick">Theme</label>
      <select id="theme-pick" className="select" value={p.themeId} onChange={(e) => p.onTheme(e.target.value)}>
        {groups.map(([label, list]) => list.length > 0 && (
          <optgroup label={label} key={label}>
            {list.map((t) => <option key={t.id} value={t.id}>{t.name}{t.custom ? ' (custom)' : ''}</option>)}
          </optgroup>
        ))}
      </select>
      <div className="agent" title={p.agent.last_seen ? `Last seen ${new Date(p.agent.last_seen).toLocaleTimeString()}` : 'No MCP client has used qtldr in the last 60 seconds'}>
        <span className={p.agent.connected ? 'dot on' : 'dot'} />
        <span>{p.agent.connected ? `${prettyClient(p.agent.client)} · MCP` : 'No agent'}</span>
      </div>
    </header>
  )
}

function prettyClient(c: string | undefined): string {
  if (!c) return 'Agent'
  return c === 'claude-code' ? 'Claude Code' : c
}

export function Footer({ palette, glossary, tips }: { palette: Palette; glossary: Glossary; tips: TipState }) {
  return (
    <footer className="footer">
      <span>Worse</span>
      <div className="swatches" aria-hidden="true">{palette.scale.map((c) => <i key={c} style={{ background: c }} />)}</div>
      <span>Better</span>
      <span className="faint">·</span>
      <span><span className="lambda">λ</span> pure · C = CRAP · M = mutation</span>
      <span onMouseEnter={() => tips.enter('grade')} onMouseLeave={() => tips.leave()} style={{ display: 'inline-flex' }}>
        <Help tipKey="grade" term={glossary.grade} tips={tips} label="the box colors" up />
      </span>
      <span className="grow" />
      <span>Click to inspect · double-click to open · / to search</span>
    </footer>
  )
}

export interface Toast { id: number; text: string }

export function Toasts({ toasts }: { toasts: Toast[] }) {
  return (
    <div className="toasts" role="status" aria-live="polite">
      {toasts.map((t) => <div className="toast" key={t.id}>{t.text}</div>)}
    </div>
  )
}

export function Search({ items, onPick, onClose }: { items: SearchItem[]; onPick: (it: SearchItem) => void; onClose: () => void }) {
  const [q, setQ] = useState('')
  const [i, setI] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const found = fuzzy(q, items)
  useEffect(() => input.current?.focus(), [])
  return (
    <div className="search" role="dialog" aria-label="Jump to a package or function">
      <input
        ref={input}
        value={q}
        placeholder="Jump to a package or function…"
        aria-label="Search"
        onChange={(e) => { setQ(e.target.value); setI(0) }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') { e.stopPropagation(); onClose() }
          else if (e.key === 'ArrowDown') { e.preventDefault(); setI((v) => Math.min(v + 1, found.length - 1)) }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setI((v) => Math.max(v - 1, 0)) }
          else if (e.key === 'Enter' && found[i]) onPick(found[i])
        }}
      />
      {found.length > 0 && (
        <ul role="listbox">
          {found.map((it, j) => (
            <li key={it.id}>
              <button type="button" role="option" aria-selected={j === i} onClick={() => onPick(it)}>
                <span className="faint">{it.kind === 'package' ? 'pkg' : 'func'}</span>{it.label}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
