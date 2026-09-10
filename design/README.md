# rig - the visual system

`visual-system.html` is the deliverable: open it in a browser, it is one
self-contained file. Everything on it is live - pick a program in the rail,
open the tray, fire a toast, and open **Lab** (top right) to move any visual
parameter of the product and watch the contrast table re-measure while you
drag.

| File | What it is |
|---|---|
| `theme.js` | The theme engine. Turns a small config object into the whole token set |
| `app.js` | The page: the mocks, the lab, the live measurement |
| `visual-system.src.html` | Markup and CSS. **Edit this**, not the built file |
| `build.py` | Inlines the two scripts into `visual-system.html` |
| `visual-system.html` | Generated. Do not edit |

```
python3 design/build.py          # rebuild after changing any source
node ~/.claude/skills/readable-output/references/audit-pages.mjs \
     design/visual-system.html   # measure both themes; must print "clean"
```

## What is on the page

| # | Section | Shows |
|---|---|---|
| 01 | The window | Rail, context bar, pane, status strip. Click a program: the pane cross-fades and the page re-hues |
| 02 | Status at rest | Every rail state side by side. Healthy is the absence of colour |
| 03 | The tray | Your panel now vs with rig, and the menu as a real `popover` |
| 04 | Toasts | Fire them. Spring in, dwell, hover to hold, collapse into a deck past three |
| 05 | The palette | The family, generated and measured twice - WCAG and CIEDE2000 |
| 06 | The notification centre | Search, filter, answer a row in place, Do Not Disturb suppressing the surface but not the record |
| 07 | When a program crashes | The pane becomes a panel. Real countdown, doubling backoff, budget, quarantine |
| 08 | Who is using rig | The operator table. Pick a client, its history opens, and looking is itself an event |
| 09 | Generated panes | Six renderers beside the declaration that produced them, with the CLI line built from the same object |

## The two rules the engine enforces

**Hues are generated in oklch.** Six evenly spaced at one lightness and one
chroma, so "a family" is arithmetic rather than a promise. Move the lightness
and all six move together and stay a family.

**Neutrals are solved, not chosen.** `--border`, `--fg-dim` and `--fg-faint`
are binary-searched to the dimmest value that still clears their WCAG target
on *every* surface they can land on. This is why the inherited `#556579`
border - 2.40:1 on a panel - is not something anyone can type any more.

**Two ground sets, not one.** Text tokens solve against five surfaces
(`bg`, `bg-2`, `panel`, `glow`, `tint`); boundary tokens against the four a
boundary can sit on. `--tint` is the extreme of the ladder in both themes, and
solving a text token against the other four and then painting it on `--tint`
is how a 3.95:1 shipped through a clean audit. The measured floor moved from
4.69 to 4.51 dark when this was fixed - the number went *down* because it is
now taken on the surface that actually loses.

## Two bugs an audit cannot see

Both are in the source now; both are worth knowing before adding anything.

**An overlay is invisible to a contrast audit.** The stage's `.floor` gradient
was painted over the window at `z-index:3`, and the window's own status strip
sits inside it: `--fg-dim` on `--bg-2` measures 7.34:1 and landed at **1.15:1**
under the gradient. `audit-pages.mjs` reads computed text and background
colours, so it passed every time. The floor is now behind the window
(`z-index:1`), which is structural rather than a tuned alpha.

**`table.tbl` zeroes `padding-inline-start`.** That is right for four short
columns and wrong for six monospace ones - the widest column takes the whole
gutter and the next column's content butts into it. The operator table uses a
`colgroup` with `table-layout:fixed` for that reason.

## Owed elsewhere

The house family lives in `~/me/library/held/_assets/held.css` and reaches
shelf through `shelf/internal/ui/assets/tokens.css`. Two of its border values
fail 3:1 in both themes and the correction belongs there, not here:

| Token | Inherited | Measures | Corrected |
|---|---|---|---|
| dark `--border` | `#556579` | 2.40:1 on `--panel` | solved per theme |
| light `--border` | `#c0cad6` | 1.59:1 on `--bg` | solved per theme |
