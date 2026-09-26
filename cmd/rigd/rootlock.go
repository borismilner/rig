package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/paths"
)

// ⛔ ONE DAEMON PER STORAGE ROOT. The estate claim is keyed by XDG_STATE_HOME,
// so two services on separate state homes (rigd.service and rig-team.service)
// both win it, and before this both defaulted to ~/.rig and would have shared
// one git repository and one index. The lock lives in the estate's internal
// area, so it is keyed by the directory the files are actually written to.

// rootLockName is the lock file in the estate's internal area.
const rootLockName = "rigd.lock"

// RootHeldError means another daemon already keeps its storage under root.
type RootHeldError struct {
	Root      string
	Incumbent int
}

func (e *RootHeldError) Error() string {
	who := "pid unknown"
	if e.Incumbent > 0 {
		who = fmt.Sprintf("pid %d", e.Incumbent)
	}
	return fmt.Sprintf("the storage root %s is already used by another rigd (%s); "+
		"give this daemon its own with --root or %s", e.Root, who, paths.RootEnv)
}

// claimRoot takes the storage root's lock, held until the returned lock is
// closed or the process dies.
func claimRoot(estate, root string) (*instance.Lock, error) {
	var areas paths.Areas
	var err error
	if estate == "" {
		areas, err = paths.ScratchAreas(root)
	} else {
		areas, err = paths.EstateAreas(root, estate)
	}
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(areas.Internal, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", areas.Internal, err)
	}
	l, err := instance.Acquire(filepath.Join(areas.Internal, rootLockName))
	var held *instance.HeldError
	if errors.As(err, &held) {
		return nil, &RootHeldError{Root: root, Incumbent: held.Incumbent}
	}
	return l, err
}
