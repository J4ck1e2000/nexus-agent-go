package gateway

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisAddr        = "127.0.0.1:6379"
	defaultRedisDB          = 0
	defaultRedisKeyPrefix   = "nexus"
	defaultGatewayPollIntvl = 2 * time.Second
	defaultGatewayPollTO    = 3 * time.Second
	defaultNodeStateTTL     = 10 * time.Minute
	defaultRedisPingTimeout = 5 * time.Second
)

// RedisConfig 描述 Gateway 的 Redis 连接配置。
type RedisConfig struct {
	Addr      string
	Password  string
	DB        int
	KeyPrefix string
}

// GatewayRuntimeConfig 描述 poller 与热状态层运行参数。
type GatewayRuntimeConfig struct {
	Redis        RedisConfig
	PollInterval time.Duration
	PollTimeout  time.Duration
	NodeStateTTL time.Duration
}

// LoadGatewayRuntimeConfigFromEnv 从环境变量读取网关运行时配置。
func LoadGatewayRuntimeConfigFromEnv() GatewayRuntimeConfig {
	return GatewayRuntimeConfig{
		Redis: RedisConfig{
			Addr:      envOrString("REDIS_ADDR", defaultRedisAddr),
			Password:  strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
			DB:        envOrInt("REDIS_DB", defaultRedisDB),
			KeyPrefix: envOrString("REDIS_KEY_PREFIX", defaultRedisKeyPrefix),
		},
		PollInterval: envOrDuration("GATEWAY_POLL_INTERVAL", defaultGatewayPollIntvl),
		PollTimeout:  envOrDuration("GATEWAY_POLL_TIMEOUT", defaultGatewayPollTO),
		NodeStateTTL: envOrDuration("NODE_STATE_TTL", defaultNodeStateTTL),
	}
}

// NewRedisClient 创建并校验 Redis 客户端（启动时会执行 PING）。
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*redis.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		addr = defaultRedisAddr
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	pingCtx, cancel := context.WithTimeout(ctx, defaultRedisPingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed (%s): %w", addr, err)
	}

	return client, nil
}

func envOrString(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func envOrInt(key string, fallback int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}

func envOrDuration(key string, fallback time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	if d, err := time.ParseDuration(val); err == nil && d > 0 {
		return d
	}
	if sec, err := strconv.Atoi(val); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return fallback
}
