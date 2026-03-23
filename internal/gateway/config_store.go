package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"nexus-agent-go/internal/model"
)

// ConfigStore 负责节点配置数据库读写。
type ConfigStore struct {
	db *gorm.DB
}

// NewConfigStore 创建配置存储实例。
func NewConfigStore(db *gorm.DB) *ConfigStore {
	return &ConfigStore{db: db}
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

	name := strings.TrimSpace(config.Name)
	url := strings.TrimSpace(config.URL)
	if name == "" || url == "" {
		return model.AgentConfig{}, errors.New("name and url are required")
	}

	node := AgentNode{
		Name:      name,
		URL:       url,
		CreatedBy: createdBy,
	}
	if err := s.db.Create(&node).Error; err != nil {
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

		for _, item := range configs {
			name := strings.TrimSpace(item.Name)
			url := strings.TrimSpace(item.URL)
			if name == "" || url == "" {
				return errors.New("name and url are required")
			}

			node := AgentNode{
				Name:      name,
				URL:       url,
				CreatedBy: createdBy,
			}
			if item.ID > 0 {
				node.ID = uint(item.ID)
			}
			if err := tx.Create(&node).Error; err != nil {
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
