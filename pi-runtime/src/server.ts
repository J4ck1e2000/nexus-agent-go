import http from "node:http";
import { loadConfig, type RuntimeConfig } from "./config.ts";
import { RunExecutor, SessionBusyError } from "./runner.ts";
import { SessionCache } from "./session-cache.ts";
import { GatewayToolClient } from "./gateway-client.ts";
import { buildNexusTools } from "./nexus-tools.ts";
import { isTerminalEvent, type RunEvent, type StartRunRequest } from "./protocol.ts";

export interface RuntimeServer {
	start(): Promise<void>;
	close(): Promise<void>;
	port: number;
}

export function createRuntimeServer(overrides: Partial<RuntimeConfig> = {}): RuntimeServer {
	const config = { ...loadConfig(), ...overrides };
	const cache = new SessionCache(config);
	const executor = new RunExecutor(config);
	let actualPort = config.port;
	// Active run registry so cancellation can reach the running session.
	const activeRuns = new Map<
		string,
		{ sessionID: string; entry: { session: import("@earendil-works/pi-coding-agent").AgentSession }; finished: boolean }
	>();

	function authorize(req: http.IncomingMessage): boolean {
		const token = req.headers["x-nexus-internal-token"];
		return typeof token === "string" && token === config.token;
	}

	const server = http.createServer(async (req, res) => {
		const url = new URL(req.url ?? "/", `http://127.0.0.1:${actualPort}`);

		if (url.pathname === "/healthz") {
			const stats = cache.stats();
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(
				JSON.stringify({
					status: "ok",
					model: config.modelID,
					sessions: stats.sessions,
					active_tools: stats.activeToolNames,
				}),
			);
			return;
		}

		if (!authorize(req)) {
			res.writeHead(401, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: "unauthorized" }));
			return;
		}

		if (req.method === "POST" && url.pathname === "/v1/runs") {
			await handleStartRun(req, res);
			return;
		}

		if (req.method === "POST" && url.pathname.startsWith("/v1/runs/") && url.pathname.endsWith("/cancel")) {
			const runID = url.pathname.split("/")[3];
			const active = activeRuns.get(runID);
			if (active && !active.finished) {
				void active.entry.session.abort();
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ status: "cancel_signalled", run_id: runID }));
			} else {
				res.writeHead(404, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ error: "run_not_active" }));
			}
			return;
		}

		res.writeHead(404, { "Content-Type": "application/json" });
		res.end(JSON.stringify({ error: "not_found" }));
	});

	async function handleStartRun(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
		const chunks: Buffer[] = [];
		for await (const chunk of req) {
			chunks.push(chunk as Buffer);
		}
		let request: StartRunRequest;
		try {
			request = JSON.parse(Buffer.concat(chunks).toString("utf8")) as StartRunRequest;
		} catch {
			res.writeHead(400, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: "invalid_payload" }));
			return;
		}

		if (
			request.protocol_version !== "1" ||
			typeof request.run_id !== "string" ||
			request.run_id === "" ||
			typeof request.input?.message !== "string" ||
			request.input.message.trim() === ""
		) {
			res.writeHead(400, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: "invalid_payload" }));
			return;
		}

		const credential = extractBearer(req.headers.authorization);
		if (!credential) {
			res.writeHead(401, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: "run_credential_required" }));
			return;
		}

		const sessionID = request.session_id || request.run_id;
		const enabledTools =
			Array.isArray(request.policy?.enabled_tool_names) && request.policy.enabled_tool_names.length > 0
				? request.policy.enabled_tool_names
				: buildNexusTools({
						runID: request.run_id,
						credential,
						client: new GatewayToolClient(config),
						toolTimeoutSec: config.toolTimeoutSec,
					}).map((tool) => tool.name);

		let entry;
		try {
			entry = await cache.acquire(sessionID, request.run_id, credential, enabledTools);
		} catch (error) {
			if (error instanceof SessionBusyError) {
				res.writeHead(429, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ error: "session_busy" }));
				return;
			}
			res.writeHead(500, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: error instanceof Error ? error.message : "session_create_failed" }));
			return;
		}

		// NDJSON event stream; one run per HTTP response.
		res.writeHead(200, {
			"Content-Type": "application/x-ndjson; charset=utf-8",
			"Cache-Control": "no-cache",
			"X-Accel-Buffering": "no",
		});
		let closed = false;
		const active = { sessionID, entry, finished: false };
		activeRuns.set(request.run_id, active);
		res.on("close", () => {
			closed = true;
			if (!active.finished) {
				// Gateway cancelled or connection dropped: stop the session promptly.
				void entry.session.abort();
			}
		});

		const sink = (event: RunEvent) => {
			if (closed) return;
			res.write(`${JSON.stringify(event)}\n`);
			if (isTerminalEvent(event.type)) {
				active.finished = true;
				activeRuns.delete(request.run_id);
				res.end();
				closed = true;
			}
		};

		let dispose = false;
		try {
			await executor.execute(request, credential, entry.session, sink);
		} catch (error) {
			// executor.execute handles its own terminal events; a thrown error here
			// means the run never reached one.
			if (!closed) {
				active.finished = true;
				sink({
					protocol_version: "1",
					run_id: request.run_id,
					seq: Number.MAX_SAFE_INTEGER,
					type: "run.failed",
					timestamp: new Date().toISOString(),
					payload: { error: error instanceof Error ? error.message : String(error) },
				});
			}
			// A poisoned session is disposed so the next run starts clean.
			dispose = true;
		} finally {
			activeRuns.delete(request.run_id);
			cache.release(sessionID, entry, dispose);
		}
	}

	return {
		get port() {
			return actualPort;
		},
		async start() {
			await new Promise<void>((resolve) => {
				server.listen(config.port, "127.0.0.1", () => {
					const address = server.address();
					if (address && typeof address === "object") {
						actualPort = address.port;
					}
					resolve();
				});
			});
		},
		async close() {
			cache.disposeAll();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		},
	};
}

function extractBearer(header: string | undefined): string | null {
	if (!header) return null;
	const parts = header.split(" ");
	if (parts.length !== 2 || parts[0].toLowerCase() !== "bearer" || parts[1].trim() === "") {
		return null;
	}
	return parts[1].trim();
}

// Standalone entrypoint: node src/server.ts
if (process.argv[1] && process.argv[1].endsWith("server.ts")) {
	const server = createRuntimeServer();
	server
		.start()
		.then(() => {
			console.log(`[pi-runtime] listening on 127.0.0.1:${server.port}`);
		})
		.catch((error) => {
			console.error("[pi-runtime] startup failed:", error);
			process.exit(1);
		});

	const shutdown = () => {
		void server.close().then(() => process.exit(0));
	};
	process.on("SIGINT", shutdown);
	process.on("SIGTERM", shutdown);
}
