package model

// MetricsSummary is the structured summary view for /metrics/summary.
type MetricsSummary struct {
	CollectedAtUnix int64   `json:"collected_at_unix"`
	Hostname        string  `json:"hostname"`
	IPAddress       string  `json:"ip_address"`
	OS              string  `json:"os"`
	UptimeSeconds   float64 `json:"uptime_seconds"`
	UptimeHuman     string  `json:"uptime_human"`
	CPUModel        string  `json:"cpu_model"`
	CPUUsage        float64 `json:"cpu_usage"`
	CPUCores        int     `json:"cpu_cores"`
	RAMTotal        float64 `json:"ram_total"`
	RAMUsed         float64 `json:"ram_used"`
	RAMPercent      float64 `json:"ram_percent"`
	NetSentMB       float64 `json:"net_sent_mb"`
	NetRecvMB       float64 `json:"net_recv_mb"`

	GPUCount             int     `json:"gpu_count"`
	ProcessCount         int     `json:"process_count"`
	GPUProcessCount      int     `json:"gpu_process_count"`
	TopCPUProcessCommand string  `json:"top_cpu_process_command"`
	TopCPUProcessPercent float64 `json:"top_cpu_process_percent"`

	GPUUtilizationAvg    float64 `json:"gpu_utilization_avg"`
	GPUMemoryUsedMBTotal float64 `json:"gpu_memory_used_mb_total"`
	GPUMemoryTotalMB     float64 `json:"gpu_memory_total_mb"`
	GPUMemoryPercentAvg  float64 `json:"gpu_memory_percent_avg"`
}

// ProcessesResponse is the structured process view for /metrics/processes.
type ProcessesResponse struct {
	CollectedAtUnix int64         `json:"collected_at_unix"`
	ProcessCount    int           `json:"process_count"`
	GPUProcessCount int           `json:"gpu_process_count"`
	Processes       []ProcessInfo `json:"processes"`
}

// GPUsResponse is the structured GPU view for /metrics/gpus.
type GPUsResponse struct {
	CollectedAtUnix int64     `json:"collected_at_unix"`
	GPUCount        int       `json:"gpu_count"`
	GPUs            []GpuInfo `json:"gpus"`
}
