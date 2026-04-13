# 节点离线

Category: node_connectivity
Tags: offline, node down, unreachable, heartbeat lost, 节点离线, 不可达, 心跳中断

## Symptoms / 症状
- Node status is offline/unreachable.
- 网关轮询失败，心跳中断。
- New jobs cannot be scheduled to this node.

## Common Causes / 常见原因
- Agent process crashed or stopped.
- 网络路由/防火墙策略变更。
- Host reboot, kernel panic, hardware fault.
- Credential/TLS mismatch.

## What To Check / 排查项
- Agent service status and recent logs.
- Gateway-to-node connectivity (DNS/TCP/latency).
- Firewall/security group/routing changes.
- Host-level events (reboot, NIC errors, disk full).

## Recommended Actions / 建议动作
- Recover agent and verify heartbeat.
- 修复网络路径或安全策略。
- Endpoint changed? update node registry config.
- Keep node unschedulable until health is stable.

## Notes / 备注
- Offline node data must not be treated as realtime capacity.
