package daemon

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// THERE ARE TWO STRUCTURAL DESCRIPTIONS OF A PROGRAM IN THIS REPOSITORY AND
// NOTHING TIED THEM TOGETHER UNTIL THIS FILE.
//
//	programToWire / commandToWire   kernel.Program -> *rigv1.Program   (protobuf)
//	meta.MarshalAnswer              meta.Answer    -> JSON bytes       (json)
//
// They are not the same rendering and one does not wrap the other. The
// duplication is deliberate: moving the wire renderers into meta would move
// five enum maps that registration itself uses, and cmd/rig cannot import
// internal/meta at all on section 17 grounds, because that drags the schema
// validator into the client. So the collapse is not available and THE GUARD IS
// THE ANSWER RATHER THAN A WAYPOINT.
//
// The defect it exists to catch: someone adds a field to kernel.Command or
// kernel.Program, updates one renderer, and nothing makes them update the
// other. It presents weeks later as "the CLI shows it and MCP does not", which
// is invisible to review and green for as long as it exists.
//
// Same idiom as TestEveryProgramFieldChangesTheVersion in internal/kernel: a
// hand-written list of fields rots, reflection does not.

func TestEveryProgramFieldReachesBothRenderers(t *testing.T) {
	base := fullKernelProgram()
	baseWire := programToWire(base)
	baseJSON := marshalOne(t, base)

	typ := reflect.TypeOf(kernel.Program{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			one := fullKernelProgram()
			f := reflect.ValueOf(&one).Elem().Field(i)
			if !nudgeField(f) {
				t.Fatalf("this test does not know how to change a %s, so "+
					"Program.%s is NOT covered by either renderer. Teach "+
					"nudgeField about it rather than skipping", f.Kind(), name)
			}
			if proto.Equal(baseWire, programToWire(one)) {
				t.Errorf("changing Program.%s did not change the WIRE rendering: "+
					"programToWire drops it, so an MCP or CLI caller reading the "+
					"wire can never see it", name)
			}
			if string(baseJSON) == string(marshalOne(t, one)) {
				t.Errorf("changing Program.%s did not change the JSON rendering: "+
					"meta's renderer drops it, so an agent reading --json or an "+
					"MCP tool answer can never see it", name)
			}
		})
	}
}

// The sibling walk, and it exists because the one above does NOT cover it:
// nudging Program.Commands appends a command, which proves only that the COUNT
// participates. A command's own fields could be dropped wholesale by either
// renderer and that test would still pass.
func TestEveryCommandFieldReachesBothRenderers(t *testing.T) {
	base := fullKernelProgram()
	baseWire := programToWire(base)
	baseJSON := marshalOne(t, base)

	typ := reflect.TypeOf(kernel.Command{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			one := fullKernelProgram()
			f := reflect.ValueOf(&one.Commands[0]).Elem().Field(i)
			if !nudgeField(f) {
				t.Fatalf("this test does not know how to change a %s, so "+
					"Command.%s is NOT covered. Teach nudgeField about it "+
					"rather than skipping", f.Kind(), name)
			}
			if proto.Equal(baseWire, programToWire(one)) {
				t.Errorf("changing Command.%s did not change the WIRE rendering: "+
					"commandToWire drops it", name)
			}
			if string(baseJSON) == string(marshalOne(t, one)) {
				t.Errorf("changing Command.%s did not change the JSON rendering: "+
					"meta's renderer drops it", name)
			}
		})
	}
}

// marshalOne renders one program through the meta JSON path, which is the
// exact path an agent reads.
func marshalOne(t *testing.T, p kernel.Program) []byte {
	t.Helper()
	b, err := meta.MarshalAnswer(meta.Answer{
		Tool:    meta.Tool("list"),
		Estate:  []kernel.Program{p},
		Program: &p,
		Command: &p.Commands[0],
	})
	if err != nil {
		t.Fatalf("MarshalAnswer: %v", err)
	}
	return b
}

// nudgeField changes a value in a way its type can express, reporting whether
// it knew how. It refuses to pretend: an unknown kind returns false so the test
// says the field is uncovered rather than passing over it.
func nudgeField(f reflect.Value) bool {
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
		// []byte holding a JSON schema must stay valid JSON, so grow it in a
		// way the renderers can still marshal.
		if f.Type().Elem().Kind() == reflect.Uint8 {
			f.SetBytes([]byte(`{"type":"object","title":"changed"}`))
			return true
		}
		f.Set(reflect.Append(f, reflect.New(f.Type().Elem()).Elem()))
		return true
	case reflect.Struct:
		for i := range f.NumField() {
			if nudgeField(f.Field(i)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// fullKernelProgram is maximal for the same reason the digest's fixture is:
// nudging a field that was already at its zero value would prove nothing about
// a field that is really used.
func fullKernelProgram() kernel.Program {
	return kernel.Program{
		Identity: kernel.Identity{
			ID: "shelf", Name: "Shelf", Version: "1.2.3",
			Icon: "shelf.png", Description: "the shelf",
		},
		Coverage:     kernel.CoveragePartial,
		CoverageNote: "the wire only",
		SemanticsGen: 3,
		Services:     []string{"search"},
		Elements:     []string{"pane"},
		Hosted:       true,
		PaneURL:      "http://127.0.0.1:1/pane",
		Preamble:     "read this first",
		Commands: []kernel.Command{{
			ID: "reindex", Title: "Reindex",
			Args:          []byte(`{"type":"object"}`),
			Examples:      []string{"rig shelf reindex"},
			Effects:       kernel.EffectsWritesFiles,
			Idempotent:    kernel.Yes,
			Sensitive:     []string{"/token"},
			Interactive:   kernel.No,
			Streams:       kernel.No,
			NeedsDisplay:  kernel.No,
			Duration:      kernel.DurationSeconds,
			Confirms:      kernel.No,
			Shape:         kernel.ShapeUnary,
			Summary:       "reindex the shelf",
			Description:   "walks every item",
			Returns:       "a count",
			DryRun:        true,
			Cost:          "cheap",
			Preconditions: []string{"the shelf exists"},
			Promote:       true,
		}},
	}
}
