import type { RuntimeConfig } from "./config.ts";
import type { ToolCallResponse } from "./protocol.ts";

export class ToolExecutionError extends Error {
	code: string;
	retryable: boolean;

	constructor(code: string, message: string, retryable = false) {
		super(message);
		this.name = "ToolExecutionError";
		this.code = code;
		this.retryable = retryable;
	}
}

/**
 * Calls the Go tool gateway on behalf of one run.
 * Business failures arrive as HTTP 200 with ok:false and are surfaced as
 * ToolExecutionError so the model can react; HTTP/auth failures propagate.
 */
export class GatewayToolClient {
	private config: RuntimeConfig;

	constructor(config: RuntimeConfig) {
		this.config = config;
	}

	async call(
		runID: string,
		credential: string,
		toolName: string,
		toolCallID: string,
		args: Record<string, unknown>,
		timeoutSec: number,
	): Promise<ToolCallResponse> {
		const controller = new AbortController();
		const timer = setTimeout(() => controller.abort(), timeoutSec * 1000);
		try {
			const response = await fetch(
				`${this.config.gatewayToolURL}/internal/api/tools/${encodeURIComponent(toolName)}`,
				{
					method: "POST",
					headers: {
						"Content-Type": "application/json",
						"X-Nexus-Internal-Token": this.config.gatewayToken,
						Authorization: `Bearer ${credential}`,
					},
					body: JSON.stringify({
						run_id: runID,
						tool_call_id: toolCallID,
						arguments: args ?? {},
					}),
					signal: controller.signal,
				},
			);
			if (response.status === 401) {
				throw new ToolExecutionError("AUTH_FAILED", "tool gateway rejected the run credential");
			}
			if (response.status === 403) {
				const body = (await response.json().catch(() => ({}))) as { error?: string };
				throw new ToolExecutionError("FORBIDDEN", `tool call forbidden: ${body.error ?? "policy"}`);
			}
			if (response.status === 404) {
				throw new ToolExecutionError("UNKNOWN_TOOL", `tool ${toolName} is not dispatchable`);
			}
			if (response.status === 409) {
				const body = (await response.json().catch(() => ({}))) as { error?: string };
				throw new ToolExecutionError("RUN_NOT_ACTIVE", `run rejected: ${body.error ?? "conflict"}`);
			}
			if (!response.ok) {
				throw new ToolExecutionError("GATEWAY_ERROR", `tool gateway status ${response.status}`);
			}
			const payload = (await response.json()) as ToolCallResponse;
			if (!payload.ok) {
				const error = payload.error ?? { code: "TOOL_INTERNAL", message: "tool failed" };
				throw new ToolExecutionError(error.code, error.message, error.retryable ?? false);
			}
			return payload;
		} finally {
			clearTimeout(timer);
		}
	}
}
