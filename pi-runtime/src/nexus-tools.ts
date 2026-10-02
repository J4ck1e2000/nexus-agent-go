import { Type, type TSchema } from "typebox";
import type { ToolDefinition } from "@earendil-works/pi-coding-agent";
import { GatewayToolClient } from "./gateway-client.ts";

/**
 * The eight read-only Nexus tools, mirroring internal/ai/toolapi.go and the
 * Eino executor schemas (internal/ai/eino_agent.go buildTools).
 * Every tool calls back into the Go tool gateway; none touches storage here.
 */
export interface NexusToolContext {
	runID: string;
	credential: string;
	client: GatewayToolClient;
	toolTimeoutSec: number;
}

function nexusTool(
	name: string,
	label: string,
	description: string,
	parameters: TSchema,
	ctx: NexusToolContext,
): ToolDefinition {
	return {
		name,
		label,
		description,
		parameters,
		async execute(toolCallID, params, _signal, _onUpdate, _toolCtx) {
			try {
				const response = await ctx.client.call(
					ctx.runID,
					ctx.credential,
					name,
					toolCallID,
					(params ?? {}) as Record<string, unknown>,
					ctx.toolTimeoutSec,
				);
				const data = response.data ?? {};
				return {
					content: [{ type: "text", text: JSON.stringify({ ok: true, data, meta: response.meta }) }],
					details: { ok: true, data, meta: response.meta },
				};
			} catch (error) {
				const message = error instanceof Error ? error.message : String(error);
				return {
					content: [{ type: "text", text: JSON.stringify({ ok: false, error: message }) }],
					details: { ok: false },
					isError: true,
				};
			}
		},
	};
}

/** Builds the tool definitions bound to one run's credentials. */
export function buildNexusTools(ctx: NexusToolContext): ToolDefinition[] {
	return [
		nexusTool(
			"get_node_metrics",
			"Node metrics",
			"Get full current metrics snapshot of one node from gateway aggregate view.",
			Type.Object({
				node_name: Type.String({ description: "Target node name" }),
			}),
			ctx,
		),
		nexusTool(
			"list_idle_nodes",
			"Idle nodes",
			"List currently available nodes ranked by idle/scheduling score.",
			Type.Object({
				min_free_vram_gb: Type.Optional(Type.Number({ description: "Optional minimum free GPU memory in GB" })),
				limit: Type.Optional(Type.Integer({ description: "Candidate count limit" })),
			}),
			ctx,
		),
		nexusTool(
			"get_gpu_processes",
			"GPU processes",
			"Get GPU process list of a node.",
			Type.Object({
				node_name: Type.String({ description: "Target node name" }),
			}),
			ctx,
		),
		nexusTool(
			"get_node_summary",
			"Node summary",
			"Get readable node summary including health/gpu/process/risk flags.",
			Type.Object({
				node_name: Type.String({ description: "Target node name" }),
			}),
			ctx,
		),
		nexusTool(
			"get_alert_history",
			"Alert history",
			"Get alert/trend summary for a node or all nodes in a window like 30m/1h.",
			Type.Object({
				node_name: Type.Optional(Type.String({ description: "Optional node name" })),
				window: Type.Optional(
					Type.Union([Type.Literal("30m"), Type.Literal("1h")], { description: "Window like 30m or 1h" }),
				),
			}),
			ctx,
		),
		nexusTool(
			"recommend_nodes_for_job",
			"Recommend nodes",
			"Recommend nodes for a job requirement.",
			Type.Object({
				requirements: Type.Object({
					min_free_vram_gb: Type.Optional(Type.Number({ description: "Minimum free VRAM in GB" })),
					gpu_count: Type.Optional(Type.Integer({ description: "Requested GPU count" })),
					prefer_low_cpu: Type.Optional(Type.Boolean({ description: "Prefer lower CPU usage" })),
					prefer_low_ram: Type.Optional(Type.Boolean({ description: "Prefer lower RAM usage" })),
					prefer_fewer_users: Type.Optional(Type.Boolean({ description: "Prefer fewer active users" })),
					prefer_fresh_data: Type.Optional(Type.Boolean({ description: "Prefer fresh data samples" })),
				}),
				limit: Type.Optional(Type.Integer({ description: "Candidate count limit" })),
			}),
			ctx,
		),
		nexusTool(
			"explain_node_anomaly",
			"Explain anomaly",
			"Generate deterministic anomaly explanation for one node.",
			Type.Object({
				node_name: Type.String({ description: "Target node name" }),
			}),
			ctx,
		),
		nexusTool(
			"search_knowledge_base",
			"Search knowledge",
			"Search local troubleshooting knowledge for common causes and remediation steps.",
			Type.Object({
				query: Type.String({ description: "Knowledge query text" }),
				limit: Type.Optional(Type.Integer({ description: "Top-K hit count" })),
			}),
			ctx,
		),
	];
}
