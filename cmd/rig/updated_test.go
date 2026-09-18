package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ⛔ "ONCE, NOT EVERY RUN" IS THE CLAUSE WITH TEETH, so the four cases are one
// SEQUENCE against one marker rather than four independent calls: a notice
// that fires on the second run is the defect, and only a sequence can see it.
// PLAN.md section 28.
func TestTheUpdateNoticeFiresOnceAndOnlyWhenTheVersionMoved(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "state", "last-announced-version")

	var said []string
	notice := func(v string) updateNotice {
		return updateNotice{
			version: v,
			marker:  marker,
			notify:  func(_ context.Context, _, body string) { said = append(said, body) },
		}
	}
	ctx := context.Background()

	// 1. The first run on a machine that has never run rig is NOT an update:
	// there is nothing it moved from.
	notice("v1").announce(ctx)
	if len(said) != 0 {
		t.Errorf("the first run ever announced %v; it has no previous version to name", said)
	}
	if got := read(t, marker); got != "v1\n" {
		t.Fatalf("marker = %q; the first run must still record what is running, or every "+
			"run after it is a first run", got)
	}

	// 2. The same version again says nothing.
	notice("v1").announce(ctx)
	if len(said) != 0 {
		t.Errorf("an unchanged version announced %v", said)
	}

	// 3. A redeployment announces, and names both ends.
	notice("v2").announce(ctx)
	if len(said) != 1 {
		t.Fatalf("a changed version announced %d times; want 1", len(said))
	}
	if said[0] != "v1  →  v2" {
		t.Errorf("body = %q; it must name what it moved FROM and TO", said[0])
	}

	// 4. ⛔ AND THE RUN AFTER IT IS SILENT. This is the clause.
	notice("v2").announce(ctx)
	if len(said) != 1 {
		t.Errorf("the notice fired again on the next run: %v", said)
	}
}

// ⛔ A NOTICE MUST NOT BE ABLE TO FAIL A COMMAND. An unwritable marker is the
// cheapest way to make every step of announce fail at once.
func TestAnUnwritableMarkerIsSilentRatherThanFatal(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file", "marker")
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fired := false
	updateNotice{
		version: "v2",
		marker:  blocked,
		notify:  func(_ context.Context, _, _ string) { fired = true },
	}.announce(context.Background())
	if fired {
		t.Error("it announced an update it could not record, so it would announce it again")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the marker: %v", err)
	}
	return string(b)
}
