/**
 * Deterministic end-to-end verification of the Pi runtime without a real
 * model key (docs/pi-runtime-redesign.md §8.2):
 *
 *   test client ──► Pi runtime ──► mock OpenAI server (scripted turns)
 *                        │
 *                        └─► stub Go tool gateway (scripted data, header checks)
 *
 * Asserts the full NDJSON event sequence, tool-callback credentials, final
 * JSON answer contract and that no host/default tools leak into the session.
 *
 * Run: node test/e2e-mock.mjs
 */
import assert from "node:assert/strict";
import http from "node:http";
import { createRuntimeServer } from "../src/server.ts";

const RUNTIME_TOKEN = "e2e-internal-token";

// ---------------------------------------------------------------------------
// 1. Mock OpenAI Chat Completions server (supports streaming SSE + plain JSON)
// ---------------------------------------------------------------------------
const FINAL_JSON = JSON.stringify({
	answer: "结论：node-03 的 GPU 当前空闲，可以直接投放任务。",
	reasoning_summary: "基于 get_node_metrics 的实时快照。",
	related_nodes: ["node-03"],
	warnings: [],
	knowledge_hits: [],
});

let chatRequests = 0;

function sseChunk(res, delta, finishReason = null) {
	res.write(
		`data: ${JSON.stringify({
			id: "chatcmpl-e2e",
			object: "chat.completion.chunk",
			created: Math.floor(Date.now() / 1000),
			model: "mock-model",
			choices: [{ index: 0, delta, finish_reason: finishReason }],
		})}\n\n`,
	);
}

const mockLLMServer = http.createServer((req, res) => {
	if (!req.url || !req.url.endsWith("/chat/completions")) {
		res.writeHead(404).end();
		return;
	}
	let body = "";
	req.on("data", (chunk) => (body += chunk));
	req.on("end", () => {
		chatRequests += 1;
		const parsed = JSON.parse(body);
		const wantsStream = parsed.stream === true;

		if (chatRequests === 1) {
			// First turn: the model asks for a tool.
			const toolCall = {
				index: 0,
				id: "call_e2e_1",
				type: "function",
				function: { name: "get_node_metrics", arguments: JSON.stringify({ node_name: "node-03" }) },
			};
			if (wantsStream) {
				res.writeHead(200, { "Content-Type": "text/event-stream" });
				sseChunk(res, { role: "assistant", tool_calls: [toolCall] });
				sseChunk(res, {}, "tool_calls");
				res.write("data: [DONE]\n\n");
				res.end();
			} else {
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(
					JSON.stringify({
						id: "chatcmpl-e2e",
						object: "chat.completion",
						created: Math.floor(Date.now() / 1000),
						model: "mock-model",
						choices: [
							{ index: 0, message: { role: "assistant", content: null, tool_calls: [toolCall] }, finish_reason: "tool_calls" },
						],
						usage: { prompt_tokens: 20, completion_tokens: 10, total_tokens: 30 },
					}),
				);
			}
			return;
		}

		// Second turn: final JSON answer streamed as text deltas.
		if (wantsStream) {
			res.writeHead(200, { "Content-Type": "text/event-stream" });
			sseChunk(res, { role: "assistant", content: FINAL_JSON.slice(0, 10) });
			sseChunk(res, { content: FINAL_JSON.slice(10) });
			sseChunk(res, {}, "stop");
			res.write("data: [DONE]\n\n");
			res.end();
		} else {
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(
				JSON.stringify({
					id: "chatcmpl-e2e",
					object: "chat.completion",
					created: Math.floor(Date.now() / 1000),
					model: "mock-model",
					choices: [
						{ index: 0, message: { role: "assistant", content: FINAL_JSON }, finish_reason: "stop" },
					],
					usage: { prompt_tokens: 40, completion_tokens: 30, total_tokens: 70 },
				}),
			);
		}
	});
});
await new Promise((resolve) => mockLLMServer.listen(0, "127.0.0.1", resolve));
const llmPort = mockLLMServer.address().port;

// ---------------------------------------------------------------------------
// 2. Stub Go tool gateway: verifies run credential + internal token headers
// ---------------------------------------------------------------------------
const toolCalls = [];
const toolGateway = http.createServer((req, res) => {
	assert.equal(req.headers["x-nexus-internal-token"], RUNTIME_TOKEN, "tool callback must carry internal token");
	assert.match(req.headers.authorization ?? "", /^Bearer run-e2e-/, "tool callback must carry run credential");
	assert.equal(req.method, "POST");
	const toolName = decodeURIComponent(req.url.split("/").pop());
	let body = "";
	req.on("data", (chunk) => (body += chunk));
	req.on("end", () => {
		toolCalls.push({ toolName, body: JSON.parse(body || "{}") });
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(
			JSON.stringify({
				ok: true,
				data: {
					node: {
						name: "node-03",
						status: "online",
						gpu_summary: { gpu_count: 2, busy_gpu_count: 0, idle_gpu_count: 2 },
					},
				},
				meta: { observed_at_unix: 1700000000, retrieved_at_unix: 1700000010, stale: false, truncated: false },
			}),
		);
	});
});
await new Promise((resolve) => toolGateway.listen(0, "127.0.0.1", resolve));
const toolPort = toolGateway.address().port;

// ---------------------------------------------------------------------------
// 3. Start the Pi runtime wired to the mocks
// ---------------------------------------------------------------------------
process.env.PI_RUNTIME_TOKEN = RUNTIME_TOKEN;
process.env.GATEWAY_TOOL_URL = `http://127.0.0.1:${toolPort}`;
process.env.GATEWAY_INTERNAL_TOKEN = RUNTIME_TOKEN;
process.env.AI_BASE_URL = `http://127.0.0.1:${llmPort}/v1`;
process.env.AI_API_KEY = "e2e-dummy-key";
process.env.AI_MODEL = "mock-model";
process.env.PI_SESSION_TTL_SEC = "60";

const runtime = createRuntimeServer({ port: 0 });
await runtime.start();
const runtimePort = runtime.port;

// ---------------------------------------------------------------------------
// 4. Health check: model configured, only Nexus tools active
// ---------------------------------------------------------------------------
const health = await fetch(`http://127.0.0.1:${runtimePort}/healthz`).then((r) => r.json());
assert.equal(health.status, "ok");
assert.equal(health.model, "mock-model");

// ---------------------------------------------------------------------------
// 5. Run one query end to end
// ---------------------------------------------------------------------------
const runResponse = await fetch(`http://127.0.0.1:${runtimePort}/v1/runs`, {
	method: "POST",
	headers: {
		"Content-Type": "application/json",
		"X-Nexus-Internal-Token": RUNTIME_TOKEN,
		Authorization: "Bearer run-e2e-credential",
	},
	body: JSON.stringify({
		protocol_version: "1",
		run_id: "run-e2e-1",
		session_id: "run-e2e-1",
		input: { message: "node-03 现在空闲吗？" },
		context: { known_nodes: ["node-03"], locale_hint: "zh", recent_entries: [] },
		policy: {
			enabled_tool_names: [
				"get_node_metrics",
				"list_idle_nodes",
				"get_gpu_processes",
				"get_node_summary",
				"get_alert_history",
				"recommend_nodes_for_job",
				"explain_node_anomaly",
				"search_knowledge_base",
			],
			deadline_at: new Date(Date.now() + 30_000).toISOString(),
		},
		model_profile: { provider: "nexus-llm", model_id: "mock-model" },
	}),
});
assert.equal(runResponse.status, 200, "start run must succeed");
assert.match(runResponse.headers.get("content-type") ?? "", /ndjson/);

const events = [];
const reader = runResponse.body.getReader();
const decoder = new TextDecoder();
let buffer = "";
for (;;) {
	const { done, value } = await reader.read();
	if (done) break;
	buffer += decoder.decode(value, { stream: true });
	let newline;
	while ((newline = buffer.indexOf("\n")) >= 0) {
		const line = buffer.slice(0, newline).trim();
		buffer = buffer.slice(newline + 1);
		if (line) events.push(JSON.parse(line));
	}
}

const eventTypes = events.map((e) => e.type);
console.log("[e2e] events:", eventTypes.join(" -> "));

// Sequence assertions: started, one tool round-trip, deltas, completed.
assert.equal(eventTypes[0], "run.started");
assert.ok(eventTypes.includes("tool.started"), "model must trigger a tool call");
assert.ok(eventTypes.includes("tool.completed"), "tool result must feed back");
assert.ok(eventTypes.includes("message.delta"), "final answer must stream");
assert.equal(eventTypes[eventTypes.length - 1], "run.completed", "run must end with run.completed");

const toolStart = events.find((e) => e.type === "tool.started");
assert.equal(toolStart.payload.tool_name, "get_node_metrics");
assert.equal(toolStart.payload.arguments.node_name, "node-03");

const toolDone = events.find((e) => e.type === "tool.completed");
assert.equal(toolDone.payload.ok, true);

// Monotonic seq per run.
for (let i = 1; i < events.length; i += 1) {
	assert.ok(events[i].seq > events[i - 1].seq, "seq must be strictly increasing");
}

// Final payload must carry the model's JSON answer.
const completed = events.find((e) => e.type === "run.completed");
const finalText = completed.payload.text;
const parsedFinal = JSON.parse(finalText);
assert.match(parsedFinal.answer, /node-03/);
assert.ok(parsedFinal.reasoning_summary.length > 0);

// Tool gateway saw exactly one callback with matching arguments.
assert.equal(toolCalls.length, 1);
assert.equal(toolCalls[0].body.run_id, "run-e2e-1");
assert.equal(toolCalls[0].body.arguments.node_name, "node-03");

// Exactly two model turns: tool request + final answer.
assert.equal(chatRequests, 2, `expected 2 model turns, got ${chatRequests}`);

// ---------------------------------------------------------------------------
// 6. Auth enforcement
// ---------------------------------------------------------------------------
const unauthorized = await fetch(`http://127.0.0.1:${runtimePort}/v1/runs`, { method: "POST" });
assert.equal(unauthorized.status, 401, "missing internal token must be rejected");

// ---------------------------------------------------------------------------
// 7. Active tool names must be exactly the Nexus read-only set (no host tools)
// ---------------------------------------------------------------------------
const healthAfter = await fetch(`http://127.0.0.1:${runtimePort}/healthz`).then((r) => r.json());
const activeTools = new Set(healthAfter.active_tools ?? []);
for (const expected of ["get_node_metrics", "list_idle_nodes", "get_gpu_processes", "get_node_summary"]) {
	assert.ok(activeTools.has(expected), `expected ${expected} active`);
}
for (const forbidden of ["bash", "read", "edit", "write"]) {
	assert.ok(!activeTools.has(forbidden), `default tool ${forbidden} must not be active`);
}

await runtime.close();
mockLLMServer.close();
toolGateway.close();

console.log("[e2e] PASS: full loop verified (model -> tool -> result -> final JSON answer)");
