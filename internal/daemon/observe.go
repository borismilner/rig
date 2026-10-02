package daemon

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/borismilner/rig/internal/observe"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 49's read side, as plan/49's build notes cut it: logs.query is a
// unary call with a cursor that may park, as events.wait does, because rigd
// has no stream handler yet. `rig logs -f` is a loop of them.

const (
	maxLogsWait     = 60 * time.Second
	defaultLogLimit = 1000
	maxLogLimit     = 10000
	maxLogGrep      = 256
	// maxLogAnswer keeps an answer inside the 1 MiB frame with room for
	// the gaps and the encoding's overhead.
	maxLogAnswer = 768 << 10
)

func (d *Daemon) serveLogs(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.LogsQueryRequest
	if !unmarshalOr(c, f, "logs.query", &req) {
		return
	}
	id := f.GetStreamId()
	if d.logs == nil {
		c.failStatus(id, &rigv1.Status{
			Code:    rigv1.Code_CODE_UNAVAILABLE,
			Message: "rig.logs.query: this rigd keeps no log store",
			Fix:     "rigd started by its own main keeps one; a daemon built without Config.Logs does not",
		})
		return
	}
	q := observe.Query{
		Since: req.GetSinceUnixNanos(), Until: req.GetUntilUnixNanos(),
		Clients: req.GetClients(), MinLevel: int(req.GetMinLevel()), After: req.GetAfter(),
		Limit: defaultLogLimit, MaxBytes: maxLogAnswer,
	}
	if n := req.GetLimit(); n > 0 {
		q.Limit = int(min(n, maxLogLimit))
	}
	if g := req.GetGrep(); g != "" {
		re, err := compileGrep(g)
		if err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "rig.logs.query: grep "+err.Error())
			return
		}
		q.Grep = re
	}
	// Decision 13: a program reads its own records; a holder of introspect,
	// every client's.
	if p := c.principal(); !p.Introspect {
		me := c.name()
		q.Allow = func(client string) bool { return me != "" && client == me }
	}

	var res observe.Result
	var err error
	if wait := min(time.Duration(req.GetTimeoutMs())*time.Millisecond, maxLogsWait); wait > 0 {
		res, err = d.logs.Wait(ctx, q, wait)
	} else {
		res, err = d.logs.Query(q)
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		c.fail(id, rigv1.Code_CODE_INTERNAL, "rig.logs.query: the log store could not be read: "+err.Error())
		return
	}
	c.reply(id, logsAnswer(res))
}

// compileGrep is RE2, so a pattern costs time linear in what it reads.
func compileGrep(g string) (*regexp.Regexp, error) {
	if len(g) > maxLogGrep {
		return nil, errGrepLong
	}
	return regexp.Compile(g)
}

var errGrepLong = errors.New("is over 256 bytes")

func logsAnswer(res observe.Result) *registryv1.LogsQueryResponse {
	out := &registryv1.LogsQueryResponse{Latest: res.Latest, Truncated: res.Truncated}
	for _, r := range res.Records {
		out.Records = append(out.Records, &registryv1.LogRecord{
			Seq: r.Seq, UnixNanos: r.At, Level: int32(r.Level), //nolint:gosec // an slog level
			Client: r.Client, Message: r.Message, Attrs: r.Attrs,
		})
	}
	for _, g := range res.Gaps {
		out.Gaps = append(out.Gaps, &registryv1.GapBand{
			FromUnixNanos: g.From, ToUnixNanos: g.To, Client: g.Client,
			Cause: g.Cause, Retained: g.Retained, Total: g.Total,
		})
	}
	return out
}
