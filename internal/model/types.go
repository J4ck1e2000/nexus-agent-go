package model

// AgentConfig 描述前端保存的单个节点配置。
type AgentConfig struct {
	// ID 为节点唯一标识。
	ID int64 `json:"id"`
	// Name 为节点显示名称。
	Name string `json:"name"`
	// URL 为节点 Agent 地址。
	URL string `json:"url"`
	// Token 为 Agent Bearer Token（可选）。
	Token string `json:"token,omitempty"`
}

// ProcessInfo 表示进程级监控数据。
type ProcessInfo struct {
	// PID 为进程号。
	PID int `json:"pid"`
	// User 为进程所属用户。
	User string `json:"user"`
	// Command 为进程命令行摘要。
	Command string `json:"command"`
	// CPUPercent 为进程 CPU 占用。
	CPUPercent float64 `json:"cpu_percent"`
	// MemoryPercent 为进程内存占用。
	MemoryPercent float64 `json:"memory_percent"`
	// GPUIndex 为进程占用的 GPU 编号（可空）。
	GPUIndex *int `json:"gpu_index,omitempty"`
	// VRAMUsedMB 为显存占用（可空）。
	VRAMUsedMB *int `json:"vram_used_mb,omitempty"`
}

// GpuInfo 表示单张显卡监控数据。
type GpuInfo struct {
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	Temperature       int     `json:"temperature"`
	FanSpeed          int     `json:"fan_speed"`
	PowerDraw         int     `json:"power_draw"`
	Utilization       int     `json:"utilization"`
	MemoryTotal       float64 `json:"memory_total"`
	MemoryUsed        float64 `json:"memory_used"`
	MemoryUtilization float64 `json:"memory_utilization"`
}

// SystemMetrics 为 Agent 返回给前端的完整指标结构。
type SystemMetrics struct {
	Hostname      string        `json:"hostname"`
	IPAddress     string        `json:"ip_address"`
	OS            string        `json:"os"`
	UptimeSeconds float64       `json:"uptime_seconds"`
	UptimeHuman   string        `json:"uptime_human"`
	CPUModel      string        `json:"cpu_model"`
	CPUUsage      float64       `json:"cpu_usage"`
	CPUCores      int           `json:"cpu_cores"`
	RAMTotal      float64       `json:"ram_total"`
	RAMUsed       float64       `json:"ram_used"`
	RAMPercent    float64       `json:"ram_percent"`
	NetSentMB     float64       `json:"net_sent_mb"`
	NetRecvMB     float64       `json:"net_recv_mb"`
	Gpus          []GpuInfo     `json:"gpus"`
	Processes     []ProcessInfo `json:"processes"`
}
