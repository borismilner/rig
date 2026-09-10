package kernel

import (
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Registry holds what every program declared.
//
// It never leaves this package, and that is enforced rather than asked for:
// the noregistryhandle analyzer fails the build on the type being named
// outside internal/kernel, and on any exported function here returning it.
// Section 14 wants an unscoped read to be unrepresentable rather than
// discouraged, and an allowlist of filtered views is exactly how a seventh
// view leaks.
//
// The shape that falls out of the rule is worth stating, because writing it
// the obvious way does not compile past the gate: a caller cannot hold a
// Registry, so it holds a Kernel and asks for a View.
type Registry struct {
	mu       sync.RWMutex
	programs map[string]entry
}

type entry struct {
	decl  Declaration
	owner Principal
	scope string

	// schemas is the compiled argument schema per command, built once at
	// registration. A command that declared none is absent rather than nil,
	// and absent means the command takes no arguments (see ValidateArgs).
	schemas map[string]*jsonschema.Schema
}

// Kernel is what a caller outside this package holds.
type Kernel struct {
	registry *Registry

	// rules is the house rules table the invoker matches on (section 13a).
	// It has its own lock because config pushes it live (section 6) while
	// calls are being authorised against it, and it is nothing to do with
	// the registry's.
	rmu   sync.RWMutex
	rules []Rule
}

// New builds a kernel with an empty registry and the shipped house rules.
//
// The rules are not empty, and section 13a is explicit about which two ship:
// the url defaults. Every other caller starts unmatched.
func New() *Kernel {
	return &Kernel{
		registry: &Registry{programs: map[string]entry{}},
		rules:    DefaultRules(),
	}
}

// Register records a program's declaration, on the connection that made it.
//
// The principal must be a program: section 14's predicate is that a
// connection completing the program handshake is a program, and is scoped for
// the life of that connection. Registration is the only thing that sets
// Scoped, so this is where that happens.
func (k *Kernel) Register(p Principal, d Declaration) (Principal, error) {
	if p.Kind != KindProgram {
		return p, fmt.Errorf("kernel: a %s connection cannot register a program", p.Kind)
	}
	if err := d.Validate(); err != nil {
		return p, err
	}

	// A declared schema that does not compile is a registration rig cannot
	// reason about, and this is the only moment at which telling the
	// program's author is cheap.
	schemas, err := compileDeclaredArgs(d)
	if err != nil {
		return p, err
	}

	scope := d.Scope
	if scope == "" {
		scope = d.Identity.ID
	}
	p.Scoped = true
	p.Scopes = append(slices.Clone(p.Scopes), scope)
	if err := p.Valid(); err != nil {
		return p, err
	}

	k.registry.mu.Lock()
	defer k.registry.mu.Unlock()
	if prev, taken := k.registry.programs[d.Identity.ID]; taken &&
		prev.owner.SessionID != p.SessionID {
		return p, fmt.Errorf("kernel: program %q is already registered by %s",
			d.Identity.ID, prev.owner)
	}
	k.registry.programs[d.Identity.ID] = entry{
		decl: d, owner: p, scope: scope, schemas: schemas,
	}
	return p, nil
}

// Deregister forgets a program when its connection goes, so a restarted
// program can take its id back.
func (k *Kernel) Deregister(sessionID string) {
	k.registry.mu.Lock()
	defer k.registry.mu.Unlock()
	for id, e := range k.registry.programs {
		if e.owner.SessionID == sessionID {
			delete(k.registry.programs, id)
		}
	}
}

// View is the only way to read the registry, and it cannot be built without a
// principal. The principal is in the type, so there is no unscoped read to
// forget to filter.
type View struct {
	r *Registry
	p Principal
}

// See opens the registry through one principal's eyes.
func (k *Kernel) See(p Principal) View { return View{r: k.registry, p: p} }

// Principal is who this view reads as.
func (v View) Principal() Principal { return v.p }

// Program is one program as this principal may see it.
type Program struct {
	Identity     Identity
	Coverage     Coverage
	CoverageNote string
	SemanticsGen int32
	Services     []string
	Hosted       bool
	PaneURL      string
	Commands     []Command
}

// Programs lists what this principal may reach, by id.
//
// A principal sees itself and the programs it may reach, and nothing else: it
// cannot enumerate other clients and does not appear in their views. An
// introspecting principal sees all of it, which is the one door out and is
// decided at connect time.
func (v View) Programs() []Program {
	v.r.mu.RLock()
	defer v.r.mu.RUnlock()

	var out []Program
	for _, e := range v.r.programs {
		if !v.canSee(e) {
			continue
		}
		out = append(out, program(e))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Identity.ID < out[j].Identity.ID
	})
	return out
}

// Program reads one program by id, or reports that this principal cannot see
// it. A program it may not see is reported missing rather than refused,
// because "you may not see shelf" tells the caller that shelf exists.
func (v View) Program(id string) (Program, bool) {
	v.r.mu.RLock()
	defer v.r.mu.RUnlock()

	e, ok := v.r.programs[id]
	if !ok || !v.canSee(e) {
		return Program{}, false
	}
	return program(e), true
}

// Command reads one command of one program.
func (v View) Command(programID, commandID string) (Command, bool) {
	p, ok := v.Program(programID)
	if !ok {
		return Command{}, false
	}
	for _, c := range p.Commands {
		if c.ID == commandID {
			return c, true
		}
	}
	return Command{}, false
}

// command reads one command's declaration with no filter at all.
//
// This is the invoker's read and nothing else's, and the reason it bypasses
// the scope filter is stated in Kernel.pair: the effects the invoker matches
// on are what the PROGRAM declared, and reading them through the caller's own
// view made a legitimate read-only call resolve as destructive whenever the
// caller happened not to be in the target's scope. It returns a Command, not
// a Program and never a Registry, so noregistryhandle's rule still holds.
func (r *Registry) command(programID, commandID string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.programs[programID]
	if !ok {
		return Command{}, false
	}
	for _, c := range e.decl.Commands {
		if c.ID == commandID {
			return c, true
		}
	}
	return Command{}, false
}

// canSee is the whole filter, in one place.
func (v View) canSee(e entry) bool {
	if v.p.Introspect {
		return true
	}
	// A program sees itself.
	if v.p.Scoped && v.p.ClientID == e.decl.Identity.ID {
		return true
	}
	return v.p.InScope(e.scope)
}

func program(e entry) Program {
	return Program{
		Identity:     e.decl.Identity,
		Coverage:     e.decl.Coverage,
		CoverageNote: e.decl.CoverageNote,
		SemanticsGen: e.decl.SemanticsGen,
		Services:     slices.Clone(e.decl.Services),
		Hosted:       e.decl.Hosted,
		PaneURL:      e.decl.PaneURL,
		Commands:     cloneCommands(e.decl.Commands),
	}
}

// cloneCommands copies the slice fields inside each command as well as the
// slice of commands.
//
// slices.Clone alone is shallow: the Command structs are copied but their
// Sensitive, Examples, Args and Preconditions still point at the registry's
// own memory, so a reader holding a View could edit what a program declared.
// Nothing does that today, which is exactly why it would have been found
// late.
func cloneCommands(in []Command) []Command {
	if in == nil {
		return nil
	}
	out := make([]Command, len(in))
	for i, c := range in {
		c.Sensitive = slices.Clone(c.Sensitive)
		c.Examples = slices.Clone(c.Examples)
		c.Args = slices.Clone(c.Args)
		c.Preconditions = slices.Clone(c.Preconditions)
		out[i] = c
	}
	return out
}
