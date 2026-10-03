package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"nexus-agent-go/internal/model"
)

// ConfigStore 负责节点配置数据库读写。
type ConfigStore struct {
	db *gorm.DB
}

var (
	ErrInvalidNodeConfig     = errors.New("invalid node config")
	ErrInvalidNodeURL        = errors.New("invalid node url")
	ErrDuplicateNodeURL      = errors.New("duplicate node url")
	ErrInvalidCollectorType  = errors.New("invalid collector type")
	ErrInvalidSSHHost        = errors.New("invalid ssh host")
	ErrInvalidSSHPort        = errors.New("invalid ssh port")
	ErrInvalidSSHUser        = errors.New("invalid ssh user")
	ErrInvalidSSHAuthType    = errors.New("invalid ssh auth type")
	ErrDuplicateNodeEndpoint = errors.New("duplicate node endpoint")
)

// maxSSHHostLen 约束主机名/IPv6 长度，避免异常输入。
const maxSSHHostLen = 255

// sshUserPattern 与 /api/config/test-ssh 保持一致的用户名规则。
var sshUserPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// NewConfigStore 创建配置存储实例。
func NewConfigStore(db *gorm.DB) *ConfigStore {
	return &ConfigStore{db: db}
}

func normalizeAgentURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", ErrInvalidNodeURL
	}

	parsed, err := neturl.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: parse failed", ErrInvalidNodeURL)
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("%w: absolute url required", ErrInvalidNodeURL)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "" {
		return "", fmt.Errorf("%w: host is required", ErrInvalidNodeURL)
	}
	if port := strings.TrimSpace(parsed.Port()); port != "" {
		parsed.Host = net.JoinHostPort(hostname, port)
	} else {
		parsed.Host = hostname
	}

	// Root path trailing slash has no semantic value for our agent endpoint.
	if parsed.Path == "/" {
		parsed.Path = ""
		parsed.RawPath = ""
	}

	return parsed.String(), nil
}

// normalizeCollectorType 归一化采集方式，空值按旧版 agent 处理。
func normalizeCollectorType(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", model.CollectorTypeAgent:
		return model.CollectorTypeAgent, nil
	case model.CollectorTypeSSH:
		return model.CollectorTypeSSH, nil
	default:
		return "", fmt.Errorf("%w: unsupported collector type %q", ErrInvalidCollectorType, raw)
	}
}

// normalizeSSHUser 校验并返回 SSH 登录用户。
func normalizeSSHUser(raw string) (string, error) {
	user := strings.TrimSpace(raw)
	if user == "" || len(user) > 64 || !sshUserPattern.MatchString(user) {
		return "", ErrInvalidSSHUser
	}
	return user, nil
}

// normalizeNodeConfig 按采集方式校验并归一化节点配置。
func normalizeNodeConfig(config model.AgentConfig) (model.AgentConfig, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return model.AgentConfig{}, ErrInvalidNodeConfig
	}

	collectorType, err := normalizeCollectorType(config.CollectorType)
	if err != nil {
		return model.AgentConfig{}, err
	}

	normalized := model.AgentConfig{
		Name:          name,
		CollectorType: collectorType,
	}

	if collectorType == model.CollectorTypeSSH {
		host := strings.ToLower(strings.TrimSpace(config.SSHHost))
		if host == "" || len(host) > maxSSHHostLen {
			return model.AgentConfig{}, ErrInvalidSSHHost
		}
		if config.SSHPort < 1 || config.SSHPort > 65535 {
			return model.AgentConfig{}, ErrInvalidSSHPort
		}
		user, err := normalizeSSHUser(config.SSHUser)
		if err != nil {
			return model.AgentConfig{}, err
		}

		authType := strings.TrimSpace(config.SSHAuthType)
		if authType == "" {
			authType = model.SSHAuthTypeKey
		}
		if authType != model.SSHAuthTypeKey {
			return model.AgentConfig{}, fmt.Errorf("%w: only %q is supported", ErrInvalidSSHAuthType, model.SSHAuthTypeKey)
		}

		normalized.SSHHost = host
		normalized.SSHPort = config.SSHPort
		normalized.SSHUser = user
		normalized.SSHAuthType = authType
		return normalized, nil
	}

	normalizedURL, err := normalizeAgentURL(config.URL)
	if err != nil {
		return model.AgentConfig{}, err
	}
	normalized.URL = normalizedURL
	return normalized, nil
}

// sshEndpointKey 生成 SSH 节点的端点唯一标识 host:port:user。
func sshEndpointKey(config model.AgentConfig) string {
	return strings.ToLower(config.SSHHost) + ":" + strconv.Itoa(config.SSHPort) + ":" + config.SSHUser
}

func isDuplicateNodeDBError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint")
}

func (s *ConfigStore) hasURLConflict(tx *gorm.DB, normalizedURL string) (bool, error) {
	var nodes []AgentNode
	if err := tx.Select("url").Find(&nodes).Error; err != nil {
		return false, fmt.Errorf("load existing node urls failed: %w", err)
	}

	for _, node := range nodes {
		existingURL, err := normalizeAgentURL(node.URL)
		if err != nil {
			continue
		}
		if existingURL == normalizedURL {
			return true, nil
		}
	}
	return false, nil
}

func (s *ConfigStore) hasSSHEndpointConflict(tx *gorm.DB, endpointKey string) (bool, error) {
	var nodes []AgentNode
	if err := tx.Where("ssh_host <> ''").Find(&nodes).Error; err != nil {
		return false, fmt.Errorf("load existing ssh nodes failed: %w", err)
	}

	for _, node := range nodes {
		if sshEndpointKey(agentNodeToConfig(node)) == endpointKey {
			return true, nil
		}
	}
	return false, nil
}

// agentNodeToConfig 将数据库行转换为业务配置，collector_type 空值按 agent 处理。
func agentNodeToConfig(node AgentNode) model.AgentConfig {
	collectorType := node.CollectorType
	if strings.TrimSpace(collectorType) == "" {
		collectorType = model.CollectorTypeAgent
	}
	return model.AgentConfig{
		ID:            int64(node.ID),
		Name:          node.Name,
		CollectorType: collectorType,
		URL:           node.URL,
		SSHHost:       node.SSHHost,
		SSHPort:       node.SSHPort,
		SSHUser:       node.SSHUser,
		SSHAuthType:   node.SSHAuthType,
	}
}

func agentNodeFromConfig(config model.AgentConfig, createdBy uint) AgentNode {
	return AgentNode{
		Name:          config.Name,
		CollectorType: config.CollectorType,
		URL:           config.URL,
		SSHHost:       config.SSHHost,
		SSHPort:       config.SSHPort,
		SSHUser:       config.SSHUser,
		SSHAuthType:   config.SSHAuthType,
		CreatedBy:     createdBy,
	}
}

// Load 读取节点配置列表。
func (s *ConfigStore) Load() ([]model.AgentConfig, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("config store db is nil")
	}

	var nodes []AgentNode
	if err := s.db.Order("id ASC").Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("load nodes failed: %w", err)
	}

	configs := make([]model.AgentConfig, 0, len(nodes))
	for _, node := range nodes {
		configs = append(configs, agentNodeToConfig(node))
	}
	return configs, nil
}

// Add 新增单个节点配置。
func (s *ConfigStore) Add(config model.AgentConfig, createdBy uint) (model.AgentConfig, error) {
	if s == nil || s.db == nil {
		return model.AgentConfig{}, errors.New("config store db is nil")
	}

	normalized, err := normalizeNodeConfig(config)
	if err != nil {
		return model.AgentConfig{}, err
	}

	if normalized.CollectorType == model.CollectorTypeSSH {
		conflict, err := s.hasSSHEndpointConflict(s.db, sshEndpointKey(normalized))
		if err != nil {
			return model.AgentConfig{}, err
		}
		if conflict {
			return model.AgentConfig{}, ErrDuplicateNodeEndpoint
		}
	} else {
		conflict, err := s.hasURLConflict(s.db, normalized.URL)
		if err != nil {
			return model.AgentConfig{}, err
		}
		if conflict {
			return model.AgentConfig{}, ErrDuplicateNodeURL
		}
	}

	node := agentNodeFromConfig(normalized, createdBy)
	if err := s.db.Create(&node).Error; err != nil {
		if isDuplicateNodeDBError(err) {
			if normalized.CollectorType == model.CollectorTypeSSH {
				return model.AgentConfig{}, ErrDuplicateNodeEndpoint
			}
			return model.AgentConfig{}, ErrDuplicateNodeURL
		}
		return model.AgentConfig{}, fmt.Errorf("create node failed: %w", err)
	}

	return agentNodeToConfig(node), nil
}

// Save 兼容旧版一次性覆盖写入。
func (s *ConfigStore) Save(configs []model.AgentConfig, createdBy uint) error {
	if s == nil || s.db == nil {
		return errors.New("config store db is nil")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&AgentNode{}).Error; err != nil {
			return fmt.Errorf("clear nodes failed: %w", err)
		}

		seenURLs := make(map[string]struct{}, len(configs))
		seenSSHEndpoints := make(map[string]struct{}, len(configs))
		for _, item := range configs {
			normalized, err := normalizeNodeConfig(item)
			if err != nil {
				return err
			}
			if normalized.CollectorType == model.CollectorTypeSSH {
				key := sshEndpointKey(normalized)
				if _, exists := seenSSHEndpoints[key]; exists {
					return ErrDuplicateNodeEndpoint
				}
				seenSSHEndpoints[key] = struct{}{}
			} else {
				if _, exists := seenURLs[normalized.URL]; exists {
					return ErrDuplicateNodeURL
				}
				seenURLs[normalized.URL] = struct{}{}
			}

			node := agentNodeFromConfig(normalized, createdBy)
			if item.ID > 0 {
				node.ID = uint(item.ID)
			}
			if err := tx.Create(&node).Error; err != nil {
				if isDuplicateNodeDBError(err) {
					if normalized.CollectorType == model.CollectorTypeSSH {
						return ErrDuplicateNodeEndpoint
					}
					return ErrDuplicateNodeURL
				}
				return fmt.Errorf("save node failed: %w", err)
			}
		}
		return nil
	})
}

// Delete 删除指定节点配置。
func (s *ConfigStore) Delete(id uint) error {
	if s == nil || s.db == nil {
		return errors.New("config store db is nil")
	}
	res := s.db.Delete(&AgentNode{}, id)
	if res.Error != nil {
		return fmt.Errorf("delete node failed: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BootstrapFromJSONIfEmpty 兼容导入旧 config.json，数据库非空时跳过。
func (s *ConfigStore) BootstrapFromJSONIfEmpty(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if s == nil || s.db == nil {
		return errors.New("config store db is nil")
	}

	var count int64
	if err := s.db.Model(&AgentNode{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count nodes failed: %w", err)
	}
	if count > 0 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read bootstrap config failed: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var configs []model.AgentConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("parse bootstrap config failed: %w", err)
	}
	if len(configs) == 0 {
		return nil
	}
	return s.Save(configs, 0)
}
