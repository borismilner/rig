package daemon

import (
	"context"
	"strings"

	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/replyfield"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// restoreAsks puts back the toasts that asked for a reply, from the record,
// when the daemon starts. The questions were only in memory, so a restart
// made every toast.answer and toast.reply on them NOT_FOUND although both
// the notification and any reply were filed (buddies found it, 2026-10-04).
//
// An answered one is restored with its answer, so its sender still learns
// it. An unanswered one goes back on the ring as well, so a renderer shows
// it again: it never closes on its own, and a restart is not an answer. A
// retracted one is not restored; its sender took it back.
func (d *Daemon) restoreAsks(ctx context.Context) {
	if d.records == nil {
		return
	}
	notes, err := d.records.Find(ctx, record.QueryFilter{Project: notificationProject, Kind: notificationKind})
	if err != nil {
		d.log.Warn("the toasts asking for a reply were not restored", "err", err)
		return
	}
	replies, err := d.records.Find(ctx, record.QueryFilter{Project: notificationProject, Kind: replyKind})
	if err != nil {
		d.log.Warn("the toasts asking for a reply were not restored", "err", err)
		return
	}
	answers := map[string]*registryv1.ToastAnswer{}
	for _, r := range replies {
		id := r.Fields["notification"]
		if id == "" || answers[id] != nil {
			continue // the first reply won
		}
		answers[id] = &registryv1.ToastAnswer{
			RecordId: id, Answered: true, Reply: r.Fields["reply"], Text: r.Body,
			Dismissed: r.Fields["dismissed"] == fieldYes, By: r.Fields["by"],
			AtUnixNano: r.Prov.CreatedAt.UnixNano(),
		}
	}

	// Find orders by id, and a record id is a UUIDv7, so this is oldest
	// first: the newest maxToastAsks are kept, as ask keeps them.
	var asking []record.Record
	for _, n := range notes {
		if n.Fields["replies"] != "" || n.Fields["reply_text"] == fieldYes {
			asking = append(asking, n)
		}
	}
	asking = asking[max(0, len(asking)-maxToastAsks):]
	open := 0
	for _, n := range asking {
		buttons := replyfield.Decode(n.Fields["replies"])
		d.toasts.ask(n.ID, buttons, n.Fields["reply_text"] == fieldYes)
		if a := answers[n.ID]; a != nil {
			d.toasts.answer(n.ID, a)
			continue
		}
		open++
		d.toasts.add(&registryv1.Toast{
			RecordId: n.ID, Severity: severityOf(n.Fields["severity"]), Title: n.Fields["title"], Body: n.Body,
			Sender: n.Fields["sender"], AtUnixNano: n.Prov.CreatedAt.UnixNano(),
			Replies: buttons, ReplyText: n.Fields["reply_text"] == fieldYes,
		})
	}
	if len(asking) > 0 {
		d.log.Info("toasts asking for a reply restored", "asks", len(asking), "unanswered", open)
	}
}

// severityOf reads the severity word notify filed, info when it is not one.
func severityOf(word string) registryv1.Severity {
	if v, ok := registryv1.Severity_value["SEVERITY_"+strings.ToUpper(word)]; ok && v != 0 {
		return registryv1.Severity(v)
	}
	return registryv1.Severity_SEVERITY_INFO
}
