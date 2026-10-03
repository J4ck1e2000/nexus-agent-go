package sshcollector

import (
	"strconv"
	"strings"
)

// parseNetDev 解析 /proc/net/dev，排除 lo 后累加 RX/TX 字节。
// 输出仍是累计值；MB/s 的速度由 Gateway NodeState 现有逻辑计算。
func parseNetDev(lines []string) NetworkSnapshot {
	var snapshot NetworkSnapshot

	for _, line := range lines {
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" || name == "lo" {
			continue
		}

		fields := strings.Fields(line[colon+1:])
		// RX 8 列 + TX 8 列；至少要有完整的 RX bytes 与 TX bytes。
		if len(fields) < 9 {
			continue
		}
		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		tx, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			continue
		}
		snapshot.RXBytes += rx
		snapshot.TXBytes += tx
	}

	return snapshot
}
