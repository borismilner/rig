package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig up|stop|restart|health` - section 18's supervision at the prompt.
//
//	rig up [<program>...]
//	rig stop <program>
//	rig restart <program>
//	rig health [<program>...]
//
// The programs are the ones declared in $XDG_CONFIG_HOME/rig/programs.json.
// A program's own progress report is not here: it is called by the program,
// over the connection rig supervises, and a terminal is not that connection.

func cmdUp(args []string) error      { return superviseVerb("up", args) }
func cmdStop(args []string) error    { return superviseVerb("stop", args) }
func cmdRestart(args []string) error { return superviseVerb("restart", args) }
func cmdHealth(args []string) error  { return superviseVerb("health", args) }

func superviseVerb(verb string, args []string) (err error) {
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	one := verb == "stop" || verb == "restart"
	if one && len(positional) != 1 {
		return badArgumentf("usage: rig %s <program>", verb)
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var got []*verbsv1.ProgramHealth
	switch verb {
	case "up":
		var resp verbsv1.UpResponse
		err = call(ctx, c, "rig.up", &verbsv1.UpRequest{Programs: positional}, &resp)
		got = resp.GetPrograms()
	case "stop":
		var resp verbsv1.StopResponse
		err = call(ctx, c, "rig.stop", &verbsv1.StopRequest{Program: positional[0]}, &resp)
		got = []*verbsv1.ProgramHealth{resp.GetProgram()}
	case "restart":
		var resp verbsv1.RestartResponse
		err = call(ctx, c, "rig.restart", &verbsv1.RestartRequest{Program: positional[0]}, &resp)
		got = []*verbsv1.ProgramHealth{resp.GetProgram()}
	default:
		var resp verbsv1.HealthResponse
		err = call(ctx, c, "rig.health", &verbsv1.HealthRequest{Programs: positional}, &resp)
		got = resp.GetPrograms()
	}
	if err != nil {
		return err
	}
	if *asJSON {
		all := make([]programOut, 0, len(got))
		for _, h := range got {
			all = append(all, programJSON(h))
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Programs []programOut `json:"programs"`
		}{all})
	}
	if len(got) == 0 {
		fmt.Println("no programs are declared. Declare them in $XDG_CONFIG_HOME/rig/programs.json.")
		return nil
	}
	var b strings.Builder
	rows := make([][]string, 0, len(got))
	for _, h := range got {
		pid := "-"
		if h.GetPid() != 0 {
			pid = strconv.Itoa(int(h.GetPid()))
		}
		rows = append(rows, []string{
			h.GetId(), programStateWord(h.GetState()), pid,
			strconv.Itoa(int(h.GetFailures())), strconv.Itoa(int(h.GetRestarts())),
			programNote(h),
		})
	}
	writeTable(&b, []string{"PROGRAM", "STATE", "PID", "FAILURES", "RESTARTS", "NOTE"}, rows)
	fmt.Print(b.String())
	return nil
}

// programNote is the one thing a human most needs to see about a program, in
// that order: a question nobody has seen, what it is blocked on, how the last
// child ended, when the next attempt is.
func programNote(h *verbsv1.ProgramHealth) string {
	var notes []string
	if p := h.GetParked(); p != "" {
		notes = append(notes, "PARKED: "+p)
	}
	if w := h.GetWaiting(); w != "" {
		notes = append(notes, "waiting on "+w)
	}
	if e := h.GetLastExit(); e != "" {
		notes = append(notes, "last "+e)
	}
	if n := h.GetNextAttemptUnixNano(); n != 0 {
		notes = append(notes, "next attempt "+time.Unix(0, n).Format(time.TimeOnly))
	}
	return strings.Join(notes, "; ")
}

// programStateWord renders the table's dash for a program rig is not running,
// which is what UNSPECIFIED means on this message and nowhere else.
func programStateWord(s verbsv1.ProgramState) string {
	if s == verbsv1.ProgramState_PROGRAM_STATE_UNSPECIFIED {
		return "-"
	}
	if w, ok := enumWord(s, "PROGRAM_STATE_"); ok {
		return w
	}
	return skewToken(s)
}

// programOut is one program as --json prints it.
type programOut struct {
	ID                  string            `json:"id"`
	State               string            `json:"state"`
	PID                 int32             `json:"pid"`
	SinceUnixNano       int64             `json:"since_unix_nano"`
	Failures            uint32            `json:"failures"`
	Restarts            uint32            `json:"restarts"`
	Marker              uint64            `json:"marker"`
	Waiting             string            `json:"waiting"`
	Parked              string            `json:"parked"`
	LastExit            string            `json:"last_exit"`
	NextAttemptUnixNano int64             `json:"next_attempt_unix_nano"`
	History             []programEventOut `json:"history"`
}

type programEventOut struct {
	AtUnixNano int64  `json:"at_unix_nano"`
	From       string `json:"from"`
	To         string `json:"to"`
	Trigger    string `json:"trigger"`
	Actor      string `json:"actor"`
	Note       string `json:"note"`
}

func programJSON(h *verbsv1.ProgramHealth) programOut {
	out := programOut{
		ID: h.GetId(), State: programStateWord(h.GetState()), PID: h.GetPid(),
		SinceUnixNano: h.GetSinceUnixNano(), Failures: h.GetFailures(),
		Restarts: h.GetRestarts(), Marker: h.GetMarker(), Waiting: h.GetWaiting(),
		Parked: h.GetParked(), LastExit: h.GetLastExit(),
		NextAttemptUnixNano: h.GetNextAttemptUnixNano(),
		History:             make([]programEventOut, 0, len(h.GetHistory())),
	}
	for _, e := range h.GetHistory() {
		out.History = append(out.History, programEventOut{
			AtUnixNano: e.GetAtUnixNano(), From: programStateWord(e.GetFrom()),
			To: programStateWord(e.GetTo()), Trigger: e.GetTrigger(),
			Actor: e.GetActor(), Note: e.GetNote(),
		})
	}
	return out
}
