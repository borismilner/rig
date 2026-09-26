package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig files` - section 48's free files at the prompt.
//
//	rig files root --program P
//	rig files root --shared
//	rig files index <path> --title T --summary S [--tag t]...
//	rig files search <words...> [--under DIR] [--limit N]
//	rig files unindexed [--under DIR] [--limit N]
//
// root prints a directory and nothing else in text mode, so
// `cd "$(rig files root --shared)"` works. A path for the index is relative
// to the shared root.

type filesFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	program *string
	shared  *bool
	title   *string
	summary *string
	tags    tagList
	under   *string
	limit   *uint
}

func filesFlagSet() *filesFlags {
	f := &filesFlags{fs: flag.NewFlagSet("files", flag.ContinueOnError)}
	f.asJSON = f.fs.Bool("json", false, "emit JSON")
	f.timeout = f.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	f.program = f.fs.String("program", "", "root: whose directory, a program id")
	f.shared = f.fs.Bool("shared", false, "root: the area every program shares")
	f.title = f.fs.String("title", "", "index: the file's title")
	f.summary = f.fs.String("summary", "", "index: one line, what a search shows")
	f.fs.Var(&f.tags, "tag", "index: a one-word tag (repeatable)")
	f.under = f.fs.String("under", "", "search, unindexed: only this directory under the root")
	f.limit = f.fs.Uint("limit", 0, "search: at most this many hits (default 5, at most 20); "+
		"unindexed: at most this many files (default 100, at most 1000)")
	return f
}

const stateKey = "state"

const filesUsage = "usage: rig files root --program P | --shared | " +
	"index <path> --title T --summary S [--tag t]... | " +
	"search <words...> [--under DIR] [--limit N] | unindexed [--under DIR] [--limit N]"

func cmdFiles(args []string) (err error) {
	f := filesFlagSet()
	flags, positional := partition(args)
	if err := f.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *f.asJSON) }()
	if len(positional) == 0 {
		return badArgumentf("%s", filesUsage)
	}
	verb, req, resp, err := f.request(positional[0], positional[1:])
	if err != nil {
		return err
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *f.timeout)
	defer cancel()
	if err := call(ctx, c, "rig.files."+verb, req, resp); err != nil {
		return err
	}
	if *f.asJSON {
		return json.NewEncoder(os.Stdout).Encode(filesJSON(resp))
	}
	printFiles(os.Stdout, resp)
	return nil
}

func (f *filesFlags) request(verb string, rest []string) (string, proto.Message, proto.Message, error) {
	if *f.limit > 0xffffffff {
		return "", nil, nil, badArgumentf("--limit must fit 32 bits")
	}
	limit := uint32(*f.limit)
	switch {
	case verb == "root" && len(rest) == 0:
		if (*f.program == "") == !*f.shared {
			return "", nil, nil, badArgumentf("rig files root needs exactly one of --program P and --shared")
		}
		return verb, &verbsv1.FilesRootRequest{Program: *f.program, Shared: *f.shared},
			&verbsv1.FilesRootResponse{}, nil
	case verb == "index" && len(rest) == 1:
		return verb, &verbsv1.FilesIndexRequest{
			Path: rest[0], Title: *f.title, Summary: *f.summary, Tags: f.tags,
		}, &verbsv1.FilesIndexResponse{}, nil
	case verb == "search" && len(rest) > 0:
		return verb, &verbsv1.FilesSearchRequest{
			Query: strings.Join(rest, " "), Under: *f.under, Limit: limit,
		}, &verbsv1.FilesSearchResponse{}, nil
	case verb == "unindexed" && len(rest) == 0:
		return verb, &verbsv1.FilesUnindexedRequest{Under: *f.under, Limit: limit},
			&verbsv1.FilesUnindexedResponse{}, nil
	}
	return "", nil, nil, badArgumentf("%s", filesUsage)
}

func printFiles(w io.Writer, resp proto.Message) {
	switch r := resp.(type) {
	case *verbsv1.FilesRootResponse:
		_, _ = fmt.Fprintln(w, r.GetPath())
	case *verbsv1.FilesIndexResponse:
		switch {
		case r.GetRemoved():
			_, _ = fmt.Fprintf(w, "%s is gone; its entry was dropped\n", r.GetPath())
		case r.GetBinary():
			_, _ = fmt.Fprintf(w, "indexed %s by its title, summary and tags (binary)\n", r.GetPath())
		case r.GetTruncated():
			_, _ = fmt.Fprintf(w, "indexed %s, the first %d bytes of its text\n", r.GetPath(), r.GetTextBytes())
		default:
			_, _ = fmt.Fprintf(w, "indexed %s, %d bytes of text\n", r.GetPath(), r.GetTextBytes())
		}
	case *verbsv1.FilesSearchResponse:
		if len(r.GetHits()) == 0 {
			_, _ = fmt.Fprintln(w, "no match")
		}
		for _, h := range r.GetHits() {
			_, _ = fmt.Fprintf(w, "%s  %s\n  %s\n  %s\n", h.GetPath(), h.GetTitle(), h.GetSummary(), h.GetSnippet())
		}
	case *verbsv1.FilesUnindexedResponse:
		if r.GetTotal() == 0 {
			_, _ = fmt.Fprintln(w, "every file is indexed")
			return
		}
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, p := range r.GetFiles() {
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", p.GetState(), p.GetPath())
		}
		_ = tw.Flush()
		if n := uint32(len(r.GetFiles())); n < r.GetTotal() { //nolint:gosec // at most MaxPending
			_, _ = fmt.Fprintf(w, "%d of %d\n", n, r.GetTotal())
		}
	}
}

func filesJSON(resp proto.Message) any {
	switch r := resp.(type) {
	case *verbsv1.FilesRootResponse:
		return map[string]any{
			"path": r.GetPath(), "program": r.GetProgram(), "commit_every_s": r.GetCommitEveryS(),
		}
	case *verbsv1.FilesIndexResponse:
		return map[string]any{
			"path": r.GetPath(), "removed": r.GetRemoved(), "text_bytes": r.GetTextBytes(),
			"binary": r.GetBinary(), "truncated": r.GetTruncated(),
		}
	case *verbsv1.FilesSearchResponse:
		hits := make([]map[string]any, 0, len(r.GetHits()))
		for _, h := range r.GetHits() {
			hits = append(hits, map[string]any{
				"path": h.GetPath(), titleKey: h.GetTitle(), "summary": h.GetSummary(),
				"snippet": h.GetSnippet(), "score": h.GetScore(),
			})
		}
		return map[string]any{"hits": hits}
	case *verbsv1.FilesUnindexedResponse:
		fs := make([]map[string]any, 0, len(r.GetFiles()))
		for _, p := range r.GetFiles() {
			fs = append(fs, map[string]any{"path": p.GetPath(), stateKey: p.GetState()})
		}
		return map[string]any{"files": fs, "total": r.GetTotal()}
	}
	return nil
}
