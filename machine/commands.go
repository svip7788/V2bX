package machine

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

//go:embed scripts/bbr.sh
var bbrScript string

const (
	maxCommandOutput = 64 * 1024
	versionFile      = "/etc/V2bX/.panel_version"
)

type command struct {
	ID     string          `json:"id"`
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// 升级/重启作用于整个进程，多站各自的 Manager 共用一把锁
var opBusy atomic.Bool

type commandRunner struct {
	m *Manager
}

func newCommandRunner(m *Manager) *commandRunner {
	return &commandRunner{m: m}
}

func (r *commandRunner) handle(raw json.RawMessage) {
	var cmd command
	if err := json.Unmarshal(raw, &cmd); err != nil || cmd.ID == "" {
		log.WithField("err", err).Warn("WS: invalid machine.command")
		return
	}
	log.WithFields(log.Fields{"id": cmd.ID, "action": cmd.Action}).Info("Machine command received")
	go r.run(cmd)
}

func (r *commandRunner) progress(id, output string) {
	r.m.send("machine.command.progress", map[string]any{"id": id, "output": output})
}

func (r *commandRunner) result(id string, ok bool, output string) {
	r.m.send("machine.command.result", map[string]any{"id": id, "ok": ok, "output": truncateTail(output)})
}

func (r *commandRunner) run(cmd command) {
	defer func() {
		if p := recover(); p != nil {
			r.result(cmd.ID, false, fmt.Sprintf("命令执行异常: %v", p))
		}
	}()
	switch cmd.Action {
	case "status":
		r.result(cmd.ID, true, r.m.statusText())
	case "logs":
		var p struct {
			Lines int `json:"lines"`
		}
		_ = json.Unmarshal(cmd.Params, &p)
		out, err := recentLogs(p.Lines)
		r.result(cmd.ID, err == nil, out)
	case "bbr":
		out, err := runShell(r.m.ctx, bbrScript, 120*time.Second)
		if err != nil {
			out += "\n>>> 失败: " + err.Error()
		}
		r.result(cmd.ID, err == nil, out)
	case "restart":
		if !opBusy.CompareAndSwap(false, true) {
			r.result(cmd.ID, false, "有升级/重启正在进行")
			return
		}
		r.result(cmd.ID, true, "V2bX 即将重启")
		go restartSelf()
	case "upgrade":
		if !opBusy.CompareAndSwap(false, true) {
			r.result(cmd.ID, false, "有升级/重启正在进行")
			return
		}
		// 过程已逐行经 progress 上报，结果只带结论避免日志重复
		restart, msg, err := r.upgrade(cmd)
		if err != nil {
			opBusy.Store(false)
			r.result(cmd.ID, false, ">>> 失败: "+err.Error())
			return
		}
		r.result(cmd.ID, true, msg)
		if restart {
			go restartSelf()
		} else {
			opBusy.Store(false)
		}
	default:
		r.result(cmd.ID, false, "不支持的命令: "+cmd.Action)
	}
}

func (m *Manager) currentVersion() string {
	for _, f := range []string{versionFile, filepath.Join(execDir(), ".version")} {
		if b, err := os.ReadFile(f); err == nil {
			if v := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0]); v != "" {
				return v
			}
		}
	}
	return m.version
}

func (m *Manager) statusText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "版本: %s\n", m.currentVersion())
	fmt.Fprintf(&b, "机器 ID: %d（%s）\n", m.cfg.MachineID, m.cfg.APIHost)
	fmt.Fprintf(&b, "WebSocket: %v\n", m.IsConnected())
	fmt.Fprintf(&b, "内核: %s\n", m.core.Type())
	nodes := m.runningNodeIDs()
	fmt.Fprintf(&b, "运行节点(%d): %s\n", len(nodes), strings.Join(nodes, ", "))
	if _, err := exec.LookPath("systemctl"); err == nil {
		out, _ := runShell(m.ctx, "systemctl status V2bX --no-pager -l 2>&1 | sed -n '1,12p'", 10*time.Second)
		b.WriteString("\n" + out)
	}
	return b.String()
}

func recentLogs(lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 2000 {
		lines = 2000
	}
	if _, err := exec.LookPath("journalctl"); err != nil {
		return "当前系统没有 journalctl，无法读取服务日志", err
	}
	return runShell(context.Background(),
		fmt.Sprintf("journalctl -u V2bX --no-pager -n %d --output cat 2>&1", lines), 20*time.Second)
}

func runShell(ctx context.Context, script string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", script)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return truncateTail(buf.String()), err
}

func truncateTail(s string) string {
	if len(s) <= maxCommandOutput {
		return s
	}
	s = s[len(s)-maxCommandOutput:]
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return "...(已截断)\n" + s
}

func execDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "/usr/local/V2bX"
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

// restartSelf systemd 服务下正常退出即由 Restart=always 拉起；OpenRC 无守护，交给 rc-service
func restartSelf() {
	time.Sleep(time.Second)
	log.Warn("Machine command: restarting V2bX")
	if os.Getenv("INVOCATION_ID") == "" {
		if _, err := exec.LookPath("rc-service"); err == nil {
			c := exec.Command("sh", "-c", "sleep 1; rc-service V2bX restart >/dev/null 2>&1")
			if c.Start() == nil {
				go func() { _ = c.Wait() }()
				return
			}
		}
	}
	if p, err := os.FindProcess(os.Getpid()); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
}
