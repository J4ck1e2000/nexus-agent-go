# 网络或 Agent 不可达

Category: connectivity
Tags: network, agent unreachable, heartbeat, timeout, 网络不可达, agent异常, 轮询超时

## Symptoms / 症状
- Gateway cannot poll node metrics.
- Agent heartbeat is missing.
- API timeout/retry spikes.

## Common Causes / 常见原因
- Agent process down or misconfigured.
- 防火墙/安全策略变更阻断链路。
- Packet loss, route flap, DNS issues.
- Endpoint URL or TLS credential mismatch.

## What To Check / 排查项
- Agent logs and service runtime state.
- DNS resolution + TCP connectivity from gateway.
- Firewall/routing/security group changes.
- Endpoint credential, cert expiry, clock drift.

## Recommended Actions / 建议动作
- Restart/fix agent and verify heartbeat.
- 修复网络路径与安全策略。
- Update endpoint configuration if changed.
- Keep node out of scheduling until stable.

## Notes / 备注
- Unreachable nodes should not be counted as available capacity.
