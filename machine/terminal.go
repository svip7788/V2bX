package machine

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	log "github.com/sirupsen/logrus"
)

const (
	maxTermSessions = 4
	termIdleTimeout = 30 * time.Minute
	termReadBuf     = 32 * 1024
)

// 面板网页终端：面板经机器 WS 转发浏览器输入，这里用 PTY 起一个登录 shell
type terminals struct {
	m        *Manager
	mu       sync.Mutex
	sessions map[string]*termSession
}

type termSession struct {
	id   string
	cmd  *exec.Cmd
	pty  *os.File
	last atomic.Int64
	once sync.Once
}

type termMsg struct {
	SID  string `json:"sid"`
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func newTerminals(m *Manager) *terminals {
	t := &terminals{m: m, sessions: make(map[string]*termSession)}
	go t.reapIdle()
	return t
}

// handle 在 WS 读协程里直接调用，不能阻塞
func (t *terminals) handle(event string, raw json.RawMessage) bool {
	var msg termMsg
	switch event {
	case "machine.term.open", "machine.term.input", "machine.term.resize", "machine.term.close":
	default:
		return false
	}
	if err := json.Unmarshal(raw, &msg); err != nil || msg.SID == "" {
		return true
	}
	switch event {
	case "machine.term.open":
		go t.open(msg)
	case "machine.term.input":
		if s := t.get(msg.SID); s != nil {
			s.last.Store(time.Now().Unix())
			_, _ = s.pty.Write([]byte(msg.Data))
		}
	case "machine.term.resize":
		if s := t.get(msg.SID); s != nil && msg.Cols > 0 && msg.Rows > 0 {
			_ = pty.Setsize(s.pty, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
		}
	case "machine.term.close":
		if s := t.get(msg.SID); s != nil {
			t.close(s, "")
		}
	}
	return true
}

func (t *terminals) get(id string) *termSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessions[id]
}

func (t *terminals) open(msg termMsg) {
	t.mu.Lock()
	if _, ok := t.sessions[msg.SID]; ok {
		t.mu.Unlock()
		return
	}
	if len(t.sessions) >= maxTermSessions {
		t.mu.Unlock()
		t.m.send("machine.term.exit", map[string]any{"sid": msg.SID, "reason": "终端会话数已达上限"})
		return
	}
	t.mu.Unlock()

	shell := "/bin/sh"
	if p, err := exec.LookPath("bash"); err == nil {
		shell = p
	}
	// 放到独立 scope：否则 shell 处在 V2bX.service 的 cgroup 里，systemctl status 会混入终端进程，重启 V2bX 也会连带杀掉终端里启动的进程
	cmd := exec.Command(shell, "-l")
	if run, err := exec.LookPath("systemd-run"); err == nil {
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			cmd = exec.Command(run, "--scope", "--quiet", shell, "-l")
		}
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "HOME="+home, "LANG=C.UTF-8")
	cols, rows := msg.Cols, msg.Rows
	if cols == 0 || rows == 0 {
		cols, rows = 120, 32
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		t.m.send("machine.term.exit", map[string]any{"sid": msg.SID, "reason": "启动终端失败: " + err.Error()})
		return
	}
	s := &termSession{id: msg.SID, cmd: cmd, pty: f}
	s.last.Store(time.Now().Unix())
	t.mu.Lock()
	t.sessions[s.id] = s
	t.mu.Unlock()
	log.WithField("sid", s.id).Info("Terminal session opened")

	buf := make([]byte, termReadBuf)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			t.m.send("machine.term.output", map[string]any{"sid": s.id, "data": base64.StdEncoding.EncodeToString(buf[:n])})
		}
		if err != nil {
			t.close(s, "")
			return
		}
	}
}

func (t *terminals) close(s *termSession, reason string) {
	s.once.Do(func() {
		t.mu.Lock()
		delete(t.sessions, s.id)
		t.mu.Unlock()
		_ = s.pty.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		go func() { _ = s.cmd.Wait() }()
		t.m.send("machine.term.exit", map[string]any{"sid": s.id, "reason": reason})
		log.WithField("sid", s.id).Info("Terminal session closed")
	})
}

func (t *terminals) closeAll() {
	t.mu.Lock()
	list := make([]*termSession, 0, len(t.sessions))
	for _, s := range t.sessions {
		list = append(list, s)
	}
	t.mu.Unlock()
	for _, s := range list {
		t.close(s, "V2bX 正在退出")
	}
}

func (t *terminals) reapIdle() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-t.m.ctx.Done():
			t.closeAll()
			return
		case <-ticker.C:
		}
		deadline := time.Now().Add(-termIdleTimeout).Unix()
		t.mu.Lock()
		var idle []*termSession
		for _, s := range t.sessions {
			if s.last.Load() < deadline {
				idle = append(idle, s)
			}
		}
		t.mu.Unlock()
		for _, s := range idle {
			t.close(s, "空闲超过 30 分钟，已自动断开")
		}
	}
}
