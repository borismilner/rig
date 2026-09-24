package daemon

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"strconv"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/record"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
	"google.golang.org/protobuf/proto"
)

// The record verbs (PLAN.md section 39).
//
// ALL NINE ARE SERVED HERE. `record.refs` WAS THE LAST AND IT LANDED 2026-09-16
// LATE, WITH ITS MESSAGES GROWN IN THE SAME CHANGE.
//
// ⛔ THE WIRE SHAPES EXISTED FIRST AND THAT IS EXACTLY WHY THEY WERE WRONG.
// They were written so section 21's additive change had somewhere to land, and
// because nothing dispatched them, nothing ever compared them to what
// internal/record actually returns: `Ref` carried three fields where the store
// computes six, and `RecordRefsRequest` could not express CrossProject at all.
// A message type is not a declaration - but it is also not a CONTRACT until
// something reads it against the thing it describes.
//
// THE RULE THAT CAME OUT OF IT: a message set written ahead of its handler is
// unverified, not merely unused, and the moment to check it is the moment
// before the first dispatch. Nothing else forces a reader to hold both files
// open at once.

// serveRecord dispatches every rig.record.* and rig.progress.* method.
//
// IT IS ITS OWN FILE FOR THE REASON serveSession IS ITS OWN METHOD: serveSelf
// crossed gocyclo's ceiling at ONE inline case, and eight would bury the
// dispatch switch it lives in.
func (d *Daemon) serveRecord(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	st, ok := d.recordStore(c, f, command)
	if !ok {
		return
	}
	switch command {
	case "record.put":
		d.serveRecordPut(ctx, c, f, st)
	case "record.get":
		d.serveRecordGet(ctx, c, f, st)
	case "record.query":
		d.serveRecordQuery(ctx, c, f, st)
	case "record.history":
		d.serveRecordHistory(ctx, c, f, st)
	case "record.link":
		d.serveRecordLink(ctx, c, f, st)
	case "record.unlink":
		d.serveRecordUnlink(ctx, c, f, st)
	case "record.refs":
		d.serveRecordRefs(ctx, c, f, st)
	case "record.retract":
		d.serveRecordRetract(ctx, c, f, st)
	case "record.delete":
		d.serveRecordDelete(ctx, c, f, st)
	case "record.replace":
		d.serveRecordReplace(ctx, c, f, st)
	case "progress.step":
		d.serveProgressStep(ctx, c, f, st)
	case "project.brief":
		d.serveProjectBrief(ctx, c, f, st)
	}
}

// recordStore answers the store, or refuses in the caller's terms.
//
// AN UNNAMED ESTATE HAS NO RECORD STORE AND THAT IS NOT A FAILURE. Open takes
// the estate name and refuses without one (record.UnnamedEstateError), the same
// way section 37 gives an unnamed estate no state directory at all. The refusal
// says which of the two situations this is, because "no store" from a daemon
// that is otherwise healthy reads as a bug unless it names the cause.
func (d *Daemon) recordStore(c *conn, f *rigv1.Frame, command string) (*record.Store, bool) {
	if d.records != nil {
		return d.records, true
	}
	if d.estate == "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_UNAVAILABLE,
			"rig."+command+": this estate is unnamed, so it has no record store. "+
				"The continuity record is per-estate state and an unnamed estate keeps none "+
				"(PLAN.md section 37). Name the estate to use the record.")
		return nil, false
	}
	c.fail(f.GetStreamId(), rigv1.Code_CODE_UNAVAILABLE,
		"rig."+command+": the record store for estate "+d.estate+" did not open, "+
			"so no record verb can be served. The daemon is otherwise healthy; "+
			"this is the store alone.")
	return nil, false
}

// provenance is WHO IS WRITING, and every field of it is the daemon's.
//
// ⛔ THE SESSION IS NEITHER `Token` NOR `SessionID` SINCE B86, AND THIS COMMENT
// USED TO SAY IT WAS `Token`. It is `recordSession` below, which reads the
// DERIVED unix session and falls back to `Token`. The old warning still holds
// for anybody reaching for a field here: `SessionID` names ONE CONNECTION and
// has never travelled to a caller, while `Token` is section 5f's session
// token, which outlives a socket and dies with one occupancy. Reading either
// one as the other stamps every version of a reconnecting seat's work as a
// different author. PLAN.md section 39 carries the table.
//
// AN ANNOUNCED SEAT IS THE FIRST ANSWER AND A TERMINAL IS THE SECOND. A
// connection that announced writes under its seat. A connection that did not
// is attributed by the daemon instead of refused - see terminalSeat below for
// why that is not a weakening.
func (d *Daemon) provenance(c *conn) (session, seat string, epoch uint64, ok bool) {
	if occ, found := d.presence.occupantOf(c.occ); found && occ.seat != "" {
		return recordSession(c.principal()), occ.seat, d.epoch, true
	}
	p := c.principal()
	if s := terminalSeat(c, p); s != "" {
		return recordSession(p), s, d.epoch, true
	}
	return "", "", 0, false
}

// recordSession is what a record's `session` field carries, and B86 is the whole of
// why it is not `Token` any more.
//
// ⛔ THE DEFECT WAS MEASURED TWICE AND THE SECOND TIME IT WAS THIS SESSION'S
// OWN DOING. `Token` dies with an occupancy and a terminal has none, so every
// `rig record put` minted a fresh one: generation 15 found 783 sessions over
// 451 decisions, and the 2026-09-18 re-seed then wrote 380 requirement records
// under 380 DISTINCT session values - `seat` constant, `epoch` constant, and
// the one field that claims to group the act disagreeing with every row.
//
// ⛔ B86's ROW POINTED AT THE WRONG LINE AND THIS IS WHERE THAT IS ANSWERED.
// It said the fix was `newPrincipal`'s `SessionID` at `principal.go:45`.
// `SessionID` never reaches a record - THIS function decides what does - and
// deriving it would have broken the registry, which keys succession and
// `Deregister` on `SessionID` naming exactly one connection. The derivation
// lands on its own field and the other two are untouched.
//
// ⛔ THE FALLBACK IS `Token`, WHICH IS TODAY'S ANSWER. An unreadable pid gives
// no unix session, and a record with an EMPTY session field would be a new
// silence where there used to be a useless value. Degrading to the old
// behaviour is the honest floor.
//
// ⛔ REOPEN CONDITION, INHERITED FROM section 39 AND NOT RESTARTED HERE: when a
// session outlives its connection at M7, the human surfaces that stopped
// printing this field get it back. That ruling is about RENDERING and this
// change does not touch it.
func recordSession(p kernel.Principal) string {
	if p.UnixSession > 0 {
		return "unix:" + strconv.Itoa(p.UnixSession)
	}
	return p.Token
}

// terminalSeat names the seat an UNANNOUNCED caller writes under, or "" when
// this connection is not one the daemon is willing to name.
//
// ⛔ RULED 2026-09-17 BY THE TEAM-LEAD, AND IT UNBLOCKED THE MVP. Four write
// verbs were reachable from no surface at all: `record.put`, `record.link`,
// `record.unlink` and `progress.step` all refused every terminal, because they
// demanded an announced seat and THERE IS NO `rig announce` AT A TERMINAL. The
// gap was known and lived in the refusal's own comment, arguing about that
// refusal's WORDING, connected to nothing it broke. Section 39's MVP is
// "seed rig's own backlog through the CLI", and it could not be done at all.
//
// ⛔ THE ALTERNATIVE IS ALREADY REFUSED IN WRITING, which is what decided this.
// PLAN.md section 37: "Make terminals handshake. No, and this one is refused on
// section 14's own terms" - section 14's `scoped` is "the whole authorisation
// state a connection carries: one boolean", and giving a terminal a hello makes
// that boolean ambiguous. "A precondition must not weaken the model it is a
// precondition for." So the CLI must NOT announce, and the daemon must name the
// caller itself.
//
// ⛔ AND IT WEAKENS NOTHING, BECAUSE A NAMED SEAT WAS THE MECHANISM AND NOT THE
// REQUIREMENT. Section 39 requires that provenance be THE DAEMON'S AND
// UNFORGEABLE. Every field below is read off the kernel's principal, which is
// minted at accept from the peer credentials of the socket; none of it is read
// off the request, and there is still no field a caller can set to claim an
// identity. A daemon-minted terminal seat satisfies that exactly as a session
// seat does.
//
// ⛔ A SCOPED CONNECTION IS STILL REFUSED, DELIBERATELY. A registered program
// has an identity of its own and writing it down as a terminal would be a lie
// the store could not later tell from the truth. It announces or it does not
// write.
//
// ⛔ THERE IS NOW AN MCP ROUTE TO THE RECORD AND THIS FUNCTION IS STILL NOT ON
// IT. Corrected 2026-09-17: this comment used to say the record verbs were
// dispatched only from the wire and that section 9 had ruled against an MCP
// route. The first half is obsolete and the second half was a MISREADING - B54
// re-read section 9 at the source and found it ruled against a sibling tool for
// asking rig about rig through the read-only `query`, and closed with "THE
// QUESTION IS: how does a WRITE to rig's own record reach an agent ... Nobody
// has asked it."
//
// ⛔ THE ROUTE THAT ANSWERS IT DOES NOT COME THROUGH HERE, WHICH IS WHY THIS
// FUNCTION IS UNCHANGED. `mcpCaller.writer` reads the seat off the connection's
// OCCUPANCY and refuses a write that has none, so an agent must announce first.
// That is strictly stronger than this concession rather than a second copy of
// it: this exists because there is no `rig announce` at a terminal and section
// 37 refuses to give one a handshake, and an agent has had `announce` as a
// first-class tool since the cutover.
//
// COORDINATION.md records that newPrincipal minting an MCP caller as
// KindTerminal is a separate OPEN RULING; this function does not settle it and
// must not be read as having done so.
func terminalSeat(c *conn, p kernel.Principal) string {
	if c.scoped.Load() || p.Kind != kernel.KindTerminal {
		return ""
	}
	// The daemon is one per unix user, so the user IS the author. The uid is
	// the fallback rather than the first choice because a name is what a human
	// reading their own backlog expects to see.
	if u, err := user.LookupId(strconv.Itoa(p.UID)); err == nil && u.Username != "" {
		return "terminal:" + u.Username
	}
	return "terminal:uid-" + strconv.Itoa(p.UID)
}

// refuseUnattributed is the one refusal every WRITE verb shares.
//
// ⛔ A TERMINAL NO LONGER REACHES THIS, AND THAT CHANGE IS WHY THE MVP MOVED.
// terminalSeat above names an unannounced terminal instead of turning it away,
// so what arrives here now is a SCOPED connection - a registered program that
// never announced. The Fix below is addressed to that caller and is right for
// it: a program is on the wire, rig.announce is a wire verb, so the thing it is
// told to do is a thing it can do.
func (d *Daemon) refuseUnattributed(c *conn, f *rigv1.Frame, command string) {
	c.failStatus(f.GetStreamId(), &rigv1.Status{
		Code: rigv1.Code_CODE_DENIED,
		Message: "rig." + command + ": this connection holds no seat, so the record " +
			"it would write could not say who wrote it",
		Precondition: "the caller announced into a named seat",
		Actual:       "this connection has not announced, or announced without a seat",
		// ⛔ NO FixCommand, AND ITS ABSENCE IS STILL THE ANSWER. `cmd/rig`
		// dispatches no `announce` word, so citing one would send the caller to
		// the CLI's default branch, which reads an unmatched word as a PROGRAM
		// name and reports that its program does not exist: a refusal blaming
		// the caller for this message's mistake. Caught by
		// refusal_verbs_test.go's dead-verb guard, which is what it is for.
		//
		// ⛔ AND THE OTHER HALF OF THIS COMMENT USED TO READ AS A JUSTIFICATION
		// WHEN IT WAS A DEFECT REPORT NOBODY CASHED. It observed that there is
		// no `rig announce` at a terminal - true - and stopped, while four write
		// verbs sat unreachable from the only surface section 39's MVP
		// demonstration runs through. A comment explaining why a message is
		// worded as it is must not also be the only place recording that the
		// message makes a verb uncallable.
		Fix: "call rig.announce on this connection first, with a seat. " +
			"Provenance is the daemon's and is never read off the request, " +
			"so there is no field you can set instead",
	})
}

func recordToWire(r record.Record) *rigv1.Record {
	return &rigv1.Record{
		Id:      r.ID,
		Version: r.Version,
		Kind:    r.Kind,
		Project: r.Project,
		Body:    r.Body,
		Fields:  r.Fields,
		Prov: &rigv1.Provenance{
			Session:    r.Prov.Session,
			Seat:       r.Prov.Seat,
			Epoch:      r.Prov.Epoch,
			AtUnixNano: r.Prov.CreatedAt.UnixNano(),
		},
		// ⛔ B77. Only Get and GetVersion ever set this on the store side, so
		// every other caller of this mapper passes nil and serves nothing -
		// which is the contract rather than an accident: the lists do not carry
		// retracted records at all.
		Retraction: retractionToWire(r.Retraction),
	}
}

// noteToWire carries one note, and `about` is the field that makes it readable.
//
// A note on the project itself and a note on a work item render differently and
// are not otherwise separable - section 39 scopes this list on the link's
// DESTINATION, so both arrive in one slice and only `about` tells them apart.
func noteToWire(n record.Note) *rigv1.BriefNote {
	return &rigv1.BriefNote{
		Id:       n.ID,
		Title:    n.Title,
		Body:     n.Body,
		Priority: n.Priority,
		About:    n.About,
		Prov: &rigv1.Provenance{
			Session:    n.Prov.Session,
			Seat:       n.Prov.Seat,
			Epoch:      n.Prov.Epoch,
			AtUnixNano: n.Prov.CreatedAt.UnixNano(),
		},
	}
}

func (d *Daemon) serveRecordPut(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordPutRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.put: "+err.Error())
		return
	}
	session, seat, epoch, ok := d.provenance(c)
	if !ok {
		d.refuseUnattributed(c, f, "record.put")
		return
	}
	rec, err := st.Put(ctx, record.PutRequest{
		ID:        req.GetId(),
		IfVersion: req.GetIfVersion(),
		Kind:      req.GetKind(),
		Project:   req.GetProject(),
		Body:      req.GetBody(),
		Fields:    req.GetFields(),
		Session:   session,
		Seat:      seat,
		Epoch:     epoch,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordPutResponse{Record: recordToWire(rec)})
}

func (d *Daemon) serveRecordGet(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordGetRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.get: "+err.Error())
		return
	}
	// VERSION 0 IS HEAD. No record has version 0 - the first write is 1 - so
	// the zero cannot collide with a version somebody meant.
	var (
		rec record.Record
		err error
	)
	if v := req.GetVersion(); v == 0 {
		rec, err = st.Get(ctx, req.GetId())
	} else {
		rec, err = st.GetVersion(ctx, req.GetId(), v)
	}
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordGetResponse{Record: recordToWire(rec)})
}

func (d *Daemon) serveRecordQuery(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordQueryRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.query: "+err.Error())
		return
	}
	// ⛔ AN EMPTY project OR kind MEANS *EVERY* ONE, AND THAT IS DELIBERATE
	// RATHER THAN A CONSEQUENCE OF protojson. Section 39 specifies record.query
	// as "by kind, field and project" and never makes any of the three
	// mandatory; the served version had turned an optional filter set into a
	// required conjunction, and since `kind` is not a closed set, that made
	// every census of the store incomplete by construction - a record under an
	// unguessed kind was invisible to every question anybody could write.
	//
	// ⛔ AN EMPTY FIELD AND AN UNSERVED FIELD ARE THE SAME BYTES, AND THE
	// AMBIGUITY IS ACCEPTED HERE ON THREE GROUNDS, WRITTEN DOWN SO THE NEXT
	// READER DOES NOT HAVE TO REDERIVE THEM:
	//
	//  1. This is a READ. A caller whose variable expanded to nothing gets MORE
	//     than it meant, never something else and never a write. The failure
	//     mode is a large answer, not a wrong one, and the answer carries each
	//     record's own project and kind, so the caller can see what it got.
	//  2. The alternative - an explicit `all_projects` bool or a sentinel - is a
	//     SECOND way to say one thing on the wire, and it invents a request
	//     that contradicts itself (`project: "rig"` with `all_projects: true`)
	//     for the server to arbitrate. That is more failure surface than a
	//     broad read, not less.
	//  3. The place a typo actually originates is the prompt, and the CLI CAN
	//     tell "not typed" from "typed empty" through flag.Visit. It refuses
	//     `--project ""` by name for exactly this reason. See cmd/rig/record.go.
	//
	// ⛔ AND THE FIELD PREDICATE DOES NOT INHERIT THAT REASONING, WHICH IS WHY
	// IT IS SPELLED OUT IN wire.proto RATHER THAN LEFT TO THIS COMMENT. An
	// empty `value` means the EMPTY STRING, not "every value": a record may
	// legitimately carry `closure_note: ""`, so there is a stored value for
	// the wildcard to collide with, which is exactly what project and kind do
	// not have. A `value` with no `field` is REFUSED by the store rather than
	// dropped, and the refusal travels back as an INVALID.
	//
	// ⛔ AND THE ANSWER IS PAGED, WHICH IS B116. An unbounded answer is not
	// a large answer, it is NO answer: the requirement query over the whole
	// plan reached 1,123,924 bytes, past wire.MaxFrameSize, and the query died
	// where nothing could read it. internal/daemon/recordpaging.go carries the
	// budget and the cursor; plan/50, "B116, the answer is paged", carries the
	// specification.
	after, err := decodeRecordCursor(req.GetAfter())
	if err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.query: "+err.Error())
		return
	}

	// ⛔ CLAMPED RATHER THAN CONVERTED: int(uint32) is NEGATIVE on a 32-bit
	// build for anything over 2^31, and a negative limit reads as "no cap at
	// all" - the widening direction this verb is careful about everywhere
	// else. The budget bounds the page either way, so the clamp costs a
	// caller nothing it could have used.
	recs, next, err := recordPage(ctx, st, record.QueryFilter{
		Project: req.GetProject(),
		Kind:    req.GetKind(),
		Field:   req.GetField(),
		Value:   req.GetValue(),
	}, after, int(min(req.GetLimit(), recordPageMaxLimit)))
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	resp := &rigv1.RecordQueryResponse{Records: recs}
	if next != (record.Cursor{}) {
		cursor, err := encodeRecordCursor(next)
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		}
		resp.Next = cursor
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) serveRecordHistory(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordHistoryRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.history: "+err.Error())
		return
	}
	recs, err := st.History(ctx, req.GetId())
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	resp := &rigv1.RecordHistoryResponse{}
	for _, r := range recs {
		resp.Versions = append(resp.Versions, recordToWire(r))
	}
	c.reply(f.GetStreamId(), resp)
}

// serveRecordLink and serveRecordUnlink DO NOT VALIDATE THE LINK TYPE.
//
// The store holds the closed set and refuses an unknown one by name, quoting
// the value back and listing the valid ones. A second copy of that set here is
// a second thing to keep in step with section 39: two validators drift, one
// does not.
func (d *Daemon) serveRecordLink(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordLinkRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.link: "+err.Error())
		return
	}
	if _, _, _, ok := d.provenance(c); !ok {
		d.refuseUnattributed(c, f, "record.link")
		return
	}
	if err := st.Link(ctx, req.GetSrc(), req.GetType(), req.GetDst()); err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordLinkResponse{})
}

func (d *Daemon) serveRecordUnlink(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordUnlinkRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.unlink: "+err.Error())
		return
	}
	if _, _, _, ok := d.provenance(c); !ok {
		d.refuseUnattributed(c, f, "record.unlink")
		return
	}
	if err := st.Unlink(ctx, req.GetSrc(), req.GetType(), req.GetDst()); err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordUnlinkResponse{})
}

// stepStateNames maps the wire's enum to the store's strings.
//
// UNSPECIFIED IS ABSENT FROM THIS MAP ON PURPOSE, so an unset field cannot
// decode as a decision - section 21's rule, which noenumzero enforces on the
// proto and this map enforces at the boundary.
var stepStateNames = map[rigv1.StepState]string{
	rigv1.StepState_STEP_STATE_STARTED: "started",
	rigv1.StepState_STEP_STATE_BLOCKED: "blocked",
	rigv1.StepState_STEP_STATE_DONE:    "done",
}

var stepStateWire = map[string]rigv1.StepState{
	"started": rigv1.StepState_STEP_STATE_STARTED,
	"blocked": rigv1.StepState_STEP_STATE_BLOCKED,
	"done":    rigv1.StepState_STEP_STATE_DONE,
}

func (d *Daemon) serveProgressStep(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.ProgressStepRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "progress.step: "+err.Error())
		return
	}
	session, seat, epoch, ok := d.provenance(c)
	if !ok {
		d.refuseUnattributed(c, f, "progress.step")
		return
	}
	// ⛔ THE PROJECT IS DERIVED FROM THE ITEM AND IS NOT A FIELD ON THE
	// REQUEST. The store needs one - progress.go refuses an empty project by
	// name - and the obvious repair was to add `project` to the wire. It is
	// the wrong repair: a step's project is not the caller's CHOICE, it is a
	// fact about the item being stepped. A field would let a caller name a
	// project the item is not in, which is a second source of truth for
	// exactly the reason section 39 keeps provenance off the request.
	//
	// It also buys a better refusal. Stepping an id that does not exist now
	// answers "no such record" instead of "a step needs a project", which is
	// the store complaining about a field the caller was never asked for.
	item, err := st.Get(ctx, req.GetItem())
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}

	// An unknown enum value arrives as the empty string and the store refuses
	// it by name, which is the one refusal a caller should see.
	rec, err := st.Step(ctx, record.StepRequest{
		Item:    req.GetItem(),
		State:   stepStateNames[req.GetState()],
		Note:    req.GetNote(),
		Project: item.Project,
		Session: session,
		Seat:    seat,
		Epoch:   epoch,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.ProgressStepResponse{Step: recordToWire(rec)})
}

func itemToWire(i record.ItemState) *rigv1.ItemState {
	w := &rigv1.ItemState{
		Id:    i.ID,
		Title: i.Title,
		State: stepStateWire[i.State],
		Note:  i.Note,
		// ⛔ ROW 2's FIELDS, WHICH THIS FUNCTION USED TO COMPUTE AND DROP.
		// The store filled every one of them and only five crossed the wire,
		// so the window had a truncated title and nothing behind it. That is
		// the whole of "the rows are not user friendly and clicking them
		// doesn't show the full description".
		DescriptionShort: i.DescriptionShort,
		Priority:         i.Priority,
		Status:           i.Status,
		Owner:            i.Owner,
		Tags:             i.Tags,
		TargetDate:       i.TargetDate,
		Semver:           i.Semver,
		ItemType:         i.ItemType,
	}
	// A ZERO Since IS "NOBODY HAS STEPPED THIS YET", and it must not become a
	// 1970 timestamp on the wire - a reader sorting by age would put it above
	// every real signal, which is the opposite of what it means.
	if !i.Since.IsZero() {
		w.SinceUnixNano = i.Since.UnixNano()
	}
	return w
}

// governingRows maps section 12's rows onto the wire.
//
// ⛔ `kind` IS COPIED AND NOT DERIVED FROM POSITION. The rows arrive grouped by
// kind, so a mapper could infer each row's kind from where it sits in the
// slice - and would then be correct until the day the derivation orders them
// any other way, at which point every row would be mislabelled and nothing
// would fail. The store already put the answer on the row.
func governingRows(in []record.GoverningRecord) []*rigv1.GoverningRecord {
	out := make([]*rigv1.GoverningRecord, 0, len(in))
	for _, g := range in {
		out = append(out, &rigv1.GoverningRecord{
			Id: g.ID, Kind: g.Kind, Title: g.Title,
		})
	}
	return out
}

// governingCounts maps section 12's per-kind counts onto the wire.
func governingCounts(in []record.KindCount) []*rigv1.KindCount {
	out := make([]*rigv1.KindCount, 0, len(in))
	for _, c := range in {
		out = append(out, &rigv1.KindCount{Kind: c.Kind, Count: c.Count})
	}
	return out
}

// closedRows maps section 13's closed work items onto the wire. B68.
//
// ⛔ THE WORD IS COPIED AND NOT RE-DERIVED. The store decided what closed each
// item - a status word it found, or `done` from the progress stream - and a
// mapper that looked at the status again here would be the same predicate
// written twice, which is how the two halves drift apart. Same argument as
// governingRows makes about `kind`.
func closedRows(in []record.ClosedItem) []*rigv1.ClosedItem {
	out := make([]*rigv1.ClosedItem, 0, len(in))
	for _, c := range in {
		out = append(out, &rigv1.ClosedItem{
			Id: c.ID, Title: c.Title, ClosingWord: c.Word,
		})
	}
	return out
}

// closedCounts maps section 13's per-word census onto the wire.
func closedCounts(in []record.WordCount) []*rigv1.WordCount {
	out := make([]*rigv1.WordCount, 0, len(in))
	for _, c := range in {
		out = append(out, &rigv1.WordCount{Word: c.Word, Count: c.Count})
	}
	return out
}

// containerFoundOnTheWire spells B76's condition as a Tristate.
//
// ⛔ THE POINT OF THE FUNCTION IS THAT IT NEVER RETURNS UNSPECIFIED. A `bool`
// on the wire would have made the zero mean both "no container" and "this
// daemon does not answer that", and those are the two things B76 was about
// telling apart. Spelling the mapping here rather than inline is what keeps
// the zero unreachable from this end: there is one expression to read.
func containerFoundOnTheWire(found bool) rigv1.Tristate {
	if found {
		return rigv1.Tristate_TRISTATE_YES
	}
	return rigv1.Tristate_TRISTATE_NO
}

// sectionStatuses maps the store's section states onto the wire.
//
// ⛔ IT PASSES THEM THROUGH AND DECIDES NOTHING, WHICH IS WHAT THE HAND-KEPT
// LIST THAT STOOD HERE PROMISED IT WOULD BECOME. That list carried a row per
// section with a reason written beside it, and its own comment said: "KEEP THIS
// LIST IN STEP BY HAND UNTIL [the store grows its own section states], and
// treat every row as a claim that expires." ⛔ **THE CLAIMS DID EXPIRE.** By the
// time B64 arrived the list still reported `case_notes` as NOT_COMPUTED, citing
// a derivation that had landed - the third row of that list to go stale in the
// same way, and the defect BACKLOG B46g was filed for.
//
// ⛔ WHY THE STORE IS THE ONLY PLACE THIS CAN LIVE, restated because deleting
// the list is the easy half and understanding why is the part that stops it
// coming back: whether a section is computABLE is a property of the code, and
// whether its slice is populated is a property of the data. A project with
// genuinely no notes returns an empty Notes, so any rule the daemon could apply
// from out here - "non-empty means computed" - reports that project's notes as
// missing. Only the derivation knows which it is, and internal/record's ledger
// now marks each section at the code that earns it.
//
// ⛔ THE SWITCH IS EXHAUSTIVE AND ITS default REFUSES. internal/record names its
// sections rather than numbering them, deliberately, so that this
// correspondence has to be written out where it can be read and tested - a cast
// would be a mapping nobody can see going wrong. A section added to the store
// with no arm here would otherwise travel as UNSPECIFIED, which section 21's
// rule says must never be a decision.
func sectionStatuses(in []record.SectionStatus) ([]*rigv1.BriefSectionStatus, error) {
	out := make([]*rigv1.BriefSectionStatus, 0, len(in))
	for _, st := range in {
		var sec rigv1.BriefSection
		switch st.Section {
		case record.SectionOpen:
			sec = rigv1.BriefSection_BRIEF_SECTION_OPEN
		case record.SectionNextUp:
			sec = rigv1.BriefSection_BRIEF_SECTION_NEXT_UP
		case record.SectionNotes:
			sec = rigv1.BriefSection_BRIEF_SECTION_NOTES
		case record.SectionBlocked:
			sec = rigv1.BriefSection_BRIEF_SECTION_BLOCKED
		case record.SectionDrift:
			sec = rigv1.BriefSection_BRIEF_SECTION_DRIFT
		case record.SectionMustRead:
			sec = rigv1.BriefSection_BRIEF_SECTION_MUST_READ
		case record.SectionProjectionBehind:
			sec = rigv1.BriefSection_BRIEF_SECTION_PROJECTION_BEHIND
		case record.SectionPending:
			sec = rigv1.BriefSection_BRIEF_SECTION_PENDING
		case record.SectionLocalOnly:
			sec = rigv1.BriefSection_BRIEF_SECTION_LOCAL_ONLY
		case record.SectionFeatures:
			sec = rigv1.BriefSection_BRIEF_SECTION_FEATURES
		case record.SectionCaseNotes:
			sec = rigv1.BriefSection_BRIEF_SECTION_CASE_NOTES
		case record.SectionGoverning:
			sec = rigv1.BriefSection_BRIEF_SECTION_GOVERNING
		case record.SectionClosed:
			sec = rigv1.BriefSection_BRIEF_SECTION_CLOSED
		default:
			return nil, fmt.Errorf("daemon: the record store reported section %q "+
				"and this wire has no member for it - a section added on one side "+
				"and not the other would travel as UNSPECIFIED, which section 21 "+
				"rules may never be a decision", st.Section)
		}

		var state rigv1.SectionState
		switch st.State {
		case record.SectionComputed:
			state = rigv1.SectionState_SECTION_STATE_COMPUTED
		case record.SectionNotComputed:
			state = rigv1.SectionState_SECTION_STATE_NOT_COMPUTED
		case record.SectionWithheldByView:
			state = rigv1.SectionState_SECTION_STATE_WITHHELD_BY_VIEW
		default:
			return nil, fmt.Errorf("daemon: section %q reported state %q, which "+
				"this wire has no member for", st.Section, st.State)
		}

		out = append(out, &rigv1.BriefSectionStatus{
			Section: sec, State: state, Reason: st.Reason,
		})
	}
	return out, nil
}

// serveRecordRefs answers what points AT a record - section 39's "correlated",
// and the direction files cannot go.
//
// ⛔ THE DEPTH IS NOT CLAMPED. A depth above the maximum is REFUSED BY NAME by
// the store, because a caller that asked for 8, got 5 and was not told has a
// partial answer that looks complete - which is the one failure this whole
// capability exists to prevent. The truncation flag is the same argument at the
// other end of the walk.
func (d *Daemon) serveRecordRefs(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordRefsRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.refs: "+err.Error())
		return
	}
	refs, err := st.Refs(ctx, record.RefsRequest{
		ID: req.GetId(),
		// The store reads zero as its own default and says which depth it
		// actually served, so the conversion carries the zero through rather
		// than substituting a number here - two places deciding one default is
		// two places to disagree from.
		Depth:        int(req.GetDepth()),
		CrossProject: req.GetCrossProject(),
	})
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}

	// ⛔ THE DEPTH ANSWERED IS DERIVED HERE BECAUSE record.Refs DOES NOT REPORT
	// IT, AND THAT IS A GAP WORTH NAMING RATHER THAN PAPERING OVER. The store
	// resolves a zero to its own default internally and returns only the edges,
	// so the one caller that has to tell a reader "how far did it actually
	// look" is this one.
	//
	// IT READS THE PACKAGE'S OWN EXPORTED CONSTANT RATHER THAN REPEATING THE
	// NUMBER. That is the difference between referencing one definition and
	// creating a second: if record.DefaultRefsDepth moves, this moves with it,
	// and there is no number here to fall out of step.
	answered := req.GetDepth()
	if answered == 0 {
		answered = uint32(record.DefaultRefsDepth)
	}
	resp := &rigv1.RecordRefsResponse{
		Id:        refs.ID,
		Depth:     answered,
		Truncated: refs.Truncated,
	}
	for _, r := range refs.Refs {
		w := &rigv1.Ref{
			Src:   r.ID,
			Type:  r.Type,
			Kind:  r.Kind,
			Title: r.Title,
			Via:   r.Via,
		}
		// BOUNDED ON BOTH SIDES, AND THE UPPER BOUND IS FOR THE CONVERTER
		// RATHER THAN FOR THE DATA. An unchecked int to uint32 turns a
		// negative into an enormous positive, and this field is read as "how
		// many hops away" - the one direction in which a wrong number looks
		// plausible rather than wrong.
		//
		// A depth above MaxRefsDepth is UNREACHABLE here: the store refuses
		// such a request BY NAME rather than clamping it, so nothing it
		// returns can exceed the bound. The check is what lets a reader - and
		// gosec - see that without holding refs.go open.
		if d := r.Depth; d > 0 && d <= record.MaxRefsDepth {
			w.Distance = uint32(d)
		}
		resp.Refs = append(resp.Refs, w)
	}
	// DETECTED, REPORTED, ORDERED AROUND, NEVER RESOLVED. rig does not pick an
	// edge to break, because choosing which one is wrong is a judgement about
	// the work rather than about the graph.
	for _, cy := range refs.Cycles {
		resp.Cycles = append(resp.Cycles, &rigv1.Cycle{Items: cy})
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) serveProjectBrief(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.ProjectBriefRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "project.brief: "+err.Error())
		return
	}
	b, err := st.Brief(ctx, req.GetProject())
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	// ⛔ A SECTION THE WIRE CANNOT NAME FAILS THE WHOLE BRIEF RATHER THAN
	// TRAVELLING AS UNSPECIFIED. Same argument as the store's own ledger one
	// level down: an answer that is silently missing a section reads as an
	// answer with nothing in that section.
	sections, err := sectionStatuses(b.Sections)
	if err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err.Error())
		return
	}
	resp := &rigv1.ProjectBriefResponse{
		Project: b.Project,

		// ⛔ ALWAYS PRESENT. health is sections 7-9 and its zero value is the
		// correct encoding of a healthy project AND of a projection that does
		// not exist - only the section status separates them, which is why it
		// is never sent without one.
		Health:   &rigv1.BriefHealth{},
		Sections: sections,

		// ⛔ SECTION 12, B64, AND IT IS MAPPED FIELD BY FIELD RATHER THAN
		// SPREAD. The store's GoverningRecord and the wire's have the same three
		// fields today; writing the correspondence out is what makes it visible
		// when they stop agreeing, which is the defect `briefFromWire` recorded
		// against itself when it went on naming five unread fields of six.
		Governing:       governingRows(b.Governing),
		GoverningCounts: governingCounts(b.GoverningCounts),

		// ⛔ SECTION 13, B68, AND `unlisted_items` TRAVELS WITH THE LIST OR
		// THE LIST LIES. A brief carrying the closed rows without the count of
		// the ones in neither list invites the reader to add two lists up for a
		// total that is short by every statusless item - the precise inference
		// the count was ruled in to correct.
		Closed:        closedRows(b.Closed),
		ClosedCounts:  closedCounts(b.ClosedCounts),
		UnlistedItems: b.Unlisted,

		// ⛔ THE PACKAGE COMPUTED THIS AND THIS FUNCTION THREW IT AWAY, SO THE
		// FIRST BRIEF rig EVER GAVE OF ITSELF SAID "(not said)" ABOUT ITS OWN
		// KIND. Measured 2026-09-17 on the production store, minutes after
		// rig's backlog was first seeded: `rig record get rig --json` answered
		// `"kind":"project"` and `rig brief rig` printed `rig (not said)`.
		//
		// brief.go:420 sets b.Kind from the container record whenever the Get
		// succeeds, and `wire.proto`'s own comment above `string kind = 9` says
		// these four fields exist because "the CLI renderer was built against
		// these and the wire did not carry them". ⛔ THE DEFERRAL WAS HONOURED
		// ON THE WIRE AND ON NEITHER SIDE OF IT - the field was reserved, the
		// package filled it, and the mapping between them was never written.
		//
		// ⛔ THAT DEFERRAL'S PRECONDITION IS GONE, AND THE SENTENCE THAT
		// STATED IT OUTLIVED IT. Until rig bb0c60f this comment read: "title,
		// status and semver are the OTHER HALF and are NOT set here on purpose:
		// `record.Brief` has no field for any of them yet, so setting them would
		// mean this function reading the store a second time and becoming a
		// second derivation." ⛔ EVERY CLAUSE OF THAT WAS TRUE WHEN WRITTEN
		// AND THE FIRST ONE WENT FALSE THE SAME DAY: `record.Brief` now carries
		// Title, Status and Semver, filled from the container record the
		// derivation already reads. There is no second read and no second
		// derivation - the values are on the brief in hand.
		//
		// ⛔ IT IS THE FOURTH INSTANCE THIS DAY OF A LABEL OUTLIVING WHAT IT
		// DESCRIBES, and the only one in the file of the seat collecting the
		// pattern. The others: a comment naming `B60-2` as the shape its guard
		// catches, which the guard silently accepts; `briefFromWire`'s comment
		// predicting its own rot and having already rotted; and a table caption
		// reading "in expected execution order" over a lexical id sort. ⛔ NO
		// GATE IN THIS REPOSITORY CHECKS A CAPTION, so each was found by a
		// person reading, and this one was found by two seats independently.
		//
		// ⛔ AND NO GUARD COULD HAVE CAUGHT IT FROM THE OTHER END. The CLI's
		// descriptor-coverage test proves the client RENDERS every wire field;
		// it cannot prove the daemon SETS one, because a field the daemon never
		// populates is byte-identical to a project that genuinely has no title.
		// That asymmetry is why these four fields are mapped together here,
		// beside the Kind whose absence was the visible half.
		Kind:   b.Kind,
		Title:  b.Title,
		Status: b.Status,
		Semver: b.Semver,

		// The container's own description, section 39's two field names.
		// ⛔ THEY ARE MAPPED HERE AND NOT ONLY DERIVED BECAUSE THIS FUNCTION IS
		// WHERE THE LAST SEVEN FIELDS WENT MISSING: itemToWire computed
		// description_short, priority, status, owner, tags, target_date and
		// semver and carried none of them, and nothing failed - it took a
		// screenshot from Boris to surface it. A field the store computes and
		// this file drops ANSWERS rather than refuses, which is worse than the
		// wire-that-lies section 39 already refuses elsewhere.
		DescriptionShort: b.DescriptionShort,
		DescriptionLong:  b.DescriptionLong,

		// ⛔ B76 AS A FACT ON THE WIRE, WHICH IS THE HALF THE FIX ABOVE LEFT
		// OUT. The derivation has carried `ContainerFound` since rig 072aea4
		// and this function dropped it, so the CLI re-derived the condition
		// from an empty Kind - correct only because these four fields happen
		// to be served, and wrong against every daemon that predates their
		// being served at all.
		//
		// ⛔ IT IS NEVER UNSPECIFIED FROM HERE. This daemon has read the
		// container, so it knows; UNSPECIFIED is reserved for a daemon that
		// does not carry the field, and a reader cannot tell "I did not look"
		// from "I looked and found nothing" if this end ever spends the zero.
		ContainerFound: containerFoundOnTheWire(b.ContainerFound),
	}

	// A NEGATIVE COUNT IS A BUG, AND ZERO IS THE HONEST ANSWER TO ONE. The
	// conversion is guarded rather than asserted because an unchecked int to
	// uint64 turns a negative into an enormous positive, and this field is
	// read as "how much of the migration is coarse" - the one direction in
	// which a wrong number would look alarming rather than wrong.
	if n := b.CoarseCitations; n > 0 {
		resp.CoarseCitations = uint64(n)
	}
	for _, i := range b.Open {
		resp.Open = append(resp.Open, itemToWire(i))
	}
	for _, i := range b.NextUp {
		resp.NextUp = append(resp.NextUp, itemToWire(i))
	}
	// A BLOCKER IS RESOLVED HERE, NOT LEFT AS AN ID. Once an `idea` item can
	// block, a blocker need not appear anywhere else in this response, so the
	// brief has to carry enough to render it. `byID` is every item the brief
	// knows; a blocker missing from it is one outside the active set, which is
	// exactly the case the blocked-set ruling added.
	byID := map[string]record.ItemState{}
	for _, i := range b.Open {
		byID[i.ID] = i
	}
	for _, i := range b.NextUp {
		byID[i.ID] = i
	}
	for _, bl := range b.Blocked {
		w := &rigv1.Blockage{Item: bl.Item, Title: bl.Title}
		for _, id := range bl.BlockedBy {
			blocker := &rigv1.Blocker{Id: id}
			if known, ok := byID[id]; ok {
				blocker.Title = known.Title
				blocker.State = stepStateWire[known.State]
			}
			w.Blockers = append(w.Blockers, blocker)
		}
		resp.Blocked = append(resp.Blocked, w)
	}
	for _, cy := range b.Cycles {
		resp.Cycles = append(resp.Cycles, &rigv1.Cycle{Items: cy})
	}

	// SECTIONS 3 AND 10, AND THE DAEMON WAS THE HALF THAT WAS MISSING. The
	// derivation landed them at 08ce632 and nothing here read the fields, so
	// project.brief answered without them while briefSections() reported them
	// NOT_COMPUTED with a reason blaming the derivation. A response that is
	// well-formed and short is the shape this file keeps having to catch:
	// the caller cannot tell a project with no notes from a daemon that never
	// looked.
	//
	// THE JOIN BETWEEN TWO FILES IS WHERE BOTH OF THIS SEAM'S DEFECTS HAVE
	// BEEN - the other direction was a wire field nothing wrote. Ownership
	// here is by file, so the join belongs to nobody, and only a seat reading
	// both sides at once finds either.
	for _, n := range b.Notes {
		resp.Notes = append(resp.Notes, noteToWire(n))
	}
	for _, ft := range b.Features {
		resp.Features = append(resp.Features, &rigv1.Feature{
			Id: ft.ID, Title: ft.Title, Stage: ft.Stage,
		})
	}
	// REPEATED, NOT A MAP, and the reason is a golden test: map iteration
	// order is unspecified in Go, so a map here would reorder the same answer
	// between runs. Raised by the record seat before the shape was chosen.
	for _, sc := range b.Stages {
		resp.FeatureStages = append(resp.FeatureStages, &rigv1.StageCount{
			Stage: sc.Stage, Count: sc.Count,
		})
	}

	c.reply(f.GetStreamId(), resp)
}

// recordCode maps a store refusal to a wire code.
//
// ⛔ IT SEPARATES THE ONES A CALLER CAN ACT ON DIFFERENTLY, rather than
// answering INVALID to everything. A version conflict is retryable after a
// re-read and a missing record is not, and a caller that cannot tell them
// apart will retry the one that never succeeds.
func recordCode(err error) rigv1.Code {
	var notFound *record.NotFoundError
	if errors.As(err, &notFound) {
		return rigv1.Code_CODE_NOT_FOUND
	}
	var conflict *record.ConflictError
	if errors.As(err, &conflict) {
		return rigv1.Code_CODE_CONFLICT
	}
	return rigv1.Code_CODE_INVALID
}

// ---- B77: full control over the records -----------------------------------
//
// ⛔ THREE HANDLERS AND NOT ONE WITH A MODE, because they are three
// capabilities. Boris, 2026-09-17: "everybody can delete/retract records and
// replace records". His two distinguishing questions keep them apart - does the
// id survive, does the HISTORY survive - and a mode flag would have invited
// exactly the collapse the plan names: shipping retract and reporting the
// sentence satisfied.
//
// ⛔ NONE OF THEM CHECKS WHO IS ASKING BEYOND ATTRIBUTION. "Everybody" is his
// word, and section 42's default is "absolutely without restrictions". The
// provenance call below is not a permission check: it establishes WHO ACTED so
// the withdrawal can be argued with, and it refuses only a connection the
// daemon cannot name at all - the same bar record.put already sets.

// retractionToWire is a withdrawal on the wire, or nil when there is none.
//
// ⛔ nil AND NOT AN EMPTY MESSAGE. An empty Retraction would decode as "this
// record is withdrawn and every fact about the withdrawal is missing", which is
// the reassuring-lie direction: a reader would treat a live record as gone.
func retractionToWire(r *record.Retraction) *rigv1.Retraction {
	if r == nil {
		return nil
	}
	return &rigv1.Retraction{
		Id:         r.ID,
		Reason:     r.Reason,
		ReplacedBy: r.ReplacedBy,
		Prov: &rigv1.Provenance{
			Session:    r.Prov.Session,
			Seat:       r.Prov.Seat,
			Epoch:      r.Prov.Epoch,
			AtUnixNano: r.Prov.CreatedAt.UnixNano(),
		},
	}
}

// edgesToWire maps an account of dropped or moved edges.
func edgesToWire(in []record.Edge) []*rigv1.Edge {
	out := make([]*rigv1.Edge, 0, len(in))
	for _, e := range in {
		out = append(out, &rigv1.Edge{Src: e.Src, Type: e.Type, Dst: e.Dst})
	}
	return out
}

func (d *Daemon) serveRecordRetract(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordRetractRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.retract: "+err.Error())
		return
	}
	session, seat, epoch, ok := d.provenance(c)
	if !ok {
		d.refuseUnattributed(c, f, "record.retract")
		return
	}
	out, err := st.Retract(ctx, record.RetractRequest{
		ID: req.GetId(), Reason: req.GetReason(),
		Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordRetractResponse{
		Retraction: retractionToWire(&out), Already: out.Already,
	})
}

func (d *Daemon) serveRecordDelete(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordDeleteRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.delete: "+err.Error())
		return
	}
	// ⛔ A DRY RUN IS STILL ATTRIBUTED. It writes nothing, but it is the step a
	// caller takes before the destructive one, and a daemon that cannot name
	// who is asking cannot name who then deleted.
	if _, _, _, ok := d.provenance(c); !ok {
		d.refuseUnattributed(c, f, "record.delete")
		return
	}
	out, err := st.Delete(ctx, record.DeleteRequest{
		ID: req.GetId(), DryRun: req.GetDryRun(),
	})
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordDeleteResponse{
		Id: out.ID, Versions: out.Versions,
		Edges: edgesToWire(out.Edges), DryRun: out.DryRun,
	})
}

func (d *Daemon) serveRecordReplace(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store) {
	var req rigv1.RecordReplaceRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "record.replace: "+err.Error())
		return
	}
	session, seat, epoch, ok := d.provenance(c)
	if !ok {
		d.refuseUnattributed(c, f, "record.replace")
		return
	}
	out, err := st.Replace(ctx, record.ReplaceRequest{
		Old: req.GetOld(), New: req.GetNew(), Reason: req.GetReason(),
		Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.RecordReplaceResponse{
		Old: out.Old, New: out.New,
		Moved: edgesToWire(out.Moved), Merged: edgesToWire(out.Merged),
		Dropped: edgesToWire(out.Dropped),
		// ⛔ THE WITHDRAWAL TRAVELS WITH THE REPLACEMENT. Without it the caller
		// is told the edges moved and not that the loser is now gone from every
		// list, which is half of what the verb did.
		Retraction: retractionToWire(&out.Retraction),
	})
}
