package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

// Backend supplies directory notifications. A factory must return a fresh,
// empty backend each time; Configure prepares replacements before swapping.
type Backend interface {
	Add(string) error
	Remove(string) error
	WatchList() []string
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

type nativeBackend struct{ *fsnotify.Watcher }

func (b nativeBackend) Events() <-chan fsnotify.Event { return b.Watcher.Events }
func (b nativeBackend) Errors() <-chan error          { return b.Watcher.Errors }

func newBackend() (Backend, error) {
	b, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return nativeBackend{b}, nil
}

// Event carries one change or a quiet batch, not a list of every changed path.
// A fired ID stays the same when publication retries. Changed IDs append the
// batch's event count. Rescan operations are synthetic.
type Event struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	WatchID   string `json:"watch_id"`
	Path      string `json:"path,omitempty"`
	Operation string `json:"operation,omitempty"`
	FirstNS   int64  `json:"first_ns"`
	LastNS    int64  `json:"last_ns"`
	DueNS     int64  `json:"due_ns"`
	Count     uint64 `json:"count"`
	Gap       bool   `json:"gap"`
}

// Gap identifies an interval in which notifications may have been missed.
// ReportGap connects this to the daemon's existing coverage log.
type Gap struct {
	Reason string
	At     time.Time
	Err    error
}

// Options connects the estate's bus and coverage log. Both callbacks are
// required, must honor ctx, and must not call back into Watcher: operations
// and callbacks are serialized. A publish succeeds only when the bus accepts
// it. Now and NewBackend are injection points for deterministic tests.
type Options struct {
	Publish    func(context.Context, Event) error
	ReportGap  func(context.Context, Gap) error
	NewBackend func() (Backend, error)
	Now        func() time.Time
}

// Watcher owns subscriptions, not State. Close stops it; the caller waits for
// Run to return before closing State. One Watcher may use a State at a time,
// under the estate's existing single-instance lock.
type Watcher struct {
	mu           sync.Mutex
	state        *State
	backend      Backend
	kernelLimit  int
	extraWatches int // old backend still live during replacement preparation
	options      Options
	config       Config
	groups       map[string]*groupState
	wake         chan struct{}
	done         chan struct{}
	closed       bool
	running      bool
	closeErr     error
}

// New restores durable configuration and pending work, subscribes, and diffs
// a complete startup scan. It does not start a goroutine. Call Configure with
// resolved settings, then Run. An unchanged restart retains the old deadline.
func New(ctx context.Context, state *State, options Options) (*Watcher, error) {
	if state == nil || options.Publish == nil || options.ReportGap == nil {
		return nil, errors.New("watch: state, Publish and ReportGap are required")
	}
	if options.NewBackend == nil {
		options.NewBackend = newBackend
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	c, groups, err := state.load(ctx)
	if err != nil {
		return nil, fmt.Errorf("watch: load: %w", err)
	}
	c, err = normalize(c)
	if err != nil {
		return nil, err
	}
	if err = validateState(c, groups); err != nil {
		return nil, err
	}
	b, err := options.NewBackend()
	if err != nil {
		return nil, err
	}
	w := &Watcher{state: state, backend: b, kernelLimit: kernelWatchLimit(), options: options,
		config: c, groups: groups, wake: make(chan struct{}, 1), done: make(chan struct{})}
	if len(groups) > 0 {
		err = w.gap(ctx, "restart", nil)
	}
	if err == nil {
		err = w.rescan(ctx, nil, false)
	}
	if err != nil {
		_ = b.Close()
		return nil, err
	}
	return w, nil
}

func validateState(c Config, groups map[string]*groupState) error {
	if len(groups) != len(c.Groups) {
		return errors.New("watch: stored config and group count disagree")
	}
	entries := 0
	for _, g := range c.Groups {
		s := groups[g.ID]
		if s == nil || !reflect.DeepEqual(s.Group, g) {
			return fmt.Errorf("watch: stored config and group %q disagree", g.ID)
		}
		if p := s.Pending; p != nil && (p.Count == 0 || p.DueNS < p.LastNS || p.LastNS < p.FirstNS) {
			return fmt.Errorf("watch: invalid pending state for %s", g.ID)
		}
		if d := s.Delivery; d != nil && (d.ID == "" || d.WatchID != g.ID || d.Kind != "rig.watch."+g.ID+".fired" || d.Count == 0) {
			return fmt.Errorf("watch: invalid pending delivery for %s", g.ID)
		}
		for path := range s.Entries {
			if !filepath.IsAbs(path) || filepath.Clean(path) != path || !matches(g, path) {
				return fmt.Errorf("watch: invalid snapshot path for %s: %q", g.ID, path)
			}
		}
		entries += len(s.Entries)
	}
	if entries > c.MaxEntries {
		return errors.New("watch: stored snapshot exceeds max_entries")
	}
	return nil
}

// Configure atomically replaces the declared groups and persisted settings.
// Removing a group explicitly discards its pending batch. Failed validation,
// registration, scan or storage leaves the old configuration active. A failed
// callback after commit is returned, but the new configuration remains saved.
// Replacement temporarily holds both backends; their combined subscriptions
// must fit the ceiling. Removing every group needs no spare subscriptions.
func (w *Watcher) Configure(ctx context.Context, c Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.configure(ctx, c)
}

func (w *Watcher) configure(ctx context.Context, c Config) error {
	if w.closed {
		return errors.New("watch: closed")
	}
	next, err := normalize(c)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(next, w.config) {
		return nil
	}
	b, err := w.options.NewBackend()
	if err != nil {
		return err
	}
	probe := &Watcher{state: w.state, backend: b, kernelLimit: w.kernelLimit, extraWatches: len(w.backend.WatchList())}
	entries, _, err := probe.scan(ctx, next)
	if err != nil {
		_ = b.Close()
		return err
	}
	updates, events := w.prepare(next, entries, nil, false)
	removed := []string{}
	for id := range w.groups {
		if _, ok := entries[id]; !ok {
			removed = append(removed, id)
		}
	}
	if err = w.state.save(ctx, &next, updates, removed); err != nil {
		_ = b.Close()
		return fmt.Errorf("watch: save configuration: %w", err)
	}
	old := w.backend
	hadGroups := len(w.groups) > 0
	w.backend = b
	w.config = next
	w.groups = map[string]*groupState{}
	for _, u := range updates {
		w.groups[u.state.Group.ID] = u.state
	}
	w.Wake()
	err = old.Close()
	if hadGroups {
		err = errors.Join(err, w.gap(ctx, "reconfigure", nil))
	}
	return errors.Join(err, w.publish(ctx, events))
}

// Declare replaces one named group, preserving every other declaration.
func (w *Watcher) Declare(ctx context.Context, g Group) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.config
	c.Groups = slices.Clone(c.Groups)
	found := false
	for i := range c.Groups {
		if c.Groups[i].ID == g.ID {
			c.Groups[i] = g
			found = true
			break
		}
	}
	if !found {
		c.Groups = append(c.Groups, g)
	}
	return w.configure(ctx, c)
}

// Config returns a copy, suitable for removing a declaration with Configure.
func (w *Watcher) Config() Config {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, _ := normalize(w.config)
	return c
}

func (w *Watcher) Usage() Usage {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := Usage{Watches: len(w.backend.WatchList()), Ceiling: w.config.MaxWatches, KernelMaxWatches: w.kernelLimit}
	for _, g := range w.groups {
		u.Entries += len(g.Entries)
	}
	return u
}

// Wake prompts a deadline check, notably from the daemon's system.resumed
// event. A pending deadline is also rechecked at least once per minute.
func (w *Watcher) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.done)
		w.closeErr = w.backend.Close()
	}
	return w.closeErr
}

// Run serves one lifecycle and closes its subscriptions on exit. Storage,
// publication and backend failures return loudly, with pending work intact.
// The owner restarts with New after fixing a failure. ctx cancellation is a
// normal shutdown and returns nil.
func (w *Watcher) Run(ctx context.Context) (err error) {
	w.mu.Lock()
	if w.running || w.closed {
		w.mu.Unlock()
		return errors.New("watch: Run requires an open, unused watcher")
	}
	w.running = true
	w.mu.Unlock()
	defer func() { err = errors.Join(err, w.Close()) }()
	for {
		w.mu.Lock()
		b := w.backend
		delay := w.nextDelay()
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return nil
		}
		var timer *time.Timer
		var tick <-chan time.Time
		if delay >= 0 {
			timer = time.NewTimer(delay)
			tick = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return nil
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return nil
		case <-w.wake:
		case e, ok := <-b.Events():
			err = w.consume(ctx, b, e, ok)
		case e, ok := <-b.Errors():
			err = w.backendError(ctx, b, e, ok)
		case <-tick:
			// Prefer an already queued change over an expired quiet period.
			select {
			case e, ok := <-b.Events():
				err = w.consume(ctx, b, e, ok)
			default:
				w.mu.Lock()
				err = w.fireDue(ctx)
				w.mu.Unlock()
			}
		}
		if timer != nil {
			timer.Stop()
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			w.mu.Lock()
			gapErr := w.gap(ctx, "stopped", err)
			w.mu.Unlock()
			return errors.Join(err, gapErr)
		}
	}
}

func (w *Watcher) nextDelay() time.Duration {
	delay := time.Duration(-1)
	now := w.options.Now()
	for _, g := range w.groups {
		if g.Delivery != nil {
			return 0
		}
		if g.Pending != nil {
			d := time.Unix(0, g.Pending.DueNS).Sub(now)
			if d < 0 {
				d = 0
			}
			if d > time.Minute {
				d = time.Minute
			}
			if delay < 0 || d < delay {
				delay = d
			}
		}
	}
	return delay
}

func (w *Watcher) consume(ctx context.Context, b Backend, e fsnotify.Event, ok bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || b != w.backend {
		return nil
	}
	if !ok {
		return errors.New("watch: backend event channel closed")
	}
	if w.ignored(e.Name) {
		return nil
	}
	if e.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		if e.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
			for _, p := range b.WatchList() {
				if p == e.Name || strings.HasPrefix(p, e.Name+string(filepath.Separator)) {
					if err := b.Remove(p); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
						return err
					}
				}
			}
		}
		return w.rescan(ctx, &e, false)
	}
	return w.change(ctx, e)
}

func (w *Watcher) backendError(ctx context.Context, b Backend, err error, ok bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || b != w.backend {
		return nil
	}
	if !ok {
		return errors.New("watch: backend error channel closed")
	}
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		if e := w.gap(ctx, "overflow", err); e != nil {
			return e
		}
		return w.rescan(ctx, nil, true)
	}
	if err == nil {
		return errors.New("watch: backend returned an empty error")
	}
	return fmt.Errorf("watch: backend: %w", err)
}

// Rescan reconciles subscriptions and differences after an external coverage
// gap. Overflow additionally invalidates every group, even without a diff.
func (w *Watcher) Rescan(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("watch: closed")
	}
	if err := w.gap(ctx, "rescan", nil); err != nil {
		return err
	}
	err := w.rescan(ctx, nil, false)
	w.Wake()
	return err
}

func (w *Watcher) rescan(ctx context.Context, raw *fsnotify.Event, overflow bool) error {
	entries, wanted, err := w.scan(ctx, w.config)
	if err != nil {
		return err
	}
	updates, events := w.prepare(w.config, entries, raw, overflow)
	if err = w.state.save(ctx, nil, updates, nil); err != nil {
		return fmt.Errorf("watch: save rescan: %w", err)
	}
	for _, u := range updates {
		w.groups[u.state.Group.ID] = u.state
	}
	return errors.Join(w.prune(wanted), w.publish(ctx, events))
}

func copyGroup(g *groupState) *groupState {
	n := *g
	if g.Pending != nil {
		p := *g.Pending
		n.Pending = &p
	}
	return &n
}

func (w *Watcher) prepare(c Config, entries map[string]map[string]stamp, raw *fsnotify.Event, overflow bool) ([]update, []Event) {
	var updates []update
	var events []Event
	for _, g := range c.Groups {
		old := w.groups[g.ID]
		n := &groupState{Group: g, Entries: entries[g.ID]}
		if old != nil {
			n = copyGroup(old)
			n.Group = g
			n.Entries = entries[g.ID]
		}
		var before map[string]stamp
		if old != nil {
			before = old.Entries
		}
		deltas := diffEntries(before, n.Entries)
		if n.Pending != nil {
			if n.Pending.ID == "" {
				n.Pending.ID = uuid.NewString()
			}
			n.Pending.DueNS = n.Pending.LastNS + g.DebounceMS*int64(time.Millisecond)
		}
		if old != nil {
			for _, d := range deltas {
				if raw == nil || d.path != raw.Name {
					events = append(events, w.mark(n, d.path, "rescan", overflow))
				}
			}
			if raw != nil && matches(g, raw.Name) {
				events = append(events, w.mark(n, raw.Name, raw.Op.String(), false))
			}
			if overflow {
				events = append(events, w.mark(n, "", "overflow", true))
			}
		}
		updates = append(updates, update{state: n, deltas: deltas})
	}
	return updates, events
}

func (w *Watcher) mark(g *groupState, path, op string, gap bool) Event {
	now := w.options.Now().UnixNano()
	if g.Pending == nil {
		g.Sequence++
		g.Pending = &pending{ID: uuid.NewString(), FirstNS: now}
	}
	p := g.Pending
	// A wall-clock correction must not make the persisted interval invalid.
	// The deadline still follows the latest observed event's clock reading.
	if now < p.FirstNS {
		p.FirstNS = now
	}
	p.LastNS = now
	p.DueNS = now + g.Group.DebounceMS*int64(time.Millisecond)
	p.Count++
	p.Gap = p.Gap || gap
	e := batchEvent(g, "changed")
	e.ID = fmt.Sprintf("%s:%d", p.ID, p.Count)
	e.Path = path
	e.Operation = op
	return e
}

func batchEvent(g *groupState, kind string) Event {
	p := g.Pending
	return Event{Kind: "rig.watch." + g.Group.ID + "." + kind, ID: p.ID, WatchID: g.Group.ID,
		FirstNS: p.FirstNS, LastNS: p.LastNS, DueNS: p.DueNS, Count: p.Count, Gap: p.Gap}
}

func (w *Watcher) change(ctx context.Context, e fsnotify.Event) error {
	var value *stamp
	info, err := os.Lstat(e.Name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		s, err := fileStamp(e.Name, info)
		if err != nil {
			return err
		}
		value = &s
	}
	var updates []update
	var events []Event
	for _, g := range w.config.Groups {
		old := w.groups[g.ID]
		if matches(g, e.Name) {
			n := copyGroup(old)
			events = append(events, w.mark(n, e.Name, e.Op.String(), false))
			updates = append(updates, update{state: n, deltas: []delta{{path: e.Name, value: value}}})
		}
	}
	if len(updates) == 0 {
		return nil
	}
	count := 0
	for _, g := range w.groups {
		count += len(g.Entries)
	}
	for _, u := range updates {
		_, exists := u.state.Entries[e.Name]
		if value != nil && !exists {
			count++
		}
		if value == nil && exists {
			count--
		}
	}
	if count > w.config.MaxEntries {
		return fmt.Errorf("watch: snapshot exceeds max_entries %d", w.config.MaxEntries)
	}
	if err = w.state.save(ctx, nil, updates, nil); err != nil {
		return fmt.Errorf("watch: save change: %w", err)
	}
	for _, u := range updates {
		if value == nil {
			delete(u.state.Entries, e.Name)
		} else {
			u.state.Entries[e.Name] = *value
		}
		w.groups[u.state.Group.ID] = u.state
	}
	return w.publish(ctx, events)
}

func (w *Watcher) fireDue(ctx context.Context) error {
	for _, g := range w.config.Groups {
		// Deliver an earlier attempt before sealing the next quiet batch.
		// Sealing is durable: later changes cannot mutate an already offered
		// event while keeping its deduplication ID.
		for {
			old := w.groups[g.ID]
			if old.Delivery == nil {
				if old.Pending == nil || old.Pending.DueNS > w.options.Now().UnixNano() {
					break
				}
				n := copyGroup(old)
				event := batchEvent(old, "fired")
				n.Delivery = &event
				n.Pending = nil
				if err := w.state.save(ctx, nil, []update{{state: n}}, nil); err != nil {
					return fmt.Errorf("watch: seal %s fired: %w", g.ID, err)
				}
				w.groups[g.ID] = n
				old = n
			}
			if err := w.options.Publish(ctx, *old.Delivery); err != nil {
				return fmt.Errorf("watch: publish %s fired: %w", g.ID, err)
			}
			n := copyGroup(old)
			n.Delivery = nil
			if err := w.state.save(ctx, nil, []update{{state: n}}, nil); err != nil {
				return fmt.Errorf("watch: acknowledge %s fired: %w", g.ID, err)
			}
			w.groups[g.ID] = n
		}
	}
	return nil
}

func (w *Watcher) publish(ctx context.Context, events []Event) error {
	for _, e := range events {
		if err := w.options.Publish(ctx, e); err != nil {
			return fmt.Errorf("watch: publish %s: %w", e.Kind, err)
		}
	}
	return nil
}

func (w *Watcher) gap(ctx context.Context, reason string, err error) error {
	return w.options.ReportGap(ctx, Gap{Reason: reason, At: w.options.Now(), Err: err})
}
