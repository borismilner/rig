package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"

	"github.com/borismilner/rig/internal/record"
)

// THE BOARD (plan/55 requirements 1 to 3, 27). Ruled 2026-10-03: "a verb
// writes, the store keeps, the bus wakes". rig.panel.put checks a card and
// applies the caps, every version is a version of one record, rig publishes
// panel.changed, and a click on a card goes back to its owner as
// panel.acted. Bus-only was turned down: an event is a fact, not a command,
// and the bus keeps events in memory, at most once (plan/52 E4, E7, E8).
//
// beacon's board (rigged f7bbee9) is the model: its caps and its fields.

const (
	panelProject = "board"
	panelKind    = "card"
	panelActKind = "card-act"
	// fieldYes is a record field's true, as a card and a toast store it.
	fieldYes = "yes"

	maxPanelTitle   = 200
	maxPanelStatus  = 80
	maxPanelBody    = 4096
	maxPanelFacts   = 6
	maxPanelLabel   = 40
	maxPanelValue   = 200
	maxPanelActions = 3
	maxPanelProject = 120
	// Past it the sender's oldest open card is closed, as beacon drops it.
	maxOpenPanelCards = 40
	// A closed card stays on the board this long.
	panelClosedShown = 24 * time.Hour
	// The most cards one list answers.
	maxPanelList = 200
)

var panelSeverities = []string{"info", "success", "warning", "error"}

// servePanel dispatches the board's three verbs.
func (d *Daemon) servePanel(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "panel.put":
		d.servePanelPut(ctx, c, f)
	case "panel.list":
		d.servePanelList(ctx, c, f)
	case "panel.act":
		d.servePanelAct(ctx, c, f)
	}
}

// panelCaller is who is calling, as a card's owner or a click's author:
// the connection's program id, else its seat. Never the request's.
type panelCaller struct {
	name, to      string // to: the bus address of the name
	session, seat string
	epoch         uint64
}

func (d *Daemon) panelCaller(c *conn, f *rigv1.Frame, command string) (panelCaller, bool) {
	p := panelCaller{name: c.name()}
	var seated bool
	p.session, p.seat, p.epoch, seated = d.provenance(c)
	p.to = p.name
	if p.name == "" {
		if !seated {
			refuseUnseatedLease(c, f, command)
			return p, false
		}
		p.name, p.to = p.seat, seatSource(p.seat)
	}
	if p.session == "" {
		p.session = recordSession(c.principal())
	}
	p.seat = p.name
	p.epoch = max(p.epoch, d.epoch)
	return p, true
}

func (d *Daemon) servePanelPut(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.PanelPutRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "panel.put: "+err.Error())
		return
	}
	if msg := checkPanelPut(&req); msg != "" {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:    rigv1.Code_CODE_INVALID,
			Message: "rig.panel.put: " + msg,
			Precondition: "a title of 1 to 200 bytes for a new card, status at most 80, body at most 4096, " +
				"severity one of info, success, warning, error, progress 0 to 1, at most 6 facts and 3 actions, all UTF-8",
		})
		return
	}
	who, ok := d.panelCaller(c, f, "panel.put")
	if !ok {
		return
	}
	st, ok := d.recordStore(c, f, "panel.put")
	if !ok {
		return
	}
	now := time.Now()

	var prev *record.Record
	if id := req.GetCard(); id != "" {
		rec, err := st.Get(ctx, id)
		switch {
		case isNotFound(err) || err == nil && (rec.Kind != panelKind || rec.Project != panelProject):
			c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.panel.put: there is no card "+id)
			return
		case err != nil:
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		case rec.Retraction != nil:
			c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.panel.put: card "+id+" was retracted")
			return
		case rec.Fields["from"] != who.name:
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
				"rig.panel.put: card "+id+" is "+rec.Fields["from"]+"'s; a card is changed only by whoever put it")
			return
		case rec.Fields["closed"] == fieldYes:
			c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT,
				"rig.panel.put: card "+id+" is closed; put a new card instead")
			return
		}
		prev = &rec
	} else if req.Title == nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.panel.put: a new card needs a title")
		return
	} else if err := d.makeRoomOnBoard(ctx, st, who, now); err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}

	body, fields := applyPanelPut(prev, &req, who, now)
	put := record.PutRequest{
		Kind: panelKind, Project: panelProject, Body: body, Fields: fields,
		Session: who.session, Seat: who.seat, Epoch: who.epoch,
	}
	if prev != nil {
		put.ID, put.IfVersion = prev.ID, prev.Version
	}
	rec, err := st.Put(ctx, put)
	if isConflict(err) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT,
			"rig.panel.put: card "+put.ID+" changed under this call; send the change again")
		return
	}
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	card := panelCardOf(rec)
	d.events.publishRig("panel.changed", card)
	c.reply(f.GetStreamId(), &verbsv1.PanelPutResponse{Card: card})
}

// makeRoomOnBoard closes the sender's oldest open card when it already has
// the most a sender may keep open, so a runaway sender cannot grow the
// board without end.
func (d *Daemon) makeRoomOnBoard(ctx context.Context, st *record.Store, who panelCaller, now time.Time) error {
	mine, err := st.Find(ctx, record.QueryFilter{Project: panelProject, Kind: panelKind, Field: "from", Value: who.name})
	if err != nil {
		return err
	}
	var open []record.Record
	for _, r := range mine {
		if r.Fields["closed"] != fieldYes {
			open = append(open, r)
		}
	}
	if len(open) < maxOpenPanelCards {
		return nil
	}
	slices.SortFunc(open, func(a, b record.Record) int { return cmpInt(a.Fields["updated"], b.Fields["updated"]) })
	for _, r := range open[:len(open)-maxOpenPanelCards+1] {
		fields := maps.Clone(r.Fields)
		fields["closed"] = fieldYes
		fields["updated"] = strconv.FormatInt(now.UnixNano(), 10)
		rec, err := st.Put(ctx, record.PutRequest{
			ID: r.ID, IfVersion: r.Version,
			Kind: panelKind, Project: panelProject, Body: r.Body, Fields: fields,
			Session: who.session, Seat: who.seat, Epoch: who.epoch,
		})
		if err != nil && !isConflict(err) {
			return err
		}
		if err == nil {
			d.events.publishRig("panel.changed", panelCardOf(rec))
		}
	}
	return nil
}

// checkPanelPut is the caps, before anything is read or written.
func checkPanelPut(r *verbsv1.PanelPutRequest) string {
	text := func(present bool, s string, lo, hi int, what string) string {
		if !present {
			return ""
		}
		if len(s) < lo || len(s) > hi || !utf8.ValidString(s) {
			return what + " is " + strconv.Itoa(lo) + " to " + strconv.Itoa(hi) + " bytes of UTF-8"
		}
		return ""
	}
	for _, msg := range []string{
		text(r.Title != nil, r.GetTitle(), 1, maxPanelTitle, "a title"),
		text(r.Status != nil, r.GetStatus(), 0, maxPanelStatus, "a status"),
		text(r.Body != nil, r.GetBody(), 0, maxPanelBody, "a body"),
		text(r.Project != nil, r.GetProject(), 0, maxPanelProject, "a project"),
	} {
		if msg != "" {
			return msg
		}
	}
	if sev := r.GetSeverity(); sev != "" && !slices.Contains(panelSeverities, sev) {
		return "severity " + strconv.Quote(sev) + " is not one of info, success, warning, error"
	}
	if p := r.GetProgress(); r.Progress != nil && !(p >= 0 && p <= 1) {
		return "progress is from 0 to 1"
	}
	if r.Progress != nil && r.GetClearProgress() {
		return "progress and clear_progress together say two things"
	}
	return checkPanelLists(r)
}

// checkPanelLists is the caps on a card's facts and actions.
func checkPanelLists(r *verbsv1.PanelPutRequest) string {
	if fs := r.GetFacts(); fs != nil {
		if len(fs.GetFacts()) > maxPanelFacts {
			return "a card shows at most 6 facts"
		}
		for _, x := range fs.GetFacts() {
			if l, v := x.GetLabel(), x.GetValue(); l == "" || len(l) > maxPanelLabel || len(v) > maxPanelValue ||
				!utf8.ValidString(l) || !utf8.ValidString(v) {
				return "a fact has a label of 1 to 40 bytes and a value of at most 200, in UTF-8"
			}
		}
	}
	if as := r.GetActions(); as != nil {
		seen := map[string]bool{}
		if len(as.GetLabels()) > maxPanelActions {
			return "a card has at most 3 actions"
		}
		for _, a := range as.GetLabels() {
			if a == "" || len(a) > maxPanelLabel || !utf8.ValidString(a) || seen[a] {
				return "an action is 1 to 40 bytes of UTF-8, each distinct"
			}
			seen[a] = true
		}
	}
	return ""
}

// applyPanelPut is the card after this change: prev with every field the
// request carries replaced, or a new card from the request alone.
func applyPanelPut(prev *record.Record, r *verbsv1.PanelPutRequest, who panelCaller, now time.Time) (string, map[string]string) {
	stamp := strconv.FormatInt(now.UnixNano(), 10)
	body := ""
	fields := map[string]string{"from": who.name, "to": who.to, "created": stamp, "closed": "no"}
	if prev != nil {
		body = prev.Body
		fields = maps.Clone(prev.Fields)
	}
	fields["updated"] = stamp
	set := func(present bool, key, v string) {
		if present {
			fields[key] = v
		}
	}
	set(r.Project != nil, "project", r.GetProject())
	set(r.Title != nil, "title", r.GetTitle())
	set(r.Status != nil, "status", r.GetStatus())
	set(r.Severity != nil, "severity", r.GetSeverity())
	if r.Body != nil {
		body = r.GetBody()
	}
	if r.Progress != nil {
		fields["progress"] = strconv.FormatFloat(r.GetProgress(), 'f', -1, 64)
	}
	if r.GetClearProgress() {
		delete(fields, "progress")
	}
	if r.Busy != nil {
		fields["busy"] = map[bool]string{true: fieldYes, false: "no"}[r.GetBusy()]
	}
	if fs := r.GetFacts(); fs != nil {
		fields["facts"] = jsonOf(fs.GetFacts())
	}
	if as := r.GetActions(); as != nil {
		fields["actions"] = jsonOf(as.GetLabels())
	}
	if r.GetClose() {
		fields["closed"] = fieldYes
	}
	return body, fields
}

func panelCardOf(r record.Record) *verbsv1.PanelCard {
	fl := r.Fields
	card := &verbsv1.PanelCard{
		Id: r.ID, Version: r.Version, From: fl["from"], Project: fl["project"],
		Title: fl["title"], Status: fl["status"], Severity: fl["severity"], Body: r.Body,
		Busy: fl["busy"] == fieldYes, Closed: fl["closed"] == fieldYes,
	}
	if p, err := strconv.ParseFloat(fl["progress"], 64); err == nil {
		card.Progress, card.HasProgress = p, true
	}
	card.CreatedUnixNano, _ = strconv.ParseInt(fl["created"], 10, 64)
	card.UpdatedUnixNano, _ = strconv.ParseInt(fl["updated"], 10, 64)
	if s := fl["facts"]; s != "" {
		var facts []struct{ Label, Value string }
		if json.Unmarshal([]byte(s), &facts) == nil {
			for _, x := range facts {
				card.Facts = append(card.Facts, &verbsv1.PanelFact{Label: x.Label, Value: x.Value})
			}
		}
	}
	if s := fl["actions"]; s != "" {
		_ = json.Unmarshal([]byte(s), &card.Actions)
	}
	return card
}

func (d *Daemon) servePanelList(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.PanelListRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "panel.list: "+err.Error())
		return
	}
	st, ok := d.recordStore(c, f, "panel.list")
	if !ok {
		return
	}
	var (
		recs []record.Record
		err  error
	)
	if from := req.GetFrom(); from != "" {
		recs, err = st.Find(ctx, record.QueryFilter{Project: panelProject, Kind: panelKind, Field: "from", Value: from})
	} else {
		recs, err = st.Query(ctx, panelProject, panelKind)
	}
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	cards := boardOf(recs, time.Now())
	resp := &verbsv1.PanelListResponse{}
	if n := len(cards); n > maxPanelList {
		resp.Omitted = uint32(min(n-maxPanelList, math.MaxUint32))
		cards = cards[:maxPanelList]
	}
	resp.Cards = cards
	c.reply(f.GetStreamId(), resp)
}

// boardOf is what the board shows of these records: every open card and
// the ones closed within panelClosedShown, newest change first.
func boardOf(recs []record.Record, now time.Time) []*verbsv1.PanelCard {
	cutoff := now.Add(-panelClosedShown).UnixNano()
	var out []*verbsv1.PanelCard
	for _, r := range recs {
		card := panelCardOf(r)
		if card.GetClosed() && card.GetUpdatedUnixNano() < cutoff {
			continue
		}
		out = append(out, card)
	}
	slices.SortStableFunc(out, func(a, b *verbsv1.PanelCard) int {
		switch {
		case a.GetUpdatedUnixNano() > b.GetUpdatedUnixNano():
			return -1
		case a.GetUpdatedUnixNano() < b.GetUpdatedUnixNano():
			return 1
		}
		return 0
	})
	return out
}

func (d *Daemon) servePanelAct(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.PanelActRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "panel.act: "+err.Error())
		return
	}
	who, ok := d.panelCaller(c, f, "panel.act")
	if !ok {
		return
	}
	st, ok := d.recordStore(c, f, "panel.act")
	if !ok {
		return
	}
	id := req.GetCard()
	rec, err := st.Get(ctx, id)
	switch {
	case isNotFound(err) || err == nil && (rec.Kind != panelKind || rec.Project != panelProject || rec.Retraction != nil):
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.panel.act: there is no card "+id)
		return
	case err != nil:
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	card := panelCardOf(rec)
	switch {
	case card.GetClosed():
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.panel.act: card "+id+" is closed")
		return
	case !slices.Contains(card.GetActions(), req.GetAction()):
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"rig.panel.act: card "+id+" offers no action "+strconv.Quote(req.GetAction()))
		return
	}
	// Filed, so what was clicked is part of the card's story after the
	// bus has forgotten it.
	if _, err := st.Put(ctx, record.PutRequest{
		Kind: panelActKind, Project: panelProject,
		Fields:  map[string]string{"card": id, "action": req.GetAction(), "by": who.name},
		Session: who.session, Seat: who.seat, Epoch: who.epoch,
	}); err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	resp := &verbsv1.PanelActResponse{Card: id, Action: req.GetAction(), By: who.name, To: card.GetFrom()}
	d.events.publishRigTo("panel.acted", resp, rec.Fields["to"])
	c.reply(f.GetStreamId(), resp)
}

func isNotFound(err error) bool {
	var e *record.NotFoundError
	return errors.As(err, &e)
}

func isConflict(err error) bool {
	var e *record.ConflictError
	return errors.As(err, &e)
}

func jsonOf(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func cmpInt(a, b string) int {
	x, _ := strconv.ParseInt(a, 10, 64)
	y, _ := strconv.ParseInt(b, 10, 64)
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}
