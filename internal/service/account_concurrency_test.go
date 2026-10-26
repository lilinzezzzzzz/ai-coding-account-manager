package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func TestDeleteAccountRejectsConcurrentCredentialUpdates(t *testing.T) {
	for _, operation := range []string{"refresh", "reset"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			db, err := database.Open(ctx, database.Config{Path: filepath.Join(t.TempDir(), "state.db")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			daos := dao.NewDAOs(db.GORM())
			account := entity.Account{ProviderID: "codex", AccountID: "busy", StorageID: "busy", Label: "busy", IsActive: true}
			other := entity.Account{ProviderID: "codex", AccountID: "other", StorageID: "other", Label: "other"}
			for _, record := range []entity.Account{account, other} {
				if err := daos.Accounts.Create(ctx, record); err != nil {
					t.Fatal(err)
				}
			}
			snapshotJSON := `{"rateLimitResetCredits":{"availableCount":1}}`
			usage := entity.UsageSnapshot{ProviderID: "codex", AccountID: "busy", Status: entity.UsageStatusReady, SnapshotJSON: &snapshotJSON}
			if err := daos.UsageSnapshots.Upsert(ctx, usage); err != nil {
				t.Fatal(err)
			}
			blocking := &blockingCredentialUpdateProvider{
				Provider: fake.New(fake.Config{ID: "codex", Accounts: []fake.AccountState{{Account: account, Usage: usage}, {Account: other}}}),
				started:  make(chan struct{}), release: make(chan struct{}),
			}
			registry := provider.NewRegistry()
			if err := registry.Register(ctx, blocking); err != nil {
				t.Fatal(err)
			}
			accounts := NewAccountService(dao.NewUnitOfWork(db.GORM()), daos, registry, NewAccountActivationCoordinator())
			done := make(chan error, 1)
			go func() {
				if operation == "reset" {
					_, err := accounts.ResetAccountRateLimit(ctx, "codex", "busy", "reset-1")
					done <- err
					return
				}
				result, err := accounts.RefreshAccount(ctx, "codex", "busy")
				if err == nil && result.ErrorCode != nil {
					err = entity.NewAppError(entity.ErrorCode(*result.ErrorCode))
				}
				done <- err
			}()
			<-blocking.started
			deleteErr := accounts.DeleteAccount(ctx, "codex", "busy")
			otherErr := accounts.DeleteAccount(ctx, "codex", "other")
			close(blocking.release)
			updateErr := <-done
			if !isAppErrorCode(deleteErr, entity.ErrorCodeOperationInProgress) {
				t.Fatalf("delete during %s = %v, want OPERATION_IN_PROGRESS", operation, deleteErr)
			}
			if otherErr != nil {
				t.Fatalf("unrelated account deletion was blocked: %v", otherErr)
			}
			if updateErr != nil {
				t.Fatalf("in-progress %s failed: %v", operation, updateErr)
			}
			if err := accounts.DeleteAccount(ctx, "codex", "busy"); err != nil {
				t.Fatalf("delete after %s finished: %v", operation, err)
			}
			if _, err := daos.Accounts.Get(ctx, "codex", "busy"); !isAppErrorCode(err, entity.ErrorCodeNotFound) {
				t.Fatalf("account was not deleted: %v", err)
			}
		})
	}
}

type blockingCredentialUpdateProvider struct {
	*fake.Provider
	started chan struct{}
	release chan struct{}
}

func (blocking *blockingCredentialUpdateProvider) RefreshAccountWithMetadata(ctx context.Context, account entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	close(blocking.started)
	<-blocking.release
	return blocking.Provider.RefreshAccountWithMetadata(ctx, account)
}

func (blocking *blockingCredentialUpdateProvider) ResetAccountRateLimit(ctx context.Context, account entity.Account, key string) (provider.RateLimitResetResult, error) {
	close(blocking.started)
	<-blocking.release
	return blocking.Provider.ResetAccountRateLimit(ctx, account, key)
}
