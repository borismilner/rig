package daemon

import (
	"context"
	"errors"
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
// ⛔ THE SESSION IS `Token`, NOT THE FIELD NAMED `SessionID`. They look like
// synonyms and are opposites: SessionID names ONE CONNECTION and has never
// travelled to a caller, while Token is section 5f's session token, which
// outlives a socket and dies with one occupancy. principal.go records that
// misnomer rather than repairing it, so this is the line where reading it
// wrong would stamp every version of a reconnecting seat's work as a
// different author. PLAN.md section 39 carries the table.
//
// AN ANNOUNCED SEAT IS THE FIRST ANSWER AND A TERMINAL IS THE SECOND. A
// connection that announced writes under its seat. A connection that did not
// is attributed by the daemon instead of refused - see terminalSeat below for
// why that is not a weakening.
func (d *Daemon) provenance(c *conn) (session, seat string, epoch uint64, ok bool) {
	if occ, found := d.presence.occupantOf(c.occ); found && occ.seat != "" {
		return c.principal().Token, occ.seat, d.epoch, true
	}
	p := c.principal()
	if s := terminalSeat(c, p); s != "" {
		return p.Token, s, d.epoch, true
	}
	return "", "", 0, false
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
// ⛔ AND THERE IS NO MCP PATH THROUGH HERE TO WIDEN. The record verbs are
// dispatched only from the wire (BACKLOG B54: they have no MCP route, and
// section 9 ruled against giving them one), so this cannot hand a write to an
// unannounced agent. COORDINATION.md records that newPrincipal minting an MCP
// caller as KindTerminal is a separate OPEN RULING; this function does not
// settle it and must not be read as having done so.
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
	recs, err := st.Query(ctx, req.GetProject(), req.GetKind())
	if err != nil {
		c.failErr(f.GetStreamId(), recordCode(err), err)
		return
	}
	resp := &rigv1.RecordQueryResponse{}
	for _, r := range recs {
		resp.Records = append(resp.Records, recordToWire(r))
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
	}
	// A ZERO Since IS "NOBODY HAS STEPPED THIS YET", and it must not become a
	// 1970 timestamp on the wire - a reader sorting by age would put it above
	// every real signal, which is the opposite of what it means.
	if !i.Since.IsZero() {
		w.SinceUnixNano = i.Since.UnixNano()
	}
	return w
}

// briefSections reports the state of all ELEVEN of section 39's brief
// sections, every time, whether or not each one can be answered.
//
// ⛔ BORIS RULED ALL ELEVEN INTO THE MVP, 2026-09-16: "Cover all of them",
// asked directly whether the four that shipped were enough. The seven missing
// ones had been cut by one seat alone, and the cut is recorded in plan/39
// along with why the argument for it did not hold: ship-nothing and
// ship-an-always-empty-field were treated as the only two options, and the
// third is a section that reports its own state.
//
// ⛔ WHY THIS LIVES HERE AND ONLY FOR NOW. The reason a section cannot be
// answered is knowledge the DERIVATION has, not the wire - BACKLOG B46g gives
// internal/record the job of carrying it. Until that lands, the daemon is the
// only thing that knows which fields record.Brief actually has, and an empty
// `sections` would be exactly the defect this commit's sibling guard exists to
// catch: a served field nothing writes. When the store grows its own section
// states, this function passes them through and stops deciding.
//
// ⛔ THIS COMMENT USED TO CLAIM THE ANSWER WAS "DERIVED FROM THE STORE'S OWN
// STRUCT, not from a list of booleans kept in step by hand". IT IS NOT, AND IT
// NEVER WAS. The list below is hand-kept, this function takes no arguments, and
// nothing here reads record.Brief at all. The claim was false when it was
// written and it is what let two rows go stale within a day: internal/record
// grew Notes, Features and Stages, and these rows went on reporting
// NOT_COMPUTED with a reason blaming a derivation that had landed.
//
// ⛔ AND THE CLAIM CANNOT BE MADE TRUE HERE, which is the argument for B46g
// rather than for a cleverer function. Whether a section is computable is a
// property of the CODE; whether a slice is populated is a property of the DATA.
// A project with no notes returns an empty Notes, and a rule of "non-empty means
// computed" would report that project's notes as NOT_COMPUTED - the
// empty-reads-as-missing defect, arriving in the very mechanism built to
// separate the two. Only the derivation knows which it is, which is exactly why
// the states move into internal/record and this function stops deciding.
//
// KEEP THIS LIST IN STEP BY HAND UNTIL IT DOES, and treat every row as a claim
// that expires.
func briefSections() []*rigv1.BriefSectionStatus {
	computed := func(s rigv1.BriefSection) *rigv1.BriefSectionStatus {
		return &rigv1.BriefSectionStatus{
			Section: s, State: rigv1.SectionState_SECTION_STATE_COMPUTED,
		}
	}
	waiting := func(s rigv1.BriefSection, why string) *rigv1.BriefSectionStatus {
		return &rigv1.BriefSectionStatus{
			Section: s, State: rigv1.SectionState_SECTION_STATE_NOT_COMPUTED,
			Reason: why,
		}
	}

	const (
		projection = "the git projection does not exist yet, so there is nothing " +
			"to be behind. PLAN.md section 39's spine, and it is what makes the " +
			"degraded path readable at all"
		derivation = "the record store's brief derivation does not collect this " +
			"kind yet. BACKLOG B46g, and it is the last thing between this brief " +
			"and all eleven sections"
	)

	return []*rigv1.BriefSectionStatus{
		computed(rigv1.BriefSection_BRIEF_SECTION_OPEN),
		computed(rigv1.BriefSection_BRIEF_SECTION_NEXT_UP),
		computed(rigv1.BriefSection_BRIEF_SECTION_NOTES),
		computed(rigv1.BriefSection_BRIEF_SECTION_BLOCKED),
		waiting(rigv1.BriefSection_BRIEF_SECTION_DRIFT,
			"the standards register does not exist. standard.stamp and "+
				"standard.drift are slice 7 and are deliberately off this wire, so "+
				"nothing can be behind a standard rig cannot yet hold"),
		// ⛔ SPECIFIED, DECLARED ON THE WIRE, AND NOT BUILT - which is why it is
		// named here rather than left as two empty fields. plan/39 RULED that
		// the slice-2 brief returns the must-read set AND marks it delivered,
		// and `must_read`/`must_read_cleared` have been on this message since
		// the wire landed with nothing writing either. A caller reading an empty
		// list would conclude this project demands nothing.
		waiting(rigv1.BriefSection_BRIEF_SECTION_MUST_READ,
			"neither half of the must-read gate is built: the SET is records "+
				"marked in the store, and the MARK is per-session state keyed on "+
				"the session Token. Both are ruled for slice 2 in PLAN.md section "+
				"39. Until they exist an empty must_read set means UNKNOWN, never "+
				"\"this project requires nothing\""),
		waiting(rigv1.BriefSection_BRIEF_SECTION_PROJECTION_BEHIND, projection),
		waiting(rigv1.BriefSection_BRIEF_SECTION_PENDING, projection),
		waiting(rigv1.BriefSection_BRIEF_SECTION_LOCAL_ONLY, projection),
		computed(rigv1.BriefSection_BRIEF_SECTION_FEATURES),
		waiting(rigv1.BriefSection_BRIEF_SECTION_CASE_NOTES, derivation),
	}
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
	resp := &rigv1.ProjectBriefResponse{
		Project: b.Project,

		// ⛔ ALWAYS PRESENT. health is sections 7-9 and its zero value is the
		// correct encoding of a healthy project AND of a projection that does
		// not exist - only the section status separates them, which is why it
		// is never sent without one.
		Health:   &rigv1.BriefHealth{},
		Sections: briefSections(),

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
