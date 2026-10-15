package sing

import (
	"io"
	"net"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

func TestReplyLatencyProbe(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go client.Write([]byte("GET /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n"))
	m := adapter.InboundContext{Destination: M.ParseSocksaddrHostPort("www.gstatic.com", 80)}
	result := make(chan bool, 1)
	go func() {
		_, answered := replyLatencyProbe(server, m)
		result <- answered
	}()
	resp, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	if !<-result {
		t.Fatal("expected probe to be answered")
	}
	if !strings.HasPrefix(string(resp), "HTTP/1.1 204 ") {
		t.Fatalf("unexpected response %q", resp)
	}
}

func TestReplyLatencyProbeKeepsPayload(t *testing.T) {
	const req = "GET /index.html HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n"
	client, server := net.Pipe()
	defer client.Close()
	go client.Write([]byte(req))
	m := adapter.InboundContext{Destination: M.ParseSocksaddrHostPort("www.gstatic.com", 80)}
	conn, answered := replyLatencyProbe(server, m)
	if answered {
		t.Fatal("unexpected probe match")
	}
	got := make([]byte, len(req))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != req {
		t.Fatalf("payload changed: %q", got)
	}
}
