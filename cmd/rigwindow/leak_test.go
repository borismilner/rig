package main

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if a test leaves a goroutine running: the tray
// is resident for the whole session, so anything it leaks accumulates.
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }
