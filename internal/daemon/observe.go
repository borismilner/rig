package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/kernel"
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

// Call records (plan/49 decision 2): every call routed to a program is
// recorded at dispatch, its payloads redacted by the target's declaration
// on the way in (decision 8), so nothing a program declared sensitive ever
// reaches a segment. rig's own verbs are not recorded: they declare no
// pointers, so §15's option 2 would leave only a size, and the waiting
// verbs (logs.query, events.wait) would record a call per poll.

// secretsProgram is §15's secrets service. Whatever it is sent or answers
// is recorded only by its key name (decision 9), before any declaration is
// consulted, because a field not declared sensitive IS recorded.
const secretsProgram = "secrets"

// The notes a call record carries where a payload is not recorded.
const (
	noteOpaque     = "not JSON, so recorded by size only (section 15, option 2)"
	noteUndeclared = "the command is not declared, so there is nothing to redact by: size only"
	noteSecret     = "the secrets service: only the key name is recorded"
	noteError      = "an error answer: its code is recorded, its message is not declared and is not"
)

// compileRedaction compiles each command's sensitive pointers. A list that
// does not compile redacts the whole payload rather than none of it.
func (d *Daemon) compileRedaction(decl kernel.Declaration) map[string]*observe.Redactor {
	out := make(map[string]*observe.Redactor, len(decl.Commands))
	for _, cmd := range decl.Commands {
		r, err := observe.Compile(cmd.Sensitive)
		if err != nil {
			d.log.Warn("a sensitive pointer did not compile, so the command's payloads are redacted whole",
				"program", decl.Identity.ID, "command", cmd.ID, "err", err)
			r, _ = observe.Compile([]string{""})
		}
		out[cmd.ID] = r
	}
	return out
}

func (d *Daemon) recordCall(from caller, program, command string, start time.Time, reply *rigv1.Frame, bad *callFailure) {
	if d.logs == nil || command == ProbeCommand {
		return
	}
	args := callArgs(from.args)
	rec := observe.Call{
		At: start, Took: time.Since(start), Caller: callerName(from.who),
		Method: program + "." + command, ArgsSize: len(args),
	}
	var r *observe.Redactor
	known := false
	d.mu.RLock()
	if to := d.programs[program]; to != nil {
		r, known = to.redact[command]
	}
	d.mu.RUnlock()

	switch {
	case program == secretsProgram:
		rec.Args, rec.ArgsNote = secretKey(args), ""
		if rec.Args == nil {
			rec.ArgsNote = noteSecret
		}
	case !known:
		rec.ArgsNote = noteUndeclared
	default:
		rec.Args, rec.ArgsNote = redacted(r, args)
	}

	switch {
	case bad != nil:
		rec.Code = bad.status.GetCode().String()
		rec.ResNote = "refused by rig: " + bad.status.GetMessage()
	case reply.GetKind() == rigv1.FrameKind_FRAME_KIND_ERROR:
		rec.Code = reply.GetStatus().GetCode().String()
		rec.ResSize = len(reply.GetStatus().GetMessage())
		rec.ResNote = noteError
	default:
		var resp rigv1.CallResponse
		if proto.Unmarshal(reply.GetPayload(), &resp) != nil {
			rec.ResSize, rec.ResNote = len(reply.GetPayload()), noteOpaque
			break
		}
		rec.ResSize = len(resp.GetResult())
		switch {
		case program == secretsProgram:
			rec.ResNote = noteSecret
		case !known:
			rec.ResNote = noteUndeclared
		default:
			rec.Result, rec.ResNote = redacted(r, resp.GetResult())
		}
	}
	d.logs.AppendCall(rec)
}

func redacted(r *observe.Redactor, payload []byte) ([]byte, string) {
	out, err := r.Apply(payload)
	if err != nil {
		return nil, noteOpaque
	}
	return out, ""
}

// secretKey is a secrets call's arguments reduced to the key it names: the
// top-level "key" or "name" string, re-encoded, or nil.
func secretKey(args []byte) []byte {
	var m map[string]any
	if json.Unmarshal(args, &m) != nil {
		return nil
	}
	for _, k := range []string{"key", "name"} {
		if v, ok := m[k].(string); ok {
			b, err := json.Marshal(map[string]string{k: v})
			if err != nil {
				return nil
			}
			return b
		}
	}
	return nil
}

// callerName is who made a call, as its record names it.
func callerName(p kernel.Principal) string {
	if p.ClientID != "" {
		return p.ClientID
	}
	return p.Kind.String()
}
