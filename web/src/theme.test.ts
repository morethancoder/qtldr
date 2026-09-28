import { describe, expect, it } from 'vitest'
import file from './themes.json'
import type { ThemeDef } from './api'
import { alpha, contrast, derive, gradeColor, mix, pickInitial } from './theme'

const themes = (file as { themes: ThemeDef[] }).themes

describe('themes.json', () => {
  it('has the 9 built-in themes', () => {
    expect(themes.map((t) => t.id)).toEqual([
      'gruvbox-dark', 'gruvbox-light', 'github-dark', 'github-light', 'tokyo-night',
      'catppuccin-mocha', 'nord', 'dracula', 'solarized-light',
    ])
  })

  // docs/UI.md §8: muted and faint must pass 4.5:1 against bg and surface.
  for (const t of themes) {
    it(`${t.id}: muted and faint pass 4.5:1 on bg and surface`, () => {
      const p = derive(t)
      for (const fg of [p.muted, p.faint]) {
        for (const bg of [p.bg, p.surface]) {
          expect(contrast(fg, bg), `${fg} on ${bg}`).toBeGreaterThanOrEqual(4.5)
        }
      }
    })
  }
})

describe('derive', () => {
  const p = derive(themes[0]!)
  it('builds the 5-step scale with the yellow/green mix', () => {
    expect(p.scale).toEqual([p.red, p.orange, p.yellow, mix(p.yellow, p.green, 0.55), p.green])
  })
  it('maps grades to the scale', () => {
    expect([1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((g) => p.scale.indexOf(gradeColor(p, g)))).toEqual([0, 0, 1, 1, 2, 2, 3, 3, 4, 4])
  })
  it('derives dark/light values', () => {
    expect(p.onAccent).toBe(p.bg)
    expect(derive(themes[1]!).onAccent).toBe('#ffffff')
    expect(p.accentSoft).toBe(alpha(p.accent, 0.16))
  })
})

describe('pickInitial', () => {
  const cfg = { theme: 'nord', theme_light: '', theme_dark: '' }
  it('prefers an explicit choice, then the OS pair, then [ui].theme', () => {
    expect(pickInitial(themes, cfg, true, 'dracula')).toBe('dracula')
    expect(pickInitial(themes, cfg, true, null)).toBe('nord')
    const pair = { theme: 'nord', theme_light: 'github-light', theme_dark: 'github-dark' }
    expect(pickInitial(themes, pair, true, null)).toBe('github-dark')
    expect(pickInitial(themes, pair, false, null)).toBe('github-light')
    expect(pickInitial(themes, { ...cfg, theme: 'gone' }, false, 'also-gone')).toBe('gruvbox-dark')
  })
  it('contrast is symmetric and 21 for black on white', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 5)
    expect(contrast('#ffffff', '#000000')).toBeCloseTo(21, 5)
  })
})
