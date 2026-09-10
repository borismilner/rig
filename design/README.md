# rig / turret - the visual system

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

## The two rules the engine enforces

**Hues are generated in oklch.** Six evenly spaced at one lightness and one
chroma, so "a family" is arithmetic rather than a promise. Move the lightness
and all six move together and stay a family.

**Neutrals are solved, not chosen.** `--border`, `--fg-dim` and `--fg-faint`
are binary-searched to the dimmest value that still clears their WCAG target
on *every* surface they can land on. This is why the inherited `#556579`
border - 2.40:1 on a panel - is not something anyone can type any more.

## Owed elsewhere

The house family lives in `~/me/library/held/_assets/held.css` and reaches
shelf through `shelf/internal/ui/assets/tokens.css`. Two of its border values
fail 3:1 in both themes and the correction belongs there, not here:

| Token | Inherited | Measures | Corrected |
|---|---|---|---|
| dark `--border` | `#556579` | 2.40:1 on `--panel` | solved per theme |
| light `--border` | `#c0cad6` | 1.59:1 on `--bg` | solved per theme |
