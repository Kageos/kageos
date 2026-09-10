package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/core/hr-server/repository"
	"github.com/kageos/kageos/pkg/auth"
	appconfig "github.com/kageos/kageos/pkg/config"
	"github.com/kageos/kageos/pkg/gormx/models"
	"github.com/kageos/kageos/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务
type AuthService struct {
	config          *appconfig.HRServerConfig
	jwtService      *auth.JWTService
	userRepo        *repository.UserRepository
	userSessionRepo *repository.UserSessionRepository
	tokenPublisher  TokenPublisher // 可选：向 gateway 发布 token 命令
}

// NewAuthService 创建认证服务（依赖注入）
func NewAuthService(userRepo *repository.UserRepository, userSessionRepo *repository.UserSessionRepository, tokenPublisher TokenPublisher) *AuthService {
	config := appconfig.GetHRServerConfig()
	jwtService := auth.NewJWTService()
	return &AuthService{
		config:          config,
		jwtService:      jwtService,
		userRepo:        userRepo,
		userSessionRepo: userSessionRepo,
		tokenPublisher:  tokenPublisher,
	}
}

const (
	defaultRefreshTokenExpireSeconds  = 24 * 3600
	rememberRefreshTokenExpireSeconds = 30 * 24 * 3600
)

// RegisterUser 注册用户
func (s *AuthService) RegisterUser(username, email, password, termsVersion, privacyVersion string) (int64, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := ValidateUserCode(username); err != nil {
		return 0, err
	}

	// 检查用户名是否已存在
	existingUser, err := s.userRepo.GetUserByUsername(username)
	if err == nil && existingUser != nil {
		return 0, fmt.Errorf("用户名已存在")
	}

	// 检查邮箱是否已存在
	existingEmail, err := s.userRepo.GetUserByEmail(email)
	if err == nil && existingEmail != nil {
		return 0, fmt.Errorf("邮箱已被注册")
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to hash password: %v", err)
		return 0, fmt.Errorf("密码加密失败")
	}

	// 创建用户（不再分配 HostID，Host 和 Nats 绑定在 App 上）
	// ⭐ 默认分配到未分配组织
	acceptedAt := models.Time(time.Now())
	user := &model.User{
		Username:           username,
		Email:              email,
		PasswordHash:       string(hashedPassword),
		RegisterType:       "email",
		Status:             "pending", // 待邮箱验证
		EmailVerified:      false,
		CreatedBy:          "system",
		DepartmentFullPath: "/org/unassigned", // ⭐ 默认分配到未分配组织
		TermsVersion:       termsVersion,
		PrivacyVersion:     privacyVersion,
		LegalAcceptedAt:    &acceptedAt,
	}

	// 保存到数据库
	err = s.userRepo.CreateUser(user)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to create user: %v", err)
		return 0, fmt.Errorf("用户创建失败")
	}

	logger.Infof(nil, "[AuthService] User registered successfully: %s", username)
	return user.ID, nil
}

// ActivateUser 激活用户
func (s *AuthService) ActivateUser(userID int64) error {
	// 获取用户信息
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to get user %d: %v", userID, err)
		return fmt.Errorf("用户不存在")
	}

	// 更新用户状态为active，并标记邮箱已验证
	user.Status = "active"
	user.EmailVerified = true
	err = s.userRepo.UpdateUser(user)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to activate user %d: %v", userID, err)
		return fmt.Errorf("用户激活失败")
	}

	logger.Infof(nil, "[AuthService] User activated successfully: %d", userID)
	return nil
}

// LoginUser 用户登录
func (s *AuthService) LoginUser(username, password string, remember bool) (*model.User, string, string, error) {
	// 获取用户信息
	user, err := s.userRepo.GetUserByUsername(username)
	if err != nil {
		logger.Warnf(nil, "[AuthService] User not found: %s, error: %v", username, err)
		return nil, "", "", fmt.Errorf("用户名或密码错误")
	}

	// 检查用户状态
	if !user.IsActive() {
		logger.Warnf(nil, "[AuthService] User not active: %s, status: %s", username, user.Status)
		return nil, "", "", fmt.Errorf("账户未激活，请先验证邮箱")
	}

	// 检查是否支持密码登录
	if !user.IsPasswordLoginSupported() {
		logger.Warnf(nil, "[AuthService] User does not support password login: %s, register_type: %s, has_password: %v", username, user.RegisterType, user.PasswordHash != "")
		return nil, "", "", fmt.Errorf("该账户不支持密码登录")
	}

	// 验证密码
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		logger.Warnf(nil, "[AuthService] Password mismatch for user: %s, error: %v", username, err)
		return nil, "", "", fmt.Errorf("用户名或密码错误")
	}

	token, refreshToken, err := s.IssueTokensForUser(user, remember)
	if err != nil {
		return nil, "", "", err
	}

	logger.Infof(nil, "[AuthService] User logged in successfully: %s (remember: %v)", username, remember)
	return user, token, refreshToken, nil
}

func (s *AuthService) IssueTokensForUser(user *model.User, remember bool) (string, string, error) {
	if user == nil {
		return "", "", fmt.Errorf("用户不存在")
	}
	if !user.IsActive() {
		logger.Warnf(nil, "[AuthService] User not active: %s, status: %s", user.Username, user.Status)
		return "", "", fmt.Errorf("账户未激活，请联系管理员")
	}

	jwtConfig := s.config.GetJWT()
	refreshTokenExpire := resolveRefreshTokenExpireSeconds(jwtConfig.RefreshTokenExpire, remember)

	tokenContext := s.buildUserTokenContext(user)
	refreshExpiresAt := time.Now().Add(time.Duration(refreshTokenExpire) * time.Second)

	// 生成 JWT Token（包含组织架构信息）
	token, err := s.jwtService.GenerateAccessTokenWithContext(tokenContext)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to generate access token: %v", err)
		return "", "", fmt.Errorf("访问令牌生成失败")
	}

	refreshToken, err := s.jwtService.GenerateRefreshTokenWithContextExpiresAt(tokenContext, refreshExpiresAt)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to generate refresh token: %v", err)
		return "", "", fmt.Errorf("刷新令牌生成失败")
	}

	// 保存用户会话（使用自定义的Refresh Token有效期）
	err = s.saveUserSessionWithExpiresAt(user.ID, token, refreshToken, refreshExpiresAt)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to save user session: %v", err)
		return "", "", fmt.Errorf("保存登录会话失败")
	}

	return token, refreshToken, nil
}

func resolveRefreshTokenExpireSeconds(configured int, remember bool) int {
	if configured <= 0 {
		configured = defaultRefreshTokenExpireSeconds
	}
	if remember && configured < rememberRefreshTokenExpireSeconds {
		return rememberRefreshTokenExpireSeconds
	}
	return configured
}

func (s *AuthService) buildUserTokenContext(user *model.User) auth.UserTokenContext {
	return auth.UserTokenContext{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              user.Email,
		DepartmentFullPath: user.DepartmentFullPath,
		LeaderUsername:     user.LeaderUsername,
	}
}

// RefreshToken 刷新Token
func (s *AuthService) RefreshToken(refreshToken string) (string, string, error) {
	// 验证 RefreshToken
	session, err := s.userSessionRepo.GetUserSessionByRefreshToken(refreshToken)
	if err != nil {
		return "", "", fmt.Errorf("RefreshToken无效或已过期")
	}
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return "", "", fmt.Errorf("RefreshToken无效或已过期")
	}
	if claims.UserID != session.UserID {
		return "", "", fmt.Errorf("RefreshToken无效")
	}

	// 获取用户信息
	user, err := s.userRepo.GetUserByID(session.UserID)
	if err != nil {
		return "", "", fmt.Errorf("用户不存在")
	}
	if !user.IsActive() {
		return "", "", fmt.Errorf("账户未激活，请重新登录")
	}

	tokenContext := s.buildUserTokenContext(user)
	refreshExpiresAt := time.Time(session.ExpiresAt)
	newAccessToken, err := s.jwtService.GenerateAccessTokenWithContext(tokenContext)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to refresh access token: %v", err)
		return "", "", fmt.Errorf("Token刷新失败")
	}
	newRefreshToken, err := s.jwtService.GenerateRefreshTokenWithContextExpiresAt(tokenContext, refreshExpiresAt)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to refresh token: %v", err)
		return "", "", fmt.Errorf("Token刷新失败")
	}

	// 更新会话中的Token和RefreshToken
	err = s.userSessionRepo.UpdateUserSessionTokens(session.ID, newAccessToken, newRefreshToken)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to update tokens: %v", err)
		return "", "", fmt.Errorf("Token刷新失败")
	}

	logger.Infof(nil, "[AuthService] Tokens refreshed successfully for user: %s", user.Username)
	return newAccessToken, newRefreshToken, nil
}

// LogoutUser 用户登出
func (s *AuthService) LogoutUser(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("未提供认证令牌")
	}

	claims, err := s.jwtService.ValidateAccessToken(token)
	if err != nil {
		return fmt.Errorf("认证令牌无效或已过期")
	}

	// user_session 是吊销状态的持久化真值；必须先落库，再通知网关清缓存。
	if err := s.userSessionRepo.DeactivateUserSession(token); err != nil {
		logger.Errorf(nil, "[AuthService] Failed to deactivate user session: %v", err)
		return fmt.Errorf("登出失败")
	}
	if s.tokenPublisher != nil {
		if err := s.tokenPublisher.InvalidateToken(nil, claims.UserID, claims.Username, token, "logout"); err != nil {
			logger.Warnf(nil, "[AuthService] 会话已停用，但发送网关失效通知失败: %v", err)
		}
	}

	logger.Infof(nil, "[AuthService] User logged out successfully")
	return nil
}

// ValidateAccessToken 同时校验 JWT 用途、持久化会话状态和当前用户状态。
// 网关只缓存此方法的结果，不能以本地 JWT 验签代替这里的授权判断。
func (s *AuthService) ValidateAccessToken(rawToken string) (*auth.AccessTokenPrincipal, error) {
	rawToken = strings.TrimSpace(rawToken)
	claims, err := s.jwtService.ValidateAccessToken(rawToken)
	if err != nil {
		return nil, err
	}
	session, err := s.userSessionRepo.GetUserSessionByToken(rawToken)
	if err != nil {
		return nil, fmt.Errorf("访问会话不存在、已过期或已吊销")
	}
	if session.UserID != claims.UserID {
		return nil, fmt.Errorf("访问会话与令牌用户不匹配")
	}
	user, err := s.userRepo.GetUserByID(session.UserID)
	if err != nil || !user.IsActive() {
		return nil, fmt.Errorf("用户不存在或已停用")
	}
	if user.Username != strings.TrimSpace(claims.Username) {
		return nil, fmt.Errorf("访问令牌用户名已失效")
	}
	current := s.buildUserTokenContext(user)
	return &auth.AccessTokenPrincipal{
		UserID:             current.UserID,
		Username:           current.Username,
		Email:              current.Email,
		DepartmentFullPath: current.DepartmentFullPath,
	}, nil
}

// saveUserSession 保存用户会话
func (s *AuthService) saveUserSession(userID int64, token, refreshToken string) error {
	// 计算过期时间（24小时）
	expiresAt := models.Time(time.Now().Add(24 * time.Hour))

	// 创建用户会话
	err := s.userSessionRepo.CreateActiveUserSession(userID, token, refreshToken, expiresAt)
	if err != nil {
		return fmt.Errorf("会话保存失败: %w", err)
	}

	return nil
}

// saveUserSessionWithExpire 保存用户会话（自定义过期时间）
func (s *AuthService) saveUserSessionWithExpire(userID int64, token, refreshToken string, expireSeconds int) error {
	// 计算过期时间
	expiresAt := time.Now().Add(time.Duration(expireSeconds) * time.Second)

	return s.saveUserSessionWithExpiresAt(userID, token, refreshToken, expiresAt)
}

// saveUserSessionWithExpiresAt 保存用户会话（指定绝对过期时间）
func (s *AuthService) saveUserSessionWithExpiresAt(userID int64, token, refreshToken string, expiresAt time.Time) error {
	modelExpiresAt := models.Time(expiresAt)

	// 创建用户会话
	err := s.userSessionRepo.CreateActiveUserSession(userID, token, refreshToken, modelExpiresAt)
	if err != nil {
		return fmt.Errorf("会话保存失败: %w", err)
	}

	return nil
}

// GetUserByEmail 根据邮箱获取用户
func (s *AuthService) GetUserByEmail(email string) (*model.User, error) {
	user, err := s.userRepo.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// ResetPasswordByEmail 在邮箱验证码已被消费后重置密码。
func (s *AuthService) ResetPasswordByEmail(email, newPassword string) error {
	// 获取用户信息
	user, err := s.userRepo.GetUserByEmail(email)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to get user by email %s: %v", email, err)
		return fmt.Errorf("用户不存在")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to hash password: %v", err)
		return fmt.Errorf("密码加密失败")
	}

	// 更新用户密码
	user.PasswordHash = string(hashedPassword)
	err = s.userRepo.UpdateUser(user)
	if err != nil {
		logger.Errorf(nil, "[AuthService] Failed to update user password: %v", err)
		return fmt.Errorf("密码更新失败")
	}
	if err := s.invalidateAllUserSessions(user, "password_reset"); err != nil {
		return err
	}

	logger.Infof(nil, "[AuthService] Password reset successfully for user: %s", user.Username)
	return nil
}

func (s *AuthService) invalidateAllUserSessions(user *model.User, reason string) error {
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
		if err := s.tokenPublisher.InvalidateUserTokens(nil, user.ID, user.Username, sessions, reason); err != nil {
			logger.Warnf(nil, "[AuthService] 会话已停用，但发送网关失效通知失败: %v", err)
		}
	}
	return nil
}
