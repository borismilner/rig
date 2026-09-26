package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// Two daemons on separate state homes both win the estate claim, so the
// storage root carries its own: the second is refused, names the fix, and
// exits with the status systemd does not retry.
func TestTwoDaemonsCannotShareOneStorageRoot(t *testing.T) {
	root := t.TempDir()
	first, err := claimRoot("production", root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = claimRoot("production", root)
	var held *RootHeldError
	if !errors.As(err, &held) || held.Incumbent != os.Getpid() {
		t.Fatalf("a second daemon on the same root: %v", err)
	}
	if !strings.Contains(err.Error(), "--root") || exitStatus(err) != exitAlreadyRunning {
		t.Fatalf("the refusal must name the fix and not be retried: %q, exit %d", err, exitStatus(err))
	}
	// Another estate under the same root is its own subtree.
	other, err := claimRoot("development", root)
	if err != nil {
		t.Fatalf("a second estate under one root: %v", err)
	}
	_ = other.Close()
	// Released, the root is free again.
	_ = first.Close()
	again, err := claimRoot("production", root)
	if err != nil {
		t.Fatalf("after the first daemon went: %v", err)
	}
	_ = again.Close()
}
