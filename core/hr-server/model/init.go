package model

import (
	"fmt"
	"strings"

	"github.com/kageos/kageos/pkg/openapitoken"
	"gorm.io/gorm"
)

// InitModels 初始化所有模型（自动迁移）
func InitModels(db *gorm.DB) error {
	if err := removeRetiredCompanySchema(db); err != nil {
		return err
	}

	// ⭐ 先创建被引用的表（父表），再创建引用它们的表（子表）
	// 这样可以确保外键约束能够正确创建
	err := db.AutoMigrate(
		// 第一层：基础表（不被其他表引用）
		&SystemSetting{},
		&SystemResourceSample{},
		&SystemCapacitySnapshot{},
		&SystemPlatformSnapshot{},
		&AuthLoginProvider{},
		&AuthOAuthState{},
		&AuthOAuthRegistrationIntent{},
		&AuthWechatLoginAttempt{},
		&User{}, // 被 UserSession、EmailVerification 引用

		// 第二层：依赖 User 的表
		&AuthExternalIdentity{},
		&UserStatusEvent{},
		&UserSession{},       // 引用 User
		&EmailVerification{}, // 引用 User
		&EmailCode{},         // 不引用其他表，但依赖 User 存在
		&openapitoken.OpenAPIToken{},

		// 第三层：部门表（自引用）
		&Department{}, // 自引用（ParentID -> ID）
	)
	if err != nil {
		return err
	}

	if err := ensureUserOrganizationMembership(db); err != nil {
		return err
	}
	if err := ensureUserEmailContactIndex(db); err != nil {
		return err
	}
	if err := ensureOAuthRegistrationIntentEmailNullable(db); err != nil {
		return err
	}

	return nil
}

// ensureUserOrganizationMembership keeps /org meaningful as the all-members
// principal. Every non-system account belongs to the organization, with
// /org/unassigned as the safe fallback.
func ensureUserOrganizationMembership(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&User{}) {
		return nil
	}
	if err := db.Model(&User{}).
		Where("username <> ?", "system").
		Where("department_full_path IS NULL OR TRIM(department_full_path) = ''").
		Update("department_full_path", "/org/unassigned").Error; err != nil {
		return fmt.Errorf("backfill user organization membership: %w", err)
	}
	return nil
}

// removeRetiredCompanySchema permanently removes the retired multi-company schema.
// The checks keep startup idempotent for both upgraded and newly created databases.
func removeRetiredCompanySchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	migrator := db.Migrator()
	for _, column := range []struct {
		model any
		name  string
	}{
		{model: &retiredUserCompanyColumn{}, name: "CompanyCode"},
		{model: &retiredOAuthCompanyColumn{}, name: "CompanyCode"},
	} {
		if !migrator.HasTable(column.model) || !migrator.HasColumn(column.model, column.name) {
			continue
		}
		if err := migrator.DropColumn(column.model, column.name); err != nil {
			return fmt.Errorf("drop retired column %s: %w", column.name, err)
		}
	}
	if migrator.HasTable("company") {
		if err := migrator.DropTable("company"); err != nil {
			return fmt.Errorf("drop retired company table: %w", err)
		}
	}
	return nil
}

type retiredUserCompanyColumn struct {
	CompanyCode string `gorm:"column:company_code"`
}

func (retiredUserCompanyColumn) TableName() string {
	return "user"
}

type retiredOAuthCompanyColumn struct {
	CompanyCode string `gorm:"column:company_code"`
}

func (retiredOAuthCompanyColumn) TableName() string {
	return "auth_oauth_registration_intents"
}

func ensureUserEmailContactIndex(db *gorm.DB) error {
	migrator := db.Migrator()
	indexes, err := migrator.GetIndexes(&User{})
	if err != nil {
		return err
	}
	for _, index := range indexes {
		unique, ok := index.Unique()
		if !ok || !unique {
			continue
		}
		columns := index.Columns()
		if len(columns) == 1 && strings.EqualFold(columns[0], "email") {
			if err := migrator.DropIndex(&User{}, index.Name()); err != nil {
				return err
			}
		}
	}
	if migrator.HasIndex(&User{}, "idx_user_email") {
		return nil
	}
	return migrator.CreateIndex(&User{}, "idx_user_email")
}

func ensureOAuthRegistrationIntentEmailNullable(db *gorm.DB) error {
	if db.Migrator().HasColumn(&AuthOAuthRegistrationIntent{}, "Email") {
		return db.Migrator().AlterColumn(&AuthOAuthRegistrationIntent{}, "Email")
	}
	return nil
}
