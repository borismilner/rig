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

// `rig store` - section 48's program stores at the prompt.
//
//	rig store list
//	rig store collections --program P
//	rig store get <collection> <id>... --program P
//	rig store put <collection> <id> <json|-> [--if-version N] --program P
//	rig store delete <collection> <id> --if-version N --program P
//	rig store query <collection> [--where 'field op value']... [--fields a,b]
//	                [--order field|-field]... [--limit N] [--offset N] [--count]
//	rig store export [collection]... --program P
//	rig store import [collection]... --program P
//
// --program IS REQUIRED because a terminal is not a registered program and has
// no store of its own (plan/48 D2); the daemon refuses its absence too.

type storeFlags struct {
	fs        *flag.FlagSet
	asJSON    *bool
	timeout   *time.Duration
	program   *string
	ifVersion *uint64
	where     *listFlag
	fields    *string
	order     *listFlag
	limit     *uint
	offset    *uint
	count     *bool
}

// listFlag collects a flag given more than once, in order.
type listFlag struct{ v []string }

func (l *listFlag) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(l.v, "; ")
}

func (l *listFlag) Set(s string) error { l.v = append(l.v, s); return nil }

func storeFlagSet() *storeFlags {
	s := &storeFlags{fs: flag.NewFlagSet("store", flag.ContinueOnError), where: &listFlag{}, order: &listFlag{}}
	s.asJSON = s.fs.Bool("json", false, "emit JSON")
	s.timeout = s.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	s.program = s.fs.String("program", "", "whose store: a program id")
	s.ifVersion = s.fs.Uint64("if-version", 0, "put, delete: the version you read (put: 0 creates)")
	s.fs.Var(s.where, "where", "query: 'field op value', op one of eq ne lt le gt ge; repeatable, joined by AND")
	s.fields = s.fs.String("fields", "", "query: answer only these fields, comma-separated")
	s.fs.Var(s.order, "order", "query: sort by field, -field for descending; repeatable")
	s.limit = s.fs.Uint("limit", 0, "query: at most this many documents")
	s.offset = s.fs.Uint("offset", 0, "query: skip this many, to read the next page")
	s.count = s.fs.Bool("count", false, "query: answer the count alone")
	return s
}

// The subcommands, named once.
const (
	storeGet    = "get"
	storePut    = "put"
	storeDelete = "delete"
	storeExport = "export"
	storeImport = "import"
	keyVersion  = "version"
	keyProgram  = "program"
	storeColls  = "collections"
)

const storeUsage = "usage: rig store list | collections | get <collection> <id>... | " +
	"put <collection> <id> <json|-> [--if-version N] | delete <collection> <id> --if-version N | " +
	"query <collection> [--where 'field op value']... [--fields a,b] [--order f|-f]... " +
	"[--limit N] [--offset N] [--count] | export [collection]... | import [collection]...; " +
	"every form takes --program P"

func cmdStore(args []string) (err error) {
	s := storeFlagSet()
	flags, positional := partition(args)
	if err := s.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *s.asJSON) }()
	if len(positional) == 1 && positional[0] == "list" {
		return storeList(*s.timeout, *s.asJSON)
	}
	if len(positional) == 0 || !storeArity(positional[0], len(positional)-1) {
		return badArgumentf("%s", storeUsage)
	}
	if *s.program == "" {
		return badArgumentf("rig store needs --program: a terminal has no store of its own, " +
			"so name the program whose store you mean")
	}
	sub, rest := positional[0], positional[1:]
	req, err := s.request(sub, rest)
	if err != nil {
		return err
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *s.timeout)
	defer cancel()

	resp := storeResponses[sub]()
	if err := call(ctx, c, "rig.store."+sub, req, resp); err != nil {
		return err
	}
	if *s.asJSON {
		return json.NewEncoder(os.Stdout).Encode(storeJSON(resp))
	}
	printStore(os.Stdout, resp, uint64(*s.offset))
	return nil
}

func storeArity(sub string, n int) bool {
	switch sub {
	case storeColls:
		return n == 0
	case storeGet:
		return n >= 2
	case storePut:
		return n == 3
	case storeDelete:
		return n == 2
	case "query":
		return n == 1
	case storeExport, storeImport:
		return true
	}
	return false
}

var storeResponses = map[string]func() proto.Message{
	storeColls:  func() proto.Message { return &verbsv1.StoreCollectionsResponse{} },
	storeGet:    func() proto.Message { return &verbsv1.StoreGetResponse{} },
	storePut:    func() proto.Message { return &verbsv1.StorePutResponse{} },
	storeDelete: func() proto.Message { return &verbsv1.StoreDeleteResponse{} },
	"query":     func() proto.Message { return &verbsv1.StoreQueryResponse{} },
	storeExport: func() proto.Message { return &verbsv1.StoreExportResponse{} },
	storeImport: func() proto.Message { return &verbsv1.StoreImportResponse{} },
}

func (s *storeFlags) request(sub string, rest []string) (proto.Message, error) {
	p := *s.program
	switch sub {
	case storeColls:
		return &verbsv1.StoreCollectionsRequest{Program: p}, nil
	case storeExport:
		return &verbsv1.StoreExportRequest{Program: p, Collections: rest}, nil
	case storeImport:
		return &verbsv1.StoreImportRequest{Program: p, Collections: rest}, nil
	case storeGet:
		return &verbsv1.StoreGetRequest{Program: p, Collection: rest[0], Ids: rest[1:]}, nil
	case storePut:
		doc, err := documentArg(rest[2])
		if err != nil {
			return nil, err
		}
		return &verbsv1.StorePutRequest{
			Program: p, Collection: rest[0], Id: rest[1],
			ExpectedVersion: *s.ifVersion, Document: doc,
		}, nil
	case storeDelete:
		if *s.ifVersion == 0 {
			return nil, badArgumentf("rig store delete needs --if-version: a delete names " +
				"the version you read, so it cannot remove a document somebody changed since")
		}
		return &verbsv1.StoreDeleteRequest{
			Program: p, Collection: rest[0], Id: rest[1], ExpectedVersion: *s.ifVersion,
		}, nil
	}
	return s.query(rest[0])
}

func (s *storeFlags) query(collection string) (proto.Message, error) {
	q := &verbsv1.StoreQueryRequest{
		Program: *s.program, Collection: collection, CountOnly: *s.count,
	}
	if *s.limit > 0xffffffff || *s.offset > 0xffffffff {
		return nil, badArgumentf("--limit and --offset must fit 32 bits")
	}
	q.Limit, q.Offset = uint32(*s.limit), uint32(*s.offset)
	for _, w := range s.where.v {
		cond, err := parseWhere(w)
		if err != nil {
			return nil, err
		}
		q.Where = append(q.Where, cond)
	}
	if *s.fields != "" {
		q.Fields = strings.Split(*s.fields, ",")
	}
	for _, o := range s.order.v {
		desc := strings.HasPrefix(o, "-")
		q.Order = append(q.Order, &verbsv1.StoreOrder{Field: strings.TrimPrefix(o, "-"), Desc: desc})
	}
	return q, nil
}

// parseWhere reads 'field op value'. The value is JSON when it parses as a
// JSON scalar (3, true, null, "3"), and otherwise the text itself as a string,
// so `--where 'state eq done'` needs no inner quotes.
func parseWhere(w string) (*verbsv1.StoreCondition, error) {
	parts := strings.SplitN(strings.TrimSpace(w), " ", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return nil, badArgumentf("--where %q is not 'field op value'", w)
	}
	value := strings.TrimSpace(parts[2])
	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		quoted, _ := json.Marshal(value)
		value = string(quoted)
	}
	switch v.(type) {
	case map[string]any, []any:
		return nil, badArgumentf("--where %q compares with an object or a list; a value is a scalar", w)
	}
	return &verbsv1.StoreCondition{Field: parts[0], Op: parts[1], Value: value}, nil
}

// documentArg is the document itself, or standard input for "-".
func documentArg(a string) (string, error) {
	if a != "-" {
		return a, nil
	}
	// One byte over the store's ceiling, so an oversized document is refused
	// by the store with its own words rather than truncated here.
	b, err := io.ReadAll(io.LimitReader(stdinSource(), 256<<10+1))
	if err != nil {
		return "", badArgumentf("reading the document from standard input: %v", err)
	}
	return string(b), nil
}

func printStore(w io.Writer, resp proto.Message, offset uint64) {
	switch r := resp.(type) {
	case *verbsv1.StorePutResponse:
		_, _ = fmt.Fprintf(w, "written at version %d\n", r.GetVersion())
	case *verbsv1.StoreDeleteResponse:
		_, _ = fmt.Fprintln(w, "deleted")
	case *verbsv1.StoreGetResponse:
		printDocs(w, r.GetDocuments())
	case *verbsv1.StoreQueryResponse:
		printDocs(w, r.GetDocuments())
		n := uint64(len(r.GetDocuments()))
		switch {
		case r.GetMore():
			_, _ = fmt.Fprintf(w, "%d of %d. Next page: --offset %d\n", n, r.GetTotal(), offset+n)
		case n == 0:
			_, _ = fmt.Fprintf(w, "%d matching\n", r.GetTotal())
		}
	case *verbsv1.StoreCollectionsResponse:
		if len(r.GetCollections()) == 0 {
			_, _ = fmt.Fprintf(w, "%s has no collections yet\n", r.GetProgram())
			return
		}
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "COLLECTION\tDOCUMENTS\tBYTES")
		for _, c := range r.GetCollections() {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\n", c.GetName(), c.GetDocuments(), c.GetBytes())
		}
		_ = tw.Flush()
	case *verbsv1.StoreExportResponse:
		printCounts(w, "exported", r.GetCollections(), false)
		for _, c := range r.GetRemoved() {
			_, _ = fmt.Fprintf(w, "removed  %s (no longer in the store)\n", c)
		}
		commit := "unchanged since the last export, nothing committed"
		if r.GetCommit() != "" {
			commit = "committed " + r.GetCommit()
		}
		_, _ = fmt.Fprintf(w, "%s\n%s\n", r.GetDir(), commit)
	case *verbsv1.StoreImportResponse:
		printCounts(w, "imported", r.GetCollections(), true)
		if len(r.GetUntouched()) > 0 {
			_, _ = fmt.Fprintf(w, "left alone: %s\n", strings.Join(r.GetUntouched(), ", "))
		}
		_, _ = fmt.Fprintf(w, "the store before the import: %s\n", r.GetSnapshot())
	}
}

func printCounts(w io.Writer, verb string, cs []*verbsv1.StoreCollectionCount, before bool) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	last := "BYTES"
	if before {
		last = "BEFORE"
	}
	_, _ = fmt.Fprintf(tw, "%s\tDOCUMENTS\t%s\n", strings.ToUpper(verb), last)
	for _, c := range cs {
		n := c.GetBytes()
		if before {
			n = c.GetReplaced()
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\n", c.GetName(), c.GetDocuments(), n)
	}
	_ = tw.Flush()
}

func printDocs(w io.Writer, docs []*verbsv1.StoreDocument) {
	for _, d := range docs {
		_, _ = fmt.Fprintf(w, "%s  v%d  %s\n", d.GetId(), d.GetVersion(), d.GetDocument())
	}
}

// storeJSON renders an answer with each document as JSON rather than as a
// string holding JSON, and versions as numbers.
func storeJSON(resp proto.Message) map[string]any {
	docs := func(in []*verbsv1.StoreDocument) []map[string]any {
		out := make([]map[string]any, 0, len(in))
		for _, d := range in {
			out = append(out, map[string]any{
				"collection": d.GetCollection(), "id": d.GetId(), keyVersion: d.GetVersion(),
				"updated":  time.Unix(0, d.GetUpdatedNs()).UTC().Format(time.RFC3339Nano),
				"document": json.RawMessage(d.GetDocument()),
			})
		}
		return out
	}
	switch r := resp.(type) {
	case *verbsv1.StorePutResponse:
		return map[string]any{keyVersion: r.GetVersion()}
	case *verbsv1.StoreGetResponse:
		return map[string]any{"documents": docs(r.GetDocuments())}
	case *verbsv1.StoreQueryResponse:
		return map[string]any{"documents": docs(r.GetDocuments()), "total": r.GetTotal(), "more": r.GetMore()}
	case *verbsv1.StoreCollectionsResponse:
		cs := make([]map[string]any, 0, len(r.GetCollections()))
		for _, c := range r.GetCollections() {
			cs = append(cs, map[string]any{"name": c.GetName(), "documents": c.GetDocuments(), "bytes": c.GetBytes()})
		}
		return map[string]any{keyProgram: r.GetProgram(), storeColls: cs}
	case *verbsv1.StoreListResponse:
		// ephemeral is written whether true or false: a script must never
		// read an absent key as durable.
		ns := make([]map[string]any, 0, len(r.GetNamespaces()))
		for _, n := range r.GetNamespaces() {
			ns = append(ns, map[string]any{
				"namespace": n.GetName(), "path": n.GetPath(), "bytes": n.GetBytes(),
				"schema_version": n.GetSchemaVersion(), "ephemeral": n.GetEphemeral(),
			})
		}
		return map[string]any{"namespaces": ns}
	case *verbsv1.StoreExportResponse:
		return map[string]any{
			keyProgram: r.GetProgram(), "dir": r.GetDir(), storeColls: countsJSON(r.GetCollections()),
			"removed": nonNil(r.GetRemoved()), "commit": r.GetCommit(),
		}
	case *verbsv1.StoreImportResponse:
		return map[string]any{
			keyProgram: r.GetProgram(), storeColls: countsJSON(r.GetCollections()),
			"untouched": nonNil(r.GetUntouched()), "snapshot": r.GetSnapshot(),
		}
	}
	return map[string]any{"deleted": true}
}

func countsJSON(in []*verbsv1.StoreCollectionCount) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, c := range in {
		out = append(out, map[string]any{
			"name": c.GetName(), "documents": c.GetDocuments(),
			"bytes": c.GetBytes(), "replaced": c.GetReplaced(),
		})
	}
	return out
}

// nonNil answers [] rather than null for an empty list, so a script reading
// the JSON never has to tell the two apart.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// storeList is `rig store list`: every database this estate keeps (plan/48
// decision 7). It takes no --program, since it names them all.
func storeList(timeout time.Duration, asJSON bool) error {
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resp := &verbsv1.StoreListResponse{}
	if err := call(ctx, c, "rig.store.list", &verbsv1.StoreListRequest{}, resp); err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(storeJSON(resp))
	}
	printStoreList(os.Stdout, resp)
	return nil
}

func printStoreList(w io.Writer, resp *verbsv1.StoreListResponse) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAMESPACE\tSCHEMA\tBYTES\tKEPT\tPATH")
	for _, ns := range resp.GetNamespaces() {
		kept := "durable"
		if ns.GetEphemeral() {
			kept = "ephemeral"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n",
			ns.GetName(), ns.GetSchemaVersion(), ns.GetBytes(), kept, ns.GetPath())
	}
	_ = tw.Flush()
}
