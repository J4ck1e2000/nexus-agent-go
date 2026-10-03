package sshcollector

import (
	"fmt"
	"strconv"
	"strings"
)

// parseMeminfo 解析 /proc/meminfo：
//   - MemTotal 必需，缺失或非法返回错误；
//   - MemAvailable 优先；老内核缺失时回退 MemFree + Buffers + Cached，
//     仍不可得才返回错误。
//
// 禁止用 MemFree 直接当可用内存（page cache 会让占用严重偏高）。
func parseMeminfo(lines []string) (MemorySnapshot, error) {
	values := make(map[string]uint64, 8)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		rest := strings.TrimSpace(line[colon+1:])
		// 值形如 "131949780 kB"，单位固定 kB。
		field := strings.Fields(rest)
		if len(field) == 0 {
			continue
		}
		value, err := strconv.ParseUint(field[0], 10, 64)
		if err != nil {
			continue
		}
		if _, exists := values[key]; !exists {
			values[key] = value
		}
	}

	total, ok := values["MemTotal"]
	if !ok {
		return MemorySnapshot{}, fmt.Errorf("%w: meminfo missing MemTotal", ErrSSHSnapshotInvalid)
	}

	available, ok := values["MemAvailable"]
	if !ok {
		// 老内核（<3.14）没有 MemAvailable，按 MemFree+Buffers+Cached 回退。
		free, freeOK := values["MemFree"]
		buffers, buffersOK := values["Buffers"]
		cached, cachedOK := values["Cached"]
		if freeOK && buffersOK && cachedOK {
			available = free + buffers + cached
		} else {
			return MemorySnapshot{}, fmt.Errorf("%w: meminfo missing MemAvailable", ErrSSHSnapshotInvalid)
		}
	}
	if available > total {
		available = total
	}

	return MemorySnapshot{TotalKB: total, AvailableKB: available}, nil
}
