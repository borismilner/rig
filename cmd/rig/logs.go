package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig logs` - section 49's merged view of the estate's log store: rigd and
// every program, time-ordered by each record's own clock, with every gap the
// store knows of drawn as a band rather than left out.

const logsUsage = "usage: rig logs [--since D|T] [--until D|T] [--client X]... [--level L] [--grep RE] [--limit N] [-f] [--json]"

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
		return badArgumentf(logsUsage)
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
