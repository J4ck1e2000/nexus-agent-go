import type { AgentSession } from "@earendil-works/pi-coding-agent";
import type { RuntimeConfig } from "./config.ts";
import { createControlledSession } from "./session-factory.ts";
import { SessionBusyError } from "./runner.ts";

interface CacheEntry {
	session: AgentSession;
	activeToolNames: string[];
	lastUsed: number;
	busy: boolean;
}

/**
 * Bounded in-memory session cache keyed by the gateway session id.
 * Warm sessions continue their conversation naturally; cold sessions start
 * fresh with the gateway-supplied bounded context preamble.
 */
export class SessionCache {
	private config: RuntimeConfig;
	private entries = new Map<string, CacheEntry>();
	sweeper: NodeJS.Timeout;

	constructor(config: RuntimeConfig) {
		this.config = config;
		this.sweeper = setInterval(() => this.sweep(), Math.min(config.sessionTTLMSec / 2, 60_000));
		this.sweeper.unref?.();
	}

	stats(): { sessions: number; activeToolNames: string[] | null } {
		let activeToolNames: string[] | null = null;
		for (const entry of this.entries.values()) {
			activeToolNames = entry.activeToolNames;
		}
		return { sessions: this.entries.size, activeToolNames };
	}

	/** Acquires the session for one run; rejects concurrent runs on the same session. */
	async acquire(
		sessionID: string,
		runID: string,
		credential: string,
		enabledTools: string[],
	): Promise<CacheEntry> {
		const existing = this.entries.get(sessionID);
		if (existing) {
			if (existing.busy) {
				throw new SessionBusyError(sessionID);
			}
			existing.busy = true;
			existing.lastUsed = Date.now();
			return existing;
		}

		// Create outside the map so concurrent cold starts do not clobber.
		const created = await createControlledSession({
			config: this.config,
			runID,
			credential,
			enabledTools,
		});
		const entry: CacheEntry = {
			session: created.session,
			activeToolNames: created.activeToolNames,
			lastUsed: Date.now(),
			busy: true,
		};
		this.entries.set(sessionID, entry);
		this.evictOverflow();
		return entry;
	}

	release(sessionID: string, entry: CacheEntry, dispose: boolean): void {
		entry.busy = false;
		entry.lastUsed = Date.now();
		if (dispose) {
			this.entries.delete(sessionID);
			entry.session.dispose();
		}
	}

	disposeAll(): void {
		for (const [id, entry] of this.entries) {
			entry.session.dispose();
			this.entries.delete(id);
		}
	}

	private evictOverflow(): void {
		while (this.entries.size > this.config.sessionCacheSize) {
			let oldestKey: string | null = null;
			let oldest = Number.POSITIVE_INFINITY;
			for (const [id, entry] of this.entries) {
				if (entry.busy) continue;
				if (entry.lastUsed < oldest) {
					oldest = entry.lastUsed;
					oldestKey = id;
				}
			}
			if (oldestKey === null) break;
			const entry = this.entries.get(oldestKey);
			this.entries.delete(oldestKey);
			entry?.session.dispose();
		}
	}

	private sweep(): void {
		const cutoff = Date.now() - this.config.sessionTTLMSec;
		for (const [id, entry] of this.entries) {
			if (!entry.busy && entry.lastUsed < cutoff) {
				this.entries.delete(id);
				entry.session.dispose();
			}
		}
	}
}
