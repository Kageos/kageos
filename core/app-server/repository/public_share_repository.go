package repository

import (
	"context"
	"strings"
	"time"

	"github.com/kageos/kageos/core/app-server/model"
	"gorm.io/gorm"
)

type PublicShareRepository struct {
	db *gorm.DB
}

func NewPublicShareRepository(db *gorm.DB) *PublicShareRepository {
	return &PublicShareRepository{db: db}
}

func (r *PublicShareRepository) Create(ctx context.Context, share *model.PublicShare) error {
	return r.db.WithContext(ctx).Create(share).Error
}

func (r *PublicShareRepository) GetByShareID(ctx context.Context, shareID string) (*model.PublicShare, error) {
	var share model.PublicShare
	if err := r.db.WithContext(ctx).Where("share_id = ?", shareID).First(&share).Error; err != nil {
		return nil, err
	}
	return &share, nil
}

type PublicShareListFilter struct {
	FullCodePath string
	Keyword      string
	CreatedBy    string
	Status       string
}

func (r *PublicShareRepository) List(ctx context.Context, tenantUser, app string, filter PublicShareListFilter) ([]*model.PublicShare, error) {
	query := r.db.WithContext(ctx).Where("tenant_user = ? AND app = ?", tenantUser, app)
	if filter.FullCodePath != "" {
		query = query.Where("full_code_path = ?", filter.FullCodePath)
	}
	if filter.Keyword != "" {
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		query = query.Where("(share_id LIKE ? OR title LIKE ? OR description LIKE ?)", keyword, keyword, keyword)
	}
	if filter.CreatedBy != "" {
		query = query.Where("created_by = ?", strings.TrimSpace(filter.CreatedBy))
	}
	switch strings.TrimSpace(filter.Status) {
	case "enabled":
		query = query.Where("enabled = ? AND (expires_at IS NULL OR expires_at > ?) AND (max_uses = 0 OR use_count < max_uses)", true, time.Now())
	case "exhausted":
		query = query.Where("enabled = ? AND (expires_at IS NULL OR expires_at > ?) AND max_uses > 0 AND use_count >= max_uses", true, time.Now())
	case "disabled":
		query = query.Where("enabled = ?", false)
	case "expired":
		query = query.Where("enabled = ? AND expires_at IS NOT NULL AND expires_at <= ?", true, time.Now())
	}
	var shares []*model.PublicShare
	if err := query.Order("created_at DESC").Find(&shares).Error; err != nil {
		return nil, err
	}
	return shares, nil
}

func (r *PublicShareRepository) Disable(ctx context.Context, shareID, actor string) error {
	return r.db.WithContext(ctx).Model(&model.PublicShare{}).
		Where("share_id = ?", shareID).
		Updates(map[string]interface{}{
			"enabled":    false,
			"updated_by": actor,
		}).Error
}

func (r *PublicShareRepository) ReserveUse(ctx context.Context, shareID string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.PublicShare{}).
		Where(
			"share_id = ? AND enabled = ? AND (expires_at IS NULL OR expires_at > ?) AND (max_uses = 0 OR use_count < max_uses)",
			shareID,
			true,
			now,
		).
		Updates(map[string]interface{}{
			"use_count":    gorm.Expr("use_count + 1"),
			"last_used_at": &now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *PublicShareRepository) ReleaseUse(ctx context.Context, shareID string) error {
	return r.db.WithContext(ctx).Model(&model.PublicShare{}).
		Where("share_id = ? AND use_count > 0", shareID).
		UpdateColumn("use_count", gorm.Expr("use_count - 1")).Error
}

func (r *PublicShareRepository) CreateEvent(ctx context.Context, event *model.PublicShareEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}
