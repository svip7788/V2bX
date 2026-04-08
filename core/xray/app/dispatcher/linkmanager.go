package dispatcher

import (
	sync "sync"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
)

type ManagedWriter struct {
	writer  buf.Writer
	manager *LinkManager
}

func (w *ManagedWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	return w.writer.WriteMultiBuffer(mb)
}

func (w *ManagedWriter) Close() error {
	w.manager.RemoveWriter(w)
	return common.Close(w.writer)
}

type LinkManager struct {
	links map[*ManagedWriter]buf.Reader
	mu    sync.Mutex
	key   string
	owner *sync.Map
}

func (m *LinkManager) AddLink(writer *ManagedWriter, reader buf.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owner != nil && len(m.key) > 0 {
		m.owner.Store(m.key, m)
	}
	m.links[writer] = reader
}

func (m *LinkManager) RemoveWriter(writer *ManagedWriter) {
	m.mu.Lock()
	delete(m.links, writer)
	empty := len(m.links) == 0
	m.mu.Unlock()
	if empty && m.owner != nil && len(m.key) > 0 {
		m.owner.CompareAndDelete(m.key, m)
	}
}

func (m *LinkManager) markOwner(key string, owner *sync.Map) {
	m.mu.Lock()
	m.key = key
	m.owner = owner
	m.mu.Unlock()
}

func (m *LinkManager) CloseAll() {
	m.mu.Lock()
	snapshot := make(map[*ManagedWriter]buf.Reader, len(m.links))
	for w, r := range m.links {
		snapshot[w] = r
	}
	m.links = make(map[*ManagedWriter]buf.Reader)
	m.mu.Unlock()
	for w, r := range snapshot {
		common.Close(w)
		common.Interrupt(r)
	}
}
