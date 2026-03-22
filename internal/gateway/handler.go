package gateway

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/model"
)

// Handler 聚合网关 API 依赖与处理逻辑。
type Handler struct {
	store       *ConfigStore
	versionInfo VersionInfo
	proxyClient *http.Client
}

// NewHandler 创建网关请求处理器。
func NewHandler(store *ConfigStore, versionInfo VersionInfo) *Handler {
	return &Handler{
		store:       store,
		versionInfo: versionInfo,
		proxyClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// RegisterAPIRoutes 注册网关 API 路由。
func (h *Handler) RegisterAPIRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/version", h.getVersion)
	api.GET("/config", h.getConfig)
	api.POST("/config", h.saveConfig)
	api.GET("/proxy", h.proxyRequest)
}

// RegisterStaticRoutes 注册静态资源与前端入口路由。
func (h *Handler) RegisterStaticRoutes(r *gin.Engine, webDir string) {
	indexPath := filepath.Join(webDir, "index.html")
	publicDir := filepath.Join(webDir, "public")

	r.Static("/public", publicDir)
	r.GET("/", func(c *gin.Context) {
		c.File(indexPath)
	})

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		targetPath := safeJoin(webDir, c.Request.URL.Path)
		if targetPath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
			c.File(targetPath)
			return
		}

		c.File(indexPath)
	})
}

// getVersion 返回版本信息供前端轮询检测。
func (h *Handler) getVersion(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.JSON(http.StatusOK, h.versionInfo)
}

// getConfig 返回当前节点配置。
func (h *Handler) getConfig(c *gin.Context) {
	configs, err := h.store.Load()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, configs)
}

// saveConfig 保存节点配置。
func (h *Handler) saveConfig(c *gin.Context) {
	var configs []model.AgentConfig
	if err := c.ShouldBindJSON(&configs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	if err := h.store.Save(configs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// proxyRequest 代理转发目标 URL 请求，解决跨域与网络可达性问题。
func (h *Handler) proxyRequest(c *gin.Context) {
	rawURL := c.Query("url")
	if rawURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'url' parameter"})
		return
	}

	targetURL, err := url.Parse(rawURL)
	if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_url"})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL.String(), nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.proxyClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Status(resp.StatusCode)

	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		c.Error(fmt.Errorf("proxy copy failed: %w", err))
	}
}

// safeJoin 限制静态文件路径在 web 根目录内，避免目录穿越。
func safeJoin(baseDir, reqPath string) string {
	cleanBase := filepath.Clean(baseDir)
	cleanReq := filepath.Clean("/" + reqPath)
	fullPath := filepath.Join(cleanBase, cleanReq)

	if fullPath == cleanBase {
		return fullPath
	}

	prefix := cleanBase + string(os.PathSeparator)
	if !strings.HasPrefix(fullPath, prefix) {
		return ""
	}
	return fullPath
}
