package meta

import "reflect"

// PartialForTest exposes the coverage warning's construction to this
// package's external tests, so the unspecified-coverage case can be reached
// without a registration that refuses it.
var PartialForTest = partialOf

// AnswerJSONShape exposes the emitted object's struct tags to this package's
// external tests.
//
// It exists because a behavioural test cannot see the difference that matters
// here. `depth` renders as an enum NAME, and "unspecified" is not the empty
// string, so `omitempty` on it is inert TODAY and a test that marshals an
// answer and looks for the key passes either way. The day someone renders the
// zero as "" - which is the obvious "tidy-up" - omitempty would start dropping
// the key, and the absent-versus-too-old ambiguity the field exists to prevent
// comes back silently. So the tag is asserted directly.
var AnswerJSONShape = reflect.TypeOf(answerJSON{})

// CapabilityMapJSONShape exposes the resource object's struct tags to this
// package's external tests.
//
// It exists for the reason AnswerJSONShape does, and the trap is the same
// one: `depth` renders as an enum NAME, so "unspecified" is not the empty
// string and an `omitempty` added to it would be inert on the day it was
// written. The four fields of this object are emitted ALWAYS - that is its
// whole shape argument - and nothing behavioural can see an omitempty that is
// not yet doing damage.
var CapabilityMapJSONShape = reflect.TypeOf(capabilityMapJSON{})
