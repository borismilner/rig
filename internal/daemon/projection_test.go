package daemon

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// withPreamble declares a program that fills every field a depth trims, so a
// test can tell "absent because the depth dropped it" from "absent because
// nothing set it".
func withPreamble(id string) *rigv1.Declaration {
	d := testDeclaration(id)
	c := proto.Clone(d.GetCommands()[0]).(*rigv1.Command)
	c.Id = "reindex"
	c.Title = "Reindex"
	c.Effects = rigv1.Effects_EFFECTS_WRITES_FILES
	c.Args = []byte(`{"type":"object"}`)
	c.Examples = []string{"rig " + id + " reindex --since 7d"}
	c.Preconditions = []string{id + ".index.path exists"}
	c.Summary = "Rebuild the index"
	c.Description = "Rebuilds the index from the tree."
	c.Returns = "The number of items indexed."
	d.Commands = append(d.Commands, c)
	d.Preamble = "Read this before touching " + id + "."
	d.CoverageNote = "search is adopted; storage is not"
	return d
}

func connectDeclaring(t *testing.T, sock, id string) *client.Client {
	t.Helper()
	c, err := client.Dial(sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, withPreamble(id)); err != nil {
		t.Fatalf("hello %s: %v", id, err)
	}
	return c
}

func estate(t *testing.T, c *client.Client, d rigv1.Depth) []*rigv1.Program {
	t.Helper()
	var resp rigv1.ProgramsResponse
	if err := c.Call(ctx5(t), "rig.programs",
		&rigv1.ProgramsRequest{Depth: d}, &resp); err != nil {
		t.Fatalf("programs at %v: %v", d, err)
	}
	return resp.GetPrograms()
}

func find(ps []*rigv1.Program, id string) *rigv1.Program {
	for _, p := range ps {
		if p.GetIdentity().GetId() == id {
			return p
		}
	}
	return nil
}

// TestThePreambleReachesACallerAtAllOverTheWire is the half of M2 slice 2 that
// was not a projection at all.
//
// A program has been able to declare a preamble since M1 - accepted at hello,
// validated, stored in the registry - and NO MESSAGE ON ANY PATH COULD CARRY
// IT BACK. Program's own comment used to say it was "Declaration minus
// preamble and scope", so the field was dead for as long as it existed while
// section 9 required describe on a program to return it.
//
// This asserts it over a real socket, because the kernel carrying it proves
// nothing about the wire.
func TestThePreambleReachesACallerAtAllOverTheWire(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	connectDeclaring(t, sock, "shelf")

	p := find(estate(t, dial(t, sock), rigv1.Depth_DEPTH_FULL), "shelf")
	if p == nil {
		t.Fatal("shelf is not in the estate")
	}
	if p.GetPreamble() != "Read this before touching shelf." {
		t.Fatalf("the preamble did not reach the caller: %q", p.GetPreamble())
	}
}

// TestDepthTravelsAndTrimsOverTheWire is the depth half, asserted on what a
// CALLER receives rather than on what the kernel returns.
func TestDepthTravelsAndTrimsOverTheWire(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	connectDeclaring(t, sock, "shelf")
	caller := dial(t, sock)

	programs := find(estate(t, caller, rigv1.Depth_DEPTH_PROGRAMS), "shelf")
	commands := find(estate(t, caller, rigv1.Depth_DEPTH_COMMANDS), "shelf")
	full := find(estate(t, caller, rigv1.Depth_DEPTH_FULL), "shelf")
	if programs == nil || commands == nil || full == nil {
		t.Fatal("shelf is missing at some depth, so the depth filtered the estate")
	}

	// Cheapest: the estate, and nothing about what it can do.
	if len(programs.GetCommands()) != 0 {
		t.Errorf("DEPTH_PROGRAMS carried %d commands", len(programs.GetCommands()))
	}
	if programs.GetPreamble() != "" {
		t.Error("DEPTH_PROGRAMS carried the preamble")
	}
	// Coverage travels at every depth: section 5k forbids a surface implying
	// completeness.
	if programs.GetCoverage() != rigv1.Coverage_COVERAGE_PARTIAL ||
		programs.GetCoverageNote() == "" {
		t.Error("DEPTH_PROGRAMS dropped coverage, which every depth must carry")
	}

	// Middle: chosen BY, not called WITH.
	mid := commandNamed(t, commands, "reindex")
	if len(mid.GetArgs()) != 0 || len(mid.GetExamples()) != 0 ||
		len(mid.GetPreconditions()) != 0 || mid.GetDescription() != "" ||
		mid.GetReturns() != "" {
		t.Error("DEPTH_COMMANDS carried what a command is called WITH")
	}
	if mid.GetSummary() == "" || mid.GetEffects() == rigv1.Effects_EFFECTS_UNSPECIFIED {
		t.Error("DEPTH_COMMANDS dropped what a command is chosen BY")
	}
	if commands.GetPreamble() != "" {
		t.Error("DEPTH_COMMANDS carried the preamble")
	}

	// Full: everything the declaration set.
	deep := commandNamed(t, full, "reindex")
	if len(deep.GetArgs()) == 0 || len(deep.GetExamples()) == 0 ||
		len(deep.GetPreconditions()) == 0 || full.GetPreamble() == "" {
		t.Error("DEPTH_FULL dropped something the program declared")
	}
}

// TestAnOldCallerGetsWhatItAlwaysGot is the compatibility rule, asserted
// rather than commented.
//
// rig.programs took an empty request before it took a depth. proto3 hands an
// old message the zero value whether the sender said anything or not, so
// reading zero as the cheapest depth would silently empty the commands list of
// every client not yet recompiled - including the rig CLI.
func TestAnOldCallerGetsWhatItAlwaysGot(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	connectDeclaring(t, sock, "shelf")

	// Exactly what a caller written before the field existed sends.
	old := find(estate(t, dial(t, sock), rigv1.Depth_DEPTH_UNSPECIFIED), "shelf")
	if old == nil {
		t.Fatal("an old caller saw no programs at all")
	}
	deep := commandNamed(t, old, "reindex")
	if len(deep.GetArgs()) == 0 || len(deep.GetExamples()) == 0 {
		t.Error("an absent depth was read as a cheaper one, so every caller " +
			"not yet recompiled silently lost its command detail")
	}
	if old.GetPreamble() == "" {
		t.Error("an absent depth did not get the full estate")
	}
}

// TestADepthNeverChangesWhichProgramsAreNamed is the property that makes the
// field safe to expose: a cheaper read that showed MORE of the estate would be
// a scope hole wearing a performance argument.
func TestADepthNeverChangesWhichProgramsAreNamed(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	connectDeclaring(t, sock, "shelf")
	shelf := connectDeclaring(t, sock, "grabbit")

	for _, d := range []rigv1.Depth{
		rigv1.Depth_DEPTH_UNSPECIFIED, rigv1.Depth_DEPTH_PROGRAMS,
		rigv1.Depth_DEPTH_COMMANDS, rigv1.Depth_DEPTH_FULL,
	} {
		// An unscoped client of the owner's sees the whole estate. Checked by
		// NAME rather than by count: a count of two would be satisfied by the
		// same program twice, or by the wrong two.
		whole := estate(t, dial(t, sock), d)
		for _, id := range []string{"shelf", "grabbit"} {
			if find(whole, id) == nil {
				t.Errorf("at %v a terminal cannot see %s", d, id)
			}
		}
		if len(whole) != 2 {
			t.Errorf("at %v a terminal sees %d programs, want 2", d, len(whole))
		}
		// A registered program is scoped to itself, at every depth.
		seen := estate(t, shelf, d)
		if len(seen) != 1 || seen[0].GetIdentity().GetId() != "grabbit" {
			t.Errorf("at %v a scoped program sees %d programs, want just itself",
				d, len(seen))
		}
	}
}

func commandNamed(t *testing.T, p *rigv1.Program, id string) *rigv1.Command {
	t.Helper()
	for _, c := range p.GetCommands() {
		if c.GetId() == id {
			return c
		}
	}
	t.Fatalf("%s declares no %s", p.GetIdentity().GetId(), id)
	return nil
}
