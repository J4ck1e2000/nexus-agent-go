package gateway

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	defaultInitAdminUsername = "admin"
	defaultInitAdminPassword = "admin123"
)

// InitMySQLFromEnv 从环境变量初始化 MySQL 连接并执行迁移。
func InitMySQLFromEnv() (*gorm.DB, error) {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	if dsn == "" {
		return nil, errors.New("MYSQL_DSN is required")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql failed: %w", err)
	}

	if err := AutoMigrate(db); err != nil {
		return nil, err
	}
	if err := InitDefaultAdminFromEnv(db); err != nil {
		return nil, err
	}
	return db, nil
}

// AutoMigrate 统一管理网关数据库迁移。
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if err := db.AutoMigrate(&User{}, &AgentNode{}); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}
	return nil
}

// InitDefaultAdminFromEnv 根据环境变量初始化默认管理员。
func InitDefaultAdminFromEnv(db *gorm.DB) error {
	username := strings.TrimSpace(os.Getenv("INIT_ADMIN_USERNAME"))
	if username == "" {
		username = defaultInitAdminUsername
	}

	password := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
	if password == "" {
		password = defaultInitAdminPassword
	}

	return EnsureDefaultAdmin(db, username, password)
}

// EnsureDefaultAdmin 在没有管理员时创建默认管理员。
func EnsureDefaultAdmin(db *gorm.DB, username, password string) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(username) == "" {
		return errors.New("default admin username is empty")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("default admin password is empty")
	}

	var adminCount int64
	if err := db.Model(&User{}).Where("role = ?", RoleAdmin).Count(&adminCount).Error; err != nil {
		return fmt.Errorf("count admin users failed: %w", err)
	}
	if adminCount > 0 {
		return nil
	}

	var existing User
	err := db.Where("username = ?", username).Take(&existing).Error
	if err == nil {
		// 用户已存在则跳过默认初始化。
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check default admin user failed: %w", err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default admin password failed: %w", err)
	}

	admin := User{
		Username:     username,
		PasswordHash: string(hashed),
		Role:         RoleAdmin,
	}
	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("create default admin failed: %w", err)
	}
	return nil
}
