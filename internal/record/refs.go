package record

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// DefaultRefsDepth and MaxRefsDepth bound the reverse traversal.
//
// Section 39's R3.2, measured: depth 4 is 5.79 ms scoped at 100x and depth 5 is
// 16.33 ms, against R2.3's 20 ms TARGET. The cap was 6 and came down to 5 on
// that measurement.
//
// ⛔ R2.3's 20 ms IS AN INVENTED TARGET, NOT A MEASURED BUDGET, and the cap
// inherits that. The honest statement is the one the lead ruled: the cap is 5
// because depth 6 exceeds the current TARGET, and if the target moves the cap
// moves with it. Do not read 5 as a fact about the machine.
const (
	DefaultRefsDepth = 4
	MaxRefsDepth     = 5
)

// RefsRequest asks what points AT a record.
type RefsRequest struct {
	// ID is the record everything in the answer points at, directly or through
	// another record in the answer.
	ID string

	// Depth bounds the traversal. Zero means DefaultRefsDepth.
	//
	// ⛔ A DEPTH ABOVE MaxRefsDepth IS REFUSED BY NAME, NEVER CLAMPED. Section
	// 39's R3.2 says so in as many words, and the reason is this capability's
	// whole purpose: a caller that asked for 8, got 5, and was not told has a
	// partial answer that looks complete.
	Depth int

	// CrossProject opts INTO leaving the record's own project.
	//
	// ⛔ IT CONTROLS WHAT IS RETURNED, NOT WHAT IS WALKED, AND THAT IS A RULING
	// RATHER THAN AN IMPLEMENTATION CHOICE. It used to control both, and the
	// two meanings on one flag are what produced the defect below.
	//
	// THE DEFECT, DEMONSTRATED BEFORE IT WAS FIXED: the filter ran per hop,
	// before the visited mark and before the frontier append, so a foreign node
	// was not merely excluded - the whole subtree behind it was unreachable,
	// INCLUDING records in the subject's own project. The fixture is
	// C(rig) <-cites- B(standards) <-cites- A(rig): `refs C` answered 0 refs,
	// truncated=false, about a record its own project points at.
	//
	// THE RULING, and its reasoning, so nobody re-decides it from the cost
	// table alone:
	//
	//  1. The gate clause is "doesn't miss anything". A traversal that silently
	//     omits records in the subject's OWN project fails that clause, and
	//     flagging truncation alone tells the caller something is missing
	//     without telling them what.
	//  2. Correctness is free at the current scale - 78 records in nine
	//     disjoint two-node components - and the day it stops being free is a
	//     day with a measurement behind it.
	//  3. One flag, one meaning.
	//
	// ⛔ WHAT THE RULING COSTS, WRITTEN DOWN RATHER THAN DISCOVERED LATER.
	// Section 39's measured table is the UNSCOPED cost of a walk, and that is
	// now the cost of the default path too: at 100x, depth 4 is 307.3 ms
	// against 5.79 ms scoped, and depth 5 is 990.0 ms against 16.33 ms. The
	// frontier is no longer pruned; only the answer is. Section 39 still calls
	// the prune a performance requirement and that paragraph is owed an
	// amendment.
	CrossProject bool
}

// Ref is one record that points at the subject, and how it was reached.
type Ref struct {
	ID    string
	Kind  string
	Title string

	// Type is the link type of the edge that reached this record.
	Type string

	// Depth is how many hops from the subject, starting at 1.
	Depth int

	// Via is the record this one points at - the subject at depth 1.
	//
	// ⛔ IT CAN NAME A RECORD THAT IS NOT IN THIS ANSWER, and that is
	// deliberate. A project-scoped walk crosses foreign nodes and then leaves
	// them out of Refs, so a row reached through one cites an id the caller
	// cannot look up in the same list. Blanking it would hide the only
	// explanation the answer carries for why Truncated is true.
	Via string
}

// Refs is the answer to "what points at this".
type Refs struct {
	ID   string
	Refs []Ref

	// Truncated is true when there is more than this answer shows: either the
	// traversal stopped at the depth bound with somewhere still to go, or the
	// project scope kept a record that WAS walked out of the list.
	//
	// ⛔ THE SECOND SOURCE IS AS REQUIRED AS THE FIRST. A scoped answer that
	// dropped a foreign record and claimed completeness is this file's cardinal
	// failure one level down: the caller is told what points at the subject and
	// not told that something else does. The flag answers ONE question - is
	// there more I did not show you - and a filtered row is more that was not
	// shown.
	//
	// ⛔ RULED, plan/39 at rig 891b89f - NOT a reading of R3.2, which is the
	// state this comment used to be in. STORE-REQUIREMENTS.md R3.2 said "a
	// traversal that hits the cap returns truncated: true", which invited the
	// cap alone as the test. THE SPEC MOVED TO THE CODE, not the code to the
	// spec, and the requirement document is amended.
	//
	// THE DIFFERENCE IS THE WHOLE POINT OF THE FLAG. A walk that reaches depth
	// 4 of a bound of 4 and finds nothing beyond has returned a COMPLETE
	// answer. Flagging it truncated tells the caller its answer is partial when
	// it is whole, and "ask again with a bigger number" is then advice to spend
	// a traversal for nothing. The flag answers ONE question - is there more I
	// did not show you - and a cap is evidence for it only when the frontier
	// still leads somewhere.
	//
	// ⛔ A PARTIAL ANSWER THAT LOOKS COMPLETE IS THE FAILURE THIS WHOLE
	// CAPABILITY EXISTS TO PREVENT. R3.2 requires the flag for that reason: the
	// caller is answering "what does this touch", and silence about the horizon
	// is the one wrong answer it cannot recover from.
	Truncated bool

	// Cycles are the cycles among the records reached, each naming its items.
	// DETECTED, REPORTED, ORDERED AROUND, NEVER RESOLVED - the same rule the
	// brief follows, and rig does not pick an edge to break.
	Cycles [][]string
}

// Refs answers what points at a record, walking the reverse edges.
//
// ⛔ TWO GUARDS, AND THEY DO DIFFERENT JOBS. SAYING WHICH IS WHICH MATTERS,
// BECAUSE THIS COMMENT GOT IT WRONG FIRST AND A MUTATION CAUGHT IT.
//
// It claimed the visited set was the hang defence. It is NOT, in this
// implementation:
//
//   - THE DEPTH BOUND terminates the walk. The loop runs at most `depth` hops
//     whatever the graph does, so a cycle cannot spin here. Deleting the
//     visited-set guard and re-running a 3-cycle proved it: the traversal still
//     finished, which is how the wrong claim was found.
//   - THE VISITED SET dedups. Without it a cyclic or diamond graph revisits
//     nodes and the frontier grows multiplicatively at every hop - the answer
//     carries the same record several times, at several depths, and a caller
//     reading "what points at this" gets a list that is wrong rather than slow.
//
// The recursive-CTE hang is real and is a DIFFERENT implementation's problem: a
// recursive UNION dedups on (node, depth), depth keeps incrementing, so the
// dedup never fires and there is no bound to stop it. Measured by this seat at
// the lead's instruction - 0->1->2->0 ran past 20 seconds and was killed, exit
// 124. It is written here so nobody rewrites this walk as a CTE without putting
// a bound in it, not because that hang can happen in this function.
func (s *Store) Refs(ctx context.Context, r RefsRequest) (Refs, error) {
	if r.ID == "" {
		return Refs{}, errors.New("record: refs needs a record to point at")
	}
	depth := r.Depth
	if depth == 0 {
		depth = DefaultRefsDepth
	}
	if depth < 0 {
		return Refs{}, fmt.Errorf("record: refs depth %d is not a depth", depth)
	}
	if depth > MaxRefsDepth {
		return Refs{}, fmt.Errorf(
			"record: refs depth %d is above the cap of %d; ask for %d or fewer hops, "+
				"because a traversal silently clamped to the cap returns a partial answer "+
				"that looks complete", depth, MaxRefsDepth, MaxRefsDepth)
	}

	subject, err := s.Get(ctx, r.ID)
	if err != nil {
		return Refs{}, err
	}

	out := Refs{ID: r.ID}
	// visited is the DEDUP, not the termination guard - see above. The subject
	// is in it from the start, so an edge back to it is an edge into a visited
	// node: recorded for the cycle report, never followed again.
	visited := map[string]bool{r.ID: true}
	edges := map[string][]string{}
	frontier := []string{r.ID}

	for hop := 1; hop <= depth && len(frontier) > 0; hop++ {
		var next []string
		for _, node := range frontier {
			in, err := s.linksTo(ctx, node)
			if err != nil {
				return Refs{}, err
			}
			for _, e := range in {
				// THE EDGE IS RECORDED WHETHER OR NOT IT IS FOLLOWED, so a cycle
				// closing onto an already-visited node is still visible to the
				// cycle report below. Dropping it here is how a cycle becomes
				// invisible rather than reported.
				edges[e.src] = append(edges[e.src], node)
				if visited[e.src] {
					continue
				}
				rec, err := s.Get(ctx, e.src)
				if err != nil {
					return Refs{}, err
				}
				// ⛔ THE NODE IS MARKED AND FOLLOWED WHATEVER PROJECT IT IS IN.
				// The scope filter is three lines below, on the ANSWER. Moving
				// it up here - which is where it used to be - makes the whole
				// subtree behind a foreign node unreachable, including the
				// subject's own project, and that is the defect this ordering
				// exists to prevent. See RefsRequest.CrossProject.
				visited[e.src] = true
				next = append(next, e.src)

				if !r.CrossProject && rec.Project != subject.Project {
					// Walked through, kept out of the answer - and SAYING SO,
					// because a filtered answer that claims completeness is the
					// same defect the frontier prune was.
					out.Truncated = true
					continue
				}
				out.Refs = append(out.Refs, Ref{
					ID: e.src, Kind: rec.Kind, Title: rec.Fields["title"],
					Type: e.typ, Depth: hop, Via: node,
				})
			}
		}
		sort.Strings(next)
		frontier = next

		// ⛔ TRUNCATED MEANS THERE WAS SOMEWHERE STILL TO GO, NOT THAT THE
		// FRONTIER WAS NON-EMPTY, AND THE DIFFERENCE IS THE WHOLE FLAG.
		//
		// The first version of this said `len(frontier) > 0` at the bound, and
		// a four-node chain answered at depth 4 - the complete answer, every
		// node reached - reported itself partial. The last hop always leaves
		// the nodes it just visited on the frontier; whether the graph
		// continues past them is a different question. Found by the test for
		// this flag, which is the only reason it is not shipping: a truncation
		// flag that is always true is exactly as useless as one that is always
		// false, and it fails in the direction that looks careful.
		//
		// ⛔ IT IS OR-ED, NEVER ASSIGNED. The scope filter above can already
		// have set the flag, and a plain assignment here clears it - a walk
		// that dropped a foreign record and then found nothing past its
		// horizon would report itself complete, which is exactly the answer
		// this flag exists to make impossible.
		if hop == depth {
			more, err := s.moreBeyond(ctx, frontier, visited)
			if err != nil {
				return Refs{}, err
			}
			out.Truncated = out.Truncated || more
		}
	}

	sort.Slice(out.Refs, func(i, j int) bool {
		if out.Refs[i].Depth != out.Refs[j].Depth {
			return out.Refs[i].Depth < out.Refs[j].Depth
		}
		return out.Refs[i].ID < out.Refs[j].ID
	})
	out.Cycles = stronglyConnected(visited, edges)
	return out, nil
}

// inEdge is one incoming link: who points here, and with what type.
type inEdge struct {
	src string
	typ string
}

// linksTo reads the edges pointing AT a record. It is a lookup rather than a
// scan - links_by_dst exists for exactly this, because rig's own citation graph
// has a node at in-degree 758 and a scan there is not interactive.
//
// ⛔ AN EDGE FROM A RETRACTED RECORD IS NOT READ. Section 39: a retracted
// record "stops appearing in a brief or a query", and a refs answer is what
// a brief is assembled from - a withdrawn record that still pointed here
// would reach the brief through this door with nothing saying it was
// withdrawn. Filtering HERE rather than on the answer covers both readers:
// the walk does not follow it, and moreBeyond does not count it as somewhere
// still to go, so it cannot raise the truncation flag either. The edge row
// itself survives, as retraction promises: un-retracting is not a verb today,
// but nothing was destroyed that one would need.
func (s *Store) linksTo(ctx context.Context, dst string) ([]inEdge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT l.src, l.type FROM links l
		 WHERE l.dst = ?
		   AND NOT EXISTS (SELECT 1 FROM retractions x WHERE x.id = l.src)
		 ORDER BY l.src, l.type`, dst)
	if err != nil {
		return nil, fmt.Errorf("record: reading what points at %s: %w", dst, err)
	}
	defer func() { _ = rows.Close() }()

	var out []inEdge
	for rows.Next() {
		var e inEdge
		if err := rows.Scan(&e.src, &e.typ); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// moreBeyond reports whether the traversal stopped with somewhere still to go.
//
// It is one of the two sources of Refs.Truncated - the horizon - and it asks
// the question the flag actually means: does any node the bound stopped at have
// an incoming edge to a record this walk has not already reached? A frontier is
// non-empty on every complete traversal too - those are the leaves - so the
// flag cannot be read off its length.
//
// ⛔ IT TAKES NO PROJECT AND NO SCOPE, AND THE ABSENCE IS THE FIX. It used to
// filter by project exactly as the hop loop did, so the same defect reached the
// horizon check: a walk whose only unexplored neighbours were foreign reported
// itself complete, when what lay behind those neighbours was never looked at
// and could be the subject's own project. The walk now crosses freely, so the
// horizon question is "is there unvisited graph", full stop. Scope is applied
// to the ANSWER, in Refs, and it raises the flag there itself.
func (s *Store) moreBeyond(ctx context.Context, frontier []string, visited map[string]bool) (bool, error) {
	for _, node := range frontier {
		in, err := s.linksTo(ctx, node)
		if err != nil {
			return false, err
		}
		for _, e := range in {
			if !visited[e.src] {
				return true, nil
			}
		}
	}
	return false, nil
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
