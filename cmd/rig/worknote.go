package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig worknote` - section 09's working notes at the prompt.
//
//	rig worknote write [--body B | --body-file F|-] [--tag t]... [--part-of ID]...
//	rig worknote mine [--project P] [--limit N]
//	rig worknote about <id> [--limit N]
//
// ⛔ THE BODY COMES THROUGH --body-file AND STANDARD INPUT, NOT ONLY ARGV, and
// that is plan/09's B60: "a working note is PROSE, and the one input path is
// the one that cannot carry it". It reuses `resolveBody`, so B75's refusal to
// silently drop a pipe is the same code and cannot drift from record put's.
//
// `mine` TAKES NO --seat. The seat is the daemon's, read off the connection's
// occupancy, so this can only ever answer the caller's own notes.

// ⛔ THE TWO NUMBERS BELOW ARE COPIES, AND THE COPY IS DELIBERATE. `cmd/rig`
// MUST NOT LINK internal/worknote, because that package imports
// internal/record and internal/record links the SQLite driver: section 22
// puts the driver in rigd alone, and section 46 decision 1 is built on it -
// the DAEMON takes the snapshot because only the daemon links the store.
// TestTheClientLinksNoSQLiteDriver enforces it and caught this import.
//
// The copies are pinned to the originals by
// TestTheClientsWorkNoteConstantsMatchThePackage, which is a test and may
// import anything.
const (
	// workNoteStamp renders a note's instant for a person, to the second, in
	// UTC. It is worknote.Stamp.
	workNoteStamp = "2006-01-02T15:04:05Z"

	// workNoteMaxLimit is the most notes one read answers. It is
	// worknote.MaxLimit, and it appears here only in the sentence that tells
	// a caller how far --limit goes.
	workNoteMaxLimit = 200
)

// idList is a repeatable --part-of.
type idList []string

func (l *idList) String() string     { return strings.Join(*l, ",") }
func (l *idList) Set(v string) error { *l = append(*l, v); return nil }

type worknoteFlags struct {
	fs       *flag.FlagSet
	asJSON   *bool
	timeout  *time.Duration
	limit    *uint
	project  *string
	body     *string
	bodyFile *string
	tags     tagList
	partOf   idList
}

func worknoteFlagSet() *worknoteFlags {
	w := &worknoteFlags{fs: flag.NewFlagSet("worknote", flag.ContinueOnError)}
	w.asJSON = w.fs.Bool("json", false, "emit JSON")
	w.timeout = w.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	w.limit = w.fs.Uint("limit", 0, "mine, about: at most this many notes (default 20, at most 200)")
	w.project = w.fs.String("project", "", "write: which project's record; mine: narrow to one project")
	w.body = w.fs.String("body", "", "write: the prose, on the command line")
	w.bodyFile = w.fs.String("body-file", "", "write: read the prose from a file, or from standard input as -")
	w.fs.Var(&w.tags, "tag", "write: a tag with no whitespace in it (repeatable)")
	w.fs.Var(&w.partOf, "part-of", "write: attach the note to a record id (repeatable)")
	return w
}

func (w *worknoteFlags) wasSet(name string) bool {
	seen := false
	w.fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func cmdWorkNote(args []string) (err error) {
	w := worknoteFlagSet()
	flags, positional := partition(args)
	if err := w.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *w.asJSON) }()
	usage := "usage: rig worknote write [--body B | --body-file F|-] [--tag t]... " +
		"[--part-of ID]... [--project P] | mine [--project P] [--limit N] | about <id> [--limit N]"
	if len(positional) == 0 {
		return badArgumentf("%s", usage)
	}
	sub, rest := positional[0], positional[1:]
	switch {
	case sub == "write" && len(rest) == 0:
	case sub == "mine" && len(rest) == 0:
	case sub == "about" && len(rest) == 1:
	default:
		return badArgumentf("%s", usage)
	}

	// THE BODY IS RESOLVED BEFORE THE DIAL, so a caller who piped prose with
	// no flag to route it is refused without a daemon having to be running.
	var body string
	if sub == "write" {
		if body, err = resolveBody("rig worknote write",
			w.wasSet("body"), w.wasSet("body-file"), *w.body, *w.bodyFile); err != nil {
			return err
		}
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *w.timeout)
	defer cancel()
	out := json.NewEncoder(os.Stdout)
	limit := uint32(min(*w.limit, 1<<16))

	switch sub {
	case "write":
		var resp verbsv1.WorkNoteWriteResponse
		if err := call(ctx, c, "rig.worknote.write", &verbsv1.WorkNoteWriteRequest{
			Project: *w.project, Body: body, Tags: w.tags, PartOf: w.partOf,
		}, &resp); err != nil {
			return err
		}
		return printWorkNote(out, resp.GetNote(), *w.asJSON)
	case "mine":
		var resp verbsv1.WorkNoteMineResponse
		if err := call(ctx, c, "rig.worknote.mine", &verbsv1.WorkNoteMineRequest{
			Project: *w.project, Limit: limit,
		}, &resp); err != nil {
			return err
		}
		return printWorkNotes(out, resp.GetNotes(), resp.GetTotal(), *w.asJSON,
			"you have written no working notes here yet. `rig worknote write` starts one.")
	default:
		var resp verbsv1.WorkNoteAboutResponse
		if err := call(ctx, c, "rig.worknote.about", &verbsv1.WorkNoteAboutRequest{
			Id: rest[0], Limit: limit,
		}, &resp); err != nil {
			return err
		}
		return printWorkNotes(out, resp.GetNotes(), resp.GetTotal(), *w.asJSON,
			"no working note is attached to "+rest[0]+". That is an answer: nobody has written one.")
	}
}

// workNoteMap is one note as --json emits it, and as the human form reads off.
func workNoteMap(n *verbsv1.WorkNote) map[string]any {
	tags, fields := n.GetTags(), n.GetFields()
	if tags == nil {
		tags = []string{}
	}
	if fields == nil {
		fields = map[string]string{}
	}
	m := map[string]any{
		"id": n.GetId(), "project": n.GetProject(), "body": n.GetBody(),
		"tags": tags, "fields": fields,
		seatKey: n.GetProv().GetSeat(), "session": n.GetProv().GetSession(),
		epochKey: n.GetProv().GetEpoch(), "at_unix_nano": n.GetProv().GetAtUnixNano(),
		"written": workNoteWritten(n),
	}
	// ⛔ THE ATTACHMENT ACCOUNT IS OMITTED ON A READ RATHER THAN EMITTED
	// EMPTY, because it is what one WRITE did. An empty `missing` beside a
	// read would read as "nothing was missing", which is a claim this answer
	// is not making.
	if len(n.GetAttached()) > 0 || len(n.GetMissing()) > 0 {
		m["attached"] = n.GetAttached()
		m["missing"] = n.GetMissing()
	}
	return m
}

// workNoteWritten renders the note's instant for a person.
func workNoteWritten(n *verbsv1.WorkNote) string {
	nanos := n.GetProv().GetAtUnixNano()
	if nanos == 0 {
		return ""
	}
	return time.Unix(0, nanos).UTC().Format(workNoteStamp)
}

func printWorkNote(out *json.Encoder, n *verbsv1.WorkNote, asJSON bool) error {
	if asJSON {
		return out.Encode(workNoteMap(n))
	}
	fmt.Printf("%s  %s  %s\n", n.GetId(), workNoteWritten(n), n.GetProv().GetSeat())
	if len(n.GetTags()) > 0 {
		fmt.Printf("tags: %s\n", strings.Join(n.GetTags(), " "))
	}
	if len(n.GetAttached()) > 0 {
		fmt.Printf("attached to: %s\n", strings.Join(n.GetAttached(), " "))
	}
	// ⛔ THE MISSING IDS ARE PRINTED LOUDLY AND ARE NOT AN ERROR. The note was
	// written; what was lost is the association, and a caller that is not told
	// which id held no record cannot fix the typo.
	if len(n.GetMissing()) > 0 {
		fmt.Printf("NOT attached, no such record: %s\n", strings.Join(n.GetMissing(), " "))
	}
	fmt.Printf("\n%s\n", n.GetBody())
	return nil
}

func printWorkNotes(out *json.Encoder, notes []*verbsv1.WorkNote, total uint64, asJSON bool, empty string) error {
	if asJSON {
		rows := make([]map[string]any, 0, len(notes))
		for _, n := range notes {
			rows = append(rows, workNoteMap(n))
		}
		return out.Encode(map[string]any{"notes": rows, "total": total})
	}
	if len(notes) == 0 {
		fmt.Println(empty)
		return nil
	}
	for _, n := range notes {
		fmt.Printf("%s  %s  %s", n.GetId(), workNoteWritten(n), n.GetProv().GetSeat())
		if len(n.GetTags()) > 0 {
			fmt.Printf("  [%s]", strings.Join(n.GetTags(), " "))
		}
		fmt.Printf("\n    %s\n", noteLine(n.GetBody()))
	}
	// ⛔ THE COUNT IS PRINTED WHENEVER THE ANSWER WAS CUT, because an answer
	// that is cut without saying so is indistinguishable from a complete one.
	if total > uint64(len(notes)) {
		fmt.Printf("\n%d of %d. `--limit` raises it, up to %d.\n", len(notes), total, workNoteMaxLimit)
	}
	return nil
}

// noteLine is the one line a list shows of a note's prose.
//
// ⛔ IT WRAPS record.go's firstLine RATHER THAN REPLACING IT, so the "there
// was more" marker that function exists for is kept and only the WIDTH is
// added: a note's first line can itself be a paragraph, and a list column
// that runs off the terminal is a list nobody can scan. The whole body is one
// `--json` or one `about` away, so this surface is a way to FIND a note
// rather than to read one.
func noteLine(body string) string {
	line := firstLine(body)
	const most = 100
	if len(line) > most {
		return line[:most] + " ..."
	}
	return line
}
