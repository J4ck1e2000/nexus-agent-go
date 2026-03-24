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
	URL       string    `gorm:"type:varchar(255);not null;uniqueIndex:uk_agent_nodes_url" json:"url"`
	CreatedBy uint      `gorm:"column:created_by;index;not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定节点表名。
func (AgentNode) TableName() string {
	return "agent_nodes"
}
