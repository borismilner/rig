package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

const askDesktop = "desktop"

type askPolicy struct {
	Kind       string `json:"kind"`
	Mode       string `json:"mode"`
	QuietHours *struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"quiet_hours"`
	MaxPerHour      int      `json:"max_per_hour"`
	RoutineChannels []string `json:"routine_channels"`
	NormalChannels  []string `json:"normal_channels"`
	UrgentChannels  []string `json:"urgent_channels"`
}

type askDelivery struct {
	channel, note     string
	suppressed, never bool
}

func (d *Daemon) policyFor(kind string) (askPolicy, error) {
	var policies []askPolicy
	for _, v := range d.settings.Get("ask.policies") {
		if v.Key != "ask.policies" {
			continue
		}
		encoded, err := json.Marshal(v.Winner.Value)
		if err != nil {
			return askPolicy{}, err
		}
		if err := json.Unmarshal(encoded, &policies); err != nil {
			return askPolicy{}, err
		}
	}
	for _, p := range policies {
		if p.Kind == kind {
			return p, nil
		}
	}
	return askPolicy{Mode: "ordinary"}, nil
}

func (d *Daemon) decideAskDelivery(ctx context.Context, st *record.Store, q *registryv1.AskOpenRequest, now time.Time) (askDelivery, error) {
	p, err := d.policyFor(q.GetKind())
	if err != nil {
		return askDelivery{}, err
	}
	if p.Mode == "never" {
		return askDelivery{channel: "none", note: "kind policy: never ask", never: true}, nil
	}
	channels := p.NormalChannels
	switch q.GetUrgency() {
	case "routine":
		channels = p.RoutineChannels
	case "urgent":
		channels = p.UrgentChannels
	}
	if len(channels) == 0 {
		channels = []string{askDesktop}
	}
	delivery := askDelivery{channel: "none", note: "requested channels unavailable on this estate", suppressed: true}
	for _, channel := range channels {
		if channel == askDesktop {
			delivery = askDelivery{channel: askDesktop, note: "shown on desktop"}
			break
		}
	}
	if delivery.channel == "none" {
		return delivery, nil
	}
	if p.Mode == "always" || askMode(q.GetFailureMode()) == "gating" {
		return delivery, nil
	}
	if p.QuietHours != nil {
		loc, err := time.LoadLocation(d.settings.String("ask.timezone"))
		if err != nil {
			return askDelivery{}, fmt.Errorf("ask timezone: %w", err)
		}
		if inQuietHours(now.In(loc), p.QuietHours.Start, p.QuietHours.End) {
			return askDelivery{channel: askDesktop, note: "held by quiet hours", suppressed: true}, nil
		}
	}
	if p.MaxPerHour > 0 {
		recs, err := st.Find(ctx, record.QueryFilter{
			Project: notificationProject, Kind: notificationKind, Field: "ask_kind", Value: q.GetKind(),
		})
		if err != nil {
			return askDelivery{}, err
		}
		count := 0
		for _, rec := range recs {
			if now.Sub(rec.Prov.CreatedAt) < time.Hour {
				count++
			}
		}
		if count >= p.MaxPerHour {
			return askDelivery{channel: askDesktop, note: "held by hourly rate limit", suppressed: true}, nil
		}
	}
	if d.toasts.suppress(askSeverity(q.GetUrgency())) {
		return askDelivery{channel: askDesktop, note: "held by do not disturb", suppressed: true}, nil
	}
	return delivery, nil
}

func inQuietHours(now time.Time, start, end string) bool {
	minute := now.Hour()*60 + now.Minute()
	parse := func(s string) int { return int(s[0]-'0')*600 + int(s[1]-'0')*60 + int(s[3]-'0')*10 + int(s[4]-'0') }
	a, b := parse(start), parse(end)
	if a == b {
		return false
	}
	if a < b {
		return minute >= a && minute < b
	}
	return minute >= a || minute < b
}
