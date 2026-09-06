package procman

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FreePort finds an unused TCP port inside the configured range by binding
// and immediately releasing it.
func (m *Manager) FreePort() (int, error) {
	return freePortIn(m.PortRange)
}

// freePortIn finds an unused TCP port inside the given range by binding
// and immediately releasing it.
func freePortIn(portRange [2]int) (int, error) {
	lo, hi := portRange[0], portRange[1]
	if lo <= 0 || hi < lo {
		return 0, fmt.Errorf("invalid port range [%d, %d]", lo, hi)
	}
	for p := lo; p <= hi; p++ {
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err != nil {
			continue
		}
		l.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free port in range [%d, %d]", lo, hi)
}

// Healthy reports whether a TCP service answers on host:port.
func Healthy(host string, port int) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 1500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// MemoryKB returns the resident set size of pid in KiB.
func MemoryKB(pid int) (int64, bool) {
	if pid <= 0 {
		return 0, false
	}
	if runtime.GOOS == "linux" {
		if kb, ok := linuxMemoryKB(pid); ok {
			return kb, true
		}
	}
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, false
	}
	kb, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || kb <= 0 {
		return 0, false
	}
	return kb, true
}

func linuxMemoryKB(pid int) (int64, bool) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				return kb, err == nil
			}
		}
	}
	return 0, false
}
