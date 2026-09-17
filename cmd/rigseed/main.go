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
)

// tagHeadingBorne marks a record whose id the document states in a HEADING.
//
// Section 39's migration ruling is import everything and flag what is
// irregular, and `tags` is the field it names for the job. A heading is not
// irregular markup, but it IS a different grain with less behind it, and a
// reader who cannot tell the two apart cannot tell a status the document never
// gave from one something lost.
const tagHeadingBorne = "heading-borne"

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
	p := planFor(o, doc)

	// ⛔ THE CHECK READS THE SAME PLAN THE SEEDER WOULD WRITE, AND THAT IS WHAT
	// MAKES IT AN ANSWER RATHER THAN A SECOND OPINION. A detector with its own
	// reading of BACKLOG.md would be the second parser this file's header
	// forbids, and the two would drift exactly where it matters.
	if o.check {
		return runCheck(o, p, name)
	}

	fmt.Printf("%s -> %d records (%s), into the %s estate as project %q\n\n",
		o.backlog, len(p.want), p.grains(), name, o.project)

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

	r.report(os.Stdout, o)
	p.report(os.Stdout, o.backlog)
	return nil
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
		"--kind", "work-item",
		"--project", o.project,
		"--id", in.id,
		"--body", in.title,
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
		return 0, fmt.Errorf("%s %s\n%s", o.rigBin, strings.Join(withJSON, " "), out)
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
		return fmt.Errorf("%s %s\n%s", o.rigBin, strings.Join(args, " "), out)
	}
	return nil
}

func capture(o options, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return exec.CommandContext(ctx, o.rigBin, args...).CombinedOutput()
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

	// linked is every part-of edge asserted, written whole - src, type, dst -
	// because a column of ids gives a reader no way to know which side of the
	// arrow they are on.
	linked []string
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
	name("PART-OF EDGES ASSERTED", r.linked,
		"the sub-letter in the id is the document stating a parent. The store's\n"+
			"      Link is idempotent by contract, so a re-run asserts the same fact.")
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
	id    string
	grain string
	title string

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

	// unimported is everything the document addresses that this seeder does
	// NOT write.
	unimported []record.Unimported

	// collided is an id the document states at BOTH grains. Reported, never
	// written twice.
	collided []string

	// unnested is a heading sitting under an id-bearing heading whose own id
	// does not agree with it. Reported, and no edge is invented.
	unnested []string

	// orphaned is a part-of the document states towards an id the document
	// never defines. Reported, and the edge is NOT attempted.
	orphaned []string
}

// planFor splits everything the document addresses into what this seeder writes
// and what it leaves alone.
//
// ⛔ THE SPLIT IS "A HEADING, ELSE REPORT IT" AND NOT A LIST OF KINDS TO SKIP.
// A new UnimportedKind added in internal/record must land somewhere LOUD. B66
// is precisely a class being invisible by construction, and a switch that
// silently accepted a new member would be that same defect one layer up.
func planFor(o options, doc record.BacklogParse) plan {
	var p plan
	stated := make(map[string]bool, len(doc.Items))

	for _, it := range doc.Items {
		stated[it.ID] = true
		p.want = append(p.want, rowIntent(o, it))
	}

	for _, u := range doc.Unimported {
		if u.Kind != record.UnimportedHeading {
			p.unimported = append(p.unimported, u)
			continue
		}
		// ⛔ AN ID STATED AT BOTH GRAINS IS REPORTED, NEVER WRITTEN TWICE. Two
		// puts against one id in one run supersede each other and the store
		// keeps whichever went last, so which content wins would be decided by
		// document order - a silent wrong answer. The parser's own reconcile
		// already drops this case; this is the belt that says so out loud the
		// day it stops.
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
	}

	// ⛔ AN EDGE TOWARDS AN ID THE DOCUMENT NEVER DEFINES IS REPORTED, NEVER
	// ATTEMPTED. internal/record's checkEdge refuses a link with a missing end,
	// so asserting one would abort a seeding run half-written - and it is a
	// defect in the DOCUMENT, not a failure of this program. Before B46 became
	// a record its six children named exactly such a parent, so this is the
	// shape the run is walking out of rather than a hypothetical.
	for i := range p.want {
		parent := p.want[i].partOf
		if parent == "" || stated[parent] {
			continue
		}
		p.orphaned = append(p.orphaned, edgeName(p.want[i].id, parent))
		p.want[i].partOf = ""
	}
	sort.Strings(p.orphaned)
	return p
}

// grains says how many records came from each grain, for the one header line.
func (p plan) grains() string {
	rows, heads := 0, 0
	for _, in := range p.want {
		if in.grain == grainHeading {
			heads++
			continue
		}
		rows++
	}
	return fmt.Sprintf("%d %s, %d %s", rows, grainRow, heads, grainHeading)
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
	return intent{id: it.ID, grain: grainRow, title: it.Title, fields: f, partOf: it.PartOf, row: it}
}

// headingIntent is the record an id stated in a HEADING becomes.
//
// ⛔ IT WRITES NO `status`, AND THAT IS THE ANSWER RATHER THAN AN OMISSION. A
// heading has no state cell. The document says nothing about whether B46 is
// open, and `active` would be this seeder inventing the one fact the row grain
// gets from the document rather than from its reader. `tags=heading-borne` is
// what makes the absence readable: a field that is not there cannot otherwise
// be told from a field something lost.
//
// ⛔ AND IT IS A DECISION SOMEBODY MAY OVERTURN, SO IT SAYS SO. It keeps B46
// out of the brief's open list, which selects on `status == "active"`
// (internal/record/brief.go). The alternative is to infer a state from the row
// grain's children, and reading a parent's state out of its children is a human
// judging - the one thing the projection criterion forbids.
func headingIntent(o options, u record.Unimported, parent string) intent {
	return intent{
		id:    u.ID,
		grain: grainHeading,
		title: u.Label,
		fields: map[string]string{
			"title":             u.Label,
			"description_short": u.Label,
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

// report names everything the document addresses that this seeder did not
// write. It is printed after a SEEDING run for the same reason --check prints
// it: a row nobody can see is a row nobody fixes.
func (p plan) report(w io.Writer, backlog string) {
	namedSet(w, "ID STATED AT BOTH GRAINS, WRITTEN ONCE", p.collided,
		"a heading and a table row give the same id. The row won; the heading\n"+
			"      was not written over it.")
	namedSet(w, "HEADING NESTING THE ID CONTRADICTS, NO EDGE WRITTEN", p.unnested,
		"the enclosing heading is not a prefix of this id, so the two\n"+
			"      derivations disagree and no part-of was invented.")
	namedSet(w, "PART-OF TOWARDS AN ID THE DOCUMENT NEVER DEFINES, NOT ASSERTED", p.orphaned,
		"the parent is in no row and no heading, so the edge was not attempted:\n"+
			"      the store refuses a link with a missing end.")
	reportUnimported(w, backlog, p.unimported)
}

// reportUnimported prints the things in the document that are not even
// candidates, ONE PER LINE WITH ITS LINE NUMBER.
//
// ⛔ A COUNT IS NOT AN ANSWER HERE AND NEVER WAS. A census whose total did not
// move while two rows did is exactly what a count cannot see, and an
// arithmetically impossible one reached Boris once already.
func reportUnimported(w io.Writer, backlog string, us []record.Unimported) {
	const label = "UNIMPORTED - the document addresses it and it is not even a candidate"
	if len(us) == 0 {
		fmt.Fprintf(w, "\n  %s\n      {}\n", label)
		return
	}
	fmt.Fprintf(w, "\n  %s\n", label)
	for _, u := range us {
		// file:line, so a report resolves to a PLACE in the document rather than
		// to the document. A reader who has to go and find the row is a reader
		// who does not.
		fmt.Fprintf(w, "      %-14s %-34s %s:%d\n", u.Kind, unimportedName(u), backlog, u.Line)
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

	// missingEdges and extraEdges are the part-of edges, whole.
	missingEdges []string
	extraEdges   []string

	unimported []record.Unimported
	collided   []string
	orphaned   []string
	unnested   []string
}

// runCheck is the whole of --check: read the store, diff it against the plan,
// print the sets, and refuse to call a divergence a success.
func runCheck(o options, p plan, estate string) error {
	store, err := storeRecords(o)
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
			o.backlog, estate, strings.Join(names, ", "))}
	}
	return nil
}

// storeRecords is every work item this project already holds, by id.
func storeRecords(o options) (map[string]map[string]string, error) {
	out, err := capture(o, "record", "query", o.project, "work-item", "--json")
	if err != nil {
		return nil, fmt.Errorf("listing the work items already in the store failed: %w\n%s", err, out)
	}
	var rs []struct {
		ID     string            `json:"id"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(out, &rs); err != nil {
		return nil, fmt.Errorf("rig record query --json did not parse: %w", err)
	}
	byID := make(map[string]map[string]string, len(rs))
	for _, r := range rs {
		byID[r.ID] = r.Fields
	}
	return byID, nil
}

// diff is the set arithmetic, and it is the whole answer for the record grain.
func diff(p plan, store map[string]map[string]string) divergence {
	d := divergence{
		unimported: p.unimported,
		collided:   p.collided,
		unnested:   p.unnested,
		orphaned:   p.orphaned,
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
func staleFields(in intent, got map[string]string) []string {
	var out []string
	for name, want := range in.fields {
		if got[name] != want {
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
func (d *divergence) compareEdges(p plan, childrenOf func(string) (map[string]bool, error)) error {
	children := map[string][]string{}
	for _, in := range p.want {
		if in.partOf != "" {
			children[in.partOf] = append(children[in.partOf], in.id)
		}
	}

	parents := make([]string, 0, len(children))
	for parent := range children {
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
				d.missingEdges = append(d.missingEdges, edgeName(child, parent))
			}
		}
		for child := range have {
			if !want[child] {
				d.extraEdges = append(d.extraEdges, edgeName(child, parent))
			}
		}
	}
	sort.Strings(d.missingEdges)
	sort.Strings(d.extraEdges)
	return nil
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
		{"unimported", len(d.unimported)},
		{"id-stated-at-both-grains", len(d.collided)},
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
	fmt.Fprintf(w, "%s against the %s estate, project %q\n", o.backlog, estate, o.project)
	fmt.Fprintf(w, "the document asks for %d records (%s)\n", len(p.want), p.grains())

	namedSet(w, "MISSING - the document states it and no record carries it", d.missing,
		"re-run rigseed without --check to write them.")
	namedSet(w, "EXTRA - a record this document does not state", d.extra,
		"either the document dropped a row or something else wrote into this project.")
	namedSet(w, "STALE - held, but its stored fields are not what the document says", d.stale,
		"the differing field names are in the brackets. Re-running rigseed supersedes them.")
	namedSet(w, "MISSING EDGES - the document states this part-of and the store has not got it", d.missingEdges,
		"re-run rigseed without --check to assert them.")
	namedSet(w, "EXTRA EDGES - the store holds a part-of the document does not state", d.extraEdges,
		"rigseed never removes an edge, so these were asserted by something else.")
	namedSet(w, "ID STATED AT BOTH GRAINS", d.collided,
		"a heading and a table row give the same id; the row is what was written.")
	namedSet(w, "HEADING NESTING THE ID CONTRADICTS", d.unnested,
		"the enclosing heading is not a prefix of this id, so no part-of was invented.")
	namedSet(w, "PART-OF TOWARDS AN ID THE DOCUMENT NEVER DEFINES, NOT ASSERTED", d.orphaned,
		"the parent is in no row and no heading, so the edge was not attempted:\n"+
			"      the store refuses a link with a missing end.")
	reportUnimported(w, o.backlog, d.unimported)

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
