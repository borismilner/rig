package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig config` - section 6's settings, resolved by rigd (plan/47).
//
// ⛔ THE CLIENT PARSES NO SETTINGS FILE AND LINKS NO CONFIG LIBRARY (plan/47
// decision 15). It asks rigd. The one file it reads is an export it was
// handed for `diff`, which is rigd's own document: one `key = <JSON>` line
// per key, so reading it needs encoding/json and nothing else.

const configUsage = "usage: rig config get [<prefix>] | origin <key> | set <key>=<value>... | export | diff <file>"

// exitCodeError carries a status other than 1 out to main: `config diff` answers
// 0 same, 1 differs, 2 unreadable.
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }

func cmdConfig(args []string) (err error) {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	flags, pos := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(pos) == 0 {
		return badArgumentf(configUsage)
	}
	sub, rest := pos[0], pos[1:]
	switch {
	case sub == storeGet && len(rest) <= 1, sub == "export" && len(rest) == 0:
		prefix := ""
		if len(rest) == 1 {
			prefix = rest[0]
		}
		resp, err := configGet(prefix)
		if err != nil {
			return err
		}
		if sub == "export" {
			return printExport(resp, *asJSON)
		}
		return printConfig(resp, *asJSON, false)
	case sub == "origin" && len(rest) == 1:
		resp, err := configGet(rest[0])
		if err != nil {
			return err
		}
		return printConfig(resp, *asJSON, true)
	case sub == "set" && len(rest) > 0:
		return configSet(rest, *asJSON)
	case sub == "diff" && len(rest) == 1:
		return configDiff(rest[0], *asJSON)
	}
	return badArgumentf(configUsage)
}

func configGet(prefix string) (*registryv1.ConfigGetResponse, error) {
	c, err := connect()
	if err != nil {
		return nil, noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	var resp registryv1.ConfigGetResponse
	if err := call(ctx, c, "rig.config.get", &registryv1.ConfigGetRequest{Prefix: prefix}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func layerWhere(l *registryv1.ConfigLayerValue) string {
	if l.GetFile() != "" {
		return l.GetLayer() + " " + l.GetFile()
	}
	return l.GetLayer()
}

func layerJSON(l *registryv1.ConfigLayerValue) map[string]any {
	m := map[string]any{"layer": l.GetLayer(), "value": json.RawMessage(l.GetValueJson())}
	if l.GetFile() != "" {
		m["file"] = l.GetFile()
	}
	return m
}

func printConfig(resp *registryv1.ConfigGetResponse, asJSON, losers bool) error {
	if asJSON {
		vals := make([]map[string]any, 0, len(resp.GetValues()))
		for _, v := range resp.GetValues() {
			m := map[string]any{"key": v.GetKey(), "apply": v.GetApply(), "winner": layerJSON(v.GetWinner())}
			ls := make([]map[string]any, 0, len(v.GetLosers()))
			for _, l := range v.GetLosers() {
				ls = append(ls, layerJSON(l))
			}
			m["losers"] = ls
			vals = append(vals, m)
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"values": vals, "orphans": nonNil(resp.GetOrphans()), "problems": nonNil(resp.GetProblems()),
			"snapshot": resp.GetSnapshotPath(),
		})
	}
	for _, v := range resp.GetValues() {
		fmt.Printf("%s = %s  (%s)\n", v.GetKey(), v.GetWinner().GetValueJson(), layerWhere(v.GetWinner()))
		if !losers {
			continue
		}
		ls := v.GetLosers()
		if len(ls) == 0 {
			fmt.Println("  no layer below it set this key")
		}
		for i := len(ls) - 1; i >= 0; i-- {
			fmt.Printf("  beat %s  (%s)\n", ls[i].GetValueJson(), layerWhere(ls[i]))
		}
		fmt.Printf("  a change applies %s\n", map[string]string{"live": "live", "restart": "at restart"}[v.GetApply()])
	}
	for _, o := range resp.GetOrphans() {
		fmt.Println("orphan, declared nowhere: " + o)
	}
	for _, p := range resp.GetProblems() {
		fmt.Println("problem, took no part: " + p)
	}
	return nil
}

func configSet(pairs []string, asJSON bool) error {
	values := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return badArgumentf("%q is not <key>=<value>; %s", p, configUsage)
		}
		// Sent as a JSON string; rigd reads it as the key's own type.
		b, _ := json.Marshal(v)
		values[k] = string(b)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	var resp registryv1.ConfigSetResponse
	if err := call(ctx, c, "rig.config.set", &registryv1.ConfigSetRequest{ValuesJson: values}, &resp); err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"outcome": resp.GetOutcome(), "snapshot": resp.GetSnapshotPath()})
	}
	keys := make([]string, 0, len(resp.GetOutcome()))
	for k := range resp.GetOutcome() {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Printf("%s: %s\n", k, resp.GetOutcome()[k])
	}
	fmt.Println("until rigd restarts; to keep it, put it in ~/.config/rig/rig.toml")
	return nil
}

// printExport prints the resolved document, the same text rigd writes as its
// snapshot, which is what `diff` reads back.
func printExport(resp *registryv1.ConfigGetResponse, asJSON bool) error {
	if asJSON {
		m := map[string]json.RawMessage{}
		for _, v := range resp.GetValues() {
			m[v.GetKey()] = json.RawMessage(v.GetWinner().GetValueJson())
		}
		return json.NewEncoder(os.Stdout).Encode(m)
	}
	fmt.Println("# rig's resolved configuration, exported by rig config export.")
	for _, v := range resp.GetValues() {
		fmt.Printf("\n# %s from %s\n%s = %s\n", v.GetKey(), layerWhere(v.GetWinner()), v.GetKey(), v.GetWinner().GetValueJson())
	}
	return nil
}

// readExport reads `key = <JSON>` lines, skipping comments and blanks.
func readExport(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, " = ")
		if !ok || !json.Valid([]byte(v)) {
			return nil, fmt.Errorf("%s:%d is not a line of `rig config export`", path, n)
		}
		out[k] = compactJSON(v)
	}
	return out, sc.Err()
}

func compactJSON(s string) string {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func configDiff(path string, asJSON bool) error {
	saved, err := readExport(path)
	if err != nil {
		return &exitCodeError{2, err}
	}
	resp, err := configGet("")
	if err != nil {
		return &exitCodeError{2, err}
	}
	type change struct {
		Key   string `json:"key"`
		Saved string `json:"saved,omitempty"`
		Live  string `json:"live,omitempty"`
	}
	var diffs []change
	live := map[string]bool{}
	for _, v := range resp.GetValues() {
		live[v.GetKey()] = true
		now := compactJSON(v.GetWinner().GetValueJson())
		if was, ok := saved[v.GetKey()]; !ok || was != now {
			diffs = append(diffs, change{v.GetKey(), was, now})
		}
	}
	for k, was := range saved {
		if !live[k] {
			diffs = append(diffs, change{Key: k, Saved: was})
		}
	}
	slices.SortFunc(diffs, func(a, b change) int { return strings.Compare(a.Key, b.Key) })
	if asJSON {
		if diffs == nil {
			diffs = []change{}
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"differs": diffs}); err != nil {
			return err
		}
	} else {
		for _, d := range diffs {
			fmt.Printf("%s: saved %s, live %s\n", d.Key, orNone(d.Saved), orNone(d.Live))
		}
	}
	if len(diffs) > 0 {
		return &exitCodeError{1, fmt.Errorf("%d setting(s) differ from %s", len(diffs), path)}
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// displayName is this user's display.name, read from rigd once per process.
// Unreachable or unset is empty, which renders the seat itself: the path
// every machine without the setting takes. A var so a test can say what
// the daemon would answer.
var displayName = sync.OnceValue(func() string {
	resp, err := configGet("display.name")
	if err != nil || len(resp.GetValues()) == 0 {
		return ""
	}
	var name string
	if json.Unmarshal([]byte(resp.GetValues()[0].GetWinner().GetValueJson()), &name) != nil {
		return ""
	}
	return name
})

// errorCode is the exit status err asks for: 1 unless it carries another.
func errorCode(err error) int {
	if ec, ok := errors.AsType[*exitCodeError](err); ok {
		return ec.code
	}
	return 1
}
