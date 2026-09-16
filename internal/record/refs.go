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
	// ⛔ THE DEFAULT IS A PERFORMANCE REQUIREMENT, NOT A PREFERENCE. Section 39:
	// a link MAY cross a project boundary and a traversal is project-scoped by
	// default, because the predicate prunes the frontier at EVERY hop - it
	// removes the work rather than filtering the same work. Measured 98%
	// cheaper. Crossing is asked for, never arrived at.
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
	Via string
}

// Refs is the answer to "what points at this".
type Refs struct {
	ID   string
	Refs []Ref

	// Truncated is true when the traversal stopped at the depth bound with
	// somewhere still to go.
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
				if !r.CrossProject && rec.Project != subject.Project {
					continue
				}
				visited[e.src] = true
				out.Refs = append(out.Refs, Ref{
					ID: e.src, Kind: rec.Kind, Title: rec.Fields["title"],
					Type: e.typ, Depth: hop, Via: node,
				})
				next = append(next, e.src)
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
		if hop == depth {
			out.Truncated, err = s.moreBeyond(ctx, frontier, visited, subject.Project, r.CrossProject)
			if err != nil {
				return Refs{}, err
			}
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
func (s *Store) linksTo(ctx context.Context, dst string) ([]inEdge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT src, type FROM links WHERE dst = ? ORDER BY src, type`, dst)
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
// It is the horizon check behind Refs.Truncated, and it asks the question the
// flag actually means: does any node the bound stopped at have an incoming edge
// to a record this answer does not already carry, inside whatever scope the
// caller asked for? A frontier is non-empty on every complete traversal too -
// those are the leaves - so the flag cannot be read off its length.
func (s *Store) moreBeyond(ctx context.Context, frontier []string, visited map[string]bool, project string, cross bool) (bool, error) {
	for _, node := range frontier {
		in, err := s.linksTo(ctx, node)
		if err != nil {
			return false, err
		}
		for _, e := range in {
			if visited[e.src] {
				continue
			}
			if !cross {
				rec, err := s.Get(ctx, e.src)
				if err != nil {
					return false, err
				}
				if rec.Project != project {
					continue
				}
			}
			return true, nil
		}
	}
	return false, nil
}
