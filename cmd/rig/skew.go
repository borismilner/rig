package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// SECTION 37, PRECONDITION 3: BUILD SKEW IS DETECTED, NOT DISCOVERED.
//
// Ruled by Boris on 2026-09-14, from three options: WARN, NEVER REFUSE. So a
// mismatch costs one line on stderr and nothing else - the exit code and stdout
// are exactly what they would have been, and an agent parsing --json sees no
// difference at all.
//
// The case this exists for is not exotic. build/rig from the development tree,
// run without an explicit XDG_RUNTIME_DIR, reaches PRODUCTION, because
// precondition 5 gives production the default runtime directory. That pair
// always differs by build string, and until this file nothing said so: the
// material was on the wire in rig.estate and no caller read it.
//
// BUILD ONLY. The semantic generation is still the open measurement in section
// 37 and is deliberately not compared here.

// unstamped is the build string of a binary nobody stamped: the compile-time
// default of `version` in this package and in cmd/rigd. The Makefile replaces
// it through -ldflags. Two unstamped builds carry the same word and can hold
// any two trees, so the word is never evidence of a match.
const unstamped = "dev"

// skewCheckTimeout is short because the check runs before the verb the caller
// actually asked for. A daemon too slow to answer rig.estate in a second will
// fail the real call on its own terms; this check must not be the reason
// anything waits longer.
const skewCheckTimeout = time.Second

// skewOut is where the warning goes. A variable only so a test can read what a
// real verb printed; nothing outside tests assigns it.
var skewOut io.Writer = os.Stderr

// connect is client.Connect plus the skew check, and every verb that reaches
// a daemon goes through it. TestEveryDaemonVerbChecksSkew walks this package
// so a verb added later cannot quietly skip it.
//
// Shell completion is the one exemption: it runs while someone holds the tab
// key, and a line on stderr there lands in the middle of their prompt.
func connect() (*client.Client, error) {
	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	checkSkew(skewOut, c, version)
	return c, nil
}

// checkSkew asks the daemon who it is and writes the one warning line, if
// there is one. It never returns an error: a skew check that fails the call it
// precedes would turn "warn, never refuse" into refusing by accident.
func checkSkew(w io.Writer, c *client.Client, build string) {
	ctx, cancel := context.WithTimeout(context.Background(), skewCheckTimeout)
	defer cancel()

	resp := &rigv1.EstateResponse{}
	err := call(ctx, c, "rig.estate", &rigv1.EstateRequest{}, resp)
	if line := skewLine(build, resp, err); line != "" {
		fmt.Fprintln(w, line)
	}
}

// skewLine is the whole policy, as a pure function so every row of the ruling
// is a test case. It returns "" only when both builds are stamped and equal.
func skewLine(build string, r *rigv1.EstateResponse, err error) string {
	const warn = "rig: warning: "

	// call() wraps a daemon refusal as *refusal, which embeds the CallError
	// rather than unwrapping to it, so both shapes are read here.
	var ce *client.CallError
	var r0 *refusal
	if errors.As(err, &r0) {
		ce = r0.CallError
	} else {
		errors.As(err, &ce)
	}
	switch {
	case ce != nil && ce.Code() == rigv1.Code_CODE_NOT_FOUND:
		// A daemon from before rig.estate existed. That is skew in itself,
		// and the direction is known: the daemon is the older half.
		return warn + "build skew: the daemon has no rig.estate, so it is " +
			"older than this rig (" + buildWord(build) + ")"
	case ce != nil:
		return warn + "build skew not checked: rig.estate was refused (" +
			ce.Status.GetMessage() + ")"
	case err != nil:
		return warn + "build skew not checked: rig.estate did not answer (" +
			err.Error() + ")"
	}

	daemon := r.GetDaemonVersion()
	where := skewEstate(r)
	switch {
	case isUnstamped(build):
		return warn + "build skew not checked: this rig is an unstamped build, " +
			"so it cannot be compared with " + where + " (" + buildWord(daemon) + ")"
	case isUnstamped(daemon):
		return warn + "build skew not checked: " + where + " is " +
			buildWord(daemon) + ", so it cannot be compared with this rig (" +
			build + ")"
	case build == daemon:
		return ""
	}
	return warn + "build skew: this rig is " + build + " and " + where +
		" is " + daemon + ". Use the rig built with that daemon, or set " +
		"XDG_RUNTIME_DIR to reach the estate you meant"
}

// isUnstamped covers the empty string as well as the default word. Section 21:
// an empty daemon_version means nothing was said, which is not a build.
func isUnstamped(b string) bool { return b == "" || b == unstamped }

// buildWord renders a build string so neither unstamped form can read as a
// real version in the sentence around it.
func buildWord(b string) string {
	switch b {
	case "":
		return "reporting no build"
	case unstamped:
		return "unstamped"
	}
	return b
}

// skewEstate names the daemon by the estate it serves, because "which rig did
// I reach" is the question the reader has to answer next.
func skewEstate(r *rigv1.EstateResponse) string {
	if r.GetName() == "" {
		return "the daemon of an estate with no name claimed"
	}
	return "the daemon of estate " + strconv.Quote(r.GetName())
}
