package sshcollector

import "testing"

func TestParseProcessLines_NormalAndSpacesInCommand(t *testing.T) {
	lines := []string{
		"   31337 dev.gpu 145.2 24.5 python benchmark.py --gpu all --steps 10000",
		"      1 root  0.0  0.0 /sbin/init splash",
		"   45123 zhang.san 212.3 64.1 python infer.py --model llm-70b",
	}
	processes := parseProcessLines(lines)
	if len(processes) != 3 {
		t.Fatalf("process count = %d, processes=%+v", len(processes), processes)
	}

	first := processes[0]
	if first.PID != 31337 || first.User != "dev.gpu" || first.CPUPercent != 145.2 || first.MemoryPercent != 24.5 {
		t.Fatalf("first process mismatch: %+v", first)
	}
	if first.Args != "python benchmark.py --gpu all --steps 10000" {
		t.Fatalf("args with spaces mismatch: %q", first.Args)
	}
}

func TestParseProcessLines_MalformedSkipped(t *testing.T) {
	lines := []string{
		"",
		"too few fields",
		"abc root 1.0 1.0 pid-not-a-number",
		"   123 root 1.0 1.0 valid process",
	}
	processes := parseProcessLines(lines)
	if len(processes) != 1 || processes[0].PID != 123 {
		t.Fatalf("malformed lines should be skipped: %+v", processes)
	}
}

func TestBuildProcesses_RetentionAndSortRules(t *testing.T) {
	snapshot, err := parseSnapshot(loadFixture(t, "snapshot_processes.txt"))
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}

	uuidToIndex := map[string]int{"GPU-eeee0000-0000-0000-0000-000000000001": 0}
	processes := buildProcesses(snapshot.Processes, snapshot.GPUProcesses, uuidToIndex)

	// GPU 进程(31337)始终保留；非 GPU 仅 CPU > 2%；43000 恰好 2.0 被排除。
	// 保留: 31337(145.2 GPU) 41000(96.4) 48000(12.0) 46000(8.8) 42000(5.0) 49000(3.9)。
	if len(processes) != 6 {
		t.Fatalf("process count = %d, processes=%+v", len(processes), processes)
	}
	if processes[0].PID != 31337 || processes[0].GPUIndex == nil || *processes[0].GPUIndex != 0 {
		t.Fatalf("gpu process should sort first: %+v", processes[0])
	}
	if processes[0].VRAMUsedMB == nil || *processes[0].VRAMUsedMB != 6100 {
		t.Fatalf("gpu process vram mismatch: %+v", processes[0])
	}
	for i := 1; i < len(processes); i++ {
		if processes[i].GPUIndex != nil {
			t.Fatalf("unexpected gpu process at %d: %+v", i, processes[i])
		}
		if processes[i-1].CPUPercent < processes[i].CPUPercent {
			t.Fatalf("non-gpu processes should be cpu descending: %+v", processes)
		}
	}
}

func TestBuildProcesses_Top10Limit(t *testing.T) {
	raw := make([]RawProcess, 0, 12)
	for i := 0; i < 12; i++ {
		// CPU 全部 > 2%，确保截断发生在 top-10 而不是保留规则。
		raw = append(raw, RawProcess{
			PID:        1000 + i,
			User:       "user",
			CPUPercent: float64(i) + 3,
			Args:       "python job.py",
		})
	}

	processes := buildProcesses(raw, nil, map[string]int{})
	if len(processes) != 10 {
		t.Fatalf("expected top 10, got %d", len(processes))
	}
	if processes[0].CPUPercent != 14 {
		t.Fatalf("highest cpu should be first, got %+v", processes[0])
	}
}

func TestBuildProcesses_CommandTruncation(t *testing.T) {
	raw := []RawProcess{{
		PID:        1,
		User:       "user",
		CPUPercent: 50,
		Args:       "python verylongcommand.py --argument-one value --argument-two value --argument-three value",
	}}
	processes := buildProcesses(raw, nil, map[string]int{})
	if len(processes) != 1 {
		t.Fatalf("process count = %d", len(processes))
	}
	cmd := processes[0].Command
	if len(cmd) > 60 {
		t.Fatalf("command too long: %q (%d)", cmd, len(cmd))
	}
	if cmd != "python verylongcommand.py --argument-one value --argument..." {
		t.Fatalf("truncation mismatch: %q", cmd)
	}
}

func TestBuildProcesses_EmptyArgsFallback(t *testing.T) {
	raw := []RawProcess{{PID: 2, User: "root", CPUPercent: 3.0, Args: ""}}
	processes := buildProcesses(raw, nil, map[string]int{})
	if len(processes) != 1 || processes[0].Command != "unknown" {
		t.Fatalf("empty args should fall back to unknown: %+v", processes)
	}
}

func TestBuildProcesses_GPUProcAlwaysKeptEvenAtZeroCPU(t *testing.T) {
	raw := []RawProcess{{PID: 77, User: "lab", CPUPercent: 0.0, Args: "python idle_gpu_proc.py"}}
	gpuProcs := []RawGPUProcess{{PID: 77, UUID: "GPU-1", VRAMMB: 1024}}
	processes := buildProcesses(raw, gpuProcs, map[string]int{"GPU-1": 1})
	if len(processes) != 1 {
		t.Fatalf("gpu process must always be kept: %+v", processes)
	}
	if processes[0].GPUIndex == nil || *processes[0].GPUIndex != 1 {
		t.Fatalf("gpu index mismatch: %+v", processes[0])
	}
}

func TestBuildProcesses_UserFallbackToSystem(t *testing.T) {
	raw := []RawProcess{{PID: 9, User: "", CPUPercent: 10.0, Args: "kernel-ish task"}}
	processes := buildProcesses(raw, nil, map[string]int{})
	if len(processes) != 1 || processes[0].User != "system" {
		t.Fatalf("empty user should fall back to system: %+v", processes)
	}
}

func TestTruncateCommand(t *testing.T) {
	if got := truncateCommand("short cmd"); got != "short cmd" {
		t.Fatalf("short command changed: %q", got)
	}
	if got := truncateCommand(""); got != "unknown" {
		t.Fatalf("empty command fallback: %q", got)
	}
}
