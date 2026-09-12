## 6. Configuration

Layers, lowest to highest, with the winner recorded per key:

```
  built-in defaults
  < /etc/rig/rig.toml
  < ~/.config/rig/rig.toml
  < the program's declared defaults
  < ~/.config/rig/apps/<id>.toml
  < environment (RIG_*)
  < command-line flags
  < a runtime override set in the UI
```

- Every key is schema-declared, so the settings UI is generated. No hand-written forms.
- `rig config origin <key>` prints the winner and every loser, with file and line.
- Changes apply live: rig validates, pushes, and shows accepted, rejected with the program's own
  reason, or needs-restart with a one-click restart.
- A change set is validated whole before any of it is sent, so nothing lands half-applied.
- `rig config export` and `rig config diff` reproduce and compare **the caller's** machine.
  Reproducing the whole machine is an estate-wide read and therefore needs `introspect`
  (§14), which an agent Boris runs holds.
- **Every resolution is written to a snapshot on disk**, already merged, with provenance, the
  database path assigned and the schema version migrated to. That file is what a program reads
  when rig is unreachable (§5g), and it is the reason there is exactly one implementation of
  config resolution.
- **A renamed key reports its losers.** `rig loose-ends` (§8) names every override that stopped
  applying, because the silent version of this bug is invisible to `rig config origin`: the
  override does not lose, it simply no longer exists under that spelling.

**The visual system is configuration, not code.** Faces, base size, type scale, tracking, line
height, corner radius, density, the hue family's lightness, chroma and rotation, the neutral
surface ladder and whether anything animates are all declared under `ui.theme` as JSON Schema,
layered and live-pushed like every other setting. `design/theme.js` is the reference
implementation and `design/visual-system.html` is the live editor over it; the export box there
emits exactly this fragment.

Two rules travel with it, because a theme is the one setting a user can break the product with:

- **The hues are generated in oklch**, six evenly spaced at one lightness and one chroma, so a
  family stays a family when any parameter moves.
- **The neutrals are solved, not chosen.** `--border`, `--fg-dim` and `--fg-faint` are searched
  to the dimmest value that still clears their WCAG target on *every* surface they can land on.
  rig refuses to apply a token set that fails, naming the token and the ground, so a theme
  cannot silently produce an unreadable product.

**`ui.theme`, as schema.** This is the whole surface - there is no second set of knobs hidden in
code, and `design/theme.js` implements exactly this object.

```jsonc
{ "$id": "rig://schema/ui.theme", "type": "object", "additionalProperties": false,
  "properties": {
    "faces":  { "type": "object", "additionalProperties": false, "properties": {
      "display": {"type":"string"}, "ui": {"type":"string"}, "mono": {"type":"string"} } },
    "type":   { "type": "object", "additionalProperties": false, "properties": {
      "base":       {"type":"number","minimum":12,"maximum":22,"default":16,"unit":"px"},
      "scale":      {"type":"number","minimum":1.10,"maximum":1.45,"default":1.26},
      "lineHeight": {"type":"number","minimum":1.2,"maximum":1.9,"default":1.55},
      "uiTight":    {"type":"number","minimum":-0.04,"maximum":0.02,"default":-0.011,"unit":"em"},
      "dispTight":  {"type":"number","minimum":-0.06,"maximum":0.02,"default":-0.024,"unit":"em"} } },
    "shape":  { "type": "object", "additionalProperties": false, "properties": {
      "radius":  {"type":"number","minimum":0,"maximum":26,"default":12,"unit":"px"},
      "density": {"type":"number","minimum":0.7,"maximum":1.4,"default":1.0},
      "gut":     {"type":"number","minimum":0.8,"maximum":3.0,"default":1.6,"unit":"rem"} } },
    "hues":   { "type": "object", "additionalProperties": false, "properties": {
      "members": { "type":"array","minItems":6,"maxItems":9,"items": {
        "type":"object","required":["name","angle","role","identity","anchored"],
        "properties": {
          "name":     {"type":"string","pattern":"^[a-z][a-z0-9-]{1,15}$"},
          "angle":    {"type":"number","minimum":0,"exclusiveMaximum":360},
          "role":     {"enum":["bad","warn","good","progress","info","-"]},
          "identity": {"type":"boolean","description":"false = no program may own it"},
          "anchored": {"type":"boolean","description":"true = the optimiser may not move it"} } } },
      "rotate": {"type":"number","minimum":-180,"maximum":180,"default":0},
      "dark":   {"$ref":"#/$defs/lc","default":{"L":0.800,"C":0.098}},
      "light":  {"$ref":"#/$defs/lc","default":{"L":0.470,"C":0.110}} } },
    "surfaces": { "type":"object","additionalProperties":false,"properties": {
      "hue":    {"type":"number","minimum":0,"exclusiveMaximum":360,"default":252},
      "chroma": {"type":"number","minimum":0,"maximum":0.06,"default":0.022},
      "dark":   {"$ref":"#/$defs/ladder"}, "light": {"$ref":"#/$defs/ladder"} } },
    "motion": {"type":"boolean","default":true,
               "description":"forced false under prefers-reduced-motion, never the other way"} },
  "$defs": {
    "lc":     {"type":"object","additionalProperties":false,"properties":{
                 "L":{"type":"number","minimum":0.2,"maximum":0.95},
                 "C":{"type":"number","minimum":0,"maximum":0.22}}},
    "ladder": {"type":"object","additionalProperties":false,
               "required":["bg","bg2","panel","glow","tint","fg"],
               "description":"oklch L per surface. Every surface is its own knob: one step cannot express a light theme, where panel goes UP toward white and the recessed grounds go DOWN away from it",
               "properties":{"bg":{"type":"number"},"bg2":{"type":"number"},
                 "panel":{"type":"number"},"glow":{"type":"number"},
                 "tint":{"type":"number"},"fg":{"type":"number"}}} } }
```

**Three things the schema cannot express, and rig enforces them after validation:**

1. `--border`, `--border-2`, `--fg-dim` and `--fg-faint` are **not in the schema at all**. They
   are solved from the ladder, so there is no spelling of a failing border.
2. **Text tokens are solved against every surface text lands on, `tint` included; boundary
   tokens against the four a boundary sits on.** The two sets are different, and solving a text
   token against the wrong one is how a 3.95:1 reaches a page that audits clean.
3. Rejection names the token *and the ground it failed on*, because "contrast too low" without
   the ground is not actionable.

---
