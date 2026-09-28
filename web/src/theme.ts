// Theme resolution: one theme drives the chrome, the map and the code. All
// colors come from themes.json; derived values follow its "_doc.derived".

import type { ThemeDef } from './api'

export interface Palette {
  id: string
  dark: boolean
  bg: string
  surface: string
  surface2: string
  border: string
  border2: string
  text: string
  muted: string
  faint: string
  accent: string
  red: string
  orange: string
  yellow: string
  green: string
  /** grade scale for grades 1–2, 3–4, 5–6, 7–8, 9–10 */
  scale: [string, string, string, string, string]
  onAccent: string
  accentSoft: string
  shadow: string
  dotText: string
  tintA: number
  strokeA: number
  boxA: number
  syntax: Record<string, string>
  shiki?: string
}

function channel(hex: string, i: number): number {
  return parseInt(hex.slice(1 + i * 2, 3 + i * 2), 16)
}

/** alpha returns hex as rgba with alpha a. */
export function alpha(hex: string, a: number): string {
  return `rgba(${channel(hex, 0)},${channel(hex, 1)},${channel(hex, 2)},${a})`
}

/** mix blends a toward b by t (0–1). */
export function mix(a: string, b: string, t: number): string {
  let out = '#'
  for (let i = 0; i < 3; i++) {
    const v = Math.round(channel(a, i) + (channel(b, i) - channel(a, i)) * t)
    out += v.toString(16).padStart(2, '0')
  }
  return out
}

function need(m: Record<string, string>, k: string): string {
  const v = m[k]
  if (!v) throw new Error(`theme is missing ${k}`)
  return v
}

export function derive(t: ThemeDef): Palette {
  const ui = (k: string) => need(t.ui, k)
  const gr = (k: string) => need(t.grade, k)
  const yellow = gr('yellow')
  const green = gr('green')
  return {
    id: t.id,
    dark: t.dark,
    bg: ui('bg'),
    surface: ui('surface'),
    surface2: ui('surface2'),
    border: ui('border'),
    border2: ui('border2'),
    text: ui('text'),
    muted: ui('muted'),
    faint: ui('faint'),
    accent: ui('accent'),
    red: gr('red'),
    orange: gr('orange'),
    yellow,
    green,
    scale: [gr('red'), gr('orange'), yellow, mix(yellow, green, 0.55), green],
    onAccent: t.dark ? ui('bg') : '#ffffff',
    accentSoft: alpha(ui('accent'), t.dark ? 0.16 : 0.12),
    shadow: t.dark ? '0 10px 30px rgba(0,0,0,0.45)' : '0 8px 24px rgba(30,30,20,0.12)',
    dotText: ui('surface'),
    tintA: t.dark ? 0.16 : 0.13,
    strokeA: t.dark ? 0.55 : 0.6,
    boxA: t.dark ? 0.06 : 0.07,
    syntax: t.syntax,
    shiki: t.shiki,
  }
}

/** gradeColor maps a 1–10 grade to the theme's 5-step scale. */
export function gradeColor(p: Palette, grade: number): string {
  const i = grade >= 9 ? 4 : grade >= 7 ? 3 : grade >= 5 ? 2 : grade >= 3 ? 1 : 0
  return p.scale[i] ?? p.red
}

/** applyVars exposes the palette as CSS custom properties. */
export function applyVars(p: Palette, el: HTMLElement = document.documentElement): void {
  const vars: Record<string, string> = {
    bg: p.bg, surface: p.surface, surface2: p.surface2, border: p.border, border2: p.border2,
    text: p.text, muted: p.muted, faint: p.faint, accent: p.accent, 'accent-soft': p.accentSoft,
    'on-accent': p.onAccent, red: p.red, orange: p.orange, yellow: p.yellow, green: p.green,
    shadow: p.shadow, 'dot-text': p.dotText,
  }
  for (const [k, v] of Object.entries(vars)) el.style.setProperty(`--qt-${k}`, v)
  el.style.colorScheme = p.dark ? 'dark' : 'light'
}

function luminance(hex: string): number {
  const lin = (c: number) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * lin(channel(hex, 0)) + 0.7152 * lin(channel(hex, 1)) + 0.0722 * lin(channel(hex, 2))
}

/** contrast is the WCAG contrast ratio of two colors. */
export function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((m, n) => n - m) as [number, number]
  return (x + 0.05) / (y + 0.05)
}

/** pickInitial chooses the theme: explicit choice this session, else the
 * OS light/dark pair when configured, else [ui].theme. */
export function pickInitial(
  themes: ThemeDef[],
  cfg: { theme: string; theme_light: string; theme_dark: string },
  prefersDark: boolean,
  explicit: string | null,
): string {
  const has = (id: string) => themes.some((t) => t.id === id)
  if (explicit && has(explicit)) return explicit
  if (cfg.theme_light && cfg.theme_dark) {
    const id = prefersDark ? cfg.theme_dark : cfg.theme_light
    if (has(id)) return id
  }
  if (has(cfg.theme)) return cfg.theme
  return themes[0]?.id ?? 'gruvbox-dark'
}
