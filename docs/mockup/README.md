# Mockup (reference only)

`explorer.dc.html` is the approved clickable mockup, authored in a design-canvas format:
HTML markup with `{{holes}}`, `<sc-for>`/`<sc-if>` template tags, and a `class Component extends DCLogic`
script whose `renderVals()` computes everything. It does not run on its own in a browser.

Use it for: exact theme colors, spacing, font sizes, card contents, inspector rows, tooltip text,
source-view annotations, and the sample `internal/pricing` / `applyTiered` data (mirror it in
`testdata/ledger`). Do not port its code: the real app uses React Flow + ELK for layout and real data.
