package daemon

import (
	"strings"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Q2: both syntaxes, and what is not a schedule is refused with its reason.
func TestScheduleSyntaxes(t *testing.T) {
	from := time.Date(2026, 10, 1, 8, 30, 0, 0, time.Local)
	for s, want := range map[string]time.Time{
		"every 15m":    from.Add(15 * time.Minute),
		"at 09:00":     time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local),
		"at 07:05":     time.Date(2026, 10, 2, 7, 5, 0, 0, time.Local),
		"0 12 * * 1-5": time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local),
		"@daily":       time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local),
	} {
		sched, err := parseSchedule(s)
		if err != nil {
			t.Errorf("%q: %v", s, err)
			continue
		}
		if got := sched.Next(from); !got.Equal(want) {
			t.Errorf("%q: next %s, want %s", s, got, want)
		}
	}
	for _, s := range []string{"", "every 10s", "every soon", "at 25:00", "at noon", "* * *", "every 1s" + strings.Repeat(" ", maxTimerSchedule)} {
		if _, err := parseSchedule(s); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}
}

// Q3: a timer that fell due many times while the machine slept fires once,
// counting the rest, and its next fire is after now, not in the past.
func TestASleptThroughTimerFiresOnceCountingTheMissed(t *testing.T) {
	start := time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local)
	now := start
	td := &timerDesk{now: func() time.Time { return now }}
	sched, _ := parseSchedule("every 15m")
	if _, _, err := td.arm("graft", "sweep", "every 15m", sched); err != nil {
		t.Fatal(err)
	}
	if fires := td.due(now); len(fires) != 0 {
		t.Fatalf("fired before due: %v", fires)
	}
	now = start.Add(2*time.Hour + time.Minute) // due 8:15, then 7 more by 10:01
	fires := td.due(now)
	if len(fires) != 1 || fires[0].owner != "graft" || fires[0].fired.GetMissed() != 7 ||
		fires[0].fired.GetDueUnixNano() != start.Add(15*time.Minute).UnixNano() {
		t.Fatalf("after the sleep: %v", fires)
	}
	if next := td.list("graft")[0].GetNextUnixNano(); next != now.Add(15*time.Minute).UnixNano() {
		t.Fatalf("next fire %s", time.Unix(0, next))
	}
	if fires := td.due(now); len(fires) != 0 {
		t.Fatalf("fired twice: %v", fires)
	}
}

// Arming again with the same schedule changes nothing, another replaces it,
// and a program's timers are bounded.
func TestArmingIsIdempotentAndBounded(t *testing.T) {
	td := &timerDesk{}
	every, _ := parseSchedule("every 1h")
	daily, _ := parseSchedule("@daily")
	first, _, _ := td.arm("graft", "a", "every 1h", every)
	again, replaced, _ := td.arm("graft", "a", "every 1h", every)
	if replaced || again.GetNextUnixNano() != first.GetNextUnixNano() {
		t.Fatalf("re-arming moved it: %v then %v", first, again)
	}
	if _, replaced, _ := td.arm("graft", "a", "@daily", daily); !replaced {
		t.Fatal("a new schedule did not replace the old")
	}
	for i := 1; i < maxTimersPerProgram; i++ {
		if _, _, err := td.arm("graft", strings.Repeat("x", i), "@daily", daily); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := td.arm("graft", "one-more", "@daily", daily); err == nil {
		t.Fatal("armed past the bound")
	}
	if _, _, err := td.arm("shelf", "a", "@daily", daily); err != nil {
		t.Fatalf("another program was bounded by graft's timers: %v", err)
	}
	if !td.disarm("graft", "a") || td.disarm("graft", "a") {
		t.Fatal("disarm did not answer whether it removed one")
	}
}

// Over the wire: only a registered program arms, it lists only its own, and
// a fire reaches its owner and nobody else.
func TestATimerFiresToItsOwnerOnly(t *testing.T) {
	floor := minTimerEvery
	t.Cleanup(func() { minTimerEvery = floor }) // after the daemon stops
	minTimerEvery = time.Second
	sock, _ := upDaemon(t, nil)
	ctx := ctx5(t)
	graft := eventProgram(t, sock, "graft")
	shelf := eventProgram(t, sock, "shelf")
	term := dial(t, sock)

	err := term.Call(ctx, "rig.timer.arm", &registryv1.TimerArmRequest{Name: "a", Schedule: "every 1s"}, &registryv1.TimerArmResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a terminal arming")
	err = graft.Call(ctx, "rig.timer.arm", &registryv1.TimerArmRequest{Name: "a", Schedule: "every 1ms"}, &registryv1.TimerArmResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a schedule under the floor")
	err = graft.Call(ctx, "rig.timer.arm", &registryv1.TimerArmRequest{Name: "has space", Schedule: "@daily"}, &registryv1.TimerArmResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a name with a space")

	var armed registryv1.TimerArmResponse
	if err := graft.Call(ctx, "rig.timer.arm", &registryv1.TimerArmRequest{Name: "tick", Schedule: "every 1s"}, &armed); err != nil {
		t.Fatal(err)
	}
	var mine, theirs registryv1.TimerListResponse
	_ = graft.Call(ctx, "rig.timer.list", &registryv1.TimerListRequest{}, &mine)
	_ = shelf.Call(ctx, "rig.timer.list", &registryv1.TimerListRequest{}, &theirs)
	if len(mine.GetTimers()) != 1 || len(theirs.GetTimers()) != 0 {
		t.Fatalf("graft lists %v, shelf lists %v", &mine, &theirs)
	}

	var got registryv1.EventsWaitResponse
	if err := graft.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{"timer.fired"}, TimeoutMs: 3000}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.GetEvents()) == 0 || !strings.Contains(got.GetEvents()[0].GetPayloadJson(), `"name":"tick"`) {
		t.Fatalf("graft waited and got %v", &got)
	}
	var other registryv1.EventsWaitResponse
	if err := shelf.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{"timer.fired"}, TimeoutMs: 1}, &other); err != nil {
		t.Fatal(err)
	}
	if len(other.GetEvents()) != 0 {
		t.Fatalf("shelf saw graft's timer: %v", &other)
	}

	var gone registryv1.TimerDisarmResponse
	if err := graft.Call(ctx, "rig.timer.disarm", &registryv1.TimerDisarmRequest{Name: "tick"}, &gone); err != nil || !gone.GetDisarmed() {
		t.Fatalf("disarm: %v %v", &gone, err)
	}
}
