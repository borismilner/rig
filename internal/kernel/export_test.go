package kernel

// MapVersion exposes the capability map's digest to this package's external
// tests, so TestEveryProgramFieldChangesTheVersion can walk a Program by
// reflection without the digest becoming part of rig's API.
//
// The alternative was an exported helper on the real type, which is a
// production surface existing only for a test - and a surface a later reader
// has to decide whether to keep.
var MapVersion = mapVersion
