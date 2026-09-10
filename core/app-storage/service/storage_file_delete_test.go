package service

import (
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/kageos/kageos/core/app-storage/model"
	"github.com/kageos/kageos/core/app-storage/repository"
	storagepkg "github.com/kageos/kageos/core/app-storage/storage"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/config"
	"github.com/kageos/kageos/pkg/contextx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type deleteTestStorage struct {
	deleted []string
}

func (s *deleteTestStorage) GetCDNDomain() string            { return "" }
func (s *deleteTestStorage) GetUploadEndpoint(string) string { return "" }
func (s *deleteTestStorage) GenerateUploadCredentials(context.Context, string, string, string, time.Duration, string) (*storagepkg.UploadCredentials, error) {
	return nil, nil
}
func (s *deleteTestStorage) GenerateDownloadURLs(context.Context, string, string, time.Duration, map[string]string) (string, string, error) {
	return "/download", "http://storage/download", nil
}
func (s *deleteTestStorage) DeleteObject(_ context.Context, bucket, key string) error {
	s.deleted = append(s.deleted, bucket+"/"+key)
	return nil
}
func (s *deleteTestStorage) GetObjectInfo(_ context.Context, _ string, key string) (*storagepkg.ObjectInfo, error) {
	return &storagepkg.ObjectInfo{Key: key, Size: 42}, nil
}
func (s *deleteTestStorage) ListObjects(context.Context, string, string, bool) ([]storagepkg.ObjectInfo, error) {
	return nil, nil
}
func (s *deleteTestStorage) EnsureBucket(context.Context, string, string) error { return nil }
func (s *deleteTestStorage) UploadObject(context.Context, string, string, io.Reader, int64, string) error {
	return nil
}
func (s *deleteTestStorage) DownloadObject(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}

func newDeleteTestService(t *testing.T) (*StorageService, *repository.FileRepository, *deleteTestStorage) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.FileUpload{}, &model.FileDownload{}); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewFileRepository(db)
	backend := &deleteTestStorage{}
	cfg := &config.AppStorageConfig{}
	cfg.Storage.MinIO.DefaultBucket = "kageos"
	cfg.Audit.UploadTracking.Enabled = true
	cfg.Audit.DownloadTracking.Enabled = true
	return NewStorageService(backend, cfg, repo), repo, backend
}

func TestGetSystemStorageAssetsFiltersByServicePathAndIncludesAuditSummary(t *testing.T) {
	service, repo, _ := newDeleteTestService(t)
	service.cfg.Storage.MinIO.ConsoleURL = "https://ops.example.invalid"
	alice := "alice"
	bob := "bob"
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/app/orders/a.csv", Router: "alice/app/orders/export.table",
		FileName: "a.csv", FileSize: 2048, Username: "alice", Tenant: "alice", Status: "completed",
	})
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/app/report/b.pdf", Router: "alice/app/report/build.form",
		FileName: "b.pdf", FileSize: 4096, Username: "alice", Tenant: "alice", Status: "deleted",
	})
	if err := service.RecordDownload(context.Background(), &model.FileDownload{FileKey: "alice/app/orders/a.csv", Action: "download", Username: &alice}); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordDownload(context.Background(), &model.FileDownload{FileKey: "alice/app/orders/a.csv", Action: "preview", Username: &bob}); err != nil {
		t.Fatal(err)
	}

	result, err := service.GetSystemStorageAssets(context.Background(), dto.SystemStorageAssetsReq{
		Page: 1, PageSize: 20, RouterPrefix: "alice/app/orders", Status: "completed", Keyword: "a.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ConsoleURL != "" {
		t.Fatal("asset API must not disclose operations console")
	}
	if result.Total != 1 || len(result.List) != 1 || result.List[0].DownloadCount != 1 || result.List[0].PreviewCount != 1 {
		t.Fatalf("unexpected asset result: %#v", result)
	}
	if result.Summary.ActiveFiles != 1 || result.Summary.ActiveBytes != 2048 || result.Summary.DeletedFiles != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if len(result.Directories) != 1 || result.Directories[0].Router != "alice/app/orders/export.table" {
		t.Fatalf("unexpected directories: %#v", result.Directories)
	}
	if len(result.Workspaces) != 1 || result.Workspaces[0].Path != "alice/app" || result.Workspaces[0].FileCount != 1 || result.Workspaces[0].SizeBytes != 2048 {
		t.Fatalf("unexpected workspaces: %#v", result.Workspaces)
	}
	audits, err := service.GetSystemAssetAudits(context.Background(), "kageos/alice/app/orders/a.csv", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 2 || audits[0].Action != "preview" || audits[1].Action != "download" {
		t.Fatalf("unexpected access audits: %#v", audits)
	}
}

func TestGetSystemStorageAssetsBuildsImageThumbnailAndPreviewMetadata(t *testing.T) {
	service, repo, _ := newDeleteTestService(t)
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/app/gallery/cover.png", Router: "alice/app/gallery/list.table",
		FileName: "cover.png", FileSize: 1024, ContentType: "image/png", Username: "alice", Status: "completed",
	})

	result, err := service.GetSystemStorageAssets(context.Background(), dto.SystemStorageAssetsReq{Page: 1, PageSize: 20, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.List) != 1 || !result.List[0].Previewable || result.List[0].PreviewKind != "image" || result.List[0].ThumbnailURL != "/download" {
		t.Fatalf("unexpected preview metadata: %#v", result.List)
	}
}

func createDeleteTestRecord(t *testing.T, repo *repository.FileRepository, record *model.FileUpload) {
	t.Helper()
	if err := repo.CreateUploadRecord(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteOwnedFileDeletesObjectAndThumbnailAndKeepsTombstones(t *testing.T) {
	service, repo, backend := newDeleteTestService(t)
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/tools/result.xlsx", Router: "alice/tools/run.form",
		FileName: "result.xlsx", FileSize: 2048, Username: "alice", Tenant: "alice", Status: "completed",
		ThumbnailRef: "kageos/alice/tools/result.xlsx.thumb.webp",
	})
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/tools/result.xlsx.thumb.webp", Router: "alice/tools/run.form",
		FileName: "result.thumb.webp", FileSize: 256, Username: "alice", Tenant: "alice", Status: "completed",
	})

	result, err := service.DeleteOwnedFile(context.Background(), "kageos/alice/tools/result.xlsx", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "deleted" || result.ReleasedBytes != 2304 || result.DeletedAt == 0 {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	wantDeleted := []string{"kageos/alice/tools/result.xlsx.thumb.webp", "kageos/alice/tools/result.xlsx"}
	if !slices.Equal(backend.deleted, wantDeleted) {
		t.Fatalf("deleted objects = %#v, want %#v", backend.deleted, wantDeleted)
	}

	for _, key := range []string{"alice/tools/result.xlsx", "alice/tools/result.xlsx.thumb.webp"} {
		record, err := repo.GetUploadRecordByBucketKey(context.Background(), "kageos", key)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != "deleted" || record.DeletedAt == nil || record.DeletedBy != "alice" {
			t.Fatalf("record %s did not keep a complete tombstone: %#v", key, record)
		}
	}
}

func TestDeleteOwnedFileRejectsAnotherUser(t *testing.T) {
	service, repo, backend := newDeleteTestService(t)
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/private.pdf", Router: "alice/tools/run.form",
		FileName: "private.pdf", FileSize: 1024, Username: "alice", Tenant: "alice", Status: "completed",
	})

	if _, err := service.DeleteOwnedFile(context.Background(), "kageos/alice/private.pdf", "bob"); err == nil {
		t.Fatal("expected ownership rejection")
	}
	if len(backend.deleted) != 0 {
		t.Fatalf("unauthorized delete reached object storage: %#v", backend.deleted)
	}
	record, err := repo.GetUploadRecordByBucketKey(context.Background(), "kageos", "alice/private.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "completed" {
		t.Fatalf("unauthorized delete changed status to %q", record.Status)
	}
}

func TestResolveDeletedFileReturnsTombstoneWithoutURL(t *testing.T) {
	service, repo, _ := newDeleteTestService(t)
	deletedAt := time.Now().Add(-time.Minute)
	createDeleteTestRecord(t, repo, &model.FileUpload{
		Bucket: "kageos", FileKey: "alice/old.csv", Router: "alice/tools/run.form",
		FileName: "old.csv", FileSize: 4096, Username: "alice", Tenant: "alice", Status: "deleted",
		DeletedAt: &deletedAt, DeletedBy: "alice",
	})

	ctx := contextx.WithRequestUser(context.Background(), "alice")
	files, err := service.ResolveFileRefs(ctx, []string{"kageos/alice/old.csv"}, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("resolved file count = %d", len(files))
	}
	file := files[0]
	if file.Status != "deleted" || file.DownloadURL != "" || file.CanDelete || file.DeletedAt == 0 || file.DeletedBy != "alice" {
		t.Fatalf("unexpected tombstone: %#v", file)
	}
}
