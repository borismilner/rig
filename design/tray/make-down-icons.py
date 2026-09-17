#!/usr/bin/env python3
"""Draw the DOWN variant of each tray icon: the estate's own glyph, badged.

    python3 design/tray/make-down-icons.py

Boris, 2026-09-17: "if the daemon is down it can also indicate it with a
red dot and details when clicked."

WHY A BADGE ON THE ESTATE'S OWN ICON, AND NOT ONE SHARED DOWN GLYPH.
A single down icon would answer "rig is not running" and lose "which
estate", and the estate is the thing the tray exists to say. Badging
keeps both facts on one glyph, and keeps the silhouette recognisable as
rig's - which matters because the icon vanishing is the failure this
whole change is about.

WHY THE DOT IS RINGED TWICE. A tray strip is light on some themes and
dark on others and the icon cannot know which. A bare red circle relies
on the background to frame it; a white ring inside a dark outline frames
itself, so the badge reads the same on either. The ratios both rings
produce are printed when this runs, against white and against black.
"""

import os
import sys

from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))

# The badge. RED IS HIS WORD and this is the one that survives being
# seven pixels across: darker reds go muddy at tray size and lighter
# ones read as orange.
RED = (211, 47, 47, 255)
RING_INNER = (255, 255, 255, 255)
RING_OUTER = (26, 26, 26, 255)

# Proportions of the canvas. The badge is bottom-right because that is
# where a notification badge is read for, and it is large: at 22 px a
# badge under a third of the width is a smudge, not a signal.
BADGE = 0.40
INNER_RING = 0.055
OUTER_RING = 0.030
MARGIN = 0.02


def luminance(rgb):
    def channel(c):
        c /= 255
        return c / 12.92 if c <= 0.03928 else ((c + 0.055) / 1.055) ** 2.4

    r, g, b = (channel(x) for x in rgb[:3])
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def contrast(a, b):
    la, lb = luminance(a), luminance(b)
    hi, lo = max(la, lb), min(la, lb)
    return (hi + 0.05) / (lo + 0.05)


def badge(src, dst):
    im = Image.open(src).convert("RGBA")
    w, h = im.size
    d = ImageDraw.Draw(im)

    size = w * BADGE
    outer = size
    x1 = w - w * MARGIN
    y1 = h - h * MARGIN
    x0, y0 = x1 - outer, y1 - outer

    # Outermost first, each inset by its own ring width, so the rings
    # are concentric without arithmetic at every call site.
    d.ellipse([x0, y0, x1, y1], fill=RING_OUTER)
    o = w * OUTER_RING
    d.ellipse([x0 + o, y0 + o, x1 - o, y1 - o], fill=RING_INNER)
    i = o + w * INNER_RING
    d.ellipse([x0 + i, y0 + i, x1 - i, y1 - i], fill=RED)

    im.save(dst)
    return im


def main():
    made = []
    for name in ("production", "development"):
        src = os.path.join(HERE, name + ".png")
        if not os.path.exists(src):
            print("missing source: " + src, file=sys.stderr)
            return 1
        dst = os.path.join(HERE, name + "-down.png")
        im = badge(src, dst)
        made.append((dst, im))

    print("contrast of the badge, measured rather than assumed:")
    for label, colour in (("red on white ring", (RED, RING_INNER)),
                          ("white ring on dark outline", (RING_INNER, RING_OUTER)),
                          ("dark outline on a white tray", (RING_OUTER, (255, 255, 255))),
                          ("dark outline on a black tray", (RING_OUTER, (0, 0, 0))),
                          ("white ring on a black tray", (RING_INNER, (0, 0, 0)))):
        print("  %-28s %5.2f:1" % (label, contrast(*colour)))

    for dst, im in made:
        print("wrote %s %s" % (dst, im.size))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
