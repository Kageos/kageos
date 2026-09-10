package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	sharedDto "github.com/kageos/kageos/dto"

	"github.com/kageos/kageos/core/app-runtime/model"
	"github.com/kageos/kageos/core/app-runtime/repository"
	"github.com/kageos/kageos/pkg/builder"
	"github.com/kageos/kageos/pkg/buildtrace"
	appconfig "github.com/kageos/kageos/pkg/config"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/gitx"
	"github.com/kageos/kageos/pkg/logger"
	"github.com/kageos/kageos/pkg/subjects"
	"github.com/nats-io/nats.go"
)

// CreateOpts 创建选项
type CreateOpts struct {
	Env     map[string]string
	Volumes []string
}

// BuildOpts 编译选项
type BuildOpts struct {
	SourceDir        string            // 源代码目录
	OutputDir        string            // 输出目录
	BinaryNameFormat string            // 二进制文件名格式
	BuildTags        []string          // 编译标签
	LdFlags          []string          // 链接参数
	Env              map[string]string // 编译环境变量
}

// StartupNotification 启动通知
type StartupNotification struct {
	User      string
	App       string
	Version   string
	Status    string
	StartTime time.Time
	Error     string
}

// CloseNotification 关闭通知
type CloseNotification struct {
	User      string
	App       string
	Version   string
	CloseTime time.Time
}

// AppManageService 应用管理服务 - 负责应用的增删改查
type AppManageService struct {
	builder              *builder.Builder
	config               *appconfig.AppManageServiceConfig
	runtimeConfig        *appconfig.AppRuntimeConfig // 运行时完整配置（用于获取网关地址等）
	runtimeDriver        AppRuntimeDriver            // 应用版本运行时抽象
	appRepo              *repository.AppRepository   // 应用数据访问层
	appDiscoveryService  *AppDiscoveryService        // 应用发现服务，用于获取运行状态
	appControlClient     *AppControlClient           // runtime -> app 控制调用
	appDatabaseService   *AppDatabaseService         // runtime-managed app DB capability issuer
	QPSTracker           *QPSTracker                 // QPS 跟踪器
	workspaceFileService *WorkspaceFileService       // 工作区源码文件服务

	// 启动等待器 - 用于等待应用启动完成通知
	startupWaiters   map[string]chan *StartupNotification // key: user/app/version
	startupWaitersMu sync.RWMutex

	// 关闭等待器 - 用于等待应用关闭完成通知
	closeWaiters   map[string]chan *CloseNotification // key: user/app/version
	closeWaitersMu sync.RWMutex

	// 周期清理控制（进程级+容器级合并为一次完整清理，由 cron + 有变动时触发）
	cleanupDone chan struct{}

	// 容器级对账巡检控制（cron 低峰期执行 + 有变动时由 ticker 触发）
	containerCleanupTicker *time.Ticker
	containerCleanupDone   chan struct{}
	containerCleanupCron   *cron.Cron // 每日定点执行，如 "0 4 * * *" 表示凌晨 4 点

	// 有版本/容器变动时置为 true，ticker 检查到后执行一次巡检
	containerCleanupMu    sync.Mutex
	containerCleanupDirty bool
}

// ============================================================================
// 容器名工具函数
// ============================================================================

// BuildContainerName 构建容器名（新格式：{user}-{app}-{version}）
// 公开函数，供其他包使用
func BuildContainerName(user, app, version string) string {
	return fmt.Sprintf("%s-%s-%s", user, app, version)
}

// buildContainerName 构建容器名（内部使用，调用公开函数）
func buildContainerName(user, app, version string) string {
	return BuildContainerName(user, app, version)
}

// parseContainerName 解析容器名（格式：{user}-{app}-{version}）
// 返回：user, app, version, error
func parseContainerName(containerName string) (string, string, string, error) {
	parts := strings.Split(containerName, "-")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid container name format: %s, expected {user}-{app}-{version}", containerName)
	}
	// 最后一部分是 version
	version := parts[len(parts)-1]
	// 前面是 user-app（假设 user 和 app 都不包含连字符）
	user := parts[0]
	app := strings.Join(parts[1:len(parts)-1], "-")
	return user, app, version, nil
}

type AppManageServiceDependencies struct {
	Builder              *builder.Builder
	Config               *appconfig.AppManageServiceConfig
	RuntimeConfig        *appconfig.AppRuntimeConfig
	ContainerService     ContainerOperator
	AppRepository        *repository.AppRepository
	AppDiscoveryService  *AppDiscoveryService
	NATSConnection       *nats.Conn
	WorkspaceFileService *WorkspaceFileService
	AppDatabaseService   *AppDatabaseService
}

// NewAppManageService 创建应用管理服务（依赖注入）
func NewAppManageService(deps AppManageServiceDependencies) *AppManageService {
	return &AppManageService{
		builder:              deps.Builder,
		config:               deps.Config,
		runtimeConfig:        deps.RuntimeConfig,
		runtimeDriver:        NewPodmanAppRuntimeDriver(deps.ContainerService),
		appRepo:              deps.AppRepository,
		appDiscoveryService:  deps.AppDiscoveryService,
		appControlClient:     NewAppControlClient(deps.NATSConnection),
		appDatabaseService:   deps.AppDatabaseService,
		QPSTracker:           NewQPSTracker(60*time.Second, 10*time.Second), // 60秒窗口，10秒检查间隔
		workspaceFileService: deps.WorkspaceFileService,
		startupWaiters:       make(map[string]chan *StartupNotification),
		closeWaiters:         make(map[string]chan *CloseNotification),
		cleanupDone:          make(chan struct{}),
		containerCleanupDone: make(chan struct{}),
	}
}

// CreateApp 创建应用目录结构
func (s *AppManageService) CreateApp(ctx context.Context, user, app string, opts ...*CreateOpts) (string, error) {
	logger.Infof(ctx, "[CreateApp] *** ENTRY *** user=%s, app=%s", user, app)

	appPaths := newRuntimeAppPaths(s.config.GetBasePath(), user, app)
	appDirRel := appPaths.AppDir()
	absAppDir, err := filepath.Abs(appDirRel)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	absPaths := newRuntimeAppPathsFromAppDir(absAppDir, user, app)

	// 2. 定义完整的目录结构（使用配置中的结构）
	dirs := []string{
		// 应用根目录
		absAppDir,
	}

	// 添加配置中定义的目录结构
	for _, dir := range s.config.GetStructure() {
		dirs = append(dirs, filepath.Join(absAppDir, dir))
	}

	// 3. 创建所有目录
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// 4. 启动脚本已内置在 kageos 镜像中，无需复制

	// 5. 创建应用时不创建版本文件，版本文件将在第一次编译时创建
	// 这样可以避免创建时就写入版本信息
	logger.Infof(ctx, "[CreateApp] Skipping version files creation, will be created on first build")

	if err := s.ensureAppGoModFile(absPaths); err != nil {
		return "", fmt.Errorf("failed to create go.mod file: %w", err)
	}

	// 6. 创建 main.go 文件
	mainGoPath := absPaths.MainGoPath()
	if err := s.createMainGoFile(mainGoPath, user, app); err != nil {
		return "", fmt.Errorf("failed to create main.go file: %w", err)
	}

	// 8. 保存应用信息到数据库
	if err := s.appRepo.CreateApp(user, app); err != nil {
		return "", fmt.Errorf("failed to create app in database: %w", err)
	}

	// 9. 创建应用时不编译和启动，节省资源
	// 编译和启动将在 UpdateApp 时进行
	logger.Infof(ctx, "[CreateApp] App directory structure created successfully, skipping build and container start to save resources")

	logger.Infof(ctx, "[CreateApp] *** EXIT *** user=%s, app=%s, appDir=%s", user, app, appDirRel)
	return appDirRel, nil
}

// BuildApp 编译应用
func (s *AppManageService) BuildApp(ctx context.Context, user, app string, opts ...*BuildOpts) (*builder.BuildResult, error) {
	//logger.Infof(ctx, "[BuildApp] *** ENTRY *** user=%s, app=%s", user, app)

	appPaths := newRuntimeAppPaths(s.config.GetBasePath(), user, app)

	// 设置默认编译选项（平台由 builder 内部固定为 linux/当前架构）
	buildOpts := &builder.BuildOpts{
		SourceDir:         appPaths.CmdAppDir(),
		OutputDir:         appPaths.BuildOutputDir(s.config.GetBuildOutputDir()),
		BinaryNameFormat:  s.config.GetBinaryNameFormat(),
		StripDebugSymbols: s.config.GetStripDebugSymbols(),
	}

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		// 转换类型，保留所有字段（平台由 builder 内部固定为 linux/当前架构）
		buildOpts = &builder.BuildOpts{
			User:              user,
			App:               app,
			SourceDir:         nonEmpty(opt.SourceDir, buildOpts.SourceDir),
			OutputDir:         nonEmpty(opt.OutputDir, buildOpts.OutputDir),
			BinaryNameFormat:  nonEmpty(opt.BinaryNameFormat, buildOpts.BinaryNameFormat),
			BuildTags:         opt.BuildTags,
			LdFlags:           opt.LdFlags,
			StripDebugSymbols: buildOpts.StripDebugSymbols,
			Env:               opt.Env,
		}
	}

	ensureSpan := buildtrace.Start(ctx, "runtime.ensure_go_mod", buildtrace.String("go_mod", appPaths.GoModPath()))
	if err := s.ensureAppGoModFile(appPaths); err != nil {
		ensureSpan.Finish(err)
		return nil, fmt.Errorf("failed to ensure app go.mod: %w", err)
	}
	ensureSpan.Finish(nil)

	// 执行编译
	buildSpan := buildtrace.Start(ctx, "runtime.builder_build_app",
		buildtrace.String("source_dir", buildOpts.SourceDir),
		buildtrace.String("output_dir", buildOpts.OutputDir),
	)
	result, err := s.builder.Build(ctx, user, app, buildOpts)
	if err != nil {
		buildSpan.Finish(err)
		logger.Errorf(ctx, "[BuildApp] *** FAILED *** user=%s, app=%s, error=%v", user, app, err)
		return nil, err
	}
	buildSpan.Finish(nil)

	return result, nil
}

// DeleteApp 删除应用
// 新架构：每个版本有独立容器，需要删除所有版本的容器
func (s *AppManageService) DeleteApp(ctx context.Context, user, app string) error {
	logger.Infof(ctx, "[DeleteApp] *** ENTRY *** user=%s, app=%s", user, app)

	// 1. 获取应用的所有版本，删除每个版本的运行时实例
	if s.runtimeDriver != nil {
		// 获取所有版本
		versions, err := s.appRepo.GetAppVersions(user, app)
		if err != nil {
			logger.Warnf(ctx, "[DeleteApp] Failed to get app versions: %v, will try to delete runtime instances by pattern", err)
			// 如果获取版本失败，尝试通过运行时名称模式查找并删除。
			// 这里可以扩展 AppRuntimeDriver 接口支持按应用查找，暂时先跳过。
		} else {
			// 删除每个版本的运行时实例
			for _, version := range versions {
				ref := AppVersionRef{User: user, App: app, Version: version.Version}
				runtimeName := ref.RuntimeName()

				// 先尝试停止运行时实例（如果正在运行）
				if err := s.runtimeDriver.StopAppVersion(ctx, ref); err != nil {
					logger.Warnf(ctx, "[DeleteApp] Failed to stop runtime instance %s (may not be running): %v", runtimeName, err)
				} else {
					logger.Infof(ctx, "[DeleteApp] Runtime instance %s stopped successfully", runtimeName)
				}

				// 强制删除运行时实例（无论是否正在运行）
				if err := s.runtimeDriver.RemoveAppVersion(ctx, ref); err != nil {
					logger.Warnf(ctx, "[DeleteApp] Failed to remove runtime instance %s: %v", runtimeName, err)
					// 不返回错误，继续删除其他实例
				} else {
					logger.Infof(ctx, "[DeleteApp] Runtime instance %s removed successfully", runtimeName)
				}
			}
			s.MarkContainerCleanupDirty() // 有运行时实例被删，下次巡检周期会做对账
		}
	} else {
		logger.Warnf(ctx, "[DeleteApp] App runtime driver is nil, skipping runtime deletion")
	}

	// 2. 删除应用目录
	appDirRel := newRuntimeAppPaths(s.config.GetBasePath(), user, app).AppDir()
	absAppDir, err := filepath.Abs(appDirRel)
	if err != nil {
		logger.Warnf(ctx, "[DeleteApp] Failed to get absolute path: %v", err)
	} else {
		if err := os.RemoveAll(absAppDir); err != nil {
			logger.Warnf(ctx, "[DeleteApp] Failed to remove app directory %s: %v", absAppDir, err)
		} else {
			logger.Infof(ctx, "[DeleteApp] App directory removed: %s", absAppDir)
		}
	}

	// 3. 删除数据库记录
	if err := s.appRepo.DeleteAppAndVersions(user, app); err != nil {
		logger.Warnf(ctx, "[DeleteApp] Failed to delete app and versions from database: %v", err)
	}

	logger.Infof(ctx, "[DeleteApp] *** EXIT *** user=%s, app=%s", user, app)
	return nil
}

// UpdateApp 更新应用（写入源码文件并重新编译部署）
// 如果提供了 sourceFiles，先执行源码文件写入。
// writeOnly 为 true 时仅写文件，不编译不部署。
func (s *AppManageService) UpdateApp(ctx context.Context, user, app string, sourceFiles []*sharedDto.SourceFileWrite, requirement, changeDescription string, writeOnly bool, forceDiff bool) (resp *sharedDto.UpdateAppResp, err error) {
	ctx, trace := buildtrace.Ensure(ctx, "runtime.update_app", user, app)
	defer func() {
		traceSnapshot := trace.Finalize(err)
		if path, persistErr := s.persistUpdateBuildTrace(ctx, user, app, trace); persistErr != nil {
			logger.Warnf(ctx, "[UpdateApp] build trace persist failed: trace_id=%s, error=%v", traceSnapshot.TraceID, persistErr)
		} else if path != "" {
			traceSnapshot = trace.Snapshot()
		}
		if resp != nil {
			resp.BuildTrace = traceSnapshot
		}
		logger.Infof(ctx, "[UpdateApp] build trace summary: trace_id=%s, status=%s, %s",
			traceSnapshot.TraceID, traceSnapshot.Status, buildtrace.Summary(traceSnapshot, 6))
	}()

	logStr := strings.Builder{}
	logStr.WriteString(fmt.Sprintf("[UpdateApp] Starting update: %s/%s\t", user, app))

	prepareSpan := buildtrace.Start(ctx, "runtime.prepare_update_state", buildtrace.String("user", user), buildtrace.String("app", app))
	state, err := s.prepareUpdateAppState(ctx, user, app)
	if err != nil {
		prepareSpan.Finish(err)
		return nil, err
	}
	prepareSpan.Finish(nil)
	s.noteUnknownUpdateVersion(state, &logStr)

	writeSpan := buildtrace.Start(ctx, "runtime.write_source_files", buildtrace.Int("file_count", len(sourceFiles)))
	sourceWriteState, err := s.writeSourceFilesForUpdate(ctx, user, app, sourceFiles)
	if err != nil {
		writeSpan.Finish(err)
		return nil, err
	}
	writeSpan.Finish(nil)

	if writeOnly {
		writeOnlySpan := buildtrace.Start(ctx, "runtime.write_only_response")
		resp = s.buildWriteOnlyUpdateResp(ctx, user, app, state.oldVersion)
		writeOnlySpan.Finish(nil)
		return resp, nil
	}

	releaseSpan := buildtrace.Start(ctx, "runtime.build_and_deploy_release",
		buildtrace.String("old_version", state.oldVersion),
		buildtrace.Bool("force_diff", forceDiff),
	)
	release, err := s.buildAndDeployUpdatedRelease(
		ctx,
		user,
		app,
		state,
		sourceWriteState,
		requirement,
		changeDescription,
		forceDiff,
		&logStr,
	)
	if err != nil {
		releaseSpan.Finish(err)
		return nil, err
	}
	releaseSpan.Finish(nil)

	completeSpan := buildtrace.Start(ctx, "runtime.complete_update_release", buildtrace.String("new_version", release.newVersion))
	resp, err = s.completeUpdatedRelease(ctx, user, app, release)
	completeSpan.Finish(err)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// updateAppStatusToActive 将应用状态更新为active（已激活）
func (s *AppManageService) updateAppStatusToActive(ctx context.Context, user, app string) error {
	appRecord, err := s.appRepo.GetApp(user, app)
	if err != nil {
		return fmt.Errorf("failed to get app record: %w", err)
	}

	// 更新状态为active
	appRecord.Status = "active"
	if err := s.appRepo.UpdateApp(appRecord); err != nil {
		return fmt.Errorf("failed to update app status to active: %w", err)
	}

	return nil
}

func (s *AppManageService) buildUpdateAppResp(
	user, app string,
	release *appReleaseResult,
	diffData *sharedDto.DiffData,
) *sharedDto.UpdateAppResp {
	return &sharedDto.UpdateAppResp{
		User:          user,
		App:           app,
		OldVersion:    release.oldVersion,
		NewVersion:    release.newVersion,
		GitCommitHash: release.gitCommitHash, // Git 提交哈希
		Diff:          diffData,              // 转换后的 diff 信息
		Error:         "",
	}
}

// sendUpdateCallbackAndWait 使用 NATS Request/Reply 模式发送 update 回调并等待响应
func (s *AppManageService) sendUpdateCallbackAndWait(ctx context.Context, user, app, version string) (*subjects.Message, error) {
	if s.appControlClient == nil {
		return nil, fmt.Errorf("NATS connection is nil")
	}

	// 构建更新回调请求
	data := map[string]interface{}{"trigger": "update_callback"}
	if s.appDatabaseService != nil && s.appDatabaseService.IsEnabled() {
		capability, err := s.appDatabaseService.IssueCapability(user, app, version, "")
		if err != nil {
			return nil, fmt.Errorf("issue app database capability: %w", err)
		}
		data["db_capability"] = capability
	}
	request := subjects.Message{
		Type:      subjects.MessageTypeStatusOnAppUpdate,
		User:      user,
		App:       app,
		Version:   version,
		Data:      data,
		Timestamp: time.Now(),
	}

	rsp, err := s.appControlClient.RequestUpdateCallback(ctx, user, app, version, &request, s.updateCallbackTimeout())
	if err != nil {
		logger.Errorf(ctx, "[sendUpdateCallbackAndWait] ❌ Request failed: %v", err)
		return rsp, err
	}
	return rsp, nil
}

// UpdateResult 更新结果
//type UpdateResult struct {
//	User       string
//	App        string
//	OldVersion string
//	NewVersion string
//	Diff       interface{} `json:"diff,omitempty"`  // API diff 信息
//	Error      error       `json:"error,omitempty"` // 回调过程中的错误
//}

// GetAppInfo 获取应用信息
func (s *AppManageService) GetAppInfo(ctx context.Context, user, app string) (map[string]interface{}, error) {
	appDir := newRuntimeAppPaths(s.config.GetBasePath(), user, app).AppDir()

	// 检查应用是否存在
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("app not found: %s/%s", user, app)
	}

	version, err := s.readCurrentVersion(user, app)
	if err != nil {
		return nil, fmt.Errorf("failed to read current version: %w", err)
	}

	return map[string]interface{}{
		"user":    user,
		"app":     app,
		"app_dir": appDir,
		"version": version,
	}, nil
}

// IsAppRunning 检查应用是否正在运行
// 使用discovery service检查运行状态，比调用podman更高效
func (s *AppManageService) IsAppRunning(ctx context.Context, user, app string) (bool, error) {
	if s.appDiscoveryService == nil {
		return false, fmt.Errorf("app discovery service not available")
	}

	// 使用discovery service检查应用是否正在运行
	return s.appDiscoveryService.IsAppRunning(user, app), nil
}

// IsAppVersionRuntimeRunning 检查指定应用版本的底层运行时实例是否正在运行。
func (s *AppManageService) IsAppVersionRuntimeRunning(ctx context.Context, user, app, version string) (bool, error) {
	if s.runtimeDriver == nil {
		return false, fmt.Errorf("app runtime driver not available")
	}
	return s.runtimeDriver.IsAppVersionRunning(ctx, AppVersionRef{User: user, App: app, Version: version})
}

// EnsureAppVersionRuntimeRunning 只保证底层运行时实例已启动，不等待应用启动通知。
func (s *AppManageService) EnsureAppVersionRuntimeRunning(ctx context.Context, user, app, version string) error {
	if s.runtimeDriver == nil {
		return fmt.Errorf("app runtime driver not available")
	}

	ref := AppVersionRef{User: user, App: app, Version: version}
	appDirRel := newRuntimeAppPaths(s.config.GetBasePath(), user, app).AppDir()
	spec, err := s.buildAppVersionSpec(ctx, ref, appDirRel)
	if err != nil {
		return err
	}

	if err := s.runtimeDriver.StartAppVersion(ctx, spec); err != nil {
		return fmt.Errorf("failed to start app runtime version: %w", err)
	}
	logger.Infof(ctx, "[EnsureAppVersionRuntimeRunning] Runtime instance %s started successfully", ref.RuntimeName())
	s.MarkContainerCleanupDirty()
	return nil
}

// createVersionContainer 创建版本容器
// 这是新架构的核心方法：每个版本使用独立的容器
func (s *AppManageService) createVersionContainer(ctx context.Context, user, app, version, appDir string) error {
	ref := AppVersionRef{User: user, App: app, Version: version}
	runtimeName := ref.RuntimeName()
	logger.Infof(ctx, "[createVersionContainer] Creating app runtime instance: %s for %s/%s/%s", runtimeName, user, app, version)

	if s.runtimeDriver == nil {
		logger.Errorf(ctx, "App runtime driver not available")
		return fmt.Errorf("app runtime driver not available")
	}

	// 检查运行时实例是否已存在
	checkSpan := buildtrace.Start(ctx, "runtime.check_version_running", buildtrace.String("runtime_name", runtimeName))
	exists, err := s.runtimeDriver.IsAppVersionRunning(ctx, ref)
	if err != nil {
		checkSpan.Finish(err)
		return fmt.Errorf("failed to check app runtime instance existence: %w", err)
	}
	checkSpan.Finish(nil)

	if exists {
		logger.Infof(ctx, "[createVersionContainer] App runtime instance %s already exists and is running; reusing it", runtimeName)
		return nil
	}

	specSpan := buildtrace.Start(ctx, "runtime.build_app_version_spec", buildtrace.String("runtime_name", runtimeName))
	spec, err := s.buildAppVersionSpec(ctx, ref, appDir)
	if err != nil {
		specSpan.Finish(err)
		return err
	}
	specSpan.Finish(nil)
	createSpan := buildtrace.Start(ctx, "runtime.driver_create_app_version",
		buildtrace.String("runtime_name", runtimeName),
		buildtrace.String("image", spec.Image),
	)
	if err := s.runtimeDriver.CreateAppVersion(ctx, spec); err != nil {
		createSpan.Finish(err)
		return err
	}
	createSpan.Finish(nil)
	logger.Infof(ctx, "App runtime instance started successfully with runtime image %s", spec.Image)
	s.MarkContainerCleanupDirty() // 有新运行时实例，下次巡检周期会做对账
	return nil
}

// buildAppVersionSpec 构建应用版本运行时启动参数。
func (s *AppManageService) buildAppVersionSpec(ctx context.Context, ref AppVersionRef, appDir string) (AppVersionSpec, error) {
	logger.Infof(ctx, "Building app runtime spec: %s, appDir: %s, version: %s", ref.RuntimeName(), appDir, ref.Version)

	// 使用 runtime 配置里的应用基础镜像启动容器，挂载应用目录。
	image := s.runtimeConfig.GetContainerBaseImage()
	// 将相对路径转换为绝对路径，避免 Podman 把它当成卷名
	absHostPath, err := filepath.Abs(appDir)
	if err != nil {
		logger.Errorf(ctx, "Failed to get absolute path: %v", err)
		return AppVersionSpec{}, fmt.Errorf("failed to get absolute path: %w", err)
	}
	containerPath := s.runtimeConfig.GetContainerPath()

	logger.Infof(ctx, "[buildAppVersionSpec] Runtime mount: image=%s, name=%s, hostPath=%s, containerPath=%s", image, ref.RuntimeName(), absHostPath, containerPath)

	// 设置环境变量
	envVars := []string{}

	// 注入 SDK 配置（专门用于容器内访问平台服务）。
	// SDK 进程启动后会在自身网络命名空间内自动探测 127.0.0.1 /
	// host.containers.internal 等本地候选地址，避免 prod host 网络和 dev bridge
	// 网络使用同一份静态地址。
	//
	// NATS 凭据通过 Podman Secret 挂载到 SDK 的固定读取路径，不能进入环境变量；
	// gateway_url 和 env_vars 中的非敏感配置仍作为环境变量注入。
	sdkConfig := appconfig.GetSDKConfig()
	natsSecret := RuntimeSecret{
		Name:   appNATSSecretName(ref),
		Target: appNATSSecretTarget,
		Data:   []byte(sdkConfig.GetNatsURL()),
	}

	// 从 SDK 配置获取允许暴露给 App 进程的非敏感环境变量。
	sdkEnvVars := sdkConfig.GetEnvVars()
	for key, value := range sdkEnvVars {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
		logger.Infof(ctx, "[buildAppVersionSpec] Injecting SDK env %s into app runtime", key)
	}

	binaryName := s.appBinaryName(ref.User, ref.App, ref.Version)
	containerWorkDir := filepath.ToSlash(filepath.Join(containerPath, "workplace", "bin"))
	containerBinDir := filepath.ToSlash(filepath.Join(containerPath, "workplace", "bin", "releases"))
	runtimeID := s.runtimeInstanceID()

	// 注入版本信息到环境变量（新架构：每个容器对应特定版本）。
	// 启动脚本优先消费这些 env，metadata 文件仅做兼容兜底。
	envVars = append(envVars,
		fmt.Sprintf("KAGEOS_APP_USER=%s", ref.User),
		fmt.Sprintf("KAGEOS_APP_NAME=%s", ref.App),
		fmt.Sprintf("APP_VERSION=%s", ref.Version),
		fmt.Sprintf("APP_BINARY_NAME=%s", binaryName),
		fmt.Sprintf("KAGEOS_APP_WORK_DIR=%s", containerWorkDir),
		fmt.Sprintf("KAGEOS_APP_BIN_DIR=%s", containerBinDir),
	)
	if runtimeID != "" {
		envVars = append(envVars, fmt.Sprintf("KAGEOS_RUNTIME_INSTANCE_ID=%s", runtimeID))
	}
	logger.Infof(ctx, "[buildAppVersionSpec] Injecting app runtime metadata env keys: %v", []string{
		"KAGEOS_APP_USER",
		"KAGEOS_APP_NAME",
		"APP_VERSION",
		"APP_BINARY_NAME",
		"KAGEOS_APP_WORK_DIR",
		"KAGEOS_APP_BIN_DIR",
		"KAGEOS_RUNTIME_INSTANCE_ID",
	})

	return AppVersionSpec{
		Ref:           ref,
		Image:         image,
		HostPath:      absHostPath,
		ContainerPath: containerPath,
		Command:       []string{"/start.sh"},
		EnvVars:       envVars,
		Secrets:       []RuntimeSecret{natsSecret},
	}, nil
}

// stopOldVersionContainer 优雅关闭旧版本容器。
// SDK 只有在停止接收新请求且已有请求全部返回后才发送 close 通知。
// 等待超时只跳过本轮清理，不强制停止，避免中断长时间函数。
func (s *AppManageService) stopOldVersionContainer(ctx context.Context, user, app, oldVersion string) error {
	ref := AppVersionRef{User: user, App: app, Version: oldVersion}
	runtimeName := ref.RuntimeName()
	logger.Infof(ctx, "[stopOldVersionContainer] Starting graceful shutdown for old runtime instance: %s", runtimeName)

	if s.runtimeDriver == nil {
		logger.Warnf(ctx, "[stopOldVersionContainer] App runtime driver not available, skipping")
		return nil
	}

	// 1. 检查运行时实例是否存在
	exists, err := s.runtimeDriver.IsAppVersionRunning(ctx, ref)
	if err != nil {
		logger.Warnf(ctx, "[stopOldVersionContainer] Failed to check runtime instance existence: %v", err)
		return nil // 不返回错误，继续执行
	}
	if !exists {
		logger.Infof(ctx, "[stopOldVersionContainer] Old runtime instance %s not found, skipping", runtimeName)
		return nil
	}

	if err := s.shutdownVersionGracefullyForCleanup(ctx, user, app, oldVersion, "stopOldVersionContainer"); err != nil {
		return err
	}

	// close 通知已确认 SDK 排空并完成资源清理，此时停止实例不会中断函数。
	logger.Infof(ctx, "[stopOldVersionContainer] Stopping runtime instance %s (not removing)", runtimeName)
	if err := s.runtimeDriver.StopAppVersion(ctx, ref); err != nil {
		return fmt.Errorf("failed to stop app runtime instance: %w", err)
	}

	logger.Infof(ctx, "[stopOldVersionContainer] Old runtime instance %s stopped successfully", runtimeName)
	s.MarkContainerCleanupDirty() // 有运行时实例被停，下次巡检周期会做对账
	return nil
}

// ShutdownAppVersion 主动关闭指定版本的应用
func (s *AppManageService) ShutdownAppVersion(ctx context.Context, user, app, version string) error {
	//logger.Infof(ctx, "[ShutdownAppVersion] Sending shutdown command to %s/%s/%s", user, app, version)

	// 构建关闭命令消息（使用 subjects.Message 格式）
	message := subjects.Message{
		Type:      subjects.MessageTypeStatusShutdown,
		User:      user,
		App:       app,
		Version:   version,
		Data:      map[string]interface{}{"command": "shutdown"},
		Timestamp: time.Now(),
	}

	if err := s.appControlClient.PublishShutdown(ctx, user, app, version, &message); err != nil {
		return err
	}

	//logger.Infof(ctx, "[ShutdownAppVersion] Shutdown command sent to %s", subject)
	return nil
}

// ShutdownOldVersions 关闭旧版本的应用（保留指定数量的最新版本）
func (s *AppManageService) ShutdownOldVersions(ctx context.Context, user, app string, keepVersions int) error {
	logger.Infof(ctx, "[ShutdownOldVersions] Shutting down old versions for %s/%s, keeping %d versions", user, app, keepVersions)

	// 从内存中获取运行中的版本（通过 AppDiscoveryService）
	runningApps := s.appDiscoveryService.GetRunningApps()
	appKey := user + "/" + app
	appInfo, exists := runningApps[appKey]
	if !exists {
		logger.Infof(ctx, "[ShutdownOldVersions] No running versions found for %s/%s", user, app)
		return nil
	}

	// 转换为版本列表
	var runningVersions []string
	for versionKey := range appInfo.Versions {
		runningVersions = append(runningVersions, versionKey)
	}

	if len(runningVersions) <= keepVersions {
		logger.Infof(ctx, "[ShutdownOldVersions] Only %d versions running, no need to shutdown", len(runningVersions))
		return nil
	}

	// 关闭旧版本（基于完整静默期，优先服务稳定）
	// 注意：这里简化逻辑，因为内存中的版本信息不包含创建时间
	// 实际应用中，应该根据业务需求决定关闭策略
	versionsToShutdown := runningVersions[keepVersions:]
	for _, version := range versionsToShutdown {
		if !s.isVersionQuietForCleanup(ctx, user, app, version, "ShutdownOldVersions") {
			continue
		}

		if err := s.shutdownVersionGracefullyForCleanup(ctx, user, app, version, "ShutdownOldVersions"); err != nil {
			logger.Warnf(ctx, "[ShutdownOldVersions] 本轮跳过版本 %s: %v", version, err)
		} else {
			logger.Infof(ctx, "[ShutdownOldVersions] Version %s closed gracefully", version)
		}
	}

	return nil
}

// StartCleanupTask 启动定时清理任务
// 进程级清理 + 容器级巡检 + release 二进制清理合并为「一次完整清理」，在凌晨 4 点与有变动时执行。
func (s *AppManageService) StartCleanupTask(ctx context.Context) {
	const containerCleanupCronExpr = "0 4 * * *" // 每天凌晨 4 点（cron：分 时 日 月 周）
	logger.Infof(ctx, "[CleanupTask] 启动定时清理 | 进程级+容器级+二进制=cron(%s)+有变动时 | 顺序=进程级→容器级→二进制 | 保留版本数=%d",
		containerCleanupCronExpr, maxKeepVersions)

	// 凌晨 4 点：先进程级（按当前版本停非当前），再容器级（保留最近 3 版本并删除多余），最后裁剪旧二进制。
	s.containerCleanupCron = cron.New(cron.WithLocation(time.Local))
	_, err := s.containerCleanupCron.AddFunc(containerCleanupCronExpr, func() {
		logger.Infof(ctx, "[CleanupTask] cron 触发 | 执行进程级清理 + 容器级巡检 + release 二进制清理 + workplace(file-cache/output/uploads)清空")
		s.runAllCleanups(ctx)
		s.runWorkplaceTempCleanup(ctx)
	})
	if err != nil {
		logger.Warnf(ctx, "[CleanupTask] cron 添加失败: %v，将仅依赖有变动时触发", err)
	} else {
		s.containerCleanupCron.Start()
	}

	// 每 1 分钟检查是否有“有变动”标记，有则执行一次完整清理（进程级+容器级+二进制）
	s.containerCleanupTicker = time.NewTicker(1 * time.Minute)

	go func() {
		defer s.containerCleanupTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Infof(ctx, "[CleanupTask] 清理任务已停止 (context canceled)")
				return
			case <-s.containerCleanupDone:
				logger.Infof(ctx, "[CleanupTask] 清理任务已停止 (signal)")
				return
			case <-s.containerCleanupTicker.C:
				s.maybeRunContainerLevelCleanup(ctx)
			}
		}
	}()
}

// runAllCleanups 执行一次完整清理：先进程级（按当前版本停非当前），再容器级（保留最近 3 版本并删除多余），最后裁剪旧 release 二进制。
func (s *AppManageService) runAllCleanups(ctx context.Context) {
	s.performCleanup(ctx)        // 进程级：按 current_version 停掉非当前且无流量的版本
	s.containerLevelCleanup(ctx) // 容器级：每应用保留最近 3 版本，其余 stop+remove
	s.releaseBinaryCleanup(ctx)  // 文件级：每应用保留 current + 最近 3 个 release 二进制
}

// runWorkplaceTempCleanup 清空各应用 workplace 下的临时目录（全部删除，无需保留）
func (s *AppManageService) runWorkplaceTempCleanup(ctx context.Context) {
	apps, err := s.getAllApps(ctx)
	if err != nil {
		logger.Errorf(ctx, "[WorkplaceCleanup] 获取应用列表失败: %v", err)
		return
	}
	for _, app := range apps {
		appPaths := newRuntimeAppPaths(s.config.GetBasePath(), app.User, app.App)
		for _, subdir := range []string{"file-cache", "output", "uploads"} {
			dir := appPaths.WorkplaceSubDir(subdir)
			if _, err := os.Stat(dir); err != nil {
				if !os.IsNotExist(err) {
					logger.Warnf(ctx, "[WorkplaceCleanup] 检查目录失败 %s: %v", dir, err)
				}
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				logger.Warnf(ctx, "[WorkplaceCleanup] 清空失败 %s: %v", dir, err)
				continue
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				logger.Warnf(ctx, "[WorkplaceCleanup] 重建目录失败 %s: %v", dir, err)
				continue
			}
			logger.Infof(ctx, "[WorkplaceCleanup] 已清空: %s/%s workplace/%s", app.User, app.App, subdir)
		}
	}
}

// StopCleanupTask 停止定时清理任务
func (s *AppManageService) StopCleanupTask(ctx context.Context) {
	if s.containerCleanupTicker != nil {
		s.containerCleanupTicker.Stop()
	}
	if s.containerCleanupCron != nil {
		s.containerCleanupCron.Stop()
	}

	select {
	case s.cleanupDone <- struct{}{}:
	default:
	}
	select {
	case s.containerCleanupDone <- struct{}{}:
	default:
	}

	logger.Infof(ctx, "[AppManageService] Cleanup tasks stopped")
}

// performCleanup 执行清理任务
func (s *AppManageService) performCleanup(ctx context.Context) {
	//logger.Infof(ctx, "[AppManageService] Performing cleanup check...")

	// 获取所有应用
	apps, err := s.getAllApps(ctx)
	if err != nil {
		logger.Errorf(ctx, "[AppManageService] Failed to get apps: %v", err)
		return
	}

	if len(apps) == 0 {
		return
	}

	// 为每个应用执行清理
	for _, app := range apps {
		// 清理非当前版本的无流量版本
		if err := s.CleanupNonCurrentVersions(ctx, app.User, app.App); err != nil {
			logger.Errorf(ctx, "[AppManageService] Failed to cleanup versions for %s/%s: %v", app.User, app.App, err)
		}

	}
}

// getAllApps 获取所有应用
func (s *AppManageService) getAllApps(ctx context.Context) ([]*model.App, error) {
	return s.appRepo.GetAllApps()
}

const (
	// maxKeepVersions 每个应用保留的最大容器版本数
	maxKeepVersions = 3

	// versionShutdownQuietPeriod 是旧版本释放前必须满足的静默期。
	// 宁可晚释放，也不能在 app-server 版本指针、缓存或长请求尚未稳定时释放旧实例。
	versionShutdownQuietPeriod = 10 * time.Minute

	// versionGracefulShutdownTimeout 只限制本轮清理等待 close 通知的时间。
	// 超时后跳过本轮，不强杀，下一轮继续尝试。
	versionGracefulShutdownTimeout = 45 * time.Second
)

// appContainerInfo 单个应用的容器信息（用于按版本排序清理）
type appContainerInfo struct {
	containerName string
	version       string
	versionNum    int // 从 "v1","v2"... 解析出的数字，用于排序
	exited        bool
}

// maybeRunContainerLevelCleanup 仅在有变动时执行一次完整清理（进程级+容器级）
func (s *AppManageService) maybeRunContainerLevelCleanup(ctx context.Context) {
	s.containerCleanupMu.Lock()
	dirty := s.containerCleanupDirty
	if dirty {
		s.containerCleanupDirty = false
	}
	s.containerCleanupMu.Unlock()

	if !dirty {
		return
	}

	logger.Infof(ctx, "[CleanupTask] 检测到版本/容器变动，执行一次完整清理（进程级→容器级→二进制）")
	s.runAllCleanups(ctx)
}

// MarkContainerCleanupDirty 标记“有容器/版本变动”，下次巡检周期会执行一次对账
func (s *AppManageService) MarkContainerCleanupDirty() {
	s.containerCleanupMu.Lock()
	defer s.containerCleanupMu.Unlock()
	s.containerCleanupDirty = true
}

func (s *AppManageService) isVersionQuietForCleanup(ctx context.Context, user, app, version, logPrefix string) bool {
	if s.QPSTracker == nil {
		logger.Warnf(ctx, "[%s] QPS tracker unavailable, skip cleanup for %s/%s/%s", logPrefix, user, app, version)
		return false
	}
	s.QPSTracker.ObserveVersion(user, app, version)
	if !s.QPSTracker.IsIdleFor(user, app, version, versionShutdownQuietPeriod) {
		logger.Infof(ctx, "[%s] Version is not quiet long enough, skip cleanup: %s/%s/%s quietPeriod=%s",
			logPrefix, user, app, version, versionShutdownQuietPeriod)
		s.MarkContainerCleanupDirty()
		return false
	}
	return true
}

func (s *AppManageService) shutdownVersionGracefullyForCleanup(ctx context.Context, user, app, version, logPrefix string) error {
	closeWaiterChan := s.registerCloseWaiter(user, app, version)
	defer s.unregisterCloseWaiter(user, app, version)

	if err := s.ShutdownAppVersion(ctx, user, app, version); err != nil {
		s.MarkContainerCleanupDirty()
		return fmt.Errorf("send shutdown command: %w", err)
	}

	select {
	case notification := <-closeWaiterChan:
		logger.Infof(ctx, "[%s] Received close notification for %s/%s/%s at %s",
			logPrefix, notification.User, notification.App, notification.Version, notification.CloseTime.Format(time.DateTime))
		s.MarkContainerCleanupDirty()
		return nil
	case <-time.After(versionGracefulShutdownTimeout):
		s.MarkContainerCleanupDirty()
		return fmt.Errorf("timeout waiting for close notification after %s", versionGracefulShutdownTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// containerLevelCleanup 运行时实例级对账巡检
// 策略：仅处理 app 表中已注册的应用（runtime 构建的），每个应用保留最近 maxKeepVersions 个版本，更老的全部清理。
// 非 runtime 构建的实例（未在 app 表注册的）一律不碰，保证基础设施等安全。
func (s *AppManageService) containerLevelCleanup(ctx context.Context) {
	if s.runtimeDriver == nil || !s.runtimeDriver.IsAvailable() {
		logger.Debugf(ctx, "[ContainerCleanup] 跳过巡检: runtimeDriver 不可用或未运行")
		return
	}

	cleanupStart := time.Now()

	// 1. 获取 app 表中已注册的应用，只清理这些应用的多余容器
	registeredApps, err := s.appRepo.GetAllApps()
	if err != nil {
		logger.Warnf(ctx, "[ContainerCleanup] 获取已注册应用列表失败: %v，跳过本次巡检", err)
		return
	}
	registeredAppKeys := make(map[string]bool)
	for _, a := range registeredApps {
		registeredAppKeys[a.User+"/"+a.App] = true
	}

	runtimeInstances, err := s.runtimeDriver.ListAppVersions(ctx)
	if err != nil {
		logger.Warnf(ctx, "[ContainerCleanup] 获取运行时实例列表失败: %v", err)
		return
	}

	// 2. 按 "user/app" 分组收集应用运行时实例，且仅收集在 app 表中已注册的应用
	appContainers := make(map[string][]appContainerInfo) // key: "user/app"
	unregisteredCount := 0

	for _, instance := range runtimeInstances {
		appKey := instance.Ref.AppKey()
		if !registeredAppKeys[appKey] {
			unregisteredCount++
			continue
		}

		vNum := parseVersionNumber(instance.Ref.Version)
		appContainers[appKey] = append(appContainers[appKey], appContainerInfo{
			containerName: instance.RuntimeName,
			version:       instance.Ref.Version,
			versionNum:    vNum,
			exited:        !instance.Running,
		})
	}

	totalAppContainers := 0
	for _, cs := range appContainers {
		totalAppContainers += len(cs)
	}

	logger.Infof(ctx, "[ContainerCleanup] 开始巡检 | 总应用运行时实例=%d | 已注册应用实例=%d（%d个应用）| 未注册=%d | 保留策略=每应用最近%d版本",
		len(runtimeInstances), totalAppContainers, len(appContainers), unregisteredCount, maxKeepVersions)

	var cleanedExited, cleanedRunning, skippedTraffic, failedClean int

	for appKey, containers := range appContainers {
		if len(containers) <= maxKeepVersions {
			continue
		}

		sortContainersByVersion(containers)

		// 日志：列出该应用所有版本
		kept := containers[:maxKeepVersions]
		toRemove := containers[maxKeepVersions:]

		keptVersions := make([]string, len(kept))
		for i, c := range kept {
			status := "运行中"
			if c.exited {
				status = "已停止"
			}
			keptVersions[i] = fmt.Sprintf("%s(%s)", c.version, status)
		}
		removeVersions := make([]string, len(toRemove))
		for i, c := range toRemove {
			status := "运行中"
			if c.exited {
				status = "已停止"
			}
			removeVersions[i] = fmt.Sprintf("%s(%s)", c.version, status)
		}

		logger.Infof(ctx, "[ContainerCleanup] 应用 %s | 共%d个版本 | 保留=%v | 待清理=%v",
			appKey, len(containers), keptVersions, removeVersions)

		parts := strings.SplitN(appKey, "/", 2)
		user, app := parts[0], parts[1]

		for _, info := range toRemove {
			if info.exited {
				removeStart := time.Now()
				ref := AppVersionRef{User: user, App: app, Version: info.version}
				if rmErr := s.runtimeDriver.RemoveAppVersion(ctx, ref); rmErr != nil {
					logger.Warnf(ctx, "[ContainerCleanup] ❌ 删除已停止容器失败 | 容器=%s | 错误=%v", info.containerName, rmErr)
					failedClean++
				} else {
					logger.Infof(ctx, "[ContainerCleanup] ✅ 已删除停止容器 | 容器=%s | 版本=%s | 耗时=%s",
						info.containerName, info.version, time.Since(removeStart).Round(time.Millisecond))
					cleanedExited++
				}
			} else {
				if s.isVersionQuietForCleanup(ctx, user, app, info.version, "ContainerCleanup") {
					stopStart := time.Now()
					logger.Infof(ctx, "[ContainerCleanup] 请求旧容器优雅退出 | 容器=%s | 版本=%s（静默期=%s）",
						info.containerName, info.version, versionShutdownQuietPeriod)
					if err := s.shutdownVersionGracefullyForCleanup(ctx, user, app, info.version, "ContainerCleanup"); err != nil {
						logger.Warnf(ctx, "[ContainerCleanup] ⏭ 本轮跳过运行中容器 | 容器=%s | 版本=%s | 原因=%v",
							info.containerName, info.version, err)
						failedClean++
					} else {
						logger.Infof(ctx, "[ContainerCleanup] ✅ 旧容器已优雅退出，后续巡检删除已停止实例 | 容器=%s | 版本=%s | 耗时=%s",
							info.containerName, info.version, time.Since(stopStart).Round(time.Millisecond))
						cleanedRunning++
					}
				} else {
					logger.Infof(ctx, "[ContainerCleanup] ⏭ 跳过运行中容器 | 容器=%s | 版本=%s | 原因=未满足静默期",
						info.containerName, info.version)
					skippedTraffic++
				}
			}
		}
	}

	totalCleaned := cleanedExited + cleanedRunning
	logger.Infof(ctx, "[ContainerCleanup] 巡检完成 | 耗时=%s | 清理=%d（已停止=%d + 运行中=%d）| 跳过=%d（有流量）| 失败=%d",
		time.Since(cleanupStart).Round(time.Millisecond), totalCleaned, cleanedExited, cleanedRunning, skippedTraffic, failedClean)
}

// parseVersionNumber 从 "v1","v2","v10" 等版本字符串中提取数字部分
func parseVersionNumber(version string) int {
	v := strings.TrimPrefix(version, "v")
	num := 0
	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			num = num*10 + int(ch-'0')
		} else {
			break
		}
	}
	return num
}

// sortContainersByVersion 按版本号降序排列（最新版本在前面）
func sortContainersByVersion(containers []appContainerInfo) {
	for i := 1; i < len(containers); i++ {
		for j := i; j > 0 && containers[j].versionNum > containers[j-1].versionNum; j-- {
			containers[j], containers[j-1] = containers[j-1], containers[j]
		}
	}
}

// CleanupNonCurrentVersions 清理非当前版本的静默版本。
// 策略：只保留 current_version（metadata 中的当前版本），其他版本满足完整静默期后才发起优雅关闭。
func (s *AppManageService) CleanupNonCurrentVersions(ctx context.Context, user, app string) error {
	//logger.Infof(ctx, "[CleanupNonCurrentVersions] Checking %s/%s", user, app)

	// 1. 读取 current_version
	currentVersion, err := s.getCurrentVersion(ctx, user, app)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion == "" {
		//logger.Warnf(ctx, "[CleanupNonCurrentVersions] No current version found for %s/%s", user, app)
		return nil
	}

	//logger.Infof(ctx, "[CleanupNonCurrentVersions] Current version: %s", currentVersion)

	// 2. 从内存中获取所有运行中的版本
	runningApps := s.appDiscoveryService.GetRunningApps()
	appKey := user + "/" + app
	appInfo, exists := runningApps[appKey]
	if !exists {
		//logger.Infof(ctx, "[CleanupNonCurrentVersions] No running versions found for %s/%s", user, app)
		return nil
	}

	// 3. 关闭非当前版本且无流量的版本
	for _, version := range appInfo.Versions {
		// 跳过当前版本
		if version.Version == currentVersion {
			//logger.Infof(ctx, "[CleanupNonCurrentVersions] Skipping current version: %s", version.Version)
			continue
		}

		if !s.isVersionQuietForCleanup(ctx, user, app, version.Version, "CleanupNonCurrentVersions") {
			continue
		}

		if err := s.shutdownVersionGracefullyForCleanup(ctx, user, app, version.Version, "CleanupNonCurrentVersions"); err != nil {
			logger.Warnf(ctx, "[CleanupNonCurrentVersions] 本轮跳过非当前版本 | %s/%s/%s | 原因=%v", user, app, version.Version, err)
			continue
		}
	}

	return nil
}

// getCurrentVersion 获取应用的当前版本（从 metadata/current_version.txt）
func (s *AppManageService) getCurrentVersion(ctx context.Context, user, app string) (string, error) {
	return s.readCurrentVersion(user, app)
}

// StartAppVersion 启动指定版本的应用（兜底启动）
// 用于应用挂了或更新失败时重新启动目标版本
// 新架构：每个版本有独立容器，直接创建或启动版本容器
func (s *AppManageService) StartAppVersion(ctx context.Context, user, app, version string) error {
	logger.Infof(ctx, "[StartAppVersion] Starting version %s/%s/%s", user, app, version)

	// 先检查应用是否已经在运行（避免重复启动）
	if s.appDiscoveryService != nil {
		if s.appDiscoveryService.IsAppVersionRunning(user, app, version) {
			logger.Infof(ctx, "[StartAppVersion] Version %s/%s/%s is already running, skipping startup", user, app, version)
			return nil
		}
	}

	ref := AppVersionRef{User: user, App: app, Version: version}
	runtimeName := ref.RuntimeName()

	// 注册启动等待器（统一在外层注册）
	waiterChan := s.registerStartupWaiter(user, app, version)
	// 确保在方法结束时清理等待器
	defer s.unregisterStartupWaiter(user, app, version)

	if s.runtimeDriver == nil {
		return fmt.Errorf("app runtime driver not available")
	}

	// 检查运行时实例是否存在且运行中
	exists, err := s.runtimeDriver.IsAppVersionRunning(ctx, ref)
	if err != nil {
		logger.Warnf(ctx, "[StartAppVersion] Failed to check runtime status: %v, will try to start", err)
		exists = false
	}

	if exists {
		// 运行时实例已存在且运行中，应用应该已经启动，等待启动通知
		logger.Infof(ctx, "[StartAppVersion] Runtime instance %s already exists and is running, waiting for startup notification", runtimeName)
	} else {
		// 运行时实例不存在或已停止，需要创建或启动实例
		if err := s.EnsureAppVersionRuntimeRunning(ctx, user, app, version); err != nil {
			return err
		}
		logger.Infof(ctx, "[StartAppVersion] Runtime instance %s started successfully", runtimeName)
	}

	startupTimeout := s.appStartupNotificationTimeout()
	logger.Infof(ctx, "[StartAppVersion] Waiting for startup notification from version %s (timeout: %s)...", version, startupTimeout)

	notification, err := s.waitForStartupNotificationOrRuntimeExit(ctx, ref, waiterChan, startupTimeout)
	if err != nil {
		logger.Warnf(ctx, "[StartAppVersion] Failed waiting for startup notification from version %s after %s: %v", version, startupTimeout, err)
		return err
	}
	logger.Infof(ctx, "[StartAppVersion] Received startup notification: %s/%s/%s, status=%s",
		notification.User, notification.App, notification.Version, notification.Status)

	if notification.Status == "running" {
		logger.Infof(ctx, "[StartAppVersion] Version %s started successfully", version)
		return nil
	}
	if notification.Error != "" {
		return fmt.Errorf("app startup failed: %s", notification.Error)
	}
	return fmt.Errorf("app started but status is not running: %s", notification.Status)
}

func (s *AppManageService) appStartupNotificationTimeout() time.Duration {
	if s.runtimeConfig == nil {
		return time.Duration((&appconfig.AppRuntimeConfig{}).GetAppStartupNotificationTimeout()) * time.Second
	}
	return time.Duration(s.runtimeConfig.GetAppStartupNotificationTimeout()) * time.Second
}

func (s *AppManageService) updateCallbackTimeout() time.Duration {
	if s.runtimeConfig == nil {
		return time.Duration((&appconfig.AppRuntimeConfig{}).GetUpdateCallbackTimeout()) * time.Second
	}
	return time.Duration(s.runtimeConfig.GetUpdateCallbackTimeout()) * time.Second
}

type lineRange struct {
	Start int
	End   int
}

// ReadAppLog 读取应用版本日志（支持 tail 和关键词检索）
func (s *AppManageService) ReadAppLog(ctx context.Context, req *sharedDto.ReadAppLogRuntimeReq) (*sharedDto.ReadAppLogRuntimeResp, error) {
	lines := req.Lines
	if lines <= 0 {
		lines = 200
	}
	if lines > 1000 {
		lines = 1000
	}
	contextLines := req.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines == 0 {
		contextLines = 2
	}
	if contextLines > 5 {
		contextLines = 5
	}
	maxMatches := req.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 50
	}
	if maxMatches > 200 {
		maxMatches = 200
	}

	version := strings.TrimSpace(req.Version)
	if version == "" {
		currentVersion, err := s.getCurrentVersion(ctx, req.User, req.App)
		if err != nil {
			return nil, fmt.Errorf("读取当前版本失败: %w", err)
		}
		if strings.TrimSpace(currentVersion) == "" {
			return nil, fmt.Errorf("当前版本为空，无法定位日志文件")
		}
		version = strings.TrimSpace(currentVersion)
	}

	appPaths := newRuntimeAppPaths(s.config.GetBasePath(), req.User, req.App)
	logFileName := appPaths.LogFileName(version)
	logFilePath := appPaths.LogFile(version)

	f, err := os.Open(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("日志文件不存在: %s", logFileName)
		}
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer f.Close()

	allLines := make([]string, 0, 1024)
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取日志文件失败: %w", err)
	}
	totalLines := len(allLines)

	resp := &sharedDto.ReadAppLogRuntimeResp{
		Success:         true,
		Message:         "读取成功",
		ResolvedVersion: version,
		LogFile:         logFileName,
		TotalLines:      totalLines,
	}
	if totalLines == 0 {
		resp.Message = "日志为空"
		return resp, nil
	}

	keyword := req.Keyword
	if strings.TrimSpace(keyword) == "" {
		start := totalLines - lines
		if start < 0 {
			start = 0
		}
		out := allLines[start:totalLines]
		resp.ReturnedLines = len(out)
		resp.Truncated = start > 0
		resp.Content = strings.Join(out, "\n")
		return resp, nil
	}

	matchRanges := make([]lineRange, 0, maxMatches)
	matchCount := 0
	needle := keyword
	if req.IgnoreCase {
		needle = strings.ToLower(needle)
	}
	for i, line := range allLines {
		hay := line
		if req.IgnoreCase {
			hay = strings.ToLower(hay)
		}
		if strings.Contains(hay, needle) {
			matchCount++
			if len(matchRanges) < maxMatches {
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				end := i + contextLines
				if end >= totalLines {
					end = totalLines - 1
				}
				matchRanges = append(matchRanges, lineRange{Start: start, End: end})
			}
		}
	}
	resp.MatchCount = matchCount
	if len(matchRanges) == 0 {
		resp.Message = "未匹配到关键词"
		return resp, nil
	}

	merged := mergeLineRanges(matchRanges)
	result := make([]string, 0, lines)
	for _, rg := range merged {
		for i := rg.Start; i <= rg.End; i++ {
			if len(result) >= lines {
				resp.Truncated = true
				break
			}
			result = append(result, fmt.Sprintf("%d|%s", i+1, allLines[i]))
		}
		if resp.Truncated {
			break
		}
	}
	if matchCount > maxMatches {
		resp.Truncated = true
	}
	resp.ReturnedLines = len(result)
	resp.Content = strings.Join(result, "\n")
	return resp, nil
}

func mergeLineRanges(ranges []lineRange) []lineRange {
	if len(ranges) == 0 {
		return nil
	}
	merged := make([]lineRange, 0, len(ranges))
	current := ranges[0]
	for i := 1; i < len(ranges); i++ {
		r := ranges[i]
		if r.Start <= current.End+1 {
			if r.End > current.End {
				current.End = r.End
			}
			continue
		}
		merged = append(merged, current)
		current = r
	}
	merged = append(merged, current)
	return merged
}

// GitCommitMessage Git 提交消息结构体
type GitCommitMessage struct {
	AppVersion        string `json:"app_version"`        // 应用版本号
	Requirement       string `json:"requirement"`        // 变更需求
	ChangeDescription string `json:"change_description"` // 变更描述
	Summary           string `json:"summary"`            // 变更摘要
	Timestamp         string `json:"timestamp"`          // 时间戳
}

// commitToGit 提交代码到 Git，返回 commit hash
func (s *AppManageService) commitToGit(
	ctx context.Context,
	user, app, version string,
	requirement, changeDescription string,
) (string, error) {
	// 1. 获取应用代码目录
	appCodeDir := newRuntimeAppPaths(s.config.GetBasePath(), user, app).APIDir()

	// 2. 从 ctx 获取用户名称
	authorName := contextx.GetRequestUser(ctx)
	if authorName == "" {
		authorName = user // 如果 ctx 中没有用户信息，使用 user 参数
	}

	// 3. 获取邮箱后缀（从配置读取）
	emailSuffix := s.config.GetGitEmailSuffix()
	if emailSuffix == "" {
		emailSuffix = "kageos.ai" // 默认后缀
	}

	// 4. 构建邮箱：{user}@{email_suffix}
	if authorName == "" || authorName == "system" {
		authorName = "system"
	}
	authorEmail := fmt.Sprintf("%s@%s", authorName, emailSuffix)

	// 4. 初始化或打开 Git 仓库
	gitRepo, err := gitx.InitOrOpen(appCodeDir, authorName, authorEmail)
	if err != nil {
		return "", fmt.Errorf("初始化 Git 仓库失败: %w", err)
	}

	// 5. 构建 commit message（JSON 格式）
	commitMsg := GitCommitMessage{
		AppVersion:        version,
		Requirement:       requirement,
		ChangeDescription: changeDescription,
		Timestamp:         time.Now().Format(time.RFC3339),
	}

	// 构建 summary
	if requirement != "" && changeDescription != "" {
		commitMsg.Summary = fmt.Sprintf("需求：%s\n\n变更描述：%s", requirement, changeDescription)
	} else if requirement != "" {
		commitMsg.Summary = requirement
	} else if changeDescription != "" {
		commitMsg.Summary = changeDescription
	}

	commitJSON, err := json.Marshal(commitMsg)
	if err != nil {
		return "", fmt.Errorf("序列化 commit message 失败: %w", err)
	}

	// 6. 添加所有文件并提交
	commitHash, err := gitRepo.AddAllAndCommit(string(commitJSON))
	if err != nil {
		return "", fmt.Errorf("Git 提交失败: %w", err)
	}

	logger.Infof(ctx, "[commitToGit] Git 提交成功: user=%s, app=%s, version=%s, commitHash=%s",
		user, app, version, commitHash)

	return commitHash, nil
}
