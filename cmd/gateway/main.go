package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/gateway"
)

// main 启动 Gateway：提供前端页面、配置 API 和代理 API。
func main() {
	runnerCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	port := envOr("PORT", "3000")
	webDir := resolveWebDir(envOr("WEB_DIR", "web"))
	legacyConfigFile := envOr("CONFIG_FILE", filepath.Join(webDir, "config.json"))
	jwtSecret := envOr("JWT_SECRET", "nexus-agent-jwt-secret")
	runtimeCfg := gateway.LoadGatewayRuntimeConfigFromEnv()

	db, err := gateway.InitMySQLFromEnv()
	if err != nil {
		log.Fatalf("init mysql failed: %v", err)
	}

	store := gateway.NewConfigStore(db)
	if err := store.BootstrapFromJSONIfEmpty(legacyConfigFile); err != nil {
		log.Printf("bootstrap from legacy config skipped: %v", err)
	}

	redisClient, err := gateway.NewRedisClient(runnerCtx, runtimeCfg.Redis)
	if err != nil {
		log.Fatalf("init redis failed: %v", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("redis close error: %v", err)
		}
	}()

	nodeStateStore := gateway.NewNodeStateStore(redisClient, runtimeCfg.Redis.KeyPrefix, runtimeCfg.NodeStateTTL)
	nodeStateService := gateway.NewNodeStateService(store, nodeStateStore, gateway.NodeStateServiceOptions{
		PollTimeout: runtimeCfg.PollTimeout,
	})
	nodePoller := gateway.NewNodePoller(nodeStateService, runtimeCfg.PollInterval)
	go nodePoller.Start(runnerCtx)

	version := gateway.VersionInfo{
		Version:     time.Now().Unix(),
		VersionName: envOr("VERSION_NAME", "V2.0"),
		Changelog: strings.TrimSpace(envOr("CHANGELOG", `1.删除了Updated时间时间，因为它没有实际意义
2.添加了浪潮服务器卡片的蓝色效果`)),
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	aiConfig := ai.LoadConfigFromEnv()
	aiAdapter := gateway.NewAIDataAdapter(nodeStateService, nodeStateStore)
	aiToolbox := ai.NewToolbox(ai.ToolboxOptions{
		DataProvider:    aiAdapter,
		HistoryProvider: aiAdapter,
	})
	aiService := ai.NewService(ai.ServiceOptions{
		Config:  aiConfig,
		Toolbox: aiToolbox,
	})

	handler := gateway.NewHandler(store, gateway.NewAuthService(db, jwtSecret), version, nodeStateService)
	handler.SetAIQueryService(aiService)
	handler.RegisterAPIRoutes(r)
	handler.RegisterStaticRoutes(r, webDir)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Nexus Agent Go Gateway running at http://0.0.0.0:%s", port)
		log.Printf("Web directory: %s", webDir)
		log.Printf("Legacy config bootstrap file: %s", legacyConfigFile)
		log.Printf("Redis addr: %s, key prefix: %s", runtimeCfg.Redis.Addr, runtimeCfg.Redis.KeyPrefix)
		log.Printf("Poll interval: %s, poll timeout: %s, node state ttl: %s", runtimeCfg.PollInterval, runtimeCfg.PollTimeout, runtimeCfg.NodeStateTTL)
		log.Printf("AI enabled: %v, mode: %s, provider: %s, model configured: %v",
			aiConfig.Enabled,
			aiConfig.Mode,
			aiConfig.Provider,
			aiConfig.AgentReady(),
		)
		log.Printf("AI model: %s, base url: %s", aiConfig.Model, aiConfig.BaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway server failed: %v", err)
		}
	}()

	<-runnerCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("gateway shutdown error: %v", err)
	}
}

// resolveWebDir 校验前端目录是否存在。
func resolveWebDir(defaultDir string) string {
	if info, err := os.Stat(defaultDir); err == nil && info.IsDir() {
		return defaultDir
	}
	return defaultDir
}

// envOr 读取环境变量，不存在时返回默认值。
func envOr(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}
