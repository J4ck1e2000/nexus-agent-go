package gateway

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	errAdminUserNotFound        = errors.New("admin_user_not_found")
	errCannotDeleteSelf         = errors.New("cannot_delete_self")
	errCannotDeleteLastAdmin    = errors.New("cannot_delete_last_admin")
	errCannotDowngradeSelf      = errors.New("cannot_downgrade_self")
	errCannotDowngradeLastAdmin = errors.New("cannot_downgrade_last_admin")
)

type updateAdminUserRoleRequest struct {
	Role string `json:"role"`
}

type createAdminUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type resetAdminUserPasswordRequest struct {
	Password string `json:"password"`
}

// createAdminUser creates a new user account from admin APIs.
func (h *Handler) createAdminUser(c *gin.Context) {
	if h == nil || h.auth == nil || h.auth.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var req createAdminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	role := UserRole(strings.ToLower(strings.TrimSpace(req.Role)))
	if role == "" {
		role = RoleUser
	}
	if role != RoleUser && role != RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_role"})
		return
	}

	user, err := h.auth.CreateUserByAdmin(req.Username, req.Password, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUsername):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_username"})
		case errors.Is(err, ErrPasswordTooShort):
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_too_short"})
		case errors.Is(err, ErrInvalidRole):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_role"})
		case errors.Is(err, ErrUserAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "user_already_exists"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}

	c.JSON(http.StatusCreated, user)
}

// listAdminUsers returns admin-visible users and supports username filtering.
func (h *Handler) listAdminUsers(c *gin.Context) {
	if h == nil || h.auth == nil || h.auth.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	query := h.auth.db.Model(&User{})
	if q != "" {
		pattern := "%" + strings.ToLower(q) + "%"
		query = query.Where("LOWER(username) LIKE ?", pattern)
	}

	var users []User
	if err := query.Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	c.JSON(http.StatusOK, users)
}

// deleteAdminUser removes a user with admin safety constraints.
func (h *Handler) deleteAdminUser(c *gin.Context) {
	if h == nil || h.auth == nil || h.auth.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	currentUser, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID, err := parseUintIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	err = h.auth.db.Transaction(func(tx *gorm.DB) error {
		var target User
		if err := tx.Where("id = ?", targetID).Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errAdminUserNotFound
			}
			return err
		}

		if target.Role == RoleAdmin {
			var adminCount int64
			if err := tx.Model(&User{}).Where("role = ?", RoleAdmin).Count(&adminCount).Error; err != nil {
				return err
			}
			if adminCount <= 1 {
				return errCannotDeleteLastAdmin
			}
		}

		if target.ID == currentUser.ID {
			return errCannotDeleteSelf
		}

		if err := tx.Delete(&User{}, target.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errAdminUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		case errors.Is(err, errCannotDeleteSelf):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_delete_self"})
		case errors.Is(err, errCannotDeleteLastAdmin):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_delete_last_admin"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// updateAdminUserRole changes a user's role with admin safety constraints.
func (h *Handler) updateAdminUserRole(c *gin.Context) {
	if h == nil || h.auth == nil || h.auth.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	currentUser, ok := currentAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID, err := parseUintIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var req updateAdminUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	role := UserRole(strings.ToLower(strings.TrimSpace(req.Role)))
	if role != RoleAdmin && role != RoleUser {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_role"})
		return
	}

	var updated User
	err = h.auth.db.Transaction(func(tx *gorm.DB) error {
		var target User
		if err := tx.Where("id = ?", targetID).Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errAdminUserNotFound
			}
			return err
		}

		if target.Role == RoleAdmin && role == RoleUser {
			var adminCount int64
			if err := tx.Model(&User{}).Where("role = ?", RoleAdmin).Count(&adminCount).Error; err != nil {
				return err
			}
			if adminCount <= 1 {
				return errCannotDowngradeLastAdmin
			}
			if target.ID == currentUser.ID {
				return errCannotDowngradeSelf
			}
		}

		target.Role = role
		if err := tx.Save(&target).Error; err != nil {
			return err
		}
		updated = target
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errAdminUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		case errors.Is(err, errCannotDowngradeSelf):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_downgrade_self"})
		case errors.Is(err, errCannotDowngradeLastAdmin):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_downgrade_last_admin"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}

	c.JSON(http.StatusOK, updated)
}

// resetAdminUserPassword resets a user's password using bcrypt hashing.
func (h *Handler) resetAdminUserPassword(c *gin.Context) {
	if h == nil || h.auth == nil || h.auth.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	targetID, err := parseUintIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var req resetAdminUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	if strings.TrimSpace(req.Password) == "" || len(req.Password) < minPasswordLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password_too_short"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	var target User
	if err := h.auth.db.Where("id = ?", targetID).Take(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	target.PasswordHash = string(hashed)
	if err := h.auth.db.Save(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func parseUintIDParam(raw string) (uint, error) {
	idVal, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(idVal), nil
}
