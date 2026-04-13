# 进程资源竞争

Category: process_contention
Tags: process contention, noisy neighbor, pid, resource contention, 进程抢占, 资源竞争, 干扰任务

## Symptoms / 症状
- One process dominates CPU/RAM/VRAM.
- 多用户反馈同节点性能突降。
- Scheduler repeatedly places jobs on already noisy node.

## Common Causes / 常见原因
- Missing per-process/per-user limits.
- Debug or stale long-running jobs not cleaned.
- Inference and training mixed without isolation.
- Child processes detached from supervisor.

## What To Check / 排查项
- Top processes by CPU/RAM/VRAM.
- User ownership, command patterns, process age.
- Active users and recent scheduling history.
- Node-level pressure changes after process spikes.

## Recommended Actions / 建议动作
- Enforce quota and concurrency limits.
- 终止异常长期占用进程并补充回收策略。
- Split workloads by pool (training/inference).
- Add noisy-neighbor detection alerts.

## Notes / 备注
- Process contention often explains low-utilization + high-memory patterns.
