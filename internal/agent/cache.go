package agent

import (
	"sync"

	"nexus-agent-go/internal/model"
)

// atomicCache 用于线程安全缓存最近一次采集结果。
type atomicCache struct {
	mu      sync.RWMutex
	metrics *model.SystemMetrics
}

// Set 写入缓存，并做浅拷贝防止外部切片被后续修改。
func (c *atomicCache) Set(metrics model.SystemMetrics) {
	cloned := metrics
	cloned.Gpus = append([]model.GpuInfo(nil), metrics.Gpus...)
	cloned.Processes = append([]model.ProcessInfo(nil), metrics.Processes...)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = &cloned
}

// Get 读取缓存，返回副本避免调用方修改内部数据。
func (c *atomicCache) Get() (model.SystemMetrics, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.metrics == nil {
		return model.SystemMetrics{}, false
	}

	cloned := *c.metrics
	cloned.Gpus = append([]model.GpuInfo(nil), c.metrics.Gpus...)
	cloned.Processes = append([]model.ProcessInfo(nil), c.metrics.Processes...)
	return cloned, true
}
