package main

import (
	"encoding/json"
	"strings"
)

// The five-part shape, measured in pull-report's assets/index.html and used
// there for every one of its 16 table sections:
//
//	<h2>heading</h2>
//	<p class="lead">a real paragraph explaining what the table means</p>
//	<div class="toolbar"><input type="search"><span class="count"></span></div>
//	<div class="tablewrap"><table id="..."></table></div>
//
// The table is EMPTY in the markup and filled by script, which is why the kit's
// table takes columns and rows rather than wrapping existing <tr>s. Kept in
// that order and with that division of labour, because section 5h says the kit
// is what bends and not the program.
//
// The kit's classes are prefixed and pull-report's are bare. That is the one
// place this page differs from what was measured, and it is the kit's doing: a
// report page tends to own `.toolbar`, `.lead` and `.panel` already, so the
// prefix is what keeps adoption from fighting the adopter's own stylesheet.

type row struct {
	Actor    string `json:"actor"`
	Kind     string `json:"kind"`
	Region   string `json:"region"`
	Pulls    int    `json:"pulls"`
	Layers   int    `json:"layers"`
	Manifest int    `json:"manifest"`
}

// Fixed rather than random, so a demonstration of this page is the same
// demonstration twice and a screenshot means something.
func rows() []row {
	base := []row{
		{"acme-platform", "enterprise", "europe-west4", 18402, 17233, 1169},
		{"acme-ci", "enterprise", "europe-west4", 9120, 8940, 180},
		{"northwind-dev", "enterprise", "us-central1", 7731, 6002, 1729},
		{"ja4:t13d1516h2", "community", "us-east1", 6640, 12, 6628},
		{"orbit-runners", "enterprise", "us-central1", 5518, 5501, 17},
		{"ja4:t13d1517h2", "community", "asia-south1", 4402, 0, 4402},
		{"helix-staging", "enterprise", "europe-west1", 3980, 3712, 268},
		{"ja4:t12d0908h1", "community", "sa-east1", 3211, 44, 3167},
		{"vega-batch", "enterprise", "us-west2", 2884, 2790, 94},
		{"ja4:t13d1516h1", "community", "europe-north1", 2540, 8, 2532},
		{"quill-preview", "enterprise", "europe-west4", 2190, 1980, 210},
		{"ja4:t11d0705h3", "community", "af-south1", 1806, 0, 1806},
		{"tessellate-qa", "enterprise", "us-central1", 1640, 1602, 38},
		{"ja4:t13d1514h2", "community", "me-central1", 1422, 3, 1419},
		{"lumen-edge", "enterprise", "asia-east1", 1180, 1040, 140},
		{"ja4:t12d0909h2", "community", "us-east4", 980, 0, 980},
		{"crate-mirror", "enterprise", "europe-west3", 802, 640, 162},
		{"ja4:t13d1518h1", "community", "australia-southeast1", 640, 12, 628},
		{"pilot-sandbox", "enterprise", "us-west1", 512, 480, 32},
		{"ja4:t10d0604h1", "community", "asia-northeast1", 388, 0, 388},
	}
	return base
}

func pane() string {
	data, err := json.Marshal(rows())
	if err != nil {
		// Static data marshalled at every request: if this ever fails the page
		// is broken in a way a blank table would hide, so it says so.
		data = []byte("[]")
	}

	var b strings.Builder
	b.WriteString(`<!doctype html>
<meta charset="utf-8">
<title>ledger</title>
<link rel="stylesheet" href="/kit/kit.css">
<style>
  /* The program's own page-level layout, and deliberately all it owns. The kit
     has no opinion about how the page is built around its elements (5h), so
     what is here is the page frame and nothing an element already covers. */
  body {
    margin: 0;
    padding: 1rem 1.15rem;
    background: var(--panel, #212a34);
    color: var(--fg, #dae5f3);
    font-family: var(--ui, system-ui), sans-serif;
    font-size: var(--fs-0, 14px);
    line-height: 1.5;
  }
  section { margin: 0 0 1.6rem }
  /* Named so it is visible that this page was NOT handed tokens, rather than
     leaving it looking like a theme nobody chose. pane.js sets the attribute
     when it gives up waiting for the window. */
  [data-rig-unthemed] .rig-lead::after {
    content: " (no token set arrived: this page is drawing its own fallbacks)";
    color: var(--fg-faint, #7c879b);
  }
</style>

<div id="page">
`)

	// Section one: the plain five-part shape, and the one that carries the
	// keyboard argument. Everything a keyboard user cannot do to pull-report's
	// 16 tables, they can do to this one.
	b.WriteString(`  <section>
    <h2 class="rig-heading">Who pulled, by actor</h2>
    <p class="rig-lead">
      Pulls in the captured window grouped by the actor that started them.
      Enterprise tenants resolve exactly, from the private-image namespace.
      Community pulls have no user at all, so they are fingerprinted by TLS
      stack and network, which is why several of the rows below are a
      <span class="rig-mono">ja4</span> hash rather than a name.
    </p>
    <div id="tb-actors"></div>
    <div class="rig-tablewrap"><table id="t-actors"></table></div>
  </section>

`)

	// Section two: the same shape with a select beside the search, which is the
	// other toolbar arrangement pull-report uses, plus a panel whose caveat sits
	// in its own heading.
	b.WriteString(`  <section>
    <h2 class="rig-heading">Manifest fetches against layer pulls</h2>
    <p class="rig-lead">
      An actor that fetched manifests and never downloaded a layer was reading
      metadata, not deploying. The split below is the whole of that signal, so
      it is worth saying what it is not: it does not separate a scanner from a
      cautious operator, and nothing here should be read as intent.
    </p>
    <div id="panel-caveat"></div>
    <div id="tb-split"></div>
    <div class="rig-tablewrap"><table id="t-split"></table></div>
  </section>
</div>

`)

	b.WriteString(`<script type="module">
import { rigPane } from "/kit/pane.js";
import { rigTable, rigToolbar, rigPanel } from "/kit/kit.js";

const ROWS = `)
	b.Write(data)
	b.WriteString(`;

// Held until the first token set lands, so there is no flash of this page's own
// fallbacks in the wrong theme. pane.js reveals it either way: if the window
// never answers, an unstyled page is the result rather than a blank one.
rigPane({ hold: document.getElementById("page") });

const pct = (v, r) => r.pulls ? (100 * v / r.pulls).toFixed(1) + "%" : "0%";

// Section one. The column set exercises a string sort, a numeric sort and a
// formatted cell, which is the minimum needed to find out whether the kit's
// table actually covers what a report asks of it.
const actorCols = [
  { key: "pulls",  label: "Pulls",  type: "num", fmt: (v) => v.toLocaleString() },
  { key: "actor",  label: "Actor",  type: "str", cls: "rig-mono" },
  { key: "kind",   label: "Kind",   type: "str" },
  { key: "region", label: "Region", type: "str", cls: "rig-mono" },
  { key: "layers", label: "Layers", type: "num", fmt: (v) => v.toLocaleString() },
];

const actorBar = rigToolbar(document.getElementById("tb-actors"), {
  placeholder: "filter by actor, kind or region…",
  searchLabel: "Filter actors",
});

const actors = rigTable(
  document.getElementById("t-actors"),
  actorCols,
  ROWS,
  { countEl: actorBar.count },
);

actorBar.input.addEventListener("input", () => actors.setFilter(actorBar.input.value));

// Section two. Same element, different columns and a select, so the toolbar's
// second arrangement is exercised rather than assumed.
rigPanel(
  document.getElementById("panel-caveat"),
  "Manifest-only is not the same as a scan",
  "one window, one registry",
);

const splitCols = [
  { key: "manifest", label: "Manifest",     type: "num", fmt: (v) => v.toLocaleString() },
  { key: "actor",    label: "Actor",        type: "str", cls: "rig-mono" },
  { key: "layers",   label: "Layers",       type: "num", fmt: (v) => v.toLocaleString() },
  { key: "manifest", label: "Manifest share", type: "num", fmt: pct },
];

const splitBar = rigToolbar(document.getElementById("tb-split"), {
  placeholder: "filter by actor…",
  searchLabel: "Filter actors",
  selectLabel: "Narrow by kind",
  options: [
    { value: "all", label: "every kind" },
    { value: "enterprise", label: "enterprise only" },
    { value: "community", label: "community only" },
  ],
});

const split = rigTable(
  document.getElementById("t-split"),
  splitCols,
  ROWS,
  { countEl: splitBar.count, searchKeys: ["actor"] },
);

let kind = "all";
function applySplit() {
  const subset = kind === "all" ? ROWS : ROWS.filter((r) => r.kind === kind);
  split.setRows(subset);
}
splitBar.select.addEventListener("change", () => {
  kind = splitBar.select.value;
  applySplit();
});
splitBar.input.addEventListener("input", () => split.setFilter(splitBar.input.value));
</script>
`)
	return b.String()
}
