// Grade → colors for boxes, dots, chips (docs/UI.md §3). Only palette values.

import type { CSSProperties } from 'react'
import type { Metrics } from '../api'
import { gradeOf, type Mode, type Tone } from '../model'
import { alpha, gradeColor, type Palette } from '../theme'

/** cardStyle: fill 16%/13%, border 55%/60%; selected = 2px accent + ring;
 * stale = dashed; no grade = neutral. */
export function cardStyle(p: Palette, grade: number | null, selected: boolean, stale: boolean): CSSProperties {
  const color = grade === null ? null : gradeColor(p, grade)
  const lineStyle = stale ? 'dashed' : 'solid'
  const base: CSSProperties = color
    ? { background: alpha(color, p.tintA), border: `1px ${lineStyle} ${alpha(color, p.strokeA)}` }
    : { background: p.surface2, border: `1px ${lineStyle} ${p.border2}` }
  if (!selected) return base
  return { ...base, border: `2px ${lineStyle} ${p.accent}`, boxShadow: `0 0 0 4px ${p.accentSoft}, ${p.shadow}` }
}

/** boxStyle is the level box: 6%/7% fill, 35% border. */
export function boxStyle(p: Palette, grade: number | null, selected: boolean): CSSProperties {
  const color = grade === null ? p.border2 : gradeColor(p, grade)
  return {
    background: alpha(color, p.boxA),
    border: selected ? `2px solid ${p.accent}` : `1px solid ${alpha(color, 0.35)}`,
  }
}

/** dots: C and M colors; grey when the metric does not apply. */
export function dots(p: Palette, m: Metrics | undefined): { c: string; m: string } {
  const c = gradeOf(m, 'crap')
  const mu = gradeOf(m, 'mutation')
  return { c: c === null ? p.border2 : gradeColor(p, c), m: mu === null ? p.border2 : gradeColor(p, mu) }
}

export function toneColor(p: Palette, tone: Tone | 'red' | 'orange' | 'yellow'): string {
  if (typeof tone === 'object') return gradeColor(p, tone.grade)
  return p[tone]
}

export function chipStyle(p: Palette, tone: Tone): CSSProperties {
  const c = toneColor(p, tone)
  return { background: alpha(c, 0.18), color: c }
}

export function modeGrade(m: Metrics | undefined, mode: Mode): number | null {
  return gradeOf(m, mode)
}
