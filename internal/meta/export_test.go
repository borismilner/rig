package meta

// PartialForTest exposes the coverage warning's construction to this
// package's external tests, so the unspecified-coverage case can be reached
// without a registration that refuses it.
var PartialForTest = partialOf
