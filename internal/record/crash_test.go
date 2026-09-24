package record

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// crashChildEnv turns this test binary into the writer that gets killed.
const crashChildEnv = "RIG_RECORD_CRASH_CHILD"

// wantCommits is how many commits the writer must report before it is killed.
// Enough that the kill lands well inside the work rather than before any of it,
// and small enough that a broken writer fails the bound quickly.
const wantCommits = 25

// ⛔ A KILLED WRITER MUST LEAVE NOTHING HALF-WRITTEN, AND THIS KILLS A REAL
// PROCESS TO FIND OUT.
//
// BORIS, 2026-09-16, ruling robustness onto the MVP itself rather than after
// it: "We must strive to a super-robust MVP implementation before we start
// using it. Terminations must be graceful and everything else that makes it
// very robust." A termination is graceful when the process chooses to exit,
// says why, and LEAVES NOTHING HALF-WRITTEN - and the third clause is a
// sentence about a database rather than about a window, so it is this
// package's.
//
// ⛔ SIGKILL RATHER THAN A FAKED ERROR, AND THE DIFFERENCE IS THE WHOLE POINT.
// A unit test that injects a failure between two statements proves the code
// handles the error it was handed. It cannot prove anything about a process
// that stops existing between the write-ahead log and the database file,
// because there is no code running at that moment to hand anything to. Section
// 37's bar is a demonstration, and a demonstration of crash safety has to
// contain a crash.
//
// WHAT IT ASSERTS, AND EACH ONE IS A DIFFERENT FAILURE:
//
//	the store REOPENS          - a killed writer left no lock nobody clears
//	every REPORTED id is there - a committed write survived the kill
//	every head has its row     - no torn state: a head pointing at a version
//	                             that does not exist is the half-written shape
//	a new write SUCCEEDS       - the store is usable, not merely readable
//
// ⛔ WHAT IT DOES NOT CLAIM, AND THE SECOND ONE WAS MEASURED RATHER THAN
// REASONED:
//
//  1. POWER LOSS. WAL with the default synchronous setting means a committed
//     transaction is in the operating system's hands, not on the platter, so
//     SIGKILL is survivable where pulling the plug may not be.
//
//  2. ⛔ THAT THE STORE IS CRASH-SAFE BY CONSTRUCTION. This test kills at an
//     arbitrary moment, so a pass says the run landed nowhere harmful - not
//     that there is nowhere harmful to land. MEASURED: mutating the DSN to
//     journal_mode(OFF) - with the change VERIFIED IN FORCE, PRAGMA
//     journal_mode reading "off" rather than "wal" - left this test GREEN on
//     three consecutive runs. An effective mutation that removes the whole
//     recovery mechanism and is not detected.
//
// So the deterministic half of the claim is asserted separately, below, and
// that is the one that bites. This test demonstrates that recovery WORKS; the
// test below pins the setting recovery DEPENDS ON. Neither is sufficient and
// the pair is honest, where either alone reads as more than it is.
func TestAKilledWriterLeavesNothingHalfWritten(t *testing.T) {
	if os.Getenv(crashChildEnv) != "" {
		t.Skip("this process is the writer being killed")
	}

	state := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestCrashWriterHelper", "-test.v")
	cmd.Env = append(os.Environ(), crashChildEnv+"=1", "XDG_STATE_HOME="+state)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the writer: %v", err)
	}

	// Read committed ids until the writer is well past its first one, so the
	// kill lands inside a transaction rather than before any work.
	type written struct {
		id      string
		version uint64
	}
	// ⛔ THE SLICE IS BEHIND A MUTEX AND THAT IS NOT CEREMONY. The reader
	// goroutine appends while this one waits on the count, which is a data
	// race - and `go test -race` is one of ci's seven, so an unguarded version
	// of this test turns the gate red rather than merely being wrong. Found by
	// the gate on this file's first run.
	var (
		mu        sync.Mutex
		committed []written
	)
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(committed) }

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			id, ver, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
			if !ok || !strings.HasPrefix(id, "wi-") {
				continue
			}
			v, err := strconv.ParseUint(ver, 10, 64)
			if err != nil {
				continue
			}
			mu.Lock()
			committed = append(committed, written{id, v})
			mu.Unlock()
		}
	}()

	// ⛔ A BOUND, BECAUSE A WRITER THAT NEVER STARTS LOOKS EXACTLY LIKE ONE
	// THAT FINISHED. Without it a broken child hangs the test, and a hang is
	// the one failure mode that reads as "still working".
	deadline := time.After(20 * time.Second)
	for count() < wantCommits {
		select {
		case <-deadline:
			_ = cmd.Process.Kill()
			t.Fatalf("the writer committed %d records in 20s; it never got going, "+
				"so nothing below would be measuring a crash", count())
		case <-time.After(10 * time.Millisecond):
		}
	}

	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("killing the writer: %v", err)
	}
	_ = cmd.Wait()
	<-done

	// The reader goroutine has finished, so the slice is this goroutine's
	// alone from here; the lock is kept anyway because a later edit that moves
	// one of these reads above the join would be silent otherwise.
	mu.Lock()
	defer mu.Unlock()
	if len(committed) < wantCommits {
		t.Fatalf("only %d committed ids were reported", len(committed))
	}
	// The LAST reported id may have been in flight on the pipe when the kill
	// landed, so the durability claim is made about everything before it.
	survived := committed[:len(committed)-1]
	t.Logf("the writer was killed after reporting %d commits; asserting over %d",
		len(committed), len(survived))

	// 1. THE STORE REOPENS.
	t.Setenv("XDG_STATE_HOME", state)
	s, err := Open("crash")
	if err != nil {
		t.Fatalf("reopening the store a killed writer left behind: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// 2. EVERY COMMIT THE WRITER REPORTED IS STILL THERE, AT ITS VERSION.
	for _, w := range survived {
		rec, err := s.Get(tctx, w.id)
		if err != nil {
			t.Fatalf("%s was reported committed and is not readable after the kill: %v", w.id, err)
		}
		if rec.Version != w.version {
			t.Errorf("%s reads at version %d, the writer reported %d", w.id, rec.Version, w.version)
		}
		if rec.Body == "" {
			t.Errorf("%s is present with an empty body: a partially written row", w.id)
		}
	}

	// 3. NO TORN STATE. Every head must point at a version that exists, which
	// is the shape a half-written transaction would leave: the pointer moved
	// and the row it names did not arrive.
	var orphans int
	if err := s.db.QueryRowContext(tctx, `
		SELECT COUNT(*) FROM heads h
		LEFT JOIN records r ON r.id = h.id AND r.version = h.version
		WHERE r.id IS NULL`).Scan(&orphans); err != nil {
		t.Fatalf("checking for heads with no row: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d head pointers name a version that does not exist: the kill "+
			"left a transaction half applied", orphans)
	}

	// 4. THE STORE IS USABLE, NOT MERELY READABLE. A write lock the dead
	// process never released would refuse here, and that is the "a lock nobody
	// clears" failure rather than the torn-data one.
	if _, err := s.Put(tctx, PutRequest{
		ID: "after-the-crash", Kind: "work-item", Project: "crash",
		Body:    "written by a process that was not there for the kill",
		Fields:  map[string]string{"title": "recovery", "status": "active"},
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing after recovery: %v", err)
	}

	// 5. AND A READ ACROSS THE WHOLE PROJECT STILL ANSWERS. A store that opens
	// and answers one row but cannot walk a project has recovered its bytes and
	// not its usefulness.
	//
	// ⛔ THIS WAS `s.Brief(tctx, "crash")` UNTIL plan/50 move 8, and the
	// substitution is not a weakening: the brief's own first act was this
	// query, and the derivation built on top of it is the docket program's
	// now. What rig owes a recovering session is that the store answers.
	if _, err := s.Query(tctx, "crash", ""); err != nil {
		t.Fatalf("a project-wide read after recovery: %v", err)
	}

	// 6. NO STRAY JOURNAL OR WAL LEFT UNRECOVERED. SQLite checkpoints and
	// removes the -wal on a clean close; what matters here is that reopening
	// RECOVERED rather than ignored, which the reads above already prove - this
	// names what is on disk so a later reader does not have to guess.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		p := filepath.Join(state, "rig", "estates", "crash", DBName+suffix)
		if fi, err := os.Stat(p); err == nil {
			t.Logf("on disk after recovery: %s (%d bytes)", filepath.Base(p), fi.Size())
		}
	}
}

// TestCrashWriterHelper IS NOT A TEST. It is the writer the test above kills.
//
// It reports each commit on stdout only AFTER Put returns, so the parent's
// durability assertion is about writes SQLite said were committed - not about
// writes that were merely attempted. Getting that backwards would turn this
// into a test that data survives which was never written.
func TestCrashWriterHelper(t *testing.T) {
	if os.Getenv(crashChildEnv) == "" {
		t.Skip("not the writer; run by TestAKilledWriterLeavesNothingHalfWritten")
	}
	s, err := Open("crash")
	if err != nil {
		fmt.Fprintf(os.Stderr, "writer: opening: %v\n", err)
		os.Exit(3)
	}
	// No Close and no cleanup, deliberately: this process is going to be
	// killed, and a writer that tidied up would be demonstrating the happy
	// path the parent is not asking about.
	body := strings.Repeat("a work item body, long enough that the write is not free. ", 200)
	for i := 0; ; i++ {
		id := fmt.Sprintf("wi-%05d", i)
		rec, err := s.Put(tctx, PutRequest{
			ID: id, Kind: "work-item", Project: "crash", Body: body,
			Fields:  map[string]string{"title": id, "status": "active"},
			Session: "writer", Seat: "backend-record",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "writer: put %s: %v\n", id, err)
			os.Exit(4)
		}
		fmt.Printf("%s %d\n", rec.ID, rec.Version)
	}
}

// ⛔ THE SETTING THE CRASH TEST DEPENDS ON, ASSERTED DETERMINISTICALLY.
//
// It exists because a mutation survived the crash test above. Turning
// journal_mode off removes the mechanism that makes an interrupted transaction
// recoverable, and the killed-writer test did not notice across three runs -
// because that test can only observe the moment it happened to kill, and the
// vulnerable window is small. A probabilistic test cannot be made to bite
// reliably by running it more; it can be PAIRED with a check on the property
// it rests on, which either holds or does not.
//
// Section 39 states all three of these as REQUIREMENTS rather than tunings and
// store.go argues each one at length. Nothing until now checked that the
// argument and the connection string agreed.
func TestTheStoreOpensWithTheSettingsSectionThirtyNineRequires(t *testing.T) {
	s := openStore(t, estate(t, "settings"))
	for _, tc := range []struct {
		pragma, want, why string
	}{
		{"journal_mode", "wal", "section 39's \"a reader is never blocked behind a writer\", and the mechanism a killed writer recovers through"},
		{"busy_timeout", "5000", "a concurrent writer WAITS rather than returning SQLITE_BUSY, which a caller cannot tell from a version conflict"},
	} {
		t.Run(tc.pragma, func(t *testing.T) {
			var got string
			if err := s.db.QueryRowContext(tctx, "PRAGMA "+tc.pragma).Scan(&got); err != nil {
				t.Fatalf("reading %s: %v", tc.pragma, err)
			}
			if !strings.EqualFold(got, tc.want) {
				t.Fatalf("PRAGMA %s is %q, want %q - %s", tc.pragma, got, tc.want, tc.why)
			}
		})
	}
}
