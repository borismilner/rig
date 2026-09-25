package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// cmdPeers prints the estate's roster (BACKLOG.md B41).
//
// THE VERB EXISTS BECAUSE THE DATA HAD NO WAY OUT. rig.peers has been on the
// wire since row 1 and the MCP door serves it as list_agents, so both an agent
// and a Go program could read the roster and a person at a prompt could not.
// That is the wrong way round: the roster is the thing a person WATCHES, and
// supervision of a seat that has moved onto rig is exactly what was missing.
//
// It is also what a live refusal already told people to run. presence_serve.go
// once answered a caller refused a held seat with "Run `rig peers` to see the
// roster", and the word fell through run()'s default branch to the PROGRAM
// path, so the caller was told its own program did not exist. That sentence
// was deleted at 7ee7233 and cmd/rig/refusal_verbs_test.go now stops another
// one shipping - but the citation was only wrong because the verb was missing,
// and this file is the half that makes it true rather than the half that
// forbade saying it.
//
// PeersRequest is empty, on rig.estate's precedent: the roster is a property
// of the daemon this shell reached, and a request field selecting one would be
// a second way to name an estate that XDG_RUNTIME_DIR already names.
func cmdPeers(args []string) (err error) {
	fs, asJSON, timeout := peersFlagSet()
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) != 0 {
		return badArgumentf("usage: rig peers [--json] [--timeout=30s]")
	}

	c, err := connect()
	if err != nil {
		// Same reading as `rig estate` and the opposite of `rig down`: an
		// unreachable socket is not a form of this answer. "Who is here" has
		// no true answer when nothing was reached, and an empty roster is a
		// REAL answer that a caller must never confuse with a dead daemon.
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resp := &verbsv1.PeersResponse{}
	if err := call(ctx, c, "rig.peers", &verbsv1.PeersRequest{}, resp); err != nil {
		return err
	}

	// THE LEASES RIDE ON THE SAME ANSWER, AND A REFUSAL OF THEM IS PART OF IT.
	// Section 16's leases are the other half of "who is doing what here": a
	// seat on the roster says what it is for, a lease says what it holds. An
	// estate with no lease store, or a daemon older than the verb, refuses
	// rig.lease.list, and that refusal is reported beside the roster rather
	// than failing it: the roster is still true.
	leases := &verbsv1.LeaseListResponse{}
	unavailable := ""
	if err := call(ctx, c, "rig.lease.list", &verbsv1.LeaseListRequest{}, leases); err != nil {
		unavailable = err.Error()
	}

	if *asJSON {
		obj := peersJSON(resp, time.Now())
		obj["leases"] = leasesJSON(leases)
		obj["leases_unavailable"] = unavailable
		return json.NewEncoder(os.Stdout).Encode(obj)
	}
	fmt.Print(peersText(resp, time.Now()))
	fmt.Print(leasesText(leases, unavailable))
	return nil
}

// leasesJSON is every lease as the wire spells it, each key present on every
// row, and an empty array rather than null when nothing is held.
func leasesJSON(r *verbsv1.LeaseListResponse) []map[string]any {
	out := make([]map[string]any, 0, len(r.GetLeases()))
	for _, l := range r.GetLeases() {
		state, ok := enumWord(l.GetState(), "LEASE_STATE_")
		if !ok {
			state = skewToken(l.GetState())
		}
		live, ok := enumWord(l.GetLiveness(), "LIVENESS_")
		if !ok {
			live = skewToken(l.GetLiveness())
		}
		out = append(out, map[string]any{
			"name":          l.GetName(),
			"state":         state,
			"holder":        l.GetHolder(),
			"token":         l.GetToken(),
			epochKey:        l.GetEpoch(),
			"witness":       l.GetWitness(),
			"remaining_ms":  l.GetRemainingMs(),
			"owner_gone":    l.GetOwnerGone(),
			"liveness":      live,
			"needs_break":   l.GetNeedsBreak(),
			"broken_by":     l.GetBrokenBy(),
			"broken_reason": l.GetBrokenReason(),
		})
	}
	return out
}

// leasesText is the human rendering of the leases, under the roster.
//
// THE TWO FACTS A READER ACTS ON GET WORDS, NOT FLAGS: an owner observed dead,
// and an orphan that needs a recorded break. Both are why section 16 makes a
// read report liveness at all.
func leasesText(r *verbsv1.LeaseListResponse, unavailable string) string {
	var b strings.Builder
	b.WriteString("\n")
	if unavailable != "" {
		b.WriteString("leases: not available from this daemon: " + unavailable + "\n")
		return b.String()
	}
	if len(r.GetLeases()) == 0 {
		b.WriteString("No lease has ever been taken on this estate.\n")
		return b.String()
	}
	rows := make([][]string, 0, len(r.GetLeases()))
	for _, l := range r.GetLeases() {
		state, ok := enumWord(l.GetState(), "LEASE_STATE_")
		if !ok {
			state = skewToken(l.GetState())
		}
		rows = append(rows, []string{
			l.GetName(), state, leaseHolderCell(l), leaseLeftCell(l), leaseNoteCell(l),
		})
	}
	writeTable(&b, []string{"LEASE", "STATE", "HOLDER", "LEFT", "NOTE"}, rows)
	return b.String()
}

func leaseHolderCell(l *verbsv1.Lease) string {
	if l.GetHolder() == "" {
		return "-"
	}
	return l.GetHolder() + " (" + l.GetWitness() + ")"
}

// leaseLeftCell is the time to the deadline, and only a held lease has one
// worth reading.
func leaseLeftCell(l *verbsv1.Lease) string {
	if l.GetState() != verbsv1.LeaseState_LEASE_STATE_HELD {
		return "-"
	}
	return (time.Duration(l.GetRemainingMs()) * time.Millisecond).Round(time.Second).String()
}

func leaseNoteCell(l *verbsv1.Lease) string {
	switch {
	case l.GetNeedsBreak():
		return "orphaned and unwitnessed: needs a recorded break"
	case l.GetOwnerGone() && l.GetState() == verbsv1.LeaseState_LEASE_STATE_HELD:
		return "holder observed dead: frees at its deadline"
	case l.GetBrokenBy() != "" && l.GetState() == verbsv1.LeaseState_LEASE_STATE_FREE:
		return "broken by " + l.GetBrokenBy() + ": " + l.GetBrokenReason()
	case l.GetOwnerGone():
		return "holder observed dead"
	default:
		return ""
	}
}

// peersFlagSet is `peers`'s flags, built here rather than inline so
// TestEveryFlagThatTakesAValueIsDeclaredToThePartitioner can WALK them. A
// verb whose flags are built inside its own body is invisible to that test.
func peersFlagSet() (fs *flag.FlagSet, asJSON *bool, timeout *time.Duration) {
	fs = flag.NewFlagSet("peers", flag.ContinueOnError)
	asJSON = fs.Bool("json", false, "emit JSON")
	timeout = fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	return fs, asJSON, timeout
}

// ---- rendering -------------------------------------------------------------

// stateLabel is the spelling for a seat state, walked off the wire's own enum
// descriptor rather than carried as a table here, exactly as roleLabel is.
//
// ok=false is a newer daemon on the same wire major - states are extensible
// within a major - and the caller renders that as its own case. It must NOT
// fall back to the zero's spelling, which would report a skew as "nothing was
// said".
func stateLabel(s verbsv1.SeatState) (string, bool) {
	return enumWord(s, "SEAT_STATE_")
}

// peersJSON is the object --json emits. Section 10: the proto's own field
// names are the contract, so they are spelled as the wire spells them.
//
// EVERY KEY IS PRESENT ON EVERY ROW AND ON EVERY ANSWER, no omitempty
// anywhere, which is the argument estateJSON and meta's answerJSON both write
// down. An absent key reads as "this was never considered"; an empty seat is
// the ANSWER for a peer that announced without claiming one, not a missing
// value.
//
// `crew` IS AN EMPTY ARRAY AND NEVER null WHEN NOBODY HAS ANNOUNCED. A null
// there is indistinguishable from a field this build failed to set, and the
// empty roster is a state a reader reaches often - `rig peers` on a daemon
// nothing has announced to is the ordinary first call.
func peersJSON(r *verbsv1.PeersResponse, now time.Time) map[string]any {
	crew := make([]map[string]any, 0, len(r.GetCrew()))
	for _, s := range r.GetCrew() {
		crew = append(crew, seatJSON(s, now))
	}
	return map[string]any{
		"crew":    crew,
		"partial": r.GetPartial(),
	}
}

// seatJSON is one row.
//
// state_number rides beside state for the reason estateJSON spends a paragraph
// on role_number: without it a client meeting a fourth state can report that it
// did not understand and cannot say WHAT it did not understand. It is emitted
// always, because a key that appears only when something is wrong is a key
// nobody's parser has a branch for at the moment it first appears.
//
// The two ages are derived here rather than left to the reader. The timestamps
// are emitted too, unchanged, because a consumer that wants to compute against
// its own clock must not be forced through this one.
func seatJSON(s *verbsv1.Seat, now time.Time) map[string]any {
	label, ok := stateLabel(s.GetState())
	if !ok {
		label = skewToken(s.GetState())
	}
	return map[string]any{
		seatKey:               s.GetSeat(),
		"generation":          s.GetGeneration(),
		epochKey:              s.GetEpoch(),
		"estate":              s.GetEstate(),
		"purpose":             s.GetPurpose(),
		"activity":            s.GetActivity(),
		"state":               label,
		"state_number":        int32(s.GetState().Number()),
		"announced_unix_nano": s.GetAnnouncedUnixNano(),
		"activity_unix_nano":  s.GetActivityUnixNano(),
		"announced_age_s":     ageSeconds(s.GetAnnouncedUnixNano(), now),
		"activity_age_s":      ageSeconds(s.GetActivityUnixNano(), now),
	}
}

// ageSeconds is how long ago a timestamp was, or -1 for one that was never
// set.
//
// A ZERO TIMESTAMP IS NOT THE UNIX EPOCH HERE AND MUST NOT RENDER AS AN AGE.
// Computed straight it would report roughly 56 years, which is a number a
// reader will believe less than they believe a negative one. -1 is out of the
// range a real age can take, so it cannot be mistaken for a measurement.
func ageSeconds(unixNano int64, now time.Time) int64 {
	if unixNano == 0 {
		return -1
	}
	d := now.Sub(time.Unix(0, unixNano))
	if d < 0 {
		// A row stamped in the future. The daemon and this client share a
		// clock today, so this is unreachable rather than tolerated - but it
		// costs one line to not print a negative age that means something
		// else.
		return 0
	}
	return int64(d / time.Second)
}

// peersText is the human rendering. It takes the response and the clock rather
// than reaching either, so every case below is testable without a live daemon.
func peersText(r *verbsv1.PeersResponse, now time.Time) string {
	var b strings.Builder

	// THE EMPTY ROSTER IS A SENTENCE, NOT A BLANK. A bare header over nothing
	// reads as a broken command, and this is the ordinary answer on a daemon
	// nothing has announced to yet. It also says what WOULD put a row here,
	// because the reader of an empty roster is usually the person who expected
	// not to be alone.
	if len(r.GetCrew()) == 0 {
		b.WriteString("nobody has announced to this estate.\n")
		b.WriteString("A row appears when a session calls announce; presence " +
			"is connection state, so\nrig remembers nobody who has " +
			"disconnected.\n")
		b.WriteString(peersPartialLine(r.GetPartial()))
		return b.String()
	}

	rows := make([][]string, 0, len(r.GetCrew()))
	for _, s := range r.GetCrew() {
		rows = append(rows, []string{
			peersSeatCell(s),
			peersStateCell(s),
			peersAgeCell(s.GetActivityUnixNano(), now),
			s.GetPurpose(),
			s.GetActivity(),
		})
	}
	writeTable(&b, []string{"SEAT", "STATE", "AGE", "PURPOSE", "ACTIVITY"}, rows)
	b.WriteString(peersPartialLine(r.GetPartial()))
	return b.String()
}

// peersSeatCell renders the seat and its generation, and the unseated case is
// a FACT rather than a blank.
//
// An empty seat with generation 0 is a peer that announced WITHOUT claiming a
// role, which the wire allows on purpose: a one-off session is present and
// addressable without pretending to be a seat somebody inherits. Rendered as
// an empty cell it reads as a daemon that failed to send the name. The
// parenthesised form cannot be a seat name, because a seat name is drawn from
// the same closed shape as a program's.
//
// THE GENERATION IS PRINTED BESIDE THE NAME AND NOT IN A COLUMN OF ITS OWN,
// because the two are one identity: `backend-1` at generation 2 is a different
// occupancy from `backend-1` at generation 1, and a reader scanning a column
// of names sees one seat where there were two.
func peersSeatCell(s *verbsv1.Seat) string {
	if s.GetSeat() == "" {
		return "(no seat)"
	}
	return fmt.Sprintf("%s#%d", s.GetSeat(), s.GetGeneration())
}

// peersStateCell renders the state, and the two cases that are not facts about
// the seat say so in the cell rather than leaving the reader to know.
func peersStateCell(s *verbsv1.Seat) string {
	label, ok := stateLabel(s.GetState())
	switch {
	case !ok:
		// A newer daemon on this wire major. The word says which side is old,
		// so the reader does not go looking at the daemon.
		return skewToken(s.GetState())
	case s.GetState() == verbsv1.SeatState_SEAT_STATE_UNSPECIFIED:
		// Section 21: the zero means nothing was said and is never a fact
		// about a seat. A reader who sees this is looking at a defect.
		return "(not said)"
	default:
		return label
	}
}

// peersAgeCell renders how long an activity line has stood.
//
// THE AGE IS THE POINT OF THE COLUMN AND THE REASON THE DAEMON HAS THE AGE
// RULE. Re-sending an unchanged line deliberately does not reset it, so a
// growing age is the board's only signal that a session is repeating itself
// rather than progressing. A column that rendered "now" for every row because
// each seat keeps calling activity would delete that signal.
func peersAgeCell(unixNano int64, now time.Time) string {
	s := ageSeconds(unixNano, now)
	switch {
	case s < 0:
		return "-"
	case s < 60:
		return strconv.FormatInt(s, 10) + "s"
	case s < 3600:
		return strconv.FormatInt(s/60, 10) + "m"
	default:
		return strconv.FormatInt(s/3600, 10) + "h"
	}
}

// peersPartialLine says what `partial` MEANS rather than printing the flag.
//
// ⛔ THIS FIELD HAS THREE MEANINGS ACROSS TWO PRODUCTS AND THIS IS THE ONE
// PLACE A PERSON READS IT. AgentBox's `partial` means "the roster cannot see
// everybody" in general; rig presence's means one specific, checkable thing -
// another named estate is running on this machine and its peers are not in
// this list. A line reading `partial: true` invites the AgentBox reading,
// which is a general disclaimer, and the whole value of rig's version is that
// it is not one.
//
// FALSE GETS A LINE TOO. Silence on the negative case is what teaches a reader
// to treat absence as a value, and "this is everybody" is the answer somebody
// deciding whether they are alone actually needs.
func peersPartialLine(partial bool) string {
	if partial {
		return "\npartial: another estate is running on this machine and its " +
			"peers are not\nlisted here. rig cannot see across estates - two " +
			"estates are two runtime\ndirectories with no shared state.\n"
	}
	return "\nThis is everybody on this estate.\n"
}

// writeTable lays out a header and its rows in aligned columns.
//
// THE LAST COLUMN IS NEVER PADDED, which is what keeps a purpose or an
// activity from dragging a trailing run of spaces across the terminal, and it
// is why this does not reach for text/tabwriter: tabwriter pads the final cell
// too, and every row here ends in free text.
func writeTable(b *strings.Builder, header []string, rows [][]string) {
	width := make([]int, len(header))
	for i, h := range header {
		width[i] = len(h)
	}
	for _, r := range rows {
		for i, cell := range r {
			if len(cell) > width[i] {
				width[i] = len(cell)
			}
		}
	}
	line := func(cells []string) {
		for i, cell := range cells {
			if i == len(cells)-1 {
				b.WriteString(cell)
				break
			}
			fmt.Fprintf(b, "%-*s  ", width[i], cell)
		}
		b.WriteByte('\n')
	}
	line(header)
	for _, r := range rows {
		line(r)
	}
}
