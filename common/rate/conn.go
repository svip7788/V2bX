package rate

import (
	"net"

	"github.com/juju/ratelimit"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
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
	if len(b) > 0 {
		c.limiter.Wait(int64(len(b)))
	}
	n, err = c.ExtendedConn.Write(b)
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
	if dataLen > 0 {
		c.limiter.Wait(dataLen)
	}
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err != nil {
		return err
	}
	return nil
}

func (c *Conn) ReaderReplaceable() bool {
	return false
}

func (c *Conn) WriterReplaceable() bool {
	return false
}

func NewPacketConnRateLimiter(c network.PacketConn, l *ratelimit.Bucket) network.PacketConn {
	return &PacketConn{
		PacketConn: c,
		limiter:    l,
	}
}

type PacketConn struct {
	network.PacketConn
	limiter *ratelimit.Bucket
}

func (c *PacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	destination, err = c.PacketConn.ReadPacket(buffer)
	if err != nil {
		return destination, err
	}
	if buffer.Len() > 0 {
		c.limiter.Wait(int64(buffer.Len()))
	}
	return destination, nil
}

func (c *PacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	dataLen := int64(buffer.Len())
	if dataLen > 0 {
		c.limiter.Wait(dataLen)
	}
	return c.PacketConn.WritePacket(buffer, destination)
}
