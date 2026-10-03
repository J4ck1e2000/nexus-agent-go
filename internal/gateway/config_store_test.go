package gateway

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"nexus-agent-go/internal/model"
)

// newTestConfigStore 基于内存 SQLite 创建测试用配置存储。
func newTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewConfigStore(db)
}

func TestNormalizeAgentURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:  "trim and lowercase scheme host and remove root slash",
			input: "  HTTP://LOCALHOST:8005/  ",
			want:  "http://localhost:8005",
		},
		{
			name:  "preserve path and query semantics",
			input: "https://Example.COM:8443/api/v1/?q=abc",
			want:  "https://example.com:8443/api/v1/?q=abc",
		},
		{
			name:    "reject empty",
			input:   "   ",
			wantErr: ErrInvalidNodeURL,
		},
		{
			name:    "reject relative url",
			input:   "/metrics",
			wantErr: ErrInvalidNodeURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAgentURL(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error mismatch: got=%v want=%v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("normalizeAgentURL failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeAgentURL mismatch: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestNormalizeNodeConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   model.AgentConfig
		want    model.AgentConfig
		wantErr error
	}{
		{
			name:  "empty collector type defaults to agent",
			input: model.AgentConfig{Name: " node-a ", URL: "HTTP://LocalHost:8005/"},
			want: model.AgentConfig{
				Name:          "node-a",
				CollectorType: model.CollectorTypeAgent,
				URL:           "http://localhost:8005",
			},
		},
		{
			name:  "ssh node is normalized",
			input: model.AgentConfig{Name: "gpu-01", CollectorType: "ssh", SSHHost: " 10.0.0.15 ", SSHPort: 22, SSHUser: " renhaokun "},
			want: model.AgentConfig{
				Name:          "gpu-01",
				CollectorType: model.CollectorTypeSSH,
				SSHHost:       "10.0.0.15",
				SSHPort:       22,
				SSHUser:       "renhaokun",
				SSHAuthType:   model.SSHAuthTypeKey,
			},
		},
		{
			name:    "missing name rejected",
			input:   model.AgentConfig{URL: "http://127.0.0.1:8005"},
			wantErr: ErrInvalidNodeConfig,
		},
		{
			name:    "unknown collector type rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "snmp", URL: "http://127.0.0.1:8005"},
			wantErr: ErrInvalidCollectorType,
		},
		{
			name:    "agent node without url rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "agent"},
			wantErr: ErrInvalidNodeURL,
		},
		{
			name:    "ssh node without host rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "ssh", SSHPort: 22, SSHUser: "ops"},
			wantErr: ErrInvalidSSHHost,
		},
		{
			name:    "ssh node with zero port rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "ssh", SSHHost: "10.0.0.1", SSHUser: "ops"},
			wantErr: ErrInvalidSSHPort,
		},
		{
			name:    "ssh node with out-of-range port rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "ssh", SSHHost: "10.0.0.1", SSHPort: 70000, SSHUser: "ops"},
			wantErr: ErrInvalidSSHPort,
		},
		{
			name:    "ssh node with empty user rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "ssh", SSHHost: "10.0.0.1", SSHPort: 22},
			wantErr: ErrInvalidSSHUser,
		},
		{
			name:    "ssh node with invalid user chars rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "ssh", SSHHost: "10.0.0.1", SSHPort: 22, SSHUser: "ops admin"},
			wantErr: ErrInvalidSSHUser,
		},
		{
			name:    "ssh node with unsupported auth type rejected",
			input:   model.AgentConfig{Name: "x", CollectorType: "ssh", SSHHost: "10.0.0.1", SSHPort: 22, SSHUser: "ops", SSHAuthType: "password"},
			wantErr: ErrInvalidSSHAuthType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeNodeConfig(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error mismatch: got=%v want=%v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeNodeConfig failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeNodeConfig mismatch: got=%+v want=%+v", got, tt.want)
			}
		})
	}
}

func TestConfigStore_AddAndLoadSSHNode(t *testing.T) {
	store := newTestConfigStore(t)

	created, err := store.Add(model.AgentConfig{
		Name:          "A6000-01",
		CollectorType: model.CollectorTypeSSH,
		SSHHost:       "10.0.0.15",
		SSHPort:       22,
		SSHUser:       "renhaokun",
	}, 1)
	if err != nil {
		t.Fatalf("add ssh node failed: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("invalid created id: %d", created.ID)
	}
	if created.CollectorType != model.CollectorTypeSSH || created.SSHHost != "10.0.0.15" || created.SSHPort != 22 || created.SSHUser != "renhaokun" {
		t.Fatalf("created ssh node mismatch: %+v", created)
	}
	if created.URL != "" {
		t.Fatalf("ssh node should not carry url, got %q", created.URL)
	}

	configs, err := store.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(configs) != 1 || configs[0].CollectorType != model.CollectorTypeSSH || configs[0].SSHHost != "10.0.0.15" {
		t.Fatalf("loaded configs mismatch: %+v", configs)
	}
}

func TestConfigStore_DuplicateSSHEndpointRejected(t *testing.T) {
	store := newTestConfigStore(t)

	payload := model.AgentConfig{
		Name:          "A6000-01",
		CollectorType: model.CollectorTypeSSH,
		SSHHost:       "10.0.0.15",
		SSHPort:       22,
		SSHUser:       "renhaokun",
	}
	if _, err := store.Add(payload, 1); err != nil {
		t.Fatalf("first add failed: %v", err)
	}

	// 同一 host:port 即是同一台全局节点，名字和 Linux 用户都不能创建副本。
	duplicate := payload
	duplicate.Name = "A6000-02"
	duplicate.SSHUser = "root"
	if _, err := store.Add(duplicate, 1); !errors.Is(err, ErrDuplicateNodeEndpoint) {
		t.Fatalf("expected same-server duplicate error, got %v", err)
	}

	// 不同端口不冲突。
	otherPort := payload
	otherPort.Name = "A6000-04"
	otherPort.SSHPort = 2222
	if _, err := store.Add(otherPort, 1); err != nil {
		t.Fatalf("different port should not conflict: %v", err)
	}
}

func TestConfigStore_LoadDefaultsEmptyCollectorTypeToAgent(t *testing.T) {
	store := newTestConfigStore(t)
	// 直接写入旧行为的数据(collector_type 为空),模拟升级前的存量行。
	if err := store.db.Exec(
		"INSERT INTO agent_nodes (name, url, collector_type, ssh_host, ssh_port, ssh_user, ssh_auth_type, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"legacy", "http://127.0.0.1:8005", "", "", 0, "", "", 1,
	).Error; err != nil {
		t.Fatalf("seed legacy row failed: %v", err)
	}

	configs, err := store.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %+v", configs)
	}
	if configs[0].CollectorType != model.CollectorTypeAgent {
		t.Fatalf("legacy row should resolve to agent, got %q", configs[0].CollectorType)
	}
	if configs[0].URL != "http://127.0.0.1:8005" {
		t.Fatalf("legacy url mismatch: %+v", configs[0])
	}
}

func TestConfigStore_SaveMixedNodes(t *testing.T) {
	store := newTestConfigStore(t)

	err := store.Save([]model.AgentConfig{
		{Name: "legacy-01", URL: "http://127.0.0.1:8005"},
		{Name: "ssh-01", CollectorType: model.CollectorTypeSSH, SSHHost: "10.0.0.15", SSHPort: 22, SSHUser: "renhaokun"},
	}, 1)
	if err != nil {
		t.Fatalf("save mixed nodes failed: %v", err)
	}

	configs, err := store.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %+v", configs)
	}
	if configs[0].CollectorType != model.CollectorTypeAgent || configs[1].CollectorType != model.CollectorTypeSSH {
		t.Fatalf("collector types mismatch: %+v", configs)
	}

	// 同一批内同一 host:port 使用不同 Linux 账号也不能重复添加。
	dupErr := store.Save([]model.AgentConfig{
		{Name: "ssh-a", CollectorType: model.CollectorTypeSSH, SSHHost: "10.0.0.15", SSHPort: 22, SSHUser: "renhaokun"},
		{Name: "ssh-b", CollectorType: model.CollectorTypeSSH, SSHHost: "10.0.0.15", SSHPort: 22, SSHUser: "root"},
	}, 1)
	if !errors.Is(dupErr, ErrDuplicateNodeEndpoint) {
		t.Fatalf("expected duplicate endpoint error, got %v", dupErr)
	}
}
