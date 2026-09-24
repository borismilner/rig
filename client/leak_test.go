package client

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if any test leaves a goroutine running after it
// returns: a goroutine that outlives its test is a leak in the code under
// test or in the test, and rigd is a long-lived daemon where either one
// accumulates.
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }
