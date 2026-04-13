# 内存占用过高

Category: memory
Tags: memory, ram, high memory, swap, leak, memory over 90, 内存高, 内存超过90, 内存90%, swap抖动, 内存泄漏

## Symptoms / 症状
- RAM usage > 90%, swap activity increases.
- 进程被 OOM killer 回收或频繁重启。
- Job latency spikes with memory pressure.

## Common Causes / 常见原因
- Memory leak in long-running service.
- Cache/prefetch buffers are oversized.
- 多用户并发导致内存争用。
- Zombie or detached processes are not cleaned.

## What To Check / 排查项
- RSS trend by process over 30m/1h.
- Swap in/out and page-fault metrics.
- Active user count and concurrency settings.
- 数据是否陈旧（避免误判）。

## Recommended Actions / 建议动作
- Reduce concurrency and buffer size.
- 回收异常进程，必要时滚动重启组件。
- Move jobs to nodes with larger memory headroom.
- Add memory limits and alert thresholds.

## Notes / 备注
- If metrics are stale, refresh telemetry before final diagnosis.

