# 指标数据陈旧 / Metrics Stale

Category: data_freshness
Tags: stale, metrics stale, telemetry lag, poll delay, stale metrics, 数据陈旧, 指标过旧, 指标太旧, 数据太旧, 采集延迟

## Symptoms / 症状
- Data age significantly exceeds polling interval.
- 指标时间戳不更新或跳变异常。
- Suggestions look inconsistent with actual node state.

## Common Causes / 常见原因
- Poller timeout/backlog.
- Redis/cache write delay.
- Agent temporary network loss.
- Clock skew across components.

## What To Check / 排查项
- last_polled_at / collected_at timelines.
- Poller error rate and timeout trend.
- Redis latency and queue depth.
- NTP/clock sync for gateway and nodes.

## Recommended Actions / 建议动作
- Mark stale data as low confidence.
- 先恢复采集链路再做异常归因。
- Tune poll timeout/fanout under burst.
- Re-evaluate after fresh samples arrive.

## Notes / 备注
- Realtime decisions should be delayed when evidence is stale.

