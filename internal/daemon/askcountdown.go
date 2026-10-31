package daemon

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

const askCountdown = "countdown"

func countdownSeconds(rec record.Record) uint32 {
	n, _ := strconv.ParseUint(rec.Fields["countdown_seconds"], 10, 32)
	return uint32(n)
}

// The existing expiry loop owns both delivery timeout and the armed grace.
// BOOTTIME makes a wall-clock correction irrelevant; a reboot denies it.
func (d *Daemon) startCountdown(fields map[string]string, seconds uint32) error {
	if d.leases == nil {
		return fmt.Errorf("countdown needs the estate's boot clock")
	}
	now, err := coord.Now()
	if err != nil {
		return err
	}
	fields["countdown_seconds"] = strconv.FormatUint(uint64(seconds), 10)
	fields["countdown_boot"] = d.leases.BootID()
	fields["countdown_deadline"] = strconv.FormatInt(int64(now.Add(time.Duration(seconds)*time.Second)), 10)
	return nil
}

func (d *Daemon) countdownRemaining(rec record.Record) (time.Duration, bool, error) {
	if d.leases == nil || rec.Fields["countdown_boot"] != d.leases.BootID() {
		return 0, false, nil
	}
	deadline, err := strconv.ParseInt(rec.Fields["countdown_deadline"], 10, 64)
	if err != nil {
		return 0, false, err
	}
	now, err := coord.Now()
	if err != nil {
		return 0, false, err
	}
	return coord.Instant(deadline).Sub(now), true, nil
}

func askIsOpen(a *registryv1.Ask) bool {
	return a.GetState() == "open" || a.GetState() == "awaiting_presentation"
}

// Only the sweep's durable terminal record permits execution. In particular,
// reading an elapsed wall deadline never races a Stop into approval.
func countdownState(a *registryv1.Ask, rec record.Record, replies []record.Record) {
	switch {
	case len(replies) > 0:
		r := replies[0]
		switch {
		case r.Fields["proceeded"] == fieldYes:
			a.State = "proceed"
		case r.Fields["withdrawn"] == fieldYes:
			a.State = "withdrawn"
		case r.Fields[askExpired] == fieldYes:
			a.State = "denied"
		default:
			a.State = "stopped"
		}
	case rec.Fields["ask_policy_never"] == fieldYes:
		a.State = "denied"
	case rec.Fields["countdown_presented"] != fieldYes:
		a.State = "awaiting_presentation"
		a.ExpiresUnixNano = 0 // no execution deadline before display
	default:
		a.State = "open"
	}
}

func (d *Daemon) serveAskPresented(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.AskPresentedRequest
	if !unmarshalOr(c, f, "ask.presented", &req) {
		return
	}
	kind := c.principal().Kind
	if c.agentDoor || kind != kernel.KindTerminal && kind != kernel.KindWindow {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, "rig.ask.presented: only an interactive renderer may acknowledge display")
		return
	}
	st, ok := d.recordStore(c, f, "ask.presented")
	if !ok {
		return
	}
	d.askMu.Lock()
	defer d.askMu.Unlock()
	rec, err := d.askRecord(ctx, st, req.GetId())
	if err != nil {
		if isNotFound(err) {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.ask.presented: no question "+strconv.Quote(req.GetId()))
		} else {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		}
		return
	}
	if rec.Fields["ask_mode"] != askCountdown {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.ask.presented: not a countdown")
		return
	}
	a, err := d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	if !askIsOpen(a) || rec.Fields["countdown_presented"] == fieldYes {
		c.reply(f.GetStreamId(), &registryv1.AskPresentedResponse{Ask: a})
		return
	}
	remaining, sameBoot, err := d.countdownRemaining(rec)
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	if !sameBoot || remaining <= 0 {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.ask.presented: presentation deadline passed")
		return
	}
	seconds := countdownSeconds(rec)
	if err := d.startCountdown(rec.Fields, seconds); err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	rec.Fields["countdown_presented"] = fieldYes
	rec.Fields["ask_expires"] = strconv.FormatInt(time.Now().Add(time.Duration(seconds)*time.Second).UnixNano(), 10)
	rec, err = st.Put(ctx, record.PutRequest{ID: rec.ID, IfVersion: rec.Version, Kind: rec.Kind, Project: rec.Project, Body: rec.Body, Fields: rec.Fields, Session: "rigd", Seat: eventSourceRig, Epoch: d.epoch})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	select {
	case d.askWake <- struct{}{}:
	default:
	}
	a, err = d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), &registryv1.AskPresentedResponse{Ask: a})
}

// Every terminal transition wakes the same directed event waiter. The record
// remains authoritative if an event is lost or its retained cursor has a gap.
func (d *Daemon) publishAskAnswer(rec record.Record, ans *registryv1.ToastAnswer) {
	if to := rec.Fields["to"]; to != "" {
		payload, _ := protojson.Marshal(ans)
		if _, _, err := d.events.publishTo(toastAnsweredKind, eventSourceRig, string(payload), to); err != nil {
			d.log.Warn("question answer event not kept", "question", rec.ID, "err", err)
		}
	}
}
