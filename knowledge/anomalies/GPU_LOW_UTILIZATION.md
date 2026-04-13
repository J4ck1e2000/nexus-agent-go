# GPU 利用率低

Category: gpu_utilization
Tags: gpu, low utilization, idle gpu, throughput, 利用率低, 吞吐低, 负载不均

## Symptoms / 症状
- GPU utilization stays below 30% for long periods.
- 显存已占用但计算利用率持续偏低。
- Throughput is low while queue/latency grows.

## Common Causes / 常见原因
- Data loader cannot feed GPU fast enough.
- CPU、磁盘或网络成为瓶颈导致 GPU 等待。
- Batch size too small or too many sync points.
- 混部干扰导致资源抖动（noisy neighbor）。

## What To Check / 排查项
- Data pipeline worker count, prefetch, cache hit.
- CPU 使用率、IO 吞吐、网络时延与丢包。
- Framework logs: dataloader stall, sync wait.
- `nvidia-smi` process behavior and clock throttling.

## Recommended Actions / 建议动作
- Increase workers/prefetch and optimize input pipeline.
- 增大 batch（在显存允许下）并减少不必要同步。
- 将任务迁移到竞争更少的节点。
- Separate online inference and heavy training pools.

## Notes / 备注
- Low utilization with high VRAM often means pipeline bottleneck, not GPU shortage.
