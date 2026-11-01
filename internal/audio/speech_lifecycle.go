package audio

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
)

// SpeechStatus describes one utterance, not the estate's sound settings.
type SpeechStatus struct {
	ID, State string
}

const (
	SpeechQueued    = "queued"
	SpeechPlaying   = "playing"
	SpeechCompleted = "completed"
	SpeechCancelled = "cancelled"
	SpeechDropped   = "dropped"
	SpeechFailed    = "failed"

	// Retain recent terminal results without retaining text or growing forever.
	speechHistory = 128
)

// ErrSpeechNotFound also covers foreign and evicted handles.
var ErrSpeechNotFound = errors.New("speech handle not found for this caller; it may have been evicted or the daemon restarted")

type speechJob struct {
	mu     sync.Mutex
	id     string
	owner  string
	state  string
	cancel chan struct{}
	done   chan struct{}
}

func terminalSpeech(state string) bool {
	return state != SpeechQueued && state != SpeechPlaying
}

func (j *speechJob) status() SpeechStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	return SpeechStatus{ID: j.id, State: j.state}
}

func (j *speechJob) begin() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state != SpeechQueued {
		return false
	}
	j.state = SpeechPlaying
	return true
}

func (j *speechJob) finish(state string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if terminalSpeech(j.state) {
		return
	}
	// Playback chooses the result. A stop arriving after drain completed
	// must not relabel that completed utterance while it is being published.
	j.state = state
	close(j.done)
}

func (j *speechJob) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if terminalSpeech(j.state) {
		return
	}
	select {
	case <-j.cancel:
	default:
		close(j.cancel)
	}
	if j.state == SpeechQueued {
		j.state = SpeechCancelled
		close(j.done)
	}
}

// StartSpeech adds a cancellable utterance to the existing audio queue. The
// caller identity is supplied by the daemon, never by a request field.
func (a *Audio) StartSpeech(owner, text string) (SpeechStatus, error) {
	line, err := a.sayLine(text)
	if err != nil {
		return SpeechStatus{}, err
	}
	j := &speechJob{id: uuid.NewString(), owner: owner, state: SpeechQueued,
		cancel: make(chan struct{}), done: make(chan struct{})}
	a.mu.Lock()
	if a.speech == nil {
		a.speech = make(map[string]*speechJob)
	}
	// All active items fit in the queue plus its current player. Evict only
	// terminal handles; never make a playing utterance impossible to stop.
	for len(a.speechOrder) >= speechHistory {
		removed := false
		for i, id := range a.speechOrder {
			if terminalSpeech(a.speech[id].status().State) {
				delete(a.speech, id)
				a.speechOrder = append(a.speechOrder[:i], a.speechOrder[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			a.mu.Unlock()
			return SpeechStatus{}, errors.New("speech handle capacity reached")
		}
	}
	a.speech[j.id] = j
	a.speechOrder = append(a.speechOrder, j.id)
	// Keep insertion and enqueue atomic against other producers and Close.
	a.enqueueLocked(item{line: line, speech: j})
	a.mu.Unlock()
	return j.status(), nil
}

func (a *Audio) speechFor(owner, id string) (*speechJob, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	j := a.speech[id]
	if j == nil || j.owner != owner {
		return nil, ErrSpeechNotFound
	}
	return j, nil
}

// SpeechState reads a caller-owned handle without changing playback.
func (a *Audio) SpeechState(owner, id string) (SpeechStatus, error) {
	j, err := a.speechFor(owner, id)
	if err != nil {
		return SpeechStatus{}, err
	}
	return j.status(), nil
}

// StopSpeech waits until the item cannot play any further. Cancellation of
// this wait does not undo the stop; a retry can observe the terminal result.
func (a *Audio) StopSpeech(ctx context.Context, owner, id string) (SpeechStatus, error) {
	j, err := a.speechFor(owner, id)
	if err != nil {
		return SpeechStatus{}, err
	}
	j.stop()
	select {
	case <-j.done:
		return j.status(), nil
	case <-ctx.Done():
		return SpeechStatus{}, ctx.Err()
	}
}
