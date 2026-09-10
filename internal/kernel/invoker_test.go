package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// withCommands is a valid declaration whose commands are exactly the ids and
// effects given, so a test can say "this program has a destructive one".
func withCommands(id string, effects map[string]kernel.Effects) kernel.Declaration {
	d := good(id)
	template := d.Commands[0]
	d.Commands = nil
	for cmd, e := range effects {
		c := template
		c.ID = cmd
		c.Title = cmd
		c.Effects = e
		d.Commands = append(d.Commands, c)
	}
	return d
}

// caller is a principal of one kind that can see the whole registry, so a
// test of the invoker is not also a test of the scope filter.
func caller(k kernel.ClientKind) kernel.Principal {
	return kernel.Principal{
		UID: 1000, Kind: k,
		ClientID: "c-" + k.String(), SessionID: "s-" + k.String(), PID: 44,
		Introspect: true,
	}
}

// cmdRef refers to one of the pilot program's commands.
func cmdRef(command string) kernel.Ref {
	return kernel.Ref{Kind: kernel.RefCommand, Program: "pilot", Command: command}
}

// pilotKernel registers one program with a command per effects level.
func pilotKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	k := kernel.New()
	_, err := k.Register(programPrincipal("pilot"), withCommands("pilot", map[string]kernel.Effects{
		"look":    kernel.EffectsReadOnly,
		"write":   kernel.EffectsWritesFiles,
		"fetch":   kernel.EffectsNetwork,
		"destroy": kernel.EffectsDestructive,
		"type":    kernel.EffectsDrivesInput,
	}))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return k
}

// TestADestructiveCommandIsRefusedAndTheRefusalNamesThePair is slice 4's demo
// at unit level: one rule is written, and the refusal names the pair it
// matched on.
func TestADestructiveCommandIsRefusedAndTheRefusalNamesThePair(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "no-unattended-destruction",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	d, err := k.Authorize(caller(kernel.KindTerminal), []kernel.Ref{cmdRef("destroy")})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatalf("a destructive command under a deny rule got %s, not deny", d.Action)
	}
	if d.Allowed() {
		t.Fatal("a denied decision reports itself allowed")
	}
	if d.Origin != kernel.OriginRule {
		t.Fatalf("origin is %s, and the audit log cannot attribute that", d.Origin)
	}
	if !strings.Contains(d.Rule, "no-unattended-destruction") {
		t.Fatalf("the decision cites %q rather than the rule that fired", d.Rule)
	}
	// The pair is the point: a refusal that does not say what it matched
	// cannot be acted on by whoever has to write the next rule.
	for _, want := range []string{"terminal", "destructive"} {
		if !strings.Contains(d.Reason, want) {
			t.Fatalf("the refusal %q does not name %s", d.Reason, want)
		}
	}
	if got := d.Pair.String(); got != "(terminal, destructive)" {
		t.Fatalf("pair renders as %s", got)
	}
}

func TestAnUnmatchedCallIsAllowedAndSaysSo(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules(nil); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	d, err := k.Authorize(caller(kernel.KindTerminal), []kernel.Ref{cmdRef("destroy")})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	// Section 13a's compatibility promise: nothing already working breaks the
	// day the table lands.
	if !d.Allowed() {
		t.Fatalf("an unmatched call got %s, and the empty table is the promise", d.Action)
	}
	if d.Origin != kernel.OriginUnmatched {
		t.Fatalf("origin is %s, not unmatched", d.Origin)
	}
	if d.Rule != "" {
		t.Fatalf("an unmatched decision cites rule %q, which does not exist", d.Rule)
	}
	if !strings.Contains(d.Reason, "(terminal, destructive)") {
		t.Fatalf("the record %q does not say which pair went unmatched", d.Reason)
	}
}

func TestEffectsIsAFloorNotAnEquality(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "floor",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsWritesFiles,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// Above the floor, and this is the case equality got wrong.
	for _, command := range []string{"write", "fetch", "destroy"} {
		d, err := k.Authorize(caller(kernel.KindAgent), []kernel.Ref{cmdRef(command)})
		if err != nil {
			t.Fatalf("authorize %s: %v", command, err)
		}
		if d.Action != kernel.ActionDeny {
			t.Fatalf("%s is at least as dangerous as writes-files and got %s",
				command, d.Action)
		}
	}

	// Below it, untouched.
	d, err := k.Authorize(caller(kernel.KindAgent), []kernel.Ref{cmdRef("look")})
	if err != nil {
		t.Fatalf("authorize look: %v", err)
	}
	if !d.Allowed() {
		t.Fatalf("read-only is below the floor and got %s", d.Action)
	}
}

func TestTheMostRestrictiveRuleWinsWhateverOrderTheyAreWrittenIn(t *testing.T) {
	allow := kernel.Rule{
		ID: "allow-all", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsReadOnly, Action: kernel.ActionAllow,
	}
	confirm := kernel.Rule{
		ID: "confirm-writes", Caller: kernel.CallerKind(kernel.KindAgent),
		Effects: kernel.EffectsWritesFiles, Action: kernel.ActionConfirm,
	}
	deny := kernel.Rule{
		ID: "deny-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}

	orders := map[string][]kernel.Rule{
		"deny last":  {allow, confirm, deny},
		"deny first": {deny, confirm, allow},
		"deny mid":   {confirm, deny, allow},
	}
	for name, rules := range orders {
		t.Run(name, func(t *testing.T) {
			k := pilotKernel(t)
			if err := k.SetRules(rules); err != nil {
				t.Fatalf("set rules: %v", err)
			}
			d, err := k.Authorize(caller(kernel.KindAgent), []kernel.Ref{cmdRef("destroy")})
			if err != nil {
				t.Fatalf("authorize: %v", err)
			}
			// All three match a destructive call through the floor. If the
			// order of lines in a file could decide this, the floor would be
			// decided by whoever edited last.
			if d.Action != kernel.ActionDeny {
				t.Fatalf("got %s: a narrower allow beat a broader deny", d.Action)
			}
			if d.Rule != `"deny-destructive"` {
				t.Fatalf("cited %s rather than the deny", d.Rule)
			}
		})
	}
}

func TestTheShippedURLDefaultCoversTheDangerousCallToo(t *testing.T) {
	// This is the defect the plan carried: with an equality match, a
	// destructive call over rig:// matched nothing and was allowed.
	k := pilotKernel(t)

	deny, err := k.Authorize(caller(kernel.KindURL), []kernel.Ref{cmdRef("destroy")})
	if err != nil {
		t.Fatalf("authorize destroy: %v", err)
	}
	if deny.Action != kernel.ActionDeny {
		t.Fatalf("a destructive call over rig:// got %s, and any web page can address it",
			deny.Action)
	}

	confirm, err := k.Authorize(caller(kernel.KindURL), []kernel.Ref{cmdRef("look")})
	if err != nil {
		t.Fatalf("authorize look: %v", err)
	}
	if confirm.Action != kernel.ActionConfirm {
		t.Fatalf("a read-only call over rig:// got %s, not confirm", confirm.Action)
	}
}

func TestEveryOtherCallerStartsUnmatchedUnderTheShippedRules(t *testing.T) {
	k := pilotKernel(t)
	for _, kind := range []kernel.ClientKind{
		kernel.KindAgent, kernel.KindTerminal, kernel.KindWindow, kernel.KindScript,
		kernel.KindProgram, kernel.KindSchedule, kernel.KindBus,
	} {
		d, err := k.Authorize(caller(kind), []kernel.Ref{cmdRef("destroy")})
		if err != nil {
			t.Fatalf("authorize as %s: %v", kind, err)
		}
		if !d.Allowed() {
			t.Fatalf("%s got %s from the shipped defaults, which breaks something "+
				"already working", kind, d.Action)
		}
	}
}

func TestAWrappingVerbCannotMaskWhatItWraps(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "no-unattended-destruction",
		Caller:  kernel.CallerKind(kernel.KindSchedule),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// `rig peers run --lease=deploy -- pilot destroy`, as the boundary sees
	// it: the outer verb has no registry entry, and matching on it alone
	// would find no effects at all.
	d, err := k.Authorize(caller(kernel.KindSchedule), []kernel.Ref{
		{Kind: kernel.RefWrapper, Program: "rig", Command: "peers run"},
		cmdRef("destroy"),
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatalf("the wrapper masked its subject: got %s", d.Action)
	}
	if d.Pair.Effects != kernel.EffectsDestructive {
		t.Fatalf("the union came out %s", d.Pair.Effects)
	}
}

func TestAWrappedCommandRigCannotResolveCountsAsDestructive(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "no-unattended-destruction",
		Caller:  kernel.CallerKind(kernel.KindSchedule),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// `rig peers run --lease=deploy -- make deploy`. rig cannot know what
	// make does, and unresolvable-and-harmless is a combination only the
	// program could declare.
	d, err := k.Authorize(caller(kernel.KindSchedule), []kernel.Ref{
		{Kind: kernel.RefWrapper, Program: "rig", Command: "peers run"},
		{Kind: kernel.RefOpaque, Command: "make deploy"},
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatalf("an unresolvable subject got %s, so the rule that names "+
			"destructive missed the one case the invoker cannot see", d.Action)
	}
}

func TestTheUnionTakesTheMostDangerousOfSeveralCommands(t *testing.T) {
	k := pilotKernel(t)
	d, err := k.Authorize(caller(kernel.KindAgent), []kernel.Ref{
		cmdRef("look"),
		cmdRef("destroy"),
		cmdRef("write"),
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Pair.Effects != kernel.EffectsDestructive {
		t.Fatalf("the union of read-only, destructive and writes-files is %s",
			d.Pair.Effects)
	}
}

func TestACommandThatIsNotRegisteredAtAllFailsSafe(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "deny-destructive",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// Nothing in the registry declares this, so rig cannot know what it does.
	d, err := k.Authorize(caller(kernel.KindAgent),
		[]kernel.Ref{{Kind: kernel.RefCommand, Program: "pilot", Command: "nosuch"}})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatalf("an unregistered target got %s rather than failing safe", d.Action)
	}
	// The CEILING, not destructive by name. "Fails safe" means the worst thing
	// it could be, and this assertion read `EffectsDestructive` until a level
	// above it existed - at which point it was asserting the second worst and
	// still passing under its own name.
	if d.Pair.Effects != kernel.EffectsCeiling {
		t.Fatalf("an unregistered target resolved as %s, want the ceiling %s",
			d.Pair.Effects, kernel.EffectsCeiling)
	}
}

func TestTheCeilingIsTheTopOfTheDangerOrder(t *testing.T) {
	// The guard on the guard. EffectsCeiling exists so "assume the worst" does
	// not quietly become "assume the second worst" the next time a level is
	// added, and this is the assertion that fails when it does. It walks the
	// names map rather than a list written here, so a new member is covered
	// the day it is added.
	// Stated as "nothing named sits above it", which is checkable through the
	// exported surface alone: an unnamed value renders as Effects(N) and
	// ParseEffects refuses it, so a named member appearing above the ceiling
	// is exactly the case where this parse starts succeeding.
	above := kernel.Effects(uint8(kernel.EffectsCeiling) + 1)
	if _, err := kernel.ParseEffects(above.String()); err == nil {
		t.Fatalf("%s is a named effects value above the ceiling %s, so an "+
			"opaque ref would resolve below it and escape a rule denying it",
			above, kernel.EffectsCeiling)
	}
	if !kernel.EffectsCeiling.AtLeastAsDangerousAs(kernel.EffectsDestructive) {
		t.Fatalf("the ceiling %s is below destructive", kernel.EffectsCeiling)
	}
}

func TestTheScopeFilterDoesNotDecideWhatACommandDoes(t *testing.T) {
	// The correction: effects are what the TARGET declared, not what the
	// caller may read. An unscoped caller that reads no program at all still
	// gets a read-only command resolved as read-only - because resolving it
	// through the caller's view made a program calling another program's
	// read-only command match as destructive.
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID: "deny-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	blind := kernel.Principal{
		UID: 1000, Kind: kernel.KindAgent,
		ClientID: "blind", SessionID: "s-blind", PID: 45,
	}
	if _, ok := k.See(blind).Program("pilot"); ok {
		t.Fatal("this caller was supposed to see nothing")
	}

	d, err := k.Authorize(blind, []kernel.Ref{cmdRef("look")})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Pair.Effects != kernel.EffectsReadOnly {
		t.Fatalf("a read-only command resolved as %s for a caller that "+
			"cannot list it", d.Pair.Effects)
	}
	if !d.Allowed() {
		t.Fatalf("a read-only call was %s under a destructive-only rule", d.Action)
	}
}

func TestAuthorizeRefusesACallItCannotReasonAbout(t *testing.T) {
	k := pilotKernel(t)

	if _, err := k.Authorize(caller(kernel.KindTerminal), nil); err == nil {
		t.Fatal("authorising nothing succeeded: the boundary has to say what runs")
	}
	if _, err := k.Authorize(caller(kernel.KindTerminal), []kernel.Ref{
		{Program: "pilot", Command: "destroy"}, // no Kind
	}); err == nil {
		t.Fatal("a ref with no kind was authorised")
	}
	if _, err := k.Authorize(kernel.Principal{UID: 1000, ClientID: "x", SessionID: "y"},
		[]kernel.Ref{cmdRef("look")}); err == nil {
		t.Fatal("a principal with no client kind was authorised")
	}
}

func TestABadTableNeverBecomesTheLiveOne(t *testing.T) {
	k := pilotKernel(t)
	before := k.Rules()

	bad := []kernel.Rule{
		{
			ID: "fine", Caller: kernel.AnyCaller(), Effects: kernel.EffectsDestructive,
			Action: kernel.ActionDeny,
		},
		{ID: "no-action", Caller: kernel.AnyCaller(), Effects: kernel.EffectsDestructive},
	}
	if err := k.SetRules(bad); err == nil {
		t.Fatal("a rule with no action was accepted")
	}
	if got := k.Rules(); len(got) != len(before) {
		t.Fatalf("the live table changed to %d rules on a refused push", len(got))
	}

	for _, r := range []kernel.Rule{
		{ID: "no-effects", Caller: kernel.AnyCaller(), Action: kernel.ActionDeny},
		{ID: "no-caller", Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny},
	} {
		if err := k.SetRules([]kernel.Rule{r}); err == nil {
			t.Fatalf("rule %q was accepted", r.ID)
		}
	}
}

func TestAnUnnamedRuleIsStillCitable(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}
	d, err := k.Authorize(caller(kernel.KindTerminal), []kernel.Ref{cmdRef("destroy")})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	// Section 13a's grammar has no id field, so the log has to be able to
	// cite a rule the owner never named.
	if d.Rule != "rules[0]" {
		t.Fatalf("an unnamed rule is cited as %q", d.Rule)
	}
}

func TestParseTheNamesAHouseRuleWrites(t *testing.T) {
	for _, s := range []string{"allow", "confirm", "deny"} {
		if _, err := kernel.ParseAction(s); err != nil {
			t.Fatalf("parse action %q: %v", s, err)
		}
	}
	for _, s := range []string{"unspecified", "", "Deny", "ask"} {
		if _, err := kernel.ParseAction(s); err == nil {
			t.Fatalf("action %q parsed", s)
		}
	}

	// "any" is a rule's wildcard, and a connection can never be it.
	if _, err := kernel.ParseRuleCaller("any"); err != nil {
		t.Fatalf("parse caller any: %v", err)
	}
	if _, err := kernel.ParseClientKind("any"); err == nil {
		t.Fatal("a principal was allowed to be of kind any")
	}
	for _, s := range []string{
		"agent", "terminal", "window", "script", "program",
		"schedule", "bus", "url",
	} {
		if _, err := kernel.ParseRuleCaller(s); err != nil {
			t.Fatalf("parse caller %q: %v", s, err)
		}
	}
	if _, err := kernel.ParseRuleCaller("phone"); err == nil {
		t.Fatal("an unknown caller parsed")
	}
}

func TestRulesDoesNotHandOutTheLiveTable(t *testing.T) {
	k := pilotKernel(t)
	got := k.Rules()
	if len(got) == 0 {
		t.Fatal("the shipped table is empty")
	}
	got[0].Action = kernel.ActionAllow

	d, err := k.Authorize(caller(kernel.KindURL), []kernel.Ref{cmdRef("destroy")})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatal("editing the slice Rules returned changed what the invoker decides")
	}
}

func TestAWrappingVerbWrappingNothingIsNotAuthorisable(t *testing.T) {
	// Only a boundary bug reaches this: a wrapping verb contributes no
	// effects, so a call made of nothing but wrappers never says what it
	// will run, and matching it against "nothing said" would match the
	// lowest floor there is.
	k := pilotKernel(t)
	_, err := k.Authorize(caller(kernel.KindSchedule), []kernel.Ref{
		{Kind: kernel.RefWrapper, Program: "rig", Command: "peers run"},
	})
	if err == nil {
		t.Fatal("a call with no subject was authorised")
	}
}

func TestEquallyRestrictiveRulesAreCitedDeterministically(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{
		{
			ID: "first", Caller: kernel.AnyCaller(), Effects: kernel.EffectsReadOnly,
			Action: kernel.ActionDeny,
		},
		{
			ID: "second", Caller: kernel.CallerKind(kernel.KindAgent),
			Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
		},
	}); err != nil {
		t.Fatalf("set rules: %v", err)
	}
	// Both deny, so the action is not in question - but the log has to name
	// the same one every time or the record is not evidence.
	for range 5 {
		d, err := k.Authorize(caller(kernel.KindAgent), []kernel.Ref{cmdRef("destroy")})
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		if d.Rule != `"first"` {
			t.Fatalf("cited %s rather than the earliest of two equal rules", d.Rule)
		}
	}
}

func TestRulesCanBeReplacedWhileCallsAreBeingAuthorised(t *testing.T) {
	// Section 6 pushes config live, so the table is replaced under callers
	// rather than at startup. Run with -race: this is what the second lock on
	// the kernel exists for.
	k := pilotKernel(t)
	deny := []kernel.Rule{{
		ID: "deny", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}}
	allow := []kernel.Rule{{
		ID: "allow", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionAllow,
	}}

	// Start from one of the two tables, so a call that lands before the
	// pusher's first write is matched too - the shipped defaults name url
	// and would leave a terminal caller unmatched.
	if err := k.SetRules(deny); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			table := deny
			if i%2 == 0 {
				table = allow
			}
			if err := k.SetRules(table); err != nil {
				t.Errorf("set rules: %v", err)
				return
			}
		}
	}()

	who := caller(kernel.KindTerminal)
	for range 200 {
		d, err := k.Authorize(who, []kernel.Ref{cmdRef("destroy")})
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		// Whichever table it caught, it must have caught a whole one.
		if d.Action != kernel.ActionDeny && d.Action != kernel.ActionAllow {
			t.Fatalf("a half-applied table produced %s", d.Action)
		}
		if d.Origin != kernel.OriginRule {
			t.Fatalf("origin %s: a live push lost the rule", d.Origin)
		}
	}
	<-done
}

// TestARuleAboutInputDoesNotDecideARuleAboutFiles is the whole justification
// for EFFECTS_DRIVES_INPUT existing, stated as the sentence a house rule could
// not express before it did: "this program may delete its own files but may
// not type into my windows".
//
// Both halves matter. If the deny leaks down onto `destroy` the value has
// bought a rename and nothing else; if it fails to catch `type` it has bought
// nothing at all.
func TestARuleAboutInputDoesNotDecideARuleAboutFiles(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "no-typing-for-programs",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDrivesInput,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	d, err := k.Authorize(caller(kernel.KindAgent), []kernel.Ref{cmdRef("type")})
	if err != nil {
		t.Fatalf("authorize type: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatalf("a rule denying drives-input got %s on a drives-input "+
			"command, so the value catches nothing", d.Action)
	}

	// The half that a floor semantic makes easy to get wrong: destructive is
	// BELOW drives-input, so a rule written at the higher level must not reach
	// down to it. Deleting files is still allowed here, and that is the point.
	d, err = k.Authorize(caller(kernel.KindAgent), []kernel.Ref{cmdRef("destroy")})
	if err != nil {
		t.Fatalf("authorize destroy: %v", err)
	}
	if d.Action == kernel.ActionDeny {
		t.Fatalf("a rule denying drives-input also refused a destructive " +
			"command, so the two levels are not separable and the new value " +
			"bought a rename")
	}
}

func TestAnOpaqueRefResolvesToTheCeilingAndNotMerelyToDestructive(t *testing.T) {
	// RefOpaque is a SECOND code path from the unresolvable-command one, and it
	// was floored at the destructive literal too. A rule naming destructive
	// still passes when either path is capped one level short, so neither test
	// that asserted a deny could see the difference. This asserts the level.
	k := pilotKernel(t)
	d, err := k.Authorize(caller(kernel.KindSchedule), []kernel.Ref{
		{Kind: kernel.RefWrapper, Program: "rig", Command: "peers run"},
		{Kind: kernel.RefOpaque, Command: "make deploy"},
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Pair.Effects != kernel.EffectsCeiling {
		t.Fatalf("an opaque subject resolved as %s, want the ceiling %s: rig "+
			"cannot see what it runs, so anything less than the worst case "+
			"lets it under a rule denying the level above",
			d.Pair.Effects, kernel.EffectsCeiling)
	}
}

// The invoker reads declared effects UNFILTERED, and this locks it at the
// kernel rather than over the wire.
//
// It used to be locked by a daemon test where one program called another's
// read-only command. Closing gap 1 on 2026-09-11 made routing check
// visibility, so that call no longer reaches the invoker at all and the wire
// test could no longer see the property. The property did not go away with
// it: a caller that CAN reach a target - a terminal today, a crew member at
// M7 - must still have the target's own declaration matched, not a
// pessimistic ceiling from a view that happens to exclude it.
//
// Resolving through the caller's view made a read-only command resolve as
// destructive whenever the caller was not in the target's scope, and a rule
// denying destructive calls then refused a read-only one.
func TestTheInvokerMatchesTheTargetsDeclarationNotTheCallersView(t *testing.T) {
	k := kernel.New()

	owner := kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "shelf", SessionID: "s1", PID: 1,
	}
	if _, err := k.Register(owner, good("shelf")); err != nil {
		t.Fatalf("register shelf: %v", err)
	}

	// grabbit is scoped to itself, so shelf is invisible to it. The invoker
	// must still read shelf's declaration.
	grabbit := kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "grabbit", SessionID: "s2", PID: 2,
		Scoped: true, Scopes: []string{"grabbit"},
	}
	if _, ok := k.See(grabbit).Program("shelf"); ok {
		t.Fatal("shelf is visible to grabbit, so this test proves nothing")
	}

	dec, err := k.Authorize(grabbit, []kernel.Ref{{
		Kind: kernel.RefCommand, Program: "shelf", Command: "reindex",
	}})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	// good("shelf") declares reindex as writes-files. Read through grabbit's
	// view shelf is unresolvable, which is EffectsCeiling.
	if dec.Pair.Effects != kernel.EffectsWritesFiles {
		t.Fatalf("the invoker resolved %s, want writes-files: it read the "+
			"caller's view rather than the target's declaration", dec.Pair.Effects)
	}
}
