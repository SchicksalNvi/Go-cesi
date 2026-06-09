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
	if err := s.repo.User.Update(user); err != nil {
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
	// 如果更新用户名，检查是否已存在
	if user.Username != "" {
		existingUser, err := s.repo.User.GetByID(user.ID)
		if err != nil {
			return err
		}

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
		existingUser, err := s.repo.User.GetByID(user.ID)
		if err != nil {
			return err
		}

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

	return s.repo.User.Update(user)
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

	user.Password = string(hashedPassword)
	return s.repo.User.Update(user)
}

// ActivateUser 激活用户
func (s *UserService) ActivateUser(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	user.IsActive = true
	return s.repo.User.Update(user)
}

// DeactivateUser 停用用户
func (s *UserService) DeactivateUser(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	user.IsActive = false
	return s.repo.User.Update(user)
}

// PromoteToAdmin 提升为管理员
func (s *UserService) PromoteToAdmin(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	user.IsAdmin = true
	return s.repo.User.Update(user)
}

// DemoteFromAdmin 取消管理员权限
func (s *UserService) DemoteFromAdmin(userID string) error {
	user, err := s.repo.User.GetByID(userID)
	if err != nil {
		return err
	}

	user.IsAdmin = false
	return s.repo.User.Update(user)
}
