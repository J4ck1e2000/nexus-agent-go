package sshcollector

import (
	"errors"
	"testing"
)

func TestParseMeminfo_WithMemAvailable(t *testing.T) {
	memory, err := parseMeminfo(loadFixtureLines(t, "meminfo.txt"))
	if err != nil {
		t.Fatalf("parseMeminfo failed: %v", err)
	}
	if memory.TotalKB != 263878812 {
		t.Fatalf("total = %d", memory.TotalKB)
	}
	if memory.AvailableKB != 162000320 {
		t.Fatalf("available = %d, want MemAvailable to win over MemFree", memory.AvailableKB)
	}
}

func TestParseMeminfo_FallsBackWhenMemAvailableMissing(t *testing.T) {
	lines := []string{
		"MemTotal:       1000000 kB",
		"MemFree:         200000 kB",
		"Buffers:          50000 kB",
		"Cached:          150000 kB",
	}
	memory, err := parseMeminfo(lines)
	if err != nil {
		t.Fatalf("parseMeminfo failed: %v", err)
	}
	// 200000 + 50000 + 150000 = 400000。
	if memory.AvailableKB != 400000 {
		t.Fatalf("available = %d, want 400000 (MemFree+Buffers+Cached)", memory.AvailableKB)
	}
}

func TestParseMeminfo_MissingMemTotal(t *testing.T) {
	lines := []string{
		"MemFree:         200000 kB",
		"Buffers:          50000 kB",
	}
	if _, err := parseMeminfo(lines); !errors.Is(err, ErrSSHSnapshotInvalid) {
		t.Fatalf("expected snapshot invalid error, got %v", err)
	}
}

func TestParseMeminfo_InvalidValues(t *testing.T) {
	lines := []string{
		"MemTotal: not-a-number kB",
	}
	if _, err := parseMeminfo(lines); !errors.Is(err, ErrSSHSnapshotInvalid) {
		t.Fatalf("expected snapshot invalid error, got %v", err)
	}
}

func TestParseMeminfo_AvailableClampedToTotal(t *testing.T) {
	lines := []string{
		"MemTotal:       1000 kB",
		"MemAvailable:   99999 kB",
	}
	memory, err := parseMeminfo(lines)
	if err != nil {
		t.Fatalf("parseMeminfo failed: %v", err)
	}
	if memory.AvailableKB != memory.TotalKB {
		t.Fatalf("available should clamp to total, got %d/%d", memory.AvailableKB, memory.TotalKB)
	}
}

func TestParseMeminfo_IgnoresMalformedLines(t *testing.T) {
	lines := []string{
		"",
		"no-colon-line",
		": empty key",
		"MemTotal:       1000 kB",
		"MemAvailable:    500 kB",
		"HugePages_Total:       0",
		"VmallocTotal:   34359738367 kB",
	}
	memory, err := parseMeminfo(lines)
	if err != nil {
		t.Fatalf("parseMeminfo failed: %v", err)
	}
	if memory.TotalKB != 1000 {
		t.Fatalf("total = %d", memory.TotalKB)
	}
}
