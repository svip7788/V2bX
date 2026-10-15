package dispatcher

import (
	"context"
	"strings"
	"testing"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/pipe"
)

func newProbeLink(t *testing.T, payload string) (*transport.Link, *pipe.Reader) {
	t.Helper()
	upR, upW := pipe.New()
	downR, downW := pipe.New()
	b := buf.New()
	b.WriteString(payload)
	if err := upW.WriteMultiBuffer(buf.MultiBuffer{b}); err != nil {
		t.Fatal(err)
	}
	return &transport.Link{Reader: upR, Writer: downW}, downR
}

func TestReplyLatencyProbe(t *testing.T) {
	dest := net.TCPDestination(net.DomainAddress("www.gstatic.com"), 80)
	link, down := newProbeLink(t, "GET /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n")
	if !replyLatencyProbe(context.Background(), link, dest) {
		t.Fatal("expected probe to be answered")
	}
	mb, err := down.ReadMultiBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if got := mb.String(); !strings.HasPrefix(got, "HTTP/1.1 204 ") {
		t.Fatalf("unexpected response %q", got)
	}
}

func TestReplyLatencyProbeKeepsPayload(t *testing.T) {
	const req = "GET /index.html HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n"
	dest := net.TCPDestination(net.DomainAddress("www.gstatic.com"), 80)
	link, _ := newProbeLink(t, req)
	if replyLatencyProbe(context.Background(), link, dest) {
		t.Fatal("unexpected probe match")
	}
	mb, err := link.Reader.ReadMultiBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if got := mb.String(); got != req {
		t.Fatalf("payload changed: %q", got)
	}
}
