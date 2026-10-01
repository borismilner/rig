package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig hand status|allow|decline|hold|pause|resume|stop` - the HANDS OFF
// strip's buttons from a terminal (plan/05 section 5m). status reads the one
// run; every other word is his answer to it, which rigd refuses to a program
// and to an agent's MCP door (H5).

// handActions are the answers, by the word he types.
var handActions = map[string]registryv1.HandAction{
	"allow":   registryv1.HandAction_HAND_ACTION_ALLOW,
	"decline": registryv1.HandAction_HAND_ACTION_DECLINE,
	"hold":    registryv1.HandAction_HAND_ACTION_HOLD,
	"pause":   registryv1.HandAction_HAND_ACTION_PAUSE,
	"resume":  registryv1.HandAction_HAND_ACTION_RESUME,
	"stop":    registryv1.HandAction_HAND_ACTION_STOP,
}

const handUsage = "usage: rig hand status|allow|decline|hold|pause|resume|stop"

func cmdHand(args []string) (err error) {
	fs := flag.NewFlagSet("hand", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) != 1 {
		return badArgumentf(handUsage)
	}
	action, isAction := handActions[positional[0]]
	if !isAction && positional[0] != "status" {
		return badArgumentf("%q is not a hand command; %s", positional[0], handUsage)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	var st *registryv1.HandState
	if isAction {
		var resp registryv1.HandAnswerResponse
		if err := call(ctx, c, "rig.hand.answer", &registryv1.HandAnswerRequest{Action: action}, &resp); err != nil {
			return err
		}
		st = resp.GetState()
	} else {
		// after 0 with no wait answers at once: seq starts at 1.
		var resp registryv1.HandWaitResponse
		if err := call(ctx, c, "rig.hand.wait", &registryv1.HandWaitRequest{}, &resp); err != nil {
			return err
		}
		st = resp.GetState()
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(handJSON(st, time.Now()))
	}
	fmt.Println(handLine(st, time.Now()))
	return nil
}

// handLeft is how long until st's deadline, never negative, to the second.
func handLeft(st *registryv1.HandState, now time.Time) time.Duration {
	d := time.Duration(st.GetDeadlineUnixNano() - now.UnixNano())
	return max(d, 0).Round(time.Second)
}

// handOut is status --json. The deadline fields are left out when the phase
// has none.
type handOut struct {
	Seq             uint64  `json:"seq"`
	Phase           string  `json:"phase"`
	Holder          string  `json:"holder"`
	Reason          string  `json:"reason"`
	Activity        string  `json:"activity"`
	Ended           string  `json:"ended"`
	DeadlineInMs    *int64  `json:"deadline_in_ms,omitempty"`
	CountdownLeftMs *uint32 `json:"countdown_left_ms,omitempty"`
}

func handJSON(st *registryv1.HandState, now time.Time) handOut {
	out := handOut{
		Seq: st.GetSeq(), Phase: enumLabel(st.GetPhase().String(), "HAND_PHASE_"),
		Holder: st.GetHolder(), Reason: st.GetReason(), Activity: st.GetActivity(), Ended: st.GetEnded(),
	}
	if st.GetDeadlineUnixNano() != 0 {
		in := handLeft(st, now).Milliseconds()
		out.DeadlineInMs = &in
	}
	if st.GetPhase() == registryv1.HandPhase_HAND_PHASE_HELD {
		left := st.GetLeftMs()
		out.CountdownLeftMs = &left
	}
	return out
}

// handLine is the state in one line, with the words that answer it.
func handLine(st *registryv1.HandState, now time.Time) string {
	who, why := st.GetHolder(), ""
	if st.GetReason() != "" {
		why = " (" + st.GetReason() + ")"
	}
	switch st.GetPhase() {
	case registryv1.HandPhase_HAND_PHASE_ASKING:
		return fmt.Sprintf("%s asks for the desktop%s: it starts in %s unless you answer.\n"+
			"rig hand allow | hold | decline", who, why, handLeft(st, now))
	case registryv1.HandPhase_HAND_PHASE_HELD:
		left := (time.Duration(st.GetLeftMs()) * time.Millisecond).Round(time.Second)
		return fmt.Sprintf("%s's countdown is held with %s left%s; it declines itself in %s.\n"+
			"rig hand resume | allow | decline", who, left, why, handLeft(st, now))
	case registryv1.HandPhase_HAND_PHASE_DRIVING:
		return fmt.Sprintf("HANDS OFF: %s drives the desktop%s, for %s%s.\n"+
			"rig hand pause | stop", who, why, handSince(st, now), handDoing(st))
	case registryv1.HandPhase_HAND_PHASE_PAUSED:
		return fmt.Sprintf("You have the desktop back from %s%s; the run stops itself in %s.\n"+
			"rig hand resume | stop", who, handDoing(st), handLeft(st, now))
	}
	if st.GetEnded() == "" {
		return "Nobody has the desktop."
	}
	return fmt.Sprintf("Nobody has the desktop. The last run, %s, ended: %s.", who, st.GetEnded())
}

func handSince(st *registryv1.HandState, now time.Time) time.Duration {
	return max(now.Sub(time.Unix(0, st.GetDrivingUnixNano())), 0).Round(time.Second)
}

func handDoing(st *registryv1.HandState) string {
	if a := strings.TrimSpace(st.GetActivity()); a != "" {
		return ", at " + a
	}
	return ""
}
