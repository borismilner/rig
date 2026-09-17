package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
	"github.com/boris-milner/rig/internal/record"
)

// The continuity record is the optional half of the meta surface that the agent
// door had no route to at all, and this is where it is satisfied. A
// compile-time assertion rather than a comment, because the assertion is what
// fails when a method drifts.
var _ meta.Records = (*mcpCaller)(nil)

// store resolves this connection's record store, or says why there is none.
//
// ⛔ AN UNNAMED ESTATE IS A REFUSAL WITH A CAUSE, NOT AN EMPTY ANSWER. Section
// 37 makes the continuity record per-estate state and gives an unnamed estate
// none, so this is a configuration the product supports rather than a fault.
// Returning an empty result here would tell an agent its project has no records
// when the truth is that this estate keeps none at all - two different facts
// rendering as the same bytes, which is section 5k's cardinal failure.
func (m *mcpCaller) store() (*record.Store, error) {
	if m.records == nil {
		return nil, &kernel.RefusalError{
			Err:          errors.New("this estate has no continuity record"),
			Precondition: "the record is per-estate state and an unnamed estate keeps none (PLAN.md section 37)",
			Actual:       "this daemon is serving an unnamed estate",
			Fix:          "reconnect to a named estate - production or development",
		}
	}
	return m.records, nil
}

// writer names the seat this connection writes under, and refuses if it has
// none.
//
// ⛔ THIS IS B74's FIX AND IT IS THE REASON THE RECORD ROUTE IS HERE RATHER THAN
// ANYWHERE ELSE. Measured 2026-09-17 on the live production store: 565 records,
// ONE distinct seat, 561 distinct sessions. Every `record put` reached the
// daemon down a fresh unannounced connection, so "show me what this seat
// decided" - the question section 09's A5 asks of an agent's own notes - was
// unanswerable by construction, and section 42's per-agent policy had no key to
// bind to.
//
// ⛔ AN AGENT MUST ANNOUNCE BEFORE IT WRITES, AND THAT IS STRICTLY STRONGER THAN
// THE TERMINAL PATH RATHER THAN A PARALLEL TO IT. `terminalSeat` exists because
// there is no `rig announce` at a terminal and section 37 refuses to give one a
// handshake; an agent has `announce` as a first-class tool and has had since the
// cutover, so the reason that concession exists does not apply here. The seat is
// read off the occupancy this connection already holds - never off the request -
// so it remains the daemon's and unforgeable, which is section 39's actual
// requirement.
func (m *mcpCaller) writer() (session, seat string, epoch uint64, err error) {
	occ, found := m.presence.occupantOf(m.occ)
	if !found || occ.seat == "" {
		return "", "", 0, &kernel.RefusalError{
			Err:          errors.New("a record write needs a seat"),
			Precondition: "every record carries who wrote it, and the seat comes from your roster row rather than from the request, so it cannot be claimed",
			Actual:       "this connection holds no seat",
			Fix:          "call announce with a seat name first, then write",
		}
	}
	return m.principalToken(), occ.seat, m.epoch, nil
}

// principalToken is the session id a write is stamped with.
//
// IT IS THE CONNECTION'S, minted at accept from the peer credentials of the
// socket, and there is no field a caller can set to change it.
func (m *mcpCaller) principalToken() string { return m.who.Token }

// rowOf converts one stored record to the surface's shape.
//
// ⛔ EVERY FIELD IS COPIED EXPLICITLY AND NONE IS SYNTHESISED. An empty Seat
// means the record genuinely has none - every row written before this route
// existed - and inventing one here would destroy the census that measures B74's
// progress. A blank is the honest value.
func rowOf(r record.Record) meta.RecordRow {
	return meta.RecordRow{
		ID: r.ID, Project: r.Project, Kind: r.Kind, Version: r.Version,
		Body: r.Body, Fields: r.Fields,
		Seat: r.Prov.Seat, Session: r.Prov.Session, Epoch: r.Prov.Epoch,
		Written: r.Prov.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func rowsOf(in []record.Record) []meta.RecordRow {
	// NON-NIL AND EMPTY, DELIBERATELY. A nil slice and an empty one are the
	// same JSON here, but they are not the same thing to a Go caller, and the
	// answer to "no records matched" is a set with nothing in it rather than an
	// absence of a set.
	out := make([]meta.RecordRow, 0, len(in))
	for _, r := range in {
		out = append(out, rowOf(r))
	}
	return out
}

func (m *mcpCaller) Put(ctx context.Context, in meta.RecordPut) (meta.RecordRow, error) {
	st, err := m.store()
	if err != nil {
		return meta.RecordRow{}, err
	}
	session, seat, epoch, err := m.writer()
	if err != nil {
		return meta.RecordRow{}, err
	}
	r, err := st.Put(ctx, record.PutRequest{
		ID: in.ID, IfVersion: in.IfVersion, Kind: in.Kind, Project: in.Project,
		Body: in.Body, Fields: in.Fields,
		Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		return meta.RecordRow{}, err
	}
	return rowOf(r), nil
}

// Get reads one record, at its current version or at an explicit one.
//
// VERSION ZERO MEANS "CURRENT" rather than "version zero", which is section
// 21's zero meaning "nothing was said" applied to a number. There is no version
// 0 in the store, so the value is free to carry that meaning.
func (m *mcpCaller) Get(ctx context.Context, id string, version uint64) (meta.RecordRow, error) {
	st, err := m.store()
	if err != nil {
		return meta.RecordRow{}, err
	}
	var r record.Record
	if version == 0 {
		r, err = st.Get(ctx, id)
	} else {
		r, err = st.GetVersion(ctx, id, version)
	}
	if err != nil {
		return meta.RecordRow{}, err
	}
	return rowOf(r), nil
}

// Query answers by project and kind, and then by field.
//
// ⛔ IT GOES THROUGH `(*Store).Find`, AND ROUTING IT THROUGH `(*Store).Query`
// MADE THE TOOL ANSWER NOTHING UNLESS BOTH FILTERS WERE GIVEN. Query's SQL is
// `WHERE r.project = ? AND r.kind = ?` - literal equality on both - so an
// empty filter was not a wildcard there, it was a value no record can hold,
// because Put refuses to write one. Measured against the live door: `project`
// alone answered 0 rows where the terminal answered 3, `kind` alone 0 against
// 2, and neither 0 against the whole census. Only both-supplied agreed.
//
// ⛔ AND NOTHING LOOKED WRONG, WHICH IS WHY IT SURVIVED. record_query's own
// description says "an empty result means nothing matched - it is an answer,
// not a failure", so the surface pre-told its reader to accept the defect.
//
// Find switches over all eight shapes and is what the CLI has always used -
// which is why the two surfaces disagreed: one of them was already right.
//
// ⛔ THE FIELD FILTER STAYS HERE RATHER THAN MOVING INTO Find's PREDICATE, and
// the reason is the SHAPE of the argument rather than a preference. Find takes
// ONE field and value; this takes a map, and a map has no first element - so
// choosing which key went into the SQL would make the query plan depend on Go
// map iteration order. Filtering the whole map here is deterministic and
// answers the same question. ⛔ IT READS THE MATCHING SET FIRST, so a field
// filter with no project and no kind is a full read - a real cost, stated
// rather than hidden.
func (m *mcpCaller) Query(ctx context.Context, project, kind string, fields map[string]string) ([]meta.RecordRow, error) {
	st, err := m.store()
	if err != nil {
		return nil, err
	}
	recs, err := st.Find(ctx, record.QueryFilter{Project: project, Kind: kind})
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return rowsOf(recs), nil
	}
	kept := make([]record.Record, 0, len(recs))
	for _, r := range recs {
		match := true
		for k, v := range fields {
			if got, ok := r.Fields[k]; !ok || got != v {
				match = false
				break
			}
		}
		if match {
			kept = append(kept, r)
		}
	}
	return rowsOf(kept), nil
}

func (m *mcpCaller) History(ctx context.Context, id string) ([]meta.RecordRow, error) {
	st, err := m.store()
	if err != nil {
		return nil, err
	}
	recs, err := st.History(ctx, id)
	if err != nil {
		return nil, err
	}
	return rowsOf(recs), nil
}

// Link and Unlink both need a seat, because an edge is a claim about two
// records and section 39 wants every claim attributable.
func (m *mcpCaller) Link(ctx context.Context, from, to, kind string) error {
	st, err := m.store()
	if err != nil {
		return err
	}
	if _, _, _, err := m.writer(); err != nil {
		return err
	}
	return st.Link(ctx, from, kind, to)
}

func (m *mcpCaller) Unlink(ctx context.Context, from, to, kind string) error {
	st, err := m.store()
	if err != nil {
		return err
	}
	if _, _, _, err := m.writer(); err != nil {
		return err
	}
	return st.Unlink(ctx, from, kind, to)
}

func (m *mcpCaller) Refs(ctx context.Context, id string) ([]meta.RecordRef, error) {
	st, err := m.store()
	if err != nil {
		return nil, err
	}
	res, err := st.Refs(ctx, record.RefsRequest{ID: id})
	if err != nil {
		return nil, err
	}
	// Truncated is carried on every row rather than beside the list, for
	// DECISION 1's reason: a row copied out of its answer must not silently
	// lose the fact that the answer was incomplete.
	out := make([]meta.RecordRef, 0, len(res.Refs))
	for _, r := range res.Refs {
		out = append(out, meta.RecordRef{
			ID: r.ID, Kind: r.Kind, Title: r.Title,
			Type: r.Type, Depth: r.Depth, Via: r.Via,
		})
	}
	return out, nil
}

// Brief answers section 39's twelve sections, and says whether the project is
// there at all.
//
// ⛔ THIS IS THE SURFACE B76 IS ABOUT, AND IT IS FIXED HERE RATHER THAN ONLY IN
// THE CLI. `rig brief <a project that does not exist>` exits 0 and reports
// "sections 1-4 computed: true", so a typo in a slug is indistinguishable from a
// project with no work - and an agent resuming on the wrong slug is told, in
// rig's own voice, that there is nothing to do. The store cannot answer it
// alone, so the existence question is asked separately and carried as its own
// field rather than inferred from an empty brief.
func (m *mcpCaller) Brief(ctx context.Context, project string) (meta.BriefAnswer, error) {
	st, err := m.store()
	if err != nil {
		return meta.BriefAnswer{}, err
	}
	known, err := projectExists(ctx, st, project)
	if err != nil {
		return meta.BriefAnswer{}, err
	}
	if !known {
		// ⛔ NOT AN ERROR. "This project does not exist" is an ANSWER to the
		// question asked, and a caller listing candidate slugs needs to
		// distinguish it from a failure to look.
		return meta.BriefAnswer{Found: false, Project: project}, nil
	}
	b, err := st.Brief(ctx, project)
	if err != nil {
		return meta.BriefAnswer{}, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return meta.BriefAnswer{}, fmt.Errorf("rendering the brief: %w", err)
	}
	return meta.BriefAnswer{Found: true, Project: project, JSON: raw}, nil
}

// projectExists asks whether anything at all is filed under this slug.
//
// ⛔ ANY RECORD COUNTS, NOT JUST A `project` RECORD. A project whose `project`
// record was never written but which holds ninety work items exists by every
// meaning a caller has - and rig's own store is exactly that shape for most of
// its history. Keying existence on one privileged kind would report the live
// project as absent, which is the failure this function exists to prevent
// arriving through its own fix.
//
// ⛔ AND IT ARRIVED ANYWAY, WITH THE PRIVILEGED KIND SPELLED "". This asked
// `(*Store).Query(ctx, project, "")`, whose SQL matches `r.kind = ?`
// literally; Put refuses an empty kind, so no record could ever match and this
// returned false for EVERY slug ever passed to it. `Brief` short-circuits on
// it, so `project_brief` - section 39's resume mechanism and section 9's A6,
// the one tool behind "use rig to work on rig" - answered `{"Found":false}`
// for every project in the store, and looked correct doing it, because "this
// project does not exist" is a well-formed answer.
//
// ⛔ THE DOC COMMENT ABOVE IS OLDER THAN THE DEFECT AND DESCRIBED IT EXACTLY.
// Naming a failure is not preventing it; `Find` is what prevents it, because
// there the empty filter is a wildcard by construction rather than by
// intention.
func projectExists(ctx context.Context, st *record.Store, project string) (bool, error) {
	if project == "" {
		return false, nil
	}
	recs, err := st.Find(ctx, record.QueryFilter{Project: project})
	if err != nil {
		return false, err
	}
	return len(recs) > 0, nil
}

func (m *mcpCaller) Step(ctx context.Context, in meta.ProgressStep) (meta.RecordRow, error) {
	st, err := m.store()
	if err != nil {
		return meta.RecordRow{}, err
	}
	session, seat, epoch, err := m.writer()
	if err != nil {
		return meta.RecordRow{}, err
	}
	r, err := st.Step(ctx, record.StepRequest{
		Item: in.Item, State: in.State, Note: in.Note, Project: in.Project,
		Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		return meta.RecordRow{}, err
	}
	return rowOf(r), nil
}

// ---- B77: full control, through the agent's own door ----------------------
//
// ⛔ THEY GO THROUGH `writer()` LIKE EVERY OTHER WRITE, AND THAT IS NOT A
// PERMISSION CHECK. "Everybody" is Boris's word and section 42's default is
// "absolutely without restrictions" - nothing here asks who wrote the record or
// whether this seat owns it. What `writer()` establishes is WHO ACTED, so the
// withdrawal can be argued with; a retraction nobody can attribute is a fact
// with no author, which is the property this store exists to keep.

func retractionRow(r record.Retraction) meta.RecordRetraction {
	return meta.RecordRetraction{
		ID: r.ID, Reason: r.Reason, ReplacedBy: r.ReplacedBy,
		Seat: r.Prov.Seat, Session: r.Prov.Session,
		Written: r.Prov.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Already: r.Already,
	}
}

func edgeRows(in []record.Edge) []meta.RecordEdge {
	out := make([]meta.RecordEdge, 0, len(in))
	for _, e := range in {
		out = append(out, meta.RecordEdge{Src: e.Src, Type: e.Type, Dst: e.Dst})
	}
	return out
}

func (m *mcpCaller) Retract(ctx context.Context, id, reason string) (meta.RecordRetraction, error) {
	st, err := m.store()
	if err != nil {
		return meta.RecordRetraction{}, err
	}
	session, seat, epoch, err := m.writer()
	if err != nil {
		return meta.RecordRetraction{}, err
	}
	got, err := st.Retract(ctx, record.RetractRequest{
		ID: id, Reason: reason, Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		return meta.RecordRetraction{}, err
	}
	return retractionRow(got), nil
}

func (m *mcpCaller) Delete(ctx context.Context, id string, dryRun bool) (meta.RecordDeletion, error) {
	st, err := m.store()
	if err != nil {
		return meta.RecordDeletion{}, err
	}
	// ⛔ A DRY RUN IS ATTRIBUTED TOO. It writes nothing, and it is the step an
	// agent takes immediately before the destructive one; a door that cannot
	// name who is asking cannot name who then deleted.
	if _, _, _, err := m.writer(); err != nil {
		return meta.RecordDeletion{}, err
	}
	got, err := st.Delete(ctx, record.DeleteRequest{ID: id, DryRun: dryRun})
	if err != nil {
		return meta.RecordDeletion{}, err
	}
	return meta.RecordDeletion{
		ID: got.ID, Versions: got.Versions,
		Edges: edgeRows(got.Edges), DryRun: got.DryRun,
	}, nil
}

func (m *mcpCaller) Replace(ctx context.Context, old, replacement, reason string) (meta.RecordReplacement, meta.RecordRetraction, error) {
	st, err := m.store()
	if err != nil {
		return meta.RecordReplacement{}, meta.RecordRetraction{}, err
	}
	session, seat, epoch, err := m.writer()
	if err != nil {
		return meta.RecordReplacement{}, meta.RecordRetraction{}, err
	}
	got, err := st.Replace(ctx, record.ReplaceRequest{
		Old: old, New: replacement, Reason: reason,
		Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		return meta.RecordReplacement{}, meta.RecordRetraction{}, err
	}
	return meta.RecordReplacement{
		Old: got.Old, New: got.New,
		Moved: edgeRows(got.Moved), Merged: edgeRows(got.Merged),
		Dropped: edgeRows(got.Dropped),
	}, retractionRow(got.Retraction), nil
}
