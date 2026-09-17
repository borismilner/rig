// rigseed puts rig's own backlog into rig, through the rig CLI.
//
// ⛔ THE CLI PATH IS THE DEMONSTRATION, NOT A STEP TOWARD ONE. Section 39's MVP
// is "being able to use rig to work on rig with respect to the project/case
// management", and a seeder that reached into the store directly would prove
// the store works while proving nothing about using rig. So every write here is
// an exec of the real binary against the real daemon, and anything the CLI
// refuses, this refuses too.
//
// ⛔ AND IT IS NOT A SECOND PARSER OF BACKLOG.md. record.ParseBacklog was
// promoted out of a test file for exactly this caller (rig 5b606d0), then fixed
// twice (c86ede8, 27a6380, a063ec8) as the document taught it what it is.
// Anything this file learned about the document's shape belongs in that parser,
// never here - two readers of one document that drift is this project's most
// expensive recorded failure.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/boris-milner/rig/internal/record"
)

// callTimeout bounds one CLI invocation. The rehearsal measured 60 puts in
// 0.30s against a live daemon, so this is three orders of magnitude of headroom
// and exists to turn a hung daemon into a failure rather than a wait.
const callTimeout = 30 * time.Second

// fieldFlag is the CLI's repeatable typed-field flag, named once because a
// seeder is mostly a pile of them.
const fieldFlag = "--field"

// fieldStatus is the one field name this file reads back out of what it built,
// so the string is not written twice.
const fieldStatus = "status"

// The two grains a backlog document states an id at.
//
// ⛔ THEY ARE CARRIED ON EVERY RECORD BECAUSE THE EVIDENCE BEHIND THEM IS NOT
// THE SAME. A row has a state cell; a heading has none. A report that merged
// them would hide which grain moved, and B66 exists because a whole grain was
// invisible.
const (
	grainRow     = "row"
	grainHeading = "heading"

	// grainRanked is a row of the critical-path table: inside a work-item
	// table this parser reads, and carrying a RANK where the id goes.
	//
	// ⛔ IT IS ITS OWN GRAIN BECAUSE THE EVIDENCE BEHIND IT IS THINNER THAN
	// EITHER OF THE OTHER TWO, AND A READER WHO CANNOT TELL WILL TRUST IT
	// EQUALLY. A row grain has a state cell and a title; this one reaches
	// neither through `Unimported`, so it arrives with a rank, a line and
	// nothing else. Folding it into `row` would put eleven records with no
	// status in the same set as seventy-eight that have one.
	grainRanked = "ranked-row"
)

// tagHeadingBorne marks a record whose id the document states in a HEADING.
//
// Section 39's migration ruling is import everything and flag what is
// irregular, and `tags` is the field it names for the job. A heading is not
// irregular markup, but it IS a different grain with less behind it, and a
// reader who cannot tell the two apart cannot tell a status the document never
// gave from one something lost.
const tagHeadingBorne = "heading-borne"

// tagRankOnly marks a record the document states with a RANK and nothing this
// seeder can reach.
//
// ⛔ IT IS THE ABSENCE MADE READABLE, WHICH IS THE ONLY HONEST ANSWER WHILE
// `Unimported` CARRIES NO WORK CELL. The eleven rows sit in
// `| # | Work | Seat | State |` and the parser exports the first cell only, so
// the title and the state are not available to this file - and re-deriving
// them by reading BACKLOG.md here would be the second reader of one document
// that this file's header names as the project's most expensive failure. The
// tag is what stops a record with a rank for a title reading as a record whose
// work is called "9".
const tagRankOnly = "rank-only"

// The fields a ranked row carries, named once.
const (
	fieldRank    = "rank"
	fieldSection = "section"
)

// exitDiverged is what --check exits with when the store and the document are
// not set-equal.
//
// ⛔ IT IS NOT 1, AND THE DIFFERENCE IS THE WHOLE POINT. "The check ran and
// found a divergence" and "the check could not answer" are different facts,
// and this repository has recorded ten instances of a check whose failure to
// run could not be told from its result. A caller that only tests for non-zero
// stays correct; one that wants to tell them apart now can.
//
// ⛔ AND IT DOES NOT SURVIVE `go run`, WHICH PRINTS `exit status 2` ON STDERR
// AND RETURNS 1. Build the binary, or read the message rather than the code.
const exitDiverged = 2

func main() {
	err := run()
	var diverged *divergedError
	switch {
	case err == nil:
		return
	case errors.As(err, &diverged):
		fmt.Fprintf(os.Stderr, "rigseed: %v\n", err)
		os.Exit(exitDiverged)
	default:
		fmt.Fprintf(os.Stderr, "rigseed: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	backlog string

	// decisions is the second document this seeder reads, and it is NOT
	// optional.
	//
	// ⛔ AN OPTIONAL PATH THAT DEFAULTS TO SKIPPING IS THE CHECK THAT NEVER
	// RAN, WHICH THIS REPOSITORY HAS RECORDED TEN TIMES. `--check` over the
	// backlog alone prints a full page of empty sets while several hundred
	// rulings sit in no record, and nothing on that page says the second
	// document was never opened. So it is required, it refuses a zero-entry
	// parse exactly as the backlog does, and a run that cannot reach it fails
	// loudly rather than narrowing its own aperture in silence.
	decisions string

	// planDir is the DIRECTORY of section files, and it is the third document
	// this seeder reads.
	//
	// ⛔ IT IS `plan/NN-*.md` AND NEVER `PLAN.md`. `PLAN.md` is GENERATED by
	// `tools/plansplit.py` and is an index of 42 pointers; importing it would
	// put 42 records in the store and let a reader call the specification
	// imported. The section files are the source, and plansplit's own rule -
	// a file's name must match its heading - is a free integrity check on the
	// set this reads.
	//
	// ⛔ AND IT IS REQUIRED, for the reason `decisions` gives: an optional path
	// that defaults to skipping is the check that never ran. `--check` over two
	// of the three documents prints a full page of sets while the whole
	// specification sits in no record, and nothing on that page says the third
	// was never opened.
	planDir string

	project string
	title   string
	estate  string
	rigBin  string
	dryRun  bool
	check   bool
}

func run() error {
	var o options
	flag.StringVar(&o.backlog, "backlog", "BACKLOG.md", "the backlog document to read")
	flag.StringVar(&o.decisions, "decisions", "DECISIONS.md", "the decisions document to read")
	flag.StringVar(&o.planDir, "plan-dir", "plan",
		"the directory of plan/NN-*.md section files to read. NEVER PLAN.md, which is generated")
	flag.StringVar(&o.project, "project", "rig", "the project every record belongs to")
	flag.StringVar(&o.title, "title", "rig", "the project record's title")
	flag.StringVar(&o.estate, "estate", "production", "the estate this MUST be pointed at")
	flag.StringVar(&o.rigBin, "rig", "rig", "the rig binary to drive")
	flag.BoolVar(&o.dryRun, "dry-run", false, "print what would be written and write nothing")
	flag.BoolVar(&o.check, "check", false,
		"write nothing: print the ID SETS where the document and the store disagree, "+
			"and exit 2 if any of them is non-empty")
	flag.Parse()

	// ⛔ THE ESTATE IS VERIFIED BY DIALLING IT, NOT BY TRUSTING A FLAG.
	//
	// "into production, never development" is an instruction a seeder cannot
	// honour by being told which one it is: the estate a CLI reaches is decided
	// by XDG_RUNTIME_DIR, which this process does not set and cannot see the
	// consequences of. So it asks the daemon who it is and refuses on a
	// mismatch. A seeder that wrote rig's whole backlog into the wrong store
	// would be discovered by its absence from the right one.
	name, err := dialEstate(o)
	if err != nil {
		return err
	}
	if name != o.estate {
		return fmt.Errorf("this shell reaches the %q estate and --estate says %q.\n"+
			"       rigseed will not write rig's backlog into an estate nobody asked for.\n"+
			"       set XDG_RUNTIME_DIR to the one you mean, or pass --estate %s if you mean it",
			name, o.estate, name)
	}

	doc, err := readBacklog(o.backlog)
	if err != nil {
		return err
	}
	if len(doc.Items) == 0 {
		return fmt.Errorf("%s parsed to zero work items, which is never right for this "+
			"document - the parser or the path is wrong, and writing nothing is the safe answer", o.backlog)
	}
	// ⛔ ONE FILE CANNOT BE BOTH DOCUMENTS, AND THE FAILURE IS SILENT WITHOUT
	// THIS. Pointed at the same path twice, the backlog parser and the
	// decisions parser both read it and both succeed: the rows arrive keyed on
	// `B\d+` and the headings arrive keyed on title slugs, so no id collides,
	// nothing is reported, and the store quietly holds every heading of
	// BACKLOG.md as a ruling. `--check` would then agree with itself about a
	// set that is wrong, which is the one shape this instrument must not have.
	if o.backlog == o.decisions {
		return fmt.Errorf("--backlog and --decisions are both %q.\n"+
			"       One file cannot be both documents: each parser would succeed on it, "+
			"the ids would not collide,\n"+
			"       and the store would hold every heading of it as a ruling with "+
			"nothing reporting that", o.backlog)
	}

	dec, err := readDecisions(o.decisions)
	if err != nil {
		return err
	}
	if len(dec.Decisions) == 0 {
		return fmt.Errorf("%s parsed to zero entries, which is never right for this "+
			"document - the parser or the path is wrong, and writing nothing is the safe answer", o.decisions)
	}

	pp, err := readPlan(o.planDir)
	if err != nil {
		return err
	}
	if len(pp.Entries) == 0 {
		return fmt.Errorf("%s parsed to zero headings, which is never right for the "+
			"specification - the parser or the path is wrong, and writing nothing is "+
			"the safe answer", o.planDir)
	}
	p := planFor(o, doc, dec, pp)

	// ⛔ THE CHECK READS THE SAME PLAN THE SEEDER WOULD WRITE, AND THAT IS WHAT
	// MAKES IT AN ANSWER RATHER THAN A SECOND OPINION. A detector with its own
	// reading of BACKLOG.md would be the second parser this file's header
	// forbids, and the two would drift exactly where it matters.
	if o.check {
		return runCheck(o, p, name)
	}

	fmt.Printf("%s -> %d records (%s), into the %s estate as project %q\n\n",
		o.sources(), len(p.want), p.grains(), name, o.project)

	// ⛔ THE PROJECT IS ITS OWN RECORD AND WITHOUT IT THE BRIEF HAS NO HEADER.
	//
	// Section 39: "CORRECTION: project is its own KIND, one record per project",
	// and "a project's id is its SLUG... never a UUIDv7 - it is already a path
	// component and a human types it". Seeding only the work items produced a
	// brief whose first two lines read "rig (not said) (no status)" and
	// "(no title)" - every row correct underneath a header that knew nothing.
	// Measured on the first real seeding rather than reasoned about.
	if err := seedProject(o); err != nil {
		return fmt.Errorf("the project record: %w", err)
	}

	r := &result{}
	for _, in := range p.want {
		if err := seedOne(o, in, r); err != nil {
			return fmt.Errorf("%s: %w", in.id, err)
		}
	}

	// ⛔ EVERY RECORD FIRST, THEN EVERY EDGE, AND THE TWO PASSES ARE NOT
	// TIDINESS. internal/record's checkEdge refuses a link whose either end is
	// missing, and the document states B46's children in a table that PRECEDES
	// nothing - B46 itself is a heading further up, but nothing guarantees
	// that for the next parent somebody writes. Two passes is what stops the
	// order of the document deciding whether the edges land.
	for _, in := range p.want {
		if in.partOf == "" {
			continue
		}
		if err := o.linkPartOf(in.id, in.partOf, r); err != nil {
			return fmt.Errorf("%s part-of %s: %w", in.id, in.partOf, err)
		}
	}

	// ⛔ THE EDGE SET IS CONVERGED AND NOT ONLY EXTENDED, AND IT RUNS AFTER THE
	// LINKING PASS SO THE STORE IT READS IS THE ONE THIS RUN JUST LEFT. Edges
	// are the only thing B77 leaves this seeder able to take back, so an edge
	// an earlier generation wrote and the documents no longer state is the one
	// class of wrong fact it can actually clear.
	if err := o.retractEdges(p, r); err != nil {
		return fmt.Errorf("taking back the edges the documents no longer state: %w", err)
	}

	r.report(os.Stdout, o)
	p.report(os.Stdout)
	return nil
}

// sources names the documents this run read, in one place.
//
// ⛔ IT IS ONE FUNCTION BECAUSE THE HEADER LINE AND THE REFUSAL LINE BOTH PRINT
// IT, AND THEY DRIFTED THE MOMENT A THIRD DOCUMENT ARRIVED. A report whose
// header names two documents above a set drawn from three is a caption that is
// precise and false.
func (o options) sources() string {
	return o.backlog + " + " + o.decisions + " + " + o.planDir + "/"
}

// dialEstate asks the daemon which estate this shell actually reaches.
func dialEstate(o options) (string, error) {
	out, err := capture(o, "estate", "--json")
	if err != nil {
		return "", fmt.Errorf("could not reach a daemon to ask which estate this is: %w", err)
	}
	var e struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &e); err != nil {
		return "", fmt.Errorf("rig estate --json did not parse: %w", err)
	}
	if e.Name == "" {
		return "", errors.New("rig estate --json answered with no name")
	}
	return e.Name, nil
}

// seedProject writes the one record that gives the brief its header.
//
// The metadata is section 39's exhaustive list for a project, and the fields it
// leaves out are left out deliberately: principles, owner and target_date are
// not facts this document states, and inventing them here would put a claim in
// the store that nothing outside it supports.
func seedProject(o options) error {
	version, _, err := currentVersion(o, o.project)
	if err != nil {
		return err
	}
	return o.write([]string{
		"record", "put",
		"--kind", "project",
		"--project", o.project,
		"--id", o.project,
		"--body", o.title,
		fieldFlag, "title=" + o.title,
		fieldFlag, "description_short=" + o.title,
		fieldFlag, "status=active",
		fieldFlag, "source=" + o.backlog,
		"--if-version", strconv.FormatUint(version, 10),
	})
}

// readBacklog reads the whole document, not only the rows it could import.
//
// ⛔ IT CALLS ParseBacklogDocument, AND THE OLD CALL TO ParseBacklog IS THE
// DEFECT B66 NAMES. ParseBacklog answers with the rows and says nothing about
// what it left behind, so this seeder was blind to eighteen things the document
// addresses - including B46, which is Boris's own MVP acceptance test. An
// absence no instrument can report survives every review.
func readBacklog(path string) (record.BacklogParse, error) {
	f, err := os.Open(path)
	if err != nil {
		return record.BacklogParse{}, err
	}
	defer f.Close()
	return record.ParseBacklogDocument(f)
}

// seedOne writes one row, and it is written to be RE-RUNNABLE.
//
// ⛔ A PUT WITH --if-version 0 MEANS CREATE, AND IT REFUSES ON AN ID THAT
// EXISTS. The cli seat measured that refusal on a live daemon: "record B1
// already exists at version 1: a put with IfVersion 0 creates, and this id is
// taken". It is correct behaviour and it makes a naive seeder single-shot - a
// partial failure would leave the operator unable to re-run without a wipe.
// So every row is PROBED first and supersedes whatever version is there.
func seedOne(o options, in intent, r *result) error {
	version, exists, err := currentVersion(o, in.id)
	if err != nil {
		return err
	}

	after, err := o.putRow(intentArgs(o, in, version))
	if err != nil {
		return err
	}
	r.count(exists, o.dryRun, version, after)

	// ⛔ A ROW THE DOCUMENT CLOSED GETS NO progress.step, AND stepStateFor
	// CARRIES THE ARGUMENT. The closure is in `status` and its reason is in
	// `closure_note`, both written by the put above, so this loop has exactly
	// one write per row and re-running it changes nothing.
	//
	// IT READS THE FIELD THAT WAS ACTUALLY BUILT rather than re-deriving the
	// disposition, so "what the store was told" and "what this report says"
	// cannot come apart. A heading-borne record has no status at all and
	// reaches neither arm, which is the honest answer for a grain the document
	// states no state for.
	switch in.fields[fieldStatus] {
	case record.StatusClosed, record.StatusClosedByRuling:
		r.closed = append(r.closed, in.id)
	}
	r.collect(in)
	return nil
}

// putArgs is the whole CLI call one ROW becomes. It keeps its own name because
// the tests read it directly and because the row grain is the one with a state
// cell behind it; intentArgs is the general form both grains go through.
func putArgs(o options, it record.BacklogItem, version uint64) []string {
	return intentArgs(o, rowIntent(o, it), version)
}

// intentArgs is the whole CLI call one record becomes, built in one place so a
// test can read it without a daemon.
func intentArgs(o options, in intent, version uint64) []string {
	args := []string{
		"record", "put",
		"--kind", in.kind,
		"--project", o.project,
		"--id", in.id,
		"--body", in.body,
	}
	// ⛔ SORTED, SO TWO RUNS OVER ONE DOCUMENT BUILD THE SAME CALL. Go
	// randomises map iteration on purpose, and an argument list that reordered
	// between runs would make a --dry-run diff unreadable and this seeder's
	// idempotence untestable.
	for _, name := range sortedKeys(in.fields) {
		args = append(args, fieldFlag, name+"="+in.fields[name])
	}
	return append(args, "--if-version", strconv.FormatUint(version, 10))
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// disposition is how the DOCUMENT closed a row, or "" for a row it left open.
//
// ⛔ THIS IS THE LINE THAT INVERTED B19. It read `status=active` for every row,
// unconditionally, so a claim the lead RETRACTED as falsified was published as
// live work with the falsified sentence as its title, and B55 and B56 - closed
// by a ruling - stood in the brief's open list. A store that says the opposite
// of its own document about the two rows where trust is decided is worse than
// no store.
//
// ⛔ AND IT COARSENS RATHER THAN GUESSES. `backlog.go` reads four terminal
// words - DONE, CLOSED, REJECTED, RETRACTED - and `BacklogItem` exports none
// of them: it exports five booleans, and B19's RETRACTED is a SECOND bold run
// in the item cell that `boldLead` never reaches. So the honest answer here is
// `closed`, which is true of all four and asserts only that the work is over.
// Recovering which word it was needs one exported field on the PARSER; reading
// it back out of the title in this file would be the second parser of
// BACKLOG.md that this file's own header forbids.
func disposition(it record.BacklogItem) string {
	switch {
	// A ruling first, because it is the more specific fact. The parser cannot
	// set both today - RuledClosed implies ClaimsDone implies !Done - and the
	// order is here so that a later parser change cannot silently downgrade a
	// ruling into a plain closure.
	case it.RuledClosed:
		return record.StatusClosedByRuling
	case it.Done:
		return record.StatusClosed
	default:
		return ""
	}
}

// statusFor is the stored status one row gets: its disposition, or active.
func statusFor(it record.BacklogItem) string {
	if d := disposition(it); d != "" {
		return d
	}
	return record.StatusActive
}

// stepStateFor is the progress state one row gets, and a row this seeder
// IMPORTS rather than WORKS gets none.
//
// ⛔ IT RETURNS "" FOR EVERY SHAPE, AND THAT IS THE ANSWER RATHER THAN A STUB.
// It filed `done` against every closed row, which asserts the work was
// COMPLETED - false for B19, RETRACTED as falsified, and for anything the
// document REJECTED. The honest word cannot be sent: `StepState` in wire.proto
// is STARTED, BLOCKED, DONE, and cmd/rig refuses anything else before the call
// leaves. A live seeding run against a throwaway estate stopped on B55 proving
// it. So the disposition goes to `status`, its reason goes to `closure_note`,
// and the stream stays what section 39 says it is: the live state of an item
// somebody is WORKING. A row that arrived already closed was never picked up
// here and has no live state to report.
//
// ⛔ AND IT MAKES THE SEEDER IDEMPOTENT. The step was the one append-only
// write in a re-runnable program, so a second run filed a second closure
// against every closed row. That whole paragraph of caveat goes with it.
func stepStateFor(_ record.BacklogItem) string { return "" }

// putRow writes one row and returns the version the store ended up at.
//
// ⛔ IT ASKS, RATHER THAN ASSUMING head+1. Since the store stopped minting a
// version for a put whose content already matches the head, "I wrote it" and
// "it changed" are different answers, and a seeder that printed "superseded
// 68" over 68 no-ops would be making exactly the kind of false statement in
// print that this seeder was just repaired for.
func (o options) putRow(args []string) (uint64, error) {
	if o.dryRun {
		fmt.Printf("  %s %s\n", o.rigBin, strings.Join(args, " "))
		return 0, nil
	}
	withJSON := make([]string, 0, len(args)+1)
	withJSON = append(withJSON, args...)
	withJSON = append(withJSON, "--json")

	out, err := capture(o, withJSON...)
	if err != nil {
		// err CARRIES THE STDERR and `out` carries whatever the refusal put on
		// stdout. Both are printed because they are different halves: the
		// refusal body says what rig refused, the stderr says what it warned.
		return 0, fmt.Errorf("%s %s: %w\n%s",
			o.rigBin, strings.Join(withJSON, " "), err, out)
	}
	var rec struct {
		Version uint64 `json:"version"`
	}
	if err := json.Unmarshal(out, &rec); err != nil {
		return 0, fmt.Errorf("rig record put --json did not parse: %w\n%s", err, out)
	}
	return rec.Version, nil
}

// currentVersion probes for an existing record. A missing id is a normal
// result, not an error: rig answers CODE_NOT_FOUND and exits 1, and 0 is the
// version that means "create".
func currentVersion(o options, id string) (version uint64, exists bool, err error) {
	out, err := capture(o, "record", "get", id, "--json")
	if err != nil {
		var refusal struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(out, &refusal) == nil && refusal.Code == "CODE_NOT_FOUND" {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("probing for an existing record failed: %w\n%s", err, out)
	}
	var rec struct {
		Version uint64 `json:"version"`
	}
	if err := json.Unmarshal(out, &rec); err != nil {
		return 0, false, fmt.Errorf("rig record get --json did not parse: %w", err)
	}
	return rec.Version, true, nil
}

// tagsFor flags what is irregular about a row rather than tidying it.
//
// ⛔ SECTION 39'S MIGRATION RULING IS IMPORT EVERYTHING AND FLAG WHAT IS
// IRREGULAR, and tags is the field it gives for the job - "free-form grouping,
// queryable like any typed field". A row this seeder quietly normalised would
// be a row nobody ever fixes.
func tagsFor(it record.BacklogItem) string {
	var t []string
	if it.Malformed {
		t = append(t, "malformed")
	}
	if it.ClaimsDone {
		t = append(t, "claims-done-unstruck")
	}
	if it.RuledClosed {
		t = append(t, "closed-by-ruling")
	}
	return strings.Join(t, ",")
}

// closureNote says HOW the document closed a row, which is the one thing the
// coarsened disposition cannot carry. It is "" for a row still open.
func closureNote(it record.BacklogItem) string {
	switch {
	case it.RuledClosed:
		return "closed in BACKLOG.md by a ruling rather than by work"
	case !it.Done:
		return ""
	case it.Struck:
		return "closed in BACKLOG.md by a struck title"
	default:
		return "closed in BACKLOG.md by a terminal disposition in its item cell"
	}
}

// write runs one CLI call, or prints it under --dry-run.
func (o options) write(args []string) error {
	if o.dryRun {
		fmt.Printf("  %s %s\n", o.rigBin, strings.Join(args, " "))
		return nil
	}
	out, err := capture(o, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s",
			o.rigBin, strings.Join(args, " "), err, out)
	}
	return nil
}

// capture runs one `rig` call and returns ITS STDOUT, with stderr folded into
// the error and nowhere else.
//
// ⛔ IT WAS `CombinedOutput` AND THAT PUT rig's DIAGNOSTICS INSIDE ITS DATA.
// Measured 2026-09-17: `rigseed --check` died with
// `rig estate --json did not parse: invalid character 'r'`, because `rig` had
// correctly written a build-skew warning to STDERR and this merged the two
// streams before the JSON decoder saw them. **Nothing was wrong with `rig`** -
// it separates them exactly as it should, and `rig estate --json 2>/dev/null`
// is clean.
//
// ⛔ THE TRIGGER IS THE NORMAL CASE, NOT AN EDGE ONE. A build-skew warning
// is what an unstamped or mismatched client prints, which is precisely the
// state a machine is in while rig is being developed - so the parse broke on
// the day somebody seeded from a working tree, and would have kept working in
// every test, where the binary under test prints nothing.
//
// On failure the two are joined deliberately: an operator reading a failed
// call wants whatever rig said, on either stream.
// ⛔ STDOUT COMES BACK ALONE AND STDERR RIDES THE ERROR, WHICH IS NOT A TIDYING
// UP. This returned stdout and stderr CONCATENATED on any non-zero exit, and
// every caller here passes --json and then unmarshals what it gets back - so a
// single line on stderr made the JSON unparseable and defeated the two callers
// whose whole job is to recognise a CODE_NOT_FOUND refusal.
//
// ⛔ IT ONLY EVER BIT AN UNSTAMPED BUILD, WHICH IS WHY IT SURVIVED. `rig` warns
// on stderr that it cannot check build skew when it is not a stamped build -
// so a seeder driven by an installed rig was fine, and a seeder driven by a
// `go build` binary could not seed an EMPTY estate at all: the first probe for
// a record that does not exist yet came back as a hard failure. That is the
// exact path the store-isolation procedure requires before production is
// touched, so the defect was reachable only by the people being careful.
//
// Measured 2026-09-17: `rigseed -estate=development` against a fresh isolated
// store, "probing for an existing record failed: exit status 1" with a
// well-formed CODE_NOT_FOUND body printed directly underneath it.
func capture(o options, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.rigBin, args...)
	var errOut strings.Builder
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil && errOut.Len() > 0 {
		return out, fmt.Errorf("%w\n%s", err, errOut.String())
	}
	return out, err
}

// result is what the operator is told, and the three lists exist because a
// count would hide them.
type result struct {
	created    int
	superseded int
	unchanged  int
	closed     []string
	claimsDone []string
	ruled      []string
	malformed  []string

	// headings is the ids the document states in a HEADING rather than in a
	// row. They were in no record at all until this seeder learned to read
	// them, so they are NAMED on every run rather than folded into a total.
	headings []string

	// ranked is the ids of the ordered table's rows, which carry a rank and
	// no state.
	ranked []string

	// notes is the ids written at kind `note` - the decisions document's
	// standing sections.
	//
	// ⛔ THEY ARE NAMED ON EVERY RUN BECAUSE `kind` REFUSES NOTHING. A ruling
	// written at the wrong kind is accepted silently and is reachable only by
	// a caller who already knows to ask for it, so the one place the choice
	// can be caught is a report that says out loud which records were not
	// written as decisions. Eight today, and the set is what makes a ninth
	// visible.
	notes []string

	// linked is every part-of edge asserted, written whole - src, type, dst -
	// because a column of ids gives a reader no way to know which side of the
	// arrow they are on.
	linked []string

	// unlinked is every part-of edge TAKEN BACK, written the same way.
	//
	// ⛔ IT IS THE ONE THING B77 LEAVES THIS SEEDER ABLE TO UNDO. rig can unlink
	// an edge and cannot retract a record, so a record the document stops
	// stating survives as EXTRA for ever while an edge it stops stating can be
	// removed. Naming them is not tidiness: an unlink is the only destructive
	// thing a seeding run does, and a run that did it silently would be a run
	// nobody could audit.
	unlinked []string
}

// count records what one put DID, which since the store stopped versioning a
// no-op is no longer the same question as whether a call was made.
//
// A DRY RUN CANNOT KNOW AND SAYS SO BY STAYING COARSE. It never reaches the
// store, so it has no `after` to compare; reporting the finer answer from a
// call that did not happen would be a guess printed as a measurement.
func (r *result) count(exists, dryRun bool, before, after uint64) {
	switch {
	case !exists:
		r.created++
	case dryRun:
		r.superseded++
	case after == before:
		r.unchanged++
	default:
		r.superseded++
	}
}

func (r *result) collect(in intent) {
	if in.grain == grainHeading {
		r.headings = append(r.headings, in.id)
	}
	if in.grain == grainRanked {
		r.ranked = append(r.ranked, in.id)
	}
	if in.kind == record.KindNote {
		r.notes = append(r.notes, in.id)
	}
	it := in.row

	// ⛔ A RULED ROW IS NOT SEEDED OPEN ANY MORE, SO IT IS NOT IN THE LIST
	// LABELLED "SEEDED OPEN". The parser sets ClaimsDone on a ruled row too,
	// and reporting it under both labels would make one of them a false
	// caption - the defect backlog.go's Struck field exists to record.
	if it.ClaimsDone && !it.RuledClosed {
		r.claimsDone = append(r.claimsDone, it.ID)
	}
	if it.RuledClosed {
		r.ruled = append(r.ruled, it.ID)
	}
	if it.Malformed {
		r.malformed = append(r.malformed, it.ID)
	}
}

// report names the rows a human has to look at.
//
// ⛔ THE ROWS THAT CLAIM A TERMINAL STATE WITHOUT BEING STRUCK ARE SEEDED OPEN,
// AND THAT IS THE RULING RATHER THAN A DEFECT - the document decides, not the
// parser. But they are the rows a reader would call closed, so they are NAMED
// here rather than left to be discovered in a brief. A seeder that reported
// only totals would have hidden all three of these lists.
func (r *result) report(w *os.File, o options) {
	verb := "wrote"
	if o.dryRun {
		verb = "would write"
	}
	fmt.Fprintf(w, "\n%s %d new and superseded %d, and %d arrived already closed.\n",
		verb, r.created, r.superseded, len(r.closed))
	if r.unchanged > 0 {
		fmt.Fprintf(w, "  %d row(s) were already exactly what this run would have written,\n"+
			"      so the store minted no version for them. That is the seeder being\n"+
			"      idempotent rather than simulating it.\n", r.unchanged)
	}

	name := func(label string, ids []string, why string) {
		if len(ids) == 0 {
			return
		}
		sort.Strings(ids)
		fmt.Fprintf(w, "\n  %s: %s\n      %s\n", label, strings.Join(ids, " "), why)
	}
	name("SEEDED OPEN WHILE CLAIMING A TERMINAL STATE", r.claimsDone,
		"their state cell leads with a terminal word and their title is not struck.\n"+
			"      The document decides, not the parser, so they are OPEN until it says otherwise.")
	name("CLOSED BY A RULING RATHER THAN BY WORK", r.ruled,
		"a tick in the id cell and a terminal state, with the item cell open.\n"+
			"      No work was finished; a decision was taken, and they are seeded\n"+
			"      closed-by-ruling rather than as live work.")
	name("IRREGULAR MARKUP, IMPORTED ANYWAY AND FLAGGED", r.malformed,
		"the row's markdown is irregular enough that its cells ran together.\n"+
			"      Imported with tags=malformed so the document's owner can mend it.")
	name("STATED IN A HEADING RATHER THAN IN A ROW", r.headings,
		"the document gives these an id but no table row, so they have NO STATE\n"+
			"      CELL and this seeder writes no status for them. tags=heading-borne\n"+
			"      is what makes the absence readable rather than lost.")
	name("A RANK WHERE AN ID GOES, AND NO STATE CELL THIS SEEDER CAN REACH", r.ranked,
		"the ordered table states a rank, not a `B` id, and no `B` id was minted.\n"+
			"      The id names its table so a second unnumbered table cannot collide\n"+
			"      with it silently, and the rank is carried as a field rather than\n"+
			"      mapped onto blocks or priority.\n"+
			"      ⛔ THEY CARRY NO status: `Unimported` does not export the State cell,\n"+
			"      so writing one would be a guess. The brief's open list selects\n"+
			"      status == \"active\" and therefore CANNOT SHOW THESE ELEVEN.")
	name("WRITTEN AS A NOTE RATHER THAN AS A DECISION", r.notes,
		"a standing section of the decisions document: what rig is, what is still\n"+
			"      open, what has not been done. It is a record attached to the project\n"+
			"      rather than a ruling, which is section 39's own definition of a note.\n"+
			"      Each one is part-of the project, which is the edge section 3 of the\n"+
			"      brief joins on - so these ARE in `rig brief "+o.project+"`.")
	name("PART-OF EDGES ASSERTED", r.linked,
		"the sub-letter in the id is the document stating a parent, or a note\n"+
			"      naming the project it is attached to. The store's Link is idempotent\n"+
			"      by contract, so a re-run asserts the same fact and changes nothing.")
	name("PART-OF EDGES TAKEN BACK", r.unlinked,
		"the store held these and neither document states them. Both ends are\n"+
			"      records this seeder writes, so they are its own earlier work and\n"+
			"      nobody else's. An edge with an end this seeder does not write is\n"+
			"      left exactly where it is.")
}

// ---------------------------------------------------------------------------
// WHAT THE DOCUMENT ASKS FOR
// ---------------------------------------------------------------------------

// intent is one record this seeder will write.
//
// ⛔ IT IS THE SINGLE STATEMENT OF WHAT THE DOCUMENT ASKS FOR, AND THAT IS WHAT
// MAKES --check AN ANSWER RATHER THAN A SECOND OPINION. The detector compares
// the store against the intents the seeder would write - not against its own
// reading of BACKLOG.md. Two readers of one document that drift is the failure
// this file's header names as the project's most expensive, and a guard that
// drifts from the thing it guards is worse than no guard.
type intent struct {
	id string

	// kind is the record kind this becomes: one of section 39's ten, never a
	// value invented here.
	//
	// ⛔ IT IS A FIELD BECAUSE `kind` REFUSES NOTHING. `Put` accepts any
	// string, so a kind that is not one of the ten is written silently and is
	// reachable only by a caller who already knows to ask for it. Carrying it
	// on the intent puts every kind this seeder writes in one place a reader
	// can enumerate, instead of in a literal inside the argument builder.
	kind string

	grain string
	title string

	// body is what goes to `--body`. On a backlog row it is the title, which
	// is all the document gives; on a decisions entry it is the entry's prose,
	// which is the substance of the ruling.
	body string

	// fields is the typed fields this record carries, by name. A map rather
	// than a built argument list because the store answers with a map, and a
	// comparison between the two should not have to parse anything.
	fields map[string]string

	// partOf is the id the document states this record is a sub-task of, or "".
	partOf string

	// row is the parsed row this came from, zero for the heading grain. It is
	// the EVIDENCE; fields is the projection of it, and the report that names
	// irregular rows reads the evidence rather than re-deriving it.
	row record.BacklogItem
}

// plan is everything one document asks of this seeder.
type plan struct {
	// want is every record to write, at both grains, in document order.
	want []intent

	// unimported is everything EITHER document addresses that this seeder does
	// NOT write, each carrying the document it came from.
	unimported []docUnimported

	// collided is an id the document states at BOTH grains. Reported, never
	// written twice.
	collided []string

	// unnested is a heading sitting under an id-bearing heading whose own id
	// does not agree with it. Reported, and no edge is invented.
	unnested []string

	// orphaned is a part-of the document states towards an id the document
	// never defines. Reported, and the edge is NOT attempted.
	orphaned []string

	// detached is a part-of the document states that this seeder deliberately
	// does not assert, written whole.
	//
	// ⛔ IT IS A RULED EXCLUSION AND NOT A DIVERGENCE, which is why it is its
	// own set rather than another line in `orphaned`. An orphan names a defect
	// in the DOCUMENT - a parent nothing defines - and wants mending; this one
	// names a decision the team-lead took about a parent that exists. Folding
	// the two together would make a standing ruling read as an open fault on
	// every run, and a report whose faults never clear is a report nobody
	// reads.
	detached []string
}

// planFor splits everything the document addresses into what this seeder writes
// and what it leaves alone.
//
// ⛔ THE SWITCH BELOW REPORTS IN ITS `default`, AND THAT IS THE PROPERTY TO
// KEEP RATHER THAN THE SHAPE. A new UnimportedKind added in internal/record
// must land somewhere LOUD, so the arms name what this seeder WRITES and
// everything else falls through to the report. B66 is precisely a class being
// invisible by construction, and a switch that silently accepted a new member
// would be that same defect one layer up.
func planFor(o options, doc record.BacklogParse, dec record.DecisionParse, pp record.PlanParse) plan {
	var p plan
	stated := make(map[string]bool, len(doc.Items))

	for _, it := range doc.Items {
		stated[it.ID] = true
		p.want = append(p.want, rowIntent(o, it))
	}

	for _, u := range doc.Unimported {
		switch u.Kind {
		case record.UnimportedHeading:
			// ⛔ AN ID STATED AT BOTH GRAINS IS REPORTED, NEVER WRITTEN TWICE.
			// Two puts against one id in one run supersede each other and the
			// store keeps whichever went last, so which content wins would be
			// decided by document order - a silent wrong answer. The parser's
			// own reconcile already drops this case; this is the belt that says
			// so out loud the day it stops.
			if stated[u.ID] {
				p.collided = append(p.collided, u.ID)
				continue
			}
			stated[u.ID] = true
			parent, agreed := headingParent(u)
			if !agreed {
				p.unnested = append(p.unnested, u.ID+" sits under "+u.Under)
			}
			p.want = append(p.want, headingIntent(o, u, parent))
		case record.UnimportedRowWithoutID:
			p.addRankedRow(o, u, stated)
		default:
			p.unimported = append(p.unimported, docUnimported{doc: o.backlog, Unimported: u})
		}
	}

	// ⛔ THE DECISIONS DOCUMENT IS FOLDED IN BEFORE THE EDGE SWEEP BELOW, NOT
	// AFTER IT. Its sub-headings state a part-of towards a parent in the SAME
	// document, and a sweep that had already run would leave 292 edges pointing
	// at ids it never saw declared - every one of them reported as orphaned and
	// silently dropped. One plan, one `stated` set, one sweep.
	p.addDecisions(o, dec, stated)

	// ⛔ AND THE SPECIFICATION LAST, INTO THE SAME `stated` SET. Three documents
	// keyed independently are three chances to claim one id, and the store
	// knows nothing about which document a record came from.
	p.addPlan(o, pp, stated)

	// ⛔ AN EDGE TOWARDS AN ID THE DOCUMENT NEVER DEFINES IS REPORTED, NEVER
	// ATTEMPTED. internal/record's checkEdge refuses a link with a missing end,
	// so asserting one would abort a seeding run half-written - and it is a
	// defect in the DOCUMENT, not a failure of this program. Before B46 became
	// a record its six children named exactly such a parent, so this is the
	// shape the run is walking out of rather than a hypothetical.
	//
	// ⛔ THE PROJECT RECORD IS DEFINED AND IS IN NO INTENT. `seedProject` writes
	// it before any row, so it is a real record, but it never enters `stated`
	// because nothing in either document states it. Without this arm the sweep
	// would strip every note's edge to the project the moment it was given one,
	// and report nine orphans against a record that is right there.
	for i := range p.want {
		parent := p.want[i].partOf
		if parent == "" || parent == o.project || stated[parent] {
			continue
		}
		p.orphaned = append(p.orphaned, edgeName(p.want[i].id, parent))
		p.want[i].partOf = ""
	}
	sort.Strings(p.orphaned)
	sort.Strings(p.detached)
	return p
}

// grains says how many records came from each grain, for the one header line.
//
// ⛔ IT COUNTS WHAT THE PLAN ACTUALLY HOLDS RATHER THAN A FIXED LIST OF TWO.
// The old form asked "is it a heading, else it is a row", so a third grain
// added to this seeder would have been counted as rows and been invisible in
// the one line a reader sees first. A whole grain being invisible by
// construction is B66, and a header that cannot name a new one is that defect
// one layer up.
func (p plan) grains() string {
	n := map[string]int{}
	for _, in := range p.want {
		n[in.grain]++
	}
	names := make([]string, 0, len(n))
	for g := range n {
		names = append(names, g)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, g := range names {
		parts = append(parts, fmt.Sprintf("%d %s", n[g], g))
	}
	return strings.Join(parts, ", ")
}

// rowIntent is the record one table row becomes.
func rowIntent(o options, it record.BacklogItem) intent {
	f := map[string]string{
		"title":             it.Title,
		"description_short": it.Title,
		fieldStatus:         statusFor(it),
		"source":            o.backlog,
	}
	// HOW the document closed it, which the coarsened status cannot carry.
	if note := closureNote(it); note != "" {
		f["closure_note"] = note
	}
	if tags := tagsFor(it); tags != "" {
		f["tags"] = tags
	}
	return intent{
		id: it.ID, kind: record.KindWorkItem, grain: grainRow,
		title: it.Title, body: it.Title, fields: f, partOf: it.PartOf, row: it,
	}
}

// headingIntent is the record an id stated in a HEADING becomes.
//
// ⛔ IT WRITES `status`, AND THE PREVIOUS ANSWER - THAT A HEADING HAS NO
// STATE CELL SO THE DOCUMENT SAYS NOTHING - WAS OVERTURNED THE SAME DAY IT WAS
// WRITTEN, BY THE LEAD, ON EVIDENCE. **The document does say it, at a grain
// that is not a cell.** Its own closure convention is the strikethrough - B7's
// state cell reads "done, struck not deleted" - and `BacklogItem.Done` reads
// exactly that mark on a row. Applying a convention the document states to a
// second grain is READING the document. The thing correctly refused was
// different: inferring a parent's state from its children's, which is a seat
// judging and is still refused.
//
// ⛔ THE COST OF THE OLD ANSWER WAS THE ROW'S WHOLE POINT. B66 exists
// because B46 - Boris's own MVP acceptance test - is in no record; a record
// with no `status` is absent from the brief's open list (`brief.go` selects
// `status == "active"`), so it was imported into invisibility and the row
// would have read as closed while its complaint stood.
//
// ⛔ AND THE CLOSED BRANCH IS PINNED SYNTHETICALLY AND BY NOTHING LIVE: no
// heading in rig's own backlog is struck today, so the live document exercises
// `active` and never `closed`. Said out loud, because a predicate only ever
// seen from one side is half-tested.
func headingIntent(o options, u record.Unimported, parent string) intent {
	// ⛔ THE TITLE IS THE PARSER'S DERIVED ONE, NOT ITS VERBATIM LABEL. It
	// was `u.Label` and it seeded B46 as `⛔ B46 - THE MVP ACCEPTANCE TEST,
	// AND IT EXISTED IN NO DOCUMENT AT ALL`, decoration and its own id
	// included, where every row title goes through `titleOf`. Two grains
	// rendering one document two ways. `Label` stays verbatim because a report
	// quoting the document has to quote it; `Title` is the derived one.
	title := u.Title
	if title == "" {
		// A heading that is only an id has no title to give, and its own id
		// is the honest stand-in - the same id the record is keyed on, which
		// reads as "the document wrote no title here".
		title = u.ID
	}
	status := record.StatusActive
	if u.Struck {
		status = record.StatusClosed
	}
	return intent{
		id:    u.ID,
		kind:  record.KindWorkItem,
		grain: grainHeading,
		title: title,
		body:  title,
		fields: map[string]string{
			"title":             title,
			"description_short": title,
			fieldStatus:         status,
			"source":            o.backlog,
			"tags":              tagHeadingBorne,
		},
		partOf: parent,
	}
}

// headingParent is the part-of edge a heading-borne id owes, and the bool says
// whether the document's two derivations agreed.
//
// ⛔ HEADING ENCLOSURE ALONE IS FALSIFIED, AND THE MEASUREMENT IS IN THE
// PARSER'S OWN NOTES: taking `Under` as part-of produced twenty-four edges the
// document never states, because the table under `## B46` became the general
// open-items table. `Under` is a fact about the FILE; part-of is a claim about
// the WORK. So the edge is emitted only where the ID agrees - `B46d` begins
// with `B46` - which is the sub-letter derivation the parser settled on,
// computed here by a prefix test rather than by re-reading the document.
func headingParent(u record.Unimported) (parent string, agreed bool) {
	switch {
	case u.Under == "" || u.ID == "":
		// Nothing was claimed, so there is nothing to disagree with.
		return "", true
	case u.ID == u.Under:
		return "", true
	case !strings.HasPrefix(u.ID, u.Under):
		return "", false
	default:
		return u.Under, true
	}
}

// rankedID is the record id one ranked row gets, and the bool is false when
// the document does not give this seeder enough to key it safely.
//
// ⛔ THE ID NAMES ITS TABLE AND NOT JUST ITS RANK, AND THAT IS THE WHOLE
// REASON `Unimported.Section` EXISTS. The labels are `0`, `6a`, `9` - unique
// in this document today, and unique only by luck. A second unnumbered table
// anywhere in BACKLOG.md would land eleven rows in the same bare `rank`
// namespace, overwrite the first eleven records and report nothing, because
// both halves of the instrument would agree about a set of ids that was wrong.
//
// ⛔ AND A ROW WITH NO SECTION IS REFUSED RATHER THAN KEYED ON THE RANK ALONE.
// The parser's own note says Section is empty on three of the seven kinds and
// calls that a gap; a seeder that fell back to the bare rank would turn that
// gap into exactly the silent collision the field was added to prevent. It is
// reported instead, which is the status quo for that row and not a regression.
//
// The shape is `<section>/<rank>`, which is the same `parent/child` key the
// decisions document already writes - ONE id convention in this store rather
// than two, because a reader who has to learn which document an id came from
// before they can read it has been given two conventions and no rule.
func rankedID(u record.Unimported) (string, bool) {
	if u.Section == "" || u.Label == "" {
		return "", false
	}
	return u.Section + "/" + u.Label, true
}

// rankedIntent is the record one ranked row becomes.
//
// ⛔ NO `status`, AND IT IS THE DOCUMENT BEING UNREACHABLE RATHER THAN THE
// DOCUMENT BEING SILENT. These rows have a State cell and `Unimported` does
// not carry it. `active` would be the store contradicting its own document on
// every row the State cell calls built - which is the B19 inversion, and the
// reason this seeder stopped writing `active` unconditionally in the first
// place. So the field is absent, the absence is tagged, and the report names
// all eleven with the consequence: the brief's open list selects
// `status == "active"` and cannot show them.
//
// ⛔ AND NO `part-of` EITHER. `Under` is empty on these rows - the enclosing
// heading carries no id - so there is nothing to point at, and heading
// enclosure alone is the derivation the parser's own notes record as
// falsified.
func rankedIntent(o options, u record.Unimported, id string) intent {
	// The rank is the stand-in until `Unimported` carries the work cell. When
	// it does, this picks the real title up with no other change, and the run
	// in between reports `stale(title)` - which is the instrument working.
	title := u.Title
	if title == "" {
		title = u.Label
	}
	return intent{
		id:    id,
		kind:  record.KindWorkItem,
		grain: grainRanked,
		title: title,
		body:  title,
		fields: map[string]string{
			"title":      title,
			"source":     o.backlog,
			fieldRank:    u.Label,
			fieldSection: u.Section,
			fieldDocLine: strconv.Itoa(u.Line),
			"tags":       tagRankOnly,
		},
	}
}

// addRankedRow plans one row of an ordered table, or reports it.
//
// ⛔ THE RANK IS A FIELD AND IS NOT MAPPED ONTO `blocks` OR `priority`. RULED
// 2026-09-17 by the team-lead and it is not this seat's to revisit: `blocks`
// asserts a dependency nobody stated and `priority` has three buckets for
// eleven ranks. A total order has no faithful home in section 39's model, so
// the gap is REPORTED in a field a reader can see rather than papered over
// with a mapping that would read as the document's own claim.
func (p *plan) addRankedRow(o options, u record.Unimported, stated map[string]bool) {
	id, ok := rankedID(u)
	if !ok {
		p.unimported = append(p.unimported, docUnimported{doc: o.backlog, Unimported: u})
		return
	}
	if stated[id] {
		p.collided = append(p.collided, id)
		return
	}
	stated[id] = true
	p.want = append(p.want, rankedIntent(o, u, id))
}

// report names everything the document addresses that this seeder did not
// write. It is printed after a SEEDING run for the same reason --check prints
// it: a row nobody can see is a row nobody fixes.
func (p plan) report(w io.Writer) {
	namedSet(w, "ID STATED TWICE, WRITTEN ONCE", p.collided,
		"two places state the same id - a heading and a row, or the backlog and\n"+
			"      the decisions document. The first read won and the second was not\n"+
			"      written over it, because two puts against one id in one run\n"+
			"      supersede each other and document order would decide the content.")
	namedSet(w, "HEADING NESTING THE ID CONTRADICTS, NO EDGE WRITTEN", p.unnested,
		"the enclosing heading is not a prefix of this id, so the two\n"+
			"      derivations disagree and no part-of was invented.")
	namedSet(w, "PART-OF TOWARDS AN ID THE DOCUMENT NEVER DEFINES, NOT ASSERTED", p.orphaned,
		"the parent is in no row and no heading, so the edge was not attempted:\n"+
			"      the store refuses a link with a missing end.")
	namedSet(w, "DELIBERATELY NOT ASSERTED - ruled, and not a divergence", p.detached,
		detachedWhy)
	reportUnimported(w, p.unimported)
}

// detachedWhy is the one statement of why a stated edge is not asserted,
// written once because the seeding run and `--check` both print it and two
// wordings of one ruling is how a ruling starts drifting.
const detachedWhy = "the parent is a NOTE. RULED by the team-lead, 2026-09-17: a note is\n" +
	"      part-of the PROJECT - that is the edge section 3 of the brief joins\n" +
	"      on - and a ruling is not part-of a note. The store's own six edges\n" +
	"      of this shape are taken back by a seeding run rather than left\n" +
	"      beside the new ones.\n" +
	"      ⛔ IT COSTS THESE RULINGS THEIR DOCUMENT-STATED PARENT, which is why\n" +
	"      every one is named here on every run."

// docUnimported is one Unimported with the document it was read from.
//
// ⛔ THE DOCUMENT IS CARRIED RATHER THAN PASSED IN ONCE, BECAUSE THERE ARE TWO
// OF THEM NOW. A report that printed one file name over a list drawn from two
// would resolve every line to the wrong place in the wrong document - a
// file:line that is precise and false, which is worse than none at all.
type docUnimported struct {
	doc string
	record.Unimported
}

// reportUnimported prints the things the documents address that are not even
// candidates, ONE PER LINE WITH ITS FILE AND LINE NUMBER.
//
// ⛔ A COUNT IS NOT AN ANSWER HERE AND NEVER WAS. A census whose total did not
// move while two rows did is exactly what a count cannot see, and an
// arithmetically impossible one reached Boris once already.
func reportUnimported(w io.Writer, us []docUnimported) {
	var open, ruled []docUnimported
	for _, u := range us {
		if deliberate(u.Unimported) {
			ruled = append(ruled, u)
			continue
		}
		open = append(open, u)
	}
	unimportedSet(w, "UNIMPORTED - the document addresses it and it is not even a candidate",
		open, unimportedWhy(open))
	unimportedSet(w, "DELIBERATELY NOT IMPORTED - ruled, and not a divergence", ruled,
		"an id in the first cell of a table that is NOT a work-item table: a\n"+
			"      cross-reference to work that is tracked elsewhere, not a work item\n"+
			"      of its own. RULED by the team-lead, 2026-09-17 - these stay\n"+
			"      unimported and --check does not fail on them.")
}

// unimportedWhy is one line per KIND present, so a set that is deliberate but
// still open does not read as a defect nobody understands.
//
// ⛔ A REASON PER KIND RATHER THAN ONE SENTENCE OVER THE BLOCK, because the
// block holds several kinds at once and a single caption would be true of one
// of them and false of the rest. The thirteen `#####` headings are the case
// this exists for: they are OUTSIDE the grain Boris ruled, so `--check` must
// keep exiting 2 over them, and a reader has to be able to tell "he has not
// ruled on this yet" from "something is broken".
func unimportedWhy(us []docUnimported) string {
	seen := map[record.UnimportedKind]bool{}
	var kinds []record.UnimportedKind
	for _, u := range us {
		if !seen[u.Kind] {
			seen[u.Kind] = true
			kinds = append(kinds, u.Kind)
		}
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	var lines []string
	for _, k := range kinds {
		lines = append(lines, string(k)+": "+unimportedReason(k))
	}
	return strings.Join(lines, "\n      ")
}

// unimportedReason is what one kind MEANS, in the one place it is written.
func unimportedReason(k record.UnimportedKind) string {
	switch k {
	case record.UnimportedHeadingTooDeep:
		return "a `#####` heading, which is BELOW the grain Boris ruled on\n" +
			"          2026-09-16 - one record per ##, ### and ####. Thirteen of these\n" +
			"          are in plan/ and THREE ARE HIS OWN RULINGS. They are reported\n" +
			"          rather than imported, and --check exits 2 over them until\n" +
			"          somebody rules. Widening the grain is one character and the\n" +
			"          ruling is his, not a seat's."
	case record.UnimportedHeadingNoTitle:
		return "nothing is left once the decoration is stripped, so there is no\n" +
			"          title to key a record on."
	case record.UnimportedDuplicateKey:
		return "two headings key the same way. The first read was written and the\n" +
			"          second was not written over it, because two puts against one id\n" +
			"          supersede each other."
	case record.UnimportedIrregularID:
		return "a first cell naming something id-SHAPED that is not a legal id, never\n" +
			"          read as the prefix it starts with."
	case record.UnimportedRowWithoutID:
		return "a row inside a work-item table whose id cell names no id."
	default:
		return "this seeder has no statement about this kind, which means it was added\n" +
			"          to internal/record and nothing here was taught what it means."
	}
}

// deliberate says whether one unimported thing is a RULED exclusion rather than
// an open failure.
//
// ⛔ IT IS A KIND TEST AND NOT A LIST OF THE FIVE IDS. B6 B10 B18 B20 B21 are
// what the document holds today; an allow-list would go stale the moment it
// gains a sixth cross-reference, and the sixth would then read as an open
// failure while the other five read as ruled - two answers to one question,
// decided by when somebody last edited this file. The parser's own definition
// of the kind IS the ruling: a first cell in a table that is not a work-item
// table.
func deliberate(u record.Unimported) bool {
	return u.Kind == record.UnimportedOtherTable
}

// openUnimported is the half that is still a divergence.
func openUnimported(us []docUnimported) []docUnimported {
	var out []docUnimported
	for _, u := range us {
		if !deliberate(u.Unimported) {
			out = append(out, u)
		}
	}
	return out
}

// unimportedSet prints one labelled block, ONE PER LINE WITH ITS FILE AND LINE
// NUMBER, and prints an EMPTY one as {}.
func unimportedSet(w io.Writer, label string, us []docUnimported, why string) {
	if len(us) == 0 {
		fmt.Fprintf(w, "\n  %s\n      {}\n", label)
		return
	}
	fmt.Fprintf(w, "\n  %s\n", label)
	for _, u := range us {
		// file:line, so a report resolves to a PLACE in the document rather than
		// to the document. A reader who has to go and find the row is a reader
		// who does not.
		fmt.Fprintf(w, "      %-14s %-34s %s:%d\n", u.Kind, unimportedName(u.Unimported), u.doc, u.Line)
	}
	if why != "" {
		fmt.Fprintf(w, "      %s\n", why)
	}
}

// unimportedName is the id where the document states one and the label where it
// does not.
//
// ⛔ ON THE ELEVEN ORDERED ROWS THE LABEL IS THE RANK, and the parser's own note
// says that rank is the only place the document's declared order survives. A
// report that printed the kind alone would throw it away.
func unimportedName(u record.Unimported) string {
	switch {
	case u.ID != "":
		return u.ID
	case u.Label != "":
		return "label " + strconv.Quote(u.Label)
	default:
		return "(nothing where the id goes)"
	}
}

// namedSet prints one labelled set, and it prints an EMPTY one as {}.
//
// ⛔ A SET THAT DISAPPEARS WHEN IT IS EMPTY CANNOT BE TOLD FROM A SET THE CHECK
// NEVER COMPUTED. That is this repository's pass-versus-no-run class in its
// cheapest form, and it has been recorded ten times.
func namedSet(w io.Writer, label string, items []string, why string) {
	fmt.Fprintf(w, "\n  %s\n", label)
	if len(items) == 0 {
		fmt.Fprintf(w, "      {}\n")
		return
	}
	fmt.Fprintf(w, "      { %s }\n      %s\n", strings.Join(items, " "), why)
}

// linkPartOf asserts one part-of edge through the CLI, like every other write
// here. record.Link is idempotent by contract, so a re-run asserts the same
// fact and the store changes nothing.
func (o options) linkPartOf(src, dst string, r *result) error {
	if err := o.write([]string{"record", "link", src, record.LinkPartOf, dst}); err != nil {
		return err
	}
	r.linked = append(r.linked, edgeName(src, dst))
	return nil
}

// edgeName writes an edge whole - src, type, dst - because a reader scanning a
// column of ids has no way to know which side of the arrow they are on.
func edgeName(src, dst string) string {
	return src + " -" + record.LinkPartOf + "-> " + dst
}

// edge is one part-of edge with its two ends still apart.
//
// ⛔ THE DIFF CARRIES THE PAIR AND RENDERS IT LATE, RATHER THAN CARRYING THE
// RENDERED STRING. The extra edges are now acted on - a seeding run unlinks the
// ones it wrote - and a caller that had to split `a -part-of-> b` back into two
// ids would be parsing this program's own report format. A report format that
// something re-reads is a format that cannot be improved.
type edge struct{ src, dst string }

func (e edge) String() string { return edgeName(e.src, e.dst) }

// edgeNames renders a set of edges for a report, in the order it was given.
func edgeNames(es []edge) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.String())
	}
	return out
}

// retractEdges removes the part-of edges the store holds and neither document
// states.
//
// ⛔ B77 IS WHY THIS EXISTS AND WHY IT IS ONLY ABOUT EDGES. rig can unlink an
// edge and cannot retract a record, so a record the documents stop stating
// survives as EXTRA for ever, while an edge they stop stating can be taken
// back. Six `decision -part-of-> note` edges written by an earlier generation
// are exactly that case: the note now points at the project instead, and
// leaving the old six beside the new nine would be two derivations of one
// relationship sitting in one table.
//
// ⛔ AND THE ANSWER TO B77 IS NEVER A DELETE. Retraction is a separate
// capability that rig has not got; nothing here removes a record.
func (o options) retractEdges(p plan, r *result) error {
	var d divergence
	if err := d.compareEdges(p, func(parent string) (map[string]bool, error) {
		return storePartOfChildren(o, parent)
	}); err != nil {
		return err
	}
	for _, e := range p.retractable(o, d.extraEdges) {
		if err := o.write([]string{"record", "unlink", e.src, record.LinkPartOf, e.dst}); err != nil {
			return fmt.Errorf("taking back %s: %w", e, err)
		}
		r.unlinked = append(r.unlinked, e.String())
	}
	return nil
}

// retractable narrows the extra edges to the ones this seeder is entitled to
// remove: both ends are records it writes.
//
// ⛔ THE NARROWING IS THE WHOLE SAFETY ARGUMENT AND NOT AN OPTIMISATION. A note
// somebody attached to a work item by hand has a SOURCE no document states, and
// a seeder that unlinked every edge it did not recognise would silently undo a
// person's work on every run - once a day, against the store whose whole
// argument is that things stop disappearing. An edge with one end outside this
// plan is somebody else's fact and is left exactly where it is.
func (p plan) retractable(o options, extra []edge) []edge {
	mine := make(map[string]bool, len(p.want)+1)
	for _, in := range p.want {
		mine[in.id] = true
	}
	// The project record is this seeder's too - `seedProject` writes it - and it
	// is the destination every note points at.
	mine[o.project] = true

	var out []edge
	for _, e := range extra {
		if mine[e.src] && mine[e.dst] {
			out = append(out, e)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// THE DIVERGENCE DETECTOR
// ---------------------------------------------------------------------------

// divergedError is what --check returns when the store and the document are not
// set-equal. It carries no numbers: the sets have already been printed.
type divergedError struct{ what string }

func (e *divergedError) Error() string { return e.what }

// divergence is every way the store and the document disagree.
//
// ⛔ EVERY FIELD IS A SET AND NOT ONE OF THEM IS A COUNT. B63 was found because
// somebody diffed the id sets by hand; it was then MISREPORTED because somebody
// read a total. A count that did not move while two rows did is the thing this
// type exists to make impossible.
type divergence struct {
	// missing is in the document and in no record.
	missing []string

	// extra is a record no part of the document states.
	extra []string

	// stale is a record whose stored fields are not what the document says,
	// each named with the fields that differ.
	stale []string

	// missingEdges and extraEdges are the part-of edges, with their two ends
	// still apart so a caller can act on them.
	missingEdges []edge
	extraEdges   []edge

	unimported []docUnimported
	collided   []string
	orphaned   []string
	unnested   []string

	// detached is a part-of the documents state that this seeder deliberately
	// does not assert. Printed, and never counted as a divergence.
	detached []string
}

// runCheck is the whole of --check: read the store, diff it against the plan,
// print the sets, and refuse to call a divergence a success.
func runCheck(o options, p plan, estate string) error {
	store, err := storeRecords(o, p.kinds())
	if err != nil {
		return err
	}
	d := diff(p, store)
	// ⛔ THE STORE LOOKUP IS PASSED IN RATHER THAN REACHED FOR, AND THAT IS WHAT
	// MAKES THE SET ARITHMETIC TESTABLE WITHOUT A DAEMON. An edge diff that can
	// only be exercised against a live store is an edge diff nobody exercises.
	if err := d.compareEdges(p, func(parent string) (map[string]bool, error) {
		return storePartOfChildren(o, parent)
	}); err != nil {
		return err
	}
	d.report(os.Stdout, o, p, estate)

	if names := d.nonEmpty(); len(names) > 0 {
		return &divergedError{what: fmt.Sprintf(
			"%s and the %s store are not set-equal: %s. The SETS are printed above; "+
				"this line is not a summary of them",
			o.sources(), estate, strings.Join(names, ", "))}
	}
	return nil
}

// held is one record as the store holds it: everything a reader can observe
// that a document also states.
//
// ⛔ IT IS NOT JUST THE FIELD MAP ANY MORE, AND THE TWO ADDITIONS ARE BOTH
// HOLES THE DETECTOR HAD. `kind` is not compared by a membership test - an id
// held at the wrong kind is present, so `missing` and `extra` are both empty
// and the fields can agree perfectly. `body` was invisible for the same
// reason, and on a work item that cost nothing because the body IS the title;
// on a decision the body is the ruling, up to three and a half thousand bytes
// of it, so an edited ruling would have read as clean for ever.
type held struct {
	kind   string
	body   string
	fields map[string]string
}

// storeRecords is every record this project already holds at the kinds the
// plan writes, by id.
//
// ⛔ ONE QUERY PER KIND RATHER THAN ONE UNFILTERED QUERY, AND THE KINDS COME
// FROM THE PLAN. An unfiltered read would return the project record and every
// progress step, and each of them would land in `extra` - a detector that
// reports the store's own bookkeeping as a divergence is one nobody reads.
// Taking the kinds from the plan is also what stops a kind added to the seeder
// and forgotten here: it would be written and then reported missing on the very
// next check, loudly, instead of never being compared at all.
func storeRecords(o options, kinds []string) (map[string]held, error) {
	byID := map[string]held{}
	for _, kind := range kinds {
		out, err := capture(o, "record", "query", o.project, kind, "--json")
		if err != nil {
			return nil, fmt.Errorf("listing the %s records already in the store failed: %w\n%s", kind, err, out)
		}
		var rs []struct {
			ID     string            `json:"id"`
			Kind   string            `json:"kind"`
			Body   string            `json:"body"`
			Fields map[string]string `json:"fields"`
		}
		if err := json.Unmarshal(out, &rs); err != nil {
			return nil, fmt.Errorf("rig record query %s --json did not parse: %w", kind, err)
		}
		for _, r := range rs {
			byID[r.ID] = held{kind: r.Kind, body: r.Body, fields: r.Fields}
		}
	}
	return byID, nil
}

// kinds is every record kind this plan writes, sorted, with no duplicates.
func (p plan) kinds() []string {
	seen := map[string]bool{}
	var out []string
	for _, in := range p.want {
		if seen[in.kind] {
			continue
		}
		seen[in.kind] = true
		out = append(out, in.kind)
	}
	sort.Strings(out)
	return out
}

// diff is the set arithmetic, and it is the whole answer for the record grain.
func diff(p plan, store map[string]held) divergence {
	d := divergence{
		unimported: p.unimported,
		collided:   p.collided,
		unnested:   p.unnested,
		orphaned:   p.orphaned,
		detached:   p.detached,
	}

	want := make(map[string]bool, len(p.want))
	for _, in := range p.want {
		want[in.id] = true
		got, held := store[in.id]
		if !held {
			d.missing = append(d.missing, in.id)
			continue
		}
		if differing := staleFields(in, got); len(differing) > 0 {
			d.stale = append(d.stale, in.id+"("+strings.Join(differing, ",")+")")
		}
	}
	for id := range store {
		if !want[id] {
			d.extra = append(d.extra, id)
		}
	}

	sort.Strings(d.missing)
	sort.Strings(d.extra)
	sort.Strings(d.stale)
	return d
}

// staleFields names the fields whose stored value is not what the document says.
//
// ⛔ IT LOOKS ONLY AT THE FIELDS THIS SEEDER WRITES, AND THE ASYMMETRY IS
// DELIBERATE RATHER THAN AN OVERSIGHT. A field somebody added by hand is not
// the document disagreeing with the store, and reporting it would make every
// hand-annotated record read as drift. The cost is named here so nobody reads a
// clean `stale` as "the store matches the document in every respect": a field
// the store carries and the document never mentions is invisible to this check.
func staleFields(in intent, got held) []string {
	var out []string
	// ⛔ kind AND body ARE COMPARED UNDER THEIR OWN NAMES AND NOT FOLDED INTO
	// A BARE "different". The whole contract of `stale` is that the brackets
	// name WHICH field moved, so a reader knows whether a title was retyped or
	// a whole ruling rewritten. Two of the three are not typed fields at all,
	// which is exactly why they were being skipped.
	if got.kind != in.kind {
		out = append(out, "kind")
	}
	if got.body != in.body {
		out = append(out, "body")
	}
	for name, want := range in.fields {
		if got.fields[name] != want {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// compareEdges diffs the part-of edges the document states against the store's.
//
// ⛔ `rig record refs` ANSWERS WHAT POINTS AT A RECORD, NOT WHAT A RECORD POINTS
// AT, so the question is asked once per PARENT rather than once per child -
// seven children become one call today.
//
// ⛔ AND EVERY NOTE IS ASKED ABOUT TOO, EVEN WHERE NOTHING IS PART-OF IT. The
// aperture used to be "the parents the documents state TODAY", and that is
// precisely blind where this seeder's own wrong edges landed: it wrote six
// `decision -part-of-> note` edges, then stopped stating them, and a detector
// built from today's parents would never look at a note again. A check that
// stops covering the thing that was just fixed is the pass-versus-no-run class
// in its most expensive form. Nine extra calls at about 3 ms each, measured.
//
// ⛔ WHAT IS STILL OUTSIDE IT, SAID OUT LOUD: an edge towards a record that is
// neither a stated parent nor a note is invisible here. Nothing writes one
// today - every edge this seeder asserts ends at a parent or at the project -
// but a clean `extraEdges` is a statement about parents and notes, not about
// every edge in the store.
func (d *divergence) compareEdges(p plan, childrenOf func(string) (map[string]bool, error)) error {
	children := map[string][]string{}
	asked := map[string]bool{}
	for _, in := range p.want {
		if in.partOf != "" {
			children[in.partOf] = append(children[in.partOf], in.id)
			asked[in.partOf] = true
		}
		if in.kind == record.KindNote {
			asked[in.id] = true
		}
	}

	parents := make([]string, 0, len(asked))
	for parent := range asked {
		parents = append(parents, parent)
	}
	sort.Strings(parents)

	for _, parent := range parents {
		have, err := childrenOf(parent)
		if err != nil {
			return err
		}
		want := map[string]bool{}
		for _, child := range children[parent] {
			want[child] = true
			if !have[child] {
				d.missingEdges = append(d.missingEdges, edge{src: child, dst: parent})
			}
		}
		for child := range have {
			if !want[child] {
				d.extraEdges = append(d.extraEdges, edge{src: child, dst: parent})
			}
		}
	}
	sortEdges(d.missingEdges)
	sortEdges(d.extraEdges)
	return nil
}

// sortEdges orders a set of edges by how they are printed, so the report reads
// the same way twice.
func sortEdges(es []edge) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].src != es[j].src {
			return es[i].src < es[j].src
		}
		return es[i].dst < es[j].dst
	})
}

// storePartOfChildren is every record the store says is part-of one parent.
func storePartOfChildren(o options, parent string) (map[string]bool, error) {
	out, err := capture(o, "record", "refs", parent, "--json")
	if err != nil {
		var refusal struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(out, &refusal) == nil && refusal.Code == "CODE_NOT_FOUND" {
			// A parent that is not a record at all has no edges, and `missing`
			// already says it is absent. That is an answer, not a failure.
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("asking what points at %s failed: %w\n%s", parent, err, out)
	}

	return parsePartOfChildren(out, parent)
}

// parsePartOfChildren reads one `rig record refs --json` reply. It is its own
// function so the distance rule and the truncation refusal can be exercised
// without a daemon.
func parsePartOfChildren(out []byte, parent string) (map[string]bool, error) {
	var r struct {
		In []struct {
			Src      string `json:"src"`
			Type     string `json:"type"`
			Distance int    `json:"distance"`
		} `json:"in"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("rig record refs --json did not parse: %w", err)
	}

	// ⛔ A TRUNCATED WALK IS NOT AN ANSWER AND MUST NOT BECOME ONE. rig prints
	// this key on every reply precisely so a partial answer cannot be read as a
	// complete one, and a detector that took a cut-short walk for "no such
	// edge" would report a divergence the store does not have.
	if r.Truncated {
		return nil, fmt.Errorf("rig record refs %s came back truncated, so an edge that "+
			"is absent cannot be told from one the walk did not reach; this check "+
			"declines to answer rather than guess", parent)
	}

	have := map[string]bool{}
	for _, e := range r.In {
		// Distance 1 is the edge the document states. The walk follows four
		// hops by default, and a grandchild is not a child.
		if e.Type == record.LinkPartOf && e.Distance == 1 {
			have[e.Src] = true
		}
	}
	return have, nil
}

// nonEmpty names the sets that are not empty, so the exit line can say WHICH
// rather than that something was wrong.
func (d divergence) nonEmpty() []string {
	var names []string
	for _, s := range []struct {
		name string
		n    int
	}{
		{"missing", len(d.missing)},
		{"extra", len(d.extra)},
		{"stale", len(d.stale)},
		{"missing-edges", len(d.missingEdges)},
		{"extra-edges", len(d.extraEdges)},
		// ⛔ ONLY THE OPEN HALF. A ruled exclusion counted here would keep
		// `--check` at exit 2 for ever over a question somebody has already
		// answered, and a check that can never go green is a check nobody
		// runs. `d.detached` is absent for the same reason.
		{"unimported", len(openUnimported(d.unimported))},
		{"id-stated-twice", len(d.collided)},
		{"heading-nesting-the-id-contradicts", len(d.unnested)},
		{"part-of-towards-an-undefined-id", len(d.orphaned)},
	} {
		if s.n > 0 {
			names = append(names, s.name)
		}
	}
	return names
}

// report prints every set, empty ones included.
func (d divergence) report(w io.Writer, o options, p plan, estate string) {
	fmt.Fprintf(w, "%s against the %s estate, project %q\n", o.sources(), estate, o.project)
	fmt.Fprintf(w, "the document asks for %d records (%s)\n", len(p.want), p.grains())

	namedSet(w, "MISSING - the document states it and no record carries it", d.missing,
		"re-run rigseed without --check to write them.")
	namedSet(w, "EXTRA - a record this document does not state", d.extra,
		"either the document dropped a row or something else wrote into this project.")
	namedSet(w, "STALE - held, but its stored fields are not what the document says", d.stale,
		"the differing field names are in the brackets. Re-running rigseed supersedes them.")
	namedSet(w, "MISSING EDGES - the document states this part-of and the store has not got it", edgeNames(d.missingEdges),
		"re-run rigseed without --check to assert them.")
	namedSet(w, "EXTRA EDGES - the store holds a part-of the document does not state", edgeNames(d.extraEdges),
		"a seeding run TAKES BACK the ones whose two ends are both records it\n"+
			"      writes. Anything still listed after one has an end this seeder does\n"+
			"      not write, so it is somebody else's edge and is left alone.")
	namedSet(w, "ID STATED TWICE", d.collided,
		"two places state the same id - a heading and a row, or the backlog and\n"+
			"      the decisions document. The first read is what was written.")
	namedSet(w, "HEADING NESTING THE ID CONTRADICTS", d.unnested,
		"the enclosing heading is not a prefix of this id, so no part-of was invented.")
	namedSet(w, "PART-OF TOWARDS AN ID THE DOCUMENT NEVER DEFINES, NOT ASSERTED", d.orphaned,
		"the parent is in no row and no heading, so the edge was not attempted:\n"+
			"      the store refuses a link with a missing end.")
	namedSet(w, "DELIBERATELY NOT ASSERTED - ruled, and not a divergence", d.detached,
		detachedWhy)
	reportUnimported(w, d.unimported)

	// ⛔ WHAT THIS CHECK DOES NOT COMPARE, PRINTED ON EVERY RUN INCLUDING A
	// CLEAN ONE. A green here means the ID SETS agree and the fields this
	// seeder writes agree. It does NOT mean rig and the document say the same
	// thing: a field the store carries that the document never mentions, the
	// document's declared ORDER, and everything in BACKLOG.md that is prose
	// are all outside it. A reader who takes a clean run for whole-document
	// agreement has been misled by an instrument, which is how a store gets
	// trusted for something it never checked.
	fmt.Fprintf(w, "\n  NOT COMPARED: fields the store carries that the document never "+
		"states,\n      the document's declared order, and every word of prose. A clean run "+
		"means\n      the SETS agree, never that rig and the document say the same thing.\n")
}
