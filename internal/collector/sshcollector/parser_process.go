package sshcollector

import (
	"strconv"
	"strings"
)

// maxProcessCommandChars 与旧 Agent 的命令行截断长度保持一致。
const maxProcessCommandChars = 60

// parseProcessLines 解析 ps -eo pid=,user=,pcpu=,pmem=,args= 输出。
// args 可能包含空格，前 4 列之外的内容全部视为命令行。
func parseProcessLines(lines []string) []RawProcess {
	processes := make([]RawProcess, 0, len(lines))

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid < 0 {
			continue
		}

		cpuPercent, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			cpuPercent = 0
		}
		memPercent, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			memPercent = 0
		}

		// 命令行按"第 4 列之后的原始文本"还原，保留原始大小写与空格。
		args := extractArgs(line, fields)

		processes = append(processes, RawProcess{
			PID:           pid,
			User:          strings.TrimSpace(fields[1]),
			CPUPercent:    cpuPercent,
			MemoryPercent: memPercent,
			Args:          args,
		})
	}
	return processes
}

// extractArgs 找到第 4 个字段结束的位置，返回其后的原始命令行文本。
func extractArgs(line string, fields []string) string {
	if len(fields) <= 4 {
		return ""
	}

	// 逐字段定位前 4 列在原行中的结束位置（行首可能空白）。
	idx := 0
	for _, field := range fields[:4] {
		next := strings.Index(line[idx:], field)
		if next < 0 {
			return ""
		}
		idx += next + len(field)
	}
	return strings.TrimSpace(line[idx:])
}

// truncateCommand 截断命令行，避免 UI 与 AI 工具输出膨胀。
func truncateCommand(args string) string {
	if args == "" {
		return "unknown"
	}
	if len(args) > maxProcessCommandChars {
		return args[:maxProcessCommandChars-3] + "..."
	}
	return args
}
