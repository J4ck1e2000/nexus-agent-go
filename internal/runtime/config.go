// Package runtime defines the integration boundary between the Go gateway and
// the external TypeScript Pi Runtime: configuration, wire events, run
// credentials, run lifecycle management and the NDJSON streaming client.
package runtime

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// EnvRuntimeURL is the base URL of the Pi Runtime service.
	EnvRuntimeURL = "PI_RUNTIME_URL"
	// EnvRuntimeToken is the shared secret between gateway and runtime.
	EnvRuntimeToken = "PI_RUNTIME_TOKEN"
	// EnvRunTokenSecret signs run credentials for tool callbacks.
	EnvRunTokenSecret = "PI_RUN_TOKEN_SECRET"
	// EnvRunTimeoutSec bounds one full run (model + tools).
	EnvRunTimeoutSec = "PI_RUN_TIMEOUT_SEC"
	// EnvToolTimeoutSec bounds one tool callback.
	EnvToolTimeoutSec = "PI_TOOL_TIMEOUT_SEC"
	// EnvMaxToolCalls bounds tool invocations per run.
	EnvMaxToolCalls = "PI_MAX_TOOL_CALLS"
)

// Default limits for gateway-side run handling.
const (
	DefaultRunTimeout    = 120 * time.Second
	DefaultToolTimeout   = 10 * time.Second
	DefaultMaxToolCalls  = 12
	runRecordTTL         = 5 * time.Minute
	managerGCInterval    = time.Minute
	protocolVersion      = "1"
	defaultScannerBuffer = 64 * 1024
	maxScannerBuffer     = 1024 * 1024
)

// Config carries gateway-side Pi runtime integration settings.
type Config struct {
	Enabled        bool
	RuntimeURL     string
	RuntimeToken   string
	RunTokenSecret string
	RunTimeout     time.Duration
	ToolTimeout    time.Duration
	MaxToolCalls   int
}

// LoadConfigFromEnv builds the gateway-side runtime config.
// Enabled is true when the required Pi Runtime URL is configured.
func LoadConfigFromEnv() Config {
	cfg := Config{
		RuntimeURL:     strings.TrimRight(strings.TrimSpace(os.Getenv(EnvRuntimeURL)), "/"),
		RuntimeToken:   strings.TrimSpace(os.Getenv(EnvRuntimeToken)),
		RunTokenSecret: strings.TrimSpace(os.Getenv(EnvRunTokenSecret)),
		RunTimeout:     envDuration(EnvRunTimeoutSec, DefaultRunTimeout),
		ToolTimeout:    envDuration(EnvToolTimeoutSec, DefaultToolTimeout),
		MaxToolCalls:   envInt(EnvMaxToolCalls, DefaultMaxToolCalls),
	}
	if cfg.RunTokenSecret == "" {
		cfg.RunTokenSecret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if cfg.RunTokenSecret == "" {
		cfg.RunTokenSecret = "nexus-agent-run-secret"
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = DefaultRunTimeout
	}
	if cfg.ToolTimeout <= 0 {
		cfg.ToolTimeout = DefaultToolTimeout
	}
	if cfg.MaxToolCalls <= 0 {
		cfg.MaxToolCalls = DefaultMaxToolCalls
	}
	cfg.Enabled = cfg.RuntimeURL != ""
	return cfg
}

// ProtocolVersion is the current wire protocol version.
func ProtocolVersion() string { return protocolVersion }

func envDuration(key string, fallback time.Duration) time.Duration {
	seconds := envInt(key, 0)
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
