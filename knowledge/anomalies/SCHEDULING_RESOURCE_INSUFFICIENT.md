# 调度资源不足

Category: scheduling
Tags: scheduling, insufficient resources, pending job, placement fail, 调度失败, 资源不足, 无法分配

## Symptoms / 症状
- Job remains pending for long time.
- 找不到满足 GPU/VRAM/并发约束的节点。
- Scheduler retries repeatedly with no success.

## Common Causes / 常见原因
- Requirement is too strict.
- 节点离线或数据陈旧导致可用候选减少。
- Active users consume most available headroom.
- GPU free memory is fragmented.

## What To Check / 排查项
- Requested profile vs current cluster headroom.
- Node online state and metrics freshness.
- Busy ratio and per-GPU free memory.
- Scheduler failure reasons from logs/events.

## Recommended Actions / 建议动作
- Relax constraints where safe.
- 优先迁移到更新鲜、竞争更低的节点。
- Use queue priority/backpressure.
- Add capacity or rebalance workloads.

## Notes / 备注
- Distinguish temporary contention from true capacity shortage.
