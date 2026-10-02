package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig logs` - section 49's merged view of the estate's log store: rigd and
// every program, time-ordered by each record's own clock, with every gap the
// store knows of drawn as a band rather than left out.

const logsUsage = "usage: rig logs [--since D|T] [--until D|T] [--client X]... [--level L] [--grep RE] [--limit N] [-f] [--json]\n" +
	"       rig logs coverage [--since D|T] [--until D|T] [--json]\n" +
	"       rig logs pin|unpin SEGMENT [--json]"

// maxLogsFollowWaitMs is one parked call of -f; rigd's own bound is 60 s.
const maxLogsFollowWaitMs = 30_000

// maxLogsLimit is rigd's own bound on --limit.
const maxLogsLimit = 10_000

// followBacklog is how much -f shows before it starts following, as tail
// shows its last lines.
const followBacklog = 100

type logsFlags struct {
	fs                  *flag.FlagSet
	since, until, level *string
	grep                *string
	clients             listFlag
	limit               *uint
	follow, asJSON      *bool
}

func logsFlagSet() *logsFlags {
	l := &logsFlags{fs: flag.NewFlagSet("logs", flag.ContinueOnError)}
	l.since = l.fs.String("since", "", "from this long ago (1h, 30m) or this time (RFC 3339)")
	l.until = l.fs.String("until", "", "up to this long ago or this time")
	l.level = l.fs.String("level", "info", "the least level shown: debug, info, warn, error")
	l.grep = l.fs.String("grep", "", "an RE2 expression over the message and its attrs")
	l.fs.Var(&l.clients, "client", "only this client, rigd or a program's id; repeat for more")
	l.limit = l.fs.Uint("limit", 0, "at most this many records, the newest (default 1000)")
	l.follow = l.fs.Bool("f", false, "keep printing records as they come")
	l.asJSON = l.fs.Bool("json", false, "emit JSON")
	return l
}

var logLevels = map[string]int32{"debug": -4, "info": 0, "warn": 4, "error": 8}

func cmdLogs(args []string) (err error) {
	l := logsFlagSet()
	flags, positional := partition(args)
	if err := l.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *l.asJSON) }()
	if len(positional) > 0 {
		return logsSub(l, positional)
	}
	req, err := l.request(time.Now())
	if err != nil {
		return err
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()

	out := json.NewEncoder(os.Stdout)
	shown := map[string]bool{} // gaps already printed while following
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.GetTimeoutMs())*time.Millisecond+defaultCallTimeout)
		var resp registryv1.LogsQueryResponse
		err := call(ctx, c, "rig.logs.query", req, &resp)
		cancel()
		if err != nil {
			return err
		}
		switch {
		case !*l.follow && *l.asJSON:
			return out.Encode(logsJSON(&resp))
		case !*l.follow:
			printLogs(&resp, shown)
			return nil
		case *l.asJSON:
			for _, g := range resp.GetGaps() {
				if k := gapKey(g); !shown[k] {
					shown[k] = true
					_ = out.Encode(map[string]any{"gap": gapJSON(g)})
				}
			}
			for _, r := range resp.GetRecords() {
				_ = out.Encode(logRecordJSON(r))
			}
		default:
			printLogs(&resp, shown)
		}
		req.After, req.Limit = resp.GetLatest(), 0
		req.TimeoutMs = maxLogsFollowWaitMs
	}
}

func (l *logsFlags) request(now time.Time) (*registryv1.LogsQueryRequest, error) {
	req := &registryv1.LogsQueryRequest{Clients: l.clients.v, Grep: *l.grep, Limit: uint32(min(*l.limit, maxLogsLimit))}
	lv, ok := logLevels[*l.level]
	if !ok {
		return nil, badArgumentf("rig logs: --level is debug, info, warn or error, not %q", *l.level)
	}
	req.MinLevel = lv
	var err error
	if req.SinceUnixNanos, err = logsTime(*l.since, now); err != nil {
		return nil, badArgumentf("rig logs: --since %s", err)
	}
	if req.UntilUnixNanos, err = logsTime(*l.until, now); err != nil {
		return nil, badArgumentf("rig logs: --until %s", err)
	}
	if *l.follow && req.GetLimit() == 0 {
		req.Limit = followBacklog
	}
	return req, nil
}

// logsTime reads a duration ago or an RFC 3339 time; "" is open.
func logsTime(v string, now time.Time) (int64, error) {
	if v == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(v); err == nil && d >= 0 {
		return now.Add(-d).UnixNano(), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return 0, fmt.Errorf("is a duration ago (1h, 30m) or an RFC 3339 time, not %q", v)
	}
	return t.UnixNano(), nil
}

func levelName(l int32) string {
	switch {
	case l >= 8:
		return "ERROR"
	case l >= 4:
		return "WARN"
	case l >= 0:
		return "INFO"
	}
	return "DEBUG"
}

const logsClock = "2006-01-02 15:04:05.000"

// printLogs prints records and gap bands in one time order. A gap is drawn
// where it starts, so the reader sees the hole where it is.
func printLogs(resp *registryv1.LogsQueryResponse, shown map[string]bool) {
	var gaps []*registryv1.GapBand
	for _, g := range resp.GetGaps() {
		if k := gapKey(g); !shown[k] {
			shown[k] = true
			gaps = append(gaps, g)
		}
	}
	for _, r := range resp.GetRecords() {
		for len(gaps) > 0 && gaps[0].GetFromUnixNanos() <= r.GetUnixNanos() {
			printGap(gaps[0])
			gaps = gaps[1:]
		}
		printRecord(r)
	}
	for _, g := range gaps {
		printGap(g)
	}
	if resp.GetTruncated() {
		fmt.Printf("(the %d newest are shown and older ones were left out: narrow with --since or raise --limit)\n",
			len(resp.GetRecords()))
	}
}

func printRecord(r *registryv1.LogRecord) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %-8s %-5s %s", time.Unix(0, r.GetUnixNanos()).Format(logsClock),
		r.GetClient(), levelName(r.GetLevel()), r.GetMessage())
	for _, k := range slices.Sorted(maps.Keys(r.GetAttrs())) {
		v := r.GetAttrs()[k]
		if strings.ContainsAny(v, " \t\n\"=") {
			v = fmt.Sprintf("%q", v)
		}
		fmt.Fprintf(&b, " %s=%s", k, v)
	}
	fmt.Println(b.String())
}

func printGap(g *registryv1.GapBand) {
	kept := "how many were lost is not known"
	if g.GetTotal() > 0 {
		kept = fmt.Sprintf("%d of %d kept", g.GetRetained(), g.GetTotal())
	}
	fmt.Printf("---- GAP %s to %s, %s, %s: %s ----\n",
		time.Unix(0, g.GetFromUnixNanos()).Format(logsClock), time.Unix(0, g.GetToUnixNanos()).Format(logsClock),
		gapClient(g.GetClient()), g.GetCause(), kept)
}

func gapClient(c string) string {
	if c == "*" {
		return "every client"
	}
	return c
}

func gapKey(g *registryv1.GapBand) string {
	return fmt.Sprintf("%d/%d/%s/%s", g.GetFromUnixNanos(), g.GetToUnixNanos(), g.GetClient(), g.GetCause())
}

func logsJSON(resp *registryv1.LogsQueryResponse) map[string]any {
	recs := make([]map[string]any, 0, len(resp.GetRecords()))
	for _, r := range resp.GetRecords() {
		recs = append(recs, logRecordJSON(r))
	}
	gaps := make([]map[string]any, 0, len(resp.GetGaps()))
	for _, g := range resp.GetGaps() {
		gaps = append(gaps, gapJSON(g))
	}
	return map[string]any{"records": recs, "gaps": gaps, "latest": resp.GetLatest(), "truncated": resp.GetTruncated()}
}

func logRecordJSON(r *registryv1.LogRecord) map[string]any {
	m := map[string]any{
		"seq": r.GetSeq(), "client": r.GetClient(), "level": levelName(r.GetLevel()), "msg": r.GetMessage(),
		"at": time.Unix(0, r.GetUnixNanos()).UTC().Format(time.RFC3339Nano),
	}
	if len(r.GetAttrs()) > 0 {
		m["attrs"] = r.GetAttrs()
	}
	return m
}

func gapJSON(g *registryv1.GapBand) map[string]any {
	return map[string]any{
		"from":   time.Unix(0, g.GetFromUnixNanos()).UTC().Format(time.RFC3339Nano),
		"to":     time.Unix(0, g.GetToUnixNanos()).UTC().Format(time.RFC3339Nano),
		"client": g.GetClient(), "cause": g.GetCause(), "retained": g.GetRetained(), "total": g.GetTotal(),
	}
}

// logsSub is decision 11's two retention verbs: what the store holds and
// where its holes are, and a pin that keeps one segment past both ceilings.
func logsSub(l *logsFlags, positional []string) error {
	switch {
	case positional[0] == "coverage" && len(positional) == 1:
		return logsCoverage(l)
	case (positional[0] == "pin" || positional[0] == "unpin") && len(positional) == 2:
		id, err := strconv.ParseUint(positional[1], 10, 64)
		if err != nil {
			return badArgumentf("rig logs %s: a segment is its number, as rig logs coverage shows it, not %q",
				positional[0], positional[1])
		}
		return logsPin(id, positional[0] == "unpin", *l.asJSON)
	}
	return badArgumentf(logsUsage)
}

func logsCoverage(l *logsFlags) error {
	if len(l.clients.v) > 0 || *l.grep != "" || *l.limit != 0 || *l.follow {
		return badArgumentf("rig logs coverage takes --since, --until and --json only")
	}
	now := time.Now()
	req := &registryv1.LogsCoverageRequest{}
	var err error
	if req.SinceUnixNanos, err = logsTime(*l.since, now); err != nil {
		return badArgumentf("rig logs coverage: --since %s", err)
	}
	if req.UntilUnixNanos, err = logsTime(*l.until, now); err != nil {
		return badArgumentf("rig logs coverage: --until %s", err)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	var resp registryv1.LogsCoverageResponse
	if err := call(ctx, c, "rig.logs.coverage", req, &resp); err != nil {
		return err
	}
	if *l.asJSON {
		segs := make([]map[string]any, 0, len(resp.GetSegments()))
		for _, g := range resp.GetSegments() {
			segs = append(segs, map[string]any{
				"id": g.GetId(), sizeKey: g.GetBytes(), "compressed": g.GetCompressed(), "archived": g.GetArchived(),
				"pinned": g.GetPinned(), "open": g.GetOpen(),
				"modified": time.Unix(0, g.GetModifiedUnixNanos()).UTC().Format(time.RFC3339Nano),
			})
		}
		gaps := make([]map[string]any, 0, len(resp.GetGaps()))
		for _, g := range resp.GetGaps() {
			gaps = append(gaps, gapJSON(g))
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"segments": segs, "gaps": gaps, sizeKey: resp.GetBytes()})
	}
	printCoverage(&resp)
	return nil
}

func printCoverage(resp *registryv1.LogsCoverageResponse) {
	fmt.Printf("%-12s %10s  %-19s  %s\n", "SEGMENT", "SIZE", "LAST WRITTEN", "STATE")
	for _, g := range resp.GetSegments() {
		var state []string
		for _, f := range []struct {
			on   bool
			name string
		}{{g.GetOpen(), "open"}, {g.GetCompressed(), "compressed"}, {g.GetArchived(), "archived"}, {g.GetPinned(), "pinned"}} {
			if f.on {
				state = append(state, f.name)
			}
		}
		fmt.Printf("%-12d %10s  %-19s  %s\n", g.GetId(), sizeOf(g.GetBytes()),
			time.Unix(0, g.GetModifiedUnixNanos()).Format(time.DateTime), strings.Join(state, ", "))
	}
	fmt.Printf("%d segments and the blob area, %s on disk\n", len(resp.GetSegments()), sizeOf(resp.GetBytes()))
	if len(resp.GetGaps()) == 0 {
		fmt.Println("no gaps in the range")
	}
	for _, g := range resp.GetGaps() {
		printGap(g)
	}
}

// sizeKey is the JSON name of a size in bytes, as the other verbs emit it.
const sizeKey = "bytes"

// sizeOf is a size in the largest binary unit it fills.
func sizeOf(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func logsPin(id uint64, unpin, asJSON bool) error {
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	var resp registryv1.LogsPinResponse
	if err := call(ctx, c, "rig.logs.pin", &registryv1.LogsPinRequest{Segment: id, Unpin: unpin}, &resp); err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"segment": id, "pinned": !unpin, "changed": resp.GetChanged()})
	}
	verb := "pinned: kept past the age and size ceilings"
	if unpin {
		verb = "unpinned: the ceilings apply to it again"
	}
	if !resp.GetChanged() {
		verb = "already " + strings.SplitN(verb, ":", 2)[0]
	}
	fmt.Printf("segment %d %s\n", id, verb)
	return nil
}
