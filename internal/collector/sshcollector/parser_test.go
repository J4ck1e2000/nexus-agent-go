package sshcollector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// loadFixture 读取 testdata/ssh 下的 fixture 文件。
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ssh", name))
	if err != nil {
		t.Fatalf("load fixture %s failed: %v", name, err)
	}
	return data
}

// loadFixtureLines 读取 fixture 并按行拆分，供 []string 参数的解析器使用。
func loadFixtureLines(t *testing.T, name string) []string {
	t.Helper()
	return splitLines(loadFixture(t, name))
}

func TestParseSnapshot_A6000Fixture(t *testing.T) {
	snapshot, err := parseSnapshot(loadFixture(t, "snapshot_a6000.txt"))
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}

	if snapshot.Hostname != "a6000-d-02" {
		t.Fatalf("hostname = %q", snapshot.Hostname)
	}
	if snapshot.OS != "Linux 6.8.0-45-generic" {
		t.Fatalf("os = %q", snapshot.OS)
	}
	if snapshot.UptimeSeconds != 1209600.25 {
		t.Fatalf("uptime = %f", snapshot.UptimeSeconds)
	}
	if snapshot.CPUModel != "AMD EPYC 7313P 16-Core Processor" {
		t.Fatalf("cpu model = %q", snapshot.CPUModel)
	}
	if snapshot.CPUCores != 32 {
		t.Fatalf("cpu cores = %d", snapshot.CPUCores)
	}
	if snapshot.CPUStat.User != 1024000 || snapshot.CPUStat.Idle != 81200000 || snapshot.CPUStat.Steal != 3200 {
		t.Fatalf("cpu stat mismatch: %+v", snapshot.CPUStat)
	}
	if snapshot.Memory.TotalKB != 263878812 || snapshot.Memory.AvailableKB != 162000320 {
		t.Fatalf("memory mismatch: %+v", snapshot.Memory)
	}
	if snapshot.Network.RXBytes != 53687091200 || snapshot.Network.TXBytes != 32212254720 {
		t.Fatalf("network mismatch: %+v", snapshot.Network)
	}
	if len(snapshot.GPUs) != 2 {
		t.Fatalf("gpu count = %d", len(snapshot.GPUs))
	}
	if len(snapshot.GPUProcesses) != 1 || snapshot.GPUProcesses[0].PID != 3017462 || snapshot.GPUProcesses[0].VRAMMB != 42800 {
		t.Fatalf("gpu processes mismatch: %+v", snapshot.GPUProcesses)
	}
	if len(snapshot.Processes) != 5 {
		t.Fatalf("process count = %d", len(snapshot.Processes))
	}
	if snapshot.Processes[2].User != "renhaokun" || snapshot.Processes[2].PID != 3017462 {
		t.Fatalf("process mismatch: %+v", snapshot.Processes[2])
	}
}

func TestParseSnapshot_NoGPUFixture(t *testing.T) {
	snapshot, err := parseSnapshot(loadFixture(t, "snapshot_no_gpu.txt"))
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}
	if len(snapshot.GPUs) != 0 {
		t.Fatalf("expected no gpus, got %d", len(snapshot.GPUs))
	}
	if len(snapshot.GPUProcesses) != 0 {
		t.Fatalf("expected no gpu processes, got %d", len(snapshot.GPUProcesses))
	}
}

func TestParseSnapshot_MissingEndMarker(t *testing.T) {
	output := []byte("__NEXUS_HOST__\nsome-host\n")
	if _, err := parseSnapshot(output); !errors.Is(err, ErrSSHSnapshotInvalid) {
		t.Fatalf("expected snapshot invalid error, got %v", err)
	}
}

func TestParseSnapshot_EmptyOutput(t *testing.T) {
	if _, err := parseSnapshot(nil); !errors.Is(err, ErrSSHSnapshotInvalid) {
		t.Fatalf("expected snapshot invalid error, got %v", err)
	}
}

func TestParseSnapshot_MalformedMeminfo(t *testing.T) {
	output := []byte(`__NEXUS_HOST__
host
__NEXUS_MEMINFO__
garbage line without total
__NEXUS_END__
`)
	if _, err := parseSnapshot(output); !errors.Is(err, ErrSSHSnapshotInvalid) {
		t.Fatalf("expected snapshot invalid error, got %v", err)
	}
}

func TestParseSnapshot_CRLFOutput(t *testing.T) {
	output := []byte("__NEXUS_HOST__\r\ncrlf-host\r\n__NEXUS_CPU_CORES__\r\n8\r\n__NEXUS_MEMINFO__\r\nMemTotal: 1000 kB\r\nMemAvailable: 500 kB\r\n__NEXUS_END__\r\n")
	snapshot, err := parseSnapshot(output)
	if err != nil {
		t.Fatalf("parseSnapshot failed: %v", err)
	}
	if snapshot.Hostname != "crlf-host" || snapshot.CPUCores != 8 {
		t.Fatalf("crlf snapshot mismatch: %+v", snapshot)
	}
}
