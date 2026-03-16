package agent

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

// round1 将浮点数保留 1 位小数。
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// formatUptime 将秒数转换为可读时长。
func formatUptime(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// readCPUModel 尝试读取 CPU 型号，Linux 优先读取 /proc/cpuinfo。
func readCPUModel() string {
	if runtime.GOOS == "linux" {
		f, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	if runtime.GOARCH != "" {
		return runtime.GOARCH
	}
	return "Unknown CPU"
}

// getOutboundIP 通过 UDP 探测本机对外通信地址。
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	return localAddr.IP.String()
}
