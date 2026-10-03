package sshcollector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus-agent-go/internal/model"
)

// buildFixtureWithCPUStat 将 fixture 的 cpu stat 行替换为给定内容。
func buildFixtureWithCPUStat(t *testing.T, fixture string, cpuLine string) []byte {
	t.Helper()
	original := string(loadFixture(t, fixture))
	replaced := strings.Replace(original, "cpu  1024000 1200 512000 81200000 25600 0 8900 3200 0 0", cpuLine, 1)
	if replaced == original {
		t.Fatalf("cpu stat line not found in fixture %s", fixture)
	}
	return []byte(replaced)
}

// fakeExecutor 用 fixture 直接应答 Run，测试完全不依赖真实 SSH 服务。
type fakeExecutor struct {
	mu      sync.Mutex
	outputs [][]byte
	errs    []error
	calls   int
	nodes   []SSHNodeConfig
	scripts []string
}

func (f *fakeExecutor) Run(ctx context.Context, node SSHNodeConfig, script string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.nodes = append(f.nodes, node)
	f.scripts = append(f.scripts, script)

	idx := f.calls - 1
	if idx < len(f.errs) && f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	if idx < len(f.outputs) {
		return f.outputs[idx], nil
	}
	return nil, errors.New("fake executor exhausted")
}

func newSSHTestNode() model.AgentConfig {
	return model.AgentConfig{
		ID:            42,
		Name:          "A6000-01",
		CollectorType: model.CollectorTypeSSH,
		SSHHost:       "10.0.0.15",
		SSHPort:       22,
		SSHUser:       "renhaokun",
	}
}

func TestSSHCollector_CollectBuildsSystemMetrics(t *testing.T) {
	fake := &fakeExecutor{
		outputs: [][]byte{loadFixture(t, "snapshot_a6000.txt")},
	}
	now := time.Unix(1710000600, 0)
	collector := New(fake, Options{CommandTimeout: 5 * time.Second})
	collector.nowFunc = func() time.Time { return now }

	metrics, collectedAt, err := collector.Collect(context.Background(), newSSHTestNode())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if collectedAt != now.Unix() {
		t.Fatalf("collectedAt = %d, want %d", collectedAt, now.Unix())
	}
	if metrics.Hostname != "a6000-d-02" {
		t.Fatalf("hostname = %q", metrics.Hostname)
	}
	if metrics.IPAddress != "10.0.0.15" {
		t.Fatalf("ip = %q, want ssh host", metrics.IPAddress)
	}
	if metrics.OS != "Linux 6.8.0-45-generic" {
		t.Fatalf("os = %q", metrics.OS)
	}
	if metrics.UptimeSeconds != 1209600.25 || metrics.UptimeHuman != "14d 0h 0m" {
		t.Fatalf("uptime mismatch: %f %q", metrics.UptimeSeconds, metrics.UptimeHuman)
	}
	if metrics.CPUModel != "AMD EPYC 7313P 16-Core Processor" || metrics.CPUCores != 32 {
		t.Fatalf("cpu mismatch: %q %d", metrics.CPUModel, metrics.CPUCores)
	}
	// 首次采样 CPU 为 0（无前值）。
	if metrics.CPUUsage != 0 {
		t.Fatalf("first sample cpu should be 0, got %f", metrics.CPUUsage)
	}
	if metrics.RAMTotal != 251.7 || metrics.RAMUsed != 97.2 || metrics.RAMPercent != 38.6 {
		t.Fatalf("ram mismatch: total=%f used=%f percent=%f", metrics.RAMTotal, metrics.RAMUsed, metrics.RAMPercent)
	}
	if metrics.NetRecvMB != 51200 || metrics.NetSentMB != 30720 {
		t.Fatalf("net mismatch: recv=%f sent=%f", metrics.NetRecvMB, metrics.NetSentMB)
	}
	if len(metrics.Gpus) != 2 {
		t.Fatalf("gpu count = %d", len(metrics.Gpus))
	}
	gpu0 := metrics.Gpus[0]
	if gpu0.MemoryTotal != 48 || gpu0.MemoryUsed != 8 || gpu0.MemoryUtilization != 16.7 {
		t.Fatalf("gpu0 memory mismatch: %+v", gpu0)
	}
	if gpu0.PowerDraw != 23 {
		t.Fatalf("gpu0 power should round from 23.05, got %d", gpu0.PowerDraw)
	}
	if len(metrics.Processes) != 1 {
		t.Fatalf("process count = %d", len(metrics.Processes))
	}
	proc := metrics.Processes[0]
	if proc.PID != 3017462 || proc.GPUIndex == nil || *proc.GPUIndex != 1 || proc.VRAMUsedMB == nil || *proc.VRAMUsedMB != 42800 {
		t.Fatalf("gpu process mapping mismatch: %+v", proc)
	}
}

func TestSSHCollector_SecondSampleComputesCPUDelta(t *testing.T) {
	fake := &fakeExecutor{
		outputs: [][]byte{
			loadFixture(t, "snapshot_a6000.txt"),
			buildFixtureWithCPUStat(t, "snapshot_a6000.txt", "cpu  1024300 1200 512200 81200100 25650 0 8910 3210 0 0"),
		},
	}
	collector := New(fake, Options{})

	node := newSSHTestNode()
	if _, _, err := collector.Collect(context.Background(), node); err != nil {
		t.Fatalf("first Collect failed: %v", err)
	}
	metrics, _, err := collector.Collect(context.Background(), node)
	if err != nil {
		t.Fatalf("second Collect failed: %v", err)
	}

	// total delta 670, idle delta 150 → (670-150)/670 = 77.6%。
	if metrics.CPUUsage != 77.6 {
		t.Fatalf("cpu usage = %f, want 77.6", metrics.CPUUsage)
	}
}

func TestSSHCollector_CPUCounterResetReturnsZero(t *testing.T) {
	fake := &fakeExecutor{
		outputs: [][]byte{
			loadFixture(t, "snapshot_a6000.txt"),
			// 计数回退（模拟重启）：total 小于上一轮。
			buildFixtureWithCPUStat(t, "snapshot_a6000.txt", "cpu  100 10 50 4000 10 0 1 1 0 0"),
		},
	}
	collector := New(fake, Options{})

	node := newSSHTestNode()
	if _, _, err := collector.Collect(context.Background(), node); err != nil {
		t.Fatalf("first Collect failed: %v", err)
	}
	metrics, _, err := collector.Collect(context.Background(), node)
	if err != nil {
		t.Fatalf("second Collect failed: %v", err)
	}
	if metrics.CPUUsage != 0 {
		t.Fatalf("counter reset should yield 0, got %f", metrics.CPUUsage)
	}
}

func TestSSHCollector_CPUZeroDeltaReturnsZero(t *testing.T) {
	fake := &fakeExecutor{
		// 两轮完全相同的累计计数 → 零差分。
		outputs: [][]byte{
			loadFixture(t, "snapshot_a6000.txt"),
			loadFixture(t, "snapshot_a6000.txt"),
		},
	}
	collector := New(fake, Options{})

	node := newSSHTestNode()
	if _, _, err := collector.Collect(context.Background(), node); err != nil {
		t.Fatalf("first Collect failed: %v", err)
	}
	metrics, _, err := collector.Collect(context.Background(), node)
	if err != nil {
		t.Fatalf("second Collect failed: %v", err)
	}
	if metrics.CPUUsage != 0 {
		t.Fatalf("zero delta should yield 0, got %f", metrics.CPUUsage)
	}
}

func TestSSHCollector_RemoteScriptIsFixed(t *testing.T) {
	fake := &fakeExecutor{
		outputs: [][]byte{loadFixture(t, "snapshot_a6000.txt")},
	}
	collector := New(fake, Options{})

	// 节点名称包含特殊字符，若脚本被拼接用户输入此处会暴露。
	node := newSSHTestNode()
	node.Name = "evil'; rm -rf /"
	if _, _, err := collector.Collect(context.Background(), node); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	fake.mu.Lock()
	script := fake.scripts[0]
	fake.mu.Unlock()
	if script != remoteMetricsScript {
		t.Fatalf("remote script must be the fixed snapshot script")
	}
}

func TestSSHCollector_ExecutorErrorPropagates(t *testing.T) {
	fake := &fakeExecutor{
		errs: []error{fmt.Errorf("%w: dial failed", ErrSSHConnectFailed)},
	}
	collector := New(fake, Options{})

	_, _, err := collector.Collect(context.Background(), newSSHTestNode())
	if !errors.Is(err, ErrSSHConnectFailed) {
		t.Fatalf("expected connect failed, got %v", err)
	}
}

func TestSSHCollector_NoGPUYieldsEmptySlice(t *testing.T) {
	fake := &fakeExecutor{
		outputs: [][]byte{loadFixture(t, "snapshot_no_gpu.txt")},
	}
	collector := New(fake, Options{})

	metrics, _, err := collector.Collect(context.Background(), newSSHTestNode())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if metrics.Gpus == nil || len(metrics.Gpus) != 0 {
		t.Fatalf("gpus should be non-nil empty slice, got %+v", metrics.Gpus)
	}
	if len(metrics.Processes) != 1 {
		t.Fatalf("cpu>2 process should be kept: %+v", metrics.Processes)
	}
}

func TestSSHCollector_TestSSH(t *testing.T) {
	fake := &fakeExecutor{
		outputs: [][]byte{loadFixture(t, "snapshot_a6000.txt")},
	}
	collector := New(fake, Options{})

	result, err := collector.TestSSH(context.Background(), "10.0.0.15", 22, "renhaokun")
	if err != nil {
		t.Fatalf("TestSSH failed: %v", err)
	}
	if result.Hostname != "a6000-d-02" || result.GPUCount != 2 {
		t.Fatalf("test ssh result mismatch: %+v", result)
	}
	if len(result.GPUNames) != 2 || result.GPUNames[0] != "NVIDIA RTX A6000" {
		t.Fatalf("gpu names mismatch: %+v", result.GPUNames)
	}

	fake.mu.Lock()
	node := fake.nodes[0]
	fake.mu.Unlock()
	if node.Host != "10.0.0.15" || node.Port != 22 || node.User != "renhaokun" {
		t.Fatalf("test ssh should reuse the same connection target: %+v", node)
	}
}

func TestSSHCollector_HostnameFallsBackToNodeName(t *testing.T) {
	output := []byte(strings.ReplaceAll(string(loadFixture(t, "snapshot_a6000.txt")), "a6000-d-02", ""))
	fake := &fakeExecutor{outputs: [][]byte{output}}
	collector := New(fake, Options{})

	metrics, _, err := collector.Collect(context.Background(), newSSHTestNode())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if metrics.Hostname != "A6000-01" {
		t.Fatalf("hostname should fall back to node name, got %q", metrics.Hostname)
	}
}

func TestErrorCode(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{ErrSSHConnectFailed, "ssh_connect_failed"},
		{fmt.Errorf("%w: dial timeout", ErrSSHConnectFailed), "ssh_connect_failed"},
		{ErrSSHAuthFailed, "ssh_auth_failed"},
		{ErrSSHHostKeyFailed, "ssh_host_key_failed"},
		{ErrSSHCommandTimeout, "ssh_command_timeout"},
		{ErrSSHCommandFailed, "ssh_command_failed"},
		{ErrSSHOutputTooLarge, "ssh_output_too_large"},
		{ErrSSHSnapshotInvalid, "ssh_snapshot_invalid"},
		{ErrEncryptedPrivateKey, "encrypted_private_key_not_supported"},
		{errors.New("mystery"), "ssh_metrics_failed"},
	}
	for _, tt := range tests {
		if got := ErrorCode(tt.err); got != tt.want {
			t.Fatalf("ErrorCode(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestLoadOptionsFromEnv(t *testing.T) {
	t.Setenv("SSH_PRIVATE_KEY_PATH", "/run/secrets/nexus_ssh_key")
	t.Setenv("SSH_KNOWN_HOSTS_PATH", "/app/config/known_hosts")
	t.Setenv("SSH_CONNECT_TIMEOUT_SEC", "9")
	t.Setenv("SSH_COMMAND_TIMEOUT_SEC", "7")
	t.Setenv("SSH_KEEPALIVE_SEC", "30")

	opts := LoadOptionsFromEnv()
	if opts.PrivateKeyPath != "/run/secrets/nexus_ssh_key" || opts.KnownHostsPath != "/app/config/known_hosts" {
		t.Fatalf("paths mismatch: %+v", opts)
	}
	if opts.ConnectTimeout != 9*time.Second || opts.CommandTimeout != 7*time.Second || opts.KeepAliveInterval != 30*time.Second {
		t.Fatalf("timeouts mismatch: %+v", opts)
	}
	if opts.MaxOutputBytes != maxMetricsOutputBytes {
		t.Fatalf("output limit mismatch: %d", opts.MaxOutputBytes)
	}
}

func TestLoadOptionsFromEnv_Defaults(t *testing.T) {
	opts := LoadOptionsFromEnv()
	if opts.ConnectTimeout != 5*time.Second || opts.CommandTimeout != 5*time.Second || opts.KeepAliveInterval != 15*time.Second {
		t.Fatalf("defaults mismatch: %+v", opts)
	}
}

func TestComputeCPUUsage_WithProcStatFixtures(t *testing.T) {
	collector := New(&fakeExecutor{}, Options{})

	stat1, err := parseCPUStatLine(t, "proc_stat_1.txt")
	if err != nil {
		t.Fatalf("parse first stat failed: %v", err)
	}
	if usage := collector.computeCPUUsage(1, stat1); usage != 0 {
		t.Fatalf("first sample should be 0, got %f", usage)
	}

	stat2, err := parseCPUStatLine(t, "proc_stat_2.txt")
	if err != nil {
		t.Fatalf("parse second stat failed: %v", err)
	}
	// total delta 616, idle delta 210 → (616-210)/616 = 65.9%。
	if usage := collector.computeCPUUsage(1, stat2); usage != 65.9 {
		t.Fatalf("cpu usage = %f, want 65.9", usage)
	}
	// 不同节点计数互不影响。
	if usage := collector.computeCPUUsage(2, stat2); usage != 0 {
		t.Fatalf("other node first sample should be 0, got %f", usage)
	}
}

// parseCPUStatLine 从 fixture 中解析出 CPUStatSnapshot。
func parseCPUStatLine(t *testing.T, fixture string) (CPUStatSnapshot, error) {
	t.Helper()
	var snapshot RawSnapshot
	if err := parseCPUStatInto(&snapshot, loadFixtureLines(t, fixture)); err != nil {
		return CPUStatSnapshot{}, err
	}
	return snapshot.CPUStat, nil
}
