# 通用排障流程 / General Diagnostic Flow

Category: diagnostic_flow
Tags: diagnostic, runbook, troubleshooting, 排障流程, 运维建议, 故障定位

## Symptoms / 症状
- User reports anomaly but root cause is unclear.
- 多个信号冲突（如高显存但低利用率）。
- Metrics are stale or partially missing.

## Common Causes / 常见原因
- Missing realtime context.
- 资源瓶颈叠加（CPU/IO/GPU/网络）。
- 依据过时快照做了错误推断。

## What To Check / 排查步骤
- Step 1: verify node online status + metrics freshness.
- 步骤 2：检查 CPU/内存/GPU 概览与活跃用户数。
- Step 3: inspect process table for dominant offenders.
- 步骤 4：结合 30m/1h 历史趋势交叉验证。

## Recommended Actions / 建议动作
- Start from reversible, low-risk actions first.
- 先降载，再执行侵入式修复动作。
- Re-check metrics after each action.
- 记录根因与处理过程，沉淀 runbook。

## Notes / 备注
- Realtime telemetry has priority; knowledge docs are generic supplements.
