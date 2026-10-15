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

func newProbeLink(t *testing.T, payloads ...string) (*transport.Link, *pipe.Reader) {
	t.Helper()
	upR, upW := pipe.New()
	downR, downW := pipe.New()
	for _, p := range payloads {
		if err := upW.WriteMultiBuffer(buf.MergeBytes(nil, []byte(p))); err != nil {
			t.Fatal(err)
		}
	}
	upW.Close()
	return &transport.Link{Reader: upR, Writer: downW}, downR
}

func readAll(t *testing.T, r *pipe.Reader) string {
	t.Helper()
	var sb strings.Builder
	for {
		mb, err := r.ReadMultiBuffer()
		sb.WriteString(mb.String())
		buf.ReleaseMulti(mb)
		if err != nil {
			return sb.String()
		}
	}
}

func TestReplyLatencyProbeKeepAlive(t *testing.T) {
	const req = "GET /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n"
	dest := net.TCPDestination(net.DomainAddress("www.gstatic.com"), 80)
	link, down := newProbeLink(t, req, req)
	if !replyLatencyProbe(context.Background(), link, dest) {
		t.Fatal("expected probe to be answered")
	}
	if got := strings.Count(readAll(t, down), "HTTP/1.1 204 "); got != 2 {
		t.Fatalf("got %d responses, want 2", got)
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
