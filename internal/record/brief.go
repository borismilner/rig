package record

import (
	"fmt"
	"sort"
	"time"
)

// DefaultNextUpN is how many work items a brief surfaces as "next up" when the
// project record does not override it. Section 39: "Default 5."
const DefaultNextUpN = 5

// Brief is the derived answer to "what is going on here".
//
// EVERY FIELD IS COMPUTED AT READ TIME AND NOTHING HERE IS STORED. Section 39
// is explicit that auto-completion is derived and never stamped, and the same
// argument covers the whole brief: a stamp with no witness is asserted rather
// than evidenced, and a derivation cannot go stale because it is recomputed
// every time. This is also why no seat writes a sentence of prose to produce
// it - that is the acceptance test, not a style preference.
type Brief struct {
	Project string

	// Open is every active item that is not done, with its latest step.
	// ⛔ DISJOINT FROM NextUp BY CONSTRUCTION - section 39's defect D2: an item
	// appearing in both lists got two incompatible rules for how its notes
	// render. Every item has exactly one rendering because it is in exactly
	// one list.
	Open []ItemState

	// NextUp is the first NextUpN items in execution order.
	NextUp []ItemState

	// Blocked is what cannot start, and on whom.
	Blocked []Blockage

	// Cycles are the blocks cycles, each naming its items.
	//
	// DETECTED, REPORTED, ORDERED AROUND, NEVER RESOLVED. rig does not pick an
	// edge to break: choosing which blocks edge is the wrong one is a
	// judgement about the work, which is domain logic and section 29's first
	// non-goal. A cyclic backlog is a real state of a real project and it is
	// one Boris would want SURFACED.
	Cycles [][]string

	// CoarseCitations is the count of citations that resolve only to a whole
	// section rather than to one record.
	//
	// RULED BY BORIS 2026-09-16: the migration imports all 4,206 citations and
	// flags the coarse ones rather than dropping them. He accepted the named
	// risk - "a flagged count that nobody ever burns down" - on the condition
	// that the brief SURFACES it. So this field is not decoration, and a brief
	// that omits it has not implemented his ruling.
	CoarseCitations int
}

// ItemState is a work item and the last thing that happened to it.
type ItemState struct {
	ID    string
	Title string

	// State is the latest step's state, or "" when the stream is empty.
	// An active item with no steps has been picked up and not yet reported on.
	State string

	// Since is when that step was recorded. Zero when there are no steps.
	Since time.Time

	// Note is the latest step's own words.
	Note string
}

// Age is how long the item has been sitting at its latest step.
//
// Section 39: "a stale step beside a live session is the signal that a seat is
// stuck." Zero for an item with no steps, because "infinitely stale" would
// sort it above every real signal.
func (i ItemState) Age(asOf time.Time) time.Duration {
	if i.Since.IsZero() {
		return 0
	}
	return asOf.Sub(i.Since)
}

// Blockage is one item and everything it is waiting on.
type Blockage struct {
	Item      string
	Title     string
	BlockedBy []string
}

// Brief derives the answer to "what is going on here" for one project.
func (s *Store) Brief(project string) (Brief, error) {
	if project == "" {
		return Brief{}, fmt.Errorf("record: a brief needs a project")
	}
	b := Brief{Project: project}

	items, err := s.Query(project, "work-item")
	if err != nil {
		return Brief{}, err
	}
	latest, err := s.latestSteps(project)
	if err != nil {
		return Brief{}, err
	}

	// THE ACTIVE SET, AND BOTH HALVES OF THE PREDICATE ARE REQUIRED.
	//
	// Section 39 is explicit that completion lives in the progress stream and
	// NOT in `status`, so `status == active` on its own keeps a finished item
	// in next-up forever and ships a list that never empties. That was defect
	// D5 and this is the corrected reading.
	active := map[string]ItemState{}
	for _, it := range items {
		if it.Fields["status"] != "active" {
			continue
		}
		st := ItemState{ID: it.ID, Title: it.Fields["title"]}
		if step, ok := latest[it.ID]; ok {
			st.State = step.Fields["state"]
			st.Since = step.Prov.CreatedAt
			st.Note = step.Body
			if st.State == "done" {
				continue
			}
		}
		active[it.ID] = st
	}

	edges, err := s.blocksAmong(active)
	if err != nil {
		return Brief{}, err
	}

	order, cycles := topoSort(active, edges)
	b.Cycles = cycles

	n := s.nextUpN(project)
	for i, id := range order {
		if i < n {
			b.NextUp = append(b.NextUp, active[id])
		} else {
			b.Open = append(b.Open, active[id])
		}
	}

	// WHAT IS BLOCKED, AND ON WHOM.
	for id, st := range active {
		var on []string
		for src, dsts := range edges {
			for _, d := range dsts {
				if d == id {
					on = append(on, src)
				}
			}
		}
		if len(on) > 0 {
			sort.Strings(on)
			b.Blocked = append(b.Blocked, Blockage{Item: id, Title: st.Title, BlockedBy: on})
		}
	}
	sort.Slice(b.Blocked, func(i, j int) bool { return b.Blocked[i].Item < b.Blocked[j].Item })

	if b.CoarseCitations, err = s.coarseCitations(project); err != nil {
		return Brief{}, err
	}
	return b, nil
}

// nextUpN reads the project's override, falling back to the default.
func (s *Store) nextUpN(project string) int {
	rec, err := s.Get(project)
	if err != nil {
		return DefaultNextUpN
	}
	var n int
	if _, err := fmt.Sscanf(rec.Fields["next_up_n"], "%d", &n); err != nil || n <= 0 {
		return DefaultNextUpN
	}
	return n
}

// blocksAmong returns the blocks edges with BOTH ends in the active set.
//
// Restricted deliberately: an edge from a done item is not a live dependency,
// and carrying it would put finished work back into the ordering.
func (s *Store) blocksAmong(active map[string]ItemState) (map[string][]string, error) {
	out := map[string][]string{}
	for id := range active {
		dsts, err := s.LinksFrom(id, LinkBlocks)
		if err != nil {
			return nil, err
		}
		for _, d := range dsts {
			if _, ok := active[d]; ok {
				out[id] = append(out[id], d)
			}
		}
	}
	return out, nil
}

// topoSort orders the active set so that a blocker comes before what it blocks,
// and returns every cycle it could not order.
//
// ⛔ THE CYCLE IS REPORTED AND THE REST IS STILL ORDERED. A derivation that
// refused to answer because the graph has a cycle would be the other failure
// section 39 names - "swallowing it silently is the failure; so is refusing to
// answer." Items in a cycle are appended after the orderable ones, in a stable
// order, so the brief is still usable while the cycle is visible.
func topoSort(active map[string]ItemState, edges map[string][]string) ([]string, [][]string) {
	indeg := map[string]int{}
	for id := range active {
		indeg[id] = 0
	}
	for _, dsts := range edges {
		for _, d := range dsts {
			indeg[d]++
		}
	}

	var ready []string
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	var order []string
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)

		var freed []string
		for _, d := range edges[id] {
			indeg[d]--
			if indeg[d] == 0 {
				freed = append(freed, d)
			}
		}
		sort.Strings(freed)
		ready = append(ready, freed...)
		sort.Strings(ready)
	}

	if len(order) == len(active) {
		return order, nil
	}

	// WHAT IS LEFT IS EXACTLY THE ITEMS IN OR DOWNSTREAM OF A CYCLE.
	remaining := map[string]bool{}
	for id := range active {
		remaining[id] = true
	}
	for _, id := range order {
		delete(remaining, id)
	}
	cycles := stronglyConnected(remaining, edges)

	rest := make([]string, 0, len(remaining))
	for id := range remaining {
		rest = append(rest, id)
	}
	sort.Strings(rest)
	return append(order, rest...), cycles
}

// stronglyConnected finds every cycle among the given nodes, by Tarjan.
//
// A COMPONENT OF ONE IS NOT A CYCLE and is dropped: those are the items merely
// DOWNSTREAM of a cycle, which are stuck but are not themselves the problem.
// Naming them in the cycle report would send a reader to the wrong edge. A
// self-loop cannot appear here because Link refuses one.
func stronglyConnected(nodes map[string]bool, edges map[string][]string) [][]string {
	var (
		index   = map[string]int{}
		low     = map[string]int{}
		onStack = map[string]bool{}
		stack   []string
		next    int
		out     [][]string
		strong  func(string)
		ordered = make([]string, 0, len(nodes))
	)
	for id := range nodes {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)

	strong = func(v string) {
		index[v] = next
		low[v] = next
		next++
		stack = append(stack, v)
		onStack[v] = true

		succ := append([]string(nil), edges[v]...)
		sort.Strings(succ)
		for _, w := range succ {
			if !nodes[w] {
				continue
			}
			if _, seen := index[w]; !seen {
				strong(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}

		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			if len(comp) > 1 {
				sort.Strings(comp)
				out = append(out, comp)
			}
		}
	}

	for _, v := range ordered {
		if _, seen := index[v]; !seen {
			strong(v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
