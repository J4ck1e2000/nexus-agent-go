import type { AgentSession, AgentSessionEvent } from "@earendil-works/pi-coding-agent";
import type { RuntimeConfig } from "./config.ts";
import {
	isTerminalEvent,
	PROTOCOL_VERSION,
	type RunEvent,
	type RunFailedPayload,
	type RunCancelledPayload,
	type RunCompletedPayload,
	type StartRunRequest,
	type ToolCompletedPayload,
	type ToolStartedPayload,
} from "./protocol.ts";

export class RunLimitExceeded extends Error {
	constructor(message: string) {
		super(message);
		this.name = "RunLimitExceeded";
	}
}

export class SessionBusyError extends Error {
	constructor(sessionID: string) {
		super(`session ${sessionID} already has an active run`);
		this.name = "SessionBusyError";
	}
}

/**
 * Executes one run against a controlled Pi session and writes NDJSON events.
 * The stable end signal is agent_settled / prompt() resolution (SDK docs:
 * agent_end may still be followed by automatic work).
 */
export class RunExecutor {
	private config: RuntimeConfig;

	constructor(config: RuntimeConfig) {
		this.config = config;
	}

	async execute(
		request: StartRunRequest,
		credential: string,
		session: AgentSession,
		sink: (event: RunEvent) => void,
	): Promise<void> {
		const runID = request.run_id;
		let seq = 0;
		let toolCalls = 0;
		let aborted = false;
		let limitError: string | null = null;

		const emit = (type: string, payload?: unknown) => {
			seq += 1;
			sink({
				protocol_version: PROTOCOL_VERSION,
				run_id: runID,
				seq,
				type,
				timestamp: new Date().toISOString(),
				payload,
			});
		};

		// Deadline: prefer the gateway-supplied policy deadline, fall back to config.
		const policyDeadline = request.policy?.deadline_at ? Date.parse(request.policy.deadline_at) : NaN;
		const deadlineMSec = Number.isFinite(policyDeadline)
			? policyDeadline
			: Date.now() + this.config.runTimeoutSec * 1000;
		const remainingMSec = deadlineMSec - Date.now();
		const deadlineTimer = setTimeout(() => {
			limitError = "deadline_exceeded";
			aborted = true;
			void session.abort();
		}, Math.max(remainingMSec, 0));

		const unsubscribe = session.subscribe((event: AgentSessionEvent) => {
			this.handleSessionEvent(event, emit, () => {
				toolCalls += 1;
				if (toolCalls > this.config.maxToolCalls) {
					limitError = "budget_exceeded";
					aborted = true;
					void session.abort();
				}
			});
		});

		emit("run.started", {});

		try {
			const prompt = this.buildPrompt(request);
			await session.prompt(prompt);
			if (aborted && limitError) {
				const payload: RunFailedPayload = { error: "budget_exceeded", details: limitError };
				emit("run.failed", payload);
				return;
			}
			if (aborted) {
				const payload: RunCancelledPayload = { reason: "aborted" };
				emit("run.cancelled", payload);
				return;
			}
			const finalText = session.getLastAssistantText?.() ?? "";
			const payload: RunCompletedPayload = { text: finalText, finish_reason: "stop" };
			emit("run.completed", payload);
		} catch (error) {
			const message = error instanceof Error ? error.message : String(error);
			if (aborted) {
				const payload: RunFailedPayload | RunCancelledPayload =
					limitError
						? { error: "budget_exceeded", details: limitError }
						: { reason: "aborted" };
				emit(limitError ? "run.failed" : "run.cancelled", payload);
				return;
			}
			emit("run.failed", { error: message } satisfies RunFailedPayload);
		} finally {
			clearTimeout(deadlineTimer);
			unsubscribe();
		}
	}

	private handleSessionEvent(
		event: AgentSessionEvent,
		emit: (type: string, payload?: unknown) => void,
		onToolStart: () => void,
	): void {
		switch (event.type) {
			case "message_update": {
				const update = event.assistantMessageEvent;
				if (update?.type === "text_delta" && typeof update.delta === "string" && update.delta.length > 0) {
					emit("message.delta", { text: update.delta });
				}
				return;
			}
			case "message_end": {
				const message = event.message;
				if (message?.role === "assistant") {
					const text = extractAssistantText(message);
					if (text) {
						emit("message.completed", { text });
					}
				}
				return;
			}
			case "tool_execution_start": {
				onToolStart();
				emit("tool.started", {
					tool_call_id: event.toolCallId,
					tool_name: event.toolName,
					arguments: event.args ?? {},
				} satisfies ToolStartedPayload);
				return;
			}
			case "tool_execution_end": {
				emit("tool.completed", {
					tool_call_id: event.toolCallId,
					tool_name: event.toolName,
					ok: !event.isError,
					result: sanitizeToolResult(event.result),
					error: event.isError ? extractToolErrorText(event.result) : undefined,
				} satisfies ToolCompletedPayload);
				return;
			}
			default:
			// Unknown/session-level events are ignored for forward compatibility.
		}
	}

	/** Bounded context recovery: a compact preamble of recent entries. */
	private buildPrompt(request: StartRunRequest): string {
		const entries = request.context?.recent_entries ?? [];
		if (entries.length === 0) {
			return request.input.message;
		}
		const maxEntries = 6;
		const maxChars = 4000;
		const recent = entries.slice(-maxEntries);
		let transcript = "";
		for (const entry of recent) {
			const line = `${entry.role}: ${entry.content}`.slice(0, 600);
			if (transcript.length + line.length > maxChars) break;
			transcript += `${line}\n`;
		}
		if (!transcript.trim()) {
			return request.input.message;
		}
		return [
			"[Conversation context recovered from the gateway — treat as background, refresh live data via tools when freshness matters]",
			transcript.trimEnd(),
			"[/Conversation context]",
			"",
			request.input.message,
		].join("\n");
	}
}

function extractAssistantText(message: { content?: unknown }): string {
	const content = message.content;
	if (typeof content === "string") {
		return content;
	}
	if (Array.isArray(content)) {
		return content
			.filter((block): block is { type: string; text?: string } => typeof block === "object" && block !== null)
			.filter((block) => block.type === "text" && typeof block.text === "string")
			.map((block) => block.text as string)
			.join("");
	}
	return "";
}

/** Tool results are trimmed to a bounded preview for the event stream. */
function sanitizeToolResult(result: unknown): Record<string, unknown> | undefined {
	if (result === undefined || result === null) {
		return undefined;
	}
	if (typeof result === "object" && !Array.isArray(result)) {
		return result as Record<string, unknown>;
	}
	return { value: String(result).slice(0, 512) };
}

function extractToolErrorText(result: unknown): string {
	if (typeof result === "object" && result !== null) {
		const record = result as Record<string, unknown>;
		const content = record.content;
		if (Array.isArray(content)) {
			for (const block of content) {
				if (
					typeof block === "object" &&
					block !== null &&
					(block as Record<string, unknown>).type === "text"
				) {
					const text = (block as Record<string, unknown>).text;
					if (typeof text === "string") {
						return text.slice(0, 300);
					}
				}
			}
		}
	}
	return "tool failed";
}
