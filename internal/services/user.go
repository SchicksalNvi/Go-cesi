package services

import (
	"golang.org/x/crypto/bcrypt"
	"superview/internal/errors"
	"superview/internal/logger"
	"superview/internal/models"
	"superview/internal/repository"
	"time"

	"go.uber.org/zap"
)

type UserService struct {
	repo *repository.Repository
}

func NewUserService(repo *repository.Repository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) CreateUser(user *models.User) error {
	// 验证用户名是否已存在
	exists, err := s.repo.User.ExistsByUsername(user.Username)
	if err != nil {
		return err
	}
	if exists {
		return errors.NewConflictError("user", "username already exists")
	}

	// 验证邮箱是否已存在
	if user.Email != "" {
		exists, err = s.repo.User.ExistsByEmail(user.Email)
		if err != nil {
			return err
		}
		if exists {
			return errors.NewConflictError("user", "email already exists")
		}
	}

	// 设置默认值
	user.IsActive = true

	return s.repo.User.Create(user)
}

func (s *UserService) Authenticate(username, password string) (*models.User, error) {
	user, err := s.repo.User.GetByUsername(username)
	if err != nil {
		return nil, err
	}

	// 检查用户是否激活
	if !user.IsActive {
		return nil, errors.NewUnauthorizedError("user account is disabled")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.NewUnauthorizedError("invalid credentials")
	}

	// 更新最后登录时间
	now := time.Now()
	user.LastLogin = &now
	if err := s.repo.User.UpdateFieldsWithVersion(user.ID, map[string]interface{}{"last_login": now}, user.TokenVersion); err != nil {
		logger.Warn("Failed to update last login time",
			zap.String("user_id", user.ID),
			zap.String("username", user.Username),
			zap.Error(err))
	}

	return user, nil
}

// GetUserByID 根据ID获取用户
func (s *UserService) GetUserByID(id string) (*models.User, error) {
	return s.repo.User.GetByID(id)
}

// GetUserByUsername 根据用户名获取用户
func (s *UserService) GetUserByUsername(username string) (*models.User, error) {
	return s.repo.User.GetByUsername(username)
}

// UpdateUser 更新用户信息
func (s *UserService) UpdateUser(user *models.User) error {
	existingUser, err := s.repo.User.GetByID(user.ID)
	if err != nil {
		return err
	}

	// 如果更新用户名，检查是否已存在
	if user.Username != "" {
		if existingUser.Username != user.Username {
			exists, err := s.repo.User.ExistsByUsername(user.Username)
			if err != nil {
				return err
			}
			if exists {
				return errors.NewConflictError("user", "username already exists")
			}
		}
	}

	// 如果更新邮箱，检查是否已存在
	if user.Email != "" {
		if existingUser.Email != user.Email {
			exists, err := s.repo.User.ExistsByEmail(user.Email)
			if err != nil {
				return err
			}
			if exists {
				return errors.NewConflictError("user", "email already exists")
			}
		}
	}

	passwordChanged := existingUser.Password != user.Password
	activeChanged := existingUser.IsActive != user.IsActive
	// Always check for concurrent session state changes
	if existingUser.TokenVersion != user.TokenVersion {
		return errors.NewConflictError("user", "user session state changed concurrently; reload and retry")
	}

	updates := make(map[string]interface{})
	if existingUser.Username != user.Username {
		updates["username"] = user.Username
	}
	if passwordChanged {
		updates["password"] = user.Password
	}
	if existingUser.Email != user.Email {
		updates["email"] = user.Email
	}
	if existingUser.FullName != user.FullName {
		updates["full_name"] = user.FullName
	}
	if activeChanged {
		updates["is_active"] = user.IsActive
	}
	if existingUser.IsAdmin != user.IsAdmin {
		updates["is_admin"] = user.IsAdmin
	}
	if len(updates) == 0 {
		return nil
	}

	revokeSessions := passwordChanged || (existingUser.IsActive && !user.IsActive) || (existingUser.IsAdmin != user.IsAdmin)
	if revokeSessions {
		return s.repo.User.UpdateFieldsAndRevokeSessions(user.ID, updates)
	}

	return s.repo.User.UpdateFields(user.ID, updates)
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(id string) error {
	// 检查用户是否存在
	_, err := s.repo.User.GetByID(id)
	if err != nil {
		return err
	}

	return s.repo.User.Delete(id)
}

// ListUsers 获取用户列表
func (s *UserService) ListUsers(page, pageSize int) ([]*models.User, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	return s.repo.User.List(offset, pageSize)
}

// ChangePassword 修改用户密码
func (s *UserService) ChangePassword(userID, oldPassword, newPassword string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.NewUnauthorizedError("invalid old password")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.NewInternalError("failed to hash password", err)
	}

	return s.SetPasswordAndRevokeSessions(user.ID, string(hashedPassword))
}

// SetPasswordAndRevokeSessions atomically stores a password hash and revokes
// every token issued before the change.
func (s *UserService) SetPasswordAndRevokeSessions(userID, passwordHash string) error {
	return s.repo.User.UpdateFieldsAndRevokeSessions(userID, map[string]interface{}{
		"password": passwordHash,
	})
}

// ActivateUser 激活用户
func (s *UserService) ActivateUser(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	return s.repo.User.UpdateFields(user.ID, map[string]interface{}{"is_active": true})
}

// DeactivateUser 停用用户
func (s *UserService) DeactivateUser(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	if user.IsActive {
		return s.repo.User.UpdateFieldsAndRevokeSessions(user.ID, map[string]interface{}{
			"is_active": false,
		})
	}
	return nil
}

// PromoteToAdmin 提升为管理员
func (s *UserService) PromoteToAdmin(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	return s.repo.User.UpdateFields(user.ID, map[string]interface{}{"is_admin": true})
}

// DemoteFromAdmin 取消管理员权限
func (s *UserService) DemoteFromAdmin(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	return s.repo.User.UpdateFields(user.ID, map[string]interface{}{"is_admin": false})
}
