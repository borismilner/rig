package kernel

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"slices"
	"strconv"
)

// Basis says HOW a capability map was filtered. It never says WHAT was
// filtered out, and that distinction is the whole of it (PLAN.md section 36,
// V20 projection totality).
//
// V20 rules that a projection must be TOTAL: sanitisation is declared rather
// than incidental, because "absent" and "withheld" are different facts and
// only one of them means "ask for more access". A scoped caller reading a map
// of one program cannot otherwise tell an estate of one from an estate of
// three it may see one of.
//
// IT STATES THE BASIS AND NAMES NOTHING, WHICH IS WHY IT DOES NOT COLLIDE
// WITH THE RULE ABOVE View.Program. That rule refuses to report a program the
// caller may not see, in as many words, because "you may not see shelf" tells
// the caller that shelf exists. Both rules hold at once as long as withheld
// never means named or counted: a count is enumeration with the labels taken
// off, and estate size is precisely what a scoped caller must not be able to
// enumerate. So there is no count here, no identity, and no size.
//
// IT DESCRIBES THE MAP, NOT THE CALLER. V20's own words are that the
// projection states its own coverage, so this is a property of the
// projection. Naming the caller's grant here would conflate two subjects,
// which is the section 21 conflation this repository keeps catching. An agent
// reading BasisScoped knows its picture is bounded; section 14 is where it
// learns the grant it lacks is called introspect.
type Basis uint8

const (
	// BasisUnspecified is nothing said, and it is its own value rather than a
	// synonym for complete. THE ASYMMETRY IS THE POINT: a map that forgot to
	// set this must not tell a scoped caller it has seen everything. The safe
	// guess and the useful guess point opposite ways, which is the same trap
	// an unset estate role and an unnamed estate were each ruled on.
	BasisUnspecified Basis = iota

	// BasisComplete is the whole estate. Nothing was filtered away for this
	// caller, so absent really does mean absent.
	BasisComplete

	// BasisScoped is what this caller may reach. Something may have been
	// filtered away and the map does not say what, so absent here means
	// "not shown to you or not there", and asking for more access is a move.
	//
	// It covers every caller that is not seeing everything, not only one with
	// scopes: an unprivileged client reading only what is unscoped is equally
	// bounded, and telling it otherwise would be the false reassurance this
	// field exists to prevent.
	BasisScoped
)

var basisNames = map[Basis]string{
	BasisUnspecified: unspecifiedName,
	BasisComplete:    "complete",
	BasisScoped:      "scoped",
}

func (b Basis) String() string {
	if n, ok := basisNames[b]; ok {
		return n
	}
	return fmt.Sprintf("Basis(%d)", uint8(b))
}

// CapabilityMap is the whole estate as ONE principal may see it (PLAN.md
// section 9, "Discovery is a resource, not a guess").
//
// An agent reads it once and knows everything that exists. When a program
// registers a new command the map changes, and no agent needs updating and no
// rig change is involved.
type CapabilityMap struct {
	// Version identifies THIS map's contents. Two maps with the same version
	// are the same map; two with different versions differ somewhere.
	Version string

	// Depth is what was asked for, carried because a map's identity includes
	// it: the same estate at two depths is two different maps and must never
	// share a version.
	Depth Depth

	// Basis says how this map was filtered: the whole estate, or what this
	// caller may reach. It is what lets a scoped caller tell an estate of one
	// from an estate of three it may see one of.
	Basis Basis

	Programs []Program
}

// CapabilityMap builds the map this principal may read.
//
// IT IS A GRANTED SURFACE AND IT COSTS NOTHING EXTRA TO BE ONE. Section 9
// says a client with no grant gets the map of what it may reach and an agent
// Boris runs holds introspect and gets all of it. That is already exactly what
// View does, so the map is built through the same filter as every other read
// rather than through a second one - and a second one is how the 2026-09-10
// sweep found a grant reaching a caller kind nobody was thinking about.
//
// THE VERSION IS OF THE PROJECTION, NOT OF THE REGISTRY, and that is the whole
// design. A counter on the registry would give a scoped caller and an
// introspecting one the SAME version for DIFFERENT maps at the same instant -
// which is precisely the pair section 9's demo puts side by side. Anything
// caching on that version then serves one caller the other's estate. So the
// version is a digest of the bytes this principal was actually given.
//
// A digest rather than a counter for a second reason, measured rather than
// assumed: nothing rig holds survives a daemon restart, so a counter restarts
// at zero while the estate it describes is unchanged. A digest is stable
// across a restart, and comparable between two daemons, because it is a
// function of content alone.
func (v View) CapabilityMap(d Depth) (CapabilityMap, error) {
	programs, err := v.Estate(d)
	if err != nil {
		return CapabilityMap{}, err
	}
	// Built first and digested second, because the version is a digest OF
	// THE MAP and a map cannot carry its own version while it is being
	// computed. The ordering is the whole of the change: what is hashed is
	// now the thing that was produced, not the arguments that produced it.
	m := CapabilityMap{Depth: d, Basis: v.basis(), Programs: programs}
	m.Version = mapVersion(m)
	return m, nil
}

// mapVersion digests a map.
//
// IT TAKES THE MAP, AND THAT IS SECTION 9'S CONTRACT RATHER THAN A CHANGE TO
// IT. Section 9: the version is "a digest of the projection that principal was
// given" and "must therefore be a function of the bytes this principal
// actually received". A digest of (depth, programs) is a digest of the INPUTS
// that happen to produce the projection. While the map had exactly two fields
// and those two fields were its inputs, the two formulations were the same
// function - so this was correct by coincidence, and the coincidence ends at
// the third field rather than a new defect beginning there.
//
// THE VERSION FIELD ITSELF IS EXCLUDED, AND IT IS THE ONE EXCLUSION. A digest
// cannot cover the field it is being written into. It is named here rather
// than left to be re-derived, because an exclusion nobody wrote down is
// indistinguishable from a field somebody forgot - which is the defect this
// whole mechanism exists to catch. Every OTHER field must reach the walk, and
// TestEveryCapabilityMapFieldChangesTheVersion is what makes that true rather
// than intended.
//
// The depth is folded in first, so the same estate at two depths never shares
// a version. Both of section 9's other required properties are properties of
// the walk and are unchanged: commands are sorted by id, and every value is
// length-prefixed.
func mapVersion(m CapabilityMap) string {
	h := sha256.New()
	writeUint(h, uint64(m.Depth))
	writeUint(h, uint64(m.Basis))
	writeUint(h, uint64(len(m.Programs)))
	for _, p := range m.Programs {
		writeProgram(h, p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeProgram writes one program's contents in a fixed order.
//
// EVERY FIELD PARTICIPATES, AND THAT IS TESTED RATHER THAN INTENDED. A field
// left out here is a field that can change without changing the version,
// which is a diff that silently reports "no change" - the same class of defect
// as a declared field no caller can read, and just as invisible to review.
// TestEveryProgramFieldChangesTheVersion walks the struct by reflection and
// fails on any field this function ignores.
func writeProgram(h hash.Hash, p Program) {
	writeString(h, p.Identity.ID)
	writeString(h, p.Identity.Name)
	writeString(h, p.Identity.Version)
	writeString(h, p.Identity.Icon)
	writeString(h, p.Identity.Description)
	writeUint(h, uint64(p.Coverage))
	writeString(h, p.CoverageNote)
	writeInt(h, int64(p.SemanticsGen))
	writeStrings(h, p.Services)
	writeStrings(h, p.Elements)
	writeBool(h, p.Hosted)
	writeString(h, p.PaneURL)
	writeString(h, p.Preamble)

	// Sorted by id, so the map is canonical. Declaration order is what a
	// program happened to write, not something it declared, and two estates
	// that differ only by it are the same estate - a diff that reported
	// otherwise would be noise an agent has to learn to ignore.
	cmds := slices.Clone(p.Commands)
	slices.SortFunc(cmds, func(a, b Command) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	writeUint(h, uint64(len(cmds)))
	for _, c := range cmds {
		writeCommand(h, c)
	}
}

func writeCommand(h hash.Hash, c Command) {
	writeString(h, c.ID)
	writeString(h, c.Title)
	writeBytes(h, c.Args)
	writeStrings(h, c.Examples)
	writeUint(h, uint64(c.Effects))
	writeUint(h, uint64(c.Idempotent))
	writeStrings(h, c.Sensitive)
	writeUint(h, uint64(c.Interactive))
	writeUint(h, uint64(c.Streams))
	writeUint(h, uint64(c.NeedsDisplay))
	writeUint(h, uint64(c.Duration))
	writeUint(h, uint64(c.Confirms))
	writeUint(h, uint64(c.Shape))
	writeString(h, c.Summary)
	writeString(h, c.Description)
	writeString(h, c.Returns)
	writeBool(h, c.DryRun)
	writeString(h, c.Cost)
	writeStrings(h, c.Preconditions)
	writeBool(h, c.Promote)
}

// Every value is length-prefixed, so no two different contents can produce the
// same bytes by running into each other - "ab"+"c" and "a"+"bc" are the
// classic case and the reason a digest over concatenated fields is wrong.
func writeString(h hash.Hash, s string) {
	writeUint(h, uint64(len(s)))
	_, _ = h.Write([]byte(s))
}

func writeBytes(h hash.Hash, b []byte) {
	writeUint(h, uint64(len(b)))
	_, _ = h.Write(b)
}

// writeStrings distinguishes a nil slice from an empty one, because the
// declaration does: an empty Sensitive means the program considered the
// question and answered none.
func writeStrings(h hash.Hash, ss []string) {
	if ss == nil {
		writeUint(h, 0)
		return
	}
	writeUint(h, uint64(len(ss))+1)
	for _, s := range ss {
		writeString(h, s)
	}
}

// writeInt writes a signed value as its decimal text, length-prefixed.
//
// Text rather than bits, and that is not fussiness: every bit-level route
// from a signed type to the unsigned one writeUint takes is an integer
// conversion gosec's G115 refuses, and a suppression here would be a
// suppression on the one function whose whole job is that two different
// values never produce the same bytes. Decimal text has no such conversion
// and is unambiguous once length-prefixed.
func writeInt(h hash.Hash, v int64) {
	writeString(h, strconv.FormatInt(v, 10))
}

func writeBool(h hash.Hash, b bool) {
	var v uint64
	if b {
		v = 1
	}
	writeUint(h, v)
}

func writeUint(h hash.Hash, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, _ = h.Write(b[:])
}

// String renders a map's identity for a log line or an error.
func (m CapabilityMap) String() string {
	return fmt.Sprintf("capability map %s at depth %s, %d programs",
		m.Version[:min(12, len(m.Version))], m.Depth, len(m.Programs))
}

// basis reports how this view filters, which is the map's basis.
//
// Complete is exactly the introspecting case and nothing else, because
// canSee's one door out of default deny is Introspect: every other principal
// is filtered, whether by its scopes or by having none. Deriving it from the
// same predicate the filter uses is deliberate - a second rule for "is this
// caller seeing everything" is how the two drift and the map starts claiming
// completeness the filter did not give it.
func (v View) basis() Basis {
	if v.p.Introspect {
		return BasisComplete
	}
	return BasisScoped
}
