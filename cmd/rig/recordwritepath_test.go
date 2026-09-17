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

// ⛔ THE EXACT BYTES OF THE DEFECT. The short-id row carries three characters
// of id and is dragged to the width of a 161-character one.
func TestAShortIdRowIsNotPaddedOutToTheWidestId(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), 100)

	for _, line := range strings.Split(got, "\n") {
		if !strings.HasPrefix(line, "B75") {
			continue
		}
		if n := utf8.RuneCountInString(line); n > 100 {
			t.Fatalf("the B75 row is %d columns wide at a 100-column "+
				"terminal. Its id is 3 characters; the width is padding "+
				"bought for a row it does not share:\n%s", n, line)
		}
		return
	}
	t.Fatalf("no row for B75 was rendered at all, so this test asserted "+
		"nothing:\n%s", got)
}

// Every line fits, except where the id ALONE does not - and then the overrun
// is the id's length rather than the table's layout.
func TestTheQueryTableIsFittedToTheTerminal(t *testing.T) {
	const width = 100
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), width)

	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		n := utf8.RuneCountInString(line)
		if n <= width {
			continue
		}
		if strings.HasPrefix(line, longQueryID()) {
			// The id is 161 characters and the terminal is 100. Nothing can
			// make that fit, and cutting the id is the one repair that costs
			// more than the overrun does.
			continue
		}
		t.Errorf("a line is %d columns wide at a %d-column terminal and its "+
			"id is not what made it so:\n%s", n, width, line)
	}
}

// ⛔ AN ID IS NEVER ELIDED, AND THIS IS THE CLAUSE THAT DIFFERS FROM THE
// BRIEF'S TABLE. briefFit shrinks the WIDEST column first, and here the widest
// column is the id - so the brief's rule applied unchanged would cut every
// doc-key id to about forty characters. A cut id cannot be handed back to
// `rig record get`, which is the only reason the column is printed.
func TestAnIdIsNeverCutToFitTheTerminal(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), 60)

	if !strings.Contains(got, longQueryID()) {
		t.Fatalf("the 161-character id was cut to fit a 60-column terminal, "+
			"so it can no longer be fetched:\n%s", got)
	}
}

// ⛔ A PIPE HAS NO WIDTH TO FIT. brief.go states the rule at briefStyle.Width
// - "discarding bytes it was going to read in full is destruction rather than
// legibility" - and a redirected `rig record query` is exactly that reader.
func TestAPipeGetsTheWholeTableUncut(t *testing.T) {
	rs := queryRows()
	got := queryText(QueryArgs{Project: "rig"}, rs, 0)

	if !strings.Contains(got, longQueryID()) {
		t.Error("the id was cut at a pipe, where there is no width to fit")
	}
	if !strings.Contains(got, recordSummary(rs[0])) {
		t.Errorf("the summary was cut at a pipe:\n%s", got)
	}
}

// ⛔ A LONG ID'S ROW CONTINUES IN THE SAME COLUMNS, which is the only thing
// that keeps the table scannable once some rows are two lines and some are
// one. A continuation starting at the left margin reads as a second record.
func TestALongIdsRowContinuesUnderTheSameColumns(t *testing.T) {
	const width = 100
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), width)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	at := strings.Index(lines[0], "VERSION")
	if at < 0 {
		t.Fatalf("no VERSION column in the header: %q", lines[0])
	}
	for i, line := range lines {
		if line != longQueryID() {
			continue
		}
		if i+1 >= len(lines) {
			t.Fatalf("the id is the last line, so its row was never "+
				"rendered:\n%s", got)
		}
		cont := lines[i+1]
		if len(cont) <= at || !strings.HasPrefix(cont[at:], "v1") {
			t.Fatalf("the continuation does not carry VERSION at column %d, "+
				"where the header puts it:\nheader: %q\ncont:   %q",
				at, lines[0], cont)
		}
		return
	}
	t.Fatalf("the long id never got a line of its own:\n%s", got)
}

// The header moves with the columns or it labels the wrong ones.
func TestTheFittedHeaderSitsOverTheColumnsItNames(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig"}, queryRows(), 100)
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("too few lines to hold a header and two rows:\n%s", got)
	}
	header := lines[0]
	at := strings.Index(header, "VERSION")
	if at < 0 {
		t.Fatalf("no VERSION column in the header: %q", header)
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "B75") {
			continue
		}
		if !strings.HasPrefix(line[at:], "v2") {
			t.Fatalf("VERSION starts at column %d in the header and the B75 "+
				"row carries %q there, so the header labels a different "+
				"column than the rows fill:\nheader: %q\nrow:    %q",
				at, line[at:min(at+4, len(line))], header, line)
		}
		return
	}
	t.Fatalf("no B75 row to align against:\n%s", got)
}
