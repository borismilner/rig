package main

import (
	"testing"
	"time"
)

func TestLogsFlagsBecomeTheRequest(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := logsFlagSet()
	if err := l.fs.Parse([]string{
		"--since", "1h", "--until", "2026-10-02T10:00:00Z", "--client", "rigd", "--client", "shelf",
		"--level", "debug", "--grep", "pid=7", "-f",
	}); err != nil {
		t.Fatal(err)
	}
	req, err := l.request(now)
	if err != nil {
		t.Fatal(err)
	}
	until, _ := time.Parse(time.RFC3339, "2026-10-02T10:00:00Z")
	if req.GetSinceUnixNanos() != now.Add(-time.Hour).UnixNano() || req.GetUntilUnixNanos() != until.UnixNano() ||
		len(req.GetClients()) != 2 || req.GetMinLevel() != -4 || req.GetGrep() != "pid=7" || req.GetLimit() != followBacklog {
		t.Fatalf("request = %v", req)
	}
	for _, bad := range [][]string{{"--since", "yesterday"}, {"--level", "loud"}} {
		l := logsFlagSet()
		_ = l.fs.Parse(bad)
		if _, err := l.request(now); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}
}

func TestEveryLogsFlagThatTakesAValueIsDeclaredToThePartitioner(t *testing.T) {
	for _, name := range []string{"since", "until", "client", "level", "grep", "limit"} {
		if !takesValue("--" + name) {
			t.Errorf("--%s takes a value and partition() does not know it", name)
		}
	}
}
