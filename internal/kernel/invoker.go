package kernel

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The invoker is the authorization floor (PLAN.md section 13a).
//
// rig already carried effects and a danger level from the registry, and the
// principal from the connection, and never put the two together. This file
// is where they meet, and it is deliberately in the kernel: no surface can
// forget it, and a surface added in 2028 is covered by rules written in 2026.
// One test covers every surface, present and future.
//
// The pair is resolved ONCE, here, at the boundary. caller is the principal
// that arrived; effects is the union over every registered command the call
// will actually run. That is what makes a wrapping verb safe: it contributes
// no effects of its own and cannot mask the effects it wraps, so
// `rig peers run --lease=NAME -- make deploy` does not slip past a rule
// written against destructive commands.

// Action is what a house rule says to do about a matched pair.
//
// The values are ordered by restrictiveness, and that ordering is the
// precedence rule: when more than one rule matches, the most restrictive
// wins. Section 13a needs this the day it ships, because the url default
// below is two rules that both match the same call.
type Action uint8

const (
	// ActionUnspecified is the zero and means nothing (section 21). A rule
	// that never said what to do is refused rather than defaulting to the
	// least restrictive action.
	ActionUnspecified Action = iota
	ActionAllow
	ActionConfirm
	ActionDeny
)

var actionNames = map[Action]string{
	ActionUnspecified: unspecifiedName,
	ActionAllow:       "allow",
	ActionConfirm:     "confirm",
	ActionDeny:        "deny",
}

func (a Action) String() string {
	if n, ok := actionNames[a]; ok {
		return n
	}
	return fmt.Sprintf("Action(%d)", uint8(a))
}

// MoreRestrictiveThan orders actions so the matcher can pick a winner
// without caring what order the rules were written in.
func (a Action) MoreRestrictiveThan(other Action) bool { return a > other }

// ParseAction reads the name a house rule writes.
func ParseAction(s string) (Action, error) {
	for a, n := range actionNames {
		if n == s && a != ActionUnspecified {
			return a, nil
		}
	}
	return ActionUnspecified, fmt.Errorf("kernel: %q is not an action", s)
}

// Origin says what produced a decision, and it is what the audit log needs
// (section 13a).
//
// An elevation is not triggered by a house rule - it is triggered by the call
// needing estate-wide authorisation (section 14) - so an origin that can only
// name a rule has no value for the commonest confirm the system will produce.
//
// The zero is not a third policy. It means no rule matched, which is the one
// case where nothing decided anything: section 13a keeps an unmatched call
// allowed, and the record has to be able to say that rather than cite a rule
// that does not exist.
type Origin uint8

const (
	OriginUnmatched Origin = iota
	OriginRule
	OriginElevation
)

var originNames = map[Origin]string{
	OriginUnmatched: "unmatched",
	OriginRule:      "rule",
	OriginElevation: "elevation",
}

func (o Origin) String() string {
	if n, ok := originNames[o]; ok {
		return n
	}
	return fmt.Sprintf("Origin(%d)", uint8(o))
}

// RuleCaller is a rule's caller matcher: one client kind, or the wildcard.
//
// "any" is a rule's wildcard and not something a connection can be, so it is
// not a ClientKind - ParseClientKind refuses it on purpose. It is a distinct
// type here rather than a tenth enum value for the same reason: a principal
// carrying Kind "any" would be a principal whose kind is unanswerable.
type RuleCaller struct {
	kind ClientKind
	all  bool
}

// AnyCaller matches every kind, present and future.
func AnyCaller() RuleCaller { return RuleCaller{all: true} }

// CallerKind matches exactly one kind.
func CallerKind(k ClientKind) RuleCaller { return RuleCaller{kind: k} }

// ParseRuleCaller reads a rule's caller field, wildcard included.
func ParseRuleCaller(s string) (RuleCaller, error) {
	if s == "any" {
		return AnyCaller(), nil
	}
	k, err := ParseClientKind(s)
	if err != nil {
		return RuleCaller{}, err
	}
	return CallerKind(k), nil
}

func (rc RuleCaller) String() string {
	if rc.all {
		return "any"
	}
	return rc.kind.String()
}

func (rc RuleCaller) matches(k ClientKind) bool { return rc.all || rc.kind == k }

func (rc RuleCaller) valid() bool { return rc.all || rc.kind.Valid() }

// Rule is one line of the house rules table.
type Rule struct {
	// ID names this rule in the audit log. It is optional because section
	// 13a's own grammar has no id field: an unnamed rule is cited by its
	// position instead, so the record can always answer "which rule".
	ID string

	Caller RuleCaller

	// Effects is a FLOOR, not an equality: this rule matches a command whose
	// declared effects are at least as dangerous as this value.
	//
	// Equality was the first reading and it cannot express the one default
	// this table ships. A rule naming read-only would match read-only calls
	// and nothing else, so a destructive call over rig:// fell through to the
	// empty set and was allowed - the opposite of what the section intends
	// two sentences earlier.
	Effects Effects

	Action Action
}

func (r Rule) validate() error {
	var bad []string
	if !r.Caller.valid() {
		bad = append(bad, "caller is not a client kind or \"any\"")
	}
	if r.Effects == EffectsUnspecified {
		bad = append(bad, "effects is unspecified: a floor of \"nothing said\" "+
			"would match every call including the ones rig could not resolve")
	}
	if r.Action == ActionUnspecified {
		bad = append(bad, "action is unspecified: say allow, confirm or deny")
	}
	if len(bad) > 0 {
		return fmt.Errorf("kernel: rule %s is unusable: %s",
			r.name(-1), strings.Join(bad, "; "))
	}
	return nil
}

// name is what the audit log cites: the id if the rule has one, its position
// if it does not.
func (r Rule) name(i int) string {
	if r.ID != "" {
		return fmt.Sprintf("%q", r.ID)
	}
	if i < 0 {
		return "(unnamed)"
	}
	return fmt.Sprintf("rules[%d]", i)
}

// DefaultRules is the only rule set that ships non-empty, and it is the two
// url rules from section 13a.
//
// A rig:// scheme registered on the desktop is reachable from any web page,
// email or chat message, and section 29's "the threat model is a mistake in
// our own code, not an adversary" was written about hosted programs. So url
// is denied above read-only and confirmed at it - two rules, because
// "read-only plus confirm" is not something the grammar can say in one. A
// destructive call over rig:// matches both, through the floor, and the
// precedence rule resolves it to deny.
//
// Every other caller starts unmatched, which is what makes the table safe to
// land at M1: nothing already working breaks the day it lands.
func DefaultRules() []Rule {
	return []Rule{
		{
			ID:      "url-deny-above-read-only",
			Caller:  CallerKind(KindURL),
			Effects: EffectsWritesFiles,
			Action:  ActionDeny,
		},
		{
			ID:      "url-confirm-read-only",
			Caller:  CallerKind(KindURL),
			Effects: EffectsReadOnly,
			Action:  ActionConfirm,
		},
	}
}

// SetRules replaces the live table, and validates before it swaps.
//
// Replacing rather than mutating is the shape section 6 needs: config is
// resolved from four layers and pushed live, so the table arrives whole. A
// bad table never becomes the live one, because a rule the matcher cannot
// reason about is worse than no rule at all - it silently matches nothing.
func (k *Kernel) SetRules(rules []Rule) error {
	for i, r := range rules {
		if err := r.validate(); err != nil {
			return fmt.Errorf("kernel: house rules refused at %s: %w",
				r.name(i), err)
		}
	}
	k.rmu.Lock()
	defer k.rmu.Unlock()
	k.rules = slices.Clone(rules)
	return nil
}

// Rules reads the live table.
func (k *Kernel) Rules() []Rule {
	k.rmu.RLock()
	defer k.rmu.RUnlock()
	return slices.Clone(k.rules)
}

// RefKind is what the boundary knows about one thing the call will run.
//
// Three cases, and section 13a needs all three separated: a registered
// command whose effects the registry declares, one of rig's own wrapping
// verbs which contributes nothing, and something rig cannot resolve at all.
type RefKind uint8

const (
	// RefUnspecified is the zero and means nothing: a boundary that did not
	// say what kind of thing it is invoking is refused.
	RefUnspecified RefKind = iota

	// RefCommand is a registered command. Its effects come from the
	// registry, declared by the program, never from the caller.
	RefCommand

	// RefWrapper is one of rig's own verbs whose subject is another command -
	// `rig peers run` at M7. It has no registry entry and contributes no
	// effects of its own, and it cannot mask what it wraps.
	RefWrapper

	// RefOpaque is something rig cannot resolve: `make deploy`. It counts as
	// destructive for matching.
	//
	// This does not change what happens when no rule exists - an unmatched
	// call is still allowed - but it means a rule naming destructive fires on
	// the one case where the invoker cannot see what it is authorising,
	// rather than silently missing it. Unresolvable and harmless is a
	// combination only the program can declare, and it has not.
	RefOpaque
)

var refKindNames = map[RefKind]string{
	RefUnspecified: unspecifiedName,
	RefCommand:     "command",
	RefWrapper:     "wrapper",
	RefOpaque:      "opaque",
}

func (rk RefKind) String() string {
	if n, ok := refKindNames[rk]; ok {
		return n
	}
	return fmt.Sprintf("RefKind(%d)", uint8(rk))
}

// Ref is one thing the call will actually run, as the boundary sees it.
type Ref struct {
	Kind RefKind

	// Program and Command name it. For RefOpaque, Command carries the literal
	// the caller wrote, so the refusal can quote it back.
	Program string
	Command string
}

// What renders a ref for a refusal a person has to read.
func (r Ref) What() string {
	switch r.Kind {
	case RefOpaque:
		return fmt.Sprintf("%q (rig cannot resolve it)", r.Command)
	case RefWrapper:
		return fmt.Sprintf("%s.%s (a wrapping verb)", r.Program, r.Command)
	default:
		return r.Program + "." + r.Command
	}
}

// Pair is what the invoker matches on, resolved once at the boundary.
type Pair struct {
	Caller  ClientKind
	Effects Effects
}

func (p Pair) String() string { return fmt.Sprintf("(%s, %s)", p.Caller, p.Effects) }

// Decision is what the invoker decided, and everything the audit log needs to
// answer "why was I asked" and "why was that allowed".
type Decision struct {
	Pair   Pair
	Action Action
	Origin Origin

	// Rule is how the audit log cites what fired: an id, or a position. Empty
	// when nothing matched.
	Rule string

	// Reason names the pair, in words, because a refusal that does not say
	// what it matched cannot be acted on by whoever has to write the rule.
	Reason string
}

// Allowed reports whether the call may proceed with no question asked.
func (d Decision) Allowed() bool { return d.Action == ActionAllow }

// pair resolves the effective pair, and reads effects unfiltered.
//
// Unfiltered is deliberate, and it is a correction. Resolving through the
// caller's own view looks safer and is not: section 14 scopes a program to
// itself, so a program calling another program's READ-ONLY command could not
// resolve the target at all, fell to opaque, and was matched as destructive.
// A rule denying destructive calls then refused a read-only one, which is the
// compatibility promise broken by the mechanism meant to keep it.
//
// What the invoker matches on is what the TARGET declared, not what the
// caller may read. The cost is one enum value: a refusal names the effects of
// a command the caller might not have been able to list. That is a smaller
// hole than the alternative, and smaller than the one already there - routing
// does not check visibility at all, so such a caller can invoke the command
// and observe it working. That gap is section 14's to close, not this
// function's to hide.
//
// A target that is not in the registry at all - never registered, or a
// program that disconnected between routing and authorising - is opaque and
// therefore destructive, which is the fail-safe half.
func (k *Kernel) pair(who Principal, refs []Ref) (Pair, error) {
	if len(refs) == 0 {
		return Pair{}, errors.New("kernel: nothing to authorise: the boundary " +
			"has to say what the call will actually run")
	}
	p := Pair{Caller: who.Kind}
	for _, r := range refs {
		switch r.Kind {
		case RefCommand:
			c, ok := k.registry.command(r.Program, r.Command)
			if !ok {
				// Not resolvable through this view, so it is opaque, so it is
				// treated as the worst thing it could be. See RefOpaque and
				// EffectsCeiling - this said EffectsDestructive until a level
				// above it existed, which would have let an opaque call slip
				// under a rule denying that level.
				p.Effects = max(p.Effects, EffectsCeiling)
				continue
			}
			p.Effects = max(p.Effects, c.Effects)
		case RefWrapper:
			// Contributes no effects of its own, and cannot mask the ones it
			// wraps - those arrive as their own refs.
		case RefOpaque:
			p.Effects = max(p.Effects, EffectsCeiling)
		default:
			return Pair{}, fmt.Errorf("kernel: ref %s has no kind: a boundary "+
				"that did not say what it is invoking cannot be authorised",
				r.What())
		}
	}
	if p.Effects == EffectsUnspecified {
		// Every ref was a wrapping verb. A wrapping verb contributes no
		// effects of its own, so nothing here says what the call will run,
		// and matching would be done against "nothing said" - which is the
		// floor that matches everything. Only a boundary bug reaches this.
		return Pair{}, errors.New("kernel: every ref is a wrapping verb, so " +
			"nothing says what the call will actually run")
	}
	return p, nil
}

// Authorize resolves the pair once and matches the table against it.
//
// It answers with a decision rather than an error for a refusal: a denial is
// a fact the audit log records and the caller is told about, not a failure of
// the invoker. The error return is for a call the invoker cannot reason
// about at all.
func (k *Kernel) Authorize(p Principal, refs []Ref) (Decision, error) {
	if err := p.Valid(); err != nil {
		return Decision{}, err
	}
	pair, err := k.pair(p, refs)
	if err != nil {
		return Decision{}, err
	}

	rule, cited, matched := k.match(pair)
	if !matched {
		return Decision{
			Pair:   pair,
			Action: ActionAllow,
			Origin: OriginUnmatched,
			Reason: fmt.Sprintf("no house rule matches %s", pair),
		}, nil
	}
	return Decision{
		Pair:   pair,
		Action: rule.Action,
		Origin: OriginRule,
		Rule:   cited,
		Reason: fmt.Sprintf("house rule %s matches %s and says %s",
			cited, pair, rule.Action),
	}, nil
}

// match finds the rule that decides this pair.
//
// The most restrictive matching rule wins: deny over confirm over allow.
// First-match would make the outcome depend on the order lines happen to sit
// in a file, and most-specific would let a narrow allow beat a broad deny.
// Both fail open, and this is the authorization floor.
//
// Among equally restrictive rules the earliest declared is the one cited, so
// the record is deterministic - it cannot change the action, only the name in
// the log.
func (k *Kernel) match(pair Pair) (winner Rule, cited string, matched bool) {
	k.rmu.RLock()
	defer k.rmu.RUnlock()

	for i, r := range k.rules {
		if !r.Caller.matches(pair.Caller) {
			continue
		}
		if !pair.Effects.AtLeastAsDangerousAs(r.Effects) {
			continue
		}
		if !matched || r.Action.MoreRestrictiveThan(winner.Action) {
			winner, cited, matched = r, r.name(i), true
		}
	}
	return winner, cited, matched
}
