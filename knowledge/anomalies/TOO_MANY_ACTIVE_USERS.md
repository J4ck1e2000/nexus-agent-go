# 活跃用户过多导致竞争

Category: user_contention
Tags: active users, concurrency contention, user quota, 多用户竞争, 活跃用户过多, 并发争用

## Symptoms / 症状
- Active user count spikes on one node.
- 队列等待时间增加，性能波动加剧。
- Frequent preemption, unstable latency.

## Common Causes / 常见原因
- No per-user quota or admission control.
- Scheduler hotspot routing to same nodes.
- Shared storage/network bottlenecks under bursts.
- 大量突发提交未做节流。

## What To Check / 排查项
- Active users per node and fairness trend.
- Queue depth, wait time, retry rate.
- Storage/network saturation indicators.
- Recent placement distribution by user/team.

## Recommended Actions / 建议动作
- Set user-level concurrency quota.
- 做负载均衡，避免热点节点集中。
- Add admission control for bursts.
- Reserve capacity for critical workloads.

## Notes / 备注
- High active-user count is a strong instability signal for scheduling.
