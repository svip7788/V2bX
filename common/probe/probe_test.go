package probe

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		host   string
		req    string
		status string
	}{
		{"www.gstatic.com", "GET /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n", "204"},
		{"www.google.com", "HEAD /generate_204?x=1 HTTP/1.1\r\n", "204"},
		{"", "GET http://www.gstatic.com/generate_204 HTTP/1.1\r\n", "204"},
		{"cp.cloudflare.com", "GET / HTTP/1.1\r\n", "204"},
		{"", "GET http://cp.cloudflare.com/ HTTP/1.1\r\n", "204"},
		{"", "GET / HTTP/1.1\r\nHost: CP.Cloudflare.com:80\r\n\r\n", "204"},
		{"captive.apple.com", "GET /hotspot-detect.html HTTP/1.1\r\n", "200"},
		{"www.msftconnecttest.com", "GET /connecttest.txt HTTP/1.1\r\n", "200"},
		{"www.google.com", "GET / HTTP/1.1\r\n", ""},
		{"www.apple.com", "GET / HTTP/1.1\r\n", ""},
		{"www.gstatic.com", "POST /generate_204 HTTP/1.1\r\n", ""},
		{"www.gstatic.com", "GET /generate_204 HTTP/1.1", ""},
		{"www.gstatic.com", "\x16\x03\x01\x02\x00\x01\x00\x01\xfc\x03\x03\r\n", ""},
	}
	for _, c := range cases {
		resp := Match(c.host, []byte(c.req))
		if c.status == "" {
			if resp != nil {
				t.Errorf("Match(%q, %q) = %q, want nil", c.host, c.req, resp)
			}
			continue
		}
		if !strings.HasPrefix(string(resp), "HTTP/1.1 "+c.status+" ") {
			t.Errorf("Match(%q, %q) = %q, want status %s", c.host, c.req, resp, c.status)
		}
	}
}

func TestServeKeepAlive(t *testing.T) {
	const req = "GET /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n"
	// 首个请求头部缺结尾、第二个测速请求分片到达、第三个是非测速请求
	reads := [][]byte{[]byte("\r\n"), []byte(req[:10]), []byte(req[10:]), []byte("GET / HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n"), []byte(req)}
	var responses int
	Serve("www.gstatic.com", []byte(req[:len(req)-2]),
		func(time.Duration) ([]byte, error) {
			if len(reads) == 0 {
				return nil, io.EOF
			}
			b := reads[0]
			reads = reads[1:]
			return b, nil
		},
		func(b []byte) error {
			responses++
			return nil
		})
	if responses != 2 {
		t.Fatalf("responses = %d, want 2", responses)
	}
}

func TestServeStopsOnEmptyRead(t *testing.T) {
	var reads int
	Serve("www.gstatic.com", []byte("GET /generate_204 HTTP/1.1\r\n\r\n"),
		func(time.Duration) ([]byte, error) {
			reads++
			return nil, nil
		},
		func([]byte) error { return nil })
	if reads != 1 {
		t.Fatalf("reads = %d, want 1", reads)
	}
}

func TestMatchHeadOmitsBody(t *testing.T) {
	resp := string(Match("detectportal.firefox.com", []byte("HEAD /success.txt HTTP/1.1\r\n")))
	if !strings.HasSuffix(resp, "Content-Length: 8\r\n\r\n") {
		t.Fatalf("unexpected HEAD response %q", resp)
	}
	resp = string(Match("detectportal.firefox.com", []byte("GET /success.txt HTTP/1.1\r\n")))
	if !strings.HasSuffix(resp, "\r\n\r\nsuccess\n") {
		t.Fatalf("unexpected GET response %q", resp)
	}
}
