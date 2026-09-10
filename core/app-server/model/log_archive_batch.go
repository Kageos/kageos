package model

import (
	"encoding/json"
	"time"

	"github.com/kageos/kageos/pkg/gormx/models"
)

const (
	LogArchiveStatusExporting = "exporting"
	LogArchiveStatusUploaded  = "uploaded"
	LogArchiveStatusCompleted = "completed"
	LogArchiveStatusFailed    = "failed"
)

// LogArchiveBatch records the durable evidence for one offline log archive.
// Archived content intentionally remains outside the online query path.
type LogArchiveBatch struct {
	models.Base
	ArchiveKey       string          `json:"archive_key" gorm:"type:varchar(160);not null;uniqueIndex"`
	ArchiveType      string          `json:"archive_type" gorm:"type:varchar(40);not null;index"`
	TenantUser       string          `json:"tenant_user" gorm:"type:varchar(100);not null;index:idx_log_archive_scope"`
	App              string          `json:"app" gorm:"type:varchar(100);not null;index:idx_log_archive_scope"`
	RangeStartedAt   time.Time       `json:"range_started_at" gorm:"not null"`
	RangeEndedAt     time.Time       `json:"range_ended_at" gorm:"not null"`
	MinLogID         int64           `json:"min_log_id" gorm:"not null"`
	MaxLogID         int64           `json:"max_log_id" gorm:"not null"`
	RecordCount      int64           `json:"record_count" gorm:"not null"`
	SelectedIDsJSON  json.RawMessage `json:"-" gorm:"type:json;not null"`
	ObjectBucket     string          `json:"object_bucket" gorm:"type:varchar(100)"`
	ObjectKey        string          `json:"object_key" gorm:"type:varchar(700)"`
	ObjectRef        string          `json:"object_ref" gorm:"type:varchar(800)"`
	FileName         string          `json:"file_name" gorm:"type:varchar(255)"`
	FileSize         int64           `json:"file_size"`
	SHA256           string          `json:"sha256" gorm:"type:char(64)"`
	Status           string          `json:"status" gorm:"type:varchar(30);not null;index"`
	SummaryJSON      json.RawMessage `json:"summary_json" gorm:"type:json"`
	Attempts         int             `json:"attempts"`
	NextRetryAt      *time.Time      `json:"next_retry_at,omitempty" gorm:"index"`
	ErrorMessage     string          `json:"error_message" gorm:"type:text"`
	ObjectVerifiedAt *time.Time      `json:"object_verified_at,omitempty"`
	ArchivedAt       *time.Time      `json:"archived_at,omitempty"`
	DeletedAtSource  *time.Time      `json:"deleted_at_source,omitempty"`
}

func (LogArchiveBatch) TableName() string { return "log_archive_batches" }
