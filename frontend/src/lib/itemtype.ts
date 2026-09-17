/* The work-item type, its icon and its word.
 *
 * ⛔ BORIS, 2026-09-17: "The different work-items should probably be classified
 * to task/bug/idea/... with appropriate icons."
 *
 * ⛔ THE SET IS OPEN AND THE TRAILING "/..." IS HIS. A type this file does not
 * know renders as its own word with no icon, rather than being refused or
 * folded into "other". A closed set here would mean the window silently
 * mislabels the first type somebody invents.
 *
 * ⛔ AND NOTHING INFERS A TYPE. An item with no `item_type` is UNTYPED, which
 * is the honest state of every row in the store today: BACKLOG.md never said
 * whether a row was a bug, and a type derived from a row's wording would be the
 * window composing content. plan/39 and plan/11 both refuse it by name.
 */

export type ItemTypeView = {
  /** The word shown beside the icon. */
  label: string;
  /** A 16x16 path, or "" when this type has no icon of its own. */
  icon: string;
  /** A theme hue member name, or "" to stay neutral. */
  hue: string;
};

/* The three he named. Each icon is a SHAPE rather than a letter, because the
 * row already carries the id in mono and a second glyph of text beside it would
 * read as part of the identifier. */
const KNOWN: Record<string, ItemTypeView> = {
  // A check-box outline: work to be done, not a judgement about it.
  task: {
    label: "task",
    icon: "M2.5 2.5h11a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1zm1.2 2v6.6h8.6V4.5H3.7z",
    hue: "steel",
  },
  // A ring with a bite out of it: something that is wrong with a built thing.
  bug: {
    label: "bug",
    icon: "M8 1.8a6.2 6.2 0 1 0 0 12.4A6.2 6.2 0 0 0 8 1.8zm0 1.9a4.3 4.3 0 0 1 3.5 1.8L5.5 11A4.3 4.3 0 0 1 8 3.7z",
    hue: "rust",
  },
  // A small spark: proposed, not yet committed to.
  idea: {
    label: "idea",
    icon: "M8 1.5l1.5 4.2L13.7 7l-4.2 1.4L8 12.6 6.5 8.4 2.3 7l4.2-1.3L8 1.5z",
    hue: "amber",
  },
};

/* ⛔ AN UNKNOWN TYPE GETS A NEUTRAL GLYPH, NOT ITS OWN WORD IN THE ROW.
 *
 * The first version printed the word in the row's type column. The contrast
 * gate FAILED it: `TINY tword 9.92px "spike"` - the column is 1.1rem wide, so
 * fitting a word in it meant 0.62rem, under the 12px floor. Shrinking text to
 * fit a column is the readability defect this project measures for, and the
 * gate caught it before it shipped.
 *
 * So an unknown type draws an outlined square - visibly "classified, but not
 * one of the three" - and its actual word appears where there is room for it
 * at full size: the row's tooltip, and the `type` line in the opened detail.
 * Nothing is lost and nothing is illegible. */
const UNKNOWN_ICON =
  "M3 3h10a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1zm1.3 2v6h7.4V5H4.3z";

/** Never returns null for a non-empty type: an unknown one is still shown. */
export function itemTypeView(t: string | undefined): ItemTypeView | null {
  if (!t) return null;
  return KNOWN[t] ?? { label: t, icon: UNKNOWN_ICON, hue: "" };
}
