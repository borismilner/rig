package daemon

import (
	"reflect"
	"testing"

	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// ⛔ THE STEP VOCABULARY IS THE WIRE ENUM AND NOTHING ELSE. The ruling is in
// internal/record/progress.go. This is the test that makes it hold: every
// non-zero StepState maps to a name in stepStateNames, the zero maps to
// nothing, and the names are exactly the set the store accepts, in the
// enum's order. A value added to any one of the three copies and not the
// others fails here rather than as a refusal in front of a caller.
func TestTheStepVocabularyIsTheWireEnumAndNothingElse(t *testing.T) {
	values := rigv1.StepState_STEP_STATE_UNSPECIFIED.Descriptor().Values()
	var fromEnum []string
	for i := range values.Len() {
		v := rigv1.StepState(values.Get(i).Number())
		name, ok := stepStateNames[v]
		if v == rigv1.StepState_STEP_STATE_UNSPECIFIED {
			if ok {
				t.Errorf("the zero StepState maps to %q; an unset field would "+
					"decode as a decision", name)
			}
			continue
		}
		if !ok {
			t.Errorf("%s has no name in stepStateNames, so the daemon hands "+
				"the store an empty state for a value the wire declares", v)
			continue
		}
		fromEnum = append(fromEnum, name)
	}
	if len(stepStateNames) != len(fromEnum) {
		t.Errorf("stepStateNames has %d entries and the enum has %d non-zero "+
			"values; the map names a value the wire does not declare",
			len(stepStateNames), len(fromEnum))
	}
	if got := record.StepStates(); !reflect.DeepEqual(got, fromEnum) {
		t.Errorf("the store accepts %v and the wire enum declares %v; the "+
			"enum is the vocabulary", got, fromEnum)
	}
}
