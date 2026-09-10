package service

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kageos/kageos/core/app-server/model"
	"github.com/kageos/kageos/core/app-server/repository"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/apicall"
	"github.com/kageos/kageos/pkg/logger"
	"github.com/kageos/kageos/pkg/scheduledauth"
	"github.com/kageos/kageos/pkg/scheduledsdk"
	"github.com/kageos/kageos/pkg/storage"
	"gorm.io/gorm"
)

const (
	LogArchiveExecutorKey = "platform.log_archive"
	logArchiveTypeOperate = "operate_log"
	logArchiveRouter      = "system/log-archives"
)

type LogArchiveConfig struct {
	Enabled       bool
	RetentionDays int
	BatchSize     int
	MaxBatches    int
	CronExpr      string
	Timezone      string
}

func DefaultLogArchiveConfig() LogArchiveConfig {
	return LogArchiveConfig{
		Enabled:       envBool("KAGEOS_LOG_ARCHIVE_ENABLED", true),
		RetentionDays: envInt("KAGEOS_LOG_ARCHIVE_RETENTION_DAYS", 90, 7, 3650),
		BatchSize:     envInt("KAGEOS_LOG_ARCHIVE_BATCH_SIZE", 10000, 100, 100000),
		MaxBatches:    envInt("KAGEOS_LOG_ARCHIVE_MAX_BATCHES", 20, 1, 1000),
		CronExpr:      envString("KAGEOS_LOG_ARCHIVE_CRON", "20 3 * * *"),
		Timezone:      envString("KAGEOS_LOG_ARCHIVE_TIMEZONE", "Asia/Shanghai"),
	}
}

type LogArchiveService struct {
	repo       *repository.LogArchiveRepository
	config     LogArchiveConfig
	httpClient *http.Client
}

func NewLogArchiveService(repo *repository.LogArchiveRepository, cfg LogArchiveConfig) *LogArchiveService {
	return &LogArchiveService{repo: repo, config: cfg, httpClient: &http.Client{Timeout: 2 * time.Minute}}
}

func (s *LogArchiveService) List(ctx context.Context, page, pageSize int) ([]*model.LogArchiveBatch, int64, error) {
	return s.repo.List(ctx, page, pageSize)
}

func (s *LogArchiveService) Config() LogArchiveConfig { return s.config }

type archiveRunSummary struct {
	Batches int   `json:"batches"`
	Records int64 `json:"records"`
}

func (s *LogArchiveService) RunScheduled(ctx context.Context, event scheduledsdk.ExecutionRequestedEvent) (*scheduledsdk.ExecutionResult, error) {
	if !s.config.Enabled {
		return &scheduledsdk.ExecutionResult{OutputSummary: "log archive disabled"}, nil
	}
	workerCtx, err := scheduledauth.WithExecutionToken(ctx, event, 2*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("create archive worker token: %w", err)
	}
	summary, err := s.Run(workerCtx)
	payload, _ := json.Marshal(summary)
	result := &scheduledsdk.ExecutionResult{OutputSummary: fmt.Sprintf("archived %d logs in %d batches", summary.Records, summary.Batches), ResultPayload: payload}
	return result, err
}

func (s *LogArchiveService) Run(ctx context.Context) (archiveRunSummary, error) {
	var out archiveRunSummary
	err := s.repo.Exclusive(ctx, func(repo *repository.LogArchiveRepository) error {
		worker := *s
		worker.repo = repo
		var err error
		out, err = worker.run(ctx)
		return err
	})
	return out, err
}

// Retry resumes the durable checkpoint, including legacy failed batches whose
// archive was verified before source cleanup. Completed requests are idempotent.
func (s *LogArchiveService) Retry(ctx context.Context, id int64) error {
	return s.repo.Exclusive(ctx, func(repo *repository.LogArchiveRepository) error {
		worker := *s
		worker.repo = repo
		batch, err := repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if batch.Status == model.LogArchiveStatusCompleted {
			return nil
		}
		return worker.attempt(ctx, batch)
	})
}

func (s *LogArchiveService) attempt(ctx context.Context, batch *model.LogArchiveBatch) error {
	batch.Attempts++
	batch.NextRetryAt = nil
	if err := s.repo.Save(ctx, batch); err != nil {
		return err
	}
	if err := s.processBatch(ctx, batch); err != nil {
		s.markFailed(ctx, batch, err)
		return err
	}
	return nil
}

func (s *LogArchiveService) run(ctx context.Context) (archiveRunSummary, error) {
	var out archiveRunSummary
	cutoff := time.Now().AddDate(0, 0, -s.config.RetentionDays)
	var failures []error
	for attempts := 0; attempts < s.config.MaxBatches; attempts++ {
		batch, err := s.nextBatch(ctx, cutoff)
		if repository.IsArchiveNotFound(err) {
			return out, errors.Join(failures...)
		}
		if err != nil {
			return out, err
		}
		if err := s.attempt(ctx, batch); err != nil {
			failures = append(failures, err)
			if ctx.Err() != nil {
				return out, errors.Join(failures...)
			}
			continue
		}
		out.Batches++
		out.Records += batch.RecordCount
	}
	return out, errors.Join(failures...)
}

func (s *LogArchiveService) nextBatch(ctx context.Context, cutoff time.Time) (*model.LogArchiveBatch, error) {
	if batch, err := s.repo.GetResumable(ctx); err == nil {
		return batch, nil
	} else if !repository.IsArchiveNotFound(err) {
		return nil, err
	}
	tenantUser, app, err := s.repo.NextScope(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	ids, err := s.repo.SelectIDs(ctx, tenantUser, app, cutoff, s.config.BatchSize)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	batch := &model.LogArchiveBatch{
		ArchiveKey:  archiveBatchKey(tenantUser, app, ids[0], ids[len(ids)-1]),
		ArchiveType: logArchiveTypeOperate,
		TenantUser:  tenantUser,
		App:         app,
		MinLogID:    ids[0], MaxLogID: ids[len(ids)-1], RecordCount: int64(len(ids)),
		Status: model.LogArchiveStatusExporting,
	}
	batch.SelectedIDsJSON, err = json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	start, end, err := s.repo.SelectedStats(ctx, ids)
	if err != nil {
		return nil, err
	}
	batch.RangeStartedAt, batch.RangeEndedAt = start, end
	if err := s.repo.Create(ctx, batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func (s *LogArchiveService) processBatch(ctx context.Context, batch *model.LogArchiveBatch) error {
	if batch.ObjectVerifiedAt != nil && batch.ObjectRef != "" && batch.SHA256 != "" {
		return s.deleteArchivedSource(ctx, batch)
	}
	tempPath, summary, err := s.exportBatch(ctx, batch)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)
	file, err := os.Open(tempPath)
	if err != nil {
		return err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	hash, err := sha256File(file)
	if err != nil {
		file.Close()
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return err
	}
	fileName := fmt.Sprintf("operate-logs-%s-%s-%d-%d.jsonl.gz", safeName(batch.TenantUser), safeName(batch.App), batch.MinLogID, batch.MaxLogID)
	archivePath := archiveRouter(batch)
	tokens, err := apicall.BatchGetUploadToken(ctx, &dto.BatchGetUploadTokenReq{
		UploadSource: dto.UploadSourceServer,
		Files:        []dto.GetUploadTokenReq{{FileName: fileName, ContentType: "application/gzip", FileSize: stat.Size(), Router: archivePath, Hash: hash, UploadSource: dto.UploadSourceServer}},
	})
	if err != nil {
		file.Close()
		return fmt.Errorf("get archive upload token: %w", err)
	}
	if tokens == nil || len(tokens.Tokens) != 1 {
		file.Close()
		return fmt.Errorf("storage returned no archive upload token")
	}
	token := tokens.Tokens[0]
	uploader, err := storage.NewUploader(token.Storage)
	if err != nil {
		file.Close()
		return err
	}
	uploaded, err := uploader.Upload(ctx, &token, file, stat.Size(), hash)
	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("upload archive: %w", err)
	}
	if closeErr != nil {
		return closeErr
	}
	if uploaded.Size != stat.Size() || uploaded.Hash != hash {
		return fmt.Errorf("archive upload result mismatch")
	}
	if err := s.verifyUploadedObject(ctx, uploaded.ServerDownloadURL, stat.Size(), hash); err != nil {
		return err
	}
	complete, err := apicall.BatchUploadComplete(ctx, &dto.BatchUploadCompleteReq{Items: []dto.BatchUploadCompleteItem{{
		Key: uploaded.Key, Bucket: token.Bucket, Success: true, Router: archivePath, FileName: fileName,
		Description: fmt.Sprintf("kageos operation log archive %s/%s", batch.TenantUser, batch.App), FileSize: stat.Size(), ContentType: "application/gzip", Hash: hash,
	}}})
	if err != nil {
		return fmt.Errorf("record archive upload: %w", err)
	}
	if complete == nil || len(complete.Results) != 1 || complete.Results[0].Status != "completed" {
		return fmt.Errorf("storage did not confirm archive upload")
	}
	now := time.Now()
	batch.ObjectBucket, batch.ObjectKey, batch.ObjectRef = token.Bucket, uploaded.Key, complete.Results[0].Ref
	batch.FileName, batch.FileSize, batch.SHA256 = fileName, stat.Size(), hash
	batch.SummaryJSON = summary
	batch.ObjectVerifiedAt, batch.ArchivedAt = &now, &now
	batch.Status, batch.ErrorMessage = model.LogArchiveStatusUploaded, ""
	if err := s.repo.Save(ctx, batch); err != nil {
		return err
	}
	return s.deleteArchivedSource(ctx, batch)
}

func (s *LogArchiveService) exportBatch(ctx context.Context, batch *model.LogArchiveBatch) (string, json.RawMessage, error) {
	file, err := os.CreateTemp("", "kageos-log-archive-*.jsonl.gz")
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	cleanup := func(e error) (string, json.RawMessage, error) { file.Close(); os.Remove(path); return "", nil, e }
	gz := gzip.NewWriter(file)
	encoder := json.NewEncoder(gz)
	resourceCounts := map[string]int64{}
	var written int64
	var selectedIDs []int64
	if err := json.Unmarshal(batch.SelectedIDsJSON, &selectedIDs); err != nil {
		return cleanup(fmt.Errorf("decode selected log ids: %w", err))
	}
	for offset := 0; offset < len(selectedIDs); offset += 500 {
		to := offset + 500
		if to > len(selectedIDs) {
			to = len(selectedIDs)
		}
		rows, err := s.repo.LoadIDs(ctx, selectedIDs[offset:to])
		if err != nil {
			gz.Close()
			return cleanup(err)
		}
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				gz.Close()
				return cleanup(err)
			}
			resourceCounts[row.ResourcePath]++
			written++
		}
	}
	if written != batch.RecordCount {
		gz.Close()
		return cleanup(fmt.Errorf("archive export count mismatch: wrote=%d expected=%d", written, batch.RecordCount))
	}
	if err := gz.Close(); err != nil {
		return cleanup(err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", nil, err
	}
	summary, err := buildArchiveSummary(resourceCounts)
	if err != nil {
		os.Remove(path)
		return "", nil, err
	}
	return path, summary, nil
}

func (s *LogArchiveService) deleteArchivedSource(ctx context.Context, batch *model.LogArchiveBatch) error {
	if batch.ObjectVerifiedAt == nil || batch.ObjectRef == "" || batch.SHA256 == "" {
		return fmt.Errorf("refuse to delete unverified archive batch %s", batch.ArchiveKey)
	}
	if _, err := s.repo.DeleteRange(ctx, batch, 1000); err != nil {
		return fmt.Errorf("delete archived logs: %w", err)
	}
	now := time.Now()
	// Do not advance the in-memory checkpoint until persistence succeeds: the
	// error path must retain uploaded, even if all source rows are already gone.
	completed := *batch
	completed.Status, completed.DeletedAtSource, completed.ErrorMessage = model.LogArchiveStatusCompleted, &now, ""
	completed.NextRetryAt = nil
	if err := s.repo.Save(ctx, &completed); err != nil {
		return err
	}
	*batch = completed
	return nil
}

func (s *LogArchiveService) verifyUploadedObject(ctx context.Context, downloadURL string, expectedSize int64, expectedHash string) error {
	if strings.TrimSpace(downloadURL) == "" {
		return fmt.Errorf("archive download URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("verify archive object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verify archive object: HTTP %d", resp.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(resp.Body, expectedSize+1))
	if err != nil {
		return fmt.Errorf("read archive object: %w", err)
	}
	if size != expectedSize || hex.EncodeToString(hash.Sum(nil)) != expectedHash {
		return fmt.Errorf("archive object size or SHA256 mismatch")
	}
	return nil
}

func (s *LogArchiveService) markFailed(ctx context.Context, batch *model.LogArchiveBatch, runErr error) {
	if batch == nil {
		return
	}
	if batch.ObjectVerifiedAt != nil && batch.ObjectRef != "" && batch.SHA256 != "" {
		batch.Status = model.LogArchiveStatusUploaded
	} else {
		batch.Status = model.LogArchiveStatusFailed
	}
	delay := 15 * time.Minute * time.Duration(1<<min(max(batch.Attempts-1, 0), 6))
	nextRetry := time.Now().Add(delay)
	batch.NextRetryAt = &nextRetry
	batch.ErrorMessage = truncate(runErr.Error(), 4000)
	if err := s.repo.Save(context.WithoutCancel(ctx), batch); err != nil {
		logger.Warnf(ctx, "[LogArchive] save failure state: %v", err)
	}
}

func sha256File(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func archiveBatchKey(tenantUser, app string, minID, maxID int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", tenantUser, app, minID, maxID)))
	return "operate-log-" + hex.EncodeToString(h[:12])
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	if value == "" {
		return "unknown"
	}
	return value
}

func archiveRouter(batch *model.LogArchiveBatch) string {
	month := batch.RangeStartedAt.Format("2006-01")
	return strings.Join([]string{logArchiveRouter, safeName(batch.TenantUser), safeName(batch.App), month}, "/")
}

func buildArchiveSummary(counts map[string]int64) (json.RawMessage, error) {
	type item struct {
		ResourcePath string `json:"resource_path"`
		Count        int64  `json:"count"`
	}
	items := make([]item, 0, len(counts))
	for path, count := range counts {
		items = append(items, item{ResourcePath: path, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].ResourcePath < items[j].ResourcePath
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > 20 {
		items = items[:20]
	}
	return json.Marshal(map[string]any{"top_resource_paths": items})
}

func envBool(key string, fallback bool) bool {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if value, err := strconv.ParseBool(raw); err == nil {
			return value
		}
	}
	return fallback
}
func envInt(key string, fallback, min, max int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}
func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
