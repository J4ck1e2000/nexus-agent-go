import os from "node:os";
import path from "node:path";
import fs from "node:fs";

export interface RuntimeConfig {
	port: number;
	token: string;
	gatewayToolURL: string;
	gatewayToken: string;
	toolTimeoutSec: number;
	runTimeoutSec: number;
	maxToolCalls: number;
	sessionTTLMSec: number;
	sessionCacheSize: number;
	/** AI model endpoint configuration (OpenAI-compatible). */
	baseURL: string;
	apiKey: string;
	modelID: string;
	/** Private directory for models.json / auth.json. */
	stateDir: string;
}

function intEnv(name: string, fallback: number): number {
	const raw = process.env[name];
	if (!raw) return fallback;
	const parsed = Number.parseInt(raw, 10);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function requiredEnv(name: string, fallback: string): string {
	const raw = process.env[name];
	if (raw && raw.trim() !== "") return raw.trim();
	if (fallback !== undefined) return fallback;
	throw new Error(`missing required environment variable ${name}`);
}

export function loadConfig(): RuntimeConfig {
	const port = intEnv("PI_RUNTIME_PORT", 8010);
	const token = requiredEnv("PI_RUNTIME_TOKEN", "nexus-pi-internal-token");
	const gatewayToolURL = requiredEnv("GATEWAY_TOOL_URL", "http://127.0.0.1:3000").replace(/\/+$/, "");
	const gatewayToken = requiredEnv("GATEWAY_INTERNAL_TOKEN", token);
	const baseURL = requiredEnv("AI_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1");
	const apiKey = requiredEnv("AI_API_KEY", "");
	const modelID = requiredEnv("AI_MODEL", "");
	if (!modelID) {
		throw new Error("AI_MODEL is required for the Pi runtime");
	}
	if (!apiKey) {
		throw new Error("AI_API_KEY is required for the Pi runtime");
	}
	const stateDir = path.join(os.tmpdir(), "nexus-pi-runtime");
	fs.mkdirSync(stateDir, { recursive: true });
	return {
		port,
		token,
		gatewayToolURL,
		gatewayToken,
		toolTimeoutSec: intEnv("PI_TOOL_TIMEOUT_SEC", 10),
		runTimeoutSec: intEnv("PI_RUN_TIMEOUT_SEC", 120),
		maxToolCalls: intEnv("PI_MAX_TOOL_CALLS", 12),
		sessionTTLMSec: intEnv("PI_SESSION_TTL_SEC", 1800) * 1000,
		sessionCacheSize: intEnv("PI_SESSION_CACHE_SIZE", 128),
		baseURL,
		apiKey,
		modelID,
		stateDir,
	};
}

/** models.json content that registers the OpenAI-compatible endpoint. */
export function modelsJSON(config: RuntimeConfig): string {
	return JSON.stringify({
		providers: {
			"nexus-llm": {
				baseUrl: config.baseURL,
				api: "openai-completions",
				apiKey: config.apiKey,
				models: [{ id: config.modelID }],
			},
		},
	});
}

export function writeModelsFile(config: RuntimeConfig): string {
	const modelsPath = path.join(config.stateDir, "models.json");
	fs.writeFileSync(modelsPath, modelsJSON(config), { encoding: "utf8", mode: 0o600 });
	return modelsPath;
}
