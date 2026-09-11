// Command schemagen emits JSON Schema for what a program declares at connect.
//
// The declaration is section 5e's, and the audience is whoever is writing one:
// a program in a language with no Go types to import, and `frontend`'s
// `gen:types`, which turns these files into TypeScript. `make schema` writes
// them and `make types` consumes them.
//
// THE SCHEMA IS BUILT FROM THE PROTO DESCRIPTORS, NOT FROM A TRANSCRIPTION.
// Every field name, JSON name, kind and enum value is read out of
// proto/rig/v1 at run time, so a field added to the wire appears here without
// anyone remembering to add it. The one thing descriptors cannot carry is
// which properties are mandatory - proto3 has no required - so that set is
// written down in mandatory.go beside the kernel rule it mirrors, and a test
// fails if the two ever disagree.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	out := flag.String("out", "schema/", "directory to write the schema files into")
	flag.Parse()

	if err := run(*out); err != nil {
		fmt.Fprintf(os.Stderr, "schemagen: %v\n", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	if dir == "" {
		return errors.New("-out is empty: say which directory to write into")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	for _, f := range Files() {
		path := filepath.Join(dir, f.Name)
		body, err := f.JSON()
		if err != nil {
			return fmt.Errorf("building %s: %w", f.Name, err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Printf("%s\n", path)
	}
	return nil
}
