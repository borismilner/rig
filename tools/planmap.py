#!/usr/bin/env python3
"""Regenerate PLAN.md's Map. Run after adding, renaming or moving a section.

The Map carries live line numbers and a changelog-share percentage, so it rots
the moment anything above it moves. Hand-keeping it was tried for one session
and the numbers were wrong within the hour. Run this instead:

    python3 tools/planmap.py           # rewrite the Map in place
    python3 tools/planmap.py --check   # exit 1 if it is stale
"""
import re, sys

PLAN = "PLAN.md"
START = "## Map - what is in this document, and what to read for what"
GROUPS = [("WHAT IT IS - read before proposing anything", 1, 4),
          ("HOW IT IS BUILT - the specification proper", 5, 22),
          ("THE ROUTE - order, gates, and what is refused", 23, 30),
          ("THE RECORD - what changed and why. **CHANGELOG, not specification**", 31, 99)]


def headings(lines):
    out = []
    for i, l in enumerate(lines):
        if l.startswith("## "):
            m = re.match(r"(\d+)\.\s*(.+)", l[3:].strip())
            if m:
                out.append((i + 1, int(m.group(1)), m.group(2).strip()))
    return out


def build(lines, maplen=0):
    """Render the Map against a body that has no Map in it.

    `maplen` is how many lines the Map itself will add. Called twice: once with
    0 to learn the length, once with it, so the line numbers and the total the
    Map reports are the ones the finished file actually has. Reporting the body
    length was the first version and it was off by the size of the Map.
    """
    heads = [(ln + maplen, n, name) for ln, n, name in headings(lines)]
    total = len(lines) + maplen - 1  # the trailing empty element of split('\n')
    h31 = [ln for ln, n, _ in heads if n == 31][0]
    share = round(100 * (total - h31 + 1) / total)
    o = [START, "",
         f"**This document is {total} lines and nobody reads it whole.** It is a reference to",
         'query, and it is queried with two questions: *"what is rig supposed to do"* and',
         '*"what did he already rule on this"*. **The Map exists so a requirement cannot hide',
         "in it**, which is the failure this project has now had four times - the tray icon,",
         "the acceptance floor, the readiness bar, and the successor clause.", "",
         "**GENERATED. Do not hand-edit** - run `python3 tools/planmap.py` after adding or",
         "renaming a section. Hand-keeping it was tried and the numbers were wrong within the",
         "hour, and a Map with wrong line numbers hides things better than no Map at all.", ""]
    for title, a, b in GROUPS:
        sel = [x for x in heads if a <= x[1] <= b]
        o += [f"### {title}", "", "| § | Section | Line |", "|---|---|---|"]
        o += [f"| {n} | {name} | `{ln}` |" for ln, n, name in sel]
        o.append("")
    o += ["### The four questions a seat actually arrives with", "",
          "| Asking | Go to |", "|---|---|",
          "| **what should I build next?** | **NOT this file.** `logbook/projects/rig/BACKLOG.md` answers WHAT NEXT; this answers WHAT IS IT |",
          '| **how much longer until rig develops rig?** | `logbook/projects/rig/READINESS.txt`, and nothing else may answer it |',
          "| **what do agents get, and when?** | §16 for the primitives, §37 for the minimum set and the staged migration |",
          "| **has he already ruled on this?** | `logbook/projects/rig/DECISIONS.md` first, then §31-38 here. **Grep before proposing** - proposing what already exists is this project's named failure mode |",
          "",
          f"**§31-38 ARE A CHANGELOG SITTING INSIDE A SPECIFICATION** - {total - h31 + 1} of these",
          f"{total} lines, {share}%. Recorded here rather than quietly tolerated: it is a known",
          "structural defect with a backlog item (`BACKLOG.md` B24), and until that item lands",
          "**a reader looking for the current rule prefers the numbered section over the pass",
          "that changed it.**", "", "---", ""]
    return o


def main():
    lines = open(PLAN).read().split("\n")
    try:
        s = lines.index(START)
    except ValueError:
        sys.exit(f"{PLAN}: no Map found (expected a line reading {START!r})")
    e = next(i for i in range(s + 1, len(lines)) if lines[i].startswith("## 1."))
    body = lines[:s] + lines[e:]
    m = build(body, len(build(body)))
    new = body[:s] + m + body[s:]
    if new == lines:
        print("Map is current")
        return
    if "--check" in sys.argv:
        sys.exit("Map is STALE - run: python3 tools/planmap.py")
    open(PLAN, "w").write("\n".join(new))
    print(f"Map regenerated: {len(new)} lines")


main()
