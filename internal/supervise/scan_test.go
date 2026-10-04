package supervise

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
}

// Everything executable in a scan directory is a program named by its file;
// the rest is not, and the first directory wins an id found twice.
func TestScanFindsExecutablesFirstDirectoryFirst(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(a, "beacon"), 0o700)
	writeFile(t, filepath.Join(a, "notes.txt"), 0o600)
	writeFile(t, filepath.Join(a, ".hidden"), 0o700)
	writeFile(t, filepath.Join(b, "beacon"), 0o700)
	writeFile(t, filepath.Join(b, "shelf"), 0o700)
	if err := os.Mkdir(filepath.Join(b, "adir"), 0o700); err != nil {
		t.Fatal(err)
	}

	found, problems := Scan([]string{a, b, filepath.Join(a, "missing")})
	if len(found) != 2 || found["beacon"] != filepath.Join(a, "beacon") || found["shelf"] == "" {
		t.Fatalf("found %v", found)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "shadowed") {
		t.Fatalf("problems %v, want the shadowed beacon alone", problems)
	}
}

func TestScanDirsResolvesHomeAndRefusesRelative(t *testing.T) {
	dirs, err := ScanDirs("~/.local/lib/rig/apps::/opt/rig", "/home/u")
	if err != nil || len(dirs) != 2 || dirs[0] != "/home/u/.local/lib/rig/apps" || dirs[1] != "/opt/rig" {
		t.Fatalf("dirs %v, err %v", dirs, err)
	}
	for _, bad := range []string{"apps", "/opt/../etc"} {
		if _, err := ScanDirs(bad, "/home/u"); err == nil {
			t.Fatalf("%q was accepted as a scan directory", bad)
		}
	}
}

// A scanned program takes its load mode from its binary unless a row of
// programs.json sets one; a path-less row overrides the scanned program,
// and one with nothing to override is reported.
func TestMergeAppliesOverridesAndLeavesTheModeToTheBinary(t *testing.T) {
	found := map[string]string{"beacon": "/s/beacon", "shelf": "/s/shelf", "own": "/s/own"}
	rows := []Spec{
		{ID: "shelf", Args: []string{"-v"}, Autostart: true},
		{ID: "gone", OnCall: true},
		{ID: "own", Path: "/elsewhere/own"},
	}
	specs, problems := Merge(rows, found)
	by := map[string]Spec{}
	for _, s := range specs {
		by[s.ID] = s
	}
	switch {
	case !by["beacon"].FromBinary || !by["beacon"].OnCall || !by["beacon"].Scanned:
		t.Fatalf("beacon %+v: a scanned program with no row is held on call until read", by["beacon"])
	case by["shelf"].FromBinary || !by["shelf"].Autostart || by["shelf"].Path != "/s/shelf" || len(by["shelf"].Args) != 1:
		t.Fatalf("shelf %+v: the override's mode and args win, the path is the scan's", by["shelf"])
	case by["own"].Path != "/elsewhere/own" || by["own"].Scanned:
		t.Fatalf("own %+v: a row with a path is declared as written", by["own"])
	case len(problems) != 1 || !strings.Contains(problems[0].Error(), "gone"):
		t.Fatalf("problems %v, want the override of gone", problems)
	}
}

// SetLoad turns a program held on call resident, and Forget takes it off
// the table, failing a call waiting on its start.
func TestSetLoadAndForget(t *testing.T) {
	h := resting(t)
	if err := h.sup.SetLoad("beacon", false); err != nil {
		t.Fatal(err)
	}
	if spec, _ := h.sup.Spec("beacon"); spec.OnCall || !spec.Autostart {
		t.Fatalf("SetLoad resident left %+v", spec)
	}
	h.sup.Forget("beacon")
	if _, ok := h.sup.Spec("beacon"); ok || len(h.sup.Declared()) != 0 {
		t.Fatalf("Forget left beacon declared: %v", h.sup.Declared())
	}
}

// A directory listed but not entered is unread, not empty: its programs are
// a problem marked unreadable, never missing. The race this pins is a scan
// that listed a directory a moment before it lost its permissions. The red
// control is the same directory readable, where the program is found.
func TestABinaryThatCannotBeStatedIsUnreadableNotGone(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root enters a directory whatever its mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stub"), []byte("v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if found, _ := Scan([]string{dir}); found["stub"] == "" {
		t.Fatal("a readable directory's program was not found")
	}
	if err := os.Chmod(dir, 0o444); err != nil { // listable, not enterable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	found, problems := Scan([]string{dir})
	unread := false
	for _, p := range problems {
		unread = unread || errors.Is(p, ErrScanUnreadable)
	}
	if found["stub"] != "" || !unread {
		t.Fatalf("found %v with problems %v, want nothing found and the directory unreadable", found, problems)
	}
}
