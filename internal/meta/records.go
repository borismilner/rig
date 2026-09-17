package meta

import (
	"context"

	"github.com/boris-milner/rig/internal/kernel"
)

// Records is the continuity record, reachable from the agent surface.
//
// ⛔ IT EXISTS BECAUSE THE RECORD HAD NO AGENT ROUTE AT ALL, AND THAT MADE rig
// STRICTLY LESS CAPABLE THAN THE THING IT REPLACES. Measured 2026-09-17 by the
// section 09 A0 survey, `[ran it]` on a live estate: `tools/call` for `record`
// and for `brief` both answered `-32602 unknown tool`, and `query` with subject
// `records` was byte-identical to an unrecognised subject. Seven tools and not
// one reached `record.put`, `record.get`, `record.query`, `record.history`,
// `record.link`, `record.unlink`, `record.refs`, `project.brief` or
// `progress.step`.
//
// Section 39 says rig is the source of truth and the logbook is what it
// replaces. Section 09's A1-A6 say an agent's working notes ARE records in rig,
// "so that nothing is ever lost". ⛔ **An agent with a filesystem tool reads and
// writes the logbook today; an agent with rig's MCP server could not touch
// rig's record at all.** That is clause A - "is anything INFERIOR to using the
// logbook?" - failing on its own terms, and it is why this interface is not an
// enhancement.
//
// OPTIONAL, AND RESOLVED PER CALL BY TYPE ASSERTION - Roster's shape, which is
// itself EstateIdentity's. Third use of a pattern this tree already chose twice
// for this exact inversion. A surface with no record store under it is a real
// configuration: an UNNAMED estate keeps no continuity record by section 37's
// rule, so the daemon serving it satisfies every other interface here and not
// this one. Reporting UNAVAILABLE rather than refusing is what that case earns.
//
// ⛔ NO METHOD HERE TAKES A PRINCIPAL, FOR ROSTER'S REASON AND ONE MORE OF ITS
// OWN. The implementation is per-connection, built around the connection's
// occupancy token, so it already knows who is calling - and knowing that is the
// point rather than a convenience. Every record written through this interface
// carries the caller's SEAT, which is what B74 found missing: 565 records, one
// distinct seat, 561 distinct sessions, because every CLI `record put` opened a
// fresh unannounced connection and there was no key for section 42's per-agent
// policy to bind to. ⛔ **A record written through this door is attributable by
// construction. That is not a feature added beside the route; it is the reason
// the route is HERE and not on the invoke surface.**
type Records interface {
	// Put writes a record and returns it as stored, at its new version.
	//
	// THE SEAT IS NOT AN ARGUMENT AND MUST NEVER BECOME ONE. It comes from the
	// connection, so a caller cannot write a record under somebody else's name.
	// An argument would make attribution a claim; taking it from the occupancy
	// makes it a fact.
	Put(ctx context.Context, in RecordPut) (RecordRow, error)

	// Get reads one record at its current version, or at an explicit one.
	Get(ctx context.Context, id string, version uint64) (RecordRow, error)

	// Query answers by project and kind, either of which may be empty.
	//
	// ⛔ AN EMPTY RESULT IS AN ANSWER AND MUST ARRIVE AS ONE. The CLI's
	// purpose-written empty result exists because "no rows" and "this build
	// cannot ask" are different facts, and section 5k forbids a surface that
	// implies completeness it has not got.
	Query(ctx context.Context, project, kind string, fields map[string]string) ([]RecordRow, error)

	// History is every version of one record, oldest first.
	//
	// APPEND-ONLY IS THE POINT OF SECTION 39's RECORD: a superseded wording is
	// still the record of what was believed, and this is the surface that says
	// so.
	History(ctx context.Context, id string) ([]RecordRow, error)

	// Link and Unlink write and remove one typed edge between two records.
	Link(ctx context.Context, from, to, kind string) error
	Unlink(ctx context.Context, from, to, kind string) error

	// Refs answers which records point at this one, by edge kind.
	Refs(ctx context.Context, id string) ([]RecordRef, error)

	// Brief is section 39's twelve-section answer for one project.
	//
	// ⛔ IT RETURNS found=false FOR A PROJECT THAT DOES NOT EXIST, AND THE
	// SURFACE ABOVE MUST NOT FLATTEN THAT INTO AN EMPTY BRIEF. B76: the CLI
	// exits 0 on a typo and reports "sections 1-4 computed: true", so a project
	// that is absent and a project with no work are indistinguishable. Every
	// section of a brief is written to separate "nothing to report" from "this
	// build cannot answer"; the container owes the same distinction.
	Brief(ctx context.Context, project string) (BriefAnswer, error)

	// Step writes one progress entry against a work item.
	Step(ctx context.Context, in ProgressStep) (RecordRow, error)
}

// RecordPut is one write. Body and Fields are both optional and both are
// carried: section 39's grain is typed fields WITH prose beside them, never
// prose an agent has to parse back out.
type RecordPut struct {
	ID      string
	Project string
	Kind    string
	Body    string
	Fields  map[string]string

	// IfVersion is the compare-and-swap. Zero means "create, and refuse if it
	// exists"; a non-zero value means "replace exactly that version".
	//
	// ⛔ IT IS THE ONLY THING STANDING BETWEEN TWO AGENTS AND A LOST WRITE, and
	// two agents writing one project is the case this whole record exists for.
	IfVersion uint64
}

// ProgressStep is one step against a work item.
type ProgressStep struct {
	Item    string
	Project string
	State   string
	Note    string
}

// RecordRow is one record as a surface renders it.
//
// ⛔ Seat AND Session ARE ON THE ROW RATHER THAN IN AN ENVELOPE, which is
// DECISION 1's rule for the epoch applied to provenance. A reader holding one
// row can say who wrote it without holding the answer it arrived in, and a row
// copied out of its answer does not silently lose its author.
type RecordRow struct {
	ID      string
	Project string
	Kind    string
	Version uint64
	Body    string
	Fields  map[string]string

	// Seat is who wrote it. EMPTY IS A REAL AND HONEST VALUE: every record
	// written before this route existed has no seat, and rendering a blank
	// rather than inventing one is what lets B74's census stay measurable.
	Seat    string
	Session string
	Epoch   uint64
	Written string
}

// RecordRef is one record that points at the subject, directly or through
// another record in the same answer.
//
// ⛔ THE FIELDS MIRROR record.Ref EXACTLY AND MUST KEEP DOING SO. A surface
// type that drops one of them decides, silently, that a caller does not need
// it - and `Via` is the field that would go first, because it looks redundant.
// It is not: a project-scoped walk crosses foreign records and leaves them out
// of the answer, so `Via` can name an id the caller cannot look up in the same
// list, and blanking it removes the only explanation the answer carries for why
// it was truncated.
type RecordRef struct {
	ID    string
	Kind  string
	Title string

	// Type is the link type of the edge that reached this record.
	Type string

	// Depth is how many hops from the subject, starting at 1.
	Depth int

	// Via is the record this one points at - the subject at depth 1.
	Via string
}

// BriefAnswer is project.brief, with the one field the CLI does not have.
type BriefAnswer struct {
	// Found separates "this project has no work" from "this project does not
	// exist", which is B76 and is the argument the whole brief is built on.
	Found bool

	Project string

	// JSON is the brief as the store rendered it. ⛔ IT IS CARRIED WHOLE RATHER
	// THAN RE-TYPED HERE, deliberately: `briefSections()` is a HAND-KEPT list
	// whose own comment used to claim it was derived from the store's struct,
	// and two of its rows went stale behind that false claim. A second hand-kept
	// mirror of the same twelve sections in this package would be the same
	// defect with one more place to rot.
	JSON []byte
}

// records asks the invoker for the continuity record, if it can give one.
//
// The type assertion is where the optional half is resolved, per call rather
// than at construction - roster()'s reason exactly, and estate()'s before it.
func (s *Server) records() (Records, bool) {
	r, ok := s.invoker.(Records)
	if !ok {
		return nil, false
	}
	return r, true
}

// recordAnswer is every record tool's envelope.
//
// ⛔ IT READS THE CAPABILITY MAP FOR Partial EXACTLY AS rosterAnswer DOES, and
// for the same reason: section 5k forbids a surface that answers without saying
// what it could not see. A record tool that skipped this would render `[]` and
// make precisely the claim it had not checked, which is worse than the read
// costs, because it is a lie that looks like diligence.
func (s *Server) recordAnswer(who kernel.Principal, tool Tool) (Answer, error) {
	m, err := s.kernel.See(who).CapabilityMap(kernel.DepthPrograms)
	if err != nil {
		return Answer{}, err
	}
	return Answer{Tool: tool, Partial: partialOf(m.Programs...)}, nil
}

// recordTool is the shared body of all nine: envelope, availability, then the
// caller's own work.
//
// ONE HELPER RATHER THAN NINE COPIES OF THE SAME SIX LINES, because the thing
// being repeated is the UNAVAILABLE branch - and nine hand-copied branches is
// nine chances for one of them to quietly return an empty answer instead of
// saying it could not look. That is the failure mode section 5k is about.
func (s *Server) recordTool(who kernel.Principal, tool Tool, do func(Records, *Answer) error) (Answer, error) {
	out, err := s.recordAnswer(who, tool)
	if err != nil {
		return Answer{}, err
	}
	rc, ok := s.records()
	if !ok {
		out.Unavailable = append(out.Unavailable, unavailableRecord)
		return out, nil
	}
	if err := do(rc, &out); err != nil {
		return Answer{}, err
	}
	return out, nil
}

// unavailableRecord is what the nine tools report when there is no record store
// under this surface.
//
// ⛔ AN UNNAMED ESTATE IS THE REAL CASE, NOT A BROKEN ONE. Section 37 says the
// continuity record is per-estate state and an unnamed estate keeps none, so a
// daemon serving one satisfies Roster and Invoker and not this. Naming it here
// is what turns "the answer is empty" into "this estate has no record", which
// are different facts about the same bytes.
const unavailableRecord = "this estate's continuity record"

// allToolNames is every tool this server dispatches, in dispatch order.
//
// ⛔ IT IS DERIVED FROM NOTHING AND THAT IS STATED HONESTLY: this is a hand-kept
// list, exactly like `briefSections()`, whose comment once claimed it was
// derived from the store's struct and was believed - which is why two of its
// rows went stale. This comment makes the opposite claim, so
// TestEveryDispatchedToolIsNamed is what keeps it true rather than a sentence.
func allToolNames() []string {
	return []string{
		string(List), string(Describe), string(Invoke), string(Query),
		string(Announce), string(SetActivity), string(ListAgents),
		string(RecordPutTool), string(RecordGetTool), string(RecordQueryTool),
		string(RecordHistoryTool), string(RecordLinkTool), string(RecordUnlinkTool),
		string(RecordRefsTool), string(ProjectBriefTool), string(ProgressStepTool),
	}
}

// RecordAnswer carries whichever shape the tool that produced it returns.
//
// ⛔ THE FIELDS ARE NOT INTERCHANGEABLE AND AN EMPTY ONE IS NOT AN ABSENCE.
// `Rows` empty on a query means the query matched nothing; `Brief` nil on a
// project_brief means the project was not found, which `Found` states outright
// rather than leaving to be inferred from a nil. Section 5k's rule is that no
// surface may imply completeness, and a nil that could mean either is exactly
// that implication.
type RecordAnswer struct {
	// Row is put's, get's and step's single answer.
	Row *RecordRow `json:"row,omitempty"`

	// Rows is query's and history's set. EMPTY IS AN ANSWER.
	Rows []RecordRow `json:"rows,omitempty"`

	// Refs is refs'.
	Refs []RecordRef `json:"refs,omitempty"`

	// Brief is project_brief's, and Found is the field B76 is about.
	Brief *BriefAnswer `json:"brief,omitempty"`

	// Linked says an edge write took effect. A BOOL RATHER THAN AN EMPTY
	// ANSWER, because link and unlink otherwise return nothing at all and a
	// caller cannot tell success from a surface that did not run.
	Linked bool `json:"linked,omitempty"`
}

func (s *Server) recordPut(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordPutTool, func(rc Records, out *Answer) error {
		row, err := rc.Put(ctx, RecordPut{
			ID: r.RecordID, Project: r.Project, Kind: r.Kind,
			Body: r.Body, Fields: r.Fields, IfVersion: r.IfVersion,
		})
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Row: &row}
		return nil
	})
}

func (s *Server) recordGet(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordGetTool, func(rc Records, out *Answer) error {
		row, err := rc.Get(ctx, r.RecordID, r.Version)
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Row: &row}
		return nil
	})
}

func (s *Server) recordQuery(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordQueryTool, func(rc Records, out *Answer) error {
		rows, err := rc.Query(ctx, r.Project, r.Kind, r.Fields)
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Rows: rows}
		return nil
	})
}

func (s *Server) recordHistory(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordHistoryTool, func(rc Records, out *Answer) error {
		rows, err := rc.History(ctx, r.RecordID)
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Rows: rows}
		return nil
	})
}

func (s *Server) recordLink(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordLinkTool, func(rc Records, out *Answer) error {
		if err := rc.Link(ctx, r.From, r.To, r.LinkKind); err != nil {
			return err
		}
		out.Record = &RecordAnswer{Linked: true}
		return nil
	})
}

func (s *Server) recordUnlink(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordUnlinkTool, func(rc Records, out *Answer) error {
		if err := rc.Unlink(ctx, r.From, r.To, r.LinkKind); err != nil {
			return err
		}
		out.Record = &RecordAnswer{Linked: true}
		return nil
	})
}

func (s *Server) recordRefs(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, RecordRefsTool, func(rc Records, out *Answer) error {
		refs, err := rc.Refs(ctx, r.RecordID)
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Refs: refs}
		return nil
	})
}

func (s *Server) projectBrief(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, ProjectBriefTool, func(rc Records, out *Answer) error {
		b, err := rc.Brief(ctx, r.Project)
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Brief: &b}
		return nil
	})
}

func (s *Server) progressStep(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, ProgressStepTool, func(rc Records, out *Answer) error {
		row, err := rc.Step(ctx, ProgressStep{
			Item: r.Item, Project: r.Project, State: r.State, Note: r.Body,
		})
		if err != nil {
			return err
		}
		out.Record = &RecordAnswer{Row: &row}
		return nil
	})
}
