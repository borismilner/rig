package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/kernel"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

const (
	statusDefaultTTL = 30 * time.Second
	statusMinTTL     = time.Second
	statusMaxTTL     = 24 * time.Hour
	statusMaxText    = 400
	statusMaxLines   = 4
	statusMaxActive  = 4
)

type statusEntry struct {
	status *registryv1.TransientStatus
	owner  string
}

type statusEvent struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed,omitempty"`
}

func validStatusText(s string) string {
	if !utf8.ValidString(s) || len(s) == 0 || len(s) > statusMaxText || strings.TrimSpace(s) == "" {
		return "text must be 1 to 400 bytes of non-blank UTF-8"
	}
	if strings.Count(s, "\n")+1 > statusMaxLines {
		return "text must contain no more than four lines"
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "text must not contain control characters other than line breaks and tabs"
		}
	}
	return ""
}

func statusTTL(ms uint32) (time.Duration, error) {
	if ms == 0 {
		return statusDefaultTTL, nil
	}
	ttl := time.Duration(ms) * time.Millisecond
	if ttl < statusMinTTL || ttl > statusMaxTTL {
		return 0, fmt.Errorf("expiry must be 1 second to 24 hours")
	}
	return ttl, nil
}

func (d *Daemon) serveStatus(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "status.put":
		d.serveStatusPut(ctx, c, f)
	case "status.list":
		d.serveStatusList(ctx, c, f)
	case "status.remove":
		d.serveStatusRemove(ctx, c, f)
	}
}

func (d *Daemon) serveStatusPut(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.StatusPutRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "status.put: "+err.Error())
		return
	}
	if msg := validStatusText(req.GetText()); msg != "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.status.put: "+msg)
		return
	}
	ttl, err := statusTTL(req.GetTtlMs())
	if err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.status.put: "+err.Error())
		return
	}
	who, ok := d.panelCaller(c, f, "status.put")
	if !ok {
		return
	}

	d.statusMu.Lock()
	d.expireStatusesLocked(time.Now())
	now := time.Now()
	id := req.GetId()
	var prior *statusEntry
	if id != "" {
		prior = d.statuses[id]
		if prior == nil {
			d.statusMu.Unlock()
			c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.status.put: no status "+id)
			return
		}
		if prior.owner != who.name {
			d.statusMu.Unlock()
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, "rig.status.put: status belongs to "+prior.status.GetOwner())
			return
		}
	} else {
		if len(d.statuses) >= statusMaxActive {
			d.statusMu.Unlock()
			c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.status.put: the panel already has four active updates")
			return
		}
		id, err = newStatusID()
		if err != nil {
			d.statusMu.Unlock()
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		}
	}
	updated := now.UnixNano()
	if prior != nil {
		updated = max(updated, prior.status.GetUpdatedUnixNano()+1)
	}
	status := &registryv1.TransientStatus{
		Id: id, Text: req.GetText(), Owner: who.name,
		ExpiresUnixNano: now.Add(ttl).UnixNano(), UpdatedUnixNano: updated,
	}
	d.statuses[id] = &statusEntry{status: status, owner: who.name}
	d.statusMu.Unlock()

	d.wakeStatusSweep()
	d.publishJSON("status.changed", statusEvent{ID: id})
	c.reply(f.GetStreamId(), &registryv1.StatusPutResponse{Status: proto.Clone(status).(*registryv1.TransientStatus)})
}

func (d *Daemon) serveStatusList(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.StatusListRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "status.list: "+err.Error())
		return
	}
	d.statusMu.Lock()
	d.expireStatusesLocked(time.Now())
	statuses := make([]*registryv1.TransientStatus, 0, len(d.statuses))
	for _, entry := range d.statuses {
		statuses = append(statuses, proto.Clone(entry.status).(*registryv1.TransientStatus))
	}
	d.statusMu.Unlock()
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].GetUpdatedUnixNano() < statuses[j].GetUpdatedUnixNano()
	})
	c.reply(f.GetStreamId(), &registryv1.StatusListResponse{Statuses: statuses})
}

func (d *Daemon) serveStatusRemove(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.StatusRemoveRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "status.remove: "+err.Error())
		return
	}
	if req.GetId() == "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.status.remove: an id is required")
		return
	}
	interactive := !c.scoped.Load() && !c.agentDoor && (c.principal().Kind == kernel.KindTerminal || c.principal().Kind == kernel.KindWindow)
	who := panelCaller{}
	if !interactive {
		var ok bool
		who, ok = d.panelCaller(c, f, "status.remove")
		if !ok {
			return
		}
	}
	d.statusMu.Lock()
	d.expireStatusesLocked(time.Now())
	entry := d.statuses[req.GetId()]
	if entry == nil {
		d.statusMu.Unlock()
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.status.remove: no status "+req.GetId())
		return
	}
	if entry.owner != who.name && !interactive {
		d.statusMu.Unlock()
		c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, "rig.status.remove: only its owner or the user at the desktop may remove this status")
		return
	}
	delete(d.statuses, req.GetId())
	d.statusMu.Unlock()
	d.wakeStatusSweep()
	d.publishJSON("status.changed", statusEvent{ID: req.GetId(), Removed: true})
	c.reply(f.GetStreamId(), &registryv1.StatusRemoveResponse{})
}

func (d *Daemon) startStatusSweep(ctx context.Context) func() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			d.statusMu.Lock()
			wait := d.expireStatusesLocked(time.Now())
			d.statusMu.Unlock()
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-d.statusWake:
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

// expireStatusesLocked removes expired entries and returns the next wake time.
// No status content is written to disk: a daemon restart clears all updates.
func (d *Daemon) expireStatusesLocked(now time.Time) time.Duration {
	wait := statusMaxTTL
	for id, entry := range d.statuses {
		expires := time.Unix(0, entry.status.GetExpiresUnixNano())
		if !expires.After(now) {
			delete(d.statuses, id)
			d.publishJSON("status.changed", statusEvent{ID: id, Removed: true})
			continue
		}
		wait = min(wait, time.Until(expires))
	}
	return max(wait, time.Millisecond)
}

func (d *Daemon) wakeStatusSweep() {
	select {
	case d.statusWake <- struct{}{}:
	default:
	}
}

func newStatusID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create status id: %w", err)
	}
	return "status-" + hex.EncodeToString(raw[:]), nil
}
