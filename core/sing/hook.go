package sing

import (
	"context"
	"net"
	"sync"

	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/common/rate"

	"github.com/InazumaV/V2bX/limiter"

	"github.com/InazumaV/V2bX/common/counter"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	N "github.com/sagernet/sing/common/network"
)

var _ adapter.ConnectionTracker = (*HookServer)(nil)

type HookServer struct {
	counter sync.Map //map[string]*counter.TrafficCounter
}

func (h *HookServer) ModeList() []string {
	return nil
}

func (h *HookServer) RoutedConnection(_ context.Context, conn net.Conn, m adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) net.Conn {
	l, err := limiter.GetLimiter(m.Inbound)
	if err != nil {
		log.Error("get limiter for ", m.Inbound, " error: ", err)
		conn.Close()
		return conn
	}
	taguuid := format.UserTag(m.Inbound, m.User)
	ip := m.Source.Addr.String()
	if b, r := l.CheckLimit(taguuid, ip, true, true); r {
		conn.Close()
		log.Error("[", m.Inbound, "] ", "Limited ", m.User, " by ip or conn")
		return conn
	} else if b != nil {
		conn = rate.NewConnRateLimiter(conn, b)
	}
	destStr := m.Destination.AddrString()
	if l.CheckDomainRule(destStr) {
		log.Error("[", m.Inbound, "] User ", m.User, " access domain ", destStr, " reject by rule")
		conn.Close()
		return conn
	}
	if protocol := m.Protocol; len(protocol) != 0 && l.CheckProtocolRule(protocol) {
		log.Error("[", m.Inbound, "] User ", m.User, " access protocol ", protocol, " reject by rule")
		conn.Close()
		return conn
	}
	t := h.getCounter(m.Inbound)
	conn = counter.NewConnCounter(conn, t.GetCounter(m.User))
	return conn
}

func (h *HookServer) getCounter(tag string) *counter.TrafficCounter {
	if c, ok := h.counter.Load(tag); ok {
		return c.(*counter.TrafficCounter)
	}
	c, _ := h.counter.LoadOrStore(tag, counter.NewTrafficCounter())
	return c.(*counter.TrafficCounter)
}

func (h *HookServer) RoutedPacketConnection(_ context.Context, conn N.PacketConn, m adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) N.PacketConn {
	l, err := limiter.GetLimiter(m.Inbound)
	if err != nil {
		log.Error("get limiter for ", m.Inbound, " error: ", err)
		conn.Close()
		return conn
	}
	ip := m.Source.Addr.String()
	taguuid := format.UserTag(m.Inbound, m.User)
	if _, r := l.CheckLimit(taguuid, ip, false, false); r {
		conn.Close()
		log.Error("[", m.Inbound, "] ", "Limited ", m.User, " by ip or conn")
		return conn
	}
	destStr := m.Destination.AddrString()
	if l.CheckDomainRule(destStr) {
		log.Error("[", m.Inbound, "] User ", m.User, " access domain ", destStr, " reject by rule")
		conn.Close()
		return conn
	}
	if protocol := m.Destination.Network(); len(protocol) != 0 && l.CheckProtocolRule(protocol) {
		log.Error("[", m.Inbound, "] User ", m.User, " access protocol ", protocol, " reject by rule")
		conn.Close()
		return conn
	}
	t := h.getCounter(m.Inbound)
	conn = counter.NewPacketConnCounter(conn, t.GetCounter(m.User))
	return conn
}
