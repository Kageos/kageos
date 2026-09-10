package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/core/hr-server/repository"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/logger"
	"github.com/kageos/kageos/pkg/openapitoken"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	userRepo        *repository.UserRepository
	tokenPublisher  TokenPublisher                    // 可选：向 gateway 发布 token 命令
	userSessionRepo *repository.UserSessionRepository // ⭐ 新增：用户会话仓库（用于查询活跃会话）
	departmentRepo  *repository.DepartmentRepository
}

// NewUserService 创建用户服务（依赖注入）
func NewUserService(
	userRepo *repository.UserRepository,
	tokenPublisher TokenPublisher,
	userSessionRepo *repository.UserSessionRepository,
	departmentRepo *repository.DepartmentRepository,
) *UserService {
	return &UserService{
		userRepo:        userRepo,
		tokenPublisher:  tokenPublisher,
		userSessionRepo: userSessionRepo,
		departmentRepo:  departmentRepo,
	}
}

// GetUserByUsername 根据用户名获取用户信息
func (s *UserService) GetUserByUsername(username string) (*model.User, error) {
	return s.userRepo.GetUserByUsername(username)
}

// SearchUsersFuzzy 模糊查询用户
func (s *UserService) SearchUsersFuzzy(keyword string, limit int) ([]*model.User, error) {
	// 限制查询数量，防止大量数据查询
	if limit <= 0 {
		limit = 10 // 默认10条
	}
	if limit > 100 {
		limit = 100 // 最大100条
	}
	return s.userRepo.SearchUsersFuzzy(keyword, limit)
}

func (s *UserService) ListUsersForSystem(req dto.SystemListUsersReq) ([]*model.User, int64, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	return s.userRepo.ListUsersForSystem(req.Keyword, strings.TrimSpace(req.Status), strings.TrimSpace(req.RegisterType), req.Page, req.PageSize)
}

func (s *UserService) CreateUserFromSystem(ctx context.Context, req dto.SystemCreateUserReq, createdBy string) (*model.User, error) {
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if err := ValidateUserCode(username); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Password) == "" {
		return nil, fmt.Errorf("密码不能为空")
	}
	if len(req.Password) < 6 {
		return nil, fmt.Errorf("密码至少 6 位")
	}
	if existingUser, err := s.userRepo.GetUserByUsername(username); err == nil && existingUser != nil {
		return nil, fmt.Errorf("用户名已存在")
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("检查用户名失败: %w", err)
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		email = placeholderSystemUserEmail(username)
	} else if existingEmail, err := s.userRepo.GetUserByEmail(email); err == nil && existingEmail != nil {
		return nil, fmt.Errorf("邮箱已被注册")
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("检查邮箱失败: %w", err)
	}

	status, err := normalizeSystemUserStatus(req.Status, "active")
	if err != nil {
		return nil, err
	}
	departmentFullPath, err := s.normalizeAndValidateDepartmentPath(req.DepartmentFullPath)
	if err != nil {
		return nil, err
	}
	leaderUsername := strings.ToLower(strings.TrimSpace(req.LeaderUsername))
	if err := s.validateLeader(leaderUsername); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf(ctx, "[UserService] Failed to hash system-created user password: %v", err)
		return nil, fmt.Errorf("密码加密失败")
	}

	user := &model.User{
		Username:           username,
		Email:              email,
		PasswordHash:       string(hashedPassword),
		RegisterType:       "system",
		Status:             status,
		EmailVerified:      status != "pending",
		CreatedBy:          firstNonEmptyString(createdBy, SystemUsername),
		Nickname:           strings.TrimSpace(req.Nickname),
		DepartmentFullPath: departmentFullPath,
		LeaderUsername:     leaderUsername,
		Type:               model.UserTypeNormal,
	}
	if err := s.userRepo.CreateUser(user); err != nil {
		logger.Errorf(ctx, "[UserService] Failed to create user from system: %v", err)
		return nil, fmt.Errorf("用户创建失败")
	}
	logger.Infof(ctx, "[UserService] system created user: %s", username)
	return user, nil
}

func (s *UserService) UpdateUserFromSystem(ctx context.Context, username string, req dto.SystemUpdateUserReq) (*model.User, error) {
	user, err := s.userRepo.GetUserByUsername(strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}
	profileChanged := false

	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email != "" && email != user.Email {
			if existingEmail, err := s.userRepo.GetUserByEmail(email); err == nil && existingEmail != nil && existingEmail.Username != user.Username {
				return nil, fmt.Errorf("邮箱已被注册")
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("检查邮箱失败: %w", err)
			}
		}
		user.Email = email
		profileChanged = true
	}
	if req.Nickname != nil {
		user.Nickname = strings.TrimSpace(*req.Nickname)
		profileChanged = true
	}
	if req.Signature != nil {
		user.Signature = strings.TrimSpace(*req.Signature)
		profileChanged = true
	}
	if req.Avatar != nil {
		user.Avatar = strings.TrimSpace(*req.Avatar)
		profileChanged = true
	}
	if req.Gender != nil {
		user.Gender = strings.TrimSpace(*req.Gender)
		profileChanged = true
	}

	if req.DepartmentFullPath != nil {
		if user.Username == SystemUsername {
			return nil, fmt.Errorf("不能修改 system 用户组织归属")
		}
		departmentFullPath, err := s.normalizeAndValidateDepartmentPath(*req.DepartmentFullPath)
		if err != nil {
			return nil, err
		}
		user.DepartmentFullPath = departmentFullPath
		profileChanged = true
	}
	if req.LeaderUsername != nil {
		if user.Username == SystemUsername {
			return nil, fmt.Errorf("不能修改 system 用户上级")
		}
		leaderUsername := strings.ToLower(strings.TrimSpace(*req.LeaderUsername))
		if err := s.validateLeader(leaderUsername); err != nil {
			return nil, err
		}
		user.LeaderUsername = leaderUsername
		profileChanged = true
	}

	if !profileChanged {
		return user, nil
	}
	if err := s.userRepo.UpdateUser(user); err != nil {
		return nil, fmt.Errorf("更新用户失败: %w", err)
	}
	if req.DepartmentFullPath != nil || req.LeaderUsername != nil {
		if err := s.invalidateUserTokens(ctx, user, "system_user_profile_changed"); err != nil {
			return nil, err
		}
	}
	logger.Infof(ctx, "[UserService] system updated user: %s", user.Username)
	return user, nil
}

func (s *UserService) ResetUserPasswordFromSystem(ctx context.Context, username, password string) (*model.User, error) {
	user, err := s.userRepo.GetUserByUsername(strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("密码至少 6 位")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败")
	}
	user.PasswordHash = string(hashedPassword)
	if user.RegisterType != "email" && user.RegisterType != "system" {
		user.RegisterType = "system"
	}
	if err := s.userRepo.UpdateUser(user); err != nil {
		return nil, fmt.Errorf("重置密码失败: %w", err)
	}
	if err := s.invalidateUserTokens(ctx, user, "system_user_password_reset"); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "[UserService] system reset password for user: %s", user.Username)
	return user, nil
}

// ChangeOwnPassword 校验当前密码后修改登录用户自己的密码。
func (s *UserService) ChangeOwnPassword(ctx context.Context, username, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetUserByUsername(strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if user.PasswordHash == "" {
		return fmt.Errorf("当前账号未设置密码")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("当前密码错误")
	}
	if len(newPassword) < 6 {
		return fmt.Errorf("新密码至少 6 位")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(newPassword)) == nil {
		return fmt.Errorf("新密码不能与当前密码相同")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败")
	}
	user.PasswordHash = string(hashedPassword)
	if err := s.userRepo.UpdateUser(user); err != nil {
		return fmt.Errorf("修改密码失败: %w", err)
	}
	if err := s.invalidateUserTokens(ctx, user, "user_password_changed"); err != nil {
		return err
	}
	logger.Infof(ctx, "[UserService] user changed own password: %s", user.Username)
	return nil
}

func (s *UserService) UpdateUserStatusFromSystem(ctx context.Context, username, status string, audit ...string) (*model.User, error) {
	user, err := s.userRepo.GetUserByUsername(strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}
	status, err = normalizeSystemUserStatus(status, "")
	if err != nil {
		return nil, err
	}
	if user.Username == SystemUsername && status != "active" {
		return nil, fmt.Errorf("不能停用 system 用户")
	}
	actor, reason := SystemUsername, "管理员调整账户状态"
	if len(audit) > 0 {
		actor = audit[0]
	}
	if len(audit) > 1 && strings.TrimSpace(audit[1]) != "" {
		reason = strings.TrimSpace(audit[1])
	}
	if len([]rune(reason)) > 500 {
		return nil, fmt.Errorf("原因不能超过 500 字")
	}
	user, sessions, tokens, err := s.userRepo.ChangeUserStatus(ctx, user.Username, status, actor, reason)
	if err != nil {
		return nil, fmt.Errorf("更新账户及撤销凭证失败: %w", err)
	}
	if status != "active" && s.tokenPublisher != nil {
		var notificationErrors []error
		if err := s.tokenPublisher.InvalidateUserTokens(ctx, user.ID, user.Username, sessions, "system_user_status_changed"); err != nil {
			notificationErrors = append(notificationErrors, err)
		}
		for _, token := range tokens {
			if err := s.tokenPublisher.InvalidateOpenAPIToken(ctx, user.ID, user.Username, token.TokenHash, token.ExpiresAt); err != nil {
				notificationErrors = append(notificationErrors, err)
			}
		}
		if len(notificationErrors) > 0 {
			return nil, fmt.Errorf("账户及凭证已停用，网关通知失败，请重试停用操作（缓存最多保留一分钟）: %w", errors.Join(notificationErrors...))
		}
	}
	logger.Infof(ctx, "[UserService] system updated user status: %s status=%s", user.Username, status)
	return user, nil
}

// GetUsersByUsernames 批量获取用户信息
func (s *UserService) GetUsersByUsernames(usernames []string) ([]*model.User, error) {
	// 限制批量查询数量，防止大量数据查询
	if len(usernames) > 100 {
		logger.Warnf(nil, "[UserService] Too many usernames in batch query, limiting to 100")
		usernames = usernames[:100]
	}
	return s.userRepo.GetUsersByUsernames(usernames)
}

// UpdateUser 更新用户信息（只更新提供的字段，空字符串会被忽略）
func (s *UserService) UpdateUser(username string, nickname, signature, avatar, gender *string) (*model.User, error) {
	// 获取用户
	user, err := s.userRepo.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	// 更新字段（只更新非 nil 的字段）
	if nickname != nil {
		user.Nickname = *nickname
	}
	if signature != nil {
		user.Signature = *signature
	}
	if avatar != nil {
		user.Avatar = *avatar
	}
	if gender != nil {
		user.Gender = *gender
	}

	// 保存更新
	err = s.userRepo.UpdateUser(user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// AssignUserOrganization 分配用户组织架构
func (s *UserService) AssignUserOrganization(ctx context.Context, username string, departmentFullPath *string, leaderUsername *string) (*model.User, error) {
	// 获取用户
	user, err := s.userRepo.GetUserByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}

	// 更新部门和 Leader
	departmentPath := ""
	if departmentFullPath != nil {
		departmentPath = *departmentFullPath
	}
	departmentPath, err = s.normalizeAndValidateDepartmentPath(departmentPath)
	if err != nil {
		return nil, err
	}
	user.DepartmentFullPath = departmentPath

	if leaderUsername != nil {
		// 验证 Leader 是否存在
		leader, err := s.userRepo.GetUserByUsername(*leaderUsername)
		if err != nil {
			return nil, fmt.Errorf("Leader 用户不存在: %w", err)
		}
		user.LeaderUsername = leader.Username
	} else {
		user.LeaderUsername = "" // 清空 Leader
	}

	// 保存到数据库
	if err := s.userRepo.UpdateUser(user); err != nil {
		return nil, fmt.Errorf("更新用户失败: %w", err)
	}

	if err := s.invalidateUserTokens(ctx, user, "organization_changed"); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "[UserService] User organization assigned: %s, department: %s, leader: %s", username, user.DepartmentFullPath, user.LeaderUsername)
	return user, nil
}

func (s *UserService) normalizeAndValidateDepartmentPath(departmentPath string) (string, error) {
	departmentPath = strings.TrimSpace(departmentPath)
	if departmentPath == "" {
		departmentPath = "/org/unassigned"
	}
	departmentPath = "/" + strings.Trim(departmentPath, "/")
	if departmentPath != "/org" && !strings.HasPrefix(departmentPath, "/org/") {
		return "", fmt.Errorf("组织路径必须位于 /org 下")
	}
	if s.departmentRepo == nil {
		return departmentPath, nil
	}
	department, err := s.departmentRepo.GetDepartmentByFullCodePath(departmentPath)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("组织不存在: %s", departmentPath)
		}
		return "", fmt.Errorf("查询组织失败: %w", err)
	}
	if !department.IsActive() {
		return "", fmt.Errorf("组织已停用: %s", departmentPath)
	}
	return departmentPath, nil
}

// GetUsersByDepartmentFullPath 根据部门完整路径获取用户列表
func (s *UserService) GetUsersByDepartmentFullPath(ctx context.Context, departmentFullPath string) ([]*model.User, error) {
	return s.userRepo.GetUsersByDepartmentFullPath(departmentFullPath)
}

func (s *UserService) validateLeader(leaderUsername string) error {
	if leaderUsername == "" {
		return nil
	}
	_, err := s.userRepo.GetUserByUsername(leaderUsername)
	if err != nil {
		return fmt.Errorf("Leader 用户不存在: %w", err)
	}
	return nil
}

func (s *UserService) invalidateUserTokens(ctx context.Context, user *model.User, reason string) error {
	if user == nil {
		return nil
	}
	sessions, err := s.userSessionRepo.GetActiveSessionsByUserID(user.ID)
	if err != nil {
		return fmt.Errorf("查询用户活跃会话失败: %w", err)
	}
	if err := s.userSessionRepo.DeactivateAllUserSessions(user.ID); err != nil {
		return fmt.Errorf("停用用户会话失败: %w", err)
	}
	if s.tokenPublisher != nil && len(sessions) > 0 {
		if err := s.tokenPublisher.InvalidateUserTokens(ctx, user.ID, user.Username, sessions, reason); err != nil {
			logger.Warnf(ctx, "[UserService] 会话已停用，但发送网关失效通知失败: %v", err)
		}
	}
	return nil
}

func normalizeSystemUserStatus(status, defaultStatus string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = defaultStatus
	}
	switch status {
	case "active", "pending", "disabled":
		return status, nil
	default:
		return "", fmt.Errorf("用户状态必须是 active、pending 或 disabled")
	}
}

func placeholderSystemUserEmail(username string) string {
	return username + "@users.local"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *UserService) CreateActiveUserOpenAPIToken(ctx context.Context, store *openapitoken.Store, input openapitoken.CreateInput) (*openapitoken.CreateResult, error) {
	return s.userRepo.CreateActiveUserOpenAPIToken(ctx, store, input)
}
