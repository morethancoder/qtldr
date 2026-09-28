// Glossary tooltips (docs/UI.md §7): open on hover or keyboard focus after
// 150 ms, pin by click, close on Esc, outside click or ×.

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import type { Term } from '../api'

export interface TipState {
  open: string | null
  pinned: string | null
  enter: (key: string) => void
  leave: () => void
  toggle: (key: string) => void
  close: () => boolean
}

/** useTips manages one open tooltip at a time for a panel. */
export function useTips(): TipState {
  const [hover, setHover] = useState<string | null>(null)
  const [pinned, setPinned] = useState<string | null>(null)
  const timer = useRef<number | undefined>(undefined)
  const enter = useCallback((key: string) => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setHover(key), 150)
  }, [])
  const leave = useCallback(() => {
    window.clearTimeout(timer.current)
    setHover(null)
  }, [])
  const toggle = useCallback((key: string) => setPinned((p) => (p === key ? null : key)), [])
  const pinnedRef = useRef(pinned)
  const hoverRef = useRef(hover)
  pinnedRef.current = pinned
  hoverRef.current = hover
  const close = useCallback(() => {
    const had = pinnedRef.current !== null || hoverRef.current !== null
    setPinned(null)
    setHover(null)
    return had
  }, [])
  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      if (!(e.target instanceof Element) || !e.target.closest('[data-tip]')) setPinned(null)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [])
  return { open: pinned ?? hover, pinned, enter, leave, toggle, close }
}

interface Props {
  tipKey: string
  term: Term | undefined
  tips: TipState
  label: string
  up?: boolean
}

/** Help is the "?" button plus its tooltip. The tooltip is portaled and
 * fixed-positioned: to the left of the inspector (never over its values), or
 * above the button for the footer (up). */
export function Help({ tipKey, term, tips, label, up }: Props) {
  const open = tips.open === tipKey && term !== undefined
  const pinned = tips.pinned === tipKey
  const btn = useRef<HTMLButtonElement>(null)
  const [pos, setPos] = useState<CSSProperties | null>(null)
  useLayoutEffect(() => {
    if (!open || !btn.current) return setPos(null)
    const r = btn.current.getBoundingClientRect()
    if (up) {
      setPos({ left: Math.max(8, r.left - 8), bottom: window.innerHeight - r.top + 8 })
      return
    }
    const panel = btn.current.closest('.inspector')?.getBoundingClientRect()
    const right = window.innerWidth - (panel?.left ?? r.left) + 10
    setPos({ right, top: Math.max(8, Math.min(r.top - 12, window.innerHeight - 260)) })
  }, [open, up])
  return (
    <span data-tip style={{ display: 'inline-flex' }}>
      <button
        ref={btn}
        type="button"
        className="q"
        aria-label={`What does ${label} mean?`}
        aria-expanded={open}
        onClick={() => tips.toggle(tipKey)}
        onFocus={() => tips.enter(tipKey)}
        onBlur={() => !pinned && tips.leave()}
      >
        ?
      </button>
      {open && term && pos && createPortal(
        <div className="tip" data-tip style={pos} role={pinned ? 'dialog' : 'tooltip'} aria-label={term.title}>
          <div className="tip-card">
            <div className="t">
              <span>{term.title}</span>
              {pinned && <button type="button" className="x" aria-label="Close" onClick={() => tips.close()}>×</button>}
            </div>
            <div className="b">{term.body}</div>
            <div className="good"><b>Good: </b>{term.good}</div>
            {term.links?.map((l) => (
              <a key={l.url} href={l.url} target="_blank" rel="noopener noreferrer">{l.text} ↗</a>
            ))}
          </div>
        </div>,
        document.body,
      )}
    </span>
  )
}
