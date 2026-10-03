package sshcollector

// RawSnapshot 是一轮远程脚本输出的原始解析结果，
// 由 SSHCollector 进一步转换为 model.SystemMetrics。
type RawSnapshot struct {
	Hostname      string
	OS            string
	UptimeSeconds float64

	CPUModel string
	CPUCores int
	CPUStat  CPUStatSnapshot

	Memory  MemorySnapshot
	Network NetworkSnapshot

	GPUs         []RawGPU
	GPUProcesses []RawGPUProcess

	Processes []RawProcess
}

// CPUStatSnapshot 对应 /proc/stat 的 cpu 汇总行（累计 jiffies）。
type CPUStatSnapshot struct {
	User    uint64
	Nice    uint64
	System  uint64
	Idle    uint64
	IOWait  uint64
	IRQ     uint64
	SoftIRQ uint64
	Steal   uint64
}

// total 返回全部字段之和。
func (s CPUStatSnapshot) total() uint64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ + s.Steal
}

// idleTime 返回 idle + iowait。
func (s CPUStatSnapshot) idleTime() uint64 {
	return s.Idle + s.IOWait
}

// MemorySnapshot 保存 /proc/meminfo 的关键字段（单位 kB）。
type MemorySnapshot struct {
	TotalKB     uint64
	AvailableKB uint64
}

// NetworkSnapshot 保存 /proc/net/dev 的累计字节数（排除 lo）。
type NetworkSnapshot struct {
	RXBytes uint64
	TXBytes uint64
}

// RawGPU 是 nvidia-smi --query-gpu 的一行。
type RawGPU struct {
	Index         int
	UUID          string
	Name          string
	Temperature   int
	FanSpeed      int
	PowerDraw     int
	Utilization   int
	MemoryTotalMB float64
	MemoryUsedMB  float64
}

// RawGPUProcess 是 nvidia-smi --query-compute-apps 的一行。
type RawGPUProcess struct {
	PID    int
	UUID   string
	VRAMMB int
}

// RawProcess 是 ps -eo pid=,user=,pcpu=,pmem=,args= 的一行。
type RawProcess struct {
	PID           int
	User          string
	CPUPercent    float64
	MemoryPercent float64
	Args          string
}
