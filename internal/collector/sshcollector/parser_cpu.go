package sshcollector

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCPUStatInto 解析 /proc/stat 的 cpu 汇总行：
// cpu  user nice system idle iowait irq softirq steal [guest guest_nice]
func parseCPUStatInto(snapshot *RawSnapshot, lines []string) error {
	line := firstNonEmptyLine(lines)
	if line == "" {
		return nil
	}

	fields := strings.Fields(line)
	if len(fields) < 9 || fields[0] != "cpu" {
		return fmt.Errorf("%w: malformed cpu stat line %q", ErrSSHSnapshotInvalid, line)
	}

	values := make([]uint64, 8)
	for i := 0; i < 8; i++ {
		value, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			return fmt.Errorf("%w: parse cpu stat field %q: %v", ErrSSHSnapshotInvalid, fields[i+1], err)
		}
		values[i] = value
	}

	snapshot.CPUStat = CPUStatSnapshot{
		User:    values[0],
		Nice:    values[1],
		System:  values[2],
		Idle:    values[3],
		IOWait:  values[4],
		IRQ:     values[5],
		SoftIRQ: values[6],
		Steal:   values[7],
	}
	return nil
}

// parseCPUModel 提取 "model name : xxx" 中的型号字符串。
func parseCPUModel(lines []string) string {
	line := firstNonEmptyLine(lines)
	if line == "" {
		return ""
	}

	lower := strings.ToLower(line)
	if idx := strings.Index(lower, "model name"); idx >= 0 {
		rest := line[idx+len("model name"):]
		if colon := strings.Index(rest, ":"); colon >= 0 {
			return strings.TrimSpace(rest[colon+1:])
		}
	}
	return strings.TrimSpace(line)
}
