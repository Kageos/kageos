package v1

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/core/hr-server/service"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/ginx/response"
	"github.com/kageos/kageos/pkg/logger"
	"github.com/kageos/kageos/pkg/openapitoken"
)

// User 用户相关API
type User struct {
	userService       *service.UserService
	departmentService *service.DepartmentService
	openAPITokenStore *openapitoken.Store
	tokenPublisher    service.TokenPublisher
}

// NewUser 创建用户API（依赖注入）
func NewUser(userService *service.UserService, departmentService *service.DepartmentService, openAPITokenStore *openapitoken.Store, tokenPublisher service.TokenPublisher) *User {
	return &User{
		userService:       userService,
		departmentService: departmentService,
		openAPITokenStore: openAPITokenStore,
		tokenPublisher:    tokenPublisher,
	}
}

// GetUserInfo 获取当前登录用户信息
// @Summary 获取当前登录用户信息
// @Description 根据请求header中的username获取当前登录用户信息
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param X-Token header string true "JWT Token"
// @Success 200 {object} dto.UserInfo "用户信息"
// @Failure 400 {string} string "请求参数错误"
// @Failure 401 {string} string "未认证"
// @Failure 404 {string} string "用户不存在"
// @Failure 500 {string} string "服务器内部错误"
// @Router /hr/api/v1/user/info [get]
func (u *User) GetUserInfo(c *gin.Context) {
	var resp *dto.UserInfo
	var err error
	defer func() {
		logger.Debugf(c, "GetUserInfo resp:%+v err:%v", resp, err)
	}()

	// 从context获取username（JWTAuth中间件已从header获取并设置到context）
	username := contextx.GetRequestUser(c)
	if username == "" {
		response.FailWithMessage(c, "未提供用户信息")
		return
	}

	// 查询用户信息
	user, err := u.userService.GetUserByUsername(username)
	if err != nil {
		response.FailWithMessage(c, "用户不存在: "+err.Error())
		return
	}
	// 转换为DTO（包含详细信息）
	ctx := contextx.ToContext(c)
	userInfos := convertUsersToDTOBatch(ctx, []*model.User{user}, u.userService, u.departmentService)
	if len(userInfos) == 0 {
		response.FailWithMessage(c, "转换用户信息失败")
		return
	}
	resp = userInfos[0]
	response.OkWithData(c, resp)
}

// QueryUser 根据用户名精确查询用户
// @Summary 根据用户名精确查询用户
// @Description 根据用户名精确查询用户信息
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param X-Token header string true "JWT Token"
// @Param username query string true "用户名"
// @Success 200 {object} dto.QueryUserResp "用户信息"
// @Failure 400 {string} string "请求参数错误"
// @Failure 401 {string} string "未认证"
// @Failure 404 {string} string "用户不存在"
// @Failure 500 {string} string "服务器内部错误"
// @Router /hr/api/v1/user/query [get]
func (u *User) QueryUser(c *gin.Context) {
	var req dto.QueryUserReq
	var resp *dto.QueryUserResp
	var err error
	defer func() {
		logger.Debugf(c, "QueryUser req:%+v resp:%+v err:%v", req, resp, err)
	}()

	// 绑定请求参数
	if err = c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}

	// 提取括号前的用户名部分（支持 "sina(新那)" 格式）
	username := extractUsernameFromDisplayName(req.Username)

	// 查询用户信息
	user, err := u.userService.GetUserByUsername(username)
	if err != nil {
		response.FailWithMessage(c, "用户不存在: "+err.Error())
		return
	}
	// 转换为DTO（包含详细信息）
	ctx := contextx.ToContext(c)
	userInfos := convertUsersToDTOBatch(ctx, []*model.User{user}, u.userService, u.departmentService)
	if len(userInfos) == 0 {
		response.FailWithMessage(c, "转换用户信息失败")
		return
	}
	resp = &dto.QueryUserResp{
		User: *userInfos[0],
	}
	response.OkWithData(c, resp)
}

// SearchUsersFuzzy 模糊查询用户
// @Summary 模糊查询用户
// @Description 根据关键词模糊查询用户（支持用户名、邮箱和昵称）
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param X-Token header string true "JWT Token"
// @Param keyword query string true "搜索关键词"
// @Param limit query int false "返回数量限制，默认10，最大100"
// @Success 200 {object} dto.SearchUsersFuzzyResp "用户列表"
// @Failure 400 {string} string "请求参数错误"
// @Failure 401 {string} string "未认证"
// @Failure 500 {string} string "服务器内部错误"
// @Router /hr/api/v1/user/search_fuzzy [get]
func (u *User) SearchUsersFuzzy(c *gin.Context) {
	var req dto.SearchUsersFuzzyReq
	var resp *dto.SearchUsersFuzzyResp
	var err error
	defer func() {
		logger.Debugf(c, "SearchUsersFuzzy req:%+v resp:%+v err:%v", req, resp, err)
	}()

	// 绑定请求参数
	if err = c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}

	// 设置默认limit
	if req.Limit <= 0 {
		req.Limit = 10
	}

	// 提取括号前的关键词部分（支持 "sina(新那)" 格式）
	keyword := extractUsernameFromDisplayName(req.Keyword)

	// 单实例内所有账号属于同一个组织空间。
	users, err := u.userService.SearchUsersFuzzy(keyword, req.Limit)
	if err != nil {
		response.FailWithMessage(c, "查询失败: "+err.Error())
		return
	}

	// 转换为DTO（包含详细信息，批量查询）
	ctx := contextx.ToContext(c)
	dtoUserInfos := convertUsersToDTOBatch(ctx, users, u.userService, u.departmentService)
	userInfos := make([]dto.UserInfo, 0, len(dtoUserInfos))
	for _, userInfo := range dtoUserInfos {
		userInfos = append(userInfos, *userInfo)
	}

	resp = &dto.SearchUsersFuzzyResp{
		Users: userInfos,
	}
	response.OkWithData(c, resp)
}

// GetUsersByUsernames 批量获取用户信息
// @Summary 批量获取用户信息
// @Description 根据用户名列表批量获取用户信息
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param X-Token header string true "JWT Token"
// @Param request body dto.GetUsersByUsernamesReq true "批量查询请求"
// @Success 200 {object} dto.GetUsersByUsernamesResp "用户列表"
// @Failure 400 {string} string "请求参数错误"
// @Failure 401 {string} string "未认证"
// @Failure 500 {string} string "服务器内部错误"
// @Router /hr/api/v1/users [post]
func (u *User) GetUsersByUsernames(c *gin.Context) {
	var req dto.GetUsersByUsernamesReq
	var resp *dto.GetUsersByUsernamesResp
	var err error
	defer func() {
		logger.Debugf(c, "GetUsersByUsernames req:%+v resp:%+v err:%v", req, resp, err)
	}()

	// 绑定请求参数
	if err = c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}

	// 单实例内所有账号属于同一个组织空间。
	users, err := u.userService.GetUsersByUsernames(req.Usernames)
	if err != nil {
		response.FailWithMessage(c, "查询失败: "+err.Error())
		return
	}

	// 转换为DTO（包含详细信息，批量查询）
	ctx := contextx.ToContext(c)
	dtoUserInfos := convertUsersToDTOBatch(ctx, users, u.userService, u.departmentService)
	userInfos := make([]dto.UserInfo, 0, len(dtoUserInfos))
	for _, userInfo := range dtoUserInfos {
		userInfos = append(userInfos, *userInfo)
	}

	resp = &dto.GetUsersByUsernamesResp{
		Users: userInfos,
	}
	response.OkWithData(c, resp)
}

// UpdateUser 更新用户信息
// @Summary 更新用户信息
// @Description 更新当前登录用户的昵称、签名、头像、性别等信息
// @Tags 用户管理
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param X-Token header string true "JWT Token"
// @Param request body dto.UpdateUserReq true "更新用户信息请求"
// @Success 200 {object} dto.UpdateUserResp "更新成功"
// @Failure 400 {string} string "请求参数错误"
// @Failure 401 {string} string "未认证"
// @Failure 404 {string} string "用户不存在"
// @Failure 500 {string} string "服务器内部错误"
// @Router /hr/api/v1/user/update [put]
func (u *User) UpdateUser(c *gin.Context) {
	var req dto.UpdateUserReq
	var resp *dto.UpdateUserResp
	var err error
	defer func() {
		logger.Debugf(c, "UpdateUser req:%+v resp:%+v err:%v", req, resp, err)
	}()

	// 绑定请求参数
	if err = c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}

	// 从context获取username
	username := contextx.GetRequestUser(c)
	if username == "" {
		response.FailWithMessage(c, "未提供用户信息")
		return
	}

	// 检查是否有字段需要更新
	if req.Nickname == nil && req.Signature == nil && req.Avatar == nil && req.Gender == nil {
		response.FailWithMessage(c, "至少需要提供一个更新字段")
		return
	}

	// 更新用户信息（直接传递指针，nil 表示不更新，非 nil 表示更新）
	user, err := u.userService.UpdateUser(username, req.Nickname, req.Signature, req.Avatar, req.Gender)
	if err != nil {
		response.FailWithMessage(c, "更新失败: "+err.Error())
		return
	}

	// 转换为DTO（包含详细信息）
	ctx := contextx.ToContext(c)
	userInfos := convertUsersToDTOBatch(ctx, []*model.User{user}, u.userService, u.departmentService)
	if len(userInfos) == 0 {
		response.FailWithMessage(c, "转换用户信息失败")
		return
	}
	resp = &dto.UpdateUserResp{
		User: *userInfos[0],
	}
	response.OkWithData(c, resp)
}

// ChangePassword 修改当前登录用户的密码
// @Summary 修改当前登录用户密码
// @Description 校验当前密码后设置新密码，成功后全部登录会话失效
// @Tags 用户管理
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param X-Token header string true "JWT Token"
// @Param request body dto.ChangeOwnPasswordReq true "修改密码请求"
// @Success 200 {object} response.Response "修改成功"
// @Failure 400 {object} response.Response "请求错误"
// @Failure 401 {object} response.Response "未认证"
// @Router /hr/api/v1/user/password [put]
func (u *User) ChangePassword(c *gin.Context) {
	var req dto.ChangeOwnPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}

	username := contextx.GetRequestUser(c)
	if username == "" {
		response.FailWithMessage(c, "未提供用户信息")
		return
	}
	if err := u.userService.ChangeOwnPassword(contextx.ToContext(c), username, req.CurrentPassword, req.NewPassword); err != nil {
		response.FailWithMessage(c, "修改密码失败: "+err.Error())
		return
	}
	response.OkWithMessage(c, "密码修改成功，请重新登录")
}

func (u *User) ListOpenAPITokens(c *gin.Context) {
	username := contextx.GetRequestUser(c)
	if username == "" {
		response.FailWithMessage(c, "未提供用户信息")
		return
	}
	tokens, err := u.openAPITokenStore.List(username)
	if err != nil {
		response.FailWithMessage(c, "查询 OpenAPI Token 失败: "+err.Error())
		return
	}
	items := make([]dto.OpenAPITokenInfo, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, convertOpenAPIToken(token))
	}
	response.OkWithData(c, &dto.ListOpenAPITokensResp{Tokens: items})
}

func (u *User) CreateOpenAPIToken(c *gin.Context) {
	username := contextx.GetRequestUser(c)
	if username == "" {
		response.FailWithMessage(c, "未提供用户信息")
		return
	}
	var req dto.CreateOpenAPITokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
		if err != nil {
			response.FailWithMessage(c, "expires_at 必须是 RFC3339 时间格式")
			return
		}
		expiresAt = &parsed
	}
	currentUser, err := u.userService.GetUserByUsername(username)
	if err != nil {
		response.FailWithMessage(c, "当前用户不存在: "+err.Error())
		return
	}
	result, err := u.userService.CreateActiveUserOpenAPIToken(contextx.ToContext(c), u.openAPITokenStore, openapitoken.CreateInput{
		OwnerUsername:      username,
		OwnerUserID:        currentUser.ID,
		OwnerEmail:         currentUser.Email,
		DepartmentFullPath: currentUser.DepartmentFullPath,
		LeaderUsername:     currentUser.LeaderUsername,
		Name:               req.Name,
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		response.FailWithMessage(c, "创建 OpenAPI Token 失败: "+err.Error())
		return
	}
	response.OkWithData(c, &dto.CreateOpenAPITokenResp{
		Token:       convertOpenAPIToken(result.Token),
		SecretToken: result.Secret,
	})
}

func (u *User) RevokeOpenAPIToken(c *gin.Context) {
	username := contextx.GetRequestUser(c)
	if username == "" {
		response.FailWithMessage(c, "未提供用户信息")
		return
	}
	var req struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	revoked, err := u.openAPITokenStore.RevokeWithResult(username, req.ID)
	if err != nil {
		response.FailWithMessage(c, "吊销 OpenAPI Token 失败: "+err.Error())
		return
	}
	if u.tokenPublisher != nil {
		if err := u.tokenPublisher.InvalidateOpenAPIToken(c, revoked.UserID, revoked.Username, revoked.TokenHash, revoked.ExpiresAt); err != nil {
			response.FailWithMessage(c, "Token 已吊销，但同步网关缓存失败: "+err.Error())
			return
		}
	}
	response.Ok(c)
}

func convertOpenAPIToken(token openapitoken.OpenAPIToken) dto.OpenAPITokenInfo {
	return dto.OpenAPITokenInfo{
		ID:                token.ID,
		Name:              token.Name,
		TokenPrefix:       token.TokenPrefix,
		ExpiresAt:         formatOptionalTime(token.ExpiresAt),
		RevokedAt:         formatOptionalTime(token.RevokedAt),
		LastUsedAt:        formatOptionalTime(token.LastUsedAt),
		LastUsedIP:        token.LastUsedIP,
		LastUsedUserAgent: token.LastUsedUserAgent,
		CreatedAt:         time.Time(token.CreatedAt).Format(time.RFC3339),
	}
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}

// extractUsernameFromDisplayName 从显示名称中提取用户名
// 原因：前端可能传递的是显示名称格式（如 "sina(新那)"），其中括号前是用户名，括号内是昵称
// 为了支持用户通过前端展示的名称进行查询，需要提取括号前的用户名部分进行实际查询
// 示例：
//   - "sina(新那)" -> "sina"
//   - "sina" -> "sina"（没有括号时返回原字符串）
func extractUsernameFromDisplayName(displayName string) string {
	return strings.TrimSpace(strings.Split(displayName, "(")[0])
}

// convertUserToDTO 将model.User转换为dto.UserInfo（基础版本，不包含详细信息）
func convertUserToDTO(user *model.User) *dto.UserInfo {
	return convertUserToDTOWithDetails(user, nil, nil)
}

// convertUserToDTOWithDetails 将model.User转换为dto.UserInfo（包含详细信息：部门名称和Leader显示名称）
// deptMap: 部门信息映射表，key 为 fullCodePath，value 为 Department（可选）
// leaderMap: Leader 信息映射表，key 为 username，value 为 User（可选）
func convertUserToDTOWithDetails(user *model.User, deptMap map[string]*model.Department, leaderMap map[string]*model.User) *dto.UserInfo {
	userInfo := &dto.UserInfo{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		RegisterType:  user.RegisterType,
		Avatar:        user.Avatar,
		Nickname:      user.Nickname,
		Signature:     user.Signature,
		Gender:        user.Gender,
		EmailVerified: user.EmailVerified,
		Status:        user.Status,
		CreatedAt:     time.Time(user.CreatedAt).Format(time.RFC3339),
	}

	// 组织架构信息（如果存在）
	if user.DepartmentFullPath != "" {
		userInfo.DepartmentFullPath = user.DepartmentFullPath

		// 从缓存中获取部门名称和完整名称路径
		if deptMap != nil {
			if dept, ok := deptMap[user.DepartmentFullPath]; ok {
				userInfo.DepartmentName = dept.Name
				// 如果 FullNamePath 为空，使用 Name 作为后备
				if dept.FullNamePath != "" {
					userInfo.DepartmentFullNamePath = dept.FullNamePath
				} else {
					// 如果 FullNamePath 为空（可能是旧数据），使用 Name 作为后备
					userInfo.DepartmentFullNamePath = dept.Name
				}
			}
		}
	}
	if user.LeaderUsername != "" {
		userInfo.LeaderUsername = user.LeaderUsername

		// 从缓存中获取 Leader 显示名称
		if leaderMap != nil {
			if leader, ok := leaderMap[user.LeaderUsername]; ok {
				// 构建显示名称：username(nickname) 或 username
				if leader.Nickname != "" {
					userInfo.LeaderDisplayName = fmt.Sprintf("%s(%s)", leader.Username, leader.Nickname)
				} else {
					userInfo.LeaderDisplayName = leader.Username
				}
			}
		}
	}

	return userInfo
}

// convertUsersToDTOBatch 批量转换用户列表为 DTO（包含详细信息）
func convertUsersToDTOBatch(ctx context.Context, users []*model.User, userService *service.UserService, departmentService *service.DepartmentService) []*dto.UserInfo {
	if len(users) == 0 {
		return []*dto.UserInfo{}
	}

	// 收集所有需要查询的部门路径和 Leader 用户名
	deptPaths := make([]string, 0)
	leaderUsernames := make([]string, 0)
	for _, user := range users {
		if user.DepartmentFullPath != "" {
			deptPaths = append(deptPaths, user.DepartmentFullPath)
		}
		if user.LeaderUsername != "" {
			leaderUsernames = append(leaderUsernames, user.LeaderUsername)
		}
	}

	// 批量查询部门信息（从缓存）
	var deptMap map[string]*model.Department
	if len(deptPaths) > 0 && departmentService != nil {
		var err error
		deptMap, err = departmentService.GetDepartmentsByFullCodePaths(ctx, deptPaths)
		if err != nil {
			logger.Warnf(ctx, "[convertUsersToDTOBatch] 批量查询部门信息失败: %v", err)
			deptMap = nil
		}
	}

	// 批量查询 Leader 信息
	var leaderMap map[string]*model.User
	if len(leaderUsernames) > 0 && userService != nil {
		// 去重
		uniqueUsernames := make(map[string]bool)
		for _, username := range leaderUsernames {
			uniqueUsernames[username] = true
		}
		usernameList := make([]string, 0, len(uniqueUsernames))
		for username := range uniqueUsernames {
			usernameList = append(usernameList, username)
		}

		leaders, err := userService.GetUsersByUsernames(usernameList)
		if err != nil {
			logger.Warnf(ctx, "[convertUsersToDTOBatch] 批量查询 Leader 信息失败: %v", err)
			leaderMap = nil
		} else {
			leaderMap = make(map[string]*model.User, len(leaders))
			for _, leader := range leaders {
				leaderMap[leader.Username] = leader
			}
		}
	}

	// 批量转换
	userInfos := make([]*dto.UserInfo, 0, len(users))
	for _, user := range users {
		userInfo := convertUserToDTOWithDetails(user, deptMap, leaderMap)
		userInfos = append(userInfos, userInfo)
	}

	return userInfos
}
