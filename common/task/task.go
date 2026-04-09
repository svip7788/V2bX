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
	done     chan struct{}
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

func (t *Task) Restart(interval time.Duration) {
	t.Close()
	t.Interval = interval
	_ = t.Start(false)
}
