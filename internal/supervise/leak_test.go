package supervise

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if any test leaves a goroutine running after it
// returns.
//
// IT MATTERS MORE HERE THAN ALMOST ANYWHERE. This package starts a goroutine
// per supervised child to wait on it, and a restart starts another - so a
// supervisor that leaked one per restart would leak on exactly the path it
// exists to handle, inside a daemon that runs for weeks.
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }
