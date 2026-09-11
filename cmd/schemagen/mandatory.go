package main

import "google.golang.org/protobuf/reflect/protoreflect"

// mandatory is which properties registration refuses when they are unsaid,
// per message, by proto field name.
//
// THIS IS THE ONE THING THE DESCRIPTORS CANNOT TELL US. proto3 has no
// required, and section 5e's mandatory set is deliberately not the same as
// "every field": a program that declares nothing but commands is valid, and
// most of Command is optional. So the set is written here, mirroring the
// kernel's own refusals (Declaration.Validate and Command.missing in
// internal/kernel), and TestMandatorySetMatchesWhatRegistrationRefuses fails
// if the two drift apart.
//
// Proto field names rather than JSON names, because the proto is the source
// and the JSON name is derived from it by the descriptor.
var mandatory = map[protoreflect.Name][]protoreflect.Name{
	"Declaration": {
		"identity",
		"coverage",
		"semantics_gen",
	},
	"Identity": {
		"id",
		"version",
	},
	"Command": {
		"id",
		"effects",
		"idempotent",
		// sensitive is mandatory AND MAY BE EMPTY. The wrapper message is what
		// carries that difference, so what is required here is the wrapper's
		// presence, not any pointer inside it.
		"sensitive",
		"interactive",
		"streams",
		"needs_display",
		"duration",
		"confirms",
		"shape",
		"summary",
		"description",
		"returns",
	},
	// SensitiveFields has none on purpose: a present wrapper with no pointers
	// is the valid way to say "this command has no sensitive fields".
	"SensitiveFields": nil,
}

// MandatoryFor returns the JSON names registration refuses to do without.
//
// Returned in the descriptor's field order rather than the order they are
// written above, so the emitted file does not reshuffle when the list here is
// re-sorted, and a diff of the schema shows a real change or nothing.
func MandatoryFor(md protoreflect.MessageDescriptor) []string {
	want := map[protoreflect.Name]bool{}
	for _, n := range mandatory[md.Name()] {
		want[n] = true
	}
	if len(want) == 0 {
		return nil
	}

	var out []string
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if want[fd.Name()] {
			out = append(out, fd.JSONName())
		}
	}
	return out
}

// bounds is what registration enforces beyond mere presence.
//
// Keyed by the field's full proto name so there is no chance of applying a
// Declaration rule to a same-named field on another message. Same drift
// hazard as the mandatory set above, and the same test covers it.
var bounds = map[protoreflect.FullName]map[string]any{
	// kernel refuses semantics_gen <= 0: it pins what every declared name
	// means for this program's lifetime, so it cannot be absent (section 21).
	"rig.v1.Declaration.semantics_gen": {"minimum": 1},
}

// BoundsFor returns the extra constraints on one field, or nil.
func BoundsFor(fd protoreflect.FieldDescriptor) map[string]any {
	return bounds[fd.FullName()]
}
