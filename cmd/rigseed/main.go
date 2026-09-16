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

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "rigseed: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	backlog string
	project string
	estate  string
	rigBin  string
	dryRun  bool
}

func run() error {
	var o options
	flag.StringVar(&o.backlog, "backlog", "BACKLOG.md", "the backlog document to read")
	flag.StringVar(&o.project, "project", "rig", "the project every record belongs to")
	flag.StringVar(&o.estate, "estate", "production", "the estate this MUST be pointed at")
	flag.StringVar(&o.rigBin, "rig", "rig", "the rig binary to drive")
	flag.BoolVar(&o.dryRun, "dry-run", false, "print what would be written and write nothing")
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

	items, err := readBacklog(o.backlog)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("%s parsed to zero work items, which is never right for this "+
			"document - the parser or the path is wrong, and writing nothing is the safe answer", o.backlog)
	}

	fmt.Printf("%s -> %d work items, into the %s estate as project %q\n\n",
		o.backlog, len(items), name, o.project)

	r := &result{}
	for _, it := range items {
		if err := seedOne(o, it, r); err != nil {
			return fmt.Errorf("%s: %w", it.ID, err)
		}
	}
	r.report(os.Stdout, o)
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

func readBacklog(path string) ([]record.BacklogItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return record.ParseBacklog(f)
}

// seedOne writes one row, and it is written to be RE-RUNNABLE.
//
// ⛔ A PUT WITH --if-version 0 MEANS CREATE, AND IT REFUSES ON AN ID THAT
// EXISTS. The cli seat measured that refusal on a live daemon: "record B1
// already exists at version 1: a put with IfVersion 0 creates, and this id is
// taken". It is correct behaviour and it makes a naive seeder single-shot - a
// partial failure would leave the operator unable to re-run without a wipe.
// So every row is PROBED first and supersedes whatever version is there.
func seedOne(o options, it record.BacklogItem, r *result) error {
	version, exists, err := currentVersion(o, it.ID)
	if err != nil {
		return err
	}

	args := []string{
		"record", "put",
		"--kind", "work-item",
		"--project", o.project,
		"--id", it.ID,
		"--body", it.Title,
		"--field", "title=" + it.Title,
		"--field", "description_short=" + it.Title,
		"--field", "status=active",
		"--field", "source=" + o.backlog,
	}
	if tags := tagsFor(it); tags != "" {
		args = append(args, "--field", "tags="+tags)
	}
	args = append(args, "--if-version", strconv.FormatUint(version, 10))

	if err := o.write(args); err != nil {
		return err
	}
	if exists {
		r.superseded++
	} else {
		r.created++
	}

	// ⛔ CLOSURE IS A progress.step, NEVER A FIELD, AND SECTION 39 SAYS SO IN
	// ITS OWN WORDS: "Once active, the live state (started/blocked/done) is the
	// latest progress.step, not a second field to keep in sync." A status field
	// carrying "done" beside a progress stream saying otherwise is the drift
	// that rule exists to prevent.
	//
	// ⛔ AND THE STEP IS APPENDED ONLY ON A RECORD THIS RUN CREATED, WHICH IS
	// THE ONE PLACE THIS SEEDER IS NOT IDEMPOTENT AND CANNOT BE MADE SO TODAY.
	// A record put supersedes, so re-running rewrites version n+1 and nothing
	// accumulates. A progress stream is APPEND-ONLY BY DESIGN, so a second run
	// would file a second "done" against every closed row - nine duplicate
	// steps, each one honestly recorded and none of them true.
	//
	// The check that would fix it does not exist: `rig progress` dispatches
	// `step` and nothing else, so THERE IS NO CLI PATH THAT READS A PROGRESS
	// STREAM, and a caller cannot ask what the latest state already is. Guarding
	// on "did I just create this record" is the honest approximation - it costs
	// a re-run after a run that failed BETWEEN the put and the step, which
	// leaves that row open and visible, rather than silently doubling a stream
	// on every ordinary re-run. BACKLOG.md carries the missing read path.
	if it.Done && !exists {
		if err := o.write([]string{
			"progress", "step", it.ID,
			"--project", o.project,
			"--state", "done",
			"--note", closureNote(it),
		}); err != nil {
			return err
		}
		r.closed = append(r.closed, it.ID)
	}
	r.collect(it)
	return nil
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

func closureNote(it record.BacklogItem) string {
	if it.Struck {
		return "closed in " + "BACKLOG.md" + " by a struck title"
	}
	return "closed in BACKLOG.md by a terminal disposition in its item cell"
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
	closed     []string
	claimsDone []string
	ruled      []string
	malformed  []string
}

func (r *result) collect(it record.BacklogItem) {
	if it.ClaimsDone {
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
	fmt.Fprintf(w, "\n%s %d new and superseded %d, and closed %d with a progress step.\n",
		verb, r.created, r.superseded, len(r.closed))

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
			"      No work was finished; a decision was taken.")
	name("IRREGULAR MARKUP, IMPORTED ANYWAY AND FLAGGED", r.malformed,
		"the row's markdown is irregular enough that its cells ran together.\n"+
			"      Imported with tags=malformed so the document's owner can mend it.")
}
