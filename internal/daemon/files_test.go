package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/48 slice 4's done-bar: a file a program writes where rig told it
// appears in a rig commit within one interval.
func TestAProgramsFileIsCommittedWithinAnInterval(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // off the developer's gitconfig
	sock, root, _ := upRootedDaemon(t, time.Second)
	ctx := ctx5(t)
	graft := program(t, sock, "graft")

	var own verbsv1.FilesRootResponse
	if err := graft.Call(ctx, "rig.files.root", &verbsv1.FilesRootRequest{}, &own); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "files", "programs", "graft")
	if own.GetPath() != want || own.GetProgram() != "graft" || own.GetCommitEveryS() != 1 {
		t.Fatalf("graft's root: %v, want %s every 1s", &own, want)
	}
	if err := os.WriteFile(filepath.Join(own.GetPath(), "plan.md"), []byte("# plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own.GetPath(), "blob.bin"), []byte{0, 1, 2}, 0o600); err != nil {
		t.Fatal(err)
	}
	files := filepath.Join(root, "files")
	deadline := time.Now().Add(4 * time.Second)
	for {
		out, _ := exec.Command("git", "-C", files, "ls-files").Output()
		if strings.Contains(string(out), "programs/graft/plan.md") {
			if strings.Contains(string(out), "blob.bin") {
				t.Fatalf("a binary was committed:\n%s", out)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("plan.md was not committed within the interval; tracked:\n%s", out)
		}
		time.Sleep(100 * time.Millisecond)
	}
	author, _ := exec.Command("git", "-C", files, "log", "-1", "--format=%an").Output()
	if strings.TrimSpace(string(author)) != "rig" {
		t.Fatalf("committed by %q", author)
	}
}

// The store verbs' rule for whose directory, plus the shared area for all.
func TestFilesRootFollowsTheStoreRule(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sock, root, d := upStoreDaemon(t)
	ctx := ctx5(t)
	shelf := program(t, sock, "shelf")
	var r verbsv1.FilesRootResponse
	wantCode(t, shelf.Call(ctx, "rig.files.root", &verbsv1.FilesRootRequest{Program: "graft"}, &r),
		rigv1.Code_CODE_DENIED, "shelf asking for graft's directory")
	if err := shelf.Call(ctx, "rig.files.root", &verbsv1.FilesRootRequest{Shared: true}, &r); err != nil ||
		r.GetPath() != filepath.Join(root, "files") || r.GetProgram() != "" {
		t.Fatalf("the shared area: %v, %v", &r, err)
	}

	term := dial(t, sock)
	wantCode(t, term.Call(ctx, "rig.files.root", &verbsv1.FilesRootRequest{}, &r),
		rigv1.Code_CODE_INVALID, "a terminal naming no program")
	if err := term.Call(ctx, "rig.files.root", &verbsv1.FilesRootRequest{Program: "grfat"}, &r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.GetPath()); !os.IsNotExist(err) {
		t.Fatalf("a terminal's question made a directory: %v", err)
	}
	wantCode(t, term.Call(ctx, "rig.files.root", &verbsv1.FilesRootRequest{Program: "../x"}, &r),
		rigv1.Code_CODE_INVALID, "a program name that is a path")

	agent := mailAgent(t, d, "files-agent")
	ans := resultOf(t, callTool(ctx, t, agent, "files_root", map[string]any{"shared": true}))
	if ans["path"] != filepath.Join(root, "files") {
		t.Fatalf("the agent's shared area: %v", ans)
	}
}

func TestNoRootMeansNoFiles(t *testing.T) {
	c := dial(t, up(t))
	wantCode(t, c.Call(ctx5(t), "rig.files.root", &verbsv1.FilesRootRequest{Shared: true},
		&verbsv1.FilesRootResponse{}), rigv1.Code_CODE_UNAVAILABLE, "files.root with no root")
}
