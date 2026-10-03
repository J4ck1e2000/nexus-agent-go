// ssh-smoke 是一次性的 SSH 采集自检工具（非常驻服务）：
// 连接一台目标服务器，执行一次完整采集，输出 JSON 摘要后退出。
//
// 用法：
//
//	SSH_HOST=10.0.0.15 \
//	SSH_PORT=22 \
//	SSH_USER=renhaokun \
//	SSH_PRIVATE_KEY_PATH=/home/me/.ssh/id_ed25519 \
//	SSH_KNOWN_HOSTS_PATH=/home/me/.ssh/known_hosts \
//	go run ./cmd/ssh-smoke
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nexus-agent-go/internal/collector/sshcollector"
	"nexus-agent-go/internal/model"
)

type smokeOutput struct {
	Hostname    string     `json:"hostname"`
	CPUUsage    float64    `json:"cpu_usage"`
	CPUCores    int        `json:"cpu_cores"`
	RAMTotal    float64    `json:"ram_total"`
	RAMUsed     float64    `json:"ram_used"`
	RAMPercent  float64    `json:"ram_percent"`
	NetRecvMB   float64    `json:"net_recv_mb"`
	NetSentMB   float64    `json:"net_sent_mb"`
	GPUCount    int        `json:"gpu_count"`
	ProcessCnt  int        `json:"process_count"`
	GPUs        []smokeGPU `json:"gpus"`
	CollectedAt int64      `json:"collected_at_unix"`
	Elapsed     string     `json:"elapsed"`
	Error       string     `json:"error,omitempty"`
}

type smokeGPU struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Temperature int     `json:"temperature"`
	Utilization int     `json:"utilization"`
	MemoryTotal float64 `json:"memory_total"`
	MemoryUsed  float64 `json:"memory_used"`
}

func main() {
	host := strings.TrimSpace(os.Getenv("SSH_HOST"))
	user := strings.TrimSpace(os.Getenv("SSH_USER"))
	port := 22
	if raw := strings.TrimSpace(os.Getenv("SSH_PORT")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			log.Fatalf("invalid SSH_PORT %q", raw)
		}
		port = parsed
	}
	if host == "" || user == "" {
		log.Fatal("SSH_HOST and SSH_USER are required")
	}

	runnerCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	collector, err := sshcollector.NewWithOptionsFromEnv(sshcollector.LoadOptionsFromEnv())
	if err != nil {
		// 错误码输出到 stderr，便于脚本判断；不含私钥路径等敏感信息。
		log.Fatalf("ssh smoke init failed: %s", sshcollector.ErrorCode(err))
	}
	defer collector.Close()

	start := time.Now()
	ctx, cancel := context.WithTimeout(runnerCtx, 30*time.Second)
	defer cancel()

	node := model.AgentConfig{
		CollectorType: model.CollectorTypeSSH,
		SSHHost:       host,
		SSHPort:       port,
		SSHUser:       user,
	}
	metrics, collectedAt, err := collector.Collect(ctx, node)
	output := smokeOutput{
		Hostname:    metrics.Hostname,
		CPUUsage:    metrics.CPUUsage,
		CPUCores:    metrics.CPUCores,
		RAMTotal:    metrics.RAMTotal,
		RAMUsed:     metrics.RAMUsed,
		RAMPercent:  metrics.RAMPercent,
		NetRecvMB:   metrics.NetRecvMB,
		NetSentMB:   metrics.NetSentMB,
		GPUCount:    len(metrics.Gpus),
		ProcessCnt:  len(metrics.Processes),
		CollectedAt: collectedAt,
		Elapsed:     time.Since(start).Round(time.Millisecond).String(),
	}
	for _, gpu := range metrics.Gpus {
		output.GPUs = append(output.GPUs, smokeGPU{
			ID:          gpu.ID,
			Name:        gpu.Name,
			Temperature: gpu.Temperature,
			Utilization: gpu.Utilization,
			MemoryTotal: gpu.MemoryTotal,
			MemoryUsed:  gpu.MemoryUsed,
		})
	}
	if err != nil {
		output.Error = sshcollector.ErrorCode(err)
	}

	encoded, encodeErr := json.MarshalIndent(output, "", "  ")
	if encodeErr != nil {
		log.Fatalf("encode output failed: %v", encodeErr)
	}
	fmt.Println(string(encoded))

	if err != nil {
		os.Exit(1)
	}
}
