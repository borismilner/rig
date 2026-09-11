package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// What the rendering must never do, which is the only reason these tests
// exist. Section 9 says a failed call returns what failed, which precondition,
// what was actually found and what would fix it. The daemon fills whichever of
// those it can honestly fill, and the whole value of the field is that it is
// TRUE when set - so the failure mode on this side is not a missing line, it
// is a line that appears when the daemon said nothing.

func refuse(s *rigv1.Status, asJSON bool) *refusal {
	return &refusal{
		CallError: &client.CallError{Method: "fakeapp.reindex", Status: s},
		asJSON:    asJSON,
	}
}

// The four fields are optional by design and absence means nothing. A label
// with an empty value behind it claims rig looked and found nothing, where the
// truth is that the daemon never said.
func TestAnAbsentFieldGetsNoLineAtAll(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status *rigv1.Status
		want   []string
		absent []string
	}{
		{
			name: "nothing but the sentence",
			status: &rigv1.Status{
				Code:    rigv1.Code_CODE_INVALID,
				Message: "arguments do not match the declared schema",
			},
			absent: []string{"precondition", "actual", "fix", "fix command"},
		},
		{
			name: "one field only",
			status: &rigv1.Status{
				Code:       rigv1.Code_CODE_INVALID,
				Message:    "arguments do not match the declared schema",
				FixCommand: "rig fakeapp reindex --since 7d",
			},
			want:   []string{"fix command   rig fakeapp reindex --since 7d"},
			absent: []string{"precondition", "actual", "\nfix ", "  fix  "},
		},
		{
			name: "all four",
			status: &rigv1.Status{
				Code:         rigv1.Code_CODE_INVALID,
				Message:      "arguments do not match the declared schema",
				Precondition: `since matches ^[0-9]+[dhm]$`,
				Actual:       `since is "soon"`,
				Fix:          "give a duration: a number then d, h or m",
				FixCommand:   "rig fakeapp reindex --since 7d",
			},
			want: []string{
				`precondition  since matches ^[0-9]+[dhm]$`,
				`actual        since is "soon"`,
				"fix           give a duration: a number then d, h or m",
				"fix command   rig fakeapp reindex --since 7d",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := errorText(refuse(tc.status, false))

			// The sentence a person already got is still the first thing.
			if !strings.HasPrefix(got, "fakeapp.reindex: CODE_INVALID: ") {
				t.Errorf("the sentence is no longer first:\n%s", got)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("rendered %q the daemon never set, in:\n%s", a, got)
				}
			}
			// A field the daemon did not set must not cost a line either.
			set := 0
			for _, v := range []string{
				tc.status.GetPrecondition(), tc.status.GetActual(),
				tc.status.GetFix(), tc.status.GetFixCommand(),
			} {
				if v != "" {
					set++
				}
			}
			if lines := strings.Count(got, "\n"); lines != set {
				t.Errorf("%d continuation lines for %d set fields:\n%s", lines, set, got)
			}
		})
	}
}

// A daemon may send an ERROR frame with no Status at all - client.Call builds
// the CallError from f.GetStatus(), which is nil when the frame carried none.
// Proto getters make that safe from panicking; what it must also be is honest,
// in both modes.
func TestARefusalWithNoStatusAtAllStillRenders(t *testing.T) {
	naked := &refusal{CallError: &client.CallError{Method: "rig.ping"}}

	got := errorText(naked)
	if strings.Contains(got, "\n") {
		t.Errorf("a refusal with no status invented a field line:\n%s", got)
	}

	var out, errb bytes.Buffer
	naked.asJSON = true
	report(&out, &errb, naked)
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("--json emitted something unparseable for a nil status: %v\n%s",
			err, out.String())
	}
	if len(obj) != 1 || obj["code"] != "CODE_UNSPECIFIED" {
		t.Errorf("want exactly {code: CODE_UNSPECIFIED}, got %#v", obj)
	}
}

// A field the daemon set to whitespace is not a fact it measured, and a label
// with blanks after it makes the same claim an empty one would.
func TestAWhitespaceFieldIsTreatedAsAbsent(t *testing.T) {
	got := errorText(refuse(&rigv1.Status{
		Code:         rigv1.Code_CODE_INVALID,
		Message:      "refused",
		Precondition: "   ",
		Fix:          "\n",
		FixCommand:   "rig fakeapp reindex --since 7d",
	}, false))
	if strings.Contains(got, "precondition") {
		t.Errorf("a blank precondition rendered a label:\n%s", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("want one continuation line for the one real field:\n%s", got)
	}
}

// A value that arrives with a newline in it must stay under its own label. A
// second line at the left margin reads as a field whose label went missing.
func TestAMultiLineValueStaysUnderItsLabel(t *testing.T) {
	got := errorText(refuse(&rigv1.Status{
		Code:    rigv1.Code_CODE_INVALID,
		Message: "refused",
		Actual:  "at '/since': 'soon' does not match\nat '/depth': -1 is below the minimum",
	}, false))

	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("want the sentence and two value lines, got %d:\n%s", len(lines), got)
	}
	// The column is read off the rendered first line rather than taken from
	// fieldColumn. A test that computes its expectation from the constant it
	// is checking cannot see that constant drift away from the format string
	// beside it - which is exactly the mutation that survived the first pass.
	column := strings.Index(lines[1], "at '/since'")
	if column <= 0 {
		t.Fatalf("no value on the first field line:\n%q", lines[1])
	}
	if !strings.HasPrefix(lines[2], strings.Repeat(" ", column)+"at '/depth'") {
		t.Errorf("the wrapped line does not start in the value column %d:\n%q",
			column, lines[2])
	}
	// And it must not have acquired a label of its own.
	if strings.Contains(strings.TrimSpace(lines[2]), "  ") {
		t.Errorf("the wrapped line looks like a labelled field:\n%q", lines[2])
	}
}

// Section 10: --json returns exactly what the MCP tool returns. So the object
// is the Status, with no CLI envelope and nothing added to it - slice 3 hands
// this same shape to MCP, and a field only one of them carries is the
// parallel rendering that requirement exists to prevent.
func TestTheJSONObjectIsTheStatusAndNothingElse(t *testing.T) {
	var buf bytes.Buffer
	err := writeJSONStatus(&buf, &rigv1.Status{
		Code:         rigv1.Code_CODE_INVALID,
		Message:      "arguments do not match the declared schema",
		Precondition: `since matches ^[0-9]+[dhm]$`,
		Actual:       `since is "soon"`,
		Fix:          "give a duration: a number then d, h or m",
		FixCommand:   "rig fakeapp reindex --since 7d",
	})
	if err != nil {
		t.Fatalf("writeJSONStatus: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("--json emitted something that is not JSON: %v\n%s", err, buf.String())
	}

	want := map[string]any{
		"code":         "CODE_INVALID",
		"message":      "arguments do not match the declared schema",
		"precondition": `since matches ^[0-9]+[dhm]$`,
		"actual":       `since is "soon"`,
		"fix":          "give a duration: a number then d, h or m",
		"fix_command":  "rig fakeapp reindex --since 7d",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("--json object\n got %#v\nwant %#v", got, want)
	}

	// The proto field names are the contract, not lowerCamelCase.
	if strings.Contains(buf.String(), "fixCommand") {
		t.Error("--json renamed fix_command to fixCommand")
	}
}

// An absent field must not appear as an empty string either: a key with ""
// behind it is the same lie as a label with nothing after it, and an agent
// reading `"fix": ""` has been told rig looked.
func TestAnAbsentFieldIsNotAnEmptyStringInJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSONStatus(&buf, &rigv1.Status{
		Code:    rigv1.Code_CODE_DENIED,
		Message: "refused by a house rule",
	}); err != nil {
		t.Fatalf("writeJSONStatus: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	for _, k := range []string{"precondition", "actual", "fix", "fix_command"} {
		if _, present := got[k]; present {
			t.Errorf("%q is present for a Status that never set it: %s", k, buf.String())
		}
	}
	if got["code"] != "CODE_DENIED" {
		t.Errorf("code = %v, want CODE_DENIED", got["code"])
	}
}

// jsonStatus is hand-written, so the thing that rots is a field added to the
// wire on the daemon side and never rendered here. This is the test that
// notices, and it notices in this package rather than in a demo.
func TestTheJSONObjectCoversEveryFieldOfStatus(t *testing.T) {
	var onTheWire []string
	fields := (&rigv1.Status{}).ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		onTheWire = append(onTheWire, string(fields.Get(i).Name()))
	}

	var rendered []string
	st := reflect.TypeOf(jsonStatus{})
	for i := range st.NumField() {
		tag, _, _ := strings.Cut(st.Field(i).Tag.Get("json"), ",")
		rendered = append(rendered, tag)
	}

	sort.Strings(onTheWire)
	sort.Strings(rendered)
	if !reflect.DeepEqual(onTheWire, rendered) {
		t.Errorf("--json does not render the Status the wire carries\n"+
			" wire: %v\n json: %v\n"+
			"a field added to Status must be added to jsonStatus, or --json "+
			"quietly drops it", onTheWire, rendered)
	}
}

// The channel is part of the promise. --json puts the object on stdout
// because in that mode the object IS the answer; human mode keeps stderr
// because there a failure is not a result.
func TestTheJSONRefusalGoesToStdoutAndTheHumanOneToStderr(t *testing.T) {
	status := &rigv1.Status{
		Code:         rigv1.Code_CODE_INVALID,
		Message:      "arguments do not match the declared schema",
		Precondition: `since matches ^[0-9]+[dhm]$`,
		FixCommand:   "rig fakeapp reindex --since 7d",
	}

	var out, errb bytes.Buffer
	report(&out, &errb, refuse(status, true))
	if errb.Len() != 0 {
		t.Errorf("--json wrote prose to stderr: %q", errb.String())
	}
	if !json.Valid(bytes.TrimSpace(out.Bytes())) {
		t.Errorf("--json wrote something unparseable to stdout: %q", out.String())
	}

	out.Reset()
	errb.Reset()
	report(&out, &errb, refuse(status, false))
	if out.Len() != 0 {
		t.Errorf("human mode wrote to stdout: %q", out.String())
	}
	if !strings.HasPrefix(errb.String(), "rig: fakeapp.reindex: ") {
		t.Errorf("human mode lost its prefix: %q", errb.String())
	}
	if !strings.Contains(errb.String(), "fix command   rig fakeapp reindex --since 7d") {
		t.Errorf("human mode dropped fix_command: %q", errb.String())
	}
}

// A local failure has no Status behind it, so there is nothing structured to
// render. It keeps prose on stderr in BOTH modes, and that is the known gap
// named on jsonStatus rather than an accident - section 10 says "every error"
// and giving rig's own failures a wire code is a separate decision.
func TestALocalFailureIsNotDressedAsAStructuredRefusal(t *testing.T) {
	plain := os.ErrNotExist

	var dressed *refusal
	if errors.As(asRefusal(plain, true), &dressed) {
		t.Errorf("asRefusal dressed a local error as a refusal: %#v", dressed)
	}
	var out, errb bytes.Buffer
	report(&out, &errb, plain)
	if out.Len() != 0 {
		t.Errorf("a local failure wrote to stdout: %q", out.String())
	}
	if !strings.HasPrefix(errb.String(), "rig: ") {
		t.Errorf("a local failure lost its prefix: %q", errb.String())
	}
}

// Every surface rig has returns its errors through one helper, so a refusal
// renders the same from `rig ping` as from a declared command. The failure
// this catches is the next wire call being added beside the helper instead of
// through it, which would silently give that one surface prose under --json.
func TestEveryWireCallInThisPackageGoesThroughCall(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || name == "refusal.go" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if bytes.Contains(src, []byte(".Call(")) {
			offenders = append(offenders, name)
		}
	}
	if len(offenders) != 0 {
		t.Errorf("these reach the wire without going through call(): %v\n"+
			"call() is what attaches the caller's --json mode to a refusal, so "+
			"a direct c.Call here prints prose where section 10 promises an "+
			"object", offenders)
	}
}
