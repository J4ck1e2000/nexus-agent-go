package main

import (
	"context"
	"errors"
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
	"nexus-agent-go/internal/collector"
	"nexus-agent-go/internal/collector/agenthttp"
	"nexus-agent-go/internal/collector/sshcollector"
	"nexus-agent-go/internal/gateway"
	"nexus-agent-go/internal/runtime"
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

	// SSH agentless 采集器：默认使用自动生成并持久保存的 Gateway 密钥。
	sshCollector := buildSSHCollector()

	nodeStateService := gateway.NewNodeStateService(store, nodeStateStore, gateway.NodeStateServiceOptions{
		PollTimeout: runtimeCfg.PollTimeout,
		Collectors: collector.NewCollectorRouter(
			agenthttp.New(nil, runtimeCfg.PollTimeout),
			sshCollector,
		),
		MaxConcurrency: runtimeCfg.MaxConcurrency,
	})
	// Gateway 关停时释放全部 SSH 连接。
	defer func() {
		if sshCollector != nil {
			sshCollector.Close()
		}
	}()
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
	if strings.TrimSpace(aiConfig.KnowledgeConfigErr) != "" {
		log.Printf("Knowledge retrieval config warning: %s", aiConfig.KnowledgeConfigErr)
	}
	aiAdapter := gateway.NewAIDataAdapter(nodeStateService, nodeStateStore)
	var knowledgeSearcher ai.KnowledgeSearcher
	loadedKnowledgeDocs := 0
	loadedKnowledgeChunks := 0
	retrievalStats := ai.NewRetrievalStatsCollector(false)
	if aiConfig.RAGEnabled {
		buildResult, err := ai.BuildKnowledgeSearcher(
			runnerCtx,
			aiConfig.RAGKnowledgeDir,
			ai.KnowledgeRetrieverOptions{
				DefaultTopK:       aiConfig.RAGTopK,
				MinScore:          aiConfig.RAGMinScore,
				MaxSnippetChars:   aiConfig.RAGMaxSnippet,
				RetrievalStrategy: "local-lexical",
			},
			aiConfig.KnowledgeRetrieval,
			log.Printf,
		)
		if err != nil {
			log.Printf("AI knowledge retrieval disabled: %v", err)
		} else {
			knowledgeSearcher = buildResult.Searcher
			loadedKnowledgeDocs = buildResult.Documents
			loadedKnowledgeChunks = buildResult.Chunks
			if knowledgeSearcher != nil {
				retrievalStats.UpdateKnowledgeState(true, loadedKnowledgeDocs, loadedKnowledgeChunks, time.Now())
			}
		}
	}
	aiToolbox := ai.NewToolbox(ai.ToolboxOptions{
		DataProvider:       aiAdapter,
		HistoryProvider:    aiAdapter,
		KnowledgeSearcher:  knowledgeSearcher,
		KnowledgeDocuments: loadedKnowledgeDocs,
		KnowledgeChunks:    loadedKnowledgeChunks,
		RetrievalStats:     retrievalStats,
		KnowledgeTopK:      aiConfig.KnowledgeRetrieval.TopK,
	})
	aiService := ai.NewSupportService(ai.ServiceOptions{
		Config:  aiConfig,
		Toolbox: aiToolbox,
	})

	handler := gateway.NewHandler(store, gateway.NewAuthService(db, jwtSecret), version, nodeStateService)
	aiChatStore := gateway.NewAIChatStore(db)
	handler.SetAIChatStore(aiChatStore)
	if sshCollector != nil {
		handler.SetSSHTester(sshCollector)
		handler.SetSSHEnroller(sshCollector)
	}

	runtimeConfig := runtime.LoadConfigFromEnv()
	if !runtimeConfig.Enabled {
		log.Fatal("PI_RUNTIME_URL is required; Pi Runtime is the only AI executor")
	}
	toolDispatcher := ai.NewToolDispatcher(aiToolbox)
	runManager := runtime.NewManager()
	defer runManager.Close()
	runtimeClient := runtime.NewClient(runtimeConfig.RuntimeURL, runtimeConfig.RuntimeToken)
	piExecutor := ai.NewPiRuntimeExecutor(ai.PiRuntimeExecutorOptions{
		Client:       runtimeClient,
		Manager:      runManager,
		Config:       runtimeConfig,
		Toolbox:      aiToolbox,
		AllowedTools: toolDispatcher.ToolNames(),
		ModelID:      aiConfig.Model,
	})
	handler.SetAIQueryService(ai.NewPiQueryService(aiService, piExecutor, aiChatStore))
	handler.SetToolGateway(gateway.ToolGatewayDeps{
		Dispatcher:     toolDispatcher,
		Manager:        runManager,
		InternalToken:  runtimeConfig.RuntimeToken,
		RunTokenSecret: runtimeConfig.RunTokenSecret,
	})
	handler.RegisterInternalToolRoutes(r)
	log.Printf("AI executor: pi (runtime %s, run timeout %s, max tool calls %d)",
		runtimeConfig.RuntimeURL, runtimeConfig.RunTimeout, runtimeConfig.MaxToolCalls)
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
		log.Printf("Poll interval: %s, poll timeout: %s, node state ttl: %s, max poll concurrency: %d", runtimeCfg.PollInterval, runtimeCfg.PollTimeout, runtimeCfg.NodeStateTTL, runtimeCfg.MaxConcurrency)
		log.Printf("AI enabled: %v, mode: %s, provider: %s, model configured: %v",
			aiConfig.Enabled,
			aiConfig.Mode,
			aiConfig.Provider,
			aiConfig.AgentReady(),
		)
		log.Printf("AI model: %s, base url: %s", aiConfig.Model, aiConfig.BaseURL)
		log.Printf("Knowledge retrieval config: %s", aiConfig.KnowledgeRetrieval.SafeSummary())
		knowledgeStats := aiService.RetrievalStats(context.Background())
		log.Printf("AI RAG requested: %v, knowledge dir: %s, loaded docs: %d, loaded chunks: %d, active: %v",
			aiConfig.RAGEnabled,
			aiConfig.RAGKnowledgeDir,
			knowledgeStats.LoadedDocuments,
			knowledgeStats.LoadedChunks,
			knowledgeStats.KnowledgeEnabled,
		)
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

// buildSSHCollector 从环境变量构建 SSH 采集器。
// SSH 未配置或初始化失败时只禁用 SSH 采集，Gateway 和其他采集模式继续运行。
func buildSSHCollector() *sshcollector.Collector {
	sshCollector, err := sshcollector.NewWithOptionsFromEnv(sshcollector.LoadOptionsFromEnv())
	if err != nil {
		if errors.Is(err, sshcollector.ErrSSHNotConfigured) {
			log.Printf("ssh collector unavailable: Gateway SSH identity could not be loaded")
			return nil
		}
		log.Printf("ssh collector disabled: init failed: %v", err)
		return nil
	}
	connectTO, commandTO, keepalive := sshCollector.OptionsSummary()
	log.Printf("ssh collector enabled: connect timeout %s, command timeout %s, keepalive %s",
		connectTO, commandTO, keepalive)
	return sshCollector
}

// envOr 读取环境变量，不存在时返回默认值。
func envOr(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}
