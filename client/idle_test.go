package client

import (
	"testing"
	"time"
)

func fired(ch <-chan struct{}, within time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(within):
		return false
	}
}

// Idle fires after the quiet period, not while a hold or a request is
// open, and the period starts again from when the last one ended.
func TestIdleWaitsForHoldsAndRequests(t *testing.T) {
	c := &Client{closed: make(chan struct{})}
	if !fired(c.Idle(20*time.Millisecond), time.Second) {
		t.Fatal("a client with nothing in hand never went idle")
	}

	release := c.Hold()
	idle := c.Idle(20 * time.Millisecond)
	if fired(idle, 100*time.Millisecond) {
		t.Fatal("went idle while a hold was taken")
	}
	answered := c.busy.add(1) // a request being answered, as answer() counts it
	release()
	release() // twice is harmless
	if fired(idle, 100*time.Millisecond) {
		t.Fatal("went idle while a request was being answered")
	}
	ended := time.Now()
	answered()
	if !fired(idle, time.Second) {
		t.Fatal("never went idle once everything ended")
	}
	if waited := time.Since(ended); waited < 20*time.Millisecond {
		t.Fatalf("went idle %v after the last request, before the quiet period", waited)
	}
}

func TestIdleEndsWhenTheClientCloses(t *testing.T) {
	c := &Client{closed: make(chan struct{})}
	defer c.Hold()()
	idle := c.Idle(time.Hour)
	close(c.closed)
	if !fired(idle, time.Second) {
		t.Fatal("Idle outlived the client")
	}
}
