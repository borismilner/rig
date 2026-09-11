package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// dialect is the JSON Schema version the emitted files declare.
const dialect = "https://json-schema.org/draft/2020-12/schema"

// keyType is JSON Schema's own keyword, not a field of rig's.
const keyType = "type"

// typed is a schema that is nothing but a type, which most of them are.
func typed(t any) map[string]any { return map[string]any{keyType: t} }

// File is one schema document to write.
type File struct {
	Name string
	Root protoreflect.MessageDescriptor
	Doc  string
}

// Files is every schema document schemagen emits.
//
// One file, not one per message. json2ts is pointed at a glob and turns each
// input file into an output file, so splitting the messages would need $ref
// across files and leave the TypeScript spread over several modules for no
// gain. Everything reachable from Declaration goes in $defs instead.
func Files() []File {
	return []File{{
		Name: "declaration.schema.json",
		Root: (&rigv1.Declaration{}).ProtoReflect().Descriptor(),
		Doc: "What a program declares to rig, once, at connect (PLAN.md " +
			"section 5e). It is data: no rig code runs inside the program to " +
			"produce it.\n\n" +
			"This describes the CANONICAL protojson form - the shape rigd " +
			"emits and the one to write. The wire additionally accepts the " +
			"proto spelling of a field name (coverage_note beside " +
			"coverageNote) and the numeric form of an enum; neither is " +
			"described here, and for that reason this schema does not close " +
			"the property set. It is deliberately never stricter than the " +
			"daemon: everything it refuses, registration refuses too.",
	}}
}

// JSON builds the document.
func (f File) JSON() ([]byte, error) {
	defs := map[string]any{}
	root, err := messageSchema(f.Root, defs)
	if err != nil {
		return nil, err
	}

	doc := map[string]any{
		"$schema":     dialect,
		"$id":         "https://rig.local/schema/" + f.Name,
		"title":       string(f.Root.Name()),
		"description": f.Doc,
	}
	for k, v := range root {
		doc[k] = v
	}
	if len(defs) > 0 {
		doc["$defs"] = defs
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// messageSchema describes one message, adding whatever it reaches to defs.
func messageSchema(md protoreflect.MessageDescriptor, defs map[string]any) (map[string]any, error) {
	props := map[string]any{}
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		s, err := fieldSchema(fd, defs)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", md.Name(), fd.Name(), err)
		}
		if extra := BoundsFor(fd); extra != nil {
			m, ok := s.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s.%s: bounds on a field that is a "+
					"reference, which would put the constraint on the shared "+
					"definition instead of this field", md.Name(), fd.Name())
			}
			for k, v := range extra {
				m[k] = v
			}
		}
		props[fd.JSONName()] = s
	}

	out := map[string]any{
		keyType:      "object",
		"properties": props,
	}
	if req := MandatoryFor(md); len(req) > 0 {
		out["required"] = req
	}
	return out, nil
}

// fieldSchema describes one field, following lists and references.
func fieldSchema(fd protoreflect.FieldDescriptor, defs map[string]any) (any, error) {
	if fd.IsMap() {
		return nil, errors.New("map fields are not described yet; the " +
			"declaration has none, and guessing at one would ship a contract " +
			"nobody checked")
	}
	base, err := scalarSchema(fd, defs)
	if err != nil {
		return nil, err
	}
	if fd.IsList() {
		return map[string]any{keyType: "array", "items": base}, nil
	}
	return base, nil
}

// scalarSchema maps one proto kind to its canonical protojson form.
func scalarSchema(fd protoreflect.FieldDescriptor, defs map[string]any) (any, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return typed("string"), nil

	case protoreflect.BoolKind:
		return typed("boolean"), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return typed("integer"), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		// protojson writes 64-bit integers as strings, because a JSON number
		// cannot hold one exactly, and accepts either form back.
		return typed([]string{"string", "integer"}), nil

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return typed("number"), nil

	case protoreflect.BytesKind:
		return map[string]any{
			keyType:           "string",
			"contentEncoding": "base64",
		}, nil

	case protoreflect.EnumKind:
		name := string(fd.Enum().Name())
		if _, seen := defs[name]; !seen {
			defs[name] = enumSchema(fd.Enum())
		}
		return map[string]any{"$ref": "#/$defs/" + name}, nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		md := fd.Message()
		name := string(md.Name())
		if _, seen := defs[name]; !seen {
			// Reserve the name before recursing, so a message that reaches
			// itself terminates instead of recursing forever.
			defs[name] = map[string]any{}
			s, err := messageSchema(md, defs)
			if err != nil {
				return nil, err
			}
			defs[name] = s
		}
		return map[string]any{"$ref": "#/$defs/" + name}, nil
	}
	return nil, fmt.Errorf("kind %s has no mapping", fd.Kind())
}

// enumSchema lists an enum's values WITHOUT its zero.
//
// Section 21: every proto enum reserves *_UNSPECIFIED = 0, an unset field and
// a field set to zero are the same bytes, and the boundary refuses zero. A
// schema that offered the zero would be offering the one value registration is
// guaranteed to reject, so it is dropped by rule rather than by a hand-kept
// list - and the drop is derived from the number, not from the name.
func enumSchema(ed protoreflect.EnumDescriptor) map[string]any {
	values := ed.Values()
	names := make([]string, 0, values.Len())
	for i := range values.Len() {
		v := values.Get(i)
		if v.Number() == 0 {
			continue
		}
		names = append(names, string(v.Name()))
	}
	return map[string]any{
		keyType: "string",
		"enum":  names,
		"description": fmt.Sprintf(
			"%s. The zero value is omitted: it means the program did not say, "+
				"and registration refuses it.", ed.FullName()),
	}
}
