// Package agenthttp 实现旧版 Nexus Agent 的 HTTP 指标采集，
// 行为与原 NodeStateService.fetchNodeCurrentMetrics 完全一致。
package agenthttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nexus-agent-go/internal/model"
)

// Collector 通过 GET {URL}/metrics/current 拉取旧版 Agent 指标。
type Collector struct {
	httpClient  *http.Client
	pollTimeout time.Duration
}

// New 创建 Agent HTTP 采集器；httpClient 为 nil 时使用默认客户端。
func New(httpClient *http.Client, pollTimeout time.Duration) *Collector {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if pollTimeout <= 0 {
		pollTimeout = 3 * time.Second
	}
	return &Collector{
		httpClient:  httpClient,
		pollTimeout: pollTimeout,
	}
}

// Collect 拉取一次节点当前指标。
func (c *Collector) Collect(ctx context.Context, node model.AgentConfig) (model.SystemMetrics, int64, error) {
	targetURL := strings.TrimSpace(node.URL)
	if targetURL == "" {
		return model.SystemMetrics{}, 0, fmt.Errorf("node url is empty")
	}
	targetURL = strings.TrimRight(targetURL, "/") + "/metrics/current"

	requestCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return model.SystemMetrics{}, 0, fmt.Errorf("build request failed: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return model.SystemMetrics{}, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return model.SystemMetrics{}, 0, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, message)
	}

	var payload struct {
		CollectedAtUnix int64 `json:"collected_at_unix"`
		model.SystemMetrics
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return model.SystemMetrics{}, 0, fmt.Errorf("decode response failed: %w", err)
	}
	return payload.SystemMetrics, payload.CollectedAtUnix, nil
}
