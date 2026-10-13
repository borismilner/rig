package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/borismilner/rig/internal/observe"
	"github.com/borismilner/rig/internal/watch"
)

func (d *Daemon) resolvedWatch() (watch.Config, error) {
	values := d.settings.Get("watch")
	flat := map[string]any{}
	for _, v := range values {
		name := strings.TrimPrefix(v.Key, "watch.")
		if name == "groups" {
			name = "watches"
		}
		flat[name] = v.Winner.Value
	}
	raw, err := json.Marshal(flat)
	if err != nil {
		return watch.Config{}, err
	}
	return watch.ReadConfig(strings.NewReader(string(raw)))
}

// New holds the estate lock and opens the watch before either socket serves.
func (d *Daemon) openWatch() error {
	c, err := d.resolvedWatch()
	if err != nil {
		return fmt.Errorf("daemon: watch settings: %w", err)
	}
	if len(c.Groups) == 0 {
		// Nothing to watch, so no watch.db: most daemons never configure a
		// group, and an idle open database is one more thing to close.
		return nil
	}
	if d.estate == "" {
		return errors.New("daemon: durable watches require a named estate; use an isolated development estate for testing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d.watchState, err = watch.OpenState(ctx, d.estate)
	if err != nil {
		return fmt.Errorf("daemon: opening watch state: %w", err)
	}
	d.watcher, err = watch.New(ctx, d.watchState, watch.Options{
		Publish: d.publishWatch, ReportGap: d.watchGap, InitialConfig: &c,
	})
	if err != nil {
		return fmt.Errorf("daemon: opening watches: %w", err)
	}
	u := d.watcher.Usage()
	d.log.Info("filesystem watches ready", "groups", len(c.Groups), "watches", u.Watches,
		"ceiling", u.Ceiling, "kernel_max_watches", u.KernelMaxWatches, "entries", u.Entries)
	return nil
}

func (d *Daemon) publishWatch(ctx context.Context, ev watch.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, _, err = d.events.publishTo(ev.Kind, eventSourceRig, string(raw), "")
	return err
}

func (d *Daemon) watchGap(ctx context.Context, gap watch.Gap) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.log.Warn("filesystem watch coverage gap", "reason", gap.Reason, "err", gap.Err)
	if d.logs != nil {
		d.logs.NoteGap(observe.Gap{From: gap.At.UnixNano(), To: gap.At.UnixNano(), Client: "rigd", Cause: "watch: " + gap.Reason})
	}
	return nil
}

func (d *Daemon) startWatch(ctx context.Context, cancel context.CancelFunc) func() error {
	if d.watcher == nil {
		return func() error { return nil }
	}
	done := make(chan error, 1)
	go func() {
		err := d.watcher.Run(ctx)
		if ctx.Err() != nil {
			err = nil
		} else if err != nil {
			d.log.Error("filesystem watch failed; stopping daemon", "err", err)
			cancel()
		}
		done <- err
	}()
	return func() error { return <-done }
}
