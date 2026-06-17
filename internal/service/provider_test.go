package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func TestProviderServiceListProvidersIsolatesDescribeFailure(t *testing.T) {
	registry := provider.NewRegistry()
	available := fake.New(fake.Config{ID: "available", DisplayName: "Available"})
	flaky := &serviceStubProvider{description: provider.Description{ID: "flaky", DisplayName: "Flaky"}}

	if err := registry.Register(context.Background(), available); err != nil {
		t.Fatalf("Register(available) error = %v", err)
	}
	if err := registry.Register(context.Background(), flaky); err != nil {
		t.Fatalf("Register(flaky) error = %v", err)
	}
	flaky.describeErr = errors.New("describe failed")

	descriptions := NewProviderService(registry).ListProviders(context.Background())
	if len(descriptions) != 2 {
		t.Fatalf("description count = %d, want 2", len(descriptions))
	}
	if descriptions[0].ID != "available" || descriptions[0].Status != provider.StatusAvailable {
		t.Fatalf("first description = %+v, want available provider", descriptions[0])
	}
	if descriptions[1].ID != "flaky" || descriptions[1].Status != provider.StatusUnavailable {
		t.Fatalf("second description = %+v, want flaky unavailable", descriptions[1])
	}
}

func TestProviderServiceGetProviderNotFound(t *testing.T) {
	_, err := NewProviderService(provider.NewRegistry()).GetProvider("missing")
	assertProviderServiceErrorCode(t, err, entity.ErrorCodeNotFound)
}

func TestAccountServiceRefreshAllKeepsGoingWhenOneProviderFails(t *testing.T) {
	appDB, err := database.Open(context.Background(), database.Config{Path: filepath.Join(t.TempDir(), "state.db")})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		if err := appDB.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	daos := dao.NewDAOs(appDB.GORM())
	registry := provider.NewRegistry()
	goodProvider := fake.New(fake.Config{
		ID: "good",
		Accounts: []fake.AccountState{{
			Account: testServiceAccount("good", "ok"),
			Usage: entity.UsageSnapshot{
				ProviderID:  "good",
				AccountID:   "ok",
				Status:      entity.UsageStatusReady,
				RefreshedAt: 1000,
			},
		}},
	})
	if err := registry.Register(context.Background(), goodProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := daos.Accounts.Create(context.Background(), testServiceAccount("good", "ok")); err != nil {
		t.Fatalf("Create(good) error = %v", err)
	}
	if err := daos.Accounts.Create(context.Background(), testServiceAccount("missing", "failed")); err != nil {
		t.Fatalf("Create(missing) error = %v", err)
	}

	accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry)
	results, err := accountService.RefreshAllUsage(context.Background())
	if err != nil {
		t.Fatalf("RefreshAllUsage() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2", len(results))
	}
	if results[0].Usage == nil || results[0].ErrorCode != nil {
		t.Fatalf("first result = %+v, want successful usage", results[0])
	}
	if results[1].ErrorCode == nil || *results[1].ErrorCode != entity.ErrorCodeNotFound {
		t.Fatalf("second error code = %v, want NOT_FOUND", results[1].ErrorCode)
	}
}

type serviceStubProvider struct {
	description provider.Description
	describeErr error
}

func (stub *serviceStubProvider) Describe(context.Context) (provider.Description, error) {
	if stub.describeErr != nil {
		return provider.Description{}, stub.describeErr
	}
	return stub.description, nil
}

func (stub *serviceStubProvider) DiscoverCurrentAccount(context.Context) (*entity.Account, error) {
	return nil, provider.Unsupported()
}

func (stub *serviceStubProvider) StartLogin(context.Context) (*provider.LoginTask, error) {
	return nil, provider.Unsupported()
}

func (stub *serviceStubProvider) PollLogin(context.Context, string) (*provider.LoginStatus, error) {
	return nil, provider.Unsupported()
}

func (stub *serviceStubProvider) CancelLogin(context.Context, string) error {
	return provider.Unsupported()
}

func (stub *serviceStubProvider) RefreshAccount(context.Context, entity.Account) (*entity.UsageSnapshot, error) {
	return nil, provider.Unsupported()
}

func (stub *serviceStubProvider) ActivateAccount(context.Context, entity.Account) error {
	return provider.Unsupported()
}

func (stub *serviceStubProvider) RemoveAccountData(context.Context, entity.Account) error {
	return provider.Unsupported()
}

func (stub *serviceStubProvider) Close(context.Context) error {
	return nil
}

func assertProviderServiceErrorCode(t *testing.T, err error, want entity.ErrorCode) {
	t.Helper()

	var appErr *entity.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError %s", err, want)
	}
	if appErr.ErrorCode() != want {
		t.Fatalf("ErrorCode() = %q, want %q", appErr.ErrorCode(), want)
	}
}

func testServiceAccount(providerID string, accountID string) entity.Account {
	return entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      accountID,
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
}
