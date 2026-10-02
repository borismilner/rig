package observe

import (
	"encoding/json/jsontext"
	"errors"
	"strings"
	"testing"
)

func TestRedactionBlanksOnlyTheDeclaredValues(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pointers []string
		in, want string
	}{
		{"a member", []string{"/token"}, `{"user":"b","token":"s3cr3t"}`, `{"user":"b","token":"[redacted]"}`},
		{"a nested object whole", []string{"/auth"}, `{"auth":{"k":"s3cr3t","n":[1]},"x":1}`, `{"auth":"[redacted]","x":1}`},
		{"inside an object", []string{"/auth/k"}, `{"auth":{"k":"s3cr3t","n":[1]}}`, `{"auth":{"k":"[redacted]","n":[1]}}`},
		{"an array element", []string{"/keys/1"}, `{"keys":["a","s3cr3t","c"]}`, `{"keys":["a","[redacted]","c"]}`},
		{"inside an element", []string{"/l/0/t"}, `{"l":[{"t":"s3cr3t"},{"t":"kept"}]}`, `{"l":[{"t":"[redacted]"},{"t":"kept"}]}`},
		{"an escaped name", []string{"/a~1b"}, `{"a/b":"s3cr3t","a":{"b":"kept"}}`, `{"a/b":"[redacted]","a":{"b":"kept"}}`},
		{"spacing is kept", []string{"/token"}, `{ "token" : "s3cr3t" , "n" : 1 }`, `{ "token" : "[redacted]" , "n" : 1 }`},
		{"two of them", []string{"/a", "/c"}, `{"a":1,"b":2,"c":3}`, `{"a":"[redacted]","b":2,"c":"[redacted]"}`},
		{"a pointer that is absent", []string{"/token"}, `{"user":"b"}`, `{"user":"b"}`},
		{"a scalar payload", []string{"/token"}, `"s"`, `"s"`},
		{"the whole payload", []string{""}, `{"token":"s3cr3t"}`, `"[redacted]"`},
		{"nothing declared", []string{}, `{"token":"kept"}`, `{"token":"kept"}`},
		{"a name equal to a value", []string{"/token"}, `{"x":"token","token":"s3cr3t"}`, `{"x":"token","token":"[redacted]"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Compile(tc.pointers)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Apply([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("Apply(%s) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// A payload with no pointer space is option 2: never its contents.
func TestAPayloadThatIsNotJSONIsRefusedNotRecorded(t *testing.T) {
	for _, pointers := range [][]string{{}, {"/token"}, {""}} {
		r, _ := Compile(pointers)
		for _, in := range []string{`\x08\x01token`, `{"token":"s3cr3t"`, `{"a":1} {"token":"s3cr3t"}`} {
			if got, err := r.Apply([]byte(in)); !errors.Is(err, ErrNoPointerSpace) {
				t.Fatalf("%v over %q answered %q, %v", pointers, in, got, err)
			}
		}
	}
	if _, err := Compile([]string{"token"}); err == nil {
		t.Fatal("a pointer without its leading slash compiled")
	}
}

// Whatever the payload, a value at a declared pointer never survives, and
// what comes out is still one JSON value.
func FuzzRedactionNeverKeepsADeclaredValue(f *testing.F) {
	f.Add(`{"token":"s3cr3t","l":[{"token":"x"}]}`)
	f.Add(`{"a":{"token":[1,2,{"q":"s3cr3t"}]}}`)
	f.Add(`[{"token":1}]`)
	r, _ := Compile([]string{"/token", "/a/token", "/l/0/token"})
	f.Fuzz(func(t *testing.T, in string) {
		out, err := r.Apply([]byte(in))
		if err != nil || strings.TrimSpace(in) == "" {
			return
		}
		if !jsontext.Value(out).IsValid() {
			t.Fatalf("Apply(%q) = %q, not JSON", in, out)
		}
		d := jsontext.NewDecoder(strings.NewReader(string(out)))
		for {
			tok, err := d.ReadToken()
			if err != nil {
				return
			}
			p := string(d.StackPointer())
			if r.exact[p] && tok.Kind() != '"' && tok.Kind() != '}' && tok.Kind() != ']' {
				t.Fatalf("Apply(%q) = %q kept %s at %s", in, out, tok, p)
			}
			if r.exact[p] && tok.Kind() == '"' && tok.String() != "[redacted]" && !isMemberName(d) {
				t.Fatalf("Apply(%q) = %q kept %s at %s", in, out, tok, p)
			}
		}
	})
}

// isMemberName says the token just read was an object member's name.
func isMemberName(d *jsontext.Decoder) bool {
	kind, n := d.StackIndex(d.StackDepth())
	return kind == '{' && n%2 == 1
}

// Decision 8's budget: under 100 ns per field.
func BenchmarkRedaction(b *testing.B) {
	payload := []byte(`{"user":"boris","project":"rig","token":"s3cr3t-0123456789","opts":{"deep":true,"n":42},"tags":["a","b","c"]}`)
	const fields = 9 // user project token opts deep n tags, and two more tags
	r, _ := Compile([]string{"/token"})
	b.ReportAllocs()
	for b.Loop() {
		if _, err := r.Apply(payload); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/fields, "ns/field")
}
