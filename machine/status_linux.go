//go:build linux

package machine

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
)

type statusCollector struct {
	cpuTotal, cpuIdle uint64
	netIn, netOut     uint64
	netAt             time.Time
}

func newStatusCollector() *statusCollector {
	c := &statusCollector{}
	c.cpuTotal, c.cpuIdle = readCPU()
	c.netIn, c.netOut = readNet()
	c.netAt = time.Now()
	return c
}

func (c *statusCollector) collect() *panel.MachineStatus {
	s := &panel.MachineStatus{}

	total, idle := readCPU()
	if dt := total - c.cpuTotal; total > c.cpuTotal {
		s.CPU = float64(dt-(idle-c.cpuIdle)) / float64(dt) * 100
		if s.CPU < 0 {
			s.CPU = 0
		}
	}
	c.cpuTotal, c.cpuIdle = total, idle

	mem := readMeminfo()
	s.Mem = panel.Usage{Total: mem["MemTotal"], Used: sub(mem["MemTotal"], mem["MemAvailable"])}
	s.Swap = panel.Usage{Total: mem["SwapTotal"], Used: sub(mem["SwapTotal"], mem["SwapFree"])}

	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) == nil {
		bs := uint64(fs.Bsize)
		s.Disk = panel.Usage{Total: fs.Blocks * bs, Used: sub(fs.Blocks, fs.Bfree) * bs}
	}

	in, out := readNet()
	now := time.Now()
	if sec := now.Sub(c.netAt).Seconds(); sec > 0 && in >= c.netIn && out >= c.netOut {
		s.Net = &struct {
			InSpeed  float64 `json:"in_speed"`
			OutSpeed float64 `json:"out_speed"`
		}{float64(in-c.netIn) / sec, float64(out-c.netOut) / sec}
	}
	c.netIn, c.netOut, c.netAt = in, out, now
	return s
}

func sub(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return 0
}

func readCPU() (total, idle uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		// guest/guest_nice 已计入 user/nice
		if i >= 8 {
			break
		}
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle
}

// readMeminfo 返回字节
func readMeminfo() map[string]uint64 {
	out := map[string]uint64{}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(fields[1], 10, 64)
		out[strings.TrimSuffix(fields[0], ":")] = n * 1024
	}
	return out
}

func readNet() (in, out uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		in += rx
		out += tx
	}
	return in, out
}
