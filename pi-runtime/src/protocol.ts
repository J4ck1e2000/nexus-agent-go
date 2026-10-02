/**
 * Wire protocol types mirroring internal/runtime in the Go gateway
 * (docs/pi-runtime-redesign.md §3 and §4).
 */

export const PROTOCOL_VERSION = "1";

export interface StartRunInput {
	message: string;
}

export interface RunHistoryEntry {
	role: string;
	content: string;
}

export interface RunContext {
	known_nodes?: string[];
	locale_hint?: string;
	recent_entries?: RunHistoryEntry[];
}

export interface RunPolicy {
	enabled_tool_names: string[];
	deadline_at?: string;
}

export interface ModelProfile {
	provider: string;
	model_id?: string;
}

export interface StartRunRequest {
	protocol_version: string;
	run_id: string;
	session_id: string;
	input: StartRunInput;
	context: RunContext;
	policy: RunPolicy;
	model_profile?: ModelProfile;
}

export interface CancelRunRequest {
	reason?: string;
}

/** Nexus runtime event envelope; payload shapes are discriminated by `type`. */
export interface RunEvent {
	protocol_version: string;
	run_id: string;
	seq: number;
	type: string;
	timestamp: string;
	payload?: unknown;
}

export interface ToolStartedPayload {
	tool_call_id: string;
	tool_name: string;
	arguments?: Record<string, unknown>;
}

export interface ToolCompletedPayload {
	tool_call_id: string;
	tool_name: string;
	ok: boolean;
	result?: Record<string, unknown>;
	meta?: ToolCallMeta;
	error?: string;
}

export interface RunCompletedPayload {
	text?: string;
	finish_reason?: string;
}

export interface RunFailedPayload {
	error: string;
	details?: string;
}

export interface RunCancelledPayload {
	reason?: string;
}

/** Tool callback response from the Go tool gateway. */
export interface ToolCallMeta {
	observed_at_unix?: number;
	retrieved_at_unix?: number;
	stale?: boolean;
	truncated?: boolean;
}

export interface ToolCallError {
	code: string;
	message: string;
	retryable?: boolean;
}

export interface ToolCallResponse {
	ok: boolean;
	data?: Record<string, unknown>;
	meta?: ToolCallMeta;
	error?: ToolCallError;
}

export function isTerminalEvent(type: string): boolean {
	return (
		type === "run.completed" ||
		type === "run.failed" ||
		type === "run.cancelled"
	);
}
