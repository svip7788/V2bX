package task

import (
	"sync"
	"time"
)

type Task struct {
	Interval time.Duration
	Execute  func() error
	access   sync.Mutex
	running  bool
	stop     chan struct{}
	execMu   sync.Mutex
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
	t.running = true
	t.stop = make(chan struct{})
	t.access.Unlock()

	go func() {
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
}

func (t *Task) Restart(interval time.Duration) {
	t.close()
	t.Interval = interval
	_ = t.Start(false)
}
