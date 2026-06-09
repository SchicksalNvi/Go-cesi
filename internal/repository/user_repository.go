package repository

import (
	"context"

	"gorm.io/gorm"
	"superview/internal/errors"
	"superview/internal/models"
)

type userRepository struct {
	BaseRepository
	db *gorm.DB
}

// NewUserRepository 创建用户仓库实例
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{
		BaseRepository: NewBaseRepository(db),
		db:             db,
	}
}

// WithContext 返回带上下文的用户仓库实例
func (r *userRepository) WithContext(ctx context.Context) UserRepository {
	return &userRepository{
		BaseRepository: r.BaseRepository.WithContext(ctx),
		db:             r.db,
	}
}

// WithTransaction 返回带事务的用户仓库实例
func (r *userRepository) WithTransaction(tx *gorm.DB) UserRepository {
	return &userRepository{
		BaseRepository: r.BaseRepository.WithTransaction(tx),
		db:             tx,
	}
}

// Create 创建用户
func (r *userRepository) Create(user *models.User) error {
	db := r.GetDB()
	if err := db.Create(user).Error; err != nil {
		return errors.NewDatabaseError("create user", err)
	}
	return nil
}

// GetByID 根据ID获取用户
func (r *userRepository) GetByID(id string) (*models.User, error) {
	db := r.GetDB()
	var user models.User
	if err := db.Preload("Roles.Permissions").Preload("NodeAccess").Preload("NodeAccess.Node").Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.NewNotFoundError("user", id)
		}
		return nil, errors.NewDatabaseError("get user by id", err)
	}
	return &user, nil
}

// GetByUsername 根据用户名获取用户
func (r *userRepository) GetByUsername(username string) (*models.User, error) {
	db := r.GetDB()
	var user models.User
	if err := db.Preload("Roles.Permissions").Preload("NodeAccess").Preload("NodeAccess.Node").Where("username = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.NewNotFoundError("user", username)
		}
		return nil, errors.NewDatabaseError("get user by username", err)
	}
	return &user, nil
}

// GetByEmail 根据邮箱获取用户
func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	db := r.GetDB()
	var user models.User
	if err := db.Preload("Roles.Permissions").Preload("NodeAccess").Preload("NodeAccess.Node").Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.NewNotFoundError("user", email)
		}
		return nil, errors.NewDatabaseError("get user by email", err)
	}
	return &user, nil
}

// Update 更新用户
func (r *userRepository) Update(user *models.User) error {
	db := r.GetDB()
	if err := db.Save(user).Error; err != nil {
		return errors.NewDatabaseError("update user", err)
	}
	return nil
}

// Delete 删除用户
func (r *userRepository) Delete(id string) error {
	db := r.GetDB()
	if err := db.Where("id = ?", id).Delete(&models.User{}).Error; err != nil {
		return errors.NewDatabaseError("delete user", err)
	}
	return nil
}

// List 获取用户列表
func (r *userRepository) List(offset, limit int) ([]*models.User, int64, error) {
	db := r.GetDB()
	var users []*models.User
	var total int64

	// 获取总数
	if err := db.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, 0, errors.NewDatabaseError("count users", err)
	}

	// 获取分页数据
	if err := db.Preload("Roles.Permissions").Preload("NodeAccess").Preload("NodeAccess.Node").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, errors.NewDatabaseError("list users", err)
	}

	return users, total, nil
}

// ExistsByUsername 检查用户名是否存在
func (r *userRepository) ExistsByUsername(username string) (bool, error) {
	db := r.GetDB()
	var count int64
	if err := db.Model(&models.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return false, errors.NewDatabaseError("check username exists", err)
	}
	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否存在
func (r *userRepository) ExistsByEmail(email string) (bool, error) {
	db := r.GetDB()
	var count int64
	if err := db.Model(&models.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, errors.NewDatabaseError("check email exists", err)
	}
	return count > 0, nil
}
