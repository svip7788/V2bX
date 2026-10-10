package userconn

import (
	"net"
	"sync"
	"testing"
)

type fakeConn struct {
	net.Conn
	closed bool
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func TestCloseUsers(t *testing.T) {
	var conns sync.Map
	a1, a2, b := &fakeConn{}, &fakeConn{}, &fakeConn{}
	conns.Store(a1, "a")
	conns.Store(a2, "a")
	conns.Store(b, "b")

	CloseUsers(&conns, map[string]struct{}{"a": {}})

	if !a1.closed || !a2.closed || b.closed {
		t.Fatalf("closed state wrong: a1=%v a2=%v b=%v", a1.closed, a2.closed, b.closed)
	}
	n := 0
	conns.Range(func(_, _ any) bool { n++; return true })
	if n != 1 {
		t.Fatalf("remaining conns = %d, want 1", n)
	}
}
