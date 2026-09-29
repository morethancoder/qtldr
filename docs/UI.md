# qtldr — UI specification

This describes the approved design. The mockup source is in `docs/mockup/` (a Claude Design canvas file: plain HTML with a small template runtime). Use it as a **visual reference**; do not port its code. Its sample data is invented, and its layout uses hand-placed coordinates that the real app replaces with ELK.

## 1. Principles
1. **One level at a time.** The map is never the whole codebase at once. You see one level, flat on one screen, and drill in. Like uml-viewer.
2. **Quiet by default.** A card shows only its name, the λ mark, two score dots, and problem lines when something is wrong. Everything else lives in the inspector.
3. **Color means risk, and the theme owns the colors.** All colors come from the active theme (`web/src/themes.json`), including the red→green grade scale and code highlighting.
4. **Every technical term is explainable.** Any metric label has a `?` that explains it, from `internal/glossary/terms.yaml`.
5. **Everything you can point at, an agent can see.** Selection is written to `.qtldr/focus.json`.

## 2. Layout (desktop, min 1280 × 720)

```
┌ top bar 56px ─────────────────────────────────────────────────────────────────────────────┐
│ ◆ qtldr │ ledger / internal/pricing / applyTiered      Color by [Combined|CRAP|Cov|Mut]  Theme ▾  ● Claude Code · MCP │
├───────────────────────────────────────────────────────────────────┬──────────────────────┤
│ canvas (flex)                                                     │ inspector 380px      │
│  caption line                                   [Up one level]    │                      │
│                                                                   │                      │
│   level content (§4)                                              │                      │
│                                                                   │                      │
├ footer 36px ──────────────────────────────────────────────────────┤                      │
│ Worse ▮▮▮▮▮ Better · λ pure · C = CRAP · M = mutation (?)   Click to inspect · double-click to open │
└───────────────────────────────────────────────────────────────────┴──────────────────────┘
```

- **Top bar:** logo; breadcrumbs (each earlier crumb is a button that jumps to that level); `Color by` segmented control (Combined, CRAP, Coverage, Mutation); `Theme` select; agent status pill (green dot when an MCP client is connected in the last 60 s, grey otherwise, text "Claude Code · MCP" or "No agent").
- **Canvas:** dotted grid background (`surface` with `border` dots, 22px). Pan with drag or trackpad; zoom with ctrl/cmd + wheel and `+`/`−`/`0` keys; fit-to-view on each level change.
- **Footer:** 5 swatches of the grade scale, legend text, a `?` for the "grade" glossary entry, and the interaction hint.
- **Inspector:** always visible; shows the selected node (§5).

## 3. Cards

All cards: `border-radius 10px`, `padding 12px`, width fixed per level (§4), **height = content** (never fixed). Fill = grade color at 16% (dark) / 13% (light); border 1px grade color at 55% / 60%. Selected: 2px `accent` border + 4px `accentSoft` ring. Missing data colors red (grade 1). Stale data: dashed border.

**Header row (always, 20px):** `λ` in accent if pure (packages and functions) · `+`/`−` visibility for functions (faint) · name in mono 14px/600, ellipsis · `C` dot and `M` dot (18px circles, grade colors, letter in `surface` color). A dot is `border2` grey when that metric does not apply (e.g., no mutation sites).

**Problem lines (packages):** up to 2 lines, 12px muted, each with a 6px dot in a severity color. Only when something is wrong, chosen in this order: untested function ("BestRule has no tests"), mutation below target ("12 mutants survived"), worst CRAP above target ("handleQuote · CRAP 18.7"), "mutation not measured yet". Healthy packages show no lines.

**Chips (functions):** up to 3 chips in one or two rows, mono 11px, tinted with their color at 18%: `untested` (red) if coverage 0; `CRAP 14.0` if above `crap_max` (color = CRAP grade); `N survived` if survivors > 0 (yellow, red if > 2). Healthy functions show no chips.

**Pills** (other packages' functions, external modules, types): height 26–30px, fully rounded. Function pills have a 9px grade dot. External modules: transparent fill, dashed `border2`, muted text. Type pills: `surface2` fill.

Double-click a card = open it (drill in). Single click = select (inspector). Enter/Space on a focused card = select; Enter twice quickly or `o` = open. All cards are real `<button>`s with an `aria-label` including name, kind and the key score.

## 4. Levels

### Level 0 — Module
- Packages of the module as cards (width 236px), laid out as a **flow from the entry**: row 0 holds the packages nothing imports (the `cmd/…` mains); every package sits below everything that imports it (longest path), so all arrows point down and shared foundations sink to the bottom. Packages with no imports either way go last. A rank wider than 4 cards wraps into balanced rows, each spanning the width.
- Arrows = imports, **backbone only**: an import is not drawn when a longer path already shows it (transitive reduction). Selecting a card outlines (accent border) every package it really imports or is imported by, including hidden ones; the backbone arrows touching it turn accent. Arrows are smooth curves.
- External modules as pills in **one centered strip under the map**, without arrows; selecting a package outlines the modules it imports.
- Layout: flow.ts chooses rows and order; ELK `layered` (INTERACTIVE layering keeps the rows, BK placement centers cards over their children, spline routing). Collapse multiple edges between the same two nodes into one.
- Opening view: readable zoom (at least 70%, at most 100%), top of the map at the top, centered on the entry that reaches the most packages; `0` fits everything.
- Caption: `<module> · packages · arrows show imports`.
- More than 25 packages: group by first path segment(s) as group cards (a group card looks like a package card, rolls up the same way, and drills into its packages).

### Level 1 — Package
- A **level box** fills the canvas: rounded 14px, fill = the package's grade color at 6% (dark) / 7% (light), border 1px at 35%. The box header (44px) holds a button with the package's λ, name and C/M dots (click selects the package itself), then the package's **types as pills** (click selects a type).
- Functions as cards inside the box (width 240px), laid out as a flow like level 0: functions nothing in the package calls on top, callees below, functions with no calls either way last, rows of at most 4. Arrows = calls inside the package, backbone only; selecting a function outlines everything it calls or is called by.
- Other module packages that are called appear as **one pill per package** (`name · N` functions called) in a strip **below the box**, without arrows; selecting a function outlines the packages it calls. Click selects the package, double-click opens it. Stdlib calls are not drawn.
- Layout: as level 0, inside the box. Opening view as level 0, but it never scrolls past the box's edges, so the header stays in view.
- More than 40 functions: show the 40 riskiest plus a "N more" card that expands in place.

### Level 2 — Function
- A level box tinted with the function's grade color. Header: name, C/M dots, `file · lines a–b`, and a legend on the right: covered (green), not covered (red), survived (yellow), note (accent).
- **Callers** as pills above the box on the left, **callees** as pills above on the right, each with a short vertical arrow into or out of the box. Clicking a caller or callee selects it; double-click opens it.
- Inside the box: the **source view** (§6), scrollable, fills the box.

Transitions: 200 ms fade/scale on level change; respect `prefers-reduced-motion`. The browser Back button and `Esc` go up one level (Esc first closes an open tooltip). The URL holds the level and selection (`#/pkg/<id>`, `#/func/<id>`), so reloads and links work.

## 5. Inspector

Order, top to bottom (omit empty sections):
1. Kind label (uppercase 11px muted): `Package`, `Function · exported`, `Function · unexported`, `Type · struct`, `Function · other package`, `External module`.
2. Title (mono 20px) + tag (`λ pure core` / `effectful shell` for packages, `λ pure` / `effectful` for functions; effectful shows the reasons on hover).
3. Subtitle (mono 12px muted): import path + counts for packages; `file:line` for functions.
4. Signature box (functions).
5. **Metric rows**, 32px each: label · `?` · value (mono) · 10px grade chip. Hover or focus on the row shows the tooltip to the left of the inspector; clicking `?` pins it (with a close ×).
   - Package: CRAP max, CRAP average, Coverage, Mutation score, Surviving mutants, Not-covered mutants, Churn, Green functions (`92 of 124 · 74%`: combined grade 9–10; the share behind a worst-of grade).
   - Module (nothing selected at level 0): Green functions.
   - Function: CRAP, Cyclomatic (CC), Cognitive, Coverage, Mutation score, Surviving mutants, Not-covered mutants, Churn (file).
   - Other-package function: CRAP, Coverage, Mutation score.
6. **List** (buttons): package → "Riskiest functions" (up to 3: name, reason, CRAP colored); function → "Surviving mutants" (`L29`, `>= → >`, count); type → "Fields".
7. **Actions:** primary button (`Open package` / `Show code` / at level 2 `Re-run mutation for this function`); `Open in` row with the configured editor first plus up to two others detected on PATH; `Copy fix prompt` and `Send to agent` side by side.

## 6. Source view

- Shiki-highlighted Go using the theme's `shiki` id; line numbers (faint, right-aligned, 52px); a 3px gutter bar per line: green covered, red not covered, yellow-green split for partial.
- Line tint: not covered → red at 9%; line with a surviving mutant → yellow at 8%.
- **Inline annotations** under the line they belong to (indented to the code column):
  - `SURVIVED` badge (yellow) · "N mutants survived" · the changes in mono (`>= → >   ·   && → ||`) · one muted line saying what test is missing (from the note if a human or agent wrote one, else a generic hint per mutation type).
  - `NOT COVERED` badge (red) for the first line of each uncovered run: "Never run by tests" + the condition text.
  - `NOTE` badge (accent): author and age ("Note from you · 2 days ago", "Note from agent:claude-code · 1 hour ago") + text. Outdated notes are greyed with "(code changed)".
- Click a line number to select that line (sets `selected_line` in focus and shows "Add note" in the inspector).
- Scroll to the function on open; the header shows `file · lines a–b`.

## 7. Tooltips (`?`)

- Content from `terms.yaml`: title, body, "Good: …" line, optional links as `text ↗` opening a new tab.
- Width 310px, `surface2` background, `border2` border, theme `shadow`. Opens on hover or keyboard focus after 150 ms; pinned by click; closes on Esc, click outside, or ×.
- Positioned outside the inspector (to its left) so it never covers the values; footer tooltip opens upward.
- `aria-expanded` on the `?` button; the tooltip has `role="tooltip"` when hovered and is a small dialog when pinned.

## 8. Themes

- The Theme select lists built-in themes (themes.json) then custom ones (`.qtldr/themes/*.json`), grouped "Dark" / "Light".
- Switching is instant and changes: chrome, card fills, grade scale, level box tint, edges (`border2`, selected edges `accent` 2px), code highlighting, tooltip colors.
- The choice is saved to `.qtldr.toml` `[ui].theme` via the server (so it follows the project) and mirrored in localStorage for fast first paint.
- `theme_light` / `theme_dark` set → follow `prefers-color-scheme` unless the user picked one explicitly this session.
- Contrast: `muted` and `faint` must pass 4.5:1 against `bg` and `surface`; a unit test checks every built-in theme.

## 9. Toasts and live updates

- Bottom-center toast (surface2, border2) for results of actions: copied prompt, sent to agent, editor command run or failed (with the exact command), refresh started/finished.
- SSE `snapshot` → re-render keeping the current level and selection; nodes whose ID vanished → go up to the nearest existing parent.
- SSE `stale` → dashed border on affected cards and a "stale" marker next to values in the inspector.
- SSE `progress` → a thin progress bar under the top bar with "Running coverage… 3/6 packages".

## 10. Accessibility and keyboard

- All interactive elements are native buttons, links, selects; visible focus ring (2px accent).
- Keys: arrows move selection between cards (spatially), Enter select, `o` open, Esc up, `/` search (fuzzy jump to any package/function), `t` cycle theme, `c` copy fix prompt, `e` open in editor.
- Minimum hit target 32px on desktop.
