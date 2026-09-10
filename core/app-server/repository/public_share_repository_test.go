package repository

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kageos/kageos/core/app-server/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPublicShareReserveUseIsAtomicAtLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.PublicShare{}); err != nil {
		t.Fatal(err)
	}
	share := &model.PublicShare{
		ShareID:      "ps_atomic",
		TenantUser:   "owner",
		App:          "ops",
		FullCodePath: "/owner/ops/form.submit",
		ResourceType: model.PublicShareResourceTypeForm,
		Action:       model.PublicShareActionFormSubmit,
		Enabled:      true,
		MaxUses:      1,
	}
	if err := db.Create(share).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewPublicShareRepository(db)

	var successes atomic.Int32
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if repo.ReserveUse(context.Background(), share.ShareID) == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful reservations = %d, want exactly 1", got)
	}
	var stored model.PublicShare
	if err := db.Where("share_id = ?", share.ShareID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UseCount != 1 {
		t.Fatalf("use_count = %d, want 1", stored.UseCount)
	}
	if err := repo.ReserveUse(context.Background(), share.ShareID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("reservation past max error = %v, want record not found", err)
	}
}

func TestPublicShareReleaseUseReturnsReservation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.PublicShare{}); err != nil {
		t.Fatal(err)
	}
	share := &model.PublicShare{
		ShareID:      "ps_release",
		TenantUser:   "owner",
		App:          "ops",
		FullCodePath: "/owner/ops/form.submit",
		ResourceType: model.PublicShareResourceTypeForm,
		Action:       model.PublicShareActionFormSubmit,
		Enabled:      true,
		MaxUses:      1,
	}
	if err := db.Create(share).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewPublicShareRepository(db)
	if err := repo.ReserveUse(context.Background(), share.ShareID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReleaseUse(context.Background(), share.ShareID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReserveUse(context.Background(), share.ShareID); err != nil {
		t.Fatalf("released reservation should be reusable: %v", err)
	}
}

func TestPublicShareListSeparatesExhaustedLinks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.PublicShare{}); err != nil {
		t.Fatal(err)
	}
	for _, share := range []model.PublicShare{
		{ShareID: "available", TenantUser: "owner", App: "ops", Enabled: true, MaxUses: 2, UseCount: 1},
		{ShareID: "exhausted", TenantUser: "owner", App: "ops", Enabled: true, MaxUses: 2, UseCount: 2},
	} {
		if err := db.Create(&share).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewPublicShareRepository(db)
	for status, expected := range map[string]string{"enabled": "available", "exhausted": "exhausted"} {
		rows, err := repo.List(context.Background(), "owner", "ops", PublicShareListFilter{Status: status})
		if err != nil || len(rows) != 1 || rows[0].ShareID != expected {
			t.Fatalf("%s: %+v %v", status, rows, err)
		}
	}
}
