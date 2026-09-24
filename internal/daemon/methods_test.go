package daemon

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// ⛔ THE HANDSHAKE SAYS WHAT THIS DAEMON SERVES, AND EVERY METHOD IT NAMES IS
// REAL. HelloResponse.methods is how a program checks for a verb before
// calling it. A list that named a method the dispatcher answers with "no such
// method" would be worse than no list, so every entry is called here and must
// reach a handler: any answer is fine except that one.
func TestHelloListsEveryServedMethodAndOnlyThose(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := c.Hello(ctx, testDeclaration("probe"))
	if err != nil {
		t.Fatal(err)
	}
	m := resp.GetMethods()
	if !sort.StringsAreSorted(m) {
		t.Errorf("methods are not sorted: %v", m)
	}
	for _, want := range []string{
		"rig.hello", "rig.ping", "rig.describe",
		"rig.record.refs", "rig.lease.acquire", "rig.programs",
	} {
		if !slices.Contains(m, want) {
			t.Errorf("methods does not list %s: %v", want, m)
		}
	}

	other := dial(t, sock)
	for _, method := range m {
		if method == "rig.hello" || method == "rig.down" {
			continue // hello would register a second program; down stops the daemon
		}
		err := other.Call(ctx, method, &rigv1.PingRequest{}, &rigv1.PingResponse{})
		var ce *client.CallError
		if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND &&
			strings.HasPrefix(ce.Status.GetMessage(), "no such method") {
			t.Errorf("%s is listed as served and the daemon says: %s", method, ce.Status.GetMessage())
		}
	}
}
