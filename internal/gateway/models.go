package gateway

import "time"

// UserRole 表示用户角色。
type UserRole string

const (
	// RoleAdmin 管理员，拥有节点增删权限。
	RoleAdmin UserRole = "admin"
	// RoleUser 普通用户，仅可查看。
	RoleUser UserRole = "user"
)

// User 为登录用户表。
type User struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"column:password_hash;type:varchar(255);not null" json:"-"`
	Role         UserRole  `gorm:"type:varchar(16);index;not null" json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定用户表名。
func (User) TableName() string {
	return "users"
}

// AgentNode 为监控节点配置表。
type AgentNode struct {
	ID   uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name string `gorm:"type:varchar(128);not null" json:"name"`
	// CollectorType 为采集方式（agent/ssh）。历史数据经迁移默认填 agent，
	// 业务层对空值同样按 agent 处理。
	CollectorType string `gorm:"type:varchar(16);not null;default:'agent'" json:"collector_type"`
	// URL 为 agent 模式的 Agent 地址；SSH 节点无 URL，存 NULL 以允许多条
	// 记录共用唯一索引 uk_agent_nodes_url。
	URL string `gorm:"type:varchar(255);uniqueIndex:uk_agent_nodes_url;default:null" json:"url,omitempty"`
	// SSHHost/SSHPort/SSHUser/SSHAuthType 为 ssh 模式连接信息，
	// SSH 私钥永不入库。
	SSHHost               string    `gorm:"type:varchar(255);not null;default:''" json:"ssh_host,omitempty"`
	SSHPort               int       `gorm:"not null;default:0" json:"ssh_port,omitempty"`
	SSHEndpointKey        *string   `gorm:"column:ssh_endpoint_key;type:varchar(300);uniqueIndex:uk_agent_nodes_ssh_endpoint" json:"-"`
	SSHUser               string    `gorm:"type:varchar(64);not null;default:''" json:"ssh_user,omitempty"`
	SSHAuthType           string    `gorm:"type:varchar(16);not null;default:''" json:"ssh_auth_type,omitempty"`
	SSHHostKey            string    `gorm:"column:ssh_host_key;type:text" json:"-"`
	SSHHostKeyFingerprint string    `gorm:"column:ssh_host_key_fingerprint;type:varchar(128)" json:"ssh_host_key_fingerprint,omitempty"`
	CreatedBy             uint      `gorm:"column:created_by;index;not null" json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// TableName 指定节点表名。
func (AgentNode) TableName() string {
	return "agent_nodes"
}

// AIConversation 为 AI 助手会话表，按用户隔离；Title 在首条提问落库时自动补全。
type AIConversation struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Title     string    `gorm:"type:varchar(128);not null;default:''" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定 AI 会话表名。
func (AIConversation) TableName() string {
	return "ai_conversations"
}

// AIMessageRoleUser/AIMessageRoleAssistant 为 AI 消息角色取值。
const (
	AIMessageRoleUser      = "user"
	AIMessageRoleAssistant = "assistant"
)

// AIMessage 为 AI 助手消息表；MetaJSON 保存 meta 载荷 JSON（snake_case 原样），
// 便于前端回放推理摘要/工具调用等详情。
type AIMessage struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID uint      `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"type:varchar(16);not null" json:"role"`
	Content        string    `gorm:"type:mediumtext" json:"content"`
	MetaJSON       *string   `gorm:"column:meta_json;type:text" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

// TableName 指定 AI 消息表名。
func (AIMessage) TableName() string {
	return "ai_messages"
}
