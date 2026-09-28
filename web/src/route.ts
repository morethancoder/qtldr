// The URL hash holds the level and selection so reloads and links work:
//   #/            module level        #/?sel=<id>
//   #/pkg/<id>    package level       #/pkg/<id>?sel=<id>
//   #/func/<id>   function level      #/func/<id>?sel=<id>&line=<n>

export type Route =
  | { level: 0; sel?: string }
  | { level: 1; pkg: string; sel?: string }
  | { level: 2; fn: string; sel?: string; line?: number }

export function parseRoute(hash: string): Route {
  const [path = '', query = ''] = hash.replace(/^#/, '').split('?')
  const params = new URLSearchParams(query)
  const sel = params.get('sel') ?? undefined
  const parts = path.split('/').filter(Boolean)
  const id = parts.length > 1 ? decodeURIComponent(parts.slice(1).join('/')) : ''
  if (parts[0] === 'pkg' && id) return { level: 1, pkg: id, sel }
  if (parts[0] === 'func' && id) {
    const line = Number(params.get('line'))
    return { level: 2, fn: id, sel, line: Number.isInteger(line) && line > 0 ? line : undefined }
  }
  return { level: 0, sel }
}

export function formatRoute(r: Route): string {
  const params = new URLSearchParams()
  if (r.sel) params.set('sel', r.sel)
  if (r.level === 2 && r.line) params.set('line', String(r.line))
  const q = params.toString() ? `?${params}` : ''
  if (r.level === 1) return `#/pkg/${encodeURIComponent(r.pkg)}${q}`
  if (r.level === 2) return `#/func/${encodeURIComponent(r.fn)}${q}`
  return `#/${q}`
}
