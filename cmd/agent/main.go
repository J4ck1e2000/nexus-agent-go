package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/agent"
	"nexus-agent-go/internal/model"
	toolsvc "nexus-agent-go/internal/tools"
)

type currentMetricsResponse struct {
	CollectedAtUnix int64 `json:"collected_at_unix"`
	model.SystemMetrics
}

// main 启动 Agent 服务：后台采集指标并暴露 HTTP API。
func main() {
	port := envOr("PORT", "8005")
	pollInterval := durationEnv("METRICS_INTERVAL", 2*time.Second)

	service := agent.NewService(pollInterval)
	metricsTools := toolsvc.NewMetricsTools(service)
	runnerCtx, stopCollector := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopCollector()
	go service.Start(runnerCtx)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsAll())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "Nexus Agent Go is running",
			"version": "0.2.0 (Go)",
		})
	})

	r.GET("/metrics", func(c *gin.Context) {
		metrics, err := metricsTools.GetCurrentMetrics(c.Request.Context())
		if err != nil {
			writeMetricsUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, metrics)
	})

	r.GET("/metrics/current", func(c *gin.Context) {
		metrics, collectedAtUnix, err := metricsTools.GetCurrentMetricsWithMeta(c.Request.Context())
		if err != nil {
			writeMetricsUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, currentMetricsResponse{
			CollectedAtUnix: collectedAtUnix,
			SystemMetrics:   metrics,
		})
	})

	r.GET("/metrics/summary", func(c *gin.Context) {
		summary, err := metricsTools.GetMetricsSummary(c.Request.Context())
		if err != nil {
			writeMetricsUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, summary)
	})

	r.GET("/metrics/processes", func(c *gin.Context) {
		processes, err := metricsTools.GetProcesses(c.Request.Context())
		if err != nil {
			writeMetricsUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, processes)
	})

	r.GET("/metrics/gpus", func(c *gin.Context) {
		gpus, err := metricsTools.GetGPUs(c.Request.Context())
		if err != nil {
			writeMetricsUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, gpus)
	})

	logMetricsRoutes(r)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Nexus Agent Go running at http://0.0.0.0:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("agent server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("agent shutdown error: %v", err)
	}
}

// corsAll 允许前端跨域访问 Agent 接口。
func corsAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-PIN")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// envOr 读取环境变量，不存在时返回默认值。
func envOr(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

// durationEnv 读取时长环境变量，支持 Go duration 或纯秒数。
func durationEnv(key string, fallback time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	if dur, err := time.ParseDuration(val); err == nil {
		return dur
	}
	if sec, err := strconv.Atoi(val); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return fallback
}

func writeMetricsUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "metrics_unavailable"})
}

func logMetricsRoutes(r *gin.Engine) {
	for _, route := range r.Routes() {
		if strings.HasPrefix(route.Path, "/metrics") {
			log.Printf("registered route: %s %s", route.Method, route.Path)
		}
	}
}
