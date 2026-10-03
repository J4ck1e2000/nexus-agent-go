package sshcollector

import (
	"errors"
	"testing"
)

func TestParseCPUStatInto(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    CPUStatSnapshot
		wantErr bool
	}{
		{
			name: "full cpu line",
			line: "cpu  1024000 1200 512000 81200000 25600 0 8900 3200 0 0",
			want: CPUStatSnapshot{User: 1024000, Nice: 1200, System: 512000, Idle: 81200000, IOWait: 25600, IRQ: 0, SoftIRQ: 8900, Steal: 3200},
		},
		{
			name: "minimal cpu line without guest fields",
			line: "cpu  10 20 30 40 50 60 70 80",
			want: CPUStatSnapshot{User: 10, Nice: 20, System: 30, Idle: 40, IOWait: 50, IRQ: 60, SoftIRQ: 70, Steal: 80},
		},
		{
			name:    "too few fields",
			line:    "cpu  10 20 30",
			wantErr: true,
		},
		{
			name:    "non numeric field",
			line:    "cpu  10 20 30 abc 50 60 70 80",
			wantErr: true,
		},
		{
			name:    "per cpu line rejected",
			line:    "cpu0 10 20 30 40 50 60 70 80",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot RawSnapshot
			err := parseCPUStatInto(&snapshot, []string{tt.line})
			if tt.wantErr {
				if !errors.Is(err, ErrSSHSnapshotInvalid) {
					t.Fatalf("expected snapshot invalid error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCPUStatInto failed: %v", err)
			}
			if snapshot.CPUStat != tt.want {
				t.Fatalf("cpu stat mismatch: got %+v want %+v", snapshot.CPUStat, tt.want)
			}
		})
	}
}

func TestParseCPUStatInto_EmptySectionKeepsZero(t *testing.T) {
	var snapshot RawSnapshot
	if err := parseCPUStatInto(&snapshot, nil); err != nil {
		t.Fatalf("empty section should be tolerated: %v", err)
	}
	if snapshot.CPUStat != (CPUStatSnapshot{}) {
		t.Fatalf("expected zero cpu stat, got %+v", snapshot.CPUStat)
	}
}

func TestParseCPUModel(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "strips model name prefix",
			lines: []string{"model name\t: AMD EPYC 7313P 16-Core Processor"},
			want:  "AMD EPYC 7313P 16-Core Processor",
		},
		{
			name:  "handles extra spaces",
			lines: []string{"model name   :   Intel(R) Xeon(R) Gold 6330 CPU"},
			want:  "Intel(R) Xeon(R) Gold 6330 CPU",
		},
		{
			name:  "plain line without prefix",
			lines: []string{"Apple M2 Ultra"},
			want:  "Apple M2 Ultra",
		},
		{
			name:  "empty section",
			lines: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCPUModel(tt.lines); got != tt.want {
				t.Fatalf("parseCPUModel = %q want %q", got, tt.want)
			}
		})
	}
}

func TestParseCPUCores(t *testing.T) {
	cores, err := parseCPUCores([]string{"64"})
	if err != nil || cores != 64 {
		t.Fatalf("parseCPUCores = %d, %v", cores, err)
	}

	cores, err = parseCPUCores([]string{"0"})
	if err != nil || cores != 0 {
		t.Fatalf("zero cores = %d, %v", cores, err)
	}

	if _, err := parseCPUCores([]string{"many"}); !errors.Is(err, ErrSSHSnapshotInvalid) {
		t.Fatalf("expected invalid error, got %v", err)
	}
}
