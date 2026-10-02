package observe

import (
	"context"
	"io"
	"log/slog"
)

// Handler is rigd's slog.Handler: a record goes into the store in-process,
// no socket and no wire (decision 3).
//
// Until the store is durable it also tees WARN and above to w, so a crash
// before the segments exist still reaches the journal; after that, nothing
// (decision 14). A store that is never attached, an unnamed estate's, keeps
// the tee: it has no file, so stderr is the only copy that outlives it.
type Handler struct {
	s      *Store
	client string
	level  slog.Leveler
	prefix string      // the open group, "a.b." or ""
	attrs  []slog.Attr // WithAttrs so far, already prefixed
	tee    slog.Handler
}

// NewHandler logs into s as client, at level and above, teeing to w.
func NewHandler(s *Store, client string, level slog.Leveler, w io.Writer) *Handler {
	h := &Handler{s: s, client: client, level: level}
	if w != nil {
		h.tee = slog.NewTextHandler(w, &slog.HandlerOptions{Level: teeLevel{level}})
	}
	return h
}

// teeLevel is the handler's level, but never below WARN.
type teeLevel struct{ l slog.Leveler }

func (t teeLevel) Level() slog.Level { return max(t.l.Level(), slog.LevelWarn) }

// Enabled implements slog.Handler.
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

// Handle implements slog.Handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	rec := Record{At: r.Time.UnixNano(), Level: int(r.Level), Client: h.client, Message: r.Message}
	if n := len(h.attrs) + r.NumAttrs(); n > 0 {
		rec.Attrs = make(map[string]string, n)
		for _, a := range h.attrs {
			flatten(rec.Attrs, "", a)
		}
		r.Attrs(func(a slog.Attr) bool {
			flatten(rec.Attrs, h.prefix, a)
			return true
		})
	}
	h.s.Append(rec)
	if h.tee != nil && h.tee.Enabled(ctx, r.Level) && !h.s.Durable() {
		return h.tee.Handle(ctx, r)
	}
	return nil
}

// flatten writes a, groups joined with ".", values as text.
func flatten(into map[string]string, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, g := range v.Group() {
			flatten(into, p, g)
		}
		return
	}
	if a.Key == "" {
		return
	}
	into[prefix+a.Key] = v.String()
}

// WithAttrs implements slog.Handler.
func (h *Handler) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	c.attrs = make([]slog.Attr, 0, len(h.attrs)+len(as))
	c.attrs = append(c.attrs, h.attrs...)
	for _, a := range as {
		if h.prefix != "" {
			a.Key = h.prefix + a.Key
		}
		c.attrs = append(c.attrs, a)
	}
	if h.tee != nil {
		c.tee = h.tee.WithAttrs(as)
	}
	return &c
}

// WithGroup implements slog.Handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix = h.prefix + name + "."
	if h.tee != nil {
		c.tee = h.tee.WithGroup(name)
	}
	return &c
}
