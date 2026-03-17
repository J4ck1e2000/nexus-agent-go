package agent

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	hostpkg "github.com/shirou/gopsutil/v4/host"
	mempkg "github.com/shirou/gopsutil/v4/mem"
	netpkg "github.com/shirou/gopsutil/v4/net"
	processpkg "github.com/shirou/gopsutil/v4/process"

	"nexus-agent-go/internal/model"
)

// Collector 负责一次性采集主机指标。
type Collector struct{}

// NewCollector 创建采集器实例。
func NewCollector() *Collector {
	return &Collector{}
}

// Collect 采集系统、网络、GPU、进程等信息并组装返回。
func (c *Collector) Collect(ctx context.Context) (model.SystemMetrics, error) {
	hostInfo, err := hostpkg.InfoWithContext(ctx)
	if err != nil {
		return model.SystemMetrics{}, err
	}

	uptimeSec := float64(hostInfo.Uptime)
	cpuUsage := 0.0
	if values, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(values) > 0 {
		cpuUsage = round1(values[0])
	}

	cpuCores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		cpuCores = 0
	}

	vm, err := mempkg.VirtualMemoryWithContext(ctx)
	if err != nil {
		return model.SystemMetrics{}, err
	}

	netSentMB, netRecvMB := 0.0, 0.0
	if counters, err := netpkg.IOCountersWithContext(ctx, false); err == nil && len(counters) > 0 {
		netSentMB = round1(float64(counters[0].BytesSent) / 1024 / 1024)
		netRecvMB = round1(float64(counters[0].BytesRecv) / 1024 / 1024)
	}

	gpus, gpuProcessMap, err := collectGPU(ctx)
	if err != nil {
		gpus = []model.GpuInfo{}
		gpuProcessMap = map[int]gpuProcess{}
	}

	processes := collectProcesses(ctx, gpuProcessMap)

	metrics := model.SystemMetrics{
		Hostname:      hostInfo.Hostname,
		IPAddress:     getOutboundIP(),
		OS:            strings.TrimSpace(hostInfo.Platform + " " + hostInfo.KernelVersion),
		UptimeSeconds: uptimeSec,
		UptimeHuman:   formatUptime(uptimeSec),
		CPUModel:      readCPUModel(),
		CPUUsage:      cpuUsage,
		CPUCores:      cpuCores,
		RAMTotal:      round1(float64(vm.Total) / 1024 / 1024 / 1024),
		RAMUsed:       round1(float64(vm.Used) / 1024 / 1024 / 1024),
		RAMPercent:    round1(vm.UsedPercent),
		NetSentMB:     netSentMB,
		NetRecvMB:     netRecvMB,
		Gpus:          gpus,
		Processes:     processes,
	}
	return metrics, nil
}

// collectProcesses 采集活跃进程，并优先保留 GPU 进程。
func collectProcesses(ctx context.Context, gpuProcessMap map[int]gpuProcess) []model.ProcessInfo {
	procs, err := processpkg.ProcessesWithContext(ctx)
	if err != nil {
		return []model.ProcessInfo{}
	}

	active := make([]model.ProcessInfo, 0, 16)
	for _, proc := range procs {
		pid := int(proc.Pid)
		gpuData, isGPUProc := gpuProcessMap[pid]

		cpuPercent := 0.0
		if v, err := proc.CPUPercentWithContext(ctx); err == nil {
			cpuPercent = round1(v)
		}

		if !isGPUProc && cpuPercent <= 2.0 {
			continue
		}

		memPercent := 0.0
		if v, err := proc.MemoryPercentWithContext(ctx); err == nil {
			memPercent = round1(float64(v))
		}

		username, err := proc.UsernameWithContext(ctx)
		if err != nil || username == "" {
			username = "system"
		}

		command := buildProcessCommand(ctx, proc)

		var gpuIndex *int
		var vramUsedMB *int
		if isGPUProc {
			gpuIndex = &gpuData.gpuIndex
			vramUsedMB = &gpuData.vramMB
		}

		active = append(active, model.ProcessInfo{
			PID:           pid,
			User:          username,
			Command:       command,
			CPUPercent:    cpuPercent,
			MemoryPercent: memPercent,
			GPUIndex:      gpuIndex,
			VRAMUsedMB:    vramUsedMB,
		})
	}

	sort.Slice(active, func(i, j int) bool {
		iGPU := active[i].GPUIndex != nil
		jGPU := active[j].GPUIndex != nil
		if iGPU != jGPU {
			return iGPU
		}
		return active[i].CPUPercent > active[j].CPUPercent
	})

	if len(active) > 10 {
		active = active[:10]
	}
	return active
}

// buildProcessCommand 提取并截断命令行，避免返回数据过大。
func buildProcessCommand(ctx context.Context, proc *processpkg.Process) string {
	name, _ := proc.NameWithContext(ctx)
	cmdline, err := proc.CmdlineSliceWithContext(ctx)
	if err != nil || len(cmdline) == 0 {
		if name == "" {
			return "unknown"
		}
		return name
	}

	parts := cmdline
	if len(parts) > 3 {
		parts = parts[:3]
	}
	cmd := strings.Join(parts, " ")
	if len(cmd) > 60 {
		return cmd[:57] + "..."
	}
	return cmd
}

// errNoMetrics 表示缓存还没有可用指标。
var errNoMetrics = errors.New("metrics_unavailable")

// Service 负责周期采集与快照读取。
type Service struct {
	collector *Collector
	interval  time.Duration
	cache     atomicCache
}

// NewService 创建带采样间隔的服务。
func NewService(interval time.Duration) *Service {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Service{collector: NewCollector(), interval: interval}
}

// Start 启动采集循环。
func (s *Service) Start(ctx context.Context) {
	s.collectOnce(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collectOnce(ctx)
		}
	}
}

// Snapshot 返回最近一次缓存数据。
func (s *Service) Snapshot() (model.SystemMetrics, error) {
	metrics, _, err := s.SnapshotWithMeta()
	if err != nil {
		return model.SystemMetrics{}, err
	}
	return metrics, nil
}

// SnapshotWithMeta returns snapshot data with collection timestamp metadata.
func (s *Service) SnapshotWithMeta() (model.SystemMetrics, int64, error) {
	metrics, collectedAtUnix, ok := s.cache.GetWithMeta()
	if ok {
		return metrics, collectedAtUnix, nil
	}
	return model.SystemMetrics{}, 0, errNoMetrics
}

// collectOnce 单次采集并写入缓存。
func (s *Service) collectOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	metrics, err := s.collector.Collect(ctx)
	if err != nil {
		return
	}
	s.cache.Set(metrics)
}
