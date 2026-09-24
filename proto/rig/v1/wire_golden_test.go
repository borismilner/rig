package rigv1_test

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/borismilner/rig/proto/rig/v1"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/wire_golden.txt")

// ⛔ THE BYTES ON THE WIRE ARE THE CONTRACT, AND THIS TEST PINS THEM FOR EVERY
// MESSAGE AND EVERY ENUM VALUE IN PROTO PACKAGE rig.v1.
//
// It walks the registry by proto package rather than by Go type, so it does
// not care which Go package a message is generated into: splitting the
// generated code across packages must leave every line of the golden file
// unchanged, and any renumbered field, retyped field or moved enum value
// changes a line. Each message is filled deterministically from its own
// descriptor (every field set, repeated fields with two elements, nested
// messages two levels deep) and encoded with deterministic marshalling.
func TestEveryWireMessageEncodesExactlyAsRecorded(t *testing.T) {
	got := goldenLines(t)
	const path = "testdata/wire_golden.txt"
	if *updateGolden {
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to record it)", err)
	}
	want := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(got) != len(want) {
		t.Errorf("%d golden lines now, %d recorded", len(got), len(want))
	}
	wantSet := map[string]bool{}
	for _, l := range want {
		wantSet[l] = true
	}
	gotSet := map[string]bool{}
	for _, l := range got {
		gotSet[l] = true
		if !wantSet[l] {
			t.Errorf("not as recorded: %s", l)
		}
	}
	for _, l := range want {
		if !gotSet[l] {
			t.Errorf("recorded and now missing: %s", l)
		}
	}
}

func goldenLines(t *testing.T) []string {
	t.Helper()
	var lines []string
	protoregistry.GlobalFiles.RangeFilesByPackage("rig.v1", func(fd protoreflect.FileDescriptor) bool {
		walkMessages(fd.Messages(), func(md protoreflect.MessageDescriptor) {
			mt, err := protoregistry.GlobalTypes.FindMessageByName(md.FullName())
			if err != nil {
				t.Fatalf("%s is described but has no Go type: %v", md.FullName(), err)
			}
			m := mt.New()
			fill(m, 2)
			b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m.Interface())
			if err != nil {
				t.Fatalf("%s: %v", md.FullName(), err)
			}
			lines = append(lines, fmt.Sprintf("message %s %s", md.FullName(), hex.EncodeToString(b)))
		})
		walkEnums(fd, func(ed protoreflect.EnumDescriptor) {
			vals := ed.Values()
			for i := range vals.Len() {
				v := vals.Get(i)
				lines = append(lines, fmt.Sprintf("enum %s %s=%d", ed.FullName(), v.Name(), v.Number()))
			}
		})
		return true
	})
	if len(lines) == 0 {
		t.Fatal("no rig.v1 file is registered, so this test checked nothing")
	}
	sort.Strings(lines)
	return lines
}

func walkMessages(ms protoreflect.MessageDescriptors, f func(protoreflect.MessageDescriptor)) {
	for i := range ms.Len() {
		md := ms.Get(i)
		if md.IsMapEntry() {
			continue
		}
		f(md)
		walkMessages(md.Messages(), f)
	}
}

func walkEnums(fd protoreflect.FileDescriptor, f func(protoreflect.EnumDescriptor)) {
	for i := range fd.Enums().Len() {
		f(fd.Enums().Get(i))
	}
	var nested func(protoreflect.MessageDescriptors)
	nested = func(ms protoreflect.MessageDescriptors) {
		for i := range ms.Len() {
			md := ms.Get(i)
			for j := range md.Enums().Len() {
				f(md.Enums().Get(j))
			}
			nested(md.Messages())
		}
	}
	nested(fd.Messages())
}

// fill sets every field of m from its descriptor alone, so the same message
// gets the same bytes whatever Go package generated it.
func fill(m protoreflect.Message, depth int) {
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.ContainingOneof() != nil && !fd.HasOptionalKeyword() {
			// One member per oneof: the first, so the choice is stable.
			if fd.ContainingOneof().Fields().Get(0) != fd {
				continue
			}
		}
		switch {
		case fd.IsMap():
			mp := m.Mutable(fd).Map()
			k := scalar(fd.MapKey(), 1).MapKey()
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				if depth > 0 {
					v := mp.NewValue()
					fill(v.Message(), depth-1)
					mp.Set(k, v)
				}
			} else {
				mp.Set(k, scalar(fd.MapValue(), 1))
			}
		case fd.IsList():
			l := m.Mutable(fd).List()
			for n := 1; n <= 2; n++ {
				if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
					if depth == 0 {
						break
					}
					v := l.NewElement()
					fill(v.Message(), depth-1)
					l.Append(v)
				} else {
					l.Append(scalar(fd, n))
				}
			}
		case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
			if depth > 0 {
				fill(m.Mutable(fd).Message(), depth-1)
			}
		default:
			m.Set(fd, scalar(fd, 1))
		}
	}
}

func scalar(fd protoreflect.FieldDescriptor, n int) protoreflect.Value {
	num := int64(fd.Number())*10 + int64(n)
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true)
	case protoreflect.EnumKind:
		vals := fd.Enum().Values()
		return protoreflect.ValueOfEnum(vals.Get(vals.Len() - 1).Number())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(int32(num))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(num)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(num))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(num))
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(float32(num))
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(float64(num))
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(fmt.Sprintf("%s-%d", fd.Name(), n))
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte{byte(fd.Number()), byte(n)})
	}
	panic(fmt.Sprintf("unhandled kind %s", fd.Kind()))
}
