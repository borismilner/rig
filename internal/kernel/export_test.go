package kernel

// MapVersion exposes the capability map's digest to this package's external
// tests, so TestEveryProgramFieldChangesTheVersion can walk a Program by
// reflection without the digest becoming part of rig's API.
//
// The alternative was an exported helper on the real type, which is a
// production surface existing only for a test - and a surface a later reader
// has to decide whether to keep.
// It takes the depth and the programs rather than the map, because that is
// what every caller here means and a map literal at each call site would say
// nothing extra. The map it builds is the map those two arguments describe.
func MapVersion(d Depth, programs []Program) string {
	return mapVersion(CapabilityMap{Depth: d, Programs: programs})
}

// MapVersionOf digests a whole map, which is what mapVersion now takes.
//
// It exists because MapVersion above cannot express the case the structural
// walk is for: a field of CapabilityMap that is neither the depth nor the
// programs. Walking the map's own fields needs to hand the digest a map.
var MapVersionOf = mapVersion
