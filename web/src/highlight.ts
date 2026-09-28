// Go syntax highlighting with Shiki: the core API, the JavaScript regex
// engine (no WASM), only the Go grammar, and only the themes qtldr ships.
// A theme with no Shiki id gets one built from its syntax colors.

import { createHighlighterCore, type HighlighterCore, type ThemeRegistration, type ThemedToken } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
import type { Palette } from './theme'

type ThemeModule = { default: ThemeRegistration }

const bundled: Record<string, () => Promise<ThemeModule>> = {
  'gruvbox-dark-hard': () => import('@shikijs/themes/gruvbox-dark-hard'),
  'gruvbox-light-medium': () => import('@shikijs/themes/gruvbox-light-medium'),
  'github-dark': () => import('@shikijs/themes/github-dark'),
  'github-light': () => import('@shikijs/themes/github-light'),
  'tokyo-night': () => import('@shikijs/themes/tokyo-night'),
  'catppuccin-mocha': () => import('@shikijs/themes/catppuccin-mocha'),
  nord: () => import('@shikijs/themes/nord'),
  dracula: () => import('@shikijs/themes/dracula'),
  'solarized-light': () => import('@shikijs/themes/solarized-light'),
}

let highlighter: Promise<HighlighterCore> | null = null

function get(): Promise<HighlighterCore> {
  highlighter ??= createHighlighterCore({
    themes: [],
    langs: [import('@shikijs/langs/go')],
    engine: createJavaScriptRegexEngine(),
  })
  return highlighter
}

/** fallbackTheme builds a Shiki theme from the palette's syntax keys. */
export function fallbackTheme(p: Palette): ThemeRegistration {
  const s = p.syntax
  const rule = (scope: string[], color: string | undefined) => ({ scope, settings: { foreground: color ?? p.text } })
  return {
    name: `qtldr-${p.id}`,
    type: p.dark ? 'dark' : 'light',
    colors: { 'editor.background': p.surface, 'editor.foreground': s.plain ?? p.text },
    tokenColors: [
      rule(['comment', 'punctuation.definition.comment'], s.com),
      rule(['string', 'string.quoted'], s.str),
      rule(['constant.numeric', 'constant.language'], s.num),
      rule(['keyword', 'storage.type', 'storage.modifier', 'keyword.control'], s.kw),
      rule(['entity.name.function', 'support.function'], s.fn),
      rule(['entity.name.type', 'support.type', 'storage.type.numeric.go', 'storage.type.string.go', 'storage.type.boolean.go'], s.ty),
    ],
  }
}

async function themeName(h: HighlighterCore, p: Palette): Promise<string> {
  const load = p.shiki ? bundled[p.shiki] : undefined
  const name = load ? p.shiki! : `qtldr-${p.id}`
  if (!h.getLoadedThemes().includes(name)) {
    await h.loadTheme(load ? (await load()).default : fallbackTheme(p))
  }
  return name
}

/** tokenize highlights Go code, one token array per line. */
export async function tokenize(code: string, p: Palette): Promise<ThemedToken[][]> {
  const h = await get()
  return h.codeToTokens(code, { lang: 'go', theme: await themeName(h, p) }).tokens
}
