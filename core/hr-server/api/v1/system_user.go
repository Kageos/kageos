package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/core/hr-server/service"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/ginx/response"
)

type SystemUser struct {
	userService       *service.UserService
	departmentService *service.DepartmentService
}

func NewSystemUser(userService *service.UserService, departmentService *service.DepartmentService) *SystemUser {
	return &SystemUser{
		userService:       userService,
		departmentService: departmentService,
	}
}

func (s *SystemUser) List(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemListUsersReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	users, total, err := s.userService.ListUsersForSystem(req)
	if err != nil {
		response.FailWithMessage(c, "查询用户失败: "+err.Error())
		return
	}
	response.OkWithData(c, &dto.SystemListUsersResp{
		Users:    systemUserDTOs(contextx.ToContext(c), users, s.userService, s.departmentService),
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}

func (s *SystemUser) Create(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemCreateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	user, err := s.userService.CreateUserFromSystem(contextx.ToContext(c), req, contextx.GetRequestUser(c))
	if err != nil {
		response.FailWithMessage(c, "创建用户失败: "+err.Error())
		return
	}
	s.respondUser(c, user)
}

func (s *SystemUser) Update(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemUpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	user, err := s.userService.UpdateUserFromSystem(contextx.ToContext(c), usernameParam(c), req)
	if err != nil {
		response.FailWithMessage(c, "更新用户失败: "+err.Error())
		return
	}
	s.respondUser(c, user)
}

func (s *SystemUser) ResetPassword(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemResetUserPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	user, err := s.userService.ResetUserPasswordFromSystem(contextx.ToContext(c), usernameParam(c), req.Password)
	if err != nil {
		response.FailWithMessage(c, "重置密码失败: "+err.Error())
		return
	}
	s.respondUser(c, user)
}

func (s *SystemUser) UpdateStatus(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemUpdateUserStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	user, err := s.userService.UpdateUserStatusFromSystem(contextx.ToContext(c), usernameParam(c), req.Status, contextx.GetRequestUser(c), req.Reason)
	if err != nil {
		response.FailWithMessage(c, "更新用户状态失败: "+err.Error())
		return
	}
	s.respondUser(c, user)
}

func (s *SystemUser) respondUser(c *gin.Context, user *model.User) {
	userInfos := convertUsersToDTOBatch(contextx.ToContext(c), []*model.User{user}, s.userService, s.departmentService)
	if len(userInfos) == 0 {
		response.FailWithMessage(c, "转换用户信息失败")
		return
	}
	response.OkWithData(c, &dto.SystemUserResp{User: *userInfos[0]})
}

func systemUserDTOs(ctx context.Context, users []*model.User, userService *service.UserService, departmentService *service.DepartmentService) []dto.UserInfo {
	dtoUsers := convertUsersToDTOBatch(ctx, users, userService, departmentService)
	result := make([]dto.UserInfo, 0, len(dtoUsers))
	for _, userInfo := range dtoUsers {
		result = append(result, *userInfo)
	}
	return result
}

func usernameParam(c *gin.Context) string {
	return strings.TrimSpace(c.Param("username"))
}

func (s *SystemUser) Import(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	var req dto.SystemImportUsersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "导入格式错误，最多 100 行、1 MB")
		return
	}
	rows, err := s.userService.ImportUsers(contextx.ToContext(c), req.Rows, req.Preview, contextx.GetRequestUser(c))
	if err != nil {
		response.FailWithMessage(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{"rows": rows})
}
