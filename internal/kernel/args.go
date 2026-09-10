package kernel

import (
	"bytes"
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
	if known {
		schema, declared = e.schemas[commandID]
		if !declared {
			declared = hasCommand(e.decl, commandID)
		}
	}
	k.registry.mu.RUnlock()

	if !known {
		return fmt.Errorf("kernel: no program %q is registered", programID)
	}
	if !declared {
		return fmt.Errorf("kernel: program %q declares no command %q",
			programID, commandID)
	}

	if schema == nil {
		// No declared schema, so no arguments. Empty and `{}` both pass,
		// because a caller that sent an empty object sent no arguments.
		if isEmptyArgs(args) {
			return nil
		}
		return fmt.Errorf("kernel: command %q.%q declares no arguments, so it "+
			"takes none", programID, commandID)
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
		return fmt.Errorf("kernel: arguments for %q.%q are not JSON: %w",
			programID, commandID, err)
	}
	if err := schema.Validate(doc); err != nil {
		// The library's error carries the failing JSON pointer and the
		// keyword, which is what section 9 means by an error an agent can act
		// on. Wrapping it keeps that detail rather than replacing it with a
		// sentence.
		return fmt.Errorf("kernel: arguments for %q.%q do not match the "+
			"declared schema: %w", programID, commandID, err)
	}
	return nil
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

// hasCommand reports whether a declaration names this command at all.
func hasCommand(d Declaration, commandID string) bool {
	for _, c := range d.Commands {
		if c.ID == commandID {
			return true
		}
	}
	return false
}
