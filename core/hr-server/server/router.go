package server

import (
	v1 "github.com/kageos/kageos/core/hr-server/api/v1"
	middleware2 "github.com/kageos/kageos/pkg/middleware"
	"github.com/kageos/kageos/pkg/pprof"
	"github.com/kageos/kageos/pkg/serverx"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	jwtAuth := middleware2.JWTAuth(middleware2.WithOpenAPITokenStore(s.openAPITokenStore))

	// 健康检查
	s.httpServer.GET("/health", s.healthHandler)

	// 注册 pprof 路由（性能分析）
	if s.cfg.IsPprofEnabled() {
		pprof.RegisterPprofRoutes(s.httpServer)
	}

	// Swagger 文档路由
	s.httpServer.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// HR 路由组（统一使用 /hr/api/v1 开头，方便网关代理）
	hr := s.httpServer.Group("/hr")
	apiV1 := hr.Group("/api/v1")

	// 认证相关路由（不需要JWT验证）
	auth := apiV1.Group("/auth")
	authHandler := v1.NewAuth(s.authService, s.authOAuthService, s.authWechatService, s.emailService, s.settingsService, s.userService, s.departmentService)
	accessTokenHandler := v1.NewAccessToken(s.authService)
	openAPITokenHandler := v1.NewOpenAPIToken(s.openAPITokenStore, s.userService)
	auth.POST("/send_email_code", authHandler.SendEmailCode)
	auth.POST("/register", authHandler.Register)
	auth.GET("/oauth/registration/:ticket", authHandler.GetOAuthRegistrationIntent)
	auth.POST("/oauth/registration/:ticket/confirm", authHandler.ConfirmOAuthRegistration)
	auth.POST("/wechat/attempts", authHandler.CreateWechatLoginAttempt)
	auth.POST("/wechat/attempts/complete", authHandler.CompleteWechatLoginAttempt)
	auth.GET("/wechat/callback", authHandler.VerifyWechatCallback)
	auth.POST("/wechat/callback", authHandler.ReceiveWechatCallback)
	auth.GET("/:provider/authorize", authHandler.OAuthAuthorize)
	auth.GET("/:provider/callback", authHandler.OAuthCallback)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.RefreshToken)
	auth.POST("/logout", authHandler.Logout)
	auth.POST("/forgot_password", authHandler.ForgotPassword)
	auth.POST("/access_token/validate", accessTokenHandler.Validate)
	auth.POST("/openapi_token/validate", openAPITokenHandler.Validate)
	authProviderHandler := v1.NewAuthLoginProvider(s.authProviderService)
	auth.GET("/methods", authProviderHandler.PublicMethods)
	loginAnnouncementHandler := v1.NewLoginAnnouncement(s.settingsService)
	auth.GET("/login-announcement", loginAnnouncementHandler.PublicGet)

	// 用户管理路由（需要JWT验证）
	user := apiV1.Group("/user")
	user.Use(jwtAuth) // 用户管理需要JWT认证
	userHandler := v1.NewUser(s.userService, s.departmentService, s.openAPITokenStore, s.tokenPublisher)
	user.GET("/info", userHandler.GetUserInfo)
	user.GET("/query", userHandler.QueryUser)
	user.GET("/search_fuzzy", userHandler.SearchUsersFuzzy)
	user.PUT("/update", userHandler.UpdateUser)
	user.PUT("/password", userHandler.ChangePassword)
	user.GET("/openapi_tokens", userHandler.ListOpenAPITokens)
	user.POST("/openapi_tokens", userHandler.CreateOpenAPIToken)
	user.POST("/openapi_tokens/revoke", userHandler.RevokeOpenAPIToken)

	systemSettings := apiV1.Group("/system/settings")
	systemSettings.Use(jwtAuth)
	systemSettingsHandler := v1.NewSystemSettings(s.settingsService, s.resourceService)
	systemSettings.GET("", systemSettingsHandler.Get)
	systemSettings.PUT("", systemSettingsHandler.Update)
	systemSettings.POST("/email/test", systemSettingsHandler.TestEmail)
	systemSettings.GET("/backup", systemSettingsHandler.GetBackup)
	systemSettings.PUT("/backup", systemSettingsHandler.UpdateBackup)
	systemSettings.POST("/backup/test", systemSettingsHandler.TestBackup)
	systemSettings.POST("/backup/run", systemSettingsHandler.RunBackupNow)
	systemSettings.GET("/tls", systemSettingsHandler.GetTLS)
	systemSettings.PUT("/tls", systemSettingsHandler.UpdateTLS)
	systemSettings.POST("/tls/reload", systemSettingsHandler.ReloadTLS)
	systemSettings.GET("/resources", systemSettingsHandler.GetResources)
	systemSettings.GET("/resources/summary", systemSettingsHandler.GetResourceSummary)
	systemSettings.GET("/resources/trends", systemSettingsHandler.GetResourceTrends)
	systemSettings.GET("/resources/storage", systemSettingsHandler.GetResourceStorage)
	systemSettings.GET("/resources/databases", systemSettingsHandler.GetResourceDatabases)
	systemSettings.GET("/resources/diagnostics", systemSettingsHandler.GetResourceDiagnostics)
	systemSettings.GET("/resources/usage", systemSettingsHandler.GetResourceUsage)

	systemAuthProviders := apiV1.Group("/system/auth/providers")
	systemAuthProviders.Use(jwtAuth)
	systemAuthProviders.GET("", authProviderHandler.List)
	systemAuthProviders.PUT("/:code/config", authProviderHandler.UpdateConfig)
	systemAuthProviders.PUT("/:code/enabled", authProviderHandler.SetEnabled)

	systemAuth := apiV1.Group("/system/auth")
	systemAuth.Use(jwtAuth)
	systemAuth.GET("/login-announcement", loginAnnouncementHandler.Get)
	systemAuth.PUT("/login-announcement", loginAnnouncementHandler.Update)

	systemUsers := apiV1.Group("/system/users")
	systemUsers.Use(jwtAuth)
	systemUserHandler := v1.NewSystemUser(s.userService, s.departmentService)
	systemUsers.GET("", systemUserHandler.List)
	systemUsers.POST("", systemUserHandler.Create)
	systemUsers.POST("/import", systemUserHandler.Import)
	systemUsers.PUT("/:username", systemUserHandler.Update)
	systemUsers.POST("/:username/password", systemUserHandler.ResetPassword)
	systemUsers.PUT("/:username/status", systemUserHandler.UpdateStatus)

	// 批量获取用户（需要JWT验证）
	users := apiV1.Group("/users")
	users.Use(jwtAuth)
	users.POST("", userHandler.GetUsersByUsernames)

	// 部门管理路由（需要JWT验证）
	department := apiV1.Group("/department")
	department.Use(jwtAuth)
	departmentHandler := v1.NewDepartment(s.departmentService)
	department.POST("", departmentHandler.CreateDepartment)
	department.GET("/tree", departmentHandler.GetDepartmentTree)
	department.GET("/:id", departmentHandler.GetDepartmentByID)
	department.PUT("/:id", departmentHandler.UpdateDepartment)
	department.DELETE("/:id", departmentHandler.DeleteDepartment)

	// 批量获取部门路由（需要JWT验证）
	departments := apiV1.Group("/departments")
	departments.Use(jwtAuth)
	departments.GET("", departmentHandler.GetDepartmentsByPaths)

	// 用户分配路由（需要JWT验证）
	userAllocationHandler := v1.NewUserAllocation(s.userService, s.departmentService)
	user.POST("/assign", userAllocationHandler.AssignUser)
	user.GET("/department", userAllocationHandler.GetUsersByDepartment)

	serverx.ApplyRouteRegistrars(serverx.ServiceHRServer, s.httpServer)
}
