package gateway

import (
	"bytes"
	"context"
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

	"nexus-agent-go/internal/ai"
	"nexus-agent-go/internal/collector/sshcollector"
	"nexus-agent-go/internal/model"
	"nexus-agent-go/internal/runtime"
)

const authUserContextKey = "auth_user"

// SSHTestTimeout 限制 SSH 连接测试的整体时长（覆盖连接 + 命令超时）。
const SSHTestTimeout = 15 * time.Second

const maxSSHEnrollmentRequestBytes = 64 * 1024

// SSHTester 抽象 SSH 连接测试能力，由 sshcollector.Collector 实现。
type SSHTester interface {
	TestSSH(ctx context.Context, host string, port int, user string) (sshcollector.TestSSHResult, error)
}

type SSHEnroller interface {
	EnrollSSH(ctx context.Context, node model.AgentConfig, password []byte, hints []sshcollector.TrustedHostKeyHint) (sshcollector.EnrollmentResult, error)
}

// Handler 聚合网关 API 依赖与处理逻辑。
type Handler struct {
	store            *ConfigStore
	auth             *AuthService
	versionInfo      VersionInfo
	proxyClient      *http.Client
	nodeStateService *NodeStateService
	sshTester        SSHTester
	sshEnroller      SSHEnroller
	aiQueryService   AIQueryService
	aiChatStore      *AIChatStore
	toolDispatcher   *ai.ToolDispatcher
	runManager       *runtime.Manager
	toolGatewayToken string
	runTokenSecret   string
}

// NewHandler 创建网关请求处理器。
func NewHandler(store *ConfigStore, auth *AuthService, versionInfo VersionInfo, nodeStateServices ...*NodeStateService) *Handler {
	var nodeStateService *NodeStateService
	if len(nodeStateServices) > 0 {
		nodeStateService = nodeStateServices[0]
	}

	return &Handler{
		store:            store,
		auth:             auth,
		versionInfo:      versionInfo,
		proxyClient:      &http.Client{Timeout: 3 * time.Second},
		nodeStateService: nodeStateService,
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
	authorized.GET("/nodes/overview", h.getNodesOverview)
	authorized.GET("/proxy", h.proxyRequest)
	h.registerAIRoutes(authorized)
	h.registerConversationRoutes(authorized)

	admin := authorized.Group("")
	admin.Use(RequireAdmin())
	admin.GET("/config", h.getConfig)
	admin.POST("/config/enroll-ssh", h.enrollSSH)
	admin.POST("/config", h.saveConfig)
	admin.POST("/config/test-ssh", h.testSSH)
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

// getNodesOverview 返回所有配置节点的聚合状态（优先 Redis 热状态）。
func (h *Handler) getNodesOverview(c *gin.Context) {
	if h.nodeStateService != nil {
		overview, err := h.nodeStateService.GetNodesOverview(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		c.JSON(http.StatusOK, overview)
		return
	}

	// 兜底路径：未注入状态服务时，仍返回配置节点的 pending 结构。
	configs, err := h.store.Load()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	overview := make([]NodeOverview, 0, len(configs))
	for _, node := range configs {
		overview = append(overview, nodeStateToOverview(pendingNodeState(node)))
	}
	c.JSON(http.StatusOK, overview)
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
		for _, config := range configs {
			if strings.TrimSpace(config.CollectorType) == model.CollectorTypeSSH {
				c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_enrollment_required"})
				return
			}
		}
		if err := h.store.Save(configs, currentUser.ID); err != nil {
			h.writeNodeConfigError(c, err)
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
	if strings.TrimSpace(payload.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if strings.TrimSpace(payload.CollectorType) == model.CollectorTypeSSH {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_enrollment_required"})
		return
	}

	created, err := h.store.Add(payload, currentUser.ID)
	if err != nil {
		h.writeNodeConfigError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// SetSSHTester 注入 SSH 连接测试实现（未注入时 test-ssh 返回未配置）。
type oneTimeSSHPassword []byte

func (p *oneTimeSSHPassword) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*p = append((*p)[:0], value...)
	return nil
}

func clearOneTimeSSHPassword(password []byte) {
	password = password[:cap(password)]
	for i := range password {
		password[i] = 0
	}
}

type enrollSSHRequest struct {
	Name            string                            `json:"name"`
	SSHHost         string                            `json:"ssh_host"`
	SSHPort         int                               `json:"ssh_port"`
	SSHUser         string                            `json:"ssh_user"`
	Password        oneTimeSSHPassword                `json:"ssh_password"`
	TrustedHostKeys []sshcollector.TrustedHostKeyHint `json:"trusted_host_keys,omitempty"`
}

func sshBootstrapTransportAllowed(request *http.Request) bool {
	if request == nil {
		return false
	}
	if request.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SSH_BOOTSTRAP_ALLOW_INSECURE_HTTP")), "true") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SSH_BOOTSTRAP_TRUST_PROXY_TLS")), "true") {
		forwardedProto := strings.TrimSpace(strings.SplitN(request.Header.Get("X-Forwarded-Proto"), ",", 2)[0])
		return strings.EqualFold(forwardedProto, "https")
	}
	return false
}

func (h *Handler) enrollSSH(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHEnrollmentRequestBytes)
	var req enrollSSHRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		clearOneTimeSSHPassword(req.Password)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	defer clearOneTimeSSHPassword(req.Password)
	if len(req.Password) > 4096 || len(req.TrustedHostKeys) > 16 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	for _, hint := range req.TrustedHostKeys {
		if len(hint.Marker) > 32 || len(hint.PublicKey) > 4096 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
			return
		}
		switch hint.Marker {
		case "", "@revoked", "@cert-authority":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
			return
		}
	}
	currentUser, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if len(req.Password) > 0 && !sshBootstrapTransportAllowed(c.Request) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insecure_transport"})
		return
	}
	node := model.AgentConfig{
		Name:          strings.TrimSpace(req.Name),
		CollectorType: model.CollectorTypeSSH,
		SSHHost:       strings.TrimSpace(req.SSHHost),
		SSHPort:       req.SSHPort,
		SSHUser:       strings.TrimSpace(req.SSHUser),
		SSHAuthType:   model.SSHAuthTypeKey,
	}
	existing, found, err := h.store.FindSSHEndpoint(node)
	if err != nil {
		h.writeNodeConfigError(c, err)
		return
	}
	if found {
		if existing.SSHUser != node.SSHUser {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_node_endpoint"})
			return
		}
		node.SSHHostKey = existing.SSHHostKey
	} else if err := h.store.CheckSSHEndpointAvailable(node); err != nil {
		h.writeNodeConfigError(c, err)
		return
	}
	if h.sshEnroller == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": sshcollector.ErrSSHNotConfigured.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	password := append([]byte(nil), req.Password...)
	defer clearOneTimeSSHPassword(password)
	enrollment, err := h.sshEnroller.EnrollSSH(ctx, node, password, req.TrustedHostKeys)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": sshcollector.ErrorCode(err)})
		return
	}
	node.SSHHostKey = enrollment.HostKey
	node.SSHHostKeyFingerprint = enrollment.HostKeyFingerprint
	var created model.AgentConfig
	if found {
		created, err = h.store.UpdateSSHEnrollment(uint(existing.ID), node)
	} else {
		created, err = h.store.Add(node, currentUser.ID)
	}
	if err != nil {
		h.writeNodeConfigError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) SetSSHEnroller(enroller SSHEnroller) {
	h.sshEnroller = enroller
}

func (h *Handler) SetSSHTester(tester SSHTester) {
	h.sshTester = tester
}

type testSSHRequest struct {
	SSHHost string `json:"ssh_host"`
	SSHPort int    `json:"ssh_port"`
	SSHUser string `json:"ssh_user"`
}

// testSSH 用 Gateway 侧配置的私钥对目标主机做一次真实采集测试。
// 仅管理员可用；失败只返回稳定错误码，不暴露 SSH 栈信息或私钥路径。
func (h *Handler) testSSH(c *gin.Context) {
	var req testSSHRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	host := strings.ToLower(strings.TrimSpace(req.SSHHost))
	if host == "" || len(host) > maxSSHHostLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if req.SSHPort < 1 || req.SSHPort > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if _, err := normalizeSSHUser(req.SSHUser); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	if h.sshTester == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": sshcollector.ErrSSHNotConfigured.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), SSHTestTimeout)
	defer cancel()

	result, err := h.sshTester.TestSSH(ctx, host, req.SSHPort, strings.TrimSpace(req.SSHUser))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": sshcollector.ErrorCode(err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"hostname":  result.Hostname,
		"gpu_count": result.GPUCount,
		"gpu_names": result.GPUNames,
	})
}

// writeNodeConfigError 将节点配置错误映射为稳定的 API 错误码。
func (h *Handler) writeNodeConfigError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDuplicateNodeURL):
		c.JSON(http.StatusConflict, gin.H{"error": "duplicate_node_url"})
	case errors.Is(err, ErrDuplicateNodeEndpoint):
		c.JSON(http.StatusConflict, gin.H{"error": "duplicate_node_endpoint"})
	case errors.Is(err, ErrInvalidNodeConfig),
		errors.Is(err, ErrInvalidNodeURL),
		errors.Is(err, ErrInvalidCollectorType),
		errors.Is(err, ErrInvalidSSHHost),
		errors.Is(err, ErrInvalidSSHPort),
		errors.Is(err, ErrInvalidSSHUser),
		errors.Is(err, ErrInvalidSSHAuthType):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
	}
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
