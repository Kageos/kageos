package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kageos/kageos/core/hr-server/service"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/ginx/response"
)

type SystemSettings struct {
	settingsService *service.SystemSettingsService
	resourceService *service.SystemResourceService
}

func NewSystemSettings(settingsService *service.SystemSettingsService, resourceService *service.SystemResourceService) *SystemSettings {
	return &SystemSettings{settingsService: settingsService, resourceService: resourceService}
}

func (s *SystemSettings) GetResources(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	hours := 24 * 7
	if raw := c.Query("hours"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			hours = parsed
		}
	}
	includeHistory := c.DefaultQuery("include_history", "true") != "false"
	overview, err := s.resourceService.Overview(hours, includeHistory)
	if err != nil {
		response.FailWithMessage(c, "获取系统资源状态失败: "+err.Error())
		return
	}
	response.OkWithData(c, overview)
}

func (s *SystemSettings) GetResourceSummary(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	result, err := s.resourceService.Summary()
	if err != nil {
		response.FailWithMessage(c, "获取系统资源概览失败: "+err.Error())
		return
	}
	response.OkWithData(c, result)
}

func (s *SystemSettings) GetResourceTrends(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	hours := queryPositiveInt(c, "hours", 24*7)
	result, err := s.resourceService.Trends(hours)
	if err != nil {
		response.FailWithMessage(c, "获取系统资源趋势失败: "+err.Error())
		return
	}
	response.OkWithData(c, result)
}

func (s *SystemSettings) GetResourceStorage(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	result, err := s.resourceService.Storage()
	if err != nil {
		response.FailWithMessage(c, "获取系统存储状态失败: "+err.Error())
		return
	}
	response.OkWithData(c, result)
}

func (s *SystemSettings) GetResourceDatabases(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	result, err := s.resourceService.Databases(
		queryPositiveInt(c, "page", 1),
		queryPositiveInt(c, "page_size", 20),
		c.Query("scope"),
		c.Query("keyword"),
		c.DefaultQuery("include_history", "true") != "false",
		service.DatabaseHistoryOptions{Days: queryPositiveInt(c, "days", 7), Name: c.Query("database")},
	)
	if err != nil {
		response.FailWithMessage(c, "获取数据库资产失败: "+err.Error())
		return
	}
	response.OkWithData(c, result)
}

func (s *SystemSettings) GetResourceDiagnostics(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	result, err := s.resourceService.Diagnostics()
	if err != nil {
		response.FailWithMessage(c, "获取资源采集诊断失败: "+err.Error())
		return
	}
	response.OkWithData(c, result)
}

func (s *SystemSettings) GetResourceUsage(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	result, err := s.resourceService.Usage(
		queryPositiveInt(c, "days", 7),
		queryPositiveInt(c, "page", 1),
		queryPositiveInt(c, "page_size", 10),
	)
	if err != nil {
		response.FailWithMessage(c, "获取调用量概览失败: "+err.Error())
		return
	}
	response.OkWithData(c, result)
}

func queryPositiveInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func (s *SystemSettings) Get(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	settings, err := s.settingsService.GetSettings()
	if err != nil {
		response.FailWithMessage(c, "获取系统设置失败: "+err.Error())
		return
	}
	response.OkWithData(c, settings)
}

func (s *SystemSettings) Update(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.UpdateSystemSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	settings, err := s.settingsService.UpdateSettings(req, contextx.GetRequestUser(c))
	if err != nil {
		response.FailWithMessage(c, "保存系统设置失败: "+err.Error())
		return
	}
	response.OkWithData(c, settings)
}

func (s *SystemSettings) TestEmail(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.TestEmailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	if err := s.settingsService.TestEmail(req.To); err != nil {
		response.FailWithMessage(c, "测试邮件发送失败: "+err.Error())
		return
	}
	response.OkWithMessage(c, "测试邮件已发送")
}

func (s *SystemSettings) GetBackup(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	overview, err := s.settingsService.GetBackupOverview()
	if err != nil {
		response.FailWithMessage(c, "获取数据备份配置失败: "+err.Error())
		return
	}
	response.OkWithData(c, overview)
}

func (s *SystemSettings) UpdateBackup(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemBackupConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	overview, err := s.settingsService.UpdateBackupConfig(req)
	if err != nil {
		response.FailWithMessage(c, "保存数据备份配置失败: "+err.Error())
		return
	}
	response.OkWithData(c, overview)
}

func (s *SystemSettings) TestBackup(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.SystemBackupConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	if err := s.settingsService.TestBackupS3(c.Request.Context(), req); err != nil {
		response.FailWithMessage(c, "S3 连接测试失败: "+err.Error())
		return
	}
	response.OkWithMessage(c, "S3 连接正常")
}

func (s *SystemSettings) RunBackupNow(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	overview, err := s.settingsService.RequestBackupRunNow()
	if err != nil {
		response.FailWithMessage(c, "创建备份请求失败: "+err.Error())
		return
	}
	response.OkWithData(c, overview)
}

func (s *SystemSettings) GetTLS(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	settings, err := s.settingsService.GetTLSSettings()
	if err != nil {
		response.FailWithMessage(c, "获取 HTTPS 证书状态失败: "+err.Error())
		return
	}
	response.OkWithData(c, settings)
}

func (s *SystemSettings) UpdateTLS(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	var req dto.UpdateTLSCertificateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, "请求参数错误: "+err.Error())
		return
	}
	settings, err := s.settingsService.UpdateTLSCertificate(req, contextx.GetRequestUser(c))
	if err != nil {
		response.FailWithMessage(c, "保存 HTTPS 证书失败: "+err.Error())
		return
	}
	response.OkWithData(c, settings)
}

func (s *SystemSettings) ReloadTLS(c *gin.Context) {
	if !requireSystemUser(c) {
		return
	}
	if err := s.settingsService.ReloadTLS(); err != nil {
		response.FailWithMessage(c, "热加载 HTTPS 证书失败: "+err.Error())
		return
	}
	settings, err := s.settingsService.GetTLSSettings()
	if err != nil {
		response.FailWithMessage(c, "读取 HTTPS 证书状态失败: "+err.Error())
		return
	}
	response.OkWithData(c, settings)
}

func requireSystemUser(c *gin.Context) bool {
	if contextx.GetRequestUser(c) != "system" {
		response.FailWithMessage(c, "仅 system 超管可操作")
		c.Abort()
		return false
	}
	return true
}
