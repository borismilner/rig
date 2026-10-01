package daemon

import (
	"errors"
	"log/slog"

	"github.com/borismilner/rig/internal/config"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 6's settings over the wire, as plan/47 builds them. Both verbs are
// on the MCP door: plan/09's rule that nothing is kept off it outranks
// plan/47's "set waits for C1" (decision 0256's correction).

// serveConfig answers config.get and config.set.
func (d *Daemon) serveConfig(c *conn, f *rigv1.Frame, command string) {
	id := f.GetStreamId()
	switch command {
	case "config.get":
		var req registryv1.ConfigGetRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		values := d.settings.Get(req.GetPrefix())
		if len(values) == 0 && req.GetPrefix() != "" {
			c.failStatus(id, &rigv1.Status{
				Code:    rigv1.Code_CODE_NOT_FOUND,
				Message: "rig.config.get: no key is " + req.GetPrefix() + " or under it",
				Fix:     "rig config get, with no key, lists every key",
			})
			return
		}
		resp := &registryv1.ConfigGetResponse{
			Orphans: d.settings.Orphans(), Problems: d.settings.Problems(), SnapshotPath: d.snapshotPath,
		}
		for _, v := range values {
			out := &registryv1.ConfigValue{Key: v.Key, Apply: v.Apply, Winner: layerOut(v.Winner)}
			for _, l := range v.Losers {
				out.Losers = append(out.Losers, layerOut(l))
			}
			resp.Values = append(resp.Values, out)
		}
		c.reply(id, resp)
	case "config.set":
		var req registryv1.ConfigSetRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		outcome, moved, err := d.settings.Set(req.GetValuesJson())
		if err != nil {
			st := &rigv1.Status{Code: rigv1.Code_CODE_INVALID, Message: "rig.config.set: " + err.Error()}
			if ref, ok := errors.AsType[*config.RefusalError](err); ok {
				st.Message = "rig.config.set: refused whole, nothing applied: " + ref.Key + " in layer " + ref.Layer
				st.Precondition = "every value in the set satisfies the key's schema; one refusal applies none"
				st.Actual = ref.Key + ": " + ref.Reason
				st.Fix = "correct " + ref.Key + "; `rig config get " + ref.Key + "` shows what it holds now"
			}
			c.failStatus(id, st)
			return
		}
		d.settingsMoved(moved)
		c.reply(id, &registryv1.ConfigSetResponse{Outcome: outcome, SnapshotPath: d.snapshotPath})
	}
}

// settingsMoved applies what is live in the daemon, rewrites the snapshot
// and tells the bus.
func (d *Daemon) settingsMoved(keys []string) {
	if len(keys) == 0 {
		return
	}
	d.applyLogLevel()
	d.writeSnapshot()
	d.events.publishRig("config.changed", &registryv1.ConfigChanged{Keys: keys})
}

// applyLogLevel sets the handler's floor from log.level, which the schema
// has already restricted to a level slog reads.
func (d *Daemon) applyLogLevel() {
	if d.logLevel == nil {
		return
	}
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(d.settings.String("log.level"))); err == nil {
		d.logLevel.Set(lv)
	}
}

// writeSnapshot writes the resolution where a reader finds it while rig is
// down (section 5g). A failure is logged: the live answer stays right, and
// only the copy for a reader without rig is stale.
func (d *Daemon) writeSnapshot() {
	if d.snapshotPath == "" {
		return
	}
	if err := d.settings.WriteSnapshot(d.snapshotPath); err != nil {
		d.log.Warn("the settings snapshot was not written", "path", d.snapshotPath, "err", err)
	}
}

func layerOut(s config.Source) *registryv1.ConfigLayerValue {
	return &registryv1.ConfigLayerValue{Layer: s.Layer, File: s.File, ValueJson: config.JSON(s.Value)}
}
