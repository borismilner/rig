package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig files` - section 48's free files at the prompt.
//
//	rig files root --program P
//	rig files root --shared
//
// It prints a directory and nothing else in text mode, so
// `cd "$(rig files root --shared)"` works.

type filesFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	program *string
	shared  *bool
}

func filesFlagSet() *filesFlags {
	f := &filesFlags{fs: flag.NewFlagSet("files", flag.ContinueOnError)}
	f.asJSON = f.fs.Bool("json", false, "emit JSON")
	f.timeout = f.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	f.program = f.fs.String("program", "", "whose directory: a program id")
	f.shared = f.fs.Bool("shared", false, "the area every program shares")
	return f
}

func cmdFiles(args []string) (err error) {
	f := filesFlagSet()
	flags, positional := partition(args)
	if err := f.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *f.asJSON) }()
	if len(positional) != 1 || positional[0] != "root" {
		return badArgumentf("usage: rig files root --program P | --shared")
	}
	if (*f.program == "") == !*f.shared {
		return badArgumentf("rig files root needs exactly one of --program P and --shared")
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *f.timeout)
	defer cancel()
	var resp verbsv1.FilesRootResponse
	if err := call(ctx, c, "rig.files.root", &verbsv1.FilesRootRequest{
		Program: *f.program, Shared: *f.shared,
	}, &resp); err != nil {
		return err
	}
	if *f.asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"path": resp.GetPath(), "program": resp.GetProgram(),
			"commit_every_s": resp.GetCommitEveryS(),
		})
	}
	fmt.Println(resp.GetPath())
	return nil
}
