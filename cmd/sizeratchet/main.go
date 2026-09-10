// Command sizeratchet records and enforces the binary-size ratchet.
//
// PLAN.md section 17 wants a ratchet rather than a budget, because a budget is
// a number somebody argues with once a year and a ratchet is a number that
// fails the build the day it moves. Section 22 splits it per binary: rigd links
// none of the terminal stack, rig links no daemon internals, and the point of
// two binaries is that each pays only for what it uses.
//
// What this does NOT do yet, stated so it is not mistaken for done: section 22
// also requires bench-size to "attribute every dependency's contribution to
// each" - specifically to split the +8.89 MiB rung that currently bundles
// bubbletea, huh, glamour, lipgloss and go-keyring together. Until that split
// exists, section 2's recovery claim is a design target rather than a
// measurement, which section 22 says plainly. That work is owed by M11 and is
// not in this tool.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type entry struct {
	Bytes    int64  `json:"bytes"`
	Recorded string `json:"recorded"`
}

type ratchet struct {
	// Note is written into the file so whoever opens it knows what moves it.
	Note string           `json:"note"`
	Bins map[string]entry `json:"bins"`
}

// row is one binary's measurement against its recorded number.
type row struct {
	name           string
	now, was, diff int64
	known, over    bool
}

type binList []string

func (b *binList) String() string     { return fmt.Sprint(*b) }
func (b *binList) Set(v string) error { *b = append(*b, v); return nil }

func main() {
	var bins binList
	path := flag.String("ratchet", "size-ratchet.json", "the ratchet file")
	update := flag.Bool("update", false, "accept the current sizes as the new ratchet")
	flag.Var(&bins, "bin", "a binary to measure (repeatable)")
	flag.Parse()

	if len(bins) == 0 {
		fmt.Fprintln(os.Stderr, "sizeratchet: no --bin given")
		os.Exit(2)
	}
	if err := run(*path, *update, bins); err != nil {
		fmt.Fprintln(os.Stderr, "sizeratchet: "+err.Error())
		os.Exit(1)
	}
}

func run(path string, update bool, bins []string) error {
	r := ratchet{
		Note: "Binary sizes in bytes. A binary over its recorded number fails " +
			"the build (PLAN.md 17). Moving a number is deliberate: " +
			"make bench-size-update, in its own commit, with the reason.",
		Bins: map[string]entry{},
	}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		if r.Bins == nil {
			r.Bins = map[string]entry{}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	var rows []row
	var failed bool

	for _, p := range bins {
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("%s: %w (run make build first)", p, err)
		}
		name := filepath.Base(p)
		e, known := r.Bins[name]
		rw := row{name: name, now: fi.Size(), was: e.Bytes, known: known}
		if known {
			rw.diff = rw.now - rw.was
			rw.over = rw.now > rw.was
		}
		if rw.over && !update {
			failed = true
		}
		rows = append(rows, rw)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	stamp := time.Now().UTC().Format(time.RFC3339)
	fmt.Printf("%-12s %12s %12s %10s\n", "binary", "bytes", "ratchet", "delta")
	for _, rw := range rows {
		switch {
		case !rw.known:
			fmt.Printf("%-12s %12d %12s %10s  new\n", rw.name, rw.now, "-", "-")
		case rw.over && !update:
			fmt.Printf("%-12s %12d %12d %+10d  OVER\n", rw.name, rw.now, rw.was, rw.diff)
		default:
			fmt.Printf("%-12s %12d %12d %+10d\n", rw.name, rw.now, rw.was, rw.diff)
		}
	}

	if update || anyNew(rows) {
		for _, rw := range rows {
			// A number only ever moves on --update. Recording a new binary is
			// not the same as raising an existing one, so an unknown binary is
			// written on a plain run and a known one is not.
			if !rw.known {
				r.Bins[rw.name] = entry{Bytes: rw.now, Recorded: stamp}
				continue
			}
			if !update {
				continue
			}
			// A row whose bytes did not move keeps its timestamp. Re-stamping
			// it destroys the only record of when that number was actually
			// set, and turns a one-line raise into a diff nobody can read -
			// which matters most when several sessions share the file.
			if rw.now == rw.was {
				continue
			}
			r.Bins[rw.name] = entry{Bytes: rw.now, Recorded: stamp}
		}
		out, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nwrote %s\n", path)
	}

	if failed {
		return errors.New("a binary is over its ratchet. If the growth is " +
			"intended, run: make bench-size-update, in its own commit, saying why")
	}
	return nil
}

func anyNew(rows []row) bool {
	for _, r := range rows {
		if !r.known {
			return true
		}
	}
	return false
}
