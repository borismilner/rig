// Package config is section 6's resolver, built as plan/47 specifies: every
// key declared in one JSON Schema, eight layers lowest to highest, the winner
// AND every loser recorded per key, a change set validated whole, and the
// result written to a snapshot on disk with its provenance.
//
// ONLY rigd LINKS THIS PACKAGE (plan/47 decision 15). A client reads its
// settings over the wire or from the snapshot, never by parsing a file, so
// there is exactly one implementation of resolution.
package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema.json
var rigSchema []byte

// SchemaID names rig's own schema in the snapshot.
const SchemaID = "rig://schema/rig"

// runtimeLayer is the layer config.set writes.
const runtimeLayer = "runtime"

// Layers are section 6's, lowest first. The two program layers are wired and
// stay empty until a program adopts the service (plan/47 decision 1).
var Layers = []string{"default", "system", "user", "program-default", "program-file", "env", "flag", runtimeLayer}

const (
	layerDefault = iota
	layerSystem
	layerUser
	layerProgramDefault
	layerProgramFile
	layerEnv
	layerFlag
	layerRuntime
)

// NotSettings are rig's RIG_* variables that are not configuration: identity,
// test harness and child-process plumbing. Any other RIG_* variable that names
// no key is an orphan (plan/47 decision 6), which is how a misspelling shows.
var NotSettings = map[string]bool{
	"RIG_ROOT": true, "RIG_ESTATE": true, "RIG_SOCKET": true, "RIG_WIRE": true,
	"RIG_PROGRAM_ID": true, "RIG_LOGBOOK": true, "RIG_SOAK": true,
	"RIG_LOCK_CHILD": true, "RIG_LOCK_PATH": true, "RIG_NAME_CHILD": true,
	"RIG_RECORD_CRASH_CHILD": true,
}

// Key is one declared setting.
type Key struct {
	Name        string // dotted, as in a file: log.level
	Env         string // RIG_LOG_LEVEL
	Flag        string // log-level
	Type        string // the schema's type keyword
	Apply       string // live | restart
	Description string
	Default     any
}

// Schema is a compiled set of declared keys.
type Schema struct {
	keys     []Key
	byName   map[string]Key
	byEnv    map[string]Key
	compiled *jsonschema.Schema
}

// RigSchema is rig's own schema, embedded in the binary.
func RigSchema() (*Schema, error) { return NewSchema(rigSchema) }

// MustRigSchema is RigSchema for a caller with no way on: the document is
// embedded and compiled by this package's tests, so a failure is a build
// that never passed them.
func MustRigSchema() *Schema {
	s, err := RigSchema()
	if err != nil {
		panic(err)
	}
	return s
}

// NewSchema compiles a schema document and lists its leaf keys.
func NewSchema(doc []byte) (*Schema, error) {
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return nil, fmt.Errorf("config schema is not JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(SchemaID, parsed); err != nil {
		return nil, fmt.Errorf("config schema: %w", err)
	}
	compiled, err := c.Compile(SchemaID)
	if err != nil {
		return nil, fmt.Errorf("config schema does not compile: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(doc, &raw); err != nil {
		return nil, err
	}
	s := &Schema{byName: map[string]Key{}, byEnv: map[string]Key{}, compiled: compiled}
	if err := s.walk("", raw); err != nil {
		return nil, err
	}
	slices.SortFunc(s.keys, func(a, b Key) int { return strings.Compare(a.Name, b.Name) })
	return s, nil
}

func (s *Schema) walk(prefix string, node map[string]any) error {
	props, ok := node["properties"].(map[string]any)
	if !ok {
		if prefix == "" {
			return errors.New("config schema declares no properties")
		}
		k := Key{Name: prefix, Default: node["default"]}
		k.Env = "RIG_" + strings.ToUpper(strings.ReplaceAll(prefix, ".", "_"))
		k.Flag = strings.ReplaceAll(prefix, ".", "-")
		k.Type, _ = node["type"].(string)
		k.Description, _ = node["description"].(string)
		k.Apply, _ = node["x-rig-apply"].(string)
		if k.Apply != "live" && k.Apply != "restart" {
			return fmt.Errorf("config schema: %s has x-rig-apply %q, not live or restart", prefix, k.Apply)
		}
		if _, ok := node["default"]; !ok {
			return fmt.Errorf("config schema: %s declares no default", prefix)
		}
		if other, dup := s.byEnv[k.Env]; dup {
			return fmt.Errorf("config schema: %s and %s both spell %s", other.Name, k.Name, k.Env)
		}
		s.keys = append(s.keys, k)
		s.byName[k.Name] = k
		s.byEnv[k.Env] = k
		return nil
	}
	for name, child := range props {
		sub, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("config schema: %s%s is not an object", prefix, name)
		}
		full := name
		if prefix != "" {
			full = prefix + "." + name
		}
		if err := s.walk(full, sub); err != nil {
			return err
		}
	}
	return nil
}

// Keys lists the declared keys, sorted.
func (s *Schema) Keys() []Key { return slices.Clone(s.keys) }

// Flags declares one string flag per key on fs, spelled as the key with
// dashes (plan/47 decision 14), and returns what was given after Parse.
func (s *Schema) Flags(set *flag.FlagSet) func() map[string]string {
	vals := map[string]*string{}
	for _, k := range s.keys {
		vals[k.Flag] = set.String(k.Flag, "", k.Description+" (setting "+k.Name+")")
	}
	return func() map[string]string {
		out := map[string]string{}
		set.Visit(func(f *flag.Flag) {
			if v, ok := vals[f.Name]; ok {
				for _, k := range s.keys {
					if k.Flag == f.Name {
						out[k.Name] = *v
					}
				}
			}
		})
		return out
	}
}

// Sources are where the layers come from.
type Sources struct {
	SystemFile, UserFile string
	Environ              []string
	Flags                map[string]string // key -> raw text
}

// Source is one layer's value for a key.
type Source struct {
	Layer string
	File  string // a file layer's path; empty otherwise
	Value any
}

// Value is one key resolved: the winner and every loser, lowest first.
type Value struct {
	Key    string
	Apply  string
	Winner Source
	Losers []Source
}

type entry struct {
	value any
	file  string
}

// Resolver holds every layer and answers the resolution.
type Resolver struct {
	schema *Schema

	mu       sync.Mutex
	writeMu  sync.Mutex // one snapshot write at a time, so the last rename is the latest state
	layers   [8]map[string]entry
	orphans  []string
	problems []string

	// appFile is a program's ~/.config/rig/apps/<id>.toml when this
	// resolves a program's settings (LoadProgram): Set writes there, so a
	// change lasts, rather than to the runtime layer a restart clears.
	program bool
	appFile string
}

// Load reads every layer. It does not fail: a file that cannot be read or
// parsed, and a value its key's schema refuses, are PROBLEMS, reported in
// every answer and in the snapshot, and that layer does not take part for
// the key. A daemon that refused to start over a typo in a settings file
// would take everything else down with it.
func Load(s *Schema, src Sources) *Resolver {
	r := &Resolver{schema: s}
	for i := range r.layers {
		r.layers[i] = map[string]entry{}
	}
	for _, k := range s.keys {
		r.layers[layerDefault][k.Name] = entry{value: k.Default}
	}
	r.loadFile(layerSystem, src.SystemFile)
	r.loadFile(layerUser, src.UserFile)
	for _, kv := range src.Environ {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, "RIG_") || NotSettings[name] {
			continue
		}
		k, declared := s.byEnv[name]
		if !declared {
			r.orphans = append(r.orphans, "env::"+name)
			continue
		}
		r.put(layerEnv, k, typed(k, val), "")
	}
	for name, val := range src.Flags {
		if k, ok := s.byName[name]; ok {
			r.put(layerFlag, k, typed(k, val), "")
		}
	}
	slices.Sort(r.orphans)
	return r
}

// LoadProgram resolves a program's declared settings over the two program
// layers (plan/47 decision 1): the schema's defaults, then file, the
// program's apps/<id>.toml, which Set also writes. Environment and flags are
// rig's spellings and take no part. An empty file keeps changes in memory.
func LoadProgram(s *Schema, file string) *Resolver {
	r := &Resolver{schema: s, program: true, appFile: file}
	for i := range r.layers {
		r.layers[i] = map[string]entry{}
	}
	for _, k := range s.keys {
		r.layers[layerProgramDefault][k.Name] = entry{value: k.Default}
	}
	r.loadFile(layerProgramFile, file)
	slices.Sort(r.orphans)
	return r
}

func (r *Resolver) loadFile(layer int, path string) {
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		r.problems = append(r.problems, fmt.Sprintf("%s:%s: unreadable: %v", Layers[layer], path, err))
		return
	}
	var doc map[string]any
	if err := toml.Unmarshal(raw, &doc); err != nil {
		r.problems = append(r.problems, fmt.Sprintf("%s:%s: not TOML: %v", Layers[layer], path, err))
		return
	}
	for name, v := range flatten("", doc) {
		key, ok := r.schema.byName[name]
		if !ok {
			r.orphans = append(r.orphans, Layers[layer]+":"+path+":"+name)
			continue
		}
		r.put(layer, key, v, path)
	}
}

// put records a layer's value for a key if the key's schema takes it.
func (r *Resolver) put(layer int, k Key, v any, file string) {
	v, err := normal(v)
	if err == nil {
		err = r.schema.validate(map[string]any{k.Name: v})
	}
	if err != nil {
		where := Layers[layer]
		if file != "" {
			where += ":" + file
		}
		r.problems = append(r.problems, fmt.Sprintf("%s: %s: %s, got %s", where, k.Name, reason(err), JSON(v)))
		return
	}
	r.layers[layer][k.Name] = entry{value: v, file: file}
}

// typed reads a value given as text, from the environment, a flag or a
// command line: text for a string key, JSON otherwise.
func typed(k Key, text string) any {
	if k.Type == "string" {
		return text
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return text // the schema refuses it, naming the type
	}
	return v
}

// normal makes a value look as JSON decoding would, which is what the
// validator expects: TOML's int64 becomes a JSON number.
func normal(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}

// validate checks flat key -> value pairs as one nested document.
func (s *Schema) validate(flat map[string]any) error {
	return s.compiled.Validate(nest(flat))
}

// nest turns dotted keys into the nested document they name.
func nest(flat map[string]any) map[string]any {
	doc := map[string]any{}
	for name, v := range flat {
		parts := strings.Split(name, ".")
		node := doc
		for _, p := range parts[:len(parts)-1] {
			next, ok := node[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[p] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = v
	}
	return doc
}

// RefusalError is a change set refused whole: the key, the layer and the reason.
type RefusalError struct{ Key, Layer, Reason string }

func (e *RefusalError) Error() string {
	return fmt.Sprintf("%s (layer %s): %s", e.Key, e.Layer, e.Reason)
}

// reason is the leaf of a validation error, which names the value and the
// keyword, rather than the root, which only says the document failed.
func reason(err error) string {
	if ve, ok := errors.AsType[*jsonschema.ValidationError](err); ok {
		for len(ve.Causes) > 0 {
			ve = ve.Causes[0]
		}
		return strings.TrimSpace(ve.Error())
	}
	return err.Error()
}

// leafKey is the dotted key a validation error is about.
func leafKey(err error) string {
	if ve, ok := errors.AsType[*jsonschema.ValidationError](err); ok {
		for len(ve.Causes) > 0 {
			ve = ve.Causes[0]
		}
		return strings.Join(ve.InstanceLocation, ".")
	}
	return ""
}

// Set applies a change set to the runtime layer, or nothing (plan/47
// decision 5). Values are JSON text; a JSON string given for a key that is
// not a string is read as the key's own type, as an environment variable
// is. It answers each key's outcome, applied or needs-restart, and the keys
// whose resolved value moved.
//
// A program's resolver writes its file layer instead, and the file itself,
// so the change outlives rigd; a file that cannot be written applies nothing.
func (r *Resolver) Set(changes map[string]string) (outcome map[string]string, moved []string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	layer := layerRuntime
	if r.program {
		layer = layerProgramFile
	}
	if len(changes) == 0 {
		return nil, nil, &RefusalError{Layer: Layers[layer], Reason: "a change set names at least one key"}
	}
	candidate := map[string]any{}
	for name, text := range changes {
		k, ok := r.schema.byName[name]
		if !ok {
			return nil, nil, &RefusalError{Key: name, Layer: Layers[layer], Reason: "not a declared key; `rig config get` lists them"}
		}
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			return nil, nil, &RefusalError{Key: name, Layer: Layers[layer], Reason: "the value is not JSON: " + err.Error()}
		}
		if s, isText := v.(string); isText && k.Type != "string" {
			v = typed(k, s)
		}
		if v, err = normal(v); err != nil {
			return nil, nil, &RefusalError{Key: name, Layer: Layers[layer], Reason: err.Error()}
		}
		candidate[name] = v
	}
	whole := map[string]any{}
	for _, k := range r.schema.keys {
		whole[k.Name] = r.winnerLocked(k.Name).Value
	}
	for name, v := range candidate {
		whole[name] = v
	}
	if err := r.schema.validate(whole); err != nil {
		key := leafKey(err)
		return nil, nil, &RefusalError{Key: key, Layer: Layers[layer], Reason: reason(err) + ", got " + JSON(whole[key])}
	}
	if r.program && r.appFile != "" {
		if err := r.writeAppFile(candidate); err != nil {
			return nil, nil, &RefusalError{Layer: Layers[layer], Reason: "not written to " + r.appFile + ": " + err.Error()}
		}
	}
	outcome = map[string]string{}
	for name, v := range candidate {
		before := r.winnerLocked(name).Value
		r.layers[layer][name] = entry{value: v, file: r.appFile}
		if !equal(before, v) {
			moved = append(moved, name)
		}
		outcome[name] = map[string]string{"live": "applied", "restart": "needs-restart"}[r.schema.byName[name].Apply]
	}
	slices.Sort(moved)
	return outcome, moved, nil
}

// writeAppFile writes the program's file layer with candidate over it,
// through a temporary file and a rename. The file is rig's to write: a
// comment in it does not survive a change made through config.set.
func (r *Resolver) writeAppFile(candidate map[string]any) error {
	flat := map[string]any{}
	for name, e := range r.layers[layerProgramFile] {
		flat[name] = e.value
	}
	maps.Copy(flat, candidate)
	for name, v := range flat {
		flat[name] = plain(v)
	}
	body, err := toml.Marshal(nest(flat))
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.appFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".app-*.toml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(append([]byte("# written by rigd: config.set with program set\n"), body...)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), r.appFile)
}

// plain undoes normal for a TOML writer, which would quote a json.Number:
// a whole number becomes an int64, any other a float64.
func plain(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		f, _ := x.Float64()
		return f
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plain(e)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = plain(e)
		}
		return out
	}
	return v
}

func equal(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (r *Resolver) winnerLocked(name string) Source {
	for i := len(r.layers) - 1; i >= 0; i-- {
		if e, ok := r.layers[i][name]; ok {
			return Source{Layer: Layers[i], File: e.file, Value: e.value}
		}
	}
	return Source{}
}

// Get resolves every key under prefix: a whole key, a dotted prefix of
// one, or empty for all.
func (r *Resolver) Get(prefix string) []Value {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Value
	for _, k := range r.schema.keys {
		if prefix != "" && k.Name != prefix && !strings.HasPrefix(k.Name, prefix+".") {
			continue
		}
		v := Value{Key: k.Name, Apply: k.Apply}
		for i := range r.layers {
			if e, ok := r.layers[i][k.Name]; ok {
				v.Losers = append(v.Losers, Source{Layer: Layers[i], File: e.file, Value: e.value})
			}
		}
		v.Winner = v.Losers[len(v.Losers)-1]
		v.Losers = v.Losers[:len(v.Losers)-1]
		out = append(out, v)
	}
	return out
}

// String answers a string key's resolved value.
func (r *Resolver) String(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, _ := r.winnerLocked(name).Value.(string)
	return s
}

// Int answers an integer key's resolved value; 0 for a key that is not one.
func (r *Resolver) Int(name string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch v := r.winnerLocked(name).Value.(type) {
	case json.Number:
		n, _ := v.Int64()
		return n
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// Orphans are values set somewhere for a key nothing declares, as
// "<layer>:<file>:<key>".
func (r *Resolver) Orphans() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.orphans)
}

// Problems are layers or values that could not take part, each with why.
func (r *Resolver) Problems() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.problems)
}

// JSON renders a value as JSON text, the form it travels in.
func JSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(b.String())
}

// Document is the resolved configuration as a TOML file: one dotted key per
// line with its value as JSON, which is also valid TOML for every scalar, a
// comment naming the layer that won, then the orphans and problems. It is the
// snapshot and the export, so a saved export diffs against the live state.
func (r *Resolver) Document() []byte {
	var b strings.Builder
	b.WriteString("# rig's resolved configuration, written by rigd after every resolution.\n")
	b.WriteString("# schema " + SchemaID + "\n")
	for _, v := range r.Get("") {
		from := v.Winner.Layer
		if v.Winner.File != "" {
			from += " " + v.Winner.File
		}
		fmt.Fprintf(&b, "\n# %s from %s\n%s = %s\n", v.Key, from, v.Key, JSON(v.Winner.Value))
	}
	if o := r.Orphans(); len(o) > 0 {
		b.WriteString("\n# orphans, set but declared nowhere:\n")
		for _, s := range o {
			b.WriteString("#   " + s + "\n")
		}
	}
	if p := r.Problems(); len(p) > 0 {
		b.WriteString("\n# problems, values that took no part:\n")
		for _, s := range p {
			b.WriteString("#   " + s + "\n")
		}
	}
	return []byte(b.String())
}

// WriteSnapshot writes Document to path through a temporary file and a
// rename, so a reader never sees half of it.
func (r *Resolver) WriteSnapshot(path string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".resolved-*.toml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(r.Document()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// flatten turns nested tables into dotted keys: [log] level = x is log.level.
func flatten(prefix string, m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			maps.Copy(out, flatten(full, sub))
			continue
		}
		out[full] = v
	}
	return out
}
