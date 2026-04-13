# GPU OOM / 显存不足

Category: gpu_memory
Tags: gpu, oom, cuda, vram, out of memory, 显存不足, 显存爆满, 显存溢出

## Symptoms / 症状
- CUDA out of memory; job exits during allocation.
- 显存占用接近 95%-100%，新任务分配失败。
- Inference/training starts but fails at first batch.

## Common Causes / 常见原因
- Batch size or sequence length is too large.
- 批大小、序列长度或并发设置过高。
- Model + optimizer states exceed available VRAM.
- 孤儿进程长期占用显存，多用户共享同卡竞争。

## What To Check / 排查项
- `nvidia-smi` process list (PID, VRAM, user, command).
- 节点 GPU busy ratio、active user count、数据新鲜度。
- Low-utilization but high-VRAM processes.
- 是否存在历史任务残留进程或容器未清理。

## Recommended Actions / 建议动作
- Reduce batch size / sequence length / precision (fp16/bf16).
- 降低 batch、切到更空闲节点，必要时拆分任务。
- Kill abnormal orphan process and retry safely.
- 评估量化、模型切分、多卡并行或 MIG 策略。

## Notes / 备注
- Realtime metrics are source of truth; this document is generic guidance.
