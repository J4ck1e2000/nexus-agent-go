// Package sshcollector 实现 agentless 的 SSH 指标采集：
// Gateway 通过 SSH 在目标 Linux 服务器上执行固定脚本，
// 从 /proc、ps、nvidia-smi 读取指标并转换为 model.SystemMetrics。
package sshcollector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"nexus-agent-go/internal/model"
)

// maxMetricsOutputBytes 限制远程脚本 stdout 大小，防止异常输出拖垮 Gateway。
const maxMetricsOutputBytes = 2 * 1024 * 1024

// Options 描述 SSH 采集器的运行配置。
type Options struct {
	// PrivateKeyPath 为 Gateway 侧私钥路径（永不入库/下发）。
	PrivateKeyPath string
	// KnownHostsPath 为 known_hosts 路径，强制开启主机指纹校验。
	KnownHostsPath string
	// ConnectTimeout 限制 TCP 连接 + SSH 握手时长。
	ConnectTimeout time.Duration
	// CommandTimeout 限制单次远程脚本执行时长。
	CommandTimeout time.Duration
	// KeepAliveInterval 为连接 keepalive 周期，<=0 关闭。
	KeepAliveInterval time.Duration
	// MaxOutputBytes 限制远程输出大小，<=0 使用默认 2MB。
	MaxOutputBytes int64
}

// LoadOptionsFromEnv 从环境变量读取 SSH 采集配置。
func LoadOptionsFromEnv() Options {
	return Options{
		PrivateKeyPath:    strings.TrimSpace(os.Getenv("SSH_PRIVATE_KEY_PATH")),
		KnownHostsPath:    strings.TrimSpace(os.Getenv("SSH_KNOWN_HOSTS_PATH")),
		ConnectTimeout:    envSeconds("SSH_CONNECT_TIMEOUT_SEC", 5*time.Second),
		CommandTimeout:    envSeconds("SSH_COMMAND_TIMEOUT_SEC", 5*time.Second),
		KeepAliveInterval: envSeconds("SSH_KEEPALIVE_SEC", 15*time.Second),
		MaxOutputBytes:    maxMetricsOutputBytes,
	}
}

func envSeconds(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

// cpuSample 缓存上一轮 CPU 累计计数，用于差分计算占用率。
type cpuSample struct {
	total uint64
	idle  uint64
}

// Collector 是 agentless SSH 指标采集器，实现 collector.NodeMetricsCollector。
type Collector struct {
	executor CommandExecutor
	options  Options
	nowFunc  func() time.Time

	mu      sync.Mutex
	prevCPU map[int64]cpuSample
	pool    *ConnectionPool
}

// New 创建采集器；executor 由调用方注入（生产为 SSHCommandExecutor，
// 测试可注入 FakeCommandExecutor）。
func New(executor CommandExecutor, opts Options) *Collector {
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = maxMetricsOutputBytes
	}
	return &Collector{
		executor: executor,
		options:  opts,
		nowFunc:  time.Now,
		prevCPU:  make(map[int64]cpuSample),
	}
}

// NewWithOptionsFromEnv 按环境变量构建生产采集器（含连接池与执行器）。
// 未设置 SSH_PRIVATE_KEY_PATH 时返回 ErrSSHNotConfigured。
func NewWithOptionsFromEnv(opts Options) (*Collector, error) {
	if strings.TrimSpace(opts.PrivateKeyPath) == "" {
		return nil, ErrSSHNotConfigured
	}
	if strings.TrimSpace(opts.KnownHostsPath) == "" {
		return nil, fmt.Errorf("ssh collector requires SSH_KNOWN_HOSTS_PATH: %w", ErrSSHNotConfigured)
	}

	signer, err := loadPrivateKey(opts.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	hostKeyCallback, err := knownhosts.New(opts.KnownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts failed: %v", err)
	}

	connectTimeout := opts.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}
	clientConfig := &ssh.ClientConfig{
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         connectTimeout,
	}

	pool := NewConnectionPool(productionDial(clientConfig), opts.KeepAliveInterval)
	executor := NewSSHCommandExecutor(pool, opts.CommandTimeout, opts.MaxOutputBytes)
	sshCollector := New(executor, opts)
	sshCollector.pool = pool
	return sshCollector, nil
}

// loadPrivateKey 读取并解析 Gateway 侧 SSH 私钥。
// 加密私钥第一阶段明确拒绝，不做交互式 passphrase。
func loadPrivateKey(path string) (ssh.Signer, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read private key failed: %v", ErrSSHConnectFailed, err)
	}
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		var passErr *ssh.PassphraseMissingError
		if errors.As(err, &passErr) {
			return nil, fmt.Errorf("%w", ErrEncryptedPrivateKey)
		}
		return nil, fmt.Errorf("%w: parse private key failed: %v", ErrSSHConnectFailed, err)
	}
	return signer, nil
}

// Pool 返回底层连接池（Gateway 关停时调用 CloseAll）。
func (c *Collector) Pool() *ConnectionPool { return c.pool }

// OptionsSummary 返回连接/命令超时与 keepalive 周期，用于启动日志。
func (c *Collector) OptionsSummary() (connectTimeout, commandTimeout, keepalive time.Duration) {
	return c.options.ConnectTimeout, c.options.CommandTimeout, c.options.KeepAliveInterval
}

// Close 释放全部 SSH 连接。
func (c *Collector) Close() {
	if c.pool != nil {
		c.pool.CloseAll()
	}
}

// Collect 采集一次节点指标并转换为 SystemMetrics。
func (c *Collector) Collect(ctx context.Context, node model.AgentConfig) (model.SystemMetrics, int64, error) {
	out, err := c.executor.Run(ctx, SSHNodeConfig{
		Host: node.SSHHost,
		Port: node.SSHPort,
		User: node.SSHUser,
	}, remoteMetricsScript)
	if err != nil {
		return model.SystemMetrics{}, 0, err
	}

	snapshot, err := parseSnapshot(out)
	if err != nil {
		return model.SystemMetrics{}, 0, err
	}

	metrics := c.buildMetrics(node, snapshot)
	return metrics, c.nowFunc().Unix(), nil
}

// TestSSHResult 是 SSH 连接测试的结果摘要。
type TestSSHResult struct {
	Hostname string   `json:"hostname"`
	GPUCount int      `json:"gpu_count"`
	GPUNames []string `json:"gpu_names"`
}

// TestSSH 执行一次完整采集并抽取连接测试所需字段，供 test-ssh API 使用。
func (c *Collector) TestSSH(ctx context.Context, host string, port int, user string) (TestSSHResult, error) {
	node := model.AgentConfig{
		CollectorType: model.CollectorTypeSSH,
		SSHHost:       host,
		SSHPort:       port,
		SSHUser:       user,
	}
	metrics, _, err := c.Collect(ctx, node)
	if err != nil {
		return TestSSHResult{}, err
	}

	names := make([]string, 0, len(metrics.Gpus))
	for _, gpu := range metrics.Gpus {
		names = append(names, gpu.Name)
	}
	return TestSSHResult{
		Hostname: metrics.Hostname,
		GPUCount: len(metrics.Gpus),
		GPUNames: names,
	}, nil
}

// buildMetrics 将 RawSnapshot 组装为 SystemMetrics（字段语义与旧 Agent 对齐）。
func (c *Collector) buildMetrics(node model.AgentConfig, snapshot *RawSnapshot) model.SystemMetrics {
	cpuUsage := c.computeCPUUsage(node.ID, snapshot.CPUStat)

	totalGB := round1(float64(snapshot.Memory.TotalKB) / 1024 / 1024)
	usedKB := snapshot.Memory.TotalKB - snapshot.Memory.AvailableKB
	usedGB := round1(float64(usedKB) / 1024 / 1024)
	ramPercent := 0.0
	if snapshot.Memory.TotalKB > 0 {
		ramPercent = round1(clampPercent(float64(usedKB) / float64(snapshot.Memory.TotalKB) * 100))
	}

	hostname := snapshot.Hostname
	if strings.TrimSpace(hostname) == "" {
		hostname = node.Name
	}

	gpus, uuidToIndex := buildGPUs(snapshot.GPUs)
	processes := buildProcesses(snapshot.Processes, snapshot.GPUProcesses, uuidToIndex)

	return model.SystemMetrics{
		Hostname:      hostname,
		IPAddress:     node.SSHHost,
		OS:            snapshot.OS,
		UptimeSeconds: snapshot.UptimeSeconds,
		UptimeHuman:   model.FormatUptime(snapshot.UptimeSeconds),
		CPUModel:      snapshot.CPUModel,
		CPUUsage:      cpuUsage,
		CPUCores:      snapshot.CPUCores,
		RAMTotal:      totalGB,
		RAMUsed:       usedGB,
		RAMPercent:    ramPercent,
		NetSentMB:     round1(float64(snapshot.Network.TXBytes) / 1024 / 1024),
		NetRecvMB:     round1(float64(snapshot.Network.RXBytes) / 1024 / 1024),
		Gpus:          gpus,
		Processes:     processes,
	}
}

// computeCPUUsage 基于 /proc/stat 累计计数做差分：
//   - 首次采样返回 0（不引入可空字段破坏 API）；
//   - 计数回退（重启）返回 0 并更新缓存；
//   - 零差分返回 0，避免除零。
func (c *Collector) computeCPUUsage(nodeID int64, stat CPUStatSnapshot) float64 {
	total := stat.total()
	idle := stat.idleTime()

	c.mu.Lock()
	defer c.mu.Unlock()

	prev, ok := c.prevCPU[nodeID]
	c.prevCPU[nodeID] = cpuSample{total: total, idle: idle}
	if !ok {
		return 0
	}

	totalDelta := int64(total) - int64(prev.total)
	idleDelta := int64(idle) - int64(prev.idle)
	if totalDelta <= 0 || idleDelta < 0 {
		return 0
	}

	usage := (float64(totalDelta) - float64(idleDelta)) / float64(totalDelta) * 100
	return round1(clampPercent(usage))
}

// buildGPUs 转换 GPU 列表，并建立 UUID → Index 映射供进程匹配。
// 无 GPU 时返回空切片（不能导致节点 offline）。
func buildGPUs(raw []RawGPU) ([]model.GpuInfo, map[string]int) {
	gpus := make([]model.GpuInfo, 0, len(raw))
	uuidToIndex := make(map[string]int, len(raw))

	for _, gpu := range raw {
		memTotalGB := round1(gpu.MemoryTotalMB / 1024)
		memUsedGB := round1(gpu.MemoryUsedMB / 1024)
		memUtil := 0.0
		if gpu.MemoryTotalMB > 0 {
			memUtil = round1(clampPercent(gpu.MemoryUsedMB / gpu.MemoryTotalMB * 100))
		}

		uuidToIndex[gpu.UUID] = gpu.Index
		gpus = append(gpus, model.GpuInfo{
			ID:                gpu.Index,
			Name:              gpu.Name,
			Temperature:       gpu.Temperature,
			FanSpeed:          gpu.FanSpeed,
			PowerDraw:         gpu.PowerDraw,
			Utilization:       gpu.Utilization,
			MemoryTotal:       memTotalGB,
			MemoryUsed:        memUsedGB,
			MemoryUtilization: memUtil,
		})
	}
	return gpus, uuidToIndex
}

// gpuProcMapping 保存进程与其占用的 GPU。
type gpuProcMapping struct {
	gpuIndex int
	vramMB   int
}

// buildProcesses 组装进程列表，复现旧 Agent 的保留规则：
//   - GPU 进程始终保留；
//   - 非 GPU 进程仅保留 CPU > 2%；
//   - GPU 进程优先，其后按 CPU 降序，最多 10 条。
func buildProcesses(raw []RawProcess, rawGPUProcs []RawGPUProcess, uuidToIndex map[string]int) []model.ProcessInfo {
	gpuProcs := make(map[int]gpuProcMapping, len(rawGPUProcs))
	for _, proc := range rawGPUProcs {
		index, ok := uuidToIndex[proc.UUID]
		if !ok {
			// UUID 与 GPU 列表对不上时忽略，不影响其余数据。
			continue
		}
		gpuProcs[proc.PID] = gpuProcMapping{gpuIndex: index, vramMB: proc.VRAMMB}
	}

	active := make([]model.ProcessInfo, 0, 16)
	for _, proc := range raw {
		gpuData, isGPUProc := gpuProcs[proc.PID]
		if !isGPUProc && proc.CPUPercent <= 2.0 {
			continue
		}

		user := proc.User
		if user == "" {
			user = "system"
		}

		var gpuIndex *int
		var vramUsedMB *int
		if isGPUProc {
			gpuIndexValue := gpuData.gpuIndex
			vramValue := gpuData.vramMB
			gpuIndex = &gpuIndexValue
			vramUsedMB = &vramValue
		}

		active = append(active, model.ProcessInfo{
			PID:           proc.PID,
			User:          user,
			Command:       truncateCommand(proc.Args),
			CPUPercent:    round1(proc.CPUPercent),
			MemoryPercent: round1(proc.MemoryPercent),
			GPUIndex:      gpuIndex,
			VRAMUsedMB:    vramUsedMB,
		})
	}

	sortProcesses(active)

	if len(active) > 10 {
		active = active[:10]
	}
	return active
}

// sortProcesses GPU 进程在前，其余按 CPU 降序。
func sortProcesses(processes []model.ProcessInfo) {
	sort.Slice(processes, func(i, j int) bool {
		leftGPU := processes[i].GPUIndex != nil
		rightGPU := processes[j].GPUIndex != nil
		if leftGPU != rightGPU {
			return leftGPU
		}
		return processes[i].CPUPercent > processes[j].CPUPercent
	})
}
