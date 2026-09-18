package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The window's project and case surface (PLAN.md section 11, requirements 6
// and 7). Boris, 2026-09-17: "project/case management tab in which I'll be
// able to see project/cases managed in `rig` with all the most important and
// relevant and useful details."
//
// ⛔ IT READS ONCE AND DOES NOT SUBSCRIBE, AND THAT IS HIS RULING RATHER THAN A
// SHORTCUT. Requirement 8 wants real-time, and he deferred it in the same
// breath: "We can have the data displayed without live updates until the
// mechanism is ready." The mechanism is section 5h's bus at M13. So there is
// deliberately no watcher and no emitted event here - a poll dressed as an
// event stream is the trap section 11 names, because every consumer written
// against it would believe it was event-driven and nothing would say otherwise
// the day the bus lands.

// SectionStatus is one of the brief's eleven sections and what it can say.
//
// ⛔ THIS IS THE FIELD THAT MAKES EVERY OTHER ONE HONEST, and it is not
// optional decoration for the view. Seven of the eleven sections are NOT
// COMPUTED today - the standards register does not exist, the must-read gate is
// unbuilt on both halves, and the projection is unwritten. Without this, an
// empty `blocked` list and an unbuilt `drift` list render identically, and a
// reader concludes this project has no drift when the truth is that rig cannot
// yet know. The proto says it outright: "A caller that renders a section
// without reading its status is the failure this field exists to prevent."
type SectionStatus struct {
	Section string `json:"section"`
	State   string `json:"state"`
	Reason  string `json:"reason"`
}

// Item is one work item as the brief sees it.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
	// Unix nanoseconds, or 0 when the item has never been stepped. The view
	// formats it; a pre-formatted age here would be wrong the moment the
	// window sat open, which is exactly what a deferred-live-update surface
	// must not do.
	SinceUnixNano int64  `json:"sinceUnixNano"`
	Note          string `json:"note"`

	// ⛔ WHAT MAKES A ROW READABLE, AND IT REACHED THE WINDOW FOR THE FIRST
	// TIME ON 2026-09-17. The store computed all of these and the daemon's
	// itemToWire dropped them, so every row here was a truncated title with
	// nothing behind it - which is exactly what Boris called not user
	// friendly, and why clicking a row could only ever show the same line
	// again.
	DescriptionShort string `json:"descriptionShort"`
	Priority         string `json:"priority"`

	// The RECORD's status (`idea`, `active`), NOT the latest step's State.
	Status string `json:"status"`

	Owner string   `json:"owner"`
	Tags  []string `json:"tags"`

	TargetDate string `json:"targetDate"`
	Semver     string `json:"semver"`

	// `task`, `bug`, `idea`, or empty for untyped. The row draws an icon for
	// it; an unknown value draws no icon and renders as its own word, because
	// the set is open.
	ItemType string `json:"itemType"`
}

// Blocked is an item that cannot proceed, with what is holding it.
type Blocked struct {
	Item     string    `json:"item"`
	Title    string    `json:"title"`
	Blockers []Blocker `json:"blockers"`
}

// Blocker is one thing holding an item up, WITH ITS OWN STATE.
//
// The state is carried rather than dropped because "blocked by B12" and
// "blocked by B12, which is itself blocked" are different situations for a
// reader deciding what to pick up, and the wire already knows which.
type Blocker struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// Note is a project note, carrying its priority and what it is about.
type Note struct {
	ID string `json:"id"`

	// Title is what the document called it. ⛔ THE WIRE DID NOT CARRY THIS
	// UNTIL 2026-09-18, so this window listed every note by the first line of
	// its BODY - a 1,951-byte paragraph in one of Boris's, and nothing at all
	// in the one whose body is empty. B97.
	Title string `json:"title"`

	Body     string `json:"body"`
	Priority string `json:"priority"`
	About    string `json:"about"`
}

// ProjectionHealth is section 39's three projection rows. Every one is NOT
// COMPUTED today; read the matching SectionStatus before rendering any of
// them.
//
// Named for the projection rather than called Health, because service.go's
// Health is the window's own reachability of the daemon. Two things called
// Health on one service is how a view binds the wrong one.
type ProjectionHealth struct {
	ProjectionBehindCommits uint64 `json:"projectionBehindCommits"`
	PendingEntries          uint64 `json:"pendingEntries"`
	LocalOnly               bool   `json:"localOnly"`
	LocalOnlyReason         string `json:"localOnlyReason"`
}

// Brief is one project or case, flattened for the tab.
type Brief struct {
	Project string `json:"project"`

	// `project` or `case`, and it decides what the rest means: a case has no
	// semver and its status reads open/resolved rather than idea/active.
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Semver string `json:"semver"`

	Open   []Item `json:"open"`
	NextUp []Item `json:"nextUp"`

	Blocked []Blocked  `json:"blocked"`
	Cycles  [][]string `json:"cycles"`

	Notes     []Note `json:"notes"`
	CaseNotes []Note `json:"caseNotes"`

	CoarseCitations uint64 `json:"coarseCitations"`

	Health   ProjectionHealth `json:"health"`
	Sections []SectionStatus  `json:"sections"`
}

// ProjectRef is one project or case the store holds.
type ProjectRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

// Projects answers what the store actually holds, rather than what the window
// was compiled believing.
//
// ⛔ IT ASKS WITH AN EMPTY PROJECT ON PURPOSE, and if the daemon refuses that,
// the refusal is REPORTED rather than papered over with a hard-coded "rig".
// There is no verb on this wire that enumerates projects - the nine record
// verbs are put, get, query, history, link, unlink, refs, progress.step and
// project.brief, and none of them answers "what is in here". Asking
// `record.query` for every `project` record is the closest honest question,
// and a window that hard-coded one name would be lying the day a second
// project exists.
func (RigService) Projects() ([]ProjectRef, error) {
	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.RecordQueryResponse{}
	req := &rigv1.RecordQueryRequest{Kind: "project"}
	if err := c.Call(ctx, "rig.record.query", req, resp); err != nil {
		return nil, fmt.Errorf("the store could not be asked which projects it holds: %w", err)
	}

	out := make([]ProjectRef, 0, len(resp.GetRecords()))
	for _, r := range resp.GetRecords() {
		f := r.GetFields()
		out = append(out, ProjectRef{
			ID:     r.GetProject(),
			Title:  f["title"],
			Kind:   f["kind"],
			Status: f["status"],
		})
	}
	// A stable order, and the view must never re-sort under a reader's hand
	// (section 11's rail rule, and the same argument applies to a list).
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Brief reads one project's brief. Named rather than discovered, because
// Projects is what discovers.
func (RigService) Brief(project string) (Brief, error) {
	var b Brief
	if project == "" {
		return b, fmt.Errorf("a brief needs a project to be about")
	}

	c, err := client.Connect()
	if err != nil {
		return b, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.ProjectBriefResponse{}
	req := &rigv1.ProjectBriefRequest{Project: project}
	if err := c.Call(ctx, "rig.project.brief", req, resp); err != nil {
		return b, err
	}
	return briefFrom(resp), nil
}

func briefFrom(r *rigv1.ProjectBriefResponse) Brief {
	b := Brief{
		Project:         r.GetProject(),
		Kind:            r.GetKind(),
		Title:           r.GetTitle(),
		Status:          r.GetStatus(),
		Semver:          r.GetSemver(),
		Open:            itemsFrom(r.GetOpen()),
		NextUp:          itemsFrom(r.GetNextUp()),
		Cycles:          cyclesFrom(r.GetCycles()),
		Notes:           notesFrom(r.GetNotes()),
		CaseNotes:       notesFrom(r.GetCaseNotes()),
		CoarseCitations: r.GetCoarseCitations(),
		Health: ProjectionHealth{
			ProjectionBehindCommits: r.GetHealth().GetProjectionBehindCommits(),
			PendingEntries:          r.GetHealth().GetPendingEntries(),
			LocalOnly:               r.GetHealth().GetLocalOnly(),
			LocalOnlyReason:         r.GetHealth().GetLocalOnlyReason(),
		},
	}

	b.Blocked = make([]Blocked, 0, len(r.GetBlocked()))
	for _, x := range r.GetBlocked() {
		who := make([]Blocker, 0, len(x.GetBlockers()))
		for _, k := range x.GetBlockers() {
			who = append(who, Blocker{ID: k.GetId(), Title: k.GetTitle(), State: stepStateName(k.GetState())})
		}
		b.Blocked = append(b.Blocked, Blocked{Item: x.GetItem(), Title: x.GetTitle(), Blockers: who})
	}

	b.Sections = make([]SectionStatus, 0, len(r.GetSections()))
	for _, s := range r.GetSections() {
		b.Sections = append(b.Sections, SectionStatus{
			Section: sectionName(s.GetSection()),
			State:   sectionStateName(s.GetState()),
			Reason:  s.GetReason(),
		})
	}
	return b
}

func itemsFrom(in []*rigv1.ItemState) []Item {
	out := make([]Item, 0, len(in))
	for _, i := range in {
		out = append(out, Item{
			ID:               i.GetId(),
			Title:            i.GetTitle(),
			State:            stepStateName(i.GetState()),
			SinceUnixNano:    i.GetSinceUnixNano(),
			Note:             i.GetNote(),
			DescriptionShort: i.GetDescriptionShort(),
			Priority:         i.GetPriority(),
			Status:           i.GetStatus(),
			Owner:            i.GetOwner(),
			Tags:             i.GetTags(),
			TargetDate:       i.GetTargetDate(),
			Semver:           i.GetSemver(),
			ItemType:         i.GetItemType(),
		})
	}
	return out
}

func notesFrom(in []*rigv1.BriefNote) []Note {
	out := make([]Note, 0, len(in))
	for _, n := range in {
		out = append(out, Note{
			ID:       n.GetId(),
			Title:    n.GetTitle(),
			Body:     n.GetBody(),
			Priority: n.GetPriority(),
			About:    n.GetAbout(),
		})
	}
	return out
}

func cyclesFrom(in []*rigv1.Cycle) [][]string {
	out := make([][]string, 0, len(in))
	for _, c := range in {
		out = append(out, c.GetItems())
	}
	return out
}

// The three name mappings below turn enums into strings the view can read.
//
// ⛔ THE DEFAULT ARM NAMES THE NUMBER RATHER THAN GUESSING. A future enum
// member rendered as "unspecified" would be a new state reported as a known
// one, which is the wire-that-lies shape this repository refuses elsewhere.
func stepStateName(s rigv1.StepState) string {
	switch s {
	case rigv1.StepState_STEP_STATE_STARTED:
		return "started"
	case rigv1.StepState_STEP_STATE_BLOCKED:
		return "blocked"
	case rigv1.StepState_STEP_STATE_DONE:
		return "done"
	case rigv1.StepState_STEP_STATE_UNSPECIFIED:
		return "not stepped"
	default:
		return fmt.Sprintf("unknown step state %d", int32(s))
	}
}

func sectionStateName(s rigv1.SectionState) string {
	switch s {
	case rigv1.SectionState_SECTION_STATE_COMPUTED:
		return "computed"
	case rigv1.SectionState_SECTION_STATE_NOT_COMPUTED:
		return "not computed"
	case rigv1.SectionState_SECTION_STATE_WITHHELD_BY_VIEW:
		return "withheld"
	case rigv1.SectionState_SECTION_STATE_UNSPECIFIED:
		return "unspecified"
	default:
		return fmt.Sprintf("unknown section state %d", int32(s))
	}
}

func sectionName(s rigv1.BriefSection) string {
	if n, ok := rigv1.BriefSection_name[int32(s)]; ok {
		return trimSectionPrefix(n)
	}
	return fmt.Sprintf("unknown section %d", int32(s))
}

func trimSectionPrefix(n string) string {
	const p = "BRIEF_SECTION_"
	if len(n) > len(p) && n[:len(p)] == p {
		return n[len(p):]
	}
	return n
}
