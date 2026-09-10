package analysis

import (
	"regexp"
	"strings"
)

// NoEnumZero is section 21's rule as a gate: "Meaningful enum zero is banned.
// Every proto enum reserves *_UNSPECIFIED = 0."
//
// The reason is in wire.proto's own comment: an unset field and a field set to
// the first value are indistinguishable on the wire, so a meaningful zero
// makes "the sender did not say" decode as a decision. Refusing zero at the
// boundary is only possible if zero means nothing.
//
// This reads the .proto files rather than the generated Go, because the proto
// is the source and the generator is faithful. It follows that an enum written
// straight into Go - a const block over iota - is not covered; nothing in rig
// has one, and the wire contract is the thing this rule protects.
var NoEnumZero = Analyzer{
	Name: "noenumzero",
	Doc:  "no meaningful enum zero (PLAN.md section 21)",
	Proto: func(p *Pass, f *ProtoFile) {
		for _, e := range protoEnums(f.Text) {
			zero, line, found := e.zeroValue()
			switch {
			case !found:
				p.ReportAt(f.Path, e.line,
					"enum %s has no zero value: proto3 gives every enum one whether it "+
						"is written or not, so an unwritten zero is an undocumented one",
					e.name)
			case !strings.HasSuffix(zero, "_UNSPECIFIED"):
				p.ReportAt(f.Path, line,
					"enum %s has the meaningful zero %s: an unset field and the first "+
						"value are the same bytes, so zero has to mean \"not said\" and "+
						"be refused at the boundary. Rename it to %s_UNSPECIFIED and "+
						"renumber",
					e.name, zero, enumPrefix(e.name, zero))
			}
		}
	},
	Whole: func(p *Pass) {
		if len(p.Protos) == 0 {
			p.Notef("no .proto files under the given roots, so nothing was checked")
		}
	},
}

type protoEnum struct {
	name   string
	line   int
	values []protoValue
}

type protoValue struct {
	name   string
	number string
	line   int
}

func (e protoEnum) zeroValue() (string, int, bool) {
	for _, v := range e.values {
		if v.number == "0" {
			return v.name, v.line, true
		}
	}
	return "", 0, false
}

// enumPrefix guesses the SCREAMING_SNAKE prefix the renamed zero should carry,
// from a sibling value if there is one and from the enum's name otherwise.
func enumPrefix(enum, zero string) string {
	if i := strings.LastIndex(zero, "_"); i > 0 {
		return zero[:i]
	}
	var out []rune
	for i, r := range enum {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, '_')
		}
		out = append(out, r)
	}
	return strings.ToUpper(string(out))
}

var (
	enumOpen  = regexp.MustCompile(`^\s*enum\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	enumValue = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(-?\d+)\s*(\[[^\]]*\])?\s*;`)
)

// protoEnums finds every enum in a .proto file.
//
// A regexp reader rather than a parser: the file is ours, the shape is fixed
// by the style the repository already uses, and a dependency on a protobuf
// parser to check one naming rule is not a trade worth making. Comments are
// stripped first so a commented-out value cannot be read as a real one.
func protoEnums(text string) []protoEnum {
	lines := strings.Split(stripProtoComments(text), "\n")
	var out []protoEnum
	var cur *protoEnum
	for i, line := range lines {
		rest := line
		if cur == nil {
			m := enumOpen.FindStringSubmatchIndex(rest)
			if m == nil {
				continue
			}
			cur = &protoEnum{name: rest[m[2]:m[3]], line: i + 1}
			// Whatever follows the brace is still this line's: an enum
			// written on one line has its values and its close there too.
			rest = rest[m[1]:]
		}
		// Values before the closing brace, because a value and the brace
		// share a line often enough to matter.
		for _, v := range enumValues(rest) {
			cur.values = append(cur.values, protoValue{
				name: v[0], number: v[1], line: i + 1,
			})
		}
		if strings.Contains(rest, "}") {
			out = append(out, *cur)
			cur = nil
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// enumValues pulls every NAME = N; out of one line's worth of text.
func enumValues(s string) [][2]string {
	var out [][2]string
	for _, m := range enumValue.FindAllStringSubmatch(s, -1) {
		out = append(out, [2]string{m[1], m[2]})
	}
	return out
}

// stripProtoComments blanks comments while keeping every line and every
// column, so reported positions still point at the source.
func stripProtoComments(text string) string {
	out := []byte(text)
	inBlock, inLine, inString := false, false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if c == '\n' {
			inLine = false
			inString = false
			continue
		}
		switch {
		case inBlock:
			if c == '*' && i+1 < len(out) && out[i+1] == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				inBlock = false
				continue
			}
			out[i] = ' '
		case inLine:
			out[i] = ' '
		case inString:
			if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '/' && i+1 < len(out) && out[i+1] == '/':
			inLine = true
			out[i] = ' '
		case c == '/' && i+1 < len(out) && out[i+1] == '*':
			inBlock = true
			out[i], out[i+1] = ' ', ' '
			i++
		}
	}
	return string(out)
}
