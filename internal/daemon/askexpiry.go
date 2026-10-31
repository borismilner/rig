package daemon

import (
	"context"
	"strconv"
	"time"

	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// startAskSweep records expiry at its deadline, and recalculates the next
// deadline when an ask opens. One timer serves the estate, not one per ask.
func (d *Daemon) startAskSweep(ctx context.Context) func() {
	if d.records == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			wait, err := d.sweepAsks(ctx)
			if err != nil {
				d.log.Warn("question expiry sweep failed", "err", err)
				wait = time.Second
			}
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-d.askWake:
			case <-ctx.Done():
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}()
	return func() { <-done }
}

func (d *Daemon) sweepAsks(ctx context.Context) (time.Duration, error) {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	st := d.records
	recs, err := st.Find(ctx, record.QueryFilter{
		Project: notificationProject, Kind: notificationKind, Field: "ask_marker", Value: askMarker,
	})
	if err != nil {
		return 0, err
	}
	replies, err := st.Find(ctx, record.QueryFilter{Project: notificationProject, Kind: replyKind})
	if err != nil {
		return 0, err
	}
	answered := make(map[string]bool, len(replies))
	for _, r := range replies {
		answered[r.Fields["notification"]] = true
	}
	now := time.Now()
	next := 24 * time.Hour
	for _, rec := range recs {
		if answered[rec.ID] || rec.Fields["ask_policy_never"] == fieldYes {
			continue
		}
		nano, err := strconv.ParseInt(rec.Fields["ask_expires"], 10, 64)
		if err != nil {
			return 0, err
		}
		remaining := time.Until(time.Unix(0, nano))
		proceed := false
		if rec.Fields["ask_mode"] == askCountdown {
			var sameBoot bool
			remaining, sameBoot, err = d.countdownRemaining(rec)
			if err != nil {
				return 0, err
			}
			proceed = sameBoot && rec.Fields["countdown_presented"] == fieldYes
			// The Go timer does not advance during suspend. One bounded
			// recheck observes BOOTTIME after resume without a second loop.
			next = min(next, time.Second)
		}
		if remaining > 0 {
			if remaining < next {
				next = remaining
			}
			continue
		}
		fields := map[string]string{"notification": rec.ID, askExpired: fieldYes, "by": eventSourceRig}
		if proceed {
			fields["proceeded"] = fieldYes
		}
		_, err = st.Put(ctx, record.PutRequest{
			Kind: replyKind, Project: notificationProject,
			Fields:  fields,
			Session: "rigd", Seat: eventSourceRig, Epoch: d.epoch,
		})
		if err != nil {
			return 0, err
		}
		ans := &registryv1.ToastAnswer{
			RecordId: rec.ID, Answered: true, Expired: true, By: eventSourceRig, AtUnixNano: now.UnixNano(),
		}
		d.toasts.answer(rec.ID, ans)
		d.toasts.finish(rec.ID, rec.Fields["sender"], true)
		d.publishJSON("toast.retracted", toastRetraction{
			RecordID: rec.ID, Sender: rec.Fields["sender"], Reason: askExpired,
		})
		d.publishAskAnswer(rec, ans)
	}
	return next, nil
}
