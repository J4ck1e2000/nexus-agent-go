# GPU 显存碎片化

Category: gpu_memory_fragmentation
Tags: gpu fragmentation, cuda allocator, vram fragmentation, 显存碎片, 分配失败, 大块连续内存

## Symptoms / 症状
- OOM occurs despite some free VRAM.
- 大块显存分配失败，小块分配仍可成功。
- Long-running mixed workloads degrade over time.

## Common Causes / 常见原因
- Frequent variable-size allocations.
- 重复加载/卸载模型导致内存布局破碎。
- Multi-tenant jobs with different memory patterns.
- Allocator settings not tuned.

## What To Check / 排查项
- Free VRAM vs largest allocatable block.
- Allocation pattern and job lifecycle.
- Framework allocator options and logs.
- 是否重启后短期恢复正常。

## Recommended Actions / 建议动作
- Stabilize input shapes/batch where possible.
- 调整 allocator 参数，减少碎片。
- Controlled restart of affected workers.
- Isolate workloads with very different memory profiles.

## Notes / 备注
- Fragmentation is a common hidden cause of intermittent OOM.
