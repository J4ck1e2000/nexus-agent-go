// Shared IPC contract between Electron main, preload and renderer.
// This file must stay free of any `electron` imports so both sides can use it.

export type UserRole = 'admin' | 'user';

export interface NexusUser {
  id: number;
  username: string;
  role: UserRole;
}

export interface UserRecord extends NexusUser {
  created_at?: string;
  updated_at?: string;
}

export type GatewayErrorCode =
  | 'network'
  | 'timeout'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'bad_request'
  | 'server_error'
  | 'gateway_error'
  | 'invalid_response'
  | 'invalid_input'
  | 'aborted';

export interface NexusError {
  code: GatewayErrorCode;
  status?: number;
  /** Machine-readable error code reported by the Gateway, e.g. "user_already_exists". */
  detail?: string;
  /** Technical description for the dev console. Must never contain secrets. */
  message: string;
}

export type NexusResult<T> = { ok: true; data: T } | { ok: false; error: NexusError };

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

export interface LoginPayload {
  username: string;
  password: string;
}

export interface RegisterPayload {
  username: string;
  password: string;
}

// ---------------------------------------------------------------------------
// Gateway connection settings
// ---------------------------------------------------------------------------

export interface DesktopSettings {
  gatewayUrl: string;
}

export interface TestConnectionResult {
  versionName: string;
  changelog: string;
}

// ---------------------------------------------------------------------------
// Node / GPU monitoring data (mirrors Gateway JSON payloads)
// ---------------------------------------------------------------------------

export type NodeStatus = 'pending' | 'online' | 'offline' | (string & {});

export type AvailabilityTier =
  | 'highlyAvailable'
  | 'available'
  | 'busy'
  | 'saturated'
  | 'offline'
  | (string & {});

export type CollectorType = 'ssh' | 'agent' | (string & {});

export interface AgentConfig {
  id: number;
  name: string;
  /**
   * Present for collector_type=agent; ssh nodes omit it (Go omitempty).
   * SSH nodes surface "ssh://user@host:port" in NodeOverview.url instead.
   */
  url?: string;
  /** Absent in legacy gateway payloads; treated as "agent". */
  collector_type?: CollectorType;
  ssh_host?: string;
  ssh_port?: number;
  ssh_user?: string;
  ssh_auth_type?: 'key';
}

export interface GpuInfo {
  id: number;
  name: string;
  temperature: number;
  fan_speed: number;
  power_draw: number;
  utilization: number;
  memory_total: number;
  memory_used: number;
  memory_utilization: number;
}

export interface ProcessInfo {
  pid: number;
  user: string;
  command: string;
  cpu_percent: number;
  memory_percent: number;
  gpu_index?: number | null;
  vram_used_mb?: number | null;
}

export interface SystemMetrics {
  hostname: string;
  ip_address: string;
  os: string;
  uptime_seconds: number;
  uptime_human: string;
  cpu_model: string;
  cpu_usage: number;
  cpu_cores: number;
  ram_total: number;
  ram_used: number;
  ram_percent: number;
  net_sent_mb: number;
  net_recv_mb: number;
  gpus: GpuInfo[];
  processes: ProcessInfo[];
}

export interface NodeNetworkMetrics {
  upSpeed: number;
  downSpeed: number;
}

export interface NodeGPUSummary {
  gpuCount: number;
  busyGpuCount: number;
  idleGpuCount: number;
  avgUtilization: number | null;
  avgMemoryPercent: number | null;
  totalMemoryUsed: number;
  totalMemory: number;
  totalPower: number;
  gpuPressure: number;
  busyRatio: number;
}

export interface NodeOverview {
  id: number;
  name: string;
  url: string;
  status: NodeStatus;
  data?: SystemMetrics | null;
  metrics?: NodeNetworkMetrics | null;
  lastSeenAt?: number | null;
  lastPolledAtUnix: number;
  collectedAtUnix: number;
  /** Go marshals nil slices as null, so both fields are nullable in practice. */
  activeUsers: string[] | null;
  activeUserCount: number | null;
  gpuSummary: NodeGPUSummary;
  availabilityScore: number;
  availabilityTier: AvailabilityTier;
  dataAgeSec?: number | null;
  error?: string;
}

export interface AddNodePayload {
  name: string;
  /** Collector to attach: "ssh" (default in the UI) or legacy "agent". */
  collector_type: CollectorType;
  /** Agent mode: agent base URL. */
  url?: string;
  /** SSH mode: connection target. Credentials never reach the desktop. */
  ssh_host?: string;
  ssh_port?: number;
  ssh_user?: string;
  ssh_auth_type?: 'key';
}

/** Response of POST /api/config/test-ssh. */
export interface TestSSHResult {
  ok: boolean;
  hostname: string;
  gpu_count: number;
  gpu_names: string[];
}

export interface TestSSHPayload {
  ssh_host: string;
  ssh_port: number;
  ssh_user: string;
}

// ---------------------------------------------------------------------------
// Admin users
// ---------------------------------------------------------------------------

export interface CreateUserPayload {
  username: string;
  password: string;
  role?: UserRole;
}

// ---------------------------------------------------------------------------
// AI assistant
// ---------------------------------------------------------------------------

export type AIStreamEventType = 'start' | 'status' | 'delta' | 'meta' | 'done' | 'error';

export interface AIStreamEvent {
  requestId: string;
  type: AIStreamEventType;
  data: unknown;
}

export interface ToolCallRecord {
  name: string;
  args: Record<string, unknown>;
}

export interface KnowledgeHitSummary {
  title: string;
  category?: string;
  snippet?: string;
  source_path?: string;
}

/** Payload of the SSE `meta` event. */
export interface AIStreamMeta {
  reasoning_summary?: string;
  mode?: string;
  tool_calls?: ToolCallRecord[];
  related_nodes?: string[];
  warnings?: string[];
  knowledge_hits?: KnowledgeHitSummary[];
}

/** Payload of the SSE `done` event (final response). */
export interface AIQueryResult {
  answer: string;
  reasoning_summary: string;
  mode: string;
  tool_calls?: ToolCallRecord[];
  related_nodes?: string[];
  warnings?: string[];
  knowledge_hits?: KnowledgeHitSummary[];
}

// ---------------------------------------------------------------------------
// Full preload API surface exposed as window.nexus
// ---------------------------------------------------------------------------

export type Unsubscribe = () => void;

export interface NexusAPI {
  auth: {
    login(payload: LoginPayload): Promise<NexusResult<NexusUser>>;
    register(payload: RegisterPayload): Promise<NexusResult<NexusUser>>;
    logout(): Promise<NexusResult<null>>;
    me(): Promise<NexusResult<NexusUser>>;
    onAuthExpired(cb: () => void): Unsubscribe;
  };
  nodes: {
    overview(): Promise<NexusResult<NodeOverview[]>>;
    list(): Promise<NexusResult<AgentConfig[]>>;
    add(payload: AddNodePayload): Promise<NexusResult<AgentConfig>>;
    remove(id: number): Promise<NexusResult<null>>;
    testSSH(payload: TestSSHPayload): Promise<NexusResult<TestSSHResult>>;
  };
  users: {
    list(q?: string): Promise<NexusResult<UserRecord[]>>;
    create(payload: CreateUserPayload): Promise<NexusResult<UserRecord>>;
    remove(id: number): Promise<NexusResult<null>>;
    setRole(id: number, role: UserRole): Promise<NexusResult<UserRecord>>;
    resetPassword(id: number, password: string): Promise<NexusResult<null>>;
  };
  ai: {
    start(requestId: string, query: string): Promise<NexusResult<null>>;
    cancel(requestId: string): Promise<NexusResult<null>>;
    onEvent(cb: (event: AIStreamEvent) => void): Unsubscribe;
  };
  settings: {
    get(): Promise<NexusResult<DesktopSettings>>;
    update(settings: DesktopSettings): Promise<NexusResult<DesktopSettings>>;
    testConnection(url: string): Promise<NexusResult<TestConnectionResult>>;
  };
}
