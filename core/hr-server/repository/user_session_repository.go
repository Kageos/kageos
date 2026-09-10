package repository

import (
	"fmt"
	"gorm.io/gorm/clause"
	"time"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/pkg/gormx/models"
	"gorm.io/gorm"
)

type UserSessionRepository struct {
	db *gorm.DB
}

func NewUserSessionRepository(db *gorm.DB) *UserSessionRepository {
	return &UserSessionRepository{db: db}
}

// CreateUserSession 创建用户会话
func (r *UserSessionRepository) CreateUserSession(userID int64, token, refreshToken string, expiresAt models.Time, userAgent, ipAddress string) error {
	session := model.UserSession{
		UserID:       userID,
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
	}
	return r.db.Create(&session).Error
}

// GetUserSessionByToken 根据token获取用户会话
func (r *UserSessionRepository) GetUserSessionByToken(token string) (*model.UserSession, error) {
	var session model.UserSession
	err := r.db.Where("token = ? AND is_active = true AND expires_at > ?", token, models.Time(time.Now())).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetUserSessionByRefreshToken 根据refresh token获取用户会话
func (r *UserSessionRepository) GetUserSessionByRefreshToken(refreshToken string) (*model.UserSession, error) {
	var session model.UserSession
	err := r.db.Where("refresh_token = ? AND is_active = true AND expires_at > ?", refreshToken, models.Time(time.Now())).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// DeactivateUserSession 停用用户会话
func (r *UserSessionRepository) DeactivateUserSession(token string) error {
	result := r.db.Model(&model.UserSession{}).
		Where("token = ? AND is_active = true", token).
		Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeactivateAllUserSessions 停用用户的所有会话
func (r *UserSessionRepository) DeactivateAllUserSessions(userID int64) error {
	return r.db.Model(&model.UserSession{}).Where("user_id = ?", userID).Update("is_active", false).Error
}

// DeleteExpiredSessions 删除过期的会话
func (r *UserSessionRepository) DeleteExpiredSessions() error {
	return r.db.Where("expires_at < ?", models.Time(time.Now())).Delete(&model.UserSession{}).Error
}

// UpdateUserSessionTokens 更新用户会话的token和refresh token
func (r *UserSessionRepository) UpdateUserSessionTokens(sessionID int64, token, refreshToken string) error {
	return r.db.Model(&model.UserSession{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"token":         token,
		"refresh_token": refreshToken,
	}).Error
}

// GetUserSessionByID 根据ID获取用户会话
func (r *UserSessionRepository) GetUserSessionByID(id int64) (*model.UserSession, error) {
	var session model.UserSession
	err := r.db.Where("id = ?", id).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetUserSessionsByUserID 根据用户ID获取所有会话
func (r *UserSessionRepository) GetUserSessionsByUserID(userID int64) ([]*model.UserSession, error) {
	var sessions []*model.UserSession
	err := r.db.Where("user_id = ?", userID).Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveSessionsByUserID 根据用户ID获取所有活跃会话
func (r *UserSessionRepository) GetActiveSessionsByUserID(userID int64) ([]*model.UserSession, error) {
	var sessions []*model.UserSession
	now := models.Time(time.Now())
	err := r.db.Where("user_id = ? AND is_active = true AND expires_at > ?", userID, now).Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// CreateActiveUserSession serializes session issuance with account freezing.
func (r *UserSessionRepository) CreateActiveUserSession(userID int64, token, refreshToken string, expiresAt models.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		if !user.IsActive() {
			return fmt.Errorf("账户已停用")
		}
		return NewUserSessionRepository(tx).CreateUserSession(userID, token, refreshToken, expiresAt, "", "")
	})
}
