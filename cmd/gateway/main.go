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

	"nexus-agent-go/internal/auth"
	"nexus-agent-go/internal/gateway"
)

// main 启动 Gateway：提供前端页面、配置 API 和代理 API。
func main() {
	port := envOr("PORT", "3000")
	webDir := resolveWebDir(envOr("WEB_DIR", "web"))
	configFile := envOr("CONFIG_FILE", filepath.Join(webDir, "config.json"))

	version := gateway.VersionInfo{
		Version:     time.Now().Unix(),
		VersionName: envOr("VERSION_NAME", "V2.0"),
		Changelog: strings.TrimSpace(envOr("CHANGELOG", `1.删除了Updated时间时间，因为它没有实际意义
2.添加了浪潮服务器卡片的蓝色效果`)),
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	gatewayToken := strings.TrimSpace(os.Getenv("NEXUS_GATEWAY_TOKEN"))
	handler := gateway.NewHandler(gateway.NewConfigStore(configFile), version)
	handler.RegisterAPIRoutes(r, auth.BearerTokenMiddleware(gatewayToken))
	handler.RegisterStaticRoutes(r, webDir)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Nexus Agent Go Gateway running at http://0.0.0.0:%s", port)
		log.Printf("Web directory: %s", webDir)
		log.Printf("Config file: %s", configFile)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

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
