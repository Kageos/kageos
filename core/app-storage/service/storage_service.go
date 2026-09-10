package service

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kageos/kageos/core/app-storage/model"
	"github.com/kageos/kageos/core/app-storage/repository"
	"github.com/kageos/kageos/core/app-storage/storage"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/config"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/logger"
)

// 导入常量以便使用
const (
	BytesPerMB = storage.BytesPerMB
)

// StorageService 存储服务
type StorageService struct {
	storage  storage.Storage // 依赖抽象接口，不依赖具体实现
	cfg      *config.AppStorageConfig
	fileRepo *repository.FileRepository
}

// NewStorageService 创建存储服务
func NewStorageService(storage storage.Storage, cfg *config.AppStorageConfig, fileRepo *repository.FileRepository) *StorageService {
	return &StorageService{
		storage:  storage,
		cfg:      cfg,
		fileRepo: fileRepo,
	}
}

// GenerateUploadToken 生成上传凭证
// uploadSource: 上传来源（browser 或 server），默认为 browser
func (s *StorageService) GenerateUploadToken(ctx context.Context, bucket string, router string, fileName string, contentType string, fileSize int64, uploadSource string) (creds *storage.UploadCredentials, key string, expire time.Time, err error) {
	// 生成唯一的文件 Key（包含函数路径）
	key = s.generateFileKey(router, fileName)
	return s.generateUploadTokenForKey(ctx, bucket, key, fileName, contentType, fileSize, uploadSource)
}

// GeneratePreviewUploadToken 为前端生成的缩略图/视频封面创建上传凭证。
// 缩略图 key 与原文件 key 保持稳定派生关系：foo.png -> foo.png.thumb.webp。
func (s *StorageService) GeneratePreviewUploadToken(ctx context.Context, bucket string, sourceKey string, previewFileName string, contentType string, fileSize int64, uploadSource string) (creds *storage.UploadCredentials, key string, expire time.Time, err error) {
	key, err = s.BuildPreviewFileKey(sourceKey, previewFileName)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return s.generateUploadTokenForKey(ctx, bucket, key, previewFileName, contentType, fileSize, uploadSource)
}

func (s *StorageService) generateUploadTokenForKey(ctx context.Context, bucket string, key string, fileName string, contentType string, fileSize int64, uploadSource string) (creds *storage.UploadCredentials, normalizedKey string, expire time.Time, err error) {
	// 校验文件大小
	if fileSize > s.cfg.Storage.Upload.MaxSize {
		return nil, "", time.Time{}, fmt.Errorf("文件大小超过限制（最大 %d MB）", s.cfg.Storage.Upload.MaxSize/BytesPerMB)
	}

	normalizedKey = normalizeObjectKey(key)
	if isUnsafeObjectKey(normalizedKey) {
		return nil, "", time.Time{}, fmt.Errorf("文件路径不合法")
	}

	bucket = s.normalizeBucket(bucket)
	expiry := time.Duration(s.cfg.Storage.Upload.TokenExpire) * time.Second

	// 通过存储接口生成上传凭证（统一接口，所有存储引擎都必须实现）
	creds, err = s.storage.GenerateUploadCredentials(ctx, bucket, normalizedKey, contentType, expiry, uploadSource)

	if err != nil {
		logger.Errorf(ctx, "Failed to generate upload credentials: %v", err)
		return nil, "", time.Time{}, fmt.Errorf("生成上传凭证失败")
	}

	expire = time.Now().Add(expiry)
	logger.Infof(ctx, "Generated upload credentials: method=%s, source=%s, key_len=%d", creds.Method, uploadSource, len(normalizedKey))

	return creds, normalizedKey, expire, nil
}

func (s *StorageService) BuildFileRef(bucket string, key string) string {
	bucket = s.normalizeBucket(bucket)
	key = normalizeObjectKey(key)
	if bucket == "" {
		return key
	}
	return bucket + "/" + key
}

func (s *StorageService) ParseFileRef(ref string) (bucket string, key string, err error) {
	ref = normalizeObjectKey(ref)
	if ref == "" {
		return "", "", fmt.Errorf("文件引用不能为空")
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("文件引用必须是 bucket/object_key 格式")
	}
	return s.normalizeBucket(parts[0]), normalizeObjectKey(parts[1]), nil
}

// GetFileURLs 获取文件访问 URL（同时返回外部和内部访问的URL）
func (s *StorageService) GetFileURLs(ctx context.Context, key string) (externalURL string, serverURL string, expire time.Time, err error) {
	return s.GetFileURLsInBucket(ctx, "", key)
}

func (s *StorageService) GetFileURLsInBucket(ctx context.Context, bucket string, key string) (externalURL string, serverURL string, expire time.Time, err error) {
	// 生成下载 URL（使用默认过期时间）
	bucket = s.normalizeBucket(bucket)
	expiry := storage.DefaultDownloadURLExpiry

	// 注意：MinIO/S3 不支持 response-cache-control 和 response-expires 作为查询参数
	// 这些响应头应该在存储对象时通过元数据设置，或者在代理层添加
	// 暂时不添加查询参数，确保预签名 URL 能正常工作
	cacheControl := make(map[string]string)
	// TODO: 未来可以通过对象元数据或在代理层设置 Cache-Control 响应头

	// 通过存储接口生成下载URL（统一接口，所有存储引擎都必须实现）
	externalURL, serverURL, err = s.storage.GenerateDownloadURLs(ctx, bucket, key, expiry, cacheControl)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate download URLs: %v", err)
		return "", "", time.Time{}, fmt.Errorf("生成下载链接失败")
	}

	expire = time.Now().Add(expiry)
	logger.Infof(ctx, "Generated download URLs: key_len=%d, external_url_len=%d, server_url_len=%d", len(key), len(externalURL), len(serverURL))
	return externalURL, serverURL, expire, nil
}

func (s *StorageService) ResolveFileRefs(ctx context.Context, refs []string, audience string) ([]dto.ResolvedFile, error) {
	out := make([]dto.ResolvedFile, 0, len(refs))
	for _, rawRef := range refs {
		bucket, key, err := s.ParseFileRef(rawRef)
		item := dto.ResolvedFile{
			Ref:    rawRef,
			Bucket: bucket,
			Key:    key,
		}
		if err != nil {
			item.Error = err.Error()
			out = append(out, item)
			continue
		}
		item.Ref = s.BuildFileRef(bucket, key)

		if s.fileRepo != nil {
			if record, e := s.fileRepo.GetUploadRecordByBucketKey(ctx, bucket, key); e == nil && record != nil {
				item.Name = record.FileName
				item.SourceName = record.FileName
				item.Description = record.Description
				item.Size = record.FileSize
				item.ContentType = record.ContentType
				item.Hash = record.Hash
				item.UploadUser = record.Username
				item.UploadTs = record.UploadedAt.UnixMilli()
				item.ThumbnailRef = record.ThumbnailRef
				item.PreviewKind = record.PreviewKind
				item.Storage = s.GetStorageType()
				item.Status = record.Status
				item.DeletedBy = record.DeletedBy
				if record.DeletedAt != nil {
					item.DeletedAt = record.DeletedAt.UnixMilli()
				}
				actor := strings.TrimSpace(contextx.GetRequestUser(ctx))
				item.CanDelete = (record.Status == "completed" || record.Status == "delete_failed") &&
					(actor == "system" || actor == record.Username)
				if record.Status == "deleted" || record.Status == "deleting" {
					out = append(out, item)
					continue
				}
			}
		}

		info, infoErr := s.storage.GetObjectInfo(ctx, bucket, key)
		if infoErr == nil && info != nil {
			if item.Name == "" {
				item.Name = filepath.Base(key)
			}
			if item.SourceName == "" {
				item.SourceName = item.Name
			}
			if item.Size == 0 {
				item.Size = info.Size
			}
			if item.ContentType == "" {
				item.ContentType = info.ContentType
			}
			if item.Hash == "" {
				item.Hash = strings.Trim(info.ETag, "\"")
			}
		} else if item.Name == "" {
			item.Name = filepath.Base(key)
			item.SourceName = item.Name
		}

		browserURL, serverURL, _, urlErr := s.GetFileURLsInBucket(ctx, bucket, key)
		if urlErr != nil {
			item.Error = urlErr.Error()
		} else {
			switch strings.ToLower(strings.TrimSpace(audience)) {
			case "browser":
				item.DownloadURL = browserURL
			case "server":
				item.ServerDownloadURL = serverURL
			default:
				item.DownloadURL = browserURL
				item.ServerDownloadURL = serverURL
			}
		}
		s.fillThumbnailURLs(ctx, &item, audience)
		out = append(out, item)
	}
	return out, nil
}

func (s *StorageService) fillThumbnailURLs(ctx context.Context, item *dto.ResolvedFile, audience string) {
	if item == nil || strings.TrimSpace(item.ThumbnailRef) == "" {
		return
	}
	bucket, key, err := s.ParseFileRef(item.ThumbnailRef)
	if err != nil {
		logger.Warnf(ctx, "Failed to parse thumbnail ref %q: %v", item.ThumbnailRef, err)
		return
	}
	browserURL, serverURL, _, err := s.GetFileURLsInBucket(ctx, bucket, key)
	if err != nil {
		logger.Warnf(ctx, "Failed to generate thumbnail URL for ref %q: %v", item.ThumbnailRef, err)
		return
	}
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case "browser":
		item.ThumbnailURL = browserURL
	case "server":
		item.ServerThumbnailURL = serverURL
	default:
		item.ThumbnailURL = browserURL
		item.ServerThumbnailURL = serverURL
	}
}

func (s *StorageService) UpdateFileDescription(ctx context.Context, ref string, bucket string, key string, description string) (*dto.UpdateFileDescriptionResp, error) {
	if ref != "" {
		parsedBucket, parsedKey, err := s.ParseFileRef(ref)
		if err != nil {
			return nil, err
		}
		bucket = parsedBucket
		key = parsedKey
	} else {
		bucket = s.normalizeBucket(bucket)
		key = normalizeObjectKey(key)
	}
	if bucket == "" || key == "" {
		return nil, fmt.Errorf("文件引用不能为空")
	}
	if s.fileRepo == nil {
		return nil, fmt.Errorf("文件元数据服务未初始化")
	}
	if err := s.fileRepo.UpdateDescriptionByBucketKey(ctx, bucket, key, description); err != nil {
		return nil, fmt.Errorf("更新文件描述失败: %w", err)
	}
	return &dto.UpdateFileDescriptionResp{
		Ref:         s.BuildFileRef(bucket, key),
		Bucket:      bucket,
		Key:         key,
		Description: description,
	}, nil
}

// DeleteFile 删除文件
func (s *StorageService) DeleteFile(ctx context.Context, key string) error {
	ref := s.BuildFileRef(s.getDefaultBucket(), key)
	_, err := s.DeleteOwnedFile(ctx, ref, contextx.GetRequestUser(ctx))
	return err
}

// DeleteOwnedFile 删除当前用户拥有的物理对象，同时保留元数据墓碑供历史记录展示。
func (s *StorageService) DeleteOwnedFile(ctx context.Context, ref string, actor string) (*dto.DeleteFileRefResult, error) {
	bucket, key, err := s.ParseFileRef(ref)
	if err != nil {
		return nil, err
	}
	ref = s.BuildFileRef(bucket, key)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("无法确认当前用户")
	}
	if s.fileRepo == nil {
		return nil, fmt.Errorf("文件元数据服务未初始化，无法安全删除")
	}

	record, err := s.fileRepo.GetUploadRecordByBucketKey(ctx, bucket, key)
	if err != nil || record == nil {
		return nil, fmt.Errorf("文件记录不存在，无法安全删除")
	}
	if actor != "system" && actor != record.Username {
		return nil, fmt.Errorf("只能删除自己上传的文件")
	}
	if record.Status == "deleted" {
		result := &dto.DeleteFileRefResult{Ref: ref, Status: "already_deleted", ReleasedBytes: record.FileSize, DeletedBy: record.DeletedBy}
		if record.DeletedAt != nil {
			result.DeletedAt = record.DeletedAt.UnixMilli()
		}
		return result, nil
	}
	if record.Status != "completed" && record.Status != "delete_failed" {
		return nil, fmt.Errorf("文件当前状态为 %s，暂时不能删除", record.Status)
	}

	if err := s.fileRepo.UpdateDeletionState(ctx, bucket, key, "deleting", actor, nil, ""); err != nil {
		return nil, fmt.Errorf("更新文件删除状态失败")
	}

	releasedBytes := record.FileSize
	if strings.TrimSpace(record.ThumbnailRef) != "" {
		thumbnailBucket, thumbnailKey, parseErr := s.ParseFileRef(record.ThumbnailRef)
		if parseErr != nil {
			s.recordDeleteFailure(ctx, bucket, key, actor, parseErr)
			return nil, fmt.Errorf("缩略图引用不合法，文件清理未完整完成")
		}
		if thumbnailRecord, getErr := s.fileRepo.GetUploadRecordByBucketKey(ctx, thumbnailBucket, thumbnailKey); getErr == nil && thumbnailRecord != nil {
			releasedBytes += thumbnailRecord.FileSize
		}
		_ = s.fileRepo.UpdateDeletionState(ctx, thumbnailBucket, thumbnailKey, "deleting", actor, nil, "")
		if err := s.storage.DeleteObject(ctx, thumbnailBucket, thumbnailKey); err != nil {
			s.recordDeleteFailure(ctx, bucket, key, actor, err)
			return nil, fmt.Errorf("缩略图删除失败，文件清理未完整完成")
		}
	}
	if err := s.storage.DeleteObject(ctx, bucket, key); err != nil {
		s.recordDeleteFailure(ctx, bucket, key, actor, err)
		return nil, fmt.Errorf("删除文件失败")
	}

	deletedAt := time.Now()
	if err := s.fileRepo.UpdateDeletionState(ctx, bucket, key, "deleted", actor, &deletedAt, ""); err != nil {
		return nil, fmt.Errorf("文件已删除，但墓碑状态保存失败")
	}
	if strings.TrimSpace(record.ThumbnailRef) != "" {
		if thumbnailBucket, thumbnailKey, parseErr := s.ParseFileRef(record.ThumbnailRef); parseErr == nil {
			_ = s.fileRepo.UpdateDeletionState(ctx, thumbnailBucket, thumbnailKey, "deleted", actor, &deletedAt, "")
		}
	}

	logger.Infof(ctx, "Deleted owned file: ref=%s actor=%s released_bytes=%d", ref, actor, releasedBytes)
	return &dto.DeleteFileRefResult{
		Ref:           ref,
		Status:        "deleted",
		ReleasedBytes: releasedBytes,
		DeletedAt:     deletedAt.UnixMilli(),
		DeletedBy:     actor,
	}, nil
}

func (s *StorageService) recordDeleteFailure(ctx context.Context, bucket string, key string, actor string, deleteErr error) {
	if err := s.fileRepo.UpdateDeletionState(ctx, bucket, key, "delete_failed", actor, nil, deleteErr.Error()); err != nil {
		logger.Errorf(ctx, "Failed to persist delete failure: key_len=%d, err=%v", len(key), err)
	}
}

// GetFileInfo 获取文件信息
func (s *StorageService) GetFileInfo(ctx context.Context, key string) (*storage.ObjectInfo, error) {
	bucket := s.getDefaultBucket()
	info, err := s.storage.GetObjectInfo(ctx, bucket, key)
	if err != nil {
		logger.Errorf(ctx, "Failed to get file info for %s: %v", key, err)
		return nil, fmt.Errorf("获取文件信息失败")
	}
	return info, nil
}

// GetBucketName 获取默认 Bucket 名称
func (s *StorageService) GetBucketName() string {
	return s.getDefaultBucket()
}

// GetCDNDomain 获取 CDN 域名
func (s *StorageService) GetCDNDomain() string {
	return s.storage.GetCDNDomain()
}

func (s *StorageService) GetSystemStorageAssets(ctx context.Context, req dto.SystemStorageAssetsReq) (*dto.SystemStorageAssetsResp, error) {
	if s.fileRepo == nil {
		return &dto.SystemStorageAssetsResp{
			List:              []dto.SystemStorageAsset{},
			Directories:       []dto.SystemStorageAssetDirectory{},
			MetadataAvailable: false,
			Coverage:          "tracked_uploads",
		}, nil
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	allowedStatuses := map[string]bool{
		"": true, "all": true, "completed": true, "pending": true, "failed": true,
		"deleting": true, "delete_failed": true, "failed_all": true, "deleted": true,
	}
	if !allowedStatuses[strings.TrimSpace(req.Status)] {
		return nil, fmt.Errorf("不支持的文件状态")
	}

	filter := repository.SystemFileAssetFilter{
		RouterPrefix: req.RouterPrefix,
		Status:       req.Status,
		Keyword:      req.Keyword,
	}
	rows, total, err := s.fileRepo.ListSystemFileAssets(ctx, filter, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	summaryRow, err := s.fileRepo.GetSystemFileAssetSummary(ctx)
	if err != nil {
		return nil, err
	}
	directoryRows, err := s.fileRepo.ListSystemFileAssetDirectories(ctx, 0)
	if err != nil {
		return nil, err
	}

	items := make([]dto.SystemStorageAsset, 0, len(rows))
	for _, row := range rows {
		var lastAccessedAt *time.Time
		if row.LastAccessedAt.Valid {
			value := row.LastAccessedAt.Time
			lastAccessedAt = &value
		}
		previewKind, previewable := systemAssetPreviewKind(row.ContentType, row.FileName, row.PreviewKind)
		thumbnailURL := ""
		if row.Status == "completed" && strings.TrimSpace(row.ThumbnailRef) != "" {
			if thumbnailBucket, thumbnailKey, parseErr := s.ParseFileRef(row.ThumbnailRef); parseErr == nil {
				thumbnailURL, _, _, _ = s.GetFileURLsInBucket(ctx, thumbnailBucket, thumbnailKey)
			}
		} else if row.Status == "completed" && previewKind == "image" {
			thumbnailURL, _, _, _ = s.GetFileURLsInBucket(ctx, row.Bucket, row.FileKey)
		}
		items = append(items, dto.SystemStorageAsset{
			ID: row.ID, Bucket: row.Bucket, Ref: s.BuildFileRef(row.Bucket, row.FileKey), FileKey: row.FileKey,
			Router: row.Router, FileName: row.FileName, Description: row.Description, FileSize: row.FileSize,
			ContentType: row.ContentType, ThumbnailRef: row.ThumbnailRef, ThumbnailURL: thumbnailURL,
			PreviewKind: previewKind, Previewable: previewable, Username: row.Username, Tenant: row.Tenant, Status: row.Status,
			UploadedAt: row.UploadedAt, DeletedAt: row.DeletedAt, DeletedBy: row.DeletedBy,
			DeleteError: row.DeleteError, DownloadCount: row.DownloadCount, PreviewCount: row.PreviewCount, LastAccessedAt: lastAccessedAt,
		})
	}
	directories := make([]dto.SystemStorageAssetDirectory, 0, len(directoryRows))
	workspaceMap := make(map[string]*dto.SystemStorageAssetWorkspace)
	for _, row := range directoryRows {
		if len(directories) < 200 {
			directories = append(directories, dto.SystemStorageAssetDirectory{Router: row.Router, FileCount: row.FileCount, SizeBytes: row.SizeBytes})
		}
		workspacePath := storageWorkspacePath(row.Router)
		if workspacePath == "" {
			continue
		}
		workspace := workspaceMap[workspacePath]
		if workspace == nil {
			workspace = &dto.SystemStorageAssetWorkspace{Path: workspacePath}
			workspaceMap[workspacePath] = workspace
		}
		workspace.FileCount += row.FileCount
		workspace.SizeBytes += row.SizeBytes
	}
	workspaces := make([]dto.SystemStorageAssetWorkspace, 0, len(workspaceMap))
	for _, workspace := range workspaceMap {
		workspaces = append(workspaces, *workspace)
	}
	sort.SliceStable(workspaces, func(i, j int) bool {
		if workspaces[i].SizeBytes != workspaces[j].SizeBytes {
			return workspaces[i].SizeBytes > workspaces[j].SizeBytes
		}
		return workspaces[i].Path < workspaces[j].Path
	})

	return &dto.SystemStorageAssetsResp{
		List: items, Total: total, Page: page, PageSize: pageSize,
		Summary: dto.SystemStorageAssetSummary{
			ActiveFiles: summaryRow.ActiveFiles, ActiveBytes: summaryRow.ActiveBytes,
			DeletedFiles: summaryRow.DeletedFiles, FailedFiles: summaryRow.FailedFiles,
		},
		Directories: directories, Workspaces: workspaces, MetadataAvailable: true,
		Coverage: "tracked_uploads",
	}, nil
}

func storageWorkspacePath(router string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(router), "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func systemAssetPreviewKind(contentType string, fileName string, recorded string) (string, bool) {
	if value := strings.ToLower(strings.TrimSpace(recorded)); value == "image" || value == "video" {
		return value, true
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	ext := strings.ToLower(filepath.Ext(fileName))
	if strings.HasPrefix(contentType, "image/") || slices.Contains([]string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg"}, ext) {
		return "image", true
	}
	if strings.HasPrefix(contentType, "video/") || slices.Contains([]string{".mp4", ".webm", ".mov", ".m4v"}, ext) {
		return "video", true
	}
	if contentType == "application/pdf" || ext == ".pdf" {
		return "pdf", true
	}
	return "", false
}

func (s *StorageService) GetSystemAssetDownloadURL(ctx context.Context, ref string) (string, error) {
	if s.fileRepo == nil {
		return "", fmt.Errorf("文件元数据服务未初始化")
	}
	bucket, key, err := s.ParseFileRef(ref)
	if err != nil {
		return "", err
	}
	record, err := s.fileRepo.GetUploadRecordByBucketKey(ctx, bucket, key)
	if err != nil || record == nil || record.Status != "completed" {
		return "", fmt.Errorf("文件不存在或当前不可下载")
	}
	externalURL, _, _, err := s.GetFileURLsInBucket(ctx, bucket, key)
	if err != nil || externalURL == "" {
		return "", fmt.Errorf("生成下载链接失败")
	}
	return externalURL, nil
}

func (s *StorageService) GetSystemAssetAudits(ctx context.Context, ref string, limit int) ([]dto.SystemStorageAssetAudit, error) {
	if s.fileRepo == nil {
		return nil, fmt.Errorf("文件元数据服务未初始化")
	}
	_, key, err := s.ParseFileRef(ref)
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.fileRepo.ListFileAccessAudits(ctx, key, limit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.SystemStorageAssetAudit, 0, len(rows))
	for _, row := range rows {
		username := ""
		if row.Username != nil {
			username = *row.Username
		}
		action := strings.TrimSpace(row.Action)
		if action == "" {
			action = "download"
		}
		items = append(items, dto.SystemStorageAssetAudit{
			ID: row.ID, Action: action, Username: username, IPAddress: row.IPAddress,
			UserAgent: row.UserAgent, AccessedAt: row.DownloadedAt,
		})
	}
	return items, nil
}

// GetStorage 获取存储接口（用于直接访问存储方法）
func (s *StorageService) GetStorage() storage.Storage {
	return s.storage
}

// GetStorageType 获取存储引擎类型
func (s *StorageService) GetStorageType() string {
	return s.cfg.GetStorageType()
}

// getDefaultBucket 获取默认 Bucket（内部方法）
func (s *StorageService) getDefaultBucket() string {
	return s.cfg.Storage.MinIO.DefaultBucket
}

func (s *StorageService) normalizeBucket(bucket string) string {
	bucket = strings.TrimSpace(bucket)
	bucket = strings.Trim(bucket, "/")
	if bucket == "" {
		return s.getDefaultBucket()
	}
	return bucket
}

// generateFileKey 生成文件存储路径
// 格式：{router}/{date}/{uuid}.{ext}
// 例如：luobei/test88888/plugins/cashier_desk/2025/01/03/xxx-xxx.jpg
func (s *StorageService) generateFileKey(router string, filename string) string {
	// 清理 router 前后的斜杠
	router = filepath.Clean(router)
	if router == "." {
		router = ""
	}
	// 移除前导斜杠
	router = trimLeadingSlash(router)

	// 生成 UUID
	id := uuid.New().String()

	// 获取文件扩展名
	ext := filepath.Ext(filename)

	// 按函数路径和日期分组
	now := time.Now()
	key := fmt.Sprintf("%s/%d/%02d/%02d/%s%s",
		router, now.Year(), now.Month(), now.Day(), id, ext)

	return key
}

// trimLeadingSlash 移除前导斜杠
// 注意：此函数与 api/v1/storage.go 中的 trimLeadingSlash 功能相同，但保留在各自包中以避免循环依赖
func trimLeadingSlash(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return s
}

func normalizeObjectKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "/")
	for strings.Contains(key, "//") {
		key = strings.ReplaceAll(key, "//", "/")
	}
	return key
}

// BuildPreviewFileKey 生成可由原文件 key 稳定推导的缩略图/封面 key。
func (s *StorageService) BuildPreviewFileKey(sourceKey string, previewFileName string) (string, error) {
	sourceKey = normalizeObjectKey(sourceKey)
	if isUnsafeObjectKey(sourceKey) {
		return "", fmt.Errorf("原文件路径不合法")
	}

	previewExt := strings.ToLower(filepath.Ext(previewFileName))
	if previewExt == "" {
		previewExt = ".webp"
	}
	if !isAllowedPreviewExtension(previewExt) {
		return "", fmt.Errorf("不支持的预览文件扩展名: %s", previewExt)
	}

	if strings.TrimSpace(sourceKey) == "" {
		return "", fmt.Errorf("原文件路径不合法")
	}
	return sourceKey + ".thumb" + previewExt, nil
}

func isAllowedPreviewExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".webp", ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
	}
}

func isUnsafeObjectKey(key string) bool {
	key = strings.TrimSpace(key)
	return key == "" || key == "." || strings.HasPrefix(key, "../") || strings.Contains(key, "/../")
}

// ListFilesByRouter 列举某个函数路径下的所有文件
func (s *StorageService) ListFilesByRouter(ctx context.Context, router string) ([]string, error) {
	bucket := s.getDefaultBucket()
	prefix := router + "/"

	objects, err := s.storage.ListObjects(ctx, bucket, prefix, true)
	if err != nil {
		logger.Errorf(ctx, "Failed to list objects: %v", err)
		return nil, fmt.Errorf("列举文件失败")
	}

	var files []string
	for _, obj := range objects {
		files = append(files, obj.Key)
	}

	return files, nil
}

// GetStorageStats 获取存储统计信息
func (s *StorageService) GetStorageStats(ctx context.Context, router string) (fileCount int, totalSize int64, err error) {
	bucket := s.getDefaultBucket()
	prefix := router + "/"

	objects, err := s.storage.ListObjects(ctx, bucket, prefix, true)
	if err != nil {
		logger.Errorf(ctx, "Failed to get storage stats: %v", err)
		return 0, 0, fmt.Errorf("获取统计信息失败")
	}

	fileCount = len(objects)
	for _, obj := range objects {
		totalSize += obj.Size
	}

	return fileCount, totalSize, nil
}

// DeleteFilesByRouter 删除某个函数路径下的所有文件
func (s *StorageService) DeleteFilesByRouter(ctx context.Context, router string) (int, error) {
	bucket := s.getDefaultBucket()
	prefix := router + "/"

	objects, err := s.storage.ListObjects(ctx, bucket, prefix, true)
	if err != nil {
		logger.Errorf(ctx, "Failed to list objects for deletion: %v", err)
		return 0, err
	}

	deletedCount := 0
	for _, obj := range objects {
		err := s.storage.DeleteObject(ctx, bucket, obj.Key)
		if err != nil {
			logger.Errorf(ctx, "Failed to delete file: key_len=%d, err=%v", len(obj.Key), err)
			continue
		}
		deletedCount++
	}

	logger.Infof(ctx, "Deleted %d files under router: %s", deletedCount, router)
	return deletedCount, nil
}

// RecordUpload 记录上传
func (s *StorageService) RecordUpload(ctx context.Context, record *model.FileUpload) error {
	if record != nil {
		record.Bucket = s.normalizeBucket(record.Bucket)
	}
	// 检查是否启用了上传记录
	if !s.cfg.Audit.UploadTracking.Enabled {
		logger.Debugf(ctx, "[StorageService] Upload tracking disabled, skipping record")
		return nil
	}

	if s.fileRepo == nil {
		logger.Warnf(ctx, "[StorageService] Database not initialized, upload record not saved (file_key_len=%d)", len(record.FileKey))
		return nil
	}

	err := s.fileRepo.CreateUploadRecord(ctx, record)
	if err != nil {
		logger.Errorf(ctx, "[StorageService] Failed to record upload (file_key_len=%d): %v", len(record.FileKey), err)
		return err
	}

	logger.Infof(ctx, "[StorageService] Upload record saved (file_key_len=%d, router=%s, size=%d)",
		len(record.FileKey), record.Router, record.FileSize)
	return nil
}

// UpdateUploadStatus 更新上传状态
func (s *StorageService) UpdateUploadStatus(ctx context.Context, fileKey string, status string) error {
	if s.fileRepo == nil {
		return nil // 审计功能未启用
	}
	return s.fileRepo.UpdateUploadStatus(ctx, fileKey, status)
}

func (s *StorageService) UpdateUploadStatusByBucketKey(ctx context.Context, bucket string, fileKey string, status string) error {
	if s.fileRepo == nil {
		return nil
	}
	return s.fileRepo.UpdateUploadStatusByBucketKey(ctx, s.normalizeBucket(bucket), fileKey, status)
}

// RecordDownload 记录下载
func (s *StorageService) RecordDownload(ctx context.Context, record *model.FileDownload) error {
	// 检查是否启用了下载记录
	if !s.cfg.Audit.DownloadTracking.Enabled {
		return nil // 下载记录未启用
	}

	if s.fileRepo == nil {
		logger.Warnf(ctx, "[StorageService] Database not initialized, download record not saved (file_key_len=%d)", len(record.FileKey))
		return nil
	}

	err := s.fileRepo.CreateDownloadRecord(ctx, record)
	if err != nil {
		logger.Errorf(ctx, "[StorageService] Failed to record download (file_key_len=%d): %v", len(record.FileKey), err)
		return err
	}

	return nil
}
