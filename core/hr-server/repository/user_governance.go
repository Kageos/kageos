package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/pkg/openapitoken"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChangeUserStatus commits the account state and credential revocation together.
// Returning already revoked credentials lets a repeated request retry cache notifications.
func (r *UserRepository) ChangeUserStatus(ctx context.Context, username, status, actor, reason string) (*model.User, []*model.UserSession, []openapitoken.OpenAPIToken, error) {
	var user model.User
	var sessions []*model.UserSession
	var tokens []openapitoken.OpenAPIToken
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("username = ?", username).First(&user).Error; err != nil {
			return err
		}
		if status != "active" {
			if err := tx.Where("user_id = ? AND expires_at > ?", user.ID, time.Now()).Find(&sessions).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.UserSession{}).Where("user_id = ?", user.ID).Update("is_active", false).Error; err != nil {
				return err
			}
			if err := tx.Where("owner_username = ?", username).Find(&tokens).Error; err != nil {
				return err
			}
			if err := tx.Model(&openapitoken.OpenAPIToken{}).Where("owner_username = ? AND revoked_at IS NULL", username).Update("revoked_at", time.Now()).Error; err != nil {
				return err
			}
		}
		previousStatus := user.Status
		updates := map[string]interface{}{"status": status}
		if status == "active" {
			updates["email_verified"] = true
		}
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserStatusEvent{Username: username, Actor: actor, PreviousStatus: previousStatus, Status: status, Reason: reason}).Error
	})
	return &user, sessions, tokens, err
}

func (r *UserRepository) CreateActiveUserOpenAPIToken(ctx context.Context, store *openapitoken.Store, input openapitoken.CreateInput) (*openapitoken.CreateResult, error) {
	return store.CreateGuarded(input, func(tx *gorm.DB) error {
		var user model.User
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("username = ?", input.OwnerUsername).First(&user).Error; err != nil {
			return err
		}
		if !user.IsActive() || user.ID != input.OwnerUserID {
			return fmt.Errorf("账户已停用或身份已变更")
		}
		return nil
	})
}
