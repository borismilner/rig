package main

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// THE WIDTH RULE, APPLIED TO THE TWO READ TABLES THAT DID NOT HAVE IT.
//
// `record query` got it at rig c147419 and `brief`'s section 12 at 072aea4.
// `record history` and `record refs` were still on the unbounded `writeTable`,
// which sizes every column to its widest cell - so one 161-column doc-key id
// started every other row's KIND cell at column 163.
//
// ⛔ THE RULE, IN THE THREE CLAUSES keyedTable APPLIES:
//
//  1. PIN the key column so the prose pays.
//  2. CHOOSE the layout at a default width when there is no terminal.
//  3. CUT a cell only at a REAL terminal. A PIPE IS NEVER CUT.
//
// Measured against a copy of the production store, six 161-column ids citing
// one record, `rig record refs rig`:
//
//	           before   after
//	pipe          263     171   (171 is a bare id, and an id is never cut)
//	pty 120       263     171   over-width non-id lines 14 -> 1
//	pty 80        263     171   over-width non-id lines 14 -> 1
//
// The one that remains is `rig`'s build-skew warning, which is an unwrapped
// 226-column sentence in a different file and is reported rather than fixed
// here.

// longDocKey is the shape that broke both tables: section 39 derives a slug
// from a heading, so a decision imported from a document carries the heading
// as its key. The live production store's longest is 161 characters, and they
// SHARE LONG PREFIXES - which is why eliding one is not merely ugly.
func longDocKey(tail string) string {
	const prefix = "2026-09-11-drafted-recommended-and-never-sent-is-a-third-state-and-it-is-worse-t/"
	return prefix + tail
}

func refsWithLongIDs() Refs {
	return Refs{
		ID: "rig", Depth: 2,
		In: []Ref{
			{
				Src:  longDocKey("a-citation-that-does-not-reach-its-claim-written-by-the-author-of-the-rule"),
				Type: "cites", Via: "rig", Kind: "decision", Distance: 1,
				Title: "A citation that does not reach its claim, written by the author of the rule, and the counting itself is the evidence",
			},
			{
				Src:  longDocKey("a-fourth-repo-logbook-refusal-and-the-counting-itself-is-the-evidence"),
				Type: "cites", Via: "rig", Kind: "decision", Distance: 1,
				Title: "A fourth repo logbook refusal",
			},
			// ⛔ A LONG `VIA` IS WHAT MAKES THE PIPE CLAUSE OBSERVABLE, AND
			// WITHOUT IT THIS FILE HAD A HOLE. VIA is the edge's far end,
			// which past the first hop is another record - so it is an id and
			// can be as long as one. With every non-key column short, the
			// step-out branch has nothing to cut and handing it the DEFAULT
			// width instead of the terminal's produced identical output:
			// measured by mutation, `briefFitAround(cols, budget, nil)`
			// survived until this row existed.
			{
				Src:  longDocKey("the-stub-stamps-every-call-the-ruling-is-in-5d-and-was-never-logged"),
				Type: "cites",
				Via:  longDocKey("the-correction-to-the-leaf-package-option-which-is-why-it-is-not-a-call"),
				Kind: "artefact", Distance: 2,
				Title: "The stub stamps every call",
			},
		},
	}
}

// ⛔ A PIPE IS NEVER CUT, AND THIS IS THE CLAUSE MOST EASILY LOST. briefStyle's
// zero width means unbounded because a pipe's consumer reads every byte;
// passing the DEFAULT to the fit instead of the real width is what put an
// ellipsis into a pipe on brief.go's first run of this code, so it is asserted
// here rather than assumed.
func TestTheReadTablesCutNothingIntoAPipe(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"refs", refsText(refsWithLongIDs(), 0)},
		{"history", historyText("B40", longHistory(), now, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(tc.got, "\u2026") {
				t.Errorf("an ellipsis reached a pipe, so bytes a consumer was "+
					"going to read in full were discarded:\n%s", tc.got)
			}
		})
	}
}

// ⛔ EVERY SRC ID SURVIVES WHOLE AT EVERY WIDTH, WHICH IS THE ONE THING THE
// COLUMN IS FOR. An elided title still reads; an elided id identifies nothing
// and cannot be pasted into `rig record get`.
//
// ⛔ AND THE PREFIX CASE IS THE ONE THAT MADE THIS A DEFECT RATHER THAN A
// PREFERENCE. Measured at 80 columns against the live store, FIVE OF SIX
// governing ids rendered the IDENTICAL stub, because the cut fell inside a
// shared prefix. The fixture above shares 81 characters of prefix on purpose,
// so a renderer that elides produces duplicates and this test says so.
func TestEverySrcIDSurvivesWholeAtEveryWidth(t *testing.T) {
	r := refsWithLongIDs()
	for _, width := range []int{0, 80, 120, 200} {
		t.Run("width="+strconv.Itoa(width), func(t *testing.T) {
			got := refsText(r, width)
			for _, e := range r.In {
				if !strings.Contains(got, e.Src) {
					t.Errorf("the src id %q is not in the rendering whole, so "+
						"it cannot be pasted into `rig record get`:\n%s",
						e.Src, got)
				}
			}
		})
	}
}

// ⛔ EVERY LINE THAT STILL EXCEEDS A REAL TERMINAL IS A BARE ID, and nothing
// else. That is the trade the rule makes: an id overruns and wraps, which
// stays readable and selectable, where every alternative destroys the key.
//
// A LINE WITH A SPACE IN IT IS NOT A BARE ID, which is the test. An
// over-width line carrying two cells is the table failing to fit, and that is
// the defect this whole pass exists to close.
func TestNothingButAnIDOverrunsARealTerminal(t *testing.T) {
	for _, width := range []int{80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			for name, got := range map[string]string{
				"refs":    refsText(refsWithLongIDs(), width),
				"history": historyText("B40", longHistory(), now, width),
			} {
				for i, line := range strings.Split(got, "\n") {
					if utf8.RuneCountInString(line) <= width {
						continue
					}
					if strings.Contains(strings.TrimSpace(line), " ") {
						t.Errorf("%s at %d columns: line %d is %d columns and "+
							"is not a bare id, so the table did not fit:\n%s",
							name, width, i+1,
							utf8.RuneCountInString(line), line)
					}
				}
			}
		})
	}
}

// longHistory is a record whose SUMMARY is prose, which is what made
// `record history` 256 columns: writeTable sized the column to the longest
// one, so a single wordy version set the width of every row.
func longHistory() []Record {
	long := "Row 5's directed messaging inherits an obligation it cannot be " +
		"built without, and the fields existing is NOT the bar: a reference " +
		"assembled from parts nobody can resolve is a reference in name only"
	return []Record{
		record(func(r *Record) {
			r.Version = 1
			r.Prov.Seat = "backend-1"
			r.Fields = map[string]string{titleKey: "short"}
		}),
		record(func(r *Record) {
			r.Version = 2
			r.Prov.Seat = "read-path"
			r.Fields = map[string]string{titleKey: long}
		}),
		record(func(r *Record) {
			r.Version = 3
			r.Prov.Seat = "read-path"
			r.Fields = map[string]string{titleKey: "short again"}
		}),
	}
}

// ⛔ THE CONTROL FOR THE WHOLE FILE: THE TABLE IS STILL A TABLE. Every
// assertion above is satisfiable by a renderer that prints nothing at all, or
// one line per record, so the columns and the rows are asserted separately.
// A collapse guard asserted over a whole rendering staying green against the
// defect it was written for is this seat's predecessor's recorded failure.
func TestTheReadTablesStillRenderTheirColumnsAndRows(t *testing.T) {
	t.Run("refs", func(t *testing.T) {
		got := refsText(refsWithLongIDs(), 80)
		for _, want := range []string{"KIND", "TYPE", "VIA", "HOPS", "decision", "artefact"} {
			if !strings.Contains(got, want) {
				t.Errorf("the refs rendering lost %q:\n%s", want, got)
			}
		}
		if !strings.Contains(got, "3 edges point at rig") {
			t.Errorf("the count line is gone or wrong:\n%s", got)
		}
	})

	t.Run("history", func(t *testing.T) {
		got := historyText("B40", longHistory(), now, 80)
		for _, want := range []string{"VERSION", "SEAT", "EPOCH", "AGE", "SUMMARY", "v1", "v2", "v3", "backend-1"} {
			if !strings.Contains(got, want) {
				t.Errorf("the history rendering lost %q:\n%s", want, got)
			}
		}
		if !strings.Contains(got, "3 versions, oldest first") {
			t.Errorf("the count line is gone or wrong:\n%s", got)
		}
	})
}

// ⛔ THE TITLE BLOCK WAS THE LAST OVERRUNNING THING ON THE PAGE, and it
// survived the table being ruled because it is not a table. `id + two spaces +
// title` is unbounded in BOTH halves at once, so six 161-column ids with
// titles rendered at 263 after the rule had already brought the table down.
//
// The id keeps its own line whole and the title wraps beneath it: neither half
// has to pay, and no id is cut.
func TestTheRefsTitleBlockKeepsTheIDWholeAndWrapsTheTitle(t *testing.T) {
	r := refsWithLongIDs()
	got := refsText(r, 80)

	// The long title must have been broken across lines rather than printed on
	// one. Its opening words are enough to find it.
	if strings.Contains(got, r.In[0].Title) {
		t.Errorf("the title block printed a %d-column title on one line, so "+
			"the block is unwrapped:\n%s",
			utf8.RuneCountInString(r.In[0].Title), got)
	}
	if !strings.Contains(got, "A citation that does not reach") {
		t.Errorf("the title block lost the title entirely, which is not the "+
			"fix - it names the ids so a reader is not left holding a column "+
			"of slugs:\n%s", got)
	}
	// And the id is still whole, on its own line.
	if !strings.Contains(got, "\n  "+r.In[0].Src+"\n") {
		t.Errorf("the src id is not on a line of its own, whole:\n%s", got)
	}
}
