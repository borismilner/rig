package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Reading just enough of a declared JSON Schema to turn flags into JSON.
//
// This is NOT a validator and must never become one. rig validates against
// the declared schema at the boundary, with §22's pinned library, and a second
// implementation here would be a second contract that disagrees with the first
// on some edge nobody tests. What this reads is the property list and each
// property's type, which is the minimum needed to know whether `--dry-run`
// consumes the next argument and whether `--since 7d` is a string or a
// number. Everything else the boundary decides.

// argSchema is the declared object schema, as much of it as flags need.
type argSchema struct {
	Properties map[string]propSchema `json:"properties"`
}

type propSchema struct {
	// name is the DECLARED name, which is what goes into the JSON object.
	// The flag may have been typed with hyphens.
	name string

	// Type is a string in every schema rig has seen, but 2020-12 allows a
	// list, so it is decoded as either.
	Type any `json:"type"`
}

func parseArgSchema(raw []byte) (argSchema, error) {
	var s argSchema
	if strings.TrimSpace(string(raw)) == "" {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("the declared argument schema is not readable: %w", err)
	}
	for name, p := range s.Properties {
		p.name = name
		s.Properties[name] = p
	}
	return s, nil
}

// property finds a declared property by the name a person typed.
//
// A declared `dry_run` is reachable as `--dry-run`, because a flag with an
// underscore in it is not what anyone types, and the declaration is the
// program's business rather than the CLI's.
func (s argSchema) property(typed string) (propSchema, bool) {
	if p, ok := s.Properties[typed]; ok {
		return p, true
	}
	under := strings.ReplaceAll(typed, "-", "_")
	if p, ok := s.Properties[under]; ok {
		return p, true
	}
	return propSchema{}, false
}

// names lists the flags this command accepts, as they would be typed.
func (s argSchema) names() []string {
	out := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		out = append(out, "--"+strings.ReplaceAll(name, "_", "-"))
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"nothing"}
	}
	return out
}

// kind is the declared type, or empty when the declaration allows several.
//
// Several means rig cannot decide what `--x 1` should become, so parse says
// so rather than guessing - and `--args` is then the way to be exact.
func (p propSchema) kind() string {
	switch t := p.Type.(type) {
	case string:
		return t
	case []any:
		if len(t) == 1 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

// flagForm says whether this property can be given as a flag at all, and
// what its value looks like when it can.
//
// ONE predicate, used by both the parser and the generated help, because the
// two disagreeing is the defect this exists to prevent: help that offers
// `--nested <object>` for something the parser then refuses is help that
// wastes the reader's time and blames them for it.
func (p propSchema) flagForm() (placeholder string, supported bool) {
	switch k := p.kind(); k {
	case "boolean":
		// Takes no value: showing one would be a lie.
		return "", true
	case "string", "integer", "number":
		return "<" + k + ">", true
	default:
		// An object, an array, or several declared types. A flag syntax for
		// those would have to be invented per program, so --args is the
		// answer instead.
		return "", false
	}
}

// parse turns one flag's text into the JSON value its declared type calls for.
func (p propSchema) parse(value string) (any, error) {
	switch p.kind() {
	case "string":
		return value, nil
	case "boolean":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("--%s takes true or false, got %q", p.flag(), value)
		}
		return b, nil
	case "integer":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--%s takes a whole number, got %q", p.flag(), value)
		}
		return n, nil
	case "number":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("--%s takes a number, got %q", p.flag(), value)
		}
		return f, nil
	case "":
		return nil, fmt.Errorf("--%s is declared with more than one type, so "+
			"pass the whole object with --args instead", p.flag())
	default:
		// An object or an array as a flag would need a syntax rig has not
		// declared anywhere. Refusing beats inventing one per program.
		return nil, fmt.Errorf("--%s is declared as %s, which has no flag form: "+
			"pass the whole object with --args instead", p.flag(), p.kind())
	}
}

// flag is how this property is typed on a command line.
func (p propSchema) flag() string { return strings.ReplaceAll(p.name, "_", "-") }
