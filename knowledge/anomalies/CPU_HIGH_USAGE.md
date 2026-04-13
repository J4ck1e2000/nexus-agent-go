# CPU 使用率过高

Category: cpu
Tags: cpu, high cpu, system load, cpu bound, 负载高, cpu过高, 争用

## Symptoms / 症状
- CPU usage > 85% for sustained windows.
- 系统负载升高，任务响应与调度延迟明显。
- GPU feeds become unstable due to host bottleneck.

## Common Causes / 常见原因
- Too many concurrent jobs on same node.
- 数据预处理线程数过多或代码低效。
- Runaway process / tight loop / log storm.
- Shared service contention (compression, encryption, ETL).

## What To Check / 排查项
- Top CPU processes by user/command/PID.
- Active users and recent scheduling placement.
- Correlation with disk/network saturation.
- 历史趋势中是否出现周期性尖峰。

## Recommended Actions / 建议动作
- Limit concurrency and set per-user quota.
- 降低数据预处理并发，优化热点逻辑。
- Isolate or terminate abnormal process safely.
- Rebalance workloads to other nodes.

## Notes / 备注
- High CPU can indirectly reduce GPU utilization and increase OOM risk.
