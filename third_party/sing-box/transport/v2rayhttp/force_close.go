package v2rayhttp

import (
	"net/http"
	"reflect"
	"sync"
	"unsafe"

	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/net/http2"
)

type clientConnPool struct {
	t     *http2.Transport
	mu    sync.Mutex
	conns map[string][]*http2.ClientConn // key is host:port
}

type efaceWords struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

func ResetTransport(rawTransport http.RoundTripper) http.RoundTripper {
	switch transport := rawTransport.(type) {
	case *http.Transport:
		transport.CloseIdleConnections()
		return transport.Clone()
	case *http2.Transport:
		connPool := transportConnPool(transport)
		if connPool == nil || reflect.TypeOf(connPool).String() != "*http2.clientConnPool" {
			transport.CloseIdleConnections()
			return transport
		}
		p := (*clientConnPool)((*efaceWords)(unsafe.Pointer(&connPool)).data)
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, vv := range p.conns {
			for _, cc := range vv {
				cc.Close()
			}
		}
		return transport
	default:
		panic(E.New("unknown transport type: ", reflect.TypeOf(transport)))
	}
}

// x/net 新版本里 (*Transport).connPool 会被内联，linkname 拿不到符号，改为直接读字段
func transportConnPool(t *http2.Transport) http2.ClientConnPool {
	field := reflect.ValueOf(t).Elem().FieldByName("connPoolOrDef")
	if !field.IsValid() || field.IsNil() {
		return nil
	}
	return *(*http2.ClientConnPool)(unsafe.Pointer(field.UnsafeAddr()))
}
