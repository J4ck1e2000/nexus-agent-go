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
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(128);not null" json:"name"`
	// CollectorType 为采集方式（agent/ssh）。历史数据经迁移默认填 agent，
	// 业务层对空值同样按 agent 处理。
	CollectorType string `gorm:"type:varchar(16);not null;default:'agent'" json:"collector_type"`
	// URL 为 agent 模式的 Agent 地址；SSH 节点无 URL，存 NULL 以允许多条
	// 记录共用唯一索引 uk_agent_nodes_url。
	URL string `gorm:"type:varchar(255);uniqueIndex:uk_agent_nodes_url;default:null" json:"url,omitempty"`
	// SSHHost/SSHPort/SSHUser/SSHAuthType 为 ssh 模式连接信息，
	// SSH 私钥永不入库。
	SSHHost     string    `gorm:"type:varchar(255);not null;default:''" json:"ssh_host,omitempty"`
	SSHPort     int       `gorm:"not null;default:0" json:"ssh_port,omitempty"`
	SSHUser     string    `gorm:"type:varchar(64);not null;default:''" json:"ssh_user,omitempty"`
	SSHAuthType string    `gorm:"type:varchar(16);not null;default:''" json:"ssh_auth_type,omitempty"`
	CreatedBy   uint      `gorm:"column:created_by;index;not null" json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定节点表名。
func (AgentNode) TableName() string {
	return "agent_nodes"
}
