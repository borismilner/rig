package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

const sinceSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["since"],
  "properties": {
    "since": {"type": "string", "pattern": "^[0-9]+[dh]$"},
    "dry_run": {"type": "boolean"}
  }
}`

// withArgs registers one program whose reindex command declares sinceSchema
// and whose ping command declares no arguments at all.
func withArgs(t *testing.T) *kernel.Kernel {
	t.Helper()
	k := kernel.New()
	d := good("shelf")
	template := d.Commands[0]

	reindex := template
	reindex.ID = "reindex"
	reindex.Args = []byte(sinceSchema)

	ping := template
	ping.ID = "ping"
	ping.Args = nil
	ping.Effects = kernel.EffectsReadOnly

	d.Commands = []kernel.Command{reindex, ping}
	if _, err := k.Register(programPrincipal("shelf"), d); err != nil {
		t.Fatalf("register: %v", err)
	}
	return k
}

func TestArgumentsAreValidatedAgainstWhatTheProgramDeclared(t *testing.T) {
	k := withArgs(t)

	if err := k.ValidateArgs("shelf", "reindex", []byte(`{"since":"7d"}`)); err != nil {
		t.Fatalf("valid arguments were refused: %v", err)
	}
	if err := k.ValidateArgs("shelf", "reindex",
		[]byte(`{"since":"7d","dry_run":true}`)); err != nil {
		t.Fatalf("valid arguments were refused: %v", err)
	}

	bad := map[string]string{
		"wrong type":     `{"since":7}`,
		"pattern":        `{"since":"soon"}`,
		"unknown field":  `{"since":"7d","untl":"now"}`,
		"missing":        `{}`,
		"nothing at all": ``,
		"not json":       `{since:`,
		"not an object":  `["7d"]`,
	}
	for name, args := range bad {
		t.Run(name, func(t *testing.T) {
			err := k.ValidateArgs("shelf", "reindex", []byte(args))
			if err == nil {
				t.Fatalf("%s was accepted", args)
			}
			// Section 9 wants an error an agent can act on, which means the
			// message has to say which command and what was wrong.
			if !strings.Contains(err.Error(), "reindex") {
				t.Fatalf("the error does not name the command: %v", err)
			}
		})
	}
}

func TestACommandThatDeclaredNoSchemaTakesNoArguments(t *testing.T) {
	k := withArgs(t)

	for _, args := range []string{``, `{}`, `null`} {
		if err := k.ValidateArgs("shelf", "ping", []byte(args)); err != nil {
			t.Fatalf("%q was refused for a command with no arguments: %v", args, err)
		}
	}
	// Silently dropping them is the failure this refuses: a caller that
	// passed an argument believes it had an effect.
	err := k.ValidateArgs("shelf", "ping", []byte(`{"since":"7d"}`))
	if err == nil {
		t.Fatal("arguments were accepted by a command that declares none")
	}
	if !strings.Contains(err.Error(), "takes none") {
		t.Fatalf("the error does not say why: %v", err)
	}
}

func TestValidateArgsRefusesWhatNobodyDeclared(t *testing.T) {
	k := withArgs(t)

	if err := k.ValidateArgs("nosuch", "reindex", nil); err == nil {
		t.Fatal("a call for an unregistered program passed validation")
	}
	// The boundary validates before routing, so a command nobody declared
	// must not reach a program on the grounds that there was no schema.
	err := k.ValidateArgs("shelf", "nosuch", nil)
	if err == nil {
		t.Fatal("a call for an undeclared command passed validation")
	}
	if !strings.Contains(err.Error(), "declares no command") {
		t.Fatalf("the error does not say what is wrong: %v", err)
	}
}

func TestASchemaThatDoesNotCompileIsRefusedAtRegistration(t *testing.T) {
	for name, schema := range map[string]string{
		"not json":      `{"type":`,
		"not a schema":  `{"type": 7}`,
		"bad regex":     `{"type":"object","properties":{"x":{"pattern":"["}}}`,
		"not an object": `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			k := kernel.New()
			d := good("shelf")
			d.Commands[0].Args = []byte(schema)
			_, err := k.Register(programPrincipal("shelf"), d)
			if err == nil {
				t.Fatalf("a declaration carrying %s was registered", name)
			}
			if _, ok := k.See(caller(kernel.KindAgent)).Program("shelf"); ok {
				t.Fatal("the refused declaration is in the registry anyway")
			}
		})
	}
}

func TestTheDeclaredSchemaIsReadableThroughTheScopeFilter(t *testing.T) {
	k := withArgs(t)

	// An introspecting client reads it, which is what generated help and
	// completion are built from.
	looker := caller(kernel.KindAgent)
	raw, ok := k.See(looker).ArgSchema("shelf", "reindex")
	if !ok {
		t.Fatal("an introspecting principal cannot read the declared schema")
	}
	if !strings.Contains(string(raw), "since") {
		t.Fatalf("the schema came back as %q", raw)
	}

	// A command with no schema has none to read, and that is not an error.
	if _, ok := k.See(looker).ArgSchema("shelf", "ping"); ok {
		t.Fatal("a command with no declared schema returned one")
	}

	// Another program does not read it: this read is the filtered one,
	// unlike the boundary's.
	other, err := k.Register(programPrincipal("grabbit"), good("grabbit"))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := k.See(other).ArgSchema("shelf", "reindex"); ok {
		t.Fatal("one program read another program's argument schema")
	}
}

// TestARefusalNamesThePreconditionAndTheFixCommand locks PLAN.md section 9's
// requirement that a failed call returns structure rather than prose: what
// failed, which precondition, what was actually found, and the exact command
// that fixes it.
//
// The fix command is asserted to be the program's OWN declared example rather
// than any string rig composes. Section 9 calls examples "the single
// highest-value field", and a fix_command rig invented would be the one field
// in the error nobody had checked.
func TestARefusalNamesThePreconditionAndTheFixCommand(t *testing.T) {
	k := kernel.New()
	d := good("shelf")
	c := d.Commands[0]
	c.ID = "reindex"
	c.Args = []byte(sinceSchema)
	c.Examples = []string{"rig shelf reindex --since 7d"}
	d.Commands = []kernel.Command{c}
	if _, err := k.Register(programPrincipal("shelf"), d); err != nil {
		t.Fatalf("register: %v", err)
	}

	err := k.ValidateArgs("shelf", "reindex", []byte(`{"since":"soon"}`))
	if err == nil {
		t.Fatal("a value the schema refuses was accepted")
	}
	f, ok := kernel.AsRefusal(err)
	if !ok {
		t.Fatalf("a schema refusal is not a RefusalError, so no surface can render "+
			"section 9's fields: %v", err)
	}
	if !strings.Contains(f.Precondition, "since") {
		t.Errorf("precondition does not name the failing field: %q", f.Precondition)
	}
	if !strings.Contains(f.Actual, "soon") {
		t.Errorf("actual does not name the value that was sent: %q", f.Actual)
	}
	if f.FixCommand != "rig shelf reindex --since 7d" {
		t.Errorf("fix command is not the program's declared example: %q", f.FixCommand)
	}
	// The sentence survives, because everything already handling a kernel
	// error keeps working and the structure is an addition.
	if !strings.Contains(f.Error(), "declared schema") {
		t.Errorf("the message was replaced rather than added to: %q", f.Error())
	}
}

// TestARefusalWithNothingTrueToSayLeavesTheFieldsEMPTY is the other half, and
// it is the one that keeps the fields worth reading. A command declaring no
// examples gets no fix command - not a composed one - because the value of
// these fields is that they are true when set.
func TestARefusalWithNothingTrueToSayLeavesTheFieldsEmpty(t *testing.T) {
	k := withArgs(t) // its commands declare no Examples

	f, ok := kernel.AsRefusal(k.ValidateArgs("shelf", "reindex", []byte(`{"since":"soon"}`)))
	if !ok {
		t.Fatal("expected a RefusalError")
	}
	if f.FixCommand != "" {
		t.Errorf("a fix command was invented for a command that declares no "+
			"examples: %q", f.FixCommand)
	}
}
