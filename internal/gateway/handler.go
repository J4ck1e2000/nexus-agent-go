package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nexus-agent-go/internal/model"
)

const authUserContextKey = "auth_user"

// Handler 聚合网关 API 依赖与处理逻辑。
type Handler struct {
	store       *ConfigStore
	auth        *AuthService
	versionInfo VersionInfo
	proxyClient *http.Client
}

// NewHandler 创建网关请求处理器。
func NewHandler(store *ConfigStore, auth *AuthService, versionInfo VersionInfo) *Handler {
	return &Handler{
		store:       store,
		auth:        auth,
		versionInfo: versionInfo,
		proxyClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// RegisterAPIRoutes 注册网关 API 路由。
func (h *Handler) RegisterAPIRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/version", h.getVersion)
	api.POST("/register", h.register)
	api.POST("/login", h.login)

	authorized := api.Group("")
	authorized.Use(h.authMiddleware())
	authorized.GET("/me", h.getMe)
	authorized.GET("/config", h.getConfig)
	authorized.GET("/proxy", h.proxyRequest)

	admin := authorized.Group("")
	admin.Use(RequireAdmin())
	admin.POST("/config", h.saveConfig)
	admin.DELETE("/config/:id", h.deleteConfig)
	admin.POST("/admin/users", h.createAdminUser)
	admin.GET("/admin/users", h.listAdminUsers)
	admin.DELETE("/admin/users/:id", h.deleteAdminUser)
	admin.PATCH("/admin/users/:id/role", h.updateAdminUserRole)
	admin.PATCH("/admin/users/:id/password", h.resetAdminUserPassword)
}

// RegisterStaticRoutes 注册静态资源与前端入口路由。
func (h *Handler) RegisterStaticRoutes(r *gin.Engine, webDir string) {
	dashboardPath := filepath.Join(webDir, "dashboard", "index.html")
	loginPath := filepath.Join(webDir, "login", "index.html")
	publicDir := filepath.Join(webDir, "public")

	r.Static("/public", publicDir)
	r.GET("/", func(c *gin.Context) {
		if info, err := os.Stat(loginPath); err == nil && !info.IsDir() {
			c.File(loginPath)
			return
		}
		c.File(dashboardPath)
	})
	r.GET("/dashboard", func(c *gin.Context) {
		c.File(dashboardPath)
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

		c.File(dashboardPath)
	})
}

// getVersion 返回版本信息供前端轮询检测。
func (h *Handler) getVersion(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.JSON(http.StatusOK, h.versionInfo)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// register 创建普通用户账号。
func (h *Handler) register(c *gin.Context) {
	if h.auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	user, err := h.auth.Register(req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUsername):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_username"})
		case errors.Is(err, ErrPasswordTooShort):
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_too_short"})
		case errors.Is(err, ErrInvalidRegisterPayload):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		case errors.Is(err, ErrUserAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "user_already_exists"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user": user,
	})
}

// login 校验用户名密码并下发 token。
func (h *Handler) login(c *gin.Context) {
	if h.auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	token, user, err := h.auth.Authenticate(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user,
	})
}

// getMe 返回当前登录用户信息。
func (h *Handler) getMe(c *gin.Context) {
	currentUser, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, currentUser)
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

// saveConfig 新增节点配置（兼容旧版数组写入）。
func (h *Handler) saveConfig(c *gin.Context) {
	currentUser, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	rawBody = bytes.TrimSpace(rawBody)
	if len(rawBody) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	if rawBody[0] == '[' {
		var configs []model.AgentConfig
		if err := json.Unmarshal(rawBody, &configs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
			return
		}
		if err := h.store.Save(configs, currentUser.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return
	}

	var payload model.AgentConfig
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	created, err := h.store.Add(payload, currentUser.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusCreated, created)
}

// deleteConfig 删除节点配置。
func (h *Handler) deleteConfig(c *gin.Context) {
	idVal, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	if err := h.store.Delete(uint(idVal)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.auth == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		user, err := h.auth.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Set(authUserContextKey, user)
		c.Next()
	}
}

// RequireAdmin 仅允许管理员访问。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := currentAuthUser(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if user.Role != RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func currentAuthUser(c *gin.Context) (*AuthUser, bool) {
	value, ok := c.Get(authUserContextKey)
	if !ok {
		return nil, false
	}
	user, ok := value.(*AuthUser)
	return user, ok && user != nil
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
