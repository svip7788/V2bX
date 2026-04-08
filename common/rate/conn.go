package rate

import (
	"net"

	"github.com/juju/ratelimit"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	"github.com/sagernet/sing/common/network"
)

func NewConnRateLimiter(c net.Conn, l *ratelimit.Bucket) net.Conn {
	return &Conn{
		ExtendedConn: bufio.NewExtendedConn(c),
		limiter:      l,
	}
}

type Conn struct {
	network.ExtendedConn
	limiter *ratelimit.Bucket
}

func (c *Conn) Read(b []byte) (n int, err error) {
	n, err = c.ExtendedConn.Read(b)
	if n > 0 {
		c.limiter.Wait(int64(n))
	}
	return n, err
}

func (c *Conn) Write(b []byte) (n int, err error) {
	n, err = c.ExtendedConn.Write(b)
	if n > 0 {
		c.limiter.Wait(int64(n))
	}
	return n, err
}

func (c *Conn) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err != nil {
		return err
	}
	if buffer.Len() > 0 {
		c.limiter.Wait(int64(buffer.Len()))
	}
	return nil
}

func (c *Conn) WriteBuffer(buffer *buf.Buffer) error {
	dataLen := int64(buffer.Len())
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err != nil {
		return err
	}
	if dataLen > 0 {
		c.limiter.Wait(dataLen)
	}
	return nil
}

func (c *Conn) ReaderReplaceable() bool {
	return false
}

func (c *Conn) WriterReplaceable() bool {
	return false
}
