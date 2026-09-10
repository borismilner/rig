#!/usr/bin/env python3
"""Inline theme.js and app.js into one self-contained page.

Chrome refuses to fetch an ES module over file://, so a page meant to be
opened by double-clicking cannot have external module scripts. The sources
stay separate files (they are the ones worth reading and diffing); this
produces the artefact.

    python3 design/build.py        # -> design/visual-system.html
"""
import re, pathlib, subprocess, sys

HERE = pathlib.Path(__file__).parent
src    = (HERE / "visual-system.src.html").read_text(encoding="utf-8")
theme  = (HERE / "theme.js").read_text(encoding="utf-8")
app    = (HERE / "app.js").read_text(encoding="utf-8")
kitjs  = (HERE / "kit" / "kit.js").read_text(encoding="utf-8")
kitcss = (HERE / "kit" / "kit.css").read_text(encoding="utf-8")

# strip module syntax: exports become plain declarations, the import goes
theme = re.sub(r"^export\s+", "", theme, flags=re.M)
app   = re.sub(r"^import\s+\{[^}]*\}\s+from\s+'\./theme\.js';\s*$", "", app, flags=re.M)


# ── the kit ────────────────────────────────────────────────────────────────
# R8 wants each element shown beside the fake application that uses it, and
# until now this page could not reach the kit at all: build.py inlined theme.js
# and app.js and nothing else, and the source had zero mentions of kit.js or
# kit.css. A tenth section written as hand-copied markup would have been a
# SECOND IMPLEMENTATION of the kit on the one page `make contrast` trusts -
# which is the same mistake the rail's CSS was ported rather than reinvented to
# avoid. So the page gets the real thing.
#
# The kit keeps its OWN scope rather than joining theme+app in one IIFE. In the
# browser it is a module, `el`, `num` and `alignOf` are private to it, and
# flattening it here would put three short names into a 53k file's scope and
# make a collision a matter of luck. Its exports come back out under one name.
def kit_module(source):
    """kit.js as an IIFE that returns its exports, so app.js can call them."""
    names = re.findall(r"^export\s+function\s+([A-Za-z_$][\w$]*)", source, flags=re.M)
    if not names:
        sys.exit("build: kit.js exports nothing, so section 10 would have "
                 "nothing to show")
    body = re.sub(r"^export\s+", "", source, flags=re.M)
    return ("var RIGKIT = (function(){\n" + body +
            "\nreturn {" + ", ".join(names) + "};\n})();"), names

# ── the section nav ────────────────────────────────────────────────────────
# R8 says this page has nine sections and no navigation of any kind, "which is
# fixed before a tenth is added". Generated rather than hand-written, so the
# section list has ONE source: a tenth section brings its own nav entry, its own
# id and its own number, and none of the three can disagree with the others.
#
# It also takes the numbers over. They were hand-typed in nine <span class="n">
# elements, which is the copy that renumbering a section would have to keep in
# step by hand.
SECTION = re.compile(
    r'<section><div class="wrap">\s*\n\s*<h2><span class="n">(\d+)</span>\s*([^<]+?)</h2>'
)


def slug(title):
    out = re.sub(r"[^a-z0-9]+", "-", title.strip().lower()).strip("-")
    if not out:
        sys.exit("build: a section heading slugifies to nothing: %r" % title)
    return out


def with_nav(page):
    """Give every section an id and a derived number, and emit the nav."""
    found = list(SECTION.finditer(page))
    if not found:
        sys.exit("build: no sections matched, so the nav would be empty")

    ids, entries = set(), []
    for i, m in enumerate(found, start=1):
        title = m.group(2).strip()
        sid = "s-" + slug(title)
        if sid in ids:
            # Two sections with the same heading would give the nav two entries
            # pointing at one place, and the second id would never be reached.
            sys.exit("build: two sections slugify to %s (%r)" % (sid, title))
        ids.add(sid)
        entries.append((sid, "%02d" % i, title))

    # Rewritten back to front, so an earlier replacement cannot move the spans
    # a later one is still holding an offset for.
    for (sid, number, title), m in zip(reversed(entries), reversed(found)):
        block = m.group(0)
        fixed = block.replace(
            '<section><div class="wrap">',
            '<section id="%s"><div class="wrap">' % sid,
            1,
        ).replace(
            '<span class="n">%s</span>' % m.group(1),
            '<span class="n">%s</span>' % number,
            1,
        )
        page = page[: m.start()] + fixed + page[m.end() :]

    nav = ['<nav class="secnav" aria-label="Sections">']
    for sid, number, title in entries:
        nav.append('  <a href="#%s"><b>%s</b> %s</a>' % (sid, number, title))
    nav.append("</nav>")

    page, n = re.subn(r"<!-- NAV -->", lambda m: "\n".join(nav), page)
    if n != 1:
        sys.exit("build: NAV placeholder not found exactly once (found %d)" % n)

    return page, entries


src, sections = with_nav(src)

# kit.css goes in verbatim. Every selector in it is rig- namespaced - that
# prefix was chosen so an adopter's own stylesheet would not collide with it,
# and the same choice is what makes it safe to drop into this page's style
# block beside 680 lines of the page's own rules.
src, n = re.subn(r"[ \t]*/\* KIT CSS \*/", lambda m: kitcss, src)
if n != 1:
    sys.exit("build: the KIT CSS placeholder is not in the style block exactly "
             "once (found %d)" % n)

kit_js, kit_names = kit_module(kitjs)

bundle = (
    "/* generated by design/build.py from theme.js + kit/kit.js + app.js"
    " - edit those */\n"
    "(function(){\n" + theme + "\n" + kit_js + "\n" + app + "\n})();"
)

# NOTE: the replacement MUST be a function. As a string, re.subn would
# interpret backslash escapes inside the JavaScript - \n in a string literal
# becomes a real newline and the bundle stops parsing.
tag = "<script>\n" + bundle.replace("</script>", "<\\/script>") + "\n</script>"
out, n = re.subn(
    r'<script type="module" src="\./app\.js"></script>',
    lambda m: tag,
    src,
)
if n != 1:
    sys.exit("build: script placeholder not found exactly once (found %d)" % n)

# theme.js and app.js share one IIFE, so a syntax error in either kills all nine
# sections at once - and this script used to print success and exit 0 over a page
# whose script does not parse, so a broken page and a working one were
# indistinguishable to anything downstream, CI included.
check = subprocess.run(["node", "--check", "-"], input=bundle,
                       capture_output=True, text=True)
if check.returncode != 0:
    sys.exit("build: the emitted script does not parse, refusing to write "
             "visual-system.html\n" + (check.stderr or check.stdout).rstrip())

(HERE / "visual-system.html").write_text(out, encoding="utf-8")
print("built design/visual-system.html  (%d KB, %d sections, kit: %s)"
      % (len(out) // 1024, len(sections), ", ".join(kit_names)))
