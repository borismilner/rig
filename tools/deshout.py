#!/usr/bin/env python3
"""Take the shouting out of the titles in BACKLOG.md and DECISIONS.md.

⛔ BORIS, 2026-09-18, with a screenshot of the window: "The data also seems
stale at least how it's shown in the GUI, I see 'shouting' titles and other
things that I thought you already fixed." And the turn after the record views
landed: "Fix the uglyness". B96.

⛔ THE DATA IS NOT STALE AND THE WINDOW IS NOT WRONG. The documents shout, the
record store holds what the documents say, and the window renders it faithfully.
A wall of capitals is why a faithful render reads as a stale one, so the fix
belongs to the DOCUMENT and not to the view - a view that quietly lowercased
its input would be the window lying about what rig holds.

⛔ THE INVARIANT, AND IT IS CHECKED ON EVERY LINE THIS TOUCHES:

    new.lower() == old.lower()

Nothing but CASE changes. That is what makes the pass safe to run against
documents the store is seeded from: `rigseed` slugs a title by lowercasing it,
so an id cannot move, and no citation anywhere can break. A run that would
change a character is a bug and stops the tool.

⛔ WHICH WORDS COME DOWN IS DECIDED BY EVIDENCE RATHER THAN BY A LIST I WROTE.

    in /usr/share/dict/words        an English word          -> lowercase
    Title-cased elsewhere in the    a name this project uses -> Title case
      same documents
    neither                         an acronym, an id, a     -> left alone
                                      path, a version

So `THE`, `IS` and `NEVER` come down; `MCP`, `CLI`, `B96`, `PLAN.md` and `§39`
do not; and `BORIS` becomes `Boris` because the documents write it that way
ten thousand times in ordinary prose. Anything inside backticks is skipped
whole - it is code, and its case is part of it.

Usage:
    python3 tools/deshout.py --check    # report, change nothing, exit 2 if any
    python3 tools/deshout.py            # rewrite in place
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys
import typing

LOGBOOK = pathlib.Path.home() / "me/projects/logbook/projects/rig"
DOCS = ["BACKLOG.md", "DECISIONS.md"]
WORDLIST = pathlib.Path("/usr/share/dict/words")

# A title is SHOUTING when most of its letters are capitals and there are
# enough of them to be sure. 12 letters is about three words: below that a
# run like "B96 IS OPEN" is as likely to be ids as emphasis, and the cost of a
# false positive on a short title is higher than the benefit.
MIN_LETTERS = 12
CAPS_SHARE = 0.7

class Vocab(typing.NamedTuple):
    """The three answers a token can get, and where each comes from."""

    common: set[str]  # lowercase in the dictionary -> an English word
    proper: set[str]  # capitalised in the dictionary -> a name
    names: set[str]   # Title-cased in THESE documents -> a name of this project


CODE_SPAN = re.compile(r"`[^`]*`")
WORD = re.compile(r"[A-Za-z]+(?:'[A-Za-z]+)?")


def shouting(text: str) -> bool:
    """Whether this title is mostly capitals, ignoring anything in backticks."""
    letters = [c for c in CODE_SPAN.sub(" ", text) if c.isalpha()]
    if len(letters) < MIN_LETTERS:
        return False
    return sum(1 for c in letters if c.isupper()) / len(letters) >= CAPS_SHARE


def load_words() -> tuple[set[str], set[str]]:
    """The dictionary, split the way it is actually written.

    ⛔ THE SPLIT IS THE WHOLE POINT AND FLATTENING IT WAS A REAL BUG. The list
    holds `Boris`, `MVP` and `Linux` beside `the` and `never`, so lowercasing
    every entry into one set turned BORIS into boris and MVP into mvp on the
    first dry run. An entry's OWN case says which it is: a common word is
    written lowercase, a name or an initialism is not.
    """
    if not WORDLIST.exists():
        sys.exit(
            f"deshout: no word list at {WORDLIST}. It is what decides which "
            f"capitals are English and which are acronyms, and guessing that "
            f"from a hand-written list is how MCP becomes mcp."
        )
    common: set[str] = set()
    proper: set[str] = set()
    for w in WORDLIST.read_text(errors="replace").split():
        stem = w[:-2] if w.endswith("'s") else w
        if not stem:
            continue
        # ⛔ SINGLE LETTERS ARE NEVER TAKEN FROM THE DICTIONARY. It holds "b"
        # and "B's", so `B25 CLOSES` came out as `b25 closes` - the B is an id
        # and the 25 is not part of the same token. "A" is the one exception
        # and it is handled where the tokens are.
        if len(stem) < 2:
            continue
        if stem.islower():
            common.add(stem.lower())
        elif not stem.isupper():
            # ⛔ TITLE-CASE ONLY. An ALL-CAPS entry is an initialism - MVP, USA -
            # and capitalising it gives "Mvp", which the first fixed dry run
            # produced. An initialism belongs in neither set: it is already
            # written the way it should be.
            proper.add(stem.lower())
    return common, proper


def load_names(texts: list[str]) -> set[str]:
    """Words these documents Title-case in ordinary prose: Boris, Wails, Svelte.

    ⛔ TAKEN FROM THE DOCUMENTS RATHER THAN FROM A LIST, because a list is a
    thing that goes stale silently. A name the project actually writes is a name
    the project actually has.
    """
    seen: dict[str, int] = {}
    for t in texts:
        for m in re.finditer(r"\b[A-Z][a-z]{2,}\b", CODE_SPAN.sub(" ", t)):
            seen[m.group(0).lower()] = seen.get(m.group(0).lower(), 0) + 1
    # Once is a typo; three times is how the project writes it.
    return {k for k, n in seen.items() if n >= 3}


def deshout(text: str, words: Vocab) -> str:
    """Lower what is English, Title what is a name, leave the rest."""
    out: list[str] = []
    last = 0
    # Backticked spans are copied through untouched, so the walk is over the
    # gaps between them, and the FIRST such gap is where the opening capital
    # goes - a title that starts with `make lint` must not become `Make lint`.
    spans = [(m.start(), m.end()) for m in CODE_SPAN.finditer(text)]
    first_plain = True
    for start, end in spans + [(len(text), len(text))]:
        plain = _deshout_plain(text[last:start], words)
        if first_plain and plain.strip():
            plain = _capitalise_first(plain)
            first_plain = False
        out.append(plain)
        out.append(text[start:end])
        last = end
    return "".join(out)


def _deshout_plain(chunk: str, words: Vocab) -> str:
    def one(m: re.Match[str]) -> str:
        tok = m.group(0)
        if not tok.isupper():
            return tok
        # "BORIS'S" is one token; the possessive tail always comes down.
        stem, _, tail = tok.partition("'")
        low = stem.lower()
        if len(stem) == 1:
            # The article, and nothing else: a lone capital beside digits is
            # an id - B25, M1 - and the letter is half of it.
            if stem != "A":
                return tok
            new = "a"
        elif low in words.common:
            new = low
        elif low in words.proper or low in words.names:
            new = stem.capitalize()
        else:
            return tok  # an acronym, an id, a path, a version
        return new + ("'" + tail.lower() if tail else "")

    return WORD.sub(one, chunk)


def _capitalise_first(text: str) -> str:
    """The first letter of the title itself, past any marker in front of it.

    A heading starts `### ⛔ **`; the capital belongs to the first word of the
    sentence and not to the hash.
    """
    for i, c in enumerate(text):
        if c.isalpha():
            return text[:i] + c.upper() + text[i + 1 :]
        # A digit or a symbol can legitimately open a title - "121 work-item
        # titles", "§39 justifies" - and then there is no letter to raise.
        if c.isdigit():
            return text
    return text


TITLE_LINE = re.compile(r"^(#{2,6} )(.*)$")


def rewrite(doc: str, words: Vocab) -> tuple[str, list[tuple[str, str]]]:
    """Returns the new document and every (old, new) pair it changed."""
    changes: list[tuple[str, str]] = []
    lines = doc.split("\n")
    out: list[str] = []
    fenced = False
    for line in lines:
        if line.startswith("```"):
            fenced = not fenced
        if fenced:
            out.append(line)
            continue

        m = TITLE_LINE.match(line)
        if m and shouting(m.group(2)):
            new_title = deshout(m.group(2), words)
            _assert_case_only(m.group(2), new_title)
            if new_title != m.group(2):
                changes.append((m.group(2), new_title))
            out.append(m.group(1) + new_title)
            continue

        # A backlog row: the TITLE is the second cell, and only the second.
        # The fourth cell is the row's whole argument, where emphasis is prose
        # and is not what he is reading as stale.
        if line.startswith("| ") and line.count(" | ") >= 2:
            cells = line.split(" | ")
            if shouting(cells[1]):
                new_cell = deshout(cells[1], words)
                _assert_case_only(cells[1], new_cell)
                if new_cell != cells[1]:
                    changes.append((cells[1], new_cell))
                cells[1] = new_cell
                out.append(" | ".join(cells))
                continue

        out.append(line)
    return "\n".join(out), changes


def _assert_case_only(old: str, new: str) -> None:
    if old.lower() != new.lower():
        sys.exit(
            "deshout: a rewrite changed more than case, which would move an id "
            "and break every citation of it. Refusing to write.\n"
            f"  old: {old}\n  new: {new}"
        )


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--check", action="store_true",
                    help="report and change nothing; exit 2 if any title shouts")
    ap.add_argument("--root", default=str(LOGBOOK),
                    help="where BACKLOG.md and DECISIONS.md live")
    args = ap.parse_args()

    root = pathlib.Path(args.root)
    paths = [root / d for d in DOCS]
    missing = [p for p in paths if not p.exists()]
    if missing:
        sys.exit("deshout: not found: " + ", ".join(str(p) for p in missing))

    texts = [p.read_text() for p in paths]
    common, proper = load_words()
    words = Vocab(common=common, proper=proper, names=load_names(texts))

    total = 0
    for path, text in zip(paths, texts):
        new, changes = rewrite(text, words)
        total += len(changes)
        print(f"{path.name}: {len(changes)} title(s) shouting")
        for old, fixed in changes[:6]:
            print(f"    - {old[:88]}")
            print(f"    + {fixed[:88]}")
        if len(changes) > 6:
            print(f"    ... and {len(changes) - 6} more")
        if changes and not args.check:
            path.write_text(new)

    if args.check and total:
        print(f"\ndeshout: {total} title(s) shout. Run without --check to lower them.\n"
              "A wall of capitals in the window is why a faithful render of these\n"
              "documents reads as stale data. PLAN.md section 11, B96.")
        return 2
    if not args.check:
        print(f"\nrewrote {total} title(s). Only case changed; every id is unmoved.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
