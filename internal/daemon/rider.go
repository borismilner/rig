package daemon

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/borismilner/rig/internal/coord"
)

// The sync rider (plan/53 slice 4): what changed for this seat since its
// last call, as one line on whatever MCP tool it called next. AgentBox's said
// who joined or left; this one also says the seat's lease was broken, its
// claim taken over, a peer is queued behind it, a signal was addressed to it,
// and the bus seq it read from, so events_wait from there has them in full.

// riderMax is how many items one line carries before "and N more".
const riderMax = 8

// riderCursor is one MCP connection's place on the bus.
type riderCursor struct {
	mu     sync.Mutex
	seat   string
	cursor uint64
}

// ride answers the line for one call, or nothing. A seat's cursor starts
// when it takes the seat, since announce's answer is the roster as it
// stands. events_wait neither carries a line nor moves the cursor: it
// answers only the kinds it was asked for, so news outside them is still
// owed.
func (m *mcpCaller) ride(tool string) string {
	seat := m.me().seat
	r := m.rider
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case seat == "":
		r.seat = ""
		return ""
	case r.seat != seat:
		r.seat, r.cursor = seat, m.events.latest()
		return ""
	case tool == "events_wait":
		return ""
	}
	from := r.cursor
	items, latest, gap := m.riderNews(from, seat)
	if latest > r.cursor {
		r.cursor = latest
	}
	return riderLine(items, from, gap)
}

// latest is the newest seq the bus has numbered.
func (b *eventBus) latest() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// riderNews reads what concerns seat after the cursor, oldest first, the seq
// it read through, and whether the ring dropped some of it.
func (d *Daemon) riderNews(after uint64, seat string) (items []string, latest uint64, gap bool) {
	me := seatSource(seat)
	b := &d.events
	b.mu.Lock()
	latest, gap = b.seq, after < b.lost
	var got []busItem
	for _, it := range b.items {
		if it.ev.GetSeq() > after {
			got = append(got, it)
		}
	}
	b.mu.Unlock()
	if st := b.durable; st != nil {
		sigs, more, err := st.SignalsAfter(after, coord.MaxSignalBatch, func(s *coord.Signal) bool {
			return s.To == me && s.Seq <= latest
		})
		if err == nil {
			for i := range sigs {
				got = append(got, busItem{ev: signalEvent(&sigs[i]), to: sigs[i].To})
			}
			if more {
				latest = sigs[len(sigs)-1].Seq
			}
		}
		slices.SortFunc(got, func(a, b busItem) int { return cmpSeq(a.ev.GetSeq(), b.ev.GetSeq()) })
	}
	for _, it := range got {
		if it.ev.GetSeq() > latest {
			break
		}
		if s := riderSays(it, seat); s != "" {
			items = append(items, s)
		}
	}
	return items, latest, gap
}

// riderSays is one event in the rider's words, or nothing when it is not
// this seat's news. Never the seat's own doing, and never a value.
func riderSays(it busItem, seat string) string {
	kind, payload := it.ev.GetKind(), []byte(it.ev.GetPayloadJson())
	switch {
	case kind == "roster.changed":
		var p rosterChange
		if json.Unmarshal(payload, &p) != nil || p.Seat == seat {
			return ""
		}
		if p.Change == "left" {
			return p.Seat + " left"
		}
		if p.Purpose != "" {
			return fmt.Sprintf("%s arrived (%q)", p.Seat, p.Purpose)
		}
		return p.Seat + " arrived"
	case kind == "lease.changed":
		var p leaseChange
		if json.Unmarshal(payload, &p) != nil || p.Holder != seat || p.By == seat {
			return ""
		}
		switch p.Change {
		case "broken":
			return fmt.Sprintf("your lease %s was broken by %s", p.Name, p.By)
		case "expired":
			return fmt.Sprintf("your lease %s expired", p.Name)
		case "fenced":
			return fmt.Sprintf("your lease %s ran out, so rig stopped the run it fenced", p.Name)
		case "queued":
			return fmt.Sprintf("%s is waiting on your lease %s", p.By, p.Name)
		}
	case strings.HasPrefix(kind, "shared."):
		var p sharedChange
		if json.Unmarshal(payload, &p) != nil || p.By == seat {
			return ""
		}
		switch {
		case p.Change == "set" && p.Was == seat:
			return fmt.Sprintf("your claim %s was taken by %s", p.Key, p.By)
		case p.Change == "deleted" && p.Owner == seat:
			return fmt.Sprintf("your claim %s was deleted by %s", p.Key, p.By)
		}
	case strings.HasPrefix(kind, "signal.") && it.to == seatSource(seat):
		return fmt.Sprintf("%s for you from %s", kind, strings.TrimPrefix(it.ev.GetSource(), "seat:"))
	}
	return ""
}

// riderLine renders the items read after from, or nothing when there are
// none and nothing was lost.
func riderLine(items []string, from uint64, gap bool) string {
	if len(items) == 0 && !gap {
		return ""
	}
	var b strings.Builder
	b.WriteString("sync: ")
	if gap {
		b.WriteString("some news was lost before this; re-read list_agents, lease_list and shared_get. ")
	}
	shown := items
	if len(shown) > riderMax {
		shown = shown[:riderMax]
	}
	b.WriteString(strings.Join(shown, "; "))
	if n := len(items) - len(shown); n > 0 {
		fmt.Fprintf(&b, "; and %d more", n)
	}
	fmt.Fprintf(&b, ". events_wait after %d has them in full.", from)
	return b.String()
}
