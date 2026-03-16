package gateway

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"nexus-agent-go/internal/model"
)

// ConfigStore 负责节点配置文件的线程安全读写。
type ConfigStore struct {
	path string
	mu   sync.RWMutex
}

// NewConfigStore 创建配置存储实例。
func NewConfigStore(path string) *ConfigStore {
	return &ConfigStore{path: path}
}

// Load 读取配置文件；文件不存在时返回空数组。
func (s *ConfigStore) Load() ([]model.AgentConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.AgentConfig{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []model.AgentConfig{}, nil
	}

	configs := make([]model.AgentConfig, 0)
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}

// Save 原子化保存配置，先写临时文件再替换。
func (s *ConfigStore) Save(configs []model.AgentConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
