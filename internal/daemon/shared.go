package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/kernel"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/53 slice 3: the shared table, served. internal/coord keeps the rows
// and the compare-and-swap; this file decides what the wire adds, which is
// the same two things lease.go decides - WHO wrote and WHAT witnesses an
// owner - plus the one thing AgentBox only did on a read: telling a waiter
// that a claim was abandoned.
//
//   - The writer and the owner are the caller's seat, never the request's.
//   - The witness is the caller's process from the socket. A terminal seat
//     has no roster row and its CLI exits at once, so its claims are
//     unwitnessed: the person at the terminal is never reported gone.
//   - Every change posts shared.<key>, a doorbell with the version and never
//     the value; waiting on shared.claims.* is one events_wait.

const (
	// ownerLeftWatch is how long after a seat leaves the roster rig looks for
	// its process to die, to post its claims as abandoned. A process that
	// outlives it is still read as gone once it dies, by whoever reads.
	ownerLeftWatch = 5 * time.Second
	ownerLeftPoll  = 100 * time.Millisecond
)

// sharedChange is shared.<key>'s payload.
type sharedChange struct {
	Key     string `json:"key"`
	Change  string `json:"change"` // set, deleted or owner_gone
	Version uint64 `json:"version"`
	Owner   string `json:"owner,omitempty"`
	By      string `json:"by,omitempty"`
}

func (d *Daemon) serveShared(c *conn, f *rigv1.Frame, command string) {
	if d.leases == nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig." + command + ": this estate keeps no shared table: an unnamed estate keeps no persistent state",
			Precondition: "rigd was started with --estate",
			Actual:       "this daemon serves an unnamed estate",
			Fix:          "start rigd with --estate <name>",
		})
		return
	}
	switch command {
	case "shared.get":
		d.serveSharedGet(c, f)
	case "shared.set":
		d.serveSharedSet(c, f)
	case "shared.delete":
		d.serveSharedDelete(c, f)
	}
}

// sharedKeyOK refuses a key that cannot also be an event kind's tail.
func sharedKeyOK(c *conn, f *rigv1.Frame, verb, key string) bool {
	why := ""
	switch {
	case key == "":
		why = `needs a key. One key per item is the idiom, claims.chunk-3 rather than one hot key, so a lost race costs nobody else a retry`
	case len("shared.")+len(key) > kernel.MaxEventKind:
		why = fmt.Sprintf("a key is at most %d bytes, so shared.<key> is an event kind", kernel.MaxEventKind-len("shared."))
	default:
		if bad := kernel.BadEventWords(key); bad != "" {
			why = fmt.Sprintf("key %q %s; a key is dotted lower-case words, like claims.chunk-3", key, bad)
		}
	}
	if why == "" {
		return true
	}
	c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig."+verb+": "+why)
	return false
}

// sharedWriter is the seat a write is made as, refusing a connection with none.
func (d *Daemon) sharedWriter(c *conn, f *rigv1.Frame, verb string) (string, bool) {
	_, seat, _, ok := d.provenance(c)
	if !ok {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig." + verb + ": a write needs a seat, so a claim names who made it and a stalled one can be judged",
			Precondition: "the caller announced into a named seat",
			Actual:       "this connection holds no seat",
			Fix:          "call announce with a seat first; reads need none",
		})
	}
	return seat, ok
}

func (d *Daemon) serveSharedGet(c *conn, f *rigv1.Frame) {
	var req verbsv1.SharedGetRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "shared.get: "+err.Error())
		return
	}
	key := req.GetKey()
	prefix, family := "", key == "*"
	if p, ok := strings.CutSuffix(key, ".*"); ok {
		if !sharedKeyOK(c, f, "shared.get", p) {
			return
		}
		prefix, family = p+".", true
	}
	if family {
		all, more, err := d.leases.SharedList(prefix)
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		}
		resp := &verbsv1.SharedGetResponse{Found: len(all) > 0, More: more}
		gone := 0
		for _, v := range all {
			w := d.sharedWire(v)
			if w.GetOwnerGone() {
				gone++
			}
			resp.Values = append(resp.Values, w)
		}
		switch {
		case more:
			resp.Note = fmt.Sprintf("capped at %d keys; there are more. Read a narrower family, or finish these first.", len(all))
		case len(all) == 0:
			resp.Note = "no keys in " + key + ": for a claim table, every item is still free."
		case gone > 0:
			resp.Note = fmt.Sprintf("%d of these %d is owned by a seat that is gone: its work was started and not finished. Take it over by setting the key at its version.", gone, len(all))
		}
		c.reply(f.GetStreamId(), resp)
		return
	}
	if !sharedKeyOK(c, f, "shared.get", key) {
		return
	}
	v, found, err := d.leases.SharedGet(key)
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	resp := &verbsv1.SharedGetResponse{Found: found, Value: d.sharedWire(v)}
	switch {
	case !found:
		resp.Note = key + " does not exist. Its version is 0, which is also the expected_version that claims it."
	case resp.GetValue().GetOwnerGone():
		resp.Note = fmt.Sprintf("%s is owned by %s, which is gone: nobody finished it. Take it over by setting it at version %d.", key, v.Owner, v.Version)
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) serveSharedSet(c *conn, f *rigv1.Frame) {
	var req verbsv1.SharedSetRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "shared.set: "+err.Error())
		return
	}
	key, value := req.GetKey(), req.GetValueJson()
	if !sharedKeyOK(c, f, "shared.set", key) {
		return
	}
	switch {
	case value == "":
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.shared.set: needs a value; to remove a key use shared.delete")
		return
	case len(value) > coord.MaxSharedValue:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, fmt.Sprintf(
			"rig.shared.set: the value is %d bytes, over %d. A shared value is coordination state - a claim, a counter, a pointer - so put the payload in a file and share its path",
			len(value), coord.MaxSharedValue))
		return
	case !json.Valid([]byte(value)):
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.shared.set: the value is not JSON")
		return
	}
	seat, ok := d.sharedWriter(c, f, "shared.set")
	if !ok {
		return
	}
	owner, w := "", coord.NoWitness()
	if req.GetOwn() {
		owner = seat
		if _, rostered := d.presence.seatNamed(seat); rostered {
			var err error
			if w, err = coord.WitnessProcess(c.principal().PID); err != nil {
				c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.shared.set: rig could not witness this caller's process, so it could never tell this claim was abandoned: "+err.Error())
				return
			}
		}
	}
	expected := req.GetExpectedVersion()
	v, applied, err := d.leases.SharedSet(key, json.RawMessage(value), expected, owner, w, seat)
	if errors.Is(err, coord.ErrSharedFull) {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_CONFLICT,
			Message:      "rig.shared.set: " + err.Error(),
			Precondition: fmt.Sprintf("fewer than %d keys, for a new key", coord.MaxSharedKeys),
			Actual:       fmt.Sprintf("%d keys", coord.MaxSharedKeys),
			Fix:          "delete the keys whose work is done; a full table refuses rather than evicting somebody's claim",
		})
		return
	}
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	resp := &verbsv1.SharedSetResponse{Applied: applied, Stale: !applied, Value: d.sharedWire(v)}
	if !applied {
		resp.Note = staleNote(key, expected, resp.GetValue())
		c.reply(f.GetStreamId(), resp)
		return
	}
	c.reply(f.GetStreamId(), resp)
	d.publishJSON("shared."+key, sharedChange{Key: key, Change: "set", Version: v.Version, Owner: v.Owner, By: seat})
}

func (d *Daemon) serveSharedDelete(c *conn, f *rigv1.Frame) {
	var req verbsv1.SharedDeleteRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "shared.delete: "+err.Error())
		return
	}
	key, expected := req.GetKey(), req.GetExpectedVersion()
	if !sharedKeyOK(c, f, "shared.delete", key) {
		return
	}
	if expected == 0 {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"rig.shared.delete: needs the expected_version you read. A delete at any version would erase a takeover somebody made since; shared.get answers the version")
		return
	}
	seat, ok := d.sharedWriter(c, f, "shared.delete")
	if !ok {
		return
	}
	v, applied, err := d.leases.SharedDelete(key, expected)
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	resp := &verbsv1.SharedDeleteResponse{Applied: applied, Stale: !applied, Value: d.sharedWire(v)}
	if !applied {
		if v.Version == 0 {
			resp.Note = key + " does not exist, so there was nothing to delete: somebody already removed it."
		} else {
			resp.Note = fmt.Sprintf("%s is at version %d, not %d: somebody wrote it since you read it. Read it before deciding; the work may now be theirs.", key, v.Version, expected)
		}
		c.reply(f.GetStreamId(), resp)
		return
	}
	c.reply(f.GetStreamId(), resp)
	d.publishJSON("shared."+key, sharedChange{Key: key, Change: "deleted", Version: v.Version, Owner: v.Owner, By: seat})
}

// staleNote says what stopped a write and what to do, in the request's terms.
func staleNote(key string, expected uint64, cur *verbsv1.SharedValue) string {
	switch {
	case expected == 0 && cur.GetOwnerGone():
		return fmt.Sprintf("%s is claimed by %s, which is gone: the work was started and abandoned. Take it over by setting it at version %d.", key, cur.GetOwner(), cur.GetVersion())
	case expected == 0 && cur.GetOwner() != "":
		return fmt.Sprintf("%s is already claimed by %s, which is still here. That is the normal end of a race, not an error: move on to the next key.", key, cur.GetOwner())
	case expected == 0:
		return fmt.Sprintf("%s already exists (version %d, written by %s), so it was not created.", key, cur.GetVersion(), cur.GetBy())
	case cur.GetVersion() == 0:
		return key + " does not exist, so it is at no version you could have read. Claim it with expected_version 0."
	default:
		return fmt.Sprintf("%s is at version %d, not %d: somebody changed it since you read it. Its value is here; decide from it and write again at %d.", key, cur.GetVersion(), expected, cur.GetVersion())
	}
}

// sharedWire renders a key with its owner's liveness evaluated now.
func (d *Daemon) sharedWire(v coord.Shared) *verbsv1.SharedValue {
	return &verbsv1.SharedValue{
		Key: v.Key, ValueJson: string(v.Value), Version: v.Version,
		Owner: v.Owner, OwnerGone: d.ownerGone(v), By: v.By, UpdatedUnixNano: v.Updated,
	}
}

// ownerGone is the two-step answer AgentBox gives, with §16's witness in
// place of a bare pid: a seat on the roster is live whatever its process
// looks like, and only off the roster is the witness asked. Unknown is not
// gone, so an unwitnessed claim is never reported abandoned.
func (d *Daemon) ownerGone(v coord.Shared) bool {
	if v.Owner == "" {
		return false
	}
	if _, live := d.presence.seatNamed(v.Owner); live {
		return false
	}
	return v.Witness.Observe(d.leases.BootID()) == coord.Dead
}

// ownerLeft posts owner_gone for each claim of seat whose process is seen to
// die within ownerLeftWatch of the seat leaving the roster.
func (d *Daemon) ownerLeft(seat string) {
	if d.leases == nil || seat == "" {
		return
	}
	owned, err := d.leases.SharedOwnedBy(seat)
	if err != nil || len(owned) == 0 {
		return
	}
	var done <-chan struct{}
	if ctx := d.serving.Load(); ctx != nil {
		done = (*ctx).Done()
	}
	go func() {
		deadline := time.Now().Add(ownerLeftWatch)
		tick := time.NewTicker(ownerLeftPoll)
		defer tick.Stop()
		for len(owned) > 0 {
			if _, back := d.presence.seatNamed(seat); back {
				return
			}
			left := owned[:0]
			for _, v := range owned {
				if !d.ownerGone(v) {
					left = append(left, v)
					continue
				}
				// Still this claim? A key rewritten since is somebody's news
				// already, posted by the write that changed it.
				if cur, found, err := d.leases.SharedGet(v.Key); err == nil && found && cur.Version == v.Version {
					d.publishJSON("shared."+v.Key, sharedChange{Key: v.Key, Change: "owner_gone", Version: v.Version, Owner: seat})
				}
			}
			owned = left
			if len(owned) == 0 || time.Now().After(deadline) {
				return
			}
			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	}()
}
