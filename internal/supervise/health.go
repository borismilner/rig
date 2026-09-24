package supervise

import "time"

// HealthPolicy is PLAN.md section 18's health definition, as knobs.
//
// ⛔ HEALTH IS EVIDENCE OF PROGRESS, NOT OF RESPONSIVENESS, and the section
// is explicit about why: "a renewal emitted by a background thread proves the
// process runs and nothing else, which is the check that cannot fail and
// therefore cannot help". So the input is a marker the program ADVANCES,
// never a probe rig sends - and idle-and-not-blocked is unhealthy rather than
// a rest state.
//
// NOT YET WIRED: the supervisor loop that reads these lands with
// supervisor.go. The type is here because Spec holds it and the declared
// programs file deserialises into it.
type HealthPolicy struct {
	// Interval is how often health is judged. Section 18: "on the interval,
	// with a timeout".
	Interval time.Duration

	// Timeout bounds one judgement.
	Timeout time.Duration

	// Idle is the threshold under "Idle-and-not-blocked is UNHEALTHY, with a
	// threshold". A marker that has not moved for this long, with nothing
	// declared as waited for, is one failure.
	Idle time.Duration

	// Degraded is how many consecutive failures make a program DEGRADED.
	// Section 18: "Three failures is degraded".
	Degraded int

	// Restart is how many make the restart budget take it. Section 18:
	// "five is a restart" - on the same counter, not five more.
	Restart int
}

// DefaultHealth is section 18's own numbers, and the two counts are the
// section's rather than a choice: three failures is degraded, five is a
// restart.
func DefaultHealth() HealthPolicy {
	return HealthPolicy{
		Interval: 5 * time.Second,
		Timeout:  2 * time.Second,
		Idle:     30 * time.Second,
		Degraded: 3,
		Restart:  5,
	}
}

// Budget is section 18's restart budget: "exponential backoff inside a
// budget. Exceeding it is quarantine - a visible state with the full history
// and a manual restart, never a silent disappearance."
//
// NOT YET WIRED: the countdown and the backoff clock land with supervisor.go.
type Budget struct {
	// Restarts is how many rig will do inside Window before quarantining.
	Restarts int

	// Window is the span the count is taken over.
	Window time.Duration

	// Backoff is the first wait. Each restart doubles it, up to MaxBackoff.
	Backoff    time.Duration
	MaxBackoff time.Duration
}

// DefaultBudget is a starting point rather than a ruling: section 18 fixes
// the SHAPE (exponential, inside a budget, quarantine on exhaustion) and
// names no numbers, so these are declared per program in the programs file
// and these are what a program that declares none gets.
func DefaultBudget() Budget {
	return Budget{
		Restarts:   5,
		Window:     10 * time.Minute,
		Backoff:    time.Second,
		MaxBackoff: time.Minute,
	}
}
