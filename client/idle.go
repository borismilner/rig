package client

import (
	"sync"
	"time"
)

// Section 54: a program started on call exits on its own when it has
// nothing left to do, because rig cannot see from outside what it still
// holds. These two are the common case of that decision, added to the
// surface by plan/54 ("exit after a set time with no call in flight and
// nothing held by the program").

// busy is how many requests are being answered plus how many holds are
// taken. changed is closed and replaced on every change, so a waiter
// re-reads the count instead of polling it.
type busy struct {
	mu      sync.Mutex
	n       int
	changed chan struct{}
}

// add moves the count by d and answers the move back, once.
func (b *busy) add(d int) (undo func()) {
	b.move(d)
	var once sync.Once
	return func() { once.Do(func() { b.move(-d) }) }
}

func (b *busy) move(d int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n += d
	if b.changed != nil {
		close(b.changed)
		b.changed = nil
	}
}

// state is the count now and a channel closed at its next change.
func (b *busy) state() (int, <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.changed == nil {
		b.changed = make(chan struct{})
	}
	return b.n, b.changed
}

// Hold says the program still has something in hand that outlives the call
// that made it, such as a card on screen waiting for an answer, so it is
// not idle. Release it when that ends; releasing twice is harmless.
func (c *Client) Hold() (release func()) { return c.busy.add(1) }

// Idle closes once d has passed with no request being answered and nothing
// held, or when the client is closed. A program started on call waits on
// it, then closes the client and exits 0, which rig reads as at rest.
func (c *Client) Idle(d time.Duration) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		defer close(out)
		for {
			n, changed := c.busy.state()
			if n > 0 {
				select {
				case <-changed:
					continue
				case <-c.closed:
					return
				}
			}
			t := time.NewTimer(d)
			select {
			case <-t.C:
				return
			case <-changed:
				t.Stop()
			case <-c.closed:
				t.Stop()
				return
			}
		}
	}()
	return out
}
