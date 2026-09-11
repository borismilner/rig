package kernel

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Arguments are validated against the schema the PROGRAM declared, at the
// boundary, before anything is invoked (PLAN.md sections 5e, 9).
//
// The schema is compiled once, at registration, and never per call. Two
// reasons, and only the second is about speed: a schema that does not compile
// is a registration rig cannot reason about, and refusing it at connect is
// the difference between one clear error to the program's author and a
// mysterious failure on the first call months later. Section 20's
// conformance suite has a program that "lies about its schema" precisely
// because that failure mode was expected.

// resourceURL is the name a declared schema is compiled under.
//
// It is per command rather than a constant, so a validation error names which
// command's schema it came from without the caller having to add it.
func resourceURL(programID, commandID string) string {
	return fmt.Sprintf("rig://schema/args/%s/%s", programID, commandID)
}

// compileArgs compiles one command's declared argument schema.
//
// A command with no declared schema gets nil, and nil means "this command
// takes no arguments" rather than "anything goes". A program that declared no
// argument schema declared no arguments, and the caller who passes some is
// better told than silently ignored - section 5e's rule that no absence may
// carry a permissive meaning.
func compileArgs(programID string, c Command) (*jsonschema.Schema, error) {
	if len(bytes.TrimSpace(c.Args)) == 0 {
		return nil, nil
	}
	url := resourceURL(programID, c.ID)

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(c.Args))
	if err != nil {
		return nil, fmt.Errorf("command %q: args is not JSON: %w", c.ID, err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("command %q: args is not a schema: %w", c.ID, err)
	}
	s, err := compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("command %q: args does not compile: %w", c.ID, err)
	}
	return s, nil
}

// compileDeclaredArgs compiles every command's schema, or refuses the lot.
//
// It reports every problem rather than the first, for the same reason
// Declaration.Validate does: an author fixing a generated declaration one
// error per run stops generating it.
func compileDeclaredArgs(d Declaration) (map[string]*jsonschema.Schema, error) {
	out := make(map[string]*jsonschema.Schema, len(d.Commands))
	var bad []string
	for _, c := range d.Commands {
		s, err := compileArgs(d.Identity.ID, c)
		if err != nil {
			bad = append(bad, err.Error())
			continue
		}
		if s != nil {
			out[c.ID] = s
		}
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("kernel: registration refused:\n  - %s",
			strings.Join(bad, "\n  - "))
	}
	return out, nil
}

// ValidateArgs checks one call's arguments against what the program declared.
//
// It is addressed by program and command rather than through a View: this is
// the boundary's read, the same as the invoker's, and for the same reason -
// what the arguments must satisfy is what the TARGET declared, not what the
// caller may read. See Kernel.pair.
//
// An unknown command is an error here rather than a silent pass. The boundary
// calls this before routing, so a call for a command nobody declared must not
// reach a program on the grounds that there was no schema to fail.
func (k *Kernel) ValidateArgs(programID, commandID string, args []byte) error {
	k.registry.mu.RLock()
	e, known := k.registry.programs[programID]
	var schema *jsonschema.Schema
	var declared bool
	var examples []string
	if known {
		schema, declared = e.schemas[commandID]
		if c, ok := commandByID(e.decl, commandID); ok {
			declared = true
			examples = c.Examples
		}
	}
	k.registry.mu.RUnlock()

	if !known {
		return refusalf("kernel: no program %q is registered", programID).with(
			fmt.Sprintf("a program named %q is connected", programID),
			fmt.Sprintf("no program named %q is connected", programID),
			"start the program, then list what is connected",
			"rig apps list")
	}
	if !declared {
		return refusalf("kernel: program %q declares no command %q",
			programID, commandID).with(
			fmt.Sprintf("%s declares a command named %q", programID, commandID),
			fmt.Sprintf("%s declares %s", programID, declaredList(e.decl)),
			"call a command the program declared; rig knows only what it declared",
			"rig "+programID+" --help")
	}

	if schema == nil {
		// No declared schema, so no arguments. Empty and `{}` both pass,
		// because a caller that sent an empty object sent no arguments.
		if isEmptyArgs(args) {
			return nil
		}
		return refusalf("kernel: command %q.%q declares no arguments, so it "+
			"takes none", programID, commandID).with(
			fmt.Sprintf("%s.%s is called with no arguments", programID, commandID),
			"arguments were sent",
			"drop the arguments; an absence must not carry a permissive meaning",
			"rig "+programID+" "+commandID)
	}

	// A schema exists, so the arguments have to be JSON. Absent arguments are
	// validated as the empty object rather than skipped: a schema with a
	// required field must refuse a call that sent nothing.
	body := args
	if isEmptyArgs(body) {
		body = []byte(`{}`)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		// No precondition is named here on purpose. The failure is that the
		// bytes are not JSON at all, so there is no field to point at, and
		// inventing one would be a guess wearing a measurement's clothes.
		return &RefusalError{
			Err: fmt.Errorf("kernel: arguments for %q.%q are not JSON: %w",
				programID, commandID, err),
			Fix:        "send the arguments as a JSON object",
			FixCommand: firstExample(examples),
		}
	}
	if err := schema.Validate(doc); err != nil {
		// The library's error carries the failing JSON pointer and the
		// keyword, which is what section 9 means by an error an agent can act
		// on. Wrapping it keeps that detail rather than replacing it with a
		// sentence, and the structured fields below lift the DEEPEST cause out
		// of it - the leaf is the one that names a real value, while the root
		// only says the document failed.
		f := &RefusalError{
			Err: fmt.Errorf("kernel: arguments for %q.%q do not match "+
				"the declared schema: %w", programID, commandID, err),
			Fix:        "pass arguments matching the schema the program declared",
			FixCommand: firstExample(examples),
		}
		if ve, ok := errors.AsType[*jsonschema.ValidationError](err); ok {
			leaf := deepest(ve)
			f.Precondition = pointer(leaf.InstanceLocation) + " satisfies " +
				strings.Join(leaf.ErrorKind.KeywordPath(), ".")
			// leaf.Error() rather than ErrorKind: the
			// printer-based form needs a golang.org/x/text printer, and making
			// that dependency DIRECT would need a section 22 row for a
			// message formatter rig does not otherwise use. The leaf's own
			// Error() already names the value and the keyword.
			f.Actual = strings.TrimSpace(leaf.Error())
		}
		return f
	}
	return nil
}

// deepest walks to the leaf cause of a validation error. The root says the
// document did not validate, which no caller can act on; the leaf says which
// value broke which keyword, which is the whole of section 9's requirement.
// A tie is broken by taking the first, because reporting one true failing
// field beats enumerating several.
func deepest(e *jsonschema.ValidationError) *jsonschema.ValidationError {
	for len(e.Causes) > 0 {
		e = e.Causes[0]
	}
	return e
}

// firstExample returns the declaration's first example, which section 9 calls
// "the single highest-value field". It is a real invocation written by the
// program's own author, so it is the one fix_command rig can offer without
// inventing anything. No examples means no command, never a guessed one.
func firstExample(examples []string) string {
	if len(examples) == 0 {
		return ""
	}
	return examples[0]
}

// declaredList names what the program does declare, so a caller that guessed
// a command sees the real set instead of being told only that it was wrong.
func declaredList(d Declaration) string {
	if len(d.Commands) == 0 {
		return "no commands"
	}
	ids := make([]string, 0, len(d.Commands))
	for _, c := range d.Commands {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ", ")
}

// commandByID finds a declared command. It replaces hasCommand's bare
// predicate because the refusal now wants the command's examples too.
func commandByID(d Declaration, commandID string) (Command, bool) {
	for _, c := range d.Commands {
		if c.ID == commandID {
			return c, true
		}
	}
	return Command{}, false
}

// ArgSchema hands back the raw declared schema for one command, as this
// principal may see it.
//
// It is the View's read rather than the boundary's, because this one is for
// showing a person or an agent what a command takes - generated help,
// completion, a form - and that is exactly the read section 14 filters.
func (v View) ArgSchema(programID, commandID string) ([]byte, bool) {
	c, ok := v.Command(programID, commandID)
	if !ok {
		return nil, false
	}
	if len(bytes.TrimSpace(c.Args)) == 0 {
		return nil, false
	}
	return c.Args, true
}

// isEmptyArgs reports whether a caller sent nothing worth validating.
func isEmptyArgs(args []byte) bool {
	t := bytes.TrimSpace(args)
	return len(t) == 0 || bytes.Equal(t, []byte("null")) || bytes.Equal(t, []byte("{}"))
}
