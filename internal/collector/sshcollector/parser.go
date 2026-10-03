package sshcollector

import (
	"fmt"
	"strconv"
	"strings"
)

// 快照段落标记，与远程脚本中的 echo 一一对应。
const (
	markerHost     = "__NEXUS_HOST__"
	markerOS       = "__NEXUS_OS__"
	markerUptime   = "__NEXUS_UPTIME__"
	markerCPUModel = "__NEXUS_CPU_MODEL__"
	markerCPUCores = "__NEXUS_CPU_CORES__"
	markerCPUStat  = "__NEXUS_CPU_STAT__"
	markerMemInfo  = "__NEXUS_MEMINFO__"
	markerNetDev   = "__NEXUS_NETDEV__"
	markerGPU      = "__NEXUS_GPU__"
	markerGPUProc  = "__NEXUS_GPU_PROC__"
	markerPS       = "__NEXUS_PS__"
	markerEnd      = "__NEXUS_END__"
)

// parseSnapshot 把远程脚本输出按标记切段并解析成 RawSnapshot。
// 结构性错误（缺少结束标记、必需数值段落损坏）返回错误；
// 可选段落（GPU/进程）缺失时保持空值，CPU-only 服务器同样有效。
func parseSnapshot(raw []byte) (*RawSnapshot, error) {
	lines := splitLines(raw)

	sections := make(map[string][]string, 12)
	current := ""
	for _, line := range lines {
		if isMarker(line) {
			current = line
			if _, ok := sections[current]; !ok {
				sections[current] = nil
			}
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}

	if _, ok := sections[markerEnd]; !ok {
		return nil, fmt.Errorf("%w: missing end marker", ErrSSHSnapshotInvalid)
	}

	snapshot := &RawSnapshot{
		Hostname: firstNonEmptyLine(sections[markerHost]),
		OS:       strings.TrimSpace(strings.Join(nonEmptyLines(sections[markerOS]), " ")),
	}

	if err := parseUptimeInto(snapshot, sections[markerUptime]); err != nil {
		return nil, err
	}
	snapshot.CPUModel = parseCPUModel(sections[markerCPUModel])

	cores, err := parseCPUCores(sections[markerCPUCores])
	if err != nil {
		return nil, err
	}
	snapshot.CPUCores = cores

	if err := parseCPUStatInto(snapshot, sections[markerCPUStat]); err != nil {
		return nil, err
	}

	memory, err := parseMeminfo(sections[markerMemInfo])
	if err != nil {
		return nil, err
	}
	snapshot.Memory = memory

	snapshot.Network = parseNetDev(sections[markerNetDev])
	snapshot.GPUs = parseGPUQueryOutput(sections[markerGPU])
	snapshot.GPUProcesses = parseGPUProcessOutput(sections[markerGPUProc])
	snapshot.Processes = parseProcessLines(sections[markerPS])

	return snapshot, nil
}

func isMarker(line string) bool {
	return strings.HasPrefix(line, "__NEXUS_") && strings.HasSuffix(line, "__")
}

func splitLines(raw []byte) []string {
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func nonEmptyLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func firstNonEmptyLine(lines []string) string {
	for _, line := range nonEmptyLines(lines) {
		return line
	}
	return ""
}

// parseUptimeInto 解析 /proc/uptime 第一个字段（秒）。
func parseUptimeInto(snapshot *RawSnapshot, lines []string) error {
	line := firstNonEmptyLine(lines)
	if line == "" {
		return nil
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return fmt.Errorf("%w: parse uptime %q: %v", ErrSSHSnapshotInvalid, fields[0], err)
	}
	if seconds < 0 {
		seconds = 0
	}
	snapshot.UptimeSeconds = seconds
	return nil
}

// parseCPUCores 解析 grep -c '^processor' 的计数输出。
func parseCPUCores(lines []string) (int, error) {
	line := firstNonEmptyLine(lines)
	if line == "" {
		return 0, nil
	}
	cores, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return 0, fmt.Errorf("%w: parse cpu cores %q: %v", ErrSSHSnapshotInvalid, line, err)
	}
	if cores < 0 {
		cores = 0
	}
	return cores, nil
}
