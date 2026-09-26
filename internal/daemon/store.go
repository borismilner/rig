package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/files"
	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/store"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 48's store verbs (P9 slice 3). internal/store is the whole engine;
// this file decides WHOSE store a call reaches, and that comes off the
// connection (plan/48 D2).

// storePageBudget is the largest encoded store answer this daemon writes. A
// frame is capped at 1 MiB (B116) and one document at store.MaxDocument, so
// a quarter of the frame is left for the envelope and a single largest
// document always fits.
const storePageBudget = 768 << 10

// stores opens each program's database the first time it is asked for, and
// keeps it open until the daemon closes.
type stores struct {
	mu        sync.Mutex
	dir       string // empty: this daemon was given no root
	ephemeral bool
	open      map[string]*store.Store

	// exports is the exports area, a git repository of its own (R36), and
	// snapshots is where an import keeps the store it replaced.
	exports   string
	snapshots string
	// exportMu makes one export or import at a time, so one export's
	// commit never carries another program's half-written files.
	exportMu sync.Mutex
	repo     *files.Repo // opened by the first export
}

// newStores places the databases under the estate's internal area. An empty
// root leaves the store verbs unavailable rather than guessing a place.
func newStores(root, estate string) (*stores, error) {
	s := &stores{open: make(map[string]*store.Store), ephemeral: estate == ""}
	if root == "" {
		return s, nil
	}
	var areas paths.Areas
	var err error
	if estate == "" {
		areas, err = paths.ScratchAreas(root)
	} else {
		areas, err = paths.EstateAreas(root, estate)
	}
	if err != nil {
		return nil, err
	}
	s.dir = filepath.Join(areas.Internal, "programs")
	s.exports = areas.Exports
	s.snapshots = filepath.Join(areas.Internal, "snapshots", "store")
	return s, nil
}

// errNoStore is a read of a program that has never written.
var errNoStore = errors.New("this program has no store yet: nothing has been written to it")

// get opens a program's store. create false answers errNoStore instead of
// creating one, so a terminal's typo on a read does not leave an empty
// database behind under the misspelled name.
func (s *stores) get(ctx context.Context, program string, create bool) (*store.Store, error) {
	if err := store.CheckName("program", program); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.open[program]; ok {
		return st, nil
	}
	if !create {
		if _, err := os.Stat(filepath.Join(s.dir, program+".db")); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("program %q: %w", program, errNoStore)
		}
	}
	st, err := store.Open(ctx, s.dir, program, s.ephemeral)
	if err != nil {
		return nil, err
	}
	s.open[program] = st
	return st, nil
}

func (s *stores) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for name, st := range s.open {
		errs = append(errs, st.Close())
		delete(s.open, name)
	}
	return errors.Join(errs...)
}

// storeProgram is whose store this call reaches.
//
// ⛔ A REGISTERED PROGRAM GETS ITS OWN STORE AND NO OTHER, whatever the
// request says: the name is the one it registered under, read off the
// connection. Naming another program is refused rather than ignored, so a
// program that meant to reach someone else's data learns it cannot.
//
// An unscoped caller - a terminal or an agent - is the owner and his seats,
// who may inspect any program's store, as with every other verb; they must
// name it, because they have no store of their own.
func storeProgram(c *conn, asked string) (string, *rigv1.Status) {
	if c.scoped.Load() {
		own := c.name()
		if asked != "" && asked != own {
			return "", &rigv1.Status{
				Code:         rigv1.Code_CODE_DENIED,
				Message:      fmt.Sprintf("a program reaches only its own store: this connection is %q and asked for %q", own, asked),
				Precondition: "a registered program names itself or leaves program empty",
				Actual:       fmt.Sprintf("program %q", asked),
				Fix:          "leave program empty",
			}
		}
		return own, nil
	}
	if asked == "" {
		return "", &rigv1.Status{
			Code:         rigv1.Code_CODE_INVALID,
			Message:      "name the program whose store to use: this connection is not a registered program, so it has no store of its own",
			Precondition: "program is set",
			Actual:       "program is empty",
			Fix:          "set program to the id of the program whose store you mean",
		}
	}
	return asked, nil
}

// serveStore dispatches the six store verbs.
func (d *Daemon) serveStore(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	req, ok := storeRequest(command)
	if !ok {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "no such method rig."+command)
		return
	}
	if err := proto.Unmarshal(f.GetPayload(), req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, command+": "+err.Error())
		return
	}
	st, program, ok := d.storeFor(ctx, c, f, command, req.GetProgram())
	if !ok {
		return
	}
	var err error
	// The engine's refusal goes out as it is: the client already prefixes
	// the method, and a second prefix here read "store.put: store.put:".
	var resp proto.Message
	switch r := req.(type) {
	case *verbsv1.StoreExportRequest:
		resp, err = d.stores.export(ctx, st, program, r.GetCollections())
	case *verbsv1.StoreImportRequest:
		resp, err = d.stores.importInto(ctx, st, program, r.GetCollections())
	default:
		resp, err = runStore(ctx, st, program, req)
	}
	if err != nil {
		c.failErr(f.GetStreamId(), storeCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), resp)
}

// storeRequester is what every store request has in common.
type storeRequester interface {
	proto.Message
	GetProgram() string
}

func storeRequest(command string) (storeRequester, bool) {
	switch command {
	case "store.put":
		return &verbsv1.StorePutRequest{}, true
	case "store.get":
		return &verbsv1.StoreGetRequest{}, true
	case "store.query":
		return &verbsv1.StoreQueryRequest{}, true
	case "store.delete":
		return &verbsv1.StoreDeleteRequest{}, true
	case "store.transact":
		return &verbsv1.StoreTransactRequest{}, true
	case "store.collections":
		return &verbsv1.StoreCollectionsRequest{}, true
	case "store.export":
		return &verbsv1.StoreExportRequest{}, true
	case "store.import":
		return &verbsv1.StoreImportRequest{}, true
	}
	return nil, false
}

// storeFor resolves the program and opens its store, answering the refusal
// itself when either fails.
func (d *Daemon) storeFor(ctx context.Context, c *conn, f *rigv1.Frame, command, asked string) (*store.Store, string, bool) {
	program, refusal := storeProgram(c, asked)
	if refusal != nil {
		refusal.Message = "rig." + command + ": " + refusal.GetMessage()
		c.failStatus(f.GetStreamId(), refusal)
		return nil, "", false
	}
	if d.stores == nil || d.stores.dir == "" {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig." + command + ": this daemon was given no storage root, so it keeps no program stores",
			Precondition: "rigd resolved a storage root at start",
			Actual:       "no root",
			Fix:          "start rigd with --root, or let it take its default",
		})
		return nil, "", false
	}
	// A program's own reads may create its store; a terminal's or an
	// agent's may not, since the name they typed is not proved to exist.
	create := c.scoped.Load() || storeWrites[command]
	st, err := d.stores.get(ctx, program, create)
	if err != nil {
		c.failErr(f.GetStreamId(), storeCode(err), err)
		return nil, "", false
	}
	return st, program, true
}

func runStore(ctx context.Context, st *store.Store, program string, req proto.Message) (proto.Message, error) {
	switch r := req.(type) {
	case *verbsv1.StorePutRequest:
		v, err := st.Put(ctx, r.GetCollection(), r.GetId(), r.GetExpectedVersion(), json.RawMessage(r.GetDocument()))
		return &verbsv1.StorePutResponse{Version: v}, err
	case *verbsv1.StoreDeleteRequest:
		return &verbsv1.StoreDeleteResponse{}, st.Delete(ctx, r.GetCollection(), r.GetId(), r.GetExpectedVersion())
	case *verbsv1.StoreTransactRequest:
		ops, err := opsFromWire(r.GetOps())
		if err != nil {
			return nil, err
		}
		vs, err := st.Transact(ctx, ops)
		return &verbsv1.StoreTransactResponse{Versions: vs}, err
	case *verbsv1.StoreGetRequest:
		return storeGet(ctx, st, r)
	case *verbsv1.StoreQueryRequest:
		return storeQuery(ctx, st, r)
	case *verbsv1.StoreCollectionsRequest:
		cs, err := st.Collections(ctx)
		resp := &verbsv1.StoreCollectionsResponse{Program: program}
		for _, ci := range cs {
			resp.Collections = append(resp.Collections, &verbsv1.StoreCollection{
				Name: ci.Name, Documents: uint64(max(ci.Count, 0)), Bytes: uint64(max(ci.Bytes, 0)),
			})
		}
		return resp, err
	}
	return nil, fmt.Errorf("no store handler for %T", req)
}

func opsFromWire(in []*verbsv1.StoreOp) ([]store.Op, error) {
	ops := make([]store.Op, 0, len(in))
	for i, o := range in {
		op := store.Op{Collection: o.GetCollection(), ID: o.GetId(), Expected: o.GetExpectedVersion()}
		switch {
		case o.GetDelete() && o.GetDocument() != "":
			return nil, &store.OpError{Index: i, Err: &store.InvalidError{
				What: "document", Value: "(set)", Rule: "empty on a delete",
			}}
		case !o.GetDelete():
			// A put's body is never nil, even when empty, so an empty
			// document is refused as not an object rather than read as a
			// delete the caller did not ask for.
			op.Body = json.RawMessage(o.GetDocument())
			if op.Body == nil {
				op.Body = json.RawMessage{}
			}
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func storeGet(ctx context.Context, st *store.Store, r *verbsv1.StoreGetRequest) (proto.Message, error) {
	docs, err := st.Get(ctx, r.GetCollection(), r.GetIds())
	if err != nil {
		return nil, err
	}
	// An answer over the frame cap is refused by conn.send, naming the size,
	// and never cut: a get leaves missing ids out, so a trimmed answer would
	// read as documents that do not exist.
	return &verbsv1.StoreGetResponse{Documents: docsToWire(docs)}, nil
}

func storeQuery(ctx context.Context, st *store.Store, r *verbsv1.StoreQueryRequest) (proto.Message, error) {
	q := store.Query{
		Collection: r.GetCollection(), Fields: r.GetFields(),
		Limit: int(r.GetLimit()), Offset: int(r.GetOffset()), CountOnly: r.GetCountOnly(),
	}
	for _, c := range r.GetWhere() {
		op, ok := storeOps[c.GetOp()]
		if !ok {
			return nil, &store.InvalidError{What: "op", Value: c.GetOp(), Rule: "one of eq, ne, lt, le, gt, ge"}
		}
		q.Where = append(q.Where, store.Cond{Field: c.GetField(), Op: op, Value: json.RawMessage(c.GetValue())})
	}
	for _, o := range r.GetOrder() {
		q.Order = append(q.Order, store.Order{Field: o.GetField(), Desc: o.GetDesc()})
	}
	res, err := st.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &verbsv1.StoreQueryResponse{Total: uint64(max(res.Total, 0)), More: res.More}
	// Filled document by document, stopping before the frame budget, and the
	// cut says more so the caller asks again from the next offset.
	size := proto.Size(resp)
	for _, d := range docsToWire(res.Docs) {
		n := proto.Size(d) + 8
		if size+n > storePageBudget {
			resp.More = true
			break
		}
		size += n
		resp.Documents = append(resp.Documents, d)
	}
	return resp, nil
}

// storeWrites may create a program's store. An export may not: exporting a
// program that never wrote is a typo, not an empty export.
var storeWrites = map[string]bool{
	"store.put": true, "store.delete": true, "store.transact": true, "store.import": true,
}

var storeOps = map[string]store.Operator{
	"eq": store.OpEq, "ne": store.OpNe, "lt": store.OpLt,
	"le": store.OpLe, "gt": store.OpGt, "ge": store.OpGe,
}

func docsToWire(docs []store.Doc) []*verbsv1.StoreDocument {
	out := make([]*verbsv1.StoreDocument, 0, len(docs))
	for _, d := range docs {
		out = append(out, &verbsv1.StoreDocument{
			Collection: d.Collection, Id: d.ID, Version: d.Version,
			UpdatedNs: d.Updated.UnixNano(), Document: string(d.Body),
		})
	}
	return out
}

// storeCode maps the engine's refusals onto the wire's codes.
func storeCode(err error) rigv1.Code {
	var conflict *store.ConflictError
	var invalid *store.InvalidError
	var future *store.FutureSchemaError
	var missing *store.NotFoundError
	switch {
	case errors.Is(err, errNoStore), errors.As(err, &missing):
		return rigv1.Code_CODE_NOT_FOUND
	case errors.As(err, &conflict):
		return rigv1.Code_CODE_CONFLICT
	case errors.As(err, &invalid):
		return rigv1.Code_CODE_INVALID
	case errors.As(err, &future):
		return rigv1.Code_CODE_UNAVAILABLE
	}
	return rigv1.Code_CODE_INTERNAL
}

// export writes a program's collections under exports/<program>/ and commits
// them (R19, R21, R35). The directory is joined from the program's checked
// name only.
func (s *stores) export(ctx context.Context, st *store.Store, program string, collections []string) (*verbsv1.StoreExportResponse, error) {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	if s.repo == nil {
		repo, err := files.Open(ctx, s.exports, files.NoSizeLimit())
		if err != nil {
			return nil, err
		}
		s.repo = repo
	}
	dir := filepath.Join(s.exports, program)
	res, err := st.Export(ctx, dir, collections)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.Commit(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	// Nothing an export writes is binary or skipped for size; a path left
	// out anyway is an export not kept, and the caller must hear it.
	if len(c.Skipped) > 0 {
		return nil, fmt.Errorf("the export was written but git left out %s, so it is not kept",
			strings.Join(c.Skipped, ", "))
	}
	resp := &verbsv1.StoreExportResponse{Program: program, Dir: dir, Removed: res.Removed, Commit: c.Commit}
	for _, e := range res.Collections {
		resp.Collections = append(resp.Collections, &verbsv1.StoreCollectionCount{
			Name: e.Collection, Documents: uint64(max(e.Documents, 0)), Bytes: uint64(max(e.Bytes, 0)),
		})
	}
	return resp, nil
}

// importInto replaces a program's collections from exports/<program>/ (R23,
// D8), after a snapshot under the internal area that undoes it.
func (s *stores) importInto(ctx context.Context, st *store.Store, program string, collections []string) (*verbsv1.StoreImportResponse, error) {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	// Nanoseconds and the program's checked name: two imports never pick
	// one path, and the store refuses a path that exists anyway.
	snap := filepath.Join(s.snapshots,
		program+"-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".db")
	res, err := st.Import(ctx, filepath.Join(s.exports, program), collections, snap)
	if err != nil {
		return nil, err
	}
	resp := &verbsv1.StoreImportResponse{Program: program, Snapshot: snap, Untouched: res.Untouched}
	for _, i := range res.Collections {
		resp.Collections = append(resp.Collections, &verbsv1.StoreCollectionCount{
			Name: i.Collection, Documents: uint64(max(i.Documents, 0)), Replaced: uint64(max(i.Replaced, 0)),
		})
	}
	return resp, nil
}
