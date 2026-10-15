// Package probe 识别客户端明文 HTTP 测速请求，由节点直接应答
package probe

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	Port        = 80
	PeekTimeout = 200 * time.Millisecond
	IdleTimeout = 30 * time.Second
	maxHeader   = 8192
)

type page struct {
	status      string
	contentType string
	body        string
}

var noContent = page{status: "204 No Content"}

func ok(contentType, body string) page {
	return page{status: "200 OK", contentType: contentType, body: body}
}

var (
	appleSuccess = ok("text/html", "<HTML><HEAD><TITLE>Success</TITLE></HEAD><BODY>Success</BODY></HTML>")
	nmOnline     = ok("text/plain", "NetworkManager is online\n")
)

// pages 按 host+path 精确匹配的检测地址；路径为 /generate_204、/gen_204 的任意域名另行匹配
var pages = map[string]page{
	"cp.cloudflare.com/":                         noContent,
	"captive.apple.com/":                         appleSuccess,
	"captive.apple.com/hotspot-detect.html":      appleSuccess,
	"www.apple.com/library/test/success.html":    appleSuccess,
	"detectportal.firefox.com/success.txt":       ok("text/plain", "success\n"),
	"detectportal.firefox.com/canonical.html":    ok("text/html", `<meta http-equiv="refresh" content="0;url=https://support.mozilla.org/kb/captive-portal"/>`),
	"www.msftconnecttest.com/connecttest.txt":    ok("text/plain", "Microsoft Connect Test"),
	"www.msftncsi.com/ncsi.txt":                  ok("text/plain", "Microsoft NCSI"),
	"nmcheck.gnome.org/check_network_status.txt": nmOnline,
	"network-test.debian.org/nm":                 nmOnline,
}

// RequestLineComplete 首包里已经包含完整请求行
func RequestLineComplete(b []byte) bool {
	return bytes.Contains(b, []byte("\r\n"))
}

// Match 首包是测速请求时返回应答内容，否则返回 nil；host 为目标域名（可为空）
func Match(host string, b []byte) []byte {
	i := bytes.Index(b, []byte("\r\n"))
	if i < 0 {
		return nil
	}
	fields := strings.Fields(string(b[:i]))
	if len(fields) != 3 || !strings.HasPrefix(fields[2], "HTTP/1.") {
		return nil
	}
	method, path := fields[0], fields[1]
	if method != "GET" && method != "HEAD" {
		return nil
	}
	if strings.HasPrefix(path, "http://") {
		rest := path[len("http://"):]
		j := strings.IndexByte(rest, '/')
		if j < 0 {
			host, path = rest, "/"
		} else {
			host, path = rest[:j], rest[j:]
		}
	}
	if j := strings.IndexByte(path, '?'); j >= 0 {
		path = path[:j]
	}
	if host == "" {
		host = headerHost(b[i+2:])
	}
	host = strings.ToLower(strings.TrimSuffix(host, ":80"))

	p, found := pages[host+path]
	if !found {
		if path != "/generate_204" && path != "/gen_204" {
			return nil
		}
		p = noContent
	}
	return p.response(method == "HEAD")
}

// Serve 以 keep-alive 方式依次应答 data 及后续读到的测速请求（mihomo unified-delay 会在同一连接上发第二次请求），
// 遇到非测速请求、空闲超时或读写出错时返回，由调用方关闭连接
func Serve(host string, data []byte, read func(timeout time.Duration) ([]byte, error), write func([]byte) error) {
	pending := append([]byte(nil), data...)
	var readErr error
	for {
		if end := bytes.Index(pending, []byte("\r\n\r\n")); end >= 0 {
			response := Match(host, pending[:end+4])
			if response == nil || write(response) != nil {
				return
			}
			pending = pending[end+4:]
			continue
		}
		if readErr != nil || len(pending) > maxHeader {
			return
		}
		var b []byte
		b, readErr = read(IdleTimeout)
		// xray 的 TimeoutWrapperReader 超时返回 (nil, nil)，空读同样视为空闲超时
		if len(b) == 0 {
			return
		}
		pending = append(pending, b...)
	}
}

func headerHost(b []byte) string {
	for _, line := range strings.Split(string(b), "\r\n") {
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(name, "Host") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (p page) response(head bool) []byte {
	var sb strings.Builder
	fmt.Fprintf(&sb, "HTTP/1.1 %s\r\n", p.status)
	if p.contentType != "" {
		fmt.Fprintf(&sb, "Content-Type: %s\r\n", p.contentType)
	}
	fmt.Fprintf(&sb, "Content-Length: %d\r\n\r\n", len(p.body))
	if !head {
		sb.WriteString(p.body)
	}
	return []byte(sb.String())
}
