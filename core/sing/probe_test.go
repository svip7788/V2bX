package sing

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

func TestReplyLatencyProbeKeepAlive(t *testing.T) {
	client, server := net.Pipe()
	m := adapter.InboundContext{Destination: M.ParseSocksaddrHostPort("www.gstatic.com", 80)}
	result := make(chan bool, 1)
	go func() {
		_, answered := replyLatencyProbe(server, m)
		result <- answered
	}()
	reader := bufio.NewReader(client)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://www.gstatic.com/generate_204", nil)
		go req.Write(client)
		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusNoContent || resp.Close {
			t.Fatalf("request %d: status %d close %v", i, resp.StatusCode, resp.Close)
		}
		resp.Body.Close()
	}
	client.Close()
	if !<-result {
		t.Fatal("expected probe to be answered")
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
