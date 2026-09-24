package daemon

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// TestEverySeatFieldReachesTheRosterRow is the guard seatToOccupant does not
// get from the compiler.
//
// WHY IT EXISTS. seatToOccupant is a keyed struct literal, so a field added to
// the wire's Seat and forgotten there compiles, and every roster row then
// carries a zero no reader can tell from a real value. The comment on that
// function used to claim the compiler covered this; it does not, and the
// measurement that showed so is what produced this test. meta.occupantToJSON's
// CONVERSION is a real compile-time contract, but it binds Occupant to
// occupantJSON only - obeying the error it raises leaves seatToOccupant
// untouched and the build green.
//
// THIS IS THE THIRD ASSERTION OVER THIS ROW AND ALL THREE EARN THEIR PLACE. Do
// not collapse them:
//
//   - TestARosterRowCarriesEverythingASupervisorReads names its fields one at a
//     time on purpose, because the thing worth asserting there is that a HUMAN
//     decided each belongs on the wire an agent reads.
//   - TestEveryToolAnswersYouTheSameWayInEveryStateACallerReaches varies the
//     caller's STATE rather than the assertion, because a mutation cannot catch
//     a case no test enters.
//   - this one guards that the MAPPING covers what it claims to. A reflective
//     check over one struct passes itself by construction; this walks Seat and
//     asserts against Occupant, which are two different types with the mapping
//     between them as the subject.
//
// IT WALKS THE GO STRUCT WITH reflect, NOT protoreflect, AND THAT IS
// DELIBERATE. protoreflect is the idiomatic choice for a generated type and is
// the wrong one here: the red-before-green pass for this test adds a field to
// the generated Go struct by hand, which protoreflect cannot see because it is
// not in the descriptor. The mutation would be a silent no-op, the test would
// stay green, and the rule that a guard must be proven red would make somebody
// discard a working test for a reason that has nothing to do with it.
func TestEverySeatFieldReachesTheRosterRow(t *testing.T) {
	// THE SKIP LIST IS ASSERTED, NOT APPLIED. An implicit "exported fields
	// only" filter would let this walk skip a real field and still pass, which
	// is the same passes-by-construction failure the test exists to prevent. So
	// the unexported names are stated here and compared against what the walk
	// actually skipped: if protobuf renames its internals, or an unexported
	// field of our own appears, this goes red rather than quietly widening.
	wantSkipped := []string{"sizeCache", "state", "unknownFields"}

	zero := seatToOccupant(&rigv1.Seat{})
	st := reflect.TypeOf(rigv1.Seat{})

	var walked, skipped []string
	for i := range st.NumField() {
		f := st.Field(i)
		if !f.IsExported() {
			skipped = append(skipped, f.Name)
			continue
		}
		walked = append(walked, f.Name)

		one := &rigv1.Seat{}
		if err := setDistinct(reflect.ValueOf(one).Elem().Field(i)); err != "" {
			t.Fatalf("Seat.%s is a %s and this walk cannot set one: %s. "+
				"EXTEND setDistinct rather than skipping the field - a walk "+
				"that cannot set a field cannot tell a mapped one from an "+
				"unmapped one, and would pass either way",
				f.Name, f.Type, err)
		}
		if seatToOccupant(one) == zero {
			t.Errorf("Seat.%s HAS NO HOME ON THE ROSTER ROW. Setting it alone "+
				"leaves seatToOccupant's output byte-identical to the zero "+
				"row, so the field crosses the wire and is dropped before any "+
				"agent sees it. seatToOccupant is a keyed literal and the "+
				"compiler will not tell you: add the field there, and to "+
				"meta.Occupant and occupantJSON", f.Name)
		}
	}

	// A WALK OVER NOTHING PASSES EVERY ASSERTION ABOVE. Say so out loud.
	if len(walked) == 0 {
		t.Fatal("this walk found no exported fields on rigv1.Seat at all, so " +
			"it proved nothing. The test is broken, not the mapping")
	}

	sort.Strings(skipped)
	if !slicesEqual(skipped, wantSkipped) {
		t.Errorf("the unexported fields of rigv1.Seat are %v, and this test "+
			"skips exactly %v. They differ, so the walk is skipping something "+
			"it was never reviewed for. Decide which it is and update the "+
			"list deliberately - do not widen it to make this pass",
			skipped, wantSkipped)
	}
}

// setDistinct sets v to a non-zero value distinguishable from the zero row.
// It returns "" on success and a reason otherwise, because a kind it cannot set
// must fail the test loudly rather than be skipped.
func setDistinct(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		v.SetString("a value no zero row carries")
	case reflect.Uint64, reflect.Uint32:
		v.SetUint(7)
	case reflect.Int64, reflect.Int32:
		// Int32 is how a proto enum arrives; 1 is the first declared value and
		// renders differently from the zero one.
		v.SetInt(1)
	case reflect.Bool:
		v.SetBool(true)
	default:
		return "no rule for this kind"
	}
	return ""
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
