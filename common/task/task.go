package task

import (
	"sync"
	"sync/atomic"
	"time"
)

type Task struct {
	Interval time.Duration
	Execute  func() error
	access   sync.Mutex
	running  bool
	stop     chan struct{}
	done     chan struct{}
	execMu   sync.Mutex

	pendingInterval atomic.Int64
}

func (t *Task) runExecute() error {
	t.execMu.Lock()
	defer t.execMu.Unlock()
	return t.Execute()
}

func (t *Task) Start(first bool) error {
	t.access.Lock()
	if t.running {
		t.access.Unlock()
		return nil
	}
	if t.Interval <= 0 {
		t.Interval = time.Second
	}
	t.running = true
	t.stop = make(chan struct{})
	t.done = make(chan struct{})
	t.access.Unlock()

	go func() {
		defer close(t.done)
		if first {
			if err := t.runExecute(); err != nil {
				t.close()
				return
			}
		}

		ticker := time.NewTicker(t.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-t.stop:
				return
			}

			if err := t.runExecute(); err != nil {
				t.close()
				return
			}

			if ns := t.pendingInterval.Swap(0); ns > 0 {
				ticker.Reset(time.Duration(ns))
				t.Interval = time.Duration(ns)
			}
		}
	}()

	return nil
}

func (t *Task) close() {
	t.access.Lock()
	if t.running {
		t.running = false
		close(t.stop)
	}
	t.access.Unlock()
}

func (t *Task) Close() {
	t.close()
	t.access.Lock()
	done := t.done
	t.access.Unlock()
	if done != nil {
		<-done
	}
}

// Restart changes the interval. Safe to call from within Execute.
func (t *Task) Restart(interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	t.pendingInterval.Store(int64(interval))
}
