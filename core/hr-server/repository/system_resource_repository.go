package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/mysqlstats"
	"gorm.io/gorm"
)

type SystemResourceRepository struct {
	db *gorm.DB
}

func NewSystemResourceRepository(db *gorm.DB) *SystemResourceRepository {
	return &SystemResourceRepository{db: db}
}

func (r *SystemResourceRepository) Create(sample *model.SystemResourceSample) error {
	return r.db.Create(sample).Error
}

func (r *SystemResourceRepository) History(since time.Time, limit int) ([]model.SystemResourceSample, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var samples []model.SystemResourceSample
	err := r.db.Where("collected_at >= ?", since).
		Order("collected_at ASC, id ASC").
		Limit(limit).
		Find(&samples).Error
	return samples, err
}

func (r *SystemResourceRepository) PruneHistory(runtimeCutoff, platformCutoff, capacityCutoff time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range []struct {
			value  any
			cutoff time.Time
		}{
			{value: &model.SystemResourceSample{}, cutoff: runtimeCutoff},
			{value: &model.SystemPlatformSnapshot{}, cutoff: platformCutoff},
			{value: &model.SystemCapacitySnapshot{}, cutoff: capacityCutoff},
		} {
			if err := tx.Unscoped().Where("collected_at < ?", item.cutoff).Delete(item.value).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SystemResourceRepository) CreateCapacity(snapshot dto.SystemResourceSnapshot) error {
	snapshot.CollectedAt = snapshot.CollectedAt.UTC()
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	start, end := localDayBounds(snapshot.CollectedAt)
	return r.db.Transaction(func(tx *gorm.DB) error {
		if !snapshot.DatabaseInventoryComplete {
			var previous model.SystemCapacitySnapshot
			if err := tx.Where("collected_at >= ? AND collected_at < ?", start, end).First(&previous).Error; err == nil {
				var saved dto.SystemResourceSnapshot
				if json.Unmarshal([]byte(previous.PayloadJSON), &saved) == nil && saved.DatabaseInventoryComplete {
					return nil
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err := tx.Unscoped().Where("collected_at >= ? AND collected_at < ?", start, end).Delete(&model.SystemCapacitySnapshot{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.SystemCapacitySnapshot{CollectedAt: snapshot.CollectedAt, PayloadJSON: string(payload)}).Error
	})
}

func (r *SystemResourceRepository) LatestCapacity() (*dto.SystemResourceSnapshot, error) {
	var row model.SystemCapacitySnapshot
	if err := r.db.Order("collected_at DESC, id DESC").First(&row).Error; err != nil {
		return nil, err
	}
	var snapshot dto.SystemResourceSnapshot
	if err := json.Unmarshal([]byte(row.PayloadJSON), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (r *SystemResourceRepository) CapacityHistory(since time.Time, limit int) ([]dto.SystemResourceSnapshot, error) {
	if limit <= 0 || limit > 400 {
		limit = 400
	}
	var rows []model.SystemCapacitySnapshot
	if err := r.db.Where("collected_at >= ?", since.UTC()).
		Order("collected_at ASC, id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]dto.SystemResourceSnapshot, 0, len(rows))
	for _, row := range rows {
		var snapshot dto.SystemResourceSnapshot
		if err := json.Unmarshal([]byte(row.PayloadJSON), &snapshot); err != nil {
			return nil, fmt.Errorf("decode capacity snapshot %d: %w", row.ID, err)
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (r *SystemResourceRepository) CreatePlatform(metrics dto.SystemPlatformMetrics) error {
	payload, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	start, end := localDayBounds(metrics.CollectedAt)
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("collected_at >= ? AND collected_at < ?", start, end).Delete(&model.SystemPlatformSnapshot{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.SystemPlatformSnapshot{CollectedAt: metrics.CollectedAt, PayloadJSON: string(payload)}).Error
	})
}

func (r *SystemResourceRepository) LatestPlatform() (*dto.SystemPlatformMetrics, error) {
	var row model.SystemPlatformSnapshot
	if err := r.db.Order("collected_at DESC, id DESC").First(&row).Error; err != nil {
		return nil, err
	}
	var metrics dto.SystemPlatformMetrics
	if err := json.Unmarshal([]byte(row.PayloadJSON), &metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}

func (r *SystemResourceRepository) PlatformHistory(since time.Time, limit int) ([]dto.SystemPlatformMetrics, error) {
	if limit <= 0 || limit > 400 {
		limit = 400
	}
	var rows []model.SystemPlatformSnapshot
	if err := r.db.Where("collected_at >= ?", since).
		Order("collected_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]dto.SystemPlatformMetrics, 0, len(rows))
	for _, row := range rows {
		var metrics dto.SystemPlatformMetrics
		if err := json.Unmarshal([]byte(row.PayloadJSON), &metrics); err != nil {
			return nil, fmt.Errorf("decode platform snapshot %d: %w", row.ID, err)
		}
		result = append(result, metrics)
	}
	return result, nil
}

func localDayBounds(value time.Time) (time.Time, time.Time) {
	local := value.In(time.Local)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

func (r *SystemResourceRepository) CollectPlatformMetrics(now time.Time) (dto.SystemPlatformMetrics, error) {
	metrics := dto.SystemPlatformMetrics{CollectedAt: now.UTC()}
	counts := []struct {
		table, where string
		target       *int64
	}{
		{"user", "deleted_at IS NULL", &metrics.UsersTotal},
		{"user", "deleted_at IS NULL AND status = 'active'", &metrics.UsersActive},
		{"user", "deleted_at IS NULL AND status = 'pending'", &metrics.UsersPending},
	}
	for _, item := range counts {
		if !r.db.Migrator().HasTable(item.table) {
			continue
		}
		if err := r.db.Table(item.table).Where(item.where).Count(item.target).Error; err != nil {
			return dto.SystemPlatformMetrics{}, fmt.Errorf("count %s: %w", item.table, err)
		}
	}

	return metrics, nil
}

func (r *SystemResourceRepository) CollectDatabaseSizes(ctx context.Context) (uint64, []dto.SystemDatabaseSize, bool) {
	sourceID := ""
	definitions := platformDatabaseDefinitions()
	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	type databaseUsage struct {
		Name      string `gorm:"column:name"`
		UsedBytes uint64 `gorm:"column:used_bytes"`
	}
	var usage []databaseUsage
	query := `SELECT s.schema_name AS name, COALESCE(SUM(t.data_length + t.index_length), 0) AS used_bytes
		FROM information_schema.schemata s
		LEFT JOIN information_schema.tables t ON t.table_schema = s.schema_name
		WHERE s.schema_name NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
		GROUP BY s.schema_name`
	if err := mysqlstats.Fresh(ctx, r.db, func(db *gorm.DB) error {
		if db.Dialector.Name() == "mysql" {
			if err := db.Raw("SELECT @@server_uuid").Scan(&sourceID).Error; err != nil {
				return err
			}
			if sourceID == "" {
				return fmt.Errorf("database identity unavailable")
			}
		}
		return db.Raw(query).Scan(&usage).Error
	}); err != nil {
		return 0, []dto.SystemDatabaseSize{}, false
	}

	usageByName := make(map[string]uint64, len(usage))
	for _, item := range usage {
		usageByName[item.Name] = item.UsedBytes
		if _, known := definitions[item.Name]; !known {
			names = append(names, item.Name)
		}
	}
	databases := make([]dto.SystemDatabaseSize, 0, len(names))
	var total uint64
	for _, name := range names {
		definition, known := definitions[name]
		kind := "platform"
		if !known {
			kind = "unmanaged"
			definition = platformDatabaseDefinition{service: "-", purpose: "unmanaged_database"}
		}
		usedBytes, exists := usageByName[name]
		status := "active"
		if !exists {
			status = "missing"
		}
		total += usedBytes
		databases = append(databases, dto.SystemDatabaseSize{
			Name: name, SourceID: sourceID, Kind: kind, Owner: definition.service,
			Directory: "platform", Purpose: definition.purpose, Status: status, UsedBytes: usedBytes,
		})
	}
	return total, databases, true
}

type platformDatabaseDefinition struct {
	service string
	purpose string
}

func platformDatabaseDefinitions() map[string]platformDatabaseDefinition {
	return map[string]platformDatabaseDefinition{
		"agent-server":     {service: "agent-server", purpose: "agent_state"},
		"app-server":       {service: "app-server / app-runtime", purpose: "workspace_metadata"},
		"app-storage":      {service: "app-storage", purpose: "storage_metadata"},
		"connector-server": {service: "connector-server", purpose: "connector_state"},
		"hr-server":        {service: "hr-server", purpose: "identity_and_monitoring"},
		"message-server":   {service: "message-server", purpose: "message_state"},
		"timer-scheduler":  {service: "timer-scheduler", purpose: "scheduler_state"},
	}
}
