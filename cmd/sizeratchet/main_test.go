package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bin writes a file of exactly n bytes and returns its path.
func bin(t *testing.T, dir, name string, n int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) map[string]entry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Bins map[string]entry `json:"bins"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r.Bins
}

func TestANewBinaryIsRecordedAndAKnownOneIsNot(t *testing.T) {
	dir := t.TempDir()
	ratchet := filepath.Join(dir, "size-ratchet.json")
	a := bin(t, dir, "alpha", 100)

	// First run records it, because an unknown binary has no number to move.
	if err := run(ratchet, false, []string{a}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	bins := read(t, ratchet)
	if bins["alpha"].Bytes != 100 {
		t.Fatalf("alpha recorded as %d", bins["alpha"].Bytes)
	}

	// Growing it without --update fails, and does not move the number.
	a = bin(t, dir, "alpha", 200)
	if err := run(ratchet, false, []string{a}); err == nil {
		t.Fatal("a binary over its ratchet did not fail the build")
	}
	if got := read(t, ratchet)["alpha"].Bytes; got != 100 {
		t.Fatalf("a failed run moved the number to %d", got)
	}
}

func TestAnUnchangedRowKeepsItsTimestampAcrossAnUpdate(t *testing.T) {
	dir := t.TempDir()
	ratchet := filepath.Join(dir, "size-ratchet.json")
	a, b := bin(t, dir, "alpha", 100), bin(t, dir, "beta", 100)

	if err := run(ratchet, false, []string{a, b}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := read(t, ratchet)

	// Only alpha grows, and only alpha's row may move. Re-stamping beta
	// destroys the record of when its number was set, and turns a one-line
	// raise into a diff nobody can read - which is what matters when several
	// sessions share the file.
	a = bin(t, dir, "alpha", 300)
	if err := run(ratchet, true, []string{a, b}); err != nil {
		t.Fatalf("update: %v", err)
	}
	after := read(t, ratchet)

	if after["alpha"].Bytes != 300 {
		t.Fatalf("alpha did not move: %d", after["alpha"].Bytes)
	}
	if after["alpha"].Recorded == "" {
		t.Fatal("alpha has no timestamp")
	}
	if after["beta"].Recorded != before["beta"].Recorded {
		t.Fatalf("beta was re-stamped from %q to %q",
			before["beta"].Recorded, after["beta"].Recorded)
	}
	if after["beta"].Bytes != 100 {
		t.Fatalf("beta's number changed to %d", after["beta"].Bytes)
	}
}

func TestAMissingBinaryIsReportedRatherThanSkipped(t *testing.T) {
	dir := t.TempDir()
	ratchet := filepath.Join(dir, "size-ratchet.json")
	err := run(ratchet, false, []string{filepath.Join(dir, "nosuch")})
	if err == nil {
		t.Fatal("a missing binary passed the gate")
	}
	if !strings.Contains(err.Error(), "nosuch") {
		t.Fatalf("the error does not name it: %v", err)
	}
}
