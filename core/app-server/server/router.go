package server

import (
	v1 "github.com/kageos/kageos/core/app-server/api/v1"
	middleware2 "github.com/kageos/kageos/pkg/middleware"
	"github.com/kageos/kageos/pkg/pprof"
	"github.com/kageos/kageos/pkg/serverx"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	jwtAuth := middleware2.JWTAuth()

	// 健康检查
	s.httpServer.GET("/health", s.healthHandler)

	// 注册 pprof 路由（性能分析）
	if s.cfg.IsPprofEnabled() {
		pprof.RegisterPprofRoutes(s.httpServer)
	}

	// Swagger 文档路由
	s.httpServer.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	publicShareHandler := v1.NewPublicShareAPI(s.publicShareService, s.appService, s.permissionService)
	publicShareWriteLimit := newPublicShareRateLimiter().Middleware()
	public := s.httpServer.Group("/public/api")
	public.POST("/anonymous-token", publicShareHandler.AnonymousToken)
	public.GET("/s/:share_id", publicShareHandler.View)
	public.GET("/s/:share_id/submissions", publicShareHandler.Submissions)
	public.POST("/s/:share_id/submit", publicShareWriteLimit, publicShareHandler.Submit)
	public.POST("/s/:share_id/callback/on_select_fuzzy", publicShareWriteLimit, publicShareHandler.CallbackOnSelectFuzzy)

	// Workspace 路由组（统一使用 /workspace/api/v1 开头，方便网关代理）
	workspace := s.httpServer.Group("/workspace")
	apiV1 := workspace.Group("/api/v1")

	// ⭐ 统一添加用户信息中间件，所有接口都需要（网关会透传 token，解析后设置到 X-Request-User header）
	apiV1.Use(middleware2.WithUserInfo())

	// 应用管理路由（需要JWT验证）
	app := apiV1.Group("/app")
	app.Use(jwtAuth) // 应用管理需要JWT认证
	appHandler := v1.NewApp(s.appService, s.serviceTreeService, s.permissionService, s.permissionRequestService)
	app.GET("/list", appHandler.GetApps)
	app.GET("/detail", appHandler.GetAppDetail)
	app.GET("/tree", middleware2.Gzip(), appHandler.GetAppWithServiceTree)
	app.DELETE("/delete", appHandler.DeleteApp)
	app.POST("/create", appHandler.CreateApp)
	app.POST("/personal-workspace", appHandler.BootstrapPersonalWorkspace)
	app.POST("/update", appHandler.UpdateApp)
	app.PUT("/workspace", appHandler.UpdateWorkspace)

	permission := apiV1.Group("/permissions")
	permission.Use(jwtAuth)
	permissionHandler := v1.NewPermission(s.permissionService, s.permissionRequestService)
	permission.GET("/assignments", permissionHandler.ListAssignments)
	permission.POST("/grant", permissionHandler.GrantRole)
	permission.POST("/batch-grant", permissionHandler.BatchGrantRoles)
	permission.POST("/revoke", permissionHandler.RevokeRole)
	permission.GET("/me", permissionHandler.GetMyPermissions)
	permission.GET("/approvers", permissionHandler.ListApprovers)
	permission.POST("/requests", permissionHandler.CreateRequest)
	permission.GET("/requests/mine", permissionHandler.ListMyRequests)
	permission.GET("/requests/pending", permissionHandler.ListPendingRequests)
	permission.GET("/requests/pending-count", permissionHandler.PendingRequestCount)
	permission.GET("/requests/summary", permissionHandler.PermissionRequestSummary)
	permission.GET("/requests/history", permissionHandler.ListRequestHistory)
	permission.POST("/requests/:id/approve", permissionHandler.ApproveRequest)
	permission.POST("/requests/:id/reject", permissionHandler.RejectRequest)
	permission.POST("/requests/:id/cancel", permissionHandler.CancelRequest)

	publicShares := apiV1.Group("/public_shares")
	publicShares.Use(jwtAuth)
	publicShares.POST("", publicShareHandler.Create)
	publicShares.GET("", publicShareHandler.List)
	publicShares.POST("/:share_id/disable", publicShareHandler.Disable)

	// 服务目录管理路由（需要JWT验证）
	serviceTree := apiV1.Group("/service_tree")
	serviceTreeHandler := v1.NewServiceTree(s.serviceTreeService, s.permissionService)

	// 需要JWT验证的路由
	serviceTreeAuth := serviceTree.Group("")
	serviceTreeAuth.Use(jwtAuth)                                                         // 服务目录管理需要JWT认证
	serviceTreeAuth.GET("/detail", serviceTreeHandler.GetServiceTreeDetail)              // 获取服务目录详情
	serviceTreeAuth.POST("/batch_detail", serviceTreeHandler.BatchGetServiceTreeDetails) // 批量获取服务目录详情
	serviceTreeAuth.GET("/overview", serviceTreeHandler.GetDirectoryOverview)            // 获取目录概览
	serviceTreeAuth.GET("/search_functions", serviceTreeHandler.SearchFunctions)         // ⭐ 搜索函数
	serviceTreeAuth.GET("/search_resources", serviceTreeHandler.SearchResources)         // 全站资源搜索（目录/函数/文档）
	serviceTreeAuth.POST("/copy", serviceTreeHandler.CopyServiceTree)                    // 复制服务目录
	serviceTreeAuth.GET("/export_capability_bundle", serviceTreeHandler.ExportCapabilityBundle)
	serviceTreeAuth.POST("/export_capability_bundle", serviceTreeHandler.ExportCapabilityBundle)
	serviceTreeAuth.POST("/install_capability_bundle", serviceTreeHandler.InstallCapabilityBundle)
	serviceTreeAuth.POST("/install_capability_bundle_from_url", serviceTreeHandler.InstallCapabilityBundleFromURL)

	// ⭐ 按类型分离的 CRUD 接口（推荐使用）
	// ==================== Package 类型接口 ====================
	packagesAuth := apiV1.Group("/packages")
	packagesAuth.Use(jwtAuth)
	packagesAuth.POST("", serviceTreeHandler.CreatePackage)       // POST /api/v1/packages
	packagesAuth.PUT("/:id", serviceTreeHandler.UpdatePackage)    // PUT /api/v1/packages/:id
	packagesAuth.DELETE("/:id", serviceTreeHandler.DeletePackage) // DELETE /api/v1/packages/:id

	// ==================== Function 类型接口 ====================
	functionsAuth := apiV1.Group("/functions")
	functionsAuth.Use(jwtAuth)
	functionsAuth.POST("", serviceTreeHandler.CreateFunction)       // POST /api/v1/functions
	functionsAuth.PUT("/:id", serviceTreeHandler.UpdateFunction)    // PUT /api/v1/functions/:id
	functionsAuth.DELETE("/:id", serviceTreeHandler.DeleteFunction) // DELETE /api/v1/functions/:id

	// ==================== Docs 类型接口 ====================
	// ⭐ docs CRUD 接口（使用 /docs/crud 避免与文档管理路由冲突）
	docsCrudAuth := apiV1.Group("/docs/crud")
	docsCrudAuth.Use(jwtAuth)
	docsCrudAuth.POST("", serviceTreeHandler.CreateDocs)       // POST /api/v1/docs/crud
	docsCrudAuth.PUT("/:id", serviceTreeHandler.UpdateDocs)    // PUT /api/v1/docs/crud/:id
	docsCrudAuth.DELETE("/:id", serviceTreeHandler.DeleteDocs) // DELETE /api/v1/docs/crud/:id

	// ⭐ 文档管理路由（基于完整路径，与 table/form/chart 风格一致）
	docs := apiV1.Group("/docs")
	docs.Use(jwtAuth)
	docHandler := v1.NewDoc(s.docService, s.permissionService)
	docs.GET("/search", docHandler.SearchDocs)                 // 搜索文档（模糊搜索）
	docs.GET("/batch", docHandler.BatchGetDocs)                // 批量获取文档（精确查询）
	docs.GET("/info/*full-code-path", docHandler.GetDoc)       // 获取文档
	docs.PUT("/info/*full-code-path", docHandler.UpdateDoc)    // 更新文档
	docs.DELETE("/info/*full-code-path", docHandler.DeleteDoc) // 删除文档

	// ==================== 服务间调用路由 ====================
	// 服务间调用路由（不需要JWT验证，但用户信息中间件已在 apiV1 级别统一添加）
	serviceTree.POST("/add_functions", serviceTreeHandler.AddFunctions) // 向服务目录添加函数（agent-server -> workspace）

	// 工作台环境信息路由（不需要JWT验证，但用户信息中间件已在 apiV1 级别统一添加）
	workspaceGroup := apiV1.Group("/workspace")
	workspaceGroup.GET("/context", serviceTreeHandler.GetWorkspaceContext)       // 获取工作台环境信息（agent-server -> workspace）
	workspaceGroup.POST("/files/write", serviceTreeHandler.WriteFileContent)     // 工作台写入单个文本文件（实时写盘）
	workspaceGroup.POST("/files/replace", serviceTreeHandler.ReplaceFileContent) // 工作台文件 search-replace（实时写盘）
	workspaceGroup.POST("/files/delete", serviceTreeHandler.DeleteFile)          // 工作台删除文件（删磁盘+删节点）
	workspaceGroup.POST("/logs/read", serviceTreeHandler.ReadAppLog)             // 工作台读取应用日志（支持 version/keyword）

	// 函数管理路由（需要JWT验证）
	function := apiV1.Group("/function")
	function.Use(jwtAuth) // 函数管理需要JWT认证
	functionHandler := v1.NewFunction(s.functionService)
	function.GET("/info/:func-type/*full-code-path", functionHandler.GetFunction)

	// 操作日志路由（需要JWT验证）
	operateLog := apiV1.Group("/operate_log")
	operateLog.Use(jwtAuth)                                                         // JWT 认证
	operateLogHandler := v1.NewOperateLog(s.operateLogService, s.permissionService) // 查询统一操作日志
	operateLog.GET("/general", operateLogHandler.GetOperateLogs)                    // 查询通用操作日志

	logArchive := apiV1.Group("/system/log_archives")
	logArchive.Use(jwtAuth)
	logArchive.GET("", v1.NewLogArchive(s.logArchiveService).List)
	logArchive.POST("/:id/retry", v1.NewLogArchive(s.logArchiveService).Retry)

	// 目录更新历史路由（需要JWT验证）
	directoryUpdateHistory := apiV1.Group("/directory_update_history")
	directoryUpdateHistory.Use(jwtAuth) // 目录更新历史需要JWT认证
	directoryUpdateHistoryHandler := v1.NewDirectoryUpdateHistory(s.directoryUpdateHistoryService)
	directoryUpdateHistory.GET("/app_version", directoryUpdateHistoryHandler.GetAppVersionUpdateHistory) // 获取应用版本更新历史（App视角）
	directoryUpdateHistory.GET("/directory", directoryUpdateHistoryHandler.GetDirectoryUpdateHistory)    // 获取目录更新历史（目录视角）

	// ⭐ 标准接口路由（使用 full-code-path）
	standardAPI := v1.NewStandardAPI(s.appService, s.permissionService)

	// Table 函数接口
	table := apiV1.Group("/table")
	table.Use(jwtAuth)
	table.GET("/search/*full-code-path", standardAPI.TableSearch)        // Table 查询
	table.GET("/template/*full-code-path", standardAPI.TableTemplate)    // Table 下载导入模板
	table.POST("/create/*full-code-path", standardAPI.TableCreate)       // Table 新增
	table.POST("/import/*full-code-path", standardAPI.TableImportRow)    // Table 导入（Admin+）
	table.PUT("/update/*full-code-path", standardAPI.TableUpdate)        // Table 更新
	table.DELETE("/delete/*full-code-path", standardAPI.TableDelete)     // Table 删除
	table.GET("/deleted/*full-code-path", standardAPI.TableDeletedRows)  // Table 回收站
	table.POST("/restore/*full-code-path", standardAPI.TableRestoreRows) // Table 恢复
	table.DELETE("/purge/*full-code-path", standardAPI.TablePurgeRows)   // Table 彻底删除
	table.GET("/recycle-policy/*full-code-path", standardAPI.TableRecyclePolicy)
	table.PUT("/recycle-policy/*full-code-path", standardAPI.UpdateTableRecyclePolicy)
	table.POST("/export/plan/*full-code-path", standardAPI.TableExportPlan)
	table.POST("/export/chunk/*full-code-path", standardAPI.TableExportChunk)

	// Form 函数接口
	form := apiV1.Group("/form")
	form.Use(jwtAuth)
	form.POST("/submit/*full-code-path", standardAPI.FormSubmit) // Form 提交

	// 工作台私有 runtime 接口（agent tool -> 当前 workspace app）
	runtime := apiV1.Group("/runtime")
	runtime.Use(jwtAuth)
	runtime.POST("/python/*full-code-path", standardAPI.RuntimePython) // run_python 私有执行

	// Chart 函数接口
	chart := apiV1.Group("/chart")
	chart.Use(jwtAuth)
	chart.GET("/query/*full-code-path", standardAPI.ChartQuery) // Chart 查询

	// Callback 接口（不需要权限检查，因为这是内部回调）
	callbackStandard := apiV1.Group("/callback")
	callbackStandard.Use(jwtAuth)
	callbackStandard.POST("/on_select_fuzzy/*full-code-path", standardAPI.CallbackOnSelectFuzzy) // 模糊搜索回调

	serverx.ApplyRouteRegistrars(serverx.ServiceAppServer, s.httpServer)
}
