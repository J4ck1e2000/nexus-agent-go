package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"strings"

	"gorm.io/gorm"

	"nexus-agent-go/internal/model"
)

// ConfigStore 负责节点配置数据库读写。
type ConfigStore struct {
	db *gorm.DB
}

var (
	ErrInvalidNodeConfig = errors.New("invalid node config")
	ErrInvalidNodeURL    = errors.New("invalid node url")
	ErrDuplicateNodeURL  = errors.New("duplicate node url")
)

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

func normalizeAgentConfig(config model.AgentConfig) (string, string, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return "", "", ErrInvalidNodeConfig
	}

	normalizedURL, err := normalizeAgentURL(config.URL)
	if err != nil {
		return "", "", err
	}
	return name, normalizedURL, nil
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
		configs = append(configs, model.AgentConfig{
			ID:   int64(node.ID),
			Name: node.Name,
			URL:  node.URL,
		})
	}
	return configs, nil
}

// Add 新增单个节点配置。
func (s *ConfigStore) Add(config model.AgentConfig, createdBy uint) (model.AgentConfig, error) {
	if s == nil || s.db == nil {
		return model.AgentConfig{}, errors.New("config store db is nil")
	}

	name, normalizedURL, err := normalizeAgentConfig(config)
	if err != nil {
		return model.AgentConfig{}, err
	}

	conflict, err := s.hasURLConflict(s.db, normalizedURL)
	if err != nil {
		return model.AgentConfig{}, err
	}
	if conflict {
		return model.AgentConfig{}, ErrDuplicateNodeURL
	}

	node := AgentNode{
		Name:      name,
		URL:       normalizedURL,
		CreatedBy: createdBy,
	}
	if err := s.db.Create(&node).Error; err != nil {
		if isDuplicateNodeDBError(err) {
			return model.AgentConfig{}, ErrDuplicateNodeURL
		}
		return model.AgentConfig{}, fmt.Errorf("create node failed: %w", err)
	}

	return model.AgentConfig{
		ID:   int64(node.ID),
		Name: node.Name,
		URL:  node.URL,
	}, nil
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
		for _, item := range configs {
			name, normalizedURL, err := normalizeAgentConfig(item)
			if err != nil {
				return err
			}
			if _, exists := seenURLs[normalizedURL]; exists {
				return ErrDuplicateNodeURL
			}
			seenURLs[normalizedURL] = struct{}{}

			node := AgentNode{
				Name:      name,
				URL:       normalizedURL,
				CreatedBy: createdBy,
			}
			if item.ID > 0 {
				node.ID = uint(item.ID)
			}
			if err := tx.Create(&node).Error; err != nil {
				if isDuplicateNodeDBError(err) {
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
