package watch

// Watcher holds the resources shared by scanning and the future event loop.
// The daemon owns its lifecycle: it opens State before construction and closes
// it only after the event loop has stopped.
type Watcher struct {
	state       *State
	backend     backend
	kernelLimit int
}

func newWatcher(state *State, backend backend, kernelLimit int) *Watcher {
	if kernelLimit == 0 {
		kernelLimit = kernelWatchLimit()
	}
	return &Watcher{
		state:       state,
		backend:     backend,
		kernelLimit: kernelLimit,
	}
}
