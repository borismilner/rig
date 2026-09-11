package daemon

import (
	"errors"
	"net"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// sessionOf asks rig.session for THIS connection's token.
//
// It takes no resume argument on purpose: a resume is refused at M6, so every
// test that exercises one has to assert on the error rather than on a
// response, and it calls Call directly. A parameter only ever passed "" is a
// parameter that lies about what the helper does - `unparam` says so.
func sessionOf(t *testing.T, c *client.Client) *rigv1.SessionResponse {
	t.Helper()
	out := &rigv1.SessionResponse{}
	if err := c.Call(ctx5(t), "rig.session", &rigv1.SessionRequest{}, out); err != nil {
		t.Fatalf("rig.session was refused: %v", err)
	}
	return out
}

// THE THREE CALLER ROWS THIS METHOD EXISTS FOR (PLAN.md section 14): an agent,
// a terminal and a script all hold an unregistered connection and never
// receive a HelloResponse, so rig.session is the only message they have on
// which to receive section 5f's token.
func TestAnUnregisteredCallerCanReachItsSessionToken(t *testing.T) {
	c := dial(t, up(t))

	got := sessionOf(t, c)
	if got.GetSession() == "" {
		t.Fatal("an unregistered caller got an empty session token, so section 5f's " +
			"\"every connection carries a session token\" is false for the three " +
			"caller rows this method exists for")
	}
	if got.GetResumed() {
		t.Error("a fresh mint reported resumed=true, which would tell a caller " +
			"rig restored state it never had")
	}
}

// IDEMPOTENT, AND THE TOKEN IS MINTED AT ACCEPT RATHER THAN BY THIS CALL. If
// asking twice produced two tokens the method would be minting, self.go's
// read-only declaration would be a lie, and a retried call would silently
// strand the caller's first session.
func TestAskingTwiceIsTheSameSessionAndNeverASecondOne(t *testing.T) {
	c := dial(t, up(t))

	first := sessionOf(t, c).GetSession()
	second := sessionOf(t, c).GetSession()

	if first != second {
		t.Fatalf("rig.session minted a second token on the same connection: %q then %q. "+
			"It is declared read-only and idempotent, and a retry must not strand "+
			"the session the first call returned", first, second)
	}
}

// Two connections are two sessions. A token shared between callers would make
// a resume hand one caller another's coordination state.
func TestTwoConnectionsCarryDifferentSessions(t *testing.T) {
	sock := up(t)

	a := sessionOf(t, dial(t, sock)).GetSession()
	b := sessionOf(t, dial(t, sock)).GetSession()

	if a == b {
		t.Fatalf("two separate connections were given the same session token %q", a)
	}
}

// SECTION 14 ROW 4: a registered program is the one caller kind with a
// handshake, so it is the one that never has to ask. This is the half that
// closes the adoption hole a method alone would have left - `request_id` is
// the recorded instance of an opt-in wire field nothing ever set.
func TestARegisteredProgramIsGivenItsTokenByHelloWithoutAsking(t *testing.T) {
	sock := up(t)
	c := dial(t, sock)
	c.Handle(func(_ string, _ []byte) (proto.Message, error) { return nil, nil })

	resp, err := c.Hello(ctx5(t), testDeclaration("shelf"))
	if err != nil {
		t.Fatalf("hello: %v", err)
	}

	if resp.GetSession() == "" {
		t.Fatal("HelloResponse carried no session token, so a registered program " +
			"would have to opt in to a mechanism section 5f says every connection " +
			"already carries")
	}
	if !resp.GetScoped() {
		t.Error("scoped is false after a successful hello")
	}

	// The token hello handed back is THIS connection's, not a second one.
	if got := sessionOf(t, c).GetSession(); got != resp.GetSession() {
		t.Errorf("hello gave token %q and rig.session gave %q on the same "+
			"connection; a program holding two would resume the wrong one",
			resp.GetSession(), got)
	}
}

// A RESUME IS REFUSED, LOUDLY, AND IS NEVER A QUIET FRESH MINT.
//
// At M6 a session holds nothing - leases, claims and subscriptions are M7, and
// section 5f puts the dedup window in the WAL, which is M7 too - so
// SESSION_DEAD is the TRUE answer to "did you keep my state". The failure this
// pins is the alternative: replying with a new token and resumed=true, which
// would tell a caller rig restored a session it did not restore.
func TestAResumeIsSessionDeadRatherThanAQuietlyMintedNewSession(t *testing.T) {
	c := dial(t, up(t))
	mine := sessionOf(t, c).GetSession()

	out := &rigv1.SessionResponse{}
	err := c.Call(ctx5(t), "rig.session", &rigv1.SessionRequest{Resume: mine}, out)

	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_SESSION_DEAD {
		t.Fatalf("a resume answered %v, want CODE_SESSION_DEAD. Section 5f names "+
			"that answer, and anything else lets a caller believe its "+
			"coordination state came back", err)
	}
	if out.GetResumed() {
		t.Error("a refused resume still set resumed=true")
	}
	if out.GetSession() != "" {
		t.Errorf("a refused resume handed back token %q; a caller that asked to "+
			"resume and received a session would believe it kept what it lost",
			out.GetSession())
	}
}

// SECTION 36, V18: THE TOKEN IS A GENERATION IDENTITY AND SessionID IS A
// CONNECTION ONE, AND THEY MUST NOT BE THE SAME VALUE.
//
// V18 rules that a connection identity is the one lifetime "no peer should
// ever hold", and measured that today session ids are "minted per connection
// by the daemon and never travel to any caller". This method makes a token
// travel to a caller, so handing out SessionID would ship precisely the defect
// V18 names - under a field whose name makes it look correct.
func TestTheTokenThatTravelsIsNotTheConnectionId(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	p := newPrincipal(left)

	if p.Token == "" {
		t.Fatal("a fresh principal has no token, so it was not minted at accept")
	}
	if p.Token == p.SessionID {
		t.Fatalf("Token and SessionID are the same value %q. SessionID names one "+
			"CONNECTION (V18's connection row, which no peer may hold) and Token "+
			"outlives one, so making them equal publishes the connection id", p.Token)
	}
}

// SECTION 14 ROW 6: a scheduled fire, a house rule and an internal timer have
// NO CONNECTION, so they have no session and can present none. The token is
// empty for them by construction rather than by exemption, and the method
// refuses rather than inventing one.
func TestACallerWithNoConnectionHasNoSession(t *testing.T) {
	var own kernel.Principal // rig's own principal: no connection, no accept
	if own.Token != "" {
		t.Fatalf("a principal with no connection carries token %q", own.Token)
	}
}

// rig.session is DECLARED, so the invoker resolves it to declared effects
// instead of treating it as unresolvable - which is what a house rule matches
// on. self.go says getting this wrong is what lets a rule fail to catch a call.
func TestSessionIsDeclaredLikeEveryOtherRigCommand(t *testing.T) {
	var found *kernel.Command
	for i, c := range selfDeclaration().Commands {
		if c.ID == "session" {
			found = &selfDeclaration().Commands[i]
			break
		}
	}
	if found == nil {
		t.Fatal("rig.session is not in rig's own declaration, so the invoker " +
			"cannot resolve it to declared effects")
	}
	if found.Effects != kernel.EffectsReadOnly {
		t.Errorf("session declares effects %v, want read-only: it reads the token "+
			"minted at accept and touches nothing outside rig", found.Effects)
	}
	if found.Idempotent != kernel.Yes {
		t.Errorf("session declares idempotent %v, want yes: asking twice returns "+
			"the same token and leaves the same state", found.Idempotent)
	}
}
