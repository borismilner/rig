package meta_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// Section 10 binds --json to exactly what the MCP tool returns, and --depth is
// the knob an agent turns to control its own context budget. An object that
// cannot say how much was asked for leaves the agent inferring it from which
// fields happen to be populated.
func TestDepthIsAlwaysEmittedEvenWhenNoDepthWasInvolved(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   meta.Answer
		want string
	}{
		{"list at programs", meta.Answer{Tool: meta.List, Depth: kernel.DepthPrograms}, "programs"},
		{"list at commands", meta.Answer{Tool: meta.List, Depth: kernel.DepthCommands}, "commands"},
		{"list at full", meta.Answer{Tool: meta.List, Depth: kernel.DepthFull}, "full"},
		// invoke projects nothing, so section 21's zero is the honest answer
		// rather than a missing key.
		{"invoke", meta.Answer{Tool: meta.Invoke}, kernel.DepthUnspecified.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := meta.MarshalAnswer(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			d, present := got["depth"]
			if !present {
				t.Fatalf("depth is absent from %s. An absent depth cannot be "+
					"told apart from a server too old to have the field, which "+
					"is the exact ambiguity partial is always emitted to avoid",
					b)
			}
			if d != tc.want {
				t.Errorf("depth is %v, want %q", d, tc.want)
			}
		})
	}
}

// Enums render as their names here, not as numbers: section 21 gives every enum
// a zero meaning "nothing was said", which is legible as a name and silently
// wrong as 0.
func TestDepthRendersAsANameNotANumber(t *testing.T) {
	b, err := meta.MarshalAnswer(meta.Answer{Tool: meta.List, Depth: kernel.DepthFull})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"depth":"full"`) {
		t.Fatalf("depth did not render as a name: %s", b)
	}
}

// THE GUARD THAT WOULD HAVE CAUGHT THE DEFECT THIS FILE EXISTS FOR.
//
// Depth was added to the meta layer and nothing emitted it, which is the same
// class as a declared field no surface can return: accepted, carried, dead.
// This walks meta.Answer by reflection and fails on any field whose change the
// JSON does not notice - so the next field added to Answer cannot be silently
// dropped by MarshalAnswer.
//
// Same idiom as the renderer guard in internal/daemon and the digest guard in
// internal/kernel: a hand-written list of fields rots, reflection does not.
func TestEveryAnswerFieldReachesTheJSON(t *testing.T) {
	base := fullAnswer()
	baseline := marshal(t, base)

	typ := reflect.TypeOf(meta.Answer{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			one := fullAnswer()
			f := reflect.ValueOf(&one).Elem().Field(i)
			if !nudge(f) {
				t.Fatalf("this test does not know how to change a %s, so "+
					"Answer.%s is NOT covered. Teach nudge about it rather "+
					"than skipping, or MarshalAnswer can quietly drop it",
					f.Kind(), name)
			}
			if string(marshal(t, one)) == string(baseline) {
				t.Errorf("changing Answer.%s did not change the JSON: "+
					"MarshalAnswer drops it, so an agent reading --json or an "+
					"MCP tool answer can never see it", name)
			}
		})
	}
}

func marshal(t *testing.T, a meta.Answer) []byte {
	t.Helper()
	b, err := meta.MarshalAnswer(a)
	if err != nil {
		t.Fatalf("MarshalAnswer: %v", err)
	}
	return b
}

func fullAnswer() meta.Answer {
	p := kernel.Program{
		Identity: kernel.Identity{ID: "shelf", Name: "Shelf", Version: "1"},
		Coverage: kernel.CoveragePartial,
		Commands: []kernel.Command{{ID: "reindex", Summary: "reindex"}},
	}
	return meta.Answer{
		Tool:        meta.List,
		Partial:     []meta.Incomplete{{Program: "shelf", Coverage: kernel.CoveragePartial, Note: "wire only"}},
		Estate:      []kernel.Program{p},
		Version:     "v1",
		Depth:       kernel.DepthCommands,
		Program:     &p,
		Command:     &p.Commands[0],
		Result:      []byte(`{"count":1}`),
		Unavailable: []string{"logs"},
	}
}

func nudge(f reflect.Value) bool {
	switch f.Kind() {
	case reflect.String:
		f.SetString(f.String() + "-changed")
		return true
	case reflect.Bool:
		f.SetBool(!f.Bool())
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(f.Int() + 1)
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.SetUint(f.Uint() + 1)
		return true
	case reflect.Slice:
		if f.Type().Elem().Kind() == reflect.Uint8 {
			f.SetBytes([]byte(`{"count":2}`))
			return true
		}
		f.Set(reflect.Append(f, reflect.New(f.Type().Elem()).Elem()))
		return true
	case reflect.Pointer:
		// Program and Command: change what is pointed AT, not the pointer, so
		// a renderer that follows the pointer is what is being tested.
		if f.IsNil() {
			f.Set(reflect.New(f.Type().Elem()))
		}
		return nudge(f.Elem())
	case reflect.Struct:
		for i := range f.NumField() {
			if nudge(f.Field(i)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// The always-emitted fields carry no omitempty, and this is asserted on the TAG
// rather than on behaviour.
//
// Measured: adding omitempty to depth does not fail any marshalling test,
// because the enum renders as a name and "unspecified" is not the empty string.
// The tag is therefore inert today and a trap tomorrow - rendering the zero as
// "" would make omitempty start dropping the key, and an absent depth cannot be
// told apart from a server too old to have the field.
func TestTheAlwaysEmittedFieldsCarryNoOmitempty(t *testing.T) {
	for _, name := range []string{"Depth", "Partial"} {
		f, ok := meta.AnswerJSONShape.FieldByName(name)
		if !ok {
			t.Fatalf("answerJSON has no field %s", name)
		}
		if strings.Contains(f.Tag.Get("json"), "omitempty") {
			t.Errorf("%s carries omitempty (%q). It is emitted ALWAYS: an "+
				"absent value cannot be told apart from a server too old to "+
				"have the field", name, f.Tag.Get("json"))
		}
	}
}
