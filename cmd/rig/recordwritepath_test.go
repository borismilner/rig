package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// ---- standard input, and the put that threw it away ------------------------
//
// BACKLOG.md B75. `echo "prose" | rig record put --kind working-note --project
// a0-survey` answered `created ... at version 1`, exited 0, and stored an
// empty body. Measured against the installed binary on 2026-09-17; the record
// it minted reads back as `{"body":"", ... "version":1}`.
//
// The cause was an ABSENCE and it was confirmed with a positive control:
// `grep -n os.Stdin cmd/rig/*.go` answers with exactly one line, mcp.go's, so
// the instrument is live and the absence in this file is a real absence rather
// than a query that missed. Nothing in the put path ever read standard input,
// so nothing could report that it had not.

// pipedStdin points stdinSource at a pipe carrying exactly these bytes, and
// puts it back afterwards.
//
// ⛔ THE WRITE END IS CLOSED BEFORE THE TEST RUNS. A pipe whose writer is
// still open never reaches EOF, so a reader blocks and the test hangs rather
// than failing - which is the failure mode that looks like an infrastructure
// problem and gets retried instead of read. The payloads here are far under
// the 64 KiB pipe buffer, so writing the whole thing before the close cannot
// deadlock.
func pipedStdin(t *testing.T, content string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString(content); err != nil {
		t.Fatalf("writing the fake standard input: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing the fake standard input's writer: %v", err)
	}
	saved := stdinSource
	stdinSource = func() *os.File { return r }
	t.Cleanup(func() {
		stdinSource = saved
		_ = r.Close()
	})
}

// terminalStdin points stdinSource at a character device.
//
// /dev/null RATHER THAN A PTY, and the difference is the whole point of the
// guard being written against the file mode. /dev/null is a character device
// exactly as a terminal is, so `rig record put < /dev/null` and `rig record
// put` at a prompt are one case here - which is what stops a script that
// redirects from /dev/null being refused for a body it never offered.
func terminalStdin(t *testing.T) {
	t.Helper()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	saved := stdinSource
	stdinSource = func() *os.File { return f }
	t.Cleanup(func() {
		stdinSource = saved
		_ = f.Close()
	})
}

// ⛔ THE ROW B75 IS ACTUALLY ABOUT. The missing flag is a gap; a success that
// destroys what it was handed is a defect, and it is the one that cannot be
// noticed from the outside - a real id came back, the exit code was 0, and
// the caller had no reason to look.
func TestProseOnStandardInputWithNoBodyFlagIsRefusedRatherThanDropped(t *testing.T) {
	f := serving(t, &fakeRecord{})
	pipedStdin(t, "the prose that vanishes\n")

	err := run([]string{
		"record", "put", "--kind", "working-note", "--project", "a0-survey",
	})
	if err == nil {
		t.Fatal("a put with prose on standard input and no body flag " +
			"SUCCEEDED, which is B75: the record is created, the exit code " +
			"is 0 and the prose is gone with nothing saying so")
	}
	// The flag is named, because the caller's next keystroke is the whole
	// value of the refusal. A message that only says "no" leaves them to
	// guess at a surface they have already demonstrated they do not know.
	if !strings.Contains(err.Error(), "--body-file -") {
		t.Errorf("the refusal does not name --body-file -, so it tells the "+
			"caller they are wrong without telling them what to type: %v", err)
	}
	// AND IT REFUSED BEFORE THE WIRE. "it refused" and "it refused without
	// writing anything" are different claims, and only the second one says no
	// half-record was left behind.
	if len(f.calls) != 0 {
		t.Errorf("the refusal reached the daemon first: calls=%v", f.calls)
	}
}

// The escape hatch has to exist or the guard above is a wall. A caller who
// means an empty body says so, and is believed.
func TestAnExplicitEmptyBodySurvivesSomethingOnStandardInput(t *testing.T) {
	f := serving(t, &fakeRecord{})
	pipedStdin(t, "ignore me\n")

	if err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig", "--body", "",
	}); err != nil {
		t.Fatalf("an explicit --body '' was refused, so the guard has no way "+
			"out: %v", err)
	}
	if f.lastPut.Body != "" {
		t.Errorf("body = %q, want the empty string the caller asked for",
			f.lastPut.Body)
	}
}

// A body given on argv is not ambiguous, so nothing is refused and standard
// input is not read.
func TestABodyOnArgvIsNotRefusedForSomethingOnStandardInput(t *testing.T) {
	f := serving(t, &fakeRecord{})
	pipedStdin(t, "not the body\n")

	if err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body", "the body the caller named",
	}); err != nil {
		t.Fatalf("--body with an inherited pipe on standard input was "+
			"refused: %v", err)
	}
	if f.lastPut.Body != "the body the caller named" {
		t.Errorf("body = %q, want what --body said", f.lastPut.Body)
	}
}

// ⛔ A CHARACTER DEVICE IS NOT AN OFFER. `rig record put < /dev/null`, and a
// put typed at a prompt, must both still create a record with no body.
func TestATerminalOnStandardInputDoesNotRefuseABodylessPut(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	if err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
	}); err != nil {
		t.Fatalf("a bodyless put at a terminal was refused, which breaks "+
			"every caller that only sets typed fields: %v", err)
	}
	if f.lastPut.Body != "" {
		t.Errorf("body = %q, want empty", f.lastPut.Body)
	}
}

// ---- B60, the flag that was missing ----------------------------------------

func TestABodyFileReachesTheWireAsTheRecordsBody(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	path := filepath.Join(t.TempDir(), "note.md")
	const prose = "# a heading\n\nand two lines\nof prose"
	if err := os.WriteFile(path, []byte(prose+"\n"), 0o600); err != nil {
		t.Fatalf("writing the body file: %v", err)
	}

	if err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", path,
	}); err != nil {
		t.Fatalf("--body-file was refused: %v", err)
	}
	if f.lastPut.Body != prose {
		t.Errorf("body = %q, want %q", f.lastPut.Body, prose)
	}
}

func TestABodyFileOfDashReadsStandardInput(t *testing.T) {
	f := serving(t, &fakeRecord{})
	pipedStdin(t, "the prose that used to vanish\n")

	if err := run([]string{
		"record", "put", "--kind", "working-note", "--project", "a0-survey",
		"--body-file", "-",
	}); err != nil {
		t.Fatalf("--body-file - was refused: %v", err)
	}
	if f.lastPut.Body != "the prose that used to vanish" {
		t.Errorf("body = %q, want the piped prose", f.lastPut.Body)
	}
}

// ⛔ ONE INTENT MUST NOT STORE TWO VALUES. `echo prose |` ends in a newline
// because that is what a line is, and `--body prose` cannot express one at
// all, so the two routes would otherwise write bodies differing by an
// invisible byte - and a record store is where that difference is compared
// later by something that cannot see it either.
func TestATrailingNewlineIsNotStoredAsPartOfTheBody(t *testing.T) {
	f := serving(t, &fakeRecord{})
	pipedStdin(t, "one line\n")

	if err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", "-",
	}); err != nil {
		t.Fatalf("--body-file - was refused: %v", err)
	}
	if f.lastPut.Body != "one line" {
		t.Errorf("body = %q, want %q: the newline belongs to the pipe, not "+
			"to the prose", f.lastPut.Body, "one line")
	}
}

// Interior and leading whitespace is the caller's, and only the file's own
// terminator is dropped.
func TestOnlyTheTerminatingNewlinesAreDropped(t *testing.T) {
	f := serving(t, &fakeRecord{})
	pipedStdin(t, "  indented\n\n  still the body  \n\n\n")

	if err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", "-",
	}); err != nil {
		t.Fatalf("--body-file - was refused: %v", err)
	}
	const want = "  indented\n\n  still the body  "
	if f.lastPut.Body != want {
		t.Errorf("body = %q, want %q", f.lastPut.Body, want)
	}
}

func TestABodyAndABodyFileTogetherAreRefusedBeforeTheWire(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("from the file"), 0o600); err != nil {
		t.Fatalf("writing the body file: %v", err)
	}

	err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body", "from argv", "--body-file", path,
	})
	if err == nil {
		t.Fatal("--body and --body-file together were accepted, so one of " +
			"two bodies the caller supplied was dropped without a word")
	}
	if !strings.Contains(err.Error(), "--body-file") ||
		!strings.Contains(err.Error(), "--body ") {
		t.Errorf("the refusal does not name both flags: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("the refusal reached the daemon first: calls=%v", f.calls)
	}
}

// The same accident every other flag on this surface already guards: a shell
// variable that expanded to nothing.
func TestAnEmptyBodyFilePathIsRefusedRatherThanReadAsStandardInput(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", "",
	})
	if err == nil {
		t.Fatal("--body-file '' was accepted, and an empty path is what a " +
			"shell variable that expanded to nothing looks like")
	}
	// ⛔ THE MESSAGE IS THE WHOLE POINT AND THE EXIT CODE IS NOT. Without the
	// guard the empty path reaches os.Open and comes back as
	// "open : no such file or directory", which refuses for the right reason
	// and says the wrong thing: the caller did not name a file that is
	// missing, they named nothing at all. Asserting only that it failed
	// leaves this test passing against the defect - measured, 2026-09-17.
	if !strings.Contains(err.Error(), "--body-file -") {
		t.Errorf("the refusal does not offer standard input, so it reads as "+
			"a missing file rather than as an empty variable: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("the refusal reached the daemon first: calls=%v", f.calls)
	}
}

func TestABodyFileThatCannotBeReadIsRefusedBeforeTheWire(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	missing := filepath.Join(t.TempDir(), "not-here.md")
	err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", missing,
	})
	if err == nil {
		t.Fatal("a --body-file that does not exist created a record with an " +
			"empty body, which is B75 again one flag over")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the refusal does not name the path, so the caller cannot "+
			"see which one rig looked for: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("the refusal reached the daemon first: calls=%v", f.calls)
	}
}

// ⛔ THE BOUND IS THE WIRE'S AND IT IS CHECKED HERE SO THE MESSAGE CAN NAME
// THE FILE. internal/wire.MaxFrameSize is 1 MiB and one put is one frame, so
// an unbounded read of a caller-named path is both an allocation nobody
// limits and a frame the daemon will refuse in a vocabulary that says nothing
// about the body.
func TestABodyTooLargeForOneFrameIsRefusedBeforeItIsSent(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	path := filepath.Join(t.TempDir(), "huge.md")
	if err := os.WriteFile(path, make([]byte, maxBodyBytes+1), 0o600); err != nil {
		t.Fatalf("writing the body file: %v", err)
	}

	err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", path,
	})
	if err == nil {
		t.Fatal("a body over the frame ceiling was accepted here and would " +
			"have failed on the wire, where nothing names the body")
	}
	if len(f.calls) != 0 {
		t.Errorf("the oversized body reached the daemon: calls=%v", f.calls)
	}
}

// ⛔ A REFUSAL IN THE WRONG VOCABULARY IS NOT A REFUSAL THE CALLER CAN ACT ON.
// Left to protobuf, a body that is not valid UTF-8 comes back as "client:
// marshal rig.record.put: string field contains invalid UTF-8" - which names
// neither the body nor the file, and a caller who pointed --body-file at a
// binary has no route from that sentence to what they typed. Found by running
// it against the live daemon, 2026-09-17.
func TestABodyThatIsNotTextIsRefusedInTheBodysOwnWords(t *testing.T) {
	f := serving(t, &fakeRecord{})
	terminalStdin(t)

	path := filepath.Join(t.TempDir(), "binary.bin")
	if err := os.WriteFile(path, []byte("valid \xff\xfe tail\n"), 0o600); err != nil {
		t.Fatalf("writing the body file: %v", err)
	}

	err := run([]string{
		"record", "put", "--kind", "note", "--project", "rig",
		"--body-file", path,
	})
	if err == nil {
		t.Fatal("a body that is not valid UTF-8 was accepted here and would " +
			"have died on the wire in protobuf's vocabulary")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the refusal does not name the file: %v", err)
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("the refusal does not say what is wrong with it: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("the bad body reached the daemon: calls=%v", f.calls)
	}
}

// The offset is the point: "somewhere in this file" is not an answer on a file
// nobody is going to read by hand.
func TestTheFirstBadByteIsNamedByOffset(t *testing.T) {
	if got := firstInvalidUTF8([]byte("ok \xff")); got != 3 {
		t.Errorf("firstInvalidUTF8 = %d, want 3", got)
	}
	// A valid multi-byte rune must not be mistaken for one.
	if got := firstInvalidUTF8([]byte("héllo \xff")); got != 7 {
		t.Errorf("firstInvalidUTF8 over a two-byte rune = %d, want 7", got)
	}
}

// ---- the 413-column table --------------------------------------------------
//
// Measured 2026-09-17 against the installed binary: `rig record query` with no
// filter answers 568 lines whose longest is 413 columns, on a store whose
// widest id is 161 characters. The id column is sized to that widest id, so a
// row whose id is three characters - a backlog item, and there are 70 of them
// - is padded out to 161 before its project is printed.

// queryRows is a listing with the two shapes that matter: a doc-key id long
// enough to be the whole line, and a backlog id short enough that the padding
// is the only reason its row is wide.
func queryRows() []Record {
	long := "2026-09-17-the-lead-section-6-s-must-read-set-is-bare-ids-and-" +
		"the-clause-that-sa/the-evidence-cuts-against-the-clause-and-it-came-" +
		"from-the-lead-s-own-earlier-mes"
	return []Record{
		{
			ID: long, Version: 1, Kind: "decision", Project: "rig",
			Body: "the evidence cuts against the clause and it came from " +
				"the lead's own earlier message, which is the part that " +
				"decides it rather than the ruling itself",
		},
		{
			ID: "B75", Version: 2, Kind: "work-item", Project: "rig",
			Fields: map[string]string{titleKey: "the record write path " +
				"reports success and stores nothing"},
		},
	}
}

func longQueryID() string { return queryRows()[0].ID }

// ⛔ THE EXACT BYTES OF THE DEFECT. Every row was padded to the width of a
// 161-character id, so a three-character backlog id started its PROJECT cell
// at column 163.
func TestNoRowIsPaddedOutToTheWidestId(t *testing.T) {
	const width = 100
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), width)

	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		n := utf8.RuneCountInString(line)
		if n <= width {
			continue
		}
		if strings.Contains(line, longQueryID()) {
			// The id is 161 characters and the terminal is 100. Nothing can
			// make that fit, and cutting it is the one repair that costs more
			// than the overrun does.
			continue
		}
		t.Errorf("a line is %d columns wide at a %d-column terminal and no "+
			"id is what made it so:\n%s", n, width, line)
	}
}

// ⛔ AN ID IS NEVER ELIDED, AND THIS IS THE CLAUSE THE HOUSE RULE PINS.
// briefFitAround shrinks the widest column that is NOT pinned; measured at 80
// columns against the live store, five of six governing ids rendered the same
// stub because the cut fell inside a shared prefix. A cut id cannot be pasted
// into `rig record get`, which is the only reason the column is printed.
func TestAnIdIsNeverCutToFitTheTerminal(t *testing.T) {
	for _, width := range []int{120, 100, 60, 20} {
		got := queryText(QueryArgs{Project: "rig"}, queryRows(), width)
		if !strings.Contains(got, longQueryID()) {
			t.Errorf("at %d columns the 161-character id was cut, so it can "+
				"no longer be fetched:\n%s", width, got)
		}
		if !strings.Contains(got, "B75") {
			t.Errorf("at %d columns the short id vanished:\n%s", width, got)
		}
	}
}

// ⛔ CLAUSE 3: A CELL IS CUT ONLY AT A REAL TERMINAL. brief.go states it at
// briefStyle.Width - "discarding bytes it was going to read in full is
// destruction rather than legibility" - and its own first run of this code put
// an ellipsis into a pipe by passing the layout default to the fit.
func TestAPipeGetsTheWholeTableUncut(t *testing.T) {
	rs := queryRows()
	got := queryText(QueryArgs{Project: "rig"}, rs, 0)

	for _, r := range rs {
		if !strings.Contains(got, r.ID) {
			t.Errorf("an id was cut at a pipe:\n%s", got)
		}
		if !strings.Contains(got, recordSummary(r)) {
			t.Errorf("a summary was cut at a pipe:\n%s", got)
		}
	}
	if strings.Contains(got, "…") {
		t.Errorf("an ellipsis reached a pipe, where there is no width to "+
			"fit:\n%s", got)
	}
}

// ⛔ CLAUSE 2: THE LAYOUT IS CHOSEN AT A DEFAULT WIDTH WHEN THERE IS NO
// TERMINAL. A layout costs the reader no bytes, so a pipe gets the stepped-out
// form rather than the 413-column one it used to get.
func TestAPipeStillGetsALayoutRatherThanTheWidestId(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), 0)

	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "B75") {
			continue
		}
		if utf8.RuneCountInString(line) > 80 {
			t.Fatalf("the B75 line is %d columns at a pipe, so the layout was "+
				"still chosen off the widest id:\n%s",
				utf8.RuneCountInString(line), line)
		}
		return
	}
	t.Fatalf("no B75 line at all:\n%s", got)
}

// ⛔ AND THE DEFAULT IS LOAD-BEARING, WHICH THE TEST ABOVE DOES NOT SHOW. With
// no fallback the layout budget at a pipe is zero, nothing can ever fit it, and
// EVERY listing steps its id out - including one whose ids are three characters
// long. Found by mutation: removing the fallback left the test above green.
func TestAQueryPipeWithShortIdsKeepsTheColumnTable(t *testing.T) {
	rs := []Record{
		{
			ID: "B75", Version: 2, Kind: "work-item", Project: "rig",
			Fields: map[string]string{titleKey: "the write path drops prose"},
		},
		{
			ID: "B60", Version: 1, Kind: "work-item", Project: "rig",
			Fields: map[string]string{titleKey: "a body that is not argv"},
		},
	}
	got := queryText(QueryArgs{Project: "rig"}, rs, 0)

	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if strings.TrimSpace(line) == "B75" {
			t.Fatalf("B75 stepped out of a table it fits in, at a pipe:\n%s",
				got)
		}
	}
	if !strings.Contains(got, "ID") {
		t.Errorf("the ID column is gone from a listing whose ids fit:\n%s", got)
	}
}

// ⛔ THE LAYOUT IS ALL-OR-NOTHING FOR THE TABLE. Two shapes of row in one table
// is a second rule: a reader scanning a column cannot tell which kind of line
// they are on. Either every id sits in the column or every id steps out.
func TestTheLayoutIsOneShapeForTheWholeTable(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), 100)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	var idLines int
	for _, line := range lines {
		for _, r := range queryRows() {
			if strings.TrimSpace(line) == r.ID {
				idLines++
			}
		}
	}
	if idLines != len(queryRows()) {
		t.Fatalf("%d of %d ids are on lines of their own, so the table has "+
			"two shapes of row in it:\n%s", idLines, len(queryRows()), got)
	}
}

// When every id fits, the table stays one line per record and the id is
// pinned, so the summary is what pays for any squeeze.
func TestAListingWhoseIdsFitKeepsOneLinePerRecord(t *testing.T) {
	rs := []Record{
		{
			ID: "B75", Version: 2, Kind: "work-item", Project: "rig",
			Fields: map[string]string{titleKey: strings.Repeat("long ", 60)},
		},
		{
			ID: "B60", Version: 1, Kind: "work-item", Project: "rig",
			Fields: map[string]string{titleKey: "a body that is not argv"},
		},
	}
	const width = 80
	got := queryText(QueryArgs{Project: "rig"}, rs, width)

	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if n := utf8.RuneCountInString(line); n > width {
			t.Errorf("a line is %d columns wide at %d:\n%s", n, width, line)
		}
	}
	// One line per record and a header, plus the blank line and the count that
	// queryText adds after the table.
	body := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(body) != len(rs)+3 {
		t.Fatalf("want a header, %d rows, a blank and a count; got %d "+
			"lines:\n%s", len(rs), len(body), got)
	}
	// ⛔ THE SUMMARY PAID, NOT THE ID. The pin is what decides which.
	if !strings.Contains(got, "B75") || !strings.Contains(got, "B60") {
		t.Errorf("an id was cut in the inline form:\n%s", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("a 300-character title fitted an 80-column line without "+
			"being cut, so nothing paid:\n%s", got)
	}
}

// The header moves with the columns or it labels the wrong ones.
func TestTheSteppedOutHeaderSitsOverTheColumnsItNames(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), 100)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	var header string
	var at int
	for _, line := range lines {
		if at = strings.Index(line, "VERSION"); at >= 0 {
			header = line
			break
		}
	}
	if header == "" {
		t.Fatalf("no VERSION column in any line:\n%s", got)
	}
	if at != 0 {
		t.Errorf("VERSION is at column %d and the id has stepped out, so it "+
			"should lead the table: %q", at, header)
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "v2") {
			return
		}
	}
	t.Fatalf("no row carries v2 under VERSION:\n%s", got)
}

// ⛔ THE ID SITS UNDER ITS OWN ROW AND INDENTED TO THE FIRST COLUMN. At the
// left margin it reads as a record of its own; under the wrong row it is
// worse than absent.
func TestEachSteppedOutIdSitsUnderItsOwnRowAndIndented(t *testing.T) {
	rs := queryRows()
	got := queryText(QueryArgs{Project: "rig"}, rs, 100)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	for _, r := range rs {
		found := false
		for i, line := range lines {
			if strings.TrimSpace(line) != r.ID {
				continue
			}
			found = true
			if !strings.HasPrefix(line, " ") {
				t.Errorf("the id line starts at the left margin, so it reads "+
					"as a record of its own: %q", line)
			}
			if i == 0 || !strings.HasPrefix(lines[i-1], "v") {
				t.Errorf("the id at line %d does not sit under a row: "+
					"above it is %q", i, lines[max(i-1, 0)])
			}
		}
		if !found {
			t.Fatalf("no line carries the id %q:\n%s", r.ID, got)
		}
	}
}
