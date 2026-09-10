package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/core/hr-server/repository"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/gormx/models"
	"github.com/kageos/kageos/pkg/openapitoken"
)

type failingGovernancePublisher struct {
	fail     bool
	sessions int
	tokens   int
}

func (p *failingGovernancePublisher) InvalidateToken(context.Context, int64, string, string, string) error {
	return nil
}
func (p *failingGovernancePublisher) InvalidateUserTokens(_ context.Context, _ int64, _ string, sessions []*model.UserSession, _ string) error {
	p.sessions += len(sessions)
	if p.fail {
		return errors.New("notification unavailable")
	}
	return nil
}
func (p *failingGovernancePublisher) InvalidateOpenAPIToken(context.Context, int64, string, string, *time.Time) error {
	p.tokens++
	if p.fail {
		return errors.New("notification unavailable")
	}
	return nil
}

func TestFreezeRetryAndRestoreDoNotResurrectCredentials(t *testing.T) {
	db := openSystemUserManagementTestDB(t)
	if err := db.AutoMigrate(&openapitoken.OpenAPIToken{}, &model.UserStatusEvent{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "alice", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	session := model.UserSession{UserID: user.ID, Token: "access", RefreshToken: "refresh", IsActive: true, ExpiresAt: models.Time(time.Now().Add(time.Hour))}
	token := openapitoken.OpenAPIToken{OwnerUserID: user.ID, OwnerUsername: user.Username, TokenHash: "test-hash"}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	publisher := &failingGovernancePublisher{fail: true}
	svc := NewUserService(repository.NewUserRepository(db), publisher, repository.NewUserSessionRepository(db), nil)
	if _, err := svc.UpdateUserStatusFromSystem(context.Background(), "alice", "disabled", "system", "abuse report"); err == nil {
		t.Fatal("expected notification failure")
	}
	db.First(&user, user.ID)
	db.First(&session, session.ID)
	db.First(&token, token.ID)
	if user.Status != "disabled" || session.IsActive || token.RevokedAt == nil {
		t.Fatal("freeze did not persist all revocations")
	}
	publisher.fail = false
	if _, err := svc.UpdateUserStatusFromSystem(context.Background(), "alice", "disabled"); err != nil {
		t.Fatal(err)
	}
	if publisher.sessions != 2 || publisher.tokens != 2 {
		t.Fatalf("notifications were not retried: %+v", publisher)
	}
	if _, err := svc.UpdateUserStatusFromSystem(context.Background(), "alice", "active"); err != nil {
		t.Fatal(err)
	}
	db.First(&session, session.ID)
	db.First(&token, token.ID)
	if session.IsActive || token.RevokedAt == nil {
		t.Fatal("restoring account resurrected old credentials")
	}
	var events []model.UserStatusEvent
	if err := db.Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Reason != "abuse report" {
		t.Fatalf("missing audit: %+v", events)
	}
}

func TestFreezeRollsBackOnCredentialStorageFailure(t *testing.T) {
	db := openSystemUserManagementTestDB(t) // Deliberately no token table.
	user := model.User{Username: "alice", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(repository.NewUserRepository(db), nil, repository.NewUserSessionRepository(db), nil)
	if _, err := svc.UpdateUserStatusFromSystem(context.Background(), "alice", "disabled"); err == nil {
		t.Fatal("expected storage failure")
	}
	db.First(&user, user.ID)
	if user.Status != "active" {
		t.Fatal("account change should roll back")
	}
}

func TestImportPreviewAndRetryNeverOverwrite(t *testing.T) {
	db := openSystemUserManagementTestDB(t)
	svc := NewUserService(repository.NewUserRepository(db), nil, repository.NewUserSessionRepository(db), nil)
	rows := []dto.SystemCreateUserReq{{Username: "Alice", Password: "password1"}, {Username: "alice", Password: "password2"}, {Username: "bob", Password: "password3", Email: "invalid"}}
	results, err := svc.ImportUsers(context.Background(), rows, true, "system")
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count != 0 || results[0].Status != "ready" || results[1].Status != "failed" || results[2].Status != "failed" {
		t.Fatalf("invalid preview: %+v", results)
	}
	results, err = svc.ImportUsers(context.Background(), rows, false, "system")
	if err != nil || results[0].Status != "created" {
		t.Fatalf("create: %+v %v", results, err)
	}
	before, _ := svc.GetUserByUsername("alice")
	rows[0].Password = "replacement"
	results, err = svc.ImportUsers(context.Background(), rows, false, "system")
	after, _ := svc.GetUserByUsername("alice")
	if err != nil || results[0].Status != "failed" || before.PasswordHash != after.PasswordHash || after.Status != "disabled" {
		t.Fatal("retry altered an existing account")
	}
}

func TestFrozenAccountCannotIssueCredentials(t *testing.T) {
	db := openSystemUserManagementTestDB(t)
	store, err := openapitoken.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "frozen", Status: "disabled"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewUserSessionRepository(db)
	if err := repo.CreateActiveUserSession(user.ID, "new-access", "new-refresh", models.Time(time.Now().Add(time.Hour))); err == nil {
		t.Fatal("frozen user issued session")
	}
	userRepo := repository.NewUserRepository(db)
	if _, err := userRepo.CreateActiveUserOpenAPIToken(context.Background(), store, openapitoken.CreateInput{OwnerUsername: user.Username, OwnerUserID: user.ID, Name: "test"}); err == nil {
		t.Fatal("frozen user issued API token")
	}
	var count int64
	db.Model(&model.UserSession{}).Count(&count)
	if count != 0 {
		t.Fatal("session was persisted")
	}
	db.Model(&openapitoken.OpenAPIToken{}).Count(&count)
	if count != 0 {
		t.Fatal("API token was persisted")
	}
}
