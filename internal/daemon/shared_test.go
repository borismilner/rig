package daemon

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/coord"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/53 slice 3: the shared table over rig's wire. What these prove is what
// an agent relied on AgentBox's `shared` for, and where rig's beats it.

func sharedSet(t *testing.T, c *client.Client, key, value string, expected uint64, own bool) *verbsv1.SharedSetResponse {
	t.Helper()
	var resp verbsv1.SharedSetResponse
	if err := c.Call(recordCtx(t), "rig.shared.set", &verbsv1.SharedSetRequest{
		Key: key, ValueJson: value, ExpectedVersion: expected, Own: own,
	}, &resp); err != nil {
		t.Fatalf("setting %s: %v", key, err)
	}
	return &resp
}

func sharedGet(t *testing.T, c *client.Client, key string) *verbsv1.SharedGetResponse {
	t.Helper()
	var resp verbsv1.SharedGetResponse
	if err := c.Call(recordCtx(t), "rig.shared.get", &verbsv1.SharedGetRequest{Key: key}, &resp); err != nil {
		t.Fatalf("reading %s: %v", key, err)
	}
	return &resp
}

// ⛔ TWO SEATS, ONE ITEM, ONE WINNER, and the loser is told who and what to
// do, as a result rather than an error.
func TestTwoSeatsClaimingOneItemGetOneWinner(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	b := seated(t, sock, "seat-b")

	won := sharedSet(t, a, "claims.chunk-7", `{"by":"a"}`, 0, true)
	if !won.GetApplied() || won.GetValue().GetOwner() != "seat-a" || won.GetValue().GetBy() != "seat-a" {
		t.Fatalf("first claim: %+v", won)
	}
	lost := sharedSet(t, b, "claims.chunk-7", `{"by":"b"}`, 0, true)
	if lost.GetApplied() || !lost.GetStale() || lost.GetValue().GetOwner() != "seat-a" ||
		!strings.Contains(lost.GetNote(), "still here") {
		t.Fatalf("second claim: %+v", lost)
	}
	got := sharedGet(t, b, "claims.chunk-7")
	if got.GetValue().GetValueJson() != `{"by":"a"}` || got.GetValue().GetOwnerGone() {
		t.Fatalf("read: %+v", got)
	}
}

// Reads need no seat. A write's refusal without one is the agent door's
// case, pinned in mcp_verbs_test.go.
func TestAReadNeedsNoSeat(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	sharedSet(t, a, "note", `1`, 0, false)
	if got := sharedGet(t, dial(t, sock), "note"); !got.GetFound() || got.GetValue().GetBy() != "seat-a" {
		t.Fatalf("a read: %+v", got)
	}
}

func TestWhatTheTableRefuses(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	for _, tc := range []struct {
		why string
		req *verbsv1.SharedSetRequest
	}{
		{"a slash in a key", &verbsv1.SharedSetRequest{Key: "claims/x", ValueJson: `1`}},
		{"a capital in a key", &verbsv1.SharedSetRequest{Key: "Claims", ValueJson: `1`}},
		{"a wildcard key", &verbsv1.SharedSetRequest{Key: "claims.*", ValueJson: `1`}},
		{"no value", &verbsv1.SharedSetRequest{Key: "k"}},
		{"a value that is not JSON", &verbsv1.SharedSetRequest{Key: "k", ValueJson: `{`}},
		{"a value over 16 KiB", &verbsv1.SharedSetRequest{Key: "k", ValueJson: `"` + strings.Repeat("x", 16<<10) + `"`}},
	} {
		err := a.Call(recordCtx(t), "rig.shared.set", tc.req, &verbsv1.SharedSetResponse{})
		wantCode(t, err, rigv1.Code_CODE_INVALID, tc.why)
	}
	v := sharedSet(t, a, "k", `1`, 0, false).GetValue().GetVersion()
	err := a.Call(recordCtx(t), "rig.shared.delete", &verbsv1.SharedDeleteRequest{Key: "k"}, &verbsv1.SharedDeleteResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a delete without a version")
	var del verbsv1.SharedDeleteResponse
	if err := a.Call(recordCtx(t), "rig.shared.delete", &verbsv1.SharedDeleteRequest{Key: "k", ExpectedVersion: v}, &del); err != nil || !del.GetApplied() {
		t.Fatalf("a delete at the read version: %+v %v", &del, err)
	}
}

// ⛔ WAITING ON A FAMILY IS ONE events.wait, and every change says who made
// it and at what version, never the value.
func TestEveryChangeIsPostedUnderItsKey(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	watch := dial(t, sock)
	cursor := waitOn(t, watch, 0, 1, "shared.claims.*").GetLatest()
	a := seated(t, sock, "seat-a")

	v := sharedSet(t, a, "claims.one", `{"secret":"never on the bus"}`, 0, true).GetValue().GetVersion()
	sharedSet(t, a, "unrelated", `1`, 0, false)
	var del verbsv1.SharedDeleteResponse
	if err := a.Call(recordCtx(t), "rig.shared.delete", &verbsv1.SharedDeleteRequest{Key: "claims.one", ExpectedVersion: v}, &del); err != nil {
		t.Fatal(err)
	}

	var seen []string
	for len(seen) < 2 {
		got := waitOn(t, watch, cursor, 5000, "shared.claims.*")
		if len(got.GetEvents()) == 0 {
			t.Fatalf("shared.claims.* said only %v", seen)
		}
		for _, ev := range got.GetEvents() {
			if strings.Contains(ev.GetPayloadJson(), "secret") {
				t.Fatalf("a value rode the bus: %s", ev.GetPayloadJson())
			}
			var p sharedChange
			_ = json.Unmarshal([]byte(ev.GetPayloadJson()), &p)
			seen = append(seen, ev.GetKind()+":"+p.Change+":"+p.By)
			cursor = ev.GetSeq()
		}
	}
	if got, want := strings.Join(seen, " "), "shared.claims.one:set:seat-a shared.claims.one:deleted:seat-a"; got != want {
		t.Fatalf("posted %q, want %q", got, want)
	}
}

// ⛔ AN ABANDONED CLAIM IS TOLD, NOT ONLY FOUND: when its owner leaves and
// its process dies, rig posts owner_gone, the read says so, and a peer takes
// it over at its version. AgentBox said so only to whoever read.
func TestAnAbandonedClaimIsPostedAndTakenOver(t *testing.T) {
	sock, st := upLeaseDaemon(t)
	watch := dial(t, sock)
	cursor := waitOn(t, watch, 0, 1, "shared.*").GetLatest()

	// The owner's seat, and a process standing in for its session.
	owner := seated(t, sock, "seat-dead")
	proc := exec.Command("sleep", "30")
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	w, err := coord.WitnessProcess(proc.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	claim, _, err := st.SharedSet("claims.chunk-3", json.RawMessage(`{}`), 0, "seat-dead", w, "seat-dead")
	if err != nil {
		t.Fatal(err)
	}
	b := seated(t, sock, "seat-b")
	// The roster is asked first: a seat still here is live whatever its
	// process looks like, which is the reconnect and the restart window.
	_ = proc.Process.Kill()
	_ = proc.Wait()
	if got := sharedGet(t, b, "claims.chunk-3"); got.GetValue().GetOwnerGone() {
		t.Fatal("a claim whose owner is on the roster read as gone")
	}

	_ = owner.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got := waitOn(t, watch, cursor, 1000, "shared.*")
		found := false
		for _, ev := range got.GetEvents() {
			cursor = ev.GetSeq()
			var p sharedChange
			_ = json.Unmarshal([]byte(ev.GetPayloadJson()), &p)
			found = found || p.Change == "owner_gone" && p.Owner == "seat-dead" && p.Version == claim.Version
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner_gone was never posted")
		}
	}

	got := sharedGet(t, b, "claims.*")
	if len(got.GetValues()) != 1 || !got.GetValues()[0].GetOwnerGone() || !strings.Contains(got.GetNote(), "gone") {
		t.Fatalf("the family read: %+v", got)
	}
	if lost := sharedSet(t, b, "claims.chunk-3", `{}`, 0, true); lost.GetApplied() || !strings.Contains(lost.GetNote(), "abandoned") {
		t.Fatalf("claiming from empty over an abandoned claim: %+v", lost)
	}
	took := sharedSet(t, b, "claims.chunk-3", `{}`, claim.Version, true)
	if !took.GetApplied() || took.GetValue().GetOwner() != "seat-b" || took.GetValue().GetOwnerGone() {
		t.Fatalf("the takeover: %+v", took)
	}
}
