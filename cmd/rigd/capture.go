package main

import (
	"cmp"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/borismilner/rig/internal/capture"
	"github.com/borismilner/rig/internal/config"
	"github.com/borismilner/rig/internal/paths"
)

// newCapture builds section 58's screen capture. Pictures live in the
// runtime directory: memory-backed, private to the user and gone at logout,
// which is where a picture of someone's screen belongs. A daemon without a
// runtime directory runs blind and says so; it does not guess one.
func newCapture(log *slog.Logger, settings *config.Resolver, estate string) *capture.Service {
	run, err := paths.RuntimeDir()
	if err != nil {
		log.Warn("screen capture is off", "err", err)
		return nil
	}
	svc, err := capture.New(capture.NewGNOME(nil), filepath.Join(run, "capture", cmp.Or(estate, "default")),
		func() capture.Limits { return captureLimits(settings) })
	if err != nil {
		log.Warn("screen capture is off", "err", err)
		return nil
	}
	log.Info("screen capture ready", "dir", svc.Dir())
	return svc
}

// captureLimits reads the capture.* settings afresh, so a live change applies
// to the next capture.
func captureLimits(s *config.Resolver) capture.Limits {
	return capture.Limits{
		Timeout:     time.Duration(s.Int("capture.timeout_ms")) * time.Millisecond,
		Concurrent:  int(s.Int("capture.concurrent")),
		MaxBytes:    s.Int("capture.max_file_mb") << 20,
		MaxPixels:   s.Int("capture.max_pixels"),
		KeepFiles:   int(s.Int("capture.keep_files")),
		KeepBytes:   s.Int("capture.keep_mb") << 20,
		KeepAge:     time.Duration(s.Int("capture.keep_hours")) * time.Hour,
		InlineBytes: int(s.Int("capture.inline_kb")) << 10,
	}
}
