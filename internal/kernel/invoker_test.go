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

func TestACommandThisCallerCannotSeeFailsSafe(t *testing.T) {
	k := pilotKernel(t)
	if err := k.SetRules([]kernel.Rule{{
		ID:      "deny-destructive",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// An unscoped, unprivileged caller reads no program at all (section 14),
	// so even the read-only command is unresolvable through its view - and
	// unresolvable is destructive.
	blind := kernel.Principal{
		UID: 1000, Kind: kernel.KindAgent,
		ClientID: "blind", SessionID: "s-blind", PID: 45,
	}
	d, err := k.Authorize(blind, []kernel.Ref{cmdRef("look")})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if d.Action != kernel.ActionDeny {
		t.Fatalf("a target this caller cannot resolve got %s rather than "+
			"failing safe", d.Action)
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
