/* rig's element kit: the behaviour half.
 *
 * PLAN.md section 5h calls behaviour "the half with no definition anywhere -
 * focus order, keyboard handling, what a click does - and the fake
 * applications are where it gets one". This file is that definition, and every
 * semantic in it was copied from a measured program rather than invented:
 * pull-report's assets/app.js:107 makeTable(tableEl, cols, rows, opts).
 *
 * WHAT WAS COPIED, so an adopter's tables behave as they already do:
 *   - the default sort is the first column, descending
 *   - clicking the current sort column flips its direction
 *   - clicking a new column sets direction by type: strings ascending,
 *     everything else descending
 *   - the filter is a case-insensitive substring across the string columns, or
 *     across opts.searchKeys when given
 *   - rows are capped (5,000 by default) so one table cannot hang the page
 *   - a column may carry html(row), fmt(value, row) and cls
 *   - an absent value renders as an em dash
 *
 * WHAT WAS NOT COPIED, and is the reason this tier exists:
 *   - pull-report's sort is MOUSE ONLY. It attaches a click listener to a bare
 *     <th>, which is not focusable, has no Enter or Space, and carries no
 *     aria-sort. A keyboard user cannot sort a single one of its 16 tables.
 *     Here the header's control is a <button>, so focus and both keys come
 *     from the platform rather than from a keydown handler, and aria-sort is
 *     on the <th> where a screen reader looks for it.
 *   - its `asc` class is toggled on every <th>, so an ascending sort marks
 *     every column as the ascending one. aria-sort here is set on the sorted
 *     column and removed from the rest.
 *   - its filter input has a placeholder and no label, so the field reaches a
 *     screen reader unnamed. Every control here gets a real label, hidden
 *     visually rather than left out.
 *   - a filter that matches nothing leaves it with an empty tbody and no
 *     words. Here it says so, in a row that spans the table.
 *
 * R6: no element knows which program is holding it. Nothing below reads a
 * program id, and the only colour any of it uses is --hue, whatever the window
 * currently points that at.
 */

const EM_DASH = "—";

/* Alignment follows the column's TYPE and not its position, which is the third
 * bend the first fake application forced. pull-report aligns its first column
 * left and every other column right, and that reads correctly there because
 * its first column is always the label and the rest are always counts. A report
 * that leads with a number and follows with names gets right-aligned text out
 * of the same rule, which looks like a mistake because it is one. Text reads
 * from the left; figures line up on the right. */
function alignOf(c) {
  return c.type === "str" ? "rig-l" : "rig-r";
}

function el(tag, className, text) {
  const n = document.createElement(tag);
  if (className) n.className = className;
  if (text !== undefined) n.textContent = text;
  return n;
}

/* A count, grouped the way a reader scans one. Its own function because the
 * toolbar and the empty state both say it and must agree. */
function num(n) {
  return n.toLocaleString();
}

/* ── table ────────────────────────────────────────────────────────────────
 *
 * rigTable(tableEl, cols, rows, opts) -> { setFilter, rowCount }
 *
 * cols: [{ key, label, type, fmt, html, cls }]
 *   type "str" sorts as text and joins the default filter; anything else
 *   sorts numerically. A column is filtered on only if it is text, unless
 *   opts.searchKeys names the keys instead.
 * opts: { sortKey, sortDir, limit, searchKeys, countEl, emptyText }
 *
 * Sort state is held by column INDEX and not by key, which is a bend the first
 * fake application forced. A report legitimately shows one field twice - a raw
 * count and its share of a total - and keying the header state by c.key made
 * the second column overwrite the first, so aria-sort landed on the wrong one
 * and sorting was ambiguous. pull-report never does this, so its makeTable
 * never had to; the kit is the half that bends (section 5h).
 */
export function rigTable(tableEl, cols, rows, opts = {}) {
  let sortAt = opts.sortKey
    ? Math.max(
        0,
        cols.findIndex((c) => c.key === opts.sortKey),
      )
    : 0;
  let sortDir = opts.sortDir || -1;
  let filter = "";
  let shown = 0;
  let data = rows;

  tableEl.classList.add("rig-table");
  tableEl.replaceChildren();

  const head = el("thead");
  const hr = el("tr");
  const headers = [];

  cols.forEach((c, i) => {
    const th = el("th");
    th.dataset.key = c.key;
    th.classList.add(alignOf(c));

    // A button, not a listener on the <th>. This is the element's whole
    // contribution over the markup it was modelled on.
    const b = el("button", null, c.label);
    b.type = "button";
    b.addEventListener("click", () => {
      if (sortAt === i) sortDir = -sortDir;
      else {
        sortAt = i;
        sortDir = c.type === "str" ? 1 : -1;
      }
      render();
      // Focus stays on the control that was pressed, so a keyboard user can
      // press it again to flip rather than having to find it a second time.
      b.focus();
    });

    th.appendChild(b);
    headers[i] = th;
    hr.appendChild(th);
  });

  head.appendChild(hr);
  const body = el("tbody");
  tableEl.append(head, body);

  if (opts.countEl) {
    // Polite, so a filter's result is heard once it settles rather than on
    // every keystroke.
    opts.countEl.setAttribute("aria-live", "polite");
    opts.countEl.classList.add("rig-count");
  }

  function matching() {
    if (!filter) return data;
    const f = filter.toLowerCase();
    const keys =
      opts.searchKeys ||
      cols.filter((c) => c.type === "str").map((c) => c.key);
    return data.filter((row) =>
      keys.some((k) =>
        String(row[k] ?? "")
          .toLowerCase()
          .includes(f),
      ),
    );
  }

  function sorted(r) {
    const col = cols[sortAt];
    const key = col.key;
    const text = col.type === "str";
    return [...r].sort((a, b) => {
      const av = a[key];
      const bv = b[key];
      if (text) {
        const as = String(av ?? "");
        const bs = String(bv ?? "");
        return as < bs ? -sortDir : as > bs ? sortDir : 0;
      }
      return (Number(av || 0) - Number(bv || 0)) * sortDir;
    });
  }

  function render() {
    const r = sorted(matching());
    shown = r.length;

    // On the sorted column only, and removed from the others. The attribute is
    // what a screen reader reads and what the indicator is drawn from, so
    // there is one source for both rather than a class and an attribute that
    // can disagree.
    headers.forEach((th, i) => {
      if (i === sortAt) {
        th.setAttribute("aria-sort", sortDir === 1 ? "ascending" : "descending");
      } else {
        th.removeAttribute("aria-sort");
      }
    });

    body.replaceChildren();

    if (r.length === 0) {
      const tr = el("tr");
      const td = el(
        "td",
        "rig-empty",
        filter
          ? `Nothing matches "${filter}".`
          : (opts.emptyText ?? "This table has no rows."),
      );
      td.colSpan = cols.length;
      tr.appendChild(td);
      body.appendChild(tr);
    } else {
      const frag = document.createDocumentFragment();
      r.slice(0, opts.limit || 5000).forEach((row) => {
        const tr = el("tr");
        cols.forEach((c) => {
          const td = el("td");
          if (c.html) td.innerHTML = c.html(row);
          else if (c.fmt) td.textContent = c.fmt(row[c.key], row);
          else td.textContent = row[c.key] ?? EM_DASH;
          td.classList.add(alignOf(c));
          // add rather than assign: assigning clobbered the alignment the
          // line above had just set, which is the sort of thing that only
          // shows up when a column carries both.
          if (c.cls) td.classList.add(...c.cls.split(/\s+/).filter(Boolean));
          tr.appendChild(td);
        });
        frag.appendChild(tr);
      });
      body.appendChild(frag);
    }

    if (opts.countEl) {
      const capped = opts.limit && r.length > opts.limit;
      // "1 rows" is the kind of thing nobody notices until a filter narrows to
      // one and the element that is meant to look like rig's stops looking
      // like anybody's. The count is singular only when it is uncapped and
      // there is exactly one row: "1 of 40 rows" is about the 40.
      const noun = !capped && r.length === 1 ? "row" : "rows";
      opts.countEl.textContent = capped
        ? `${num(opts.limit)} of ${num(r.length)} ${noun}`
        : `${num(r.length)} ${noun}`;
    }
  }

  render();

  return {
    setFilter(v) {
      filter = v;
      render();
    },

    // The other bend the first fake application forced. The measured API has
    // only setFilter, so a <select> that narrows by category - which
    // pull-report puts beside its search - had nowhere to go: it is not text
    // typed into a box, it is a different set of rows. Rebuilding the table per
    // selection would lose the sort the reader had chosen, which is the part
    // that makes this an element rather than a redraw.
    setRows(next) {
      data = next;
      render();
    },

    get rowCount() {
      return shown;
    },
  };
}

/* ── toolbar ──────────────────────────────────────────────────────────────
 *
 * rigToolbar(host, opts) -> { input, select, count }
 *
 * Search, an optional select, and a live count, wired to whatever is given in
 * opts.onsearch / opts.onselect. It emits events and reads no program state,
 * which is R6: it takes data and emits, and never learns whose table it is.
 */
export function rigToolbar(host, opts = {}) {
  host.classList.add("rig-toolbar");
  host.replaceChildren();

  const id = `rig-${Math.random().toString(36).slice(2, 8)}`;

  const label = el("label", "rig-sr", opts.searchLabel || "Filter rows");
  label.htmlFor = `${id}-search`;

  const input = el("input");
  input.type = "search";
  input.id = `${id}-search`;
  input.placeholder = opts.placeholder || "filter…";

  host.append(label, input);

  let select = null;
  if (opts.options && opts.options.length) {
    const slabel = el("label", "rig-sr", opts.selectLabel || "Narrow rows");
    slabel.htmlFor = `${id}-select`;
    select = el("select");
    select.id = `${id}-select`;
    opts.options.forEach((o) => {
      const option = el("option", null, o.label);
      option.value = o.value;
      select.appendChild(option);
    });
    if (opts.onselect) {
      select.addEventListener("change", () => opts.onselect(select.value));
    }
    host.append(slabel, select);
  }

  const count = el("span", "rig-count");
  host.appendChild(count);

  if (opts.onsearch) {
    input.addEventListener("input", () => opts.onsearch(input.value));
  }

  return { input, select, count };
}

/* ── panel ────────────────────────────────────────────────────────────────
 *
 * rigPanel(host, title, note) -> the element to put content in
 *
 * The caveat goes in the heading, which is where pull-report puts it: under
 * the box it reads as applying to whatever comes next.
 */
export function rigPanel(host, title, note) {
  host.classList.add("rig-panel");
  const h = el("h3", "rig-panel-title", title);
  if (note) h.appendChild(el("span", "rig-note", note));
  const content = el("div");
  host.replaceChildren(h, content);
  return content;
}
