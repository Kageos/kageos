package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"net/mail"
	"strings"

	"github.com/kageos/kageos/dto"
)

// ImportUsers validates all rows before any write. Each created username is unique;
// retries never overwrite existing accounts or reset their password.
func (s *UserService) ImportUsers(ctx context.Context, rows []dto.SystemCreateUserReq, preview bool, actor string) ([]dto.SystemImportUserResult, error) {
	if len(rows) == 0 || len(rows) > 100 {
		return nil, fmt.Errorf("每次导入 1–100 个用户")
	}
	results := make([]dto.SystemImportUserResult, len(rows))
	usernames, emails := map[string]bool{}, map[string]bool{}
	for i, row := range rows {
		row.Username = strings.ToLower(strings.TrimSpace(row.Username))
		row.Email = strings.ToLower(strings.TrimSpace(row.Email))
		// Pre-created accounts remain frozen until the administrator enables them.
		if row.Status == "" {
			row.Status = "disabled"
		}
		rows[i] = row
		result := dto.SystemImportUserResult{Row: i + 2, Username: row.Username, Status: "ready"}
		var err error
		switch {
		case usernames[row.Username]:
			err = fmt.Errorf("文件内用户名重复")
		case row.Email != "" && emails[row.Email]:
			err = fmt.Errorf("文件内邮箱重复")
		default:
			err = s.validateImportUser(row)
		}
		usernames[row.Username] = true
		if row.Email != "" {
			emails[row.Email] = true
		}
		if err != nil {
			result.Status = "failed"
			result.Message = err.Error()
		}
		results[i] = result
	}
	if preview {
		return results, nil
	}
	for i, row := range rows {
		if results[i].Status != "ready" {
			continue
		}
		if err := ctx.Err(); err != nil {
			results[i].Status = "failed"
			results[i].Message = "请求已中断，请重新校验后重试"
			continue
		}
		if _, err := s.CreateUserFromSystem(ctx, row, actor); err != nil {
			results[i].Status = "failed"
			results[i].Message = err.Error()
		} else {
			results[i].Status = "created"
		}
	}
	return results, nil
}

func (s *UserService) validateImportUser(row dto.SystemCreateUserReq) error {
	if err := ValidateUserCode(row.Username); err != nil {
		return err
	}
	if len(row.Password) < 6 || len(row.Password) > 72 || strings.TrimSpace(row.Password) == "" {
		return fmt.Errorf("密码须为 6–72 字节")
	}
	if len(row.Nickname) > 100 || len(row.Email) > 255 {
		return fmt.Errorf("昵称或邮箱过长")
	}
	if row.Email != "" {
		address, err := mail.ParseAddress(row.Email)
		if err != nil || address.Address != row.Email {
			return fmt.Errorf("邮箱格式不正确")
		}
	}
	if _, err := normalizeSystemUserStatus(row.Status, "disabled"); err != nil {
		return err
	}
	if _, err := s.normalizeAndValidateDepartmentPath(row.DepartmentFullPath); err != nil {
		return err
	}
	if err := s.validateLeader(strings.ToLower(strings.TrimSpace(row.LeaderUsername))); err != nil {
		return err
	}
	// Creation performs these checks again to handle races after preview.
	if _, err := s.userRepo.GetUserByUsername(row.Username); err == nil {
		return fmt.Errorf("用户名已存在，不会覆盖")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询用户失败")
	}
	if row.Email != "" {
		if _, err := s.userRepo.GetUserByEmail(row.Email); err == nil {
			return fmt.Errorf("邮箱已被注册")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询邮箱失败")
		}
	}
	return nil
}
