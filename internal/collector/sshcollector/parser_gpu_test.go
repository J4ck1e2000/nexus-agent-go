package sshcollector

import "testing"

func TestParseGPUQueryOutput_MultiGPU(t *testing.T) {
	snapshot, err := parseSnapshot(loadFixture(t, "snapshot_multi_gpu.txt"))
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}
	if len(snapshot.GPUs) != 4 {
		t.Fatalf("gpu count = %d", len(snapshot.GPUs))
	}

	first := snapshot.GPUs[0]
	if first.Index != 0 || first.Name != "NVIDIA A100-SXM4-80GB" || first.UUID != "GPU-aaaa0000-0000-0000-0000-000000000001" {
		t.Fatalf("first gpu mismatch: %+v", first)
	}
	if first.Temperature != 39 || first.FanSpeed != 0 || first.PowerDraw != 45 || first.Utilization != 0 {
		t.Fatalf("first gpu values mismatch: %+v", first)
	}
	if first.MemoryTotalMB != 81920 || first.MemoryUsedMB != 1024 {
		t.Fatalf("first gpu memory mismatch: %f/%f", first.MemoryTotalMB, first.MemoryUsedMB)
	}

	third := snapshot.GPUs[2]
	if third.PowerDraw != 120 || third.Utilization != 99 {
		t.Fatalf("third gpu values mismatch: %+v", third)
	}
}

func TestParseGPUQueryOutput_NAFields(t *testing.T) {
	snapshot, err := parseSnapshot(loadFixture(t, "snapshot_partial_gpu.txt"))
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}
	if len(snapshot.GPUs) != 1 {
		t.Fatalf("gpu count = %d", len(snapshot.GPUs))
	}
	gpu := snapshot.GPUs[0]
	// [N/A] 值必须安全降级为 0。
	if gpu.FanSpeed != 0 || gpu.PowerDraw != 0 || gpu.Utilization != 0 {
		t.Fatalf("N/A fields should map to 0: %+v", gpu)
	}
	if gpu.Temperature != 55 {
		t.Fatalf("temperature = %d", gpu.Temperature)
	}
}

func TestParseGPUQueryOutput_EmptyOutput(t *testing.T) {
	if gpus := parseGPUQueryOutput(nil); len(gpus) != 0 {
		t.Fatalf("empty output should yield no gpus, got %+v", gpus)
	}
	if gpus := parseGPUQueryOutput([]string{""}); len(gpus) != 0 {
		t.Fatalf("blank output should yield no gpus, got %+v", gpus)
	}
}

func TestParseGPUQueryOutput_MalformedLinesSkipped(t *testing.T) {
	lines := []string{
		"not-enough,fields",
		"abc,GPU-x,Some GPU,40,30,50,10,8192,1024",
		"1,GPU-y,NVIDIA A100,40,30,50,10,8192",
		"2,GPU-z,NVIDIA A100,40,30,50,10,8192,1024",
	}
	gpus := parseGPUQueryOutput(lines)
	if len(gpus) != 1 {
		t.Fatalf("only the well-formed gpu should survive, got %+v", gpus)
	}
	if gpus[0].Index != 2 || gpus[0].UUID != "GPU-z" {
		t.Fatalf("surviving gpu mismatch: %+v", gpus[0])
	}
}

func TestParseGPUQueryOutput_NameWithComma(t *testing.T) {
	lines := []string{
		`0, GPU-comma-01, "NVIDIA GeForce RTX 3090, Rev. 2", 40, 30, 50, 10, 24576, 1024`,
	}
	gpus := parseGPUQueryOutput(lines)
	if len(gpus) != 1 {
		t.Fatalf("quoted gpu line should parse, got %+v", gpus)
	}
	if gpus[0].Name != "NVIDIA GeForce RTX 3090, Rev. 2" {
		t.Fatalf("name with comma mismatch: %q", gpus[0].Name)
	}
}

func TestParseGPUProcessOutput_UnknownUUIDIgnored(t *testing.T) {
	snapshot, err := parseSnapshot(loadFixture(t, "snapshot_multi_gpu.txt"))
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}
	if len(snapshot.GPUProcesses) != 3 {
		t.Fatalf("raw gpu process count = %d", len(snapshot.GPUProcesses))
	}
	// Unknown UUID 在 buildProcesses 阶段被忽略，不影响其余进程。
	uuidToIndex := map[string]int{}
	for _, gpu := range snapshot.GPUs {
		uuidToIndex[gpu.UUID] = gpu.Index
	}
	processes := buildProcesses(snapshot.Processes, snapshot.GPUProcesses, uuidToIndex)

	for _, proc := range processes {
		if proc.PID == 45999 && proc.GPUIndex != nil {
			t.Fatalf("unknown uuid process should not map to a gpu: %+v", proc)
		}
	}
}

func TestParseGPUProcessOutput_EmptyProcessList(t *testing.T) {
	if procs := parseGPUProcessOutput(nil); len(procs) != 0 {
		t.Fatalf("empty output should yield no gpu processes, got %+v", procs)
	}
}
