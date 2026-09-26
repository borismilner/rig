package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

// plan/48 R14-R15: the flag beats RIG_ROOT, RIG_ROOT beats ~/.rig.
func TestRootIsTheFlagThenTheEnvironmentThenHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv(RootEnv, "")
	if got, err := Root(""); err != nil || got != filepath.Join(home, ".rig") {
		t.Fatalf("default root = %q, %v; want %q", got, err, filepath.Join(home, ".rig"))
	}

	t.Setenv(RootEnv, "/srv/rig-data/")
	if got, err := Root(""); err != nil || got != "/srv/rig-data" {
		t.Fatalf("root with %s set = %q, %v", RootEnv, got, err)
	}

	if got, err := Root("/mnt/elsewhere"); err != nil || got != "/mnt/elsewhere" {
		t.Fatalf("the flag did not beat the environment: %q, %v", got, err)
	}
}

// A relative root would resolve against wherever the daemon was started.
func TestARelativeRootIsRefusedAndNamesItsSource(t *testing.T) {
	t.Setenv(RootEnv, "relative/dir")
	if _, err := Root(""); err == nil || !strings.Contains(err.Error(), RootEnv) {
		t.Fatalf("a relative %s was not refused by name: %v", RootEnv, err)
	}
	if _, err := Root("./x"); err == nil || !strings.Contains(err.Error(), "--root") {
		t.Fatalf("a relative --root was not refused by name: %v", err)
	}
}

// An unnamed estate must never land in the live store through the
// environment: every test here starts one with the developer's RIG_ROOT.
func TestAnUnnamedEstateIgnoresTheEnvironment(t *testing.T) {
	rt := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", rt)
	t.Setenv(RootEnv, "/srv/live")
	got, err := ScratchRoot("")
	if err != nil || got != filepath.Join(rt, "rig", "root") {
		t.Fatalf("scratch root = %q, %v; want it under the runtime dir", got, err)
	}
	if got, _ := ScratchRoot("/tmp/told"); got != "/tmp/told" {
		t.Fatalf("an explicit --root was not honoured for an unnamed estate: %q", got)
	}
}

// R16: the three areas are siblings, one subtree per estate.
func TestTheThreeAreasAreSiblingsUnderTheEstate(t *testing.T) {
	a, err := EstateAreas("/r", "production")
	if err != nil {
		t.Fatal(err)
	}
	want := Areas{
		Internal: "/r/estates/production/internal",
		Files:    "/r/estates/production/files",
		Exports:  "/r/estates/production/exports",
	}
	if a != want {
		t.Fatalf("areas = %+v, want %+v", a, want)
	}
	for _, p := range []string{a.Files, a.Exports} {
		if strings.HasPrefix(p+"/", a.Internal+"/") || strings.HasPrefix(a.Internal+"/", p+"/") {
			t.Fatalf("%s and %s nest", p, a.Internal)
		}
	}
}

// The estate name becomes a path element, so a bad one never reaches Join.
func TestAnEstateNameThatIsAPathIsRefused(t *testing.T) {
	for _, name := range []string{"", "../x", "a/b", "production/../../etc"} {
		if _, err := EstateAreas("/r", name); err == nil {
			t.Errorf("estate name %q was accepted as a path element", name)
		}
	}
}
