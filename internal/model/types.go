package model

import (
	"fmt"
	"time"
)

// 节点采集方式：agent 为旧版 HTTP Agent，ssh 为无代理 SSH 采集。
const (
	// CollectorTypeAgent 表示节点由部署在目标服务器上的 Nexus Agent 提供 HTTP 指标。
	CollectorTypeAgent = "agent"
	// CollectorTypeSSH 表示节点由 Gateway 通过 SSH 直接采集（agentless）。
	CollectorTypeSSH = "ssh"
)

// SSHAuthTypeKey 表示 SSH 仅支持密钥认证。
const SSHAuthTypeKey = "key"

// AgentConfig 描述前端保存的单个节点配置。
type AgentConfig struct {
	// ID 为节点唯一标识。
	ID int64 `json:"id"`
	// Name 为节点显示名称。
	Name string `json:"name"`
	// CollectorType 为采集方式（agent/ssh），空值按 agent 处理。
	CollectorType string `json:"collector_type,omitempty"`
	// URL 为节点 Agent 地址，仅 collector_type=agent 时有效。
	URL string `json:"url,omitempty"`

	// SSHHost 为 SSH 主机地址，仅 collector_type=ssh 时有效。
	SSHHost string `json:"ssh_host,omitempty"`
	// SSHPort 为 SSH 端口，仅 collector_type=ssh 时有效。
	SSHPort int `json:"ssh_port,omitempty"`
	// SSHUser 为 SSH 登录用户，仅 collector_type=ssh 时有效。
	SSHUser string `json:"ssh_user,omitempty"`
	// SSHAuthType 为 SSH 认证方式（当前仅 key）。
	SSHAuthType string `json:"ssh_auth_type,omitempty"`
	// SSHHostKey stores the pinned public host key internally; it is never sent to clients.
	SSHHostKey string `json:"-"`
	// SSHHostKeyFingerprint is safe to show to administrators.
	SSHHostKeyFingerprint string `json:"ssh_host_key_fingerprint,omitempty"`
}

// FormatUptime 将秒数转换为可读时长，供 Agent 与 SSH Collector 共用。
func FormatUptime(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
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
