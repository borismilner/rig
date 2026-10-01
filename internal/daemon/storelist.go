package daemon

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/store"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// serveStoreList is plan/48 decision 7: every database this estate keeps, with
// its file, size, schema version and whether it is ephemeral. It reads the
// list from what is open and what is on disk, never from a constant, so a
// namespace that stops being opened stops being listed.
func (d *Daemon) serveStoreList(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.StoreListRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "store.list: "+err.Error())
		return
	}
	// A program sees only its own store (plan/48 D2), so the estate's list,
	// which names every program's file, is not its to read.
	if c.scoped.Load() {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig.store.list: a program reaches only its own store; use store.collections",
			Precondition: "the caller is a terminal or an agent",
			Actual:       "a registered program",
			Fix:          "call rig.store.collections for this program's own store",
		})
		return
	}

	var out []*verbsv1.StoreNamespace
	if d.records != nil {
		out = append(out, namespace("record", d.records.Path(), record.SchemaVersion, d.records.Ephemeral()))
	}
	if d.leases != nil {
		out = append(out, namespace("coord", d.leases.Path(), coord.SchemaVersion, false))
	}
	if d.stores != nil && d.stores.dir != "" {
		paths, _ := filepath.Glob(filepath.Join(d.stores.dir, "*.db"))
		sort.Strings(paths)
		for _, p := range paths {
			v, ok := fileVersion(ctx, p)
			if !ok {
				continue
			}
			id := strings.TrimSuffix(filepath.Base(p), ".db")
			out = append(out, namespace("programs/"+id, p, v, d.stores.ephemeral))
		}
	}
	c.reply(f.GetStreamId(), &verbsv1.StoreListResponse{Namespaces: out})
}

func namespace(name, path string, version uint32, ephemeral bool) *verbsv1.StoreNamespace {
	ns := &verbsv1.StoreNamespace{Name: name, Path: path, SchemaVersion: uint64(version), Ephemeral: ephemeral}
	if fi, err := os.Stat(path); err == nil {
		ns.Bytes = uint64(max(fi.Size(), 0))
	}
	return ns
}

// fileVersion reads a program store's stamped version without migrating it.
// A file that is not a readable database is left out of the list rather than
// failing it.
func fileVersion(ctx context.Context, path string) (uint32, bool) {
	db, err := store.OpenReadOnly(path)
	if err != nil {
		return 0, false
	}
	defer func() { _ = db.Close() }()
	v, err := store.Version(ctx, db)
	return v, err == nil
}
