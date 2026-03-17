package agent

import (
	"sync"
	"time"

	"nexus-agent-go/internal/model"
)

type cachedSnapshot struct {
	metrics         model.SystemMetrics
	collectedAtUnix int64
}

// atomicCache keeps the most recent metrics snapshot in memory.
type atomicCache struct {
	mu       sync.RWMutex
	snapshot *cachedSnapshot
}

// Set stores a cloned snapshot and attaches collection timestamp metadata.
func (c *atomicCache) Set(metrics model.SystemMetrics) {
	cloned := metrics
	cloned.Gpus = append([]model.GpuInfo(nil), metrics.Gpus...)
	cloned.Processes = append([]model.ProcessInfo(nil), metrics.Processes...)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot = &cachedSnapshot{
		metrics:         cloned,
		collectedAtUnix: time.Now().Unix(),
	}
}

// Get returns a cloned snapshot for backward compatibility.
func (c *atomicCache) Get() (model.SystemMetrics, bool) {
	metrics, _, ok := c.GetWithMeta()
	return metrics, ok
}

// GetWithMeta returns a cloned snapshot and collection timestamp.
func (c *atomicCache) GetWithMeta() (model.SystemMetrics, int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.snapshot == nil {
		return model.SystemMetrics{}, 0, false
	}

	cloned := c.snapshot.metrics
	cloned.Gpus = append([]model.GpuInfo(nil), c.snapshot.metrics.Gpus...)
	cloned.Processes = append([]model.ProcessInfo(nil), c.snapshot.metrics.Processes...)
	return cloned, c.snapshot.collectedAtUnix, true
}
