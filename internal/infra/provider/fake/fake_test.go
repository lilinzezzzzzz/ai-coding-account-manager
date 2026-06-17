package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func TestFakeProviderRefreshesConfiguredUsageStates(t *testing.T) {
	states := []entity.UsageStatus{
		entity.UsageStatusReady,
		entity.UsageStatusRefreshing,
		entity.UsageStatusAuthExpired,
		entity.UsageStatusRateLimitReached,
		entity.UsageStatusUnavailable,
		entity.UsageStatusUnsupported,
	}

	configured := make([]fake.AccountState, 0, len(states))
	for _, status := range states {
		accountID := string(status)
		configured = append(configured, fake.AccountState{
			Account: testAccount("fake", accountID),
			Usage: entity.UsageSnapshot{
				ProviderID:  "fake",
				AccountID:   accountID,
				Status:      status,
				RefreshedAt: 1000,
			},
		})
	}
	fakeProvider := fake.New(fake.Config{Accounts: configured})

	for _, status := range states {
		account := testAccount("fake", string(status))
		snapshot, err := fakeProvider.RefreshAccount(context.Background(), account)
		if err != nil {
			t.Fatalf("RefreshAccount(%s) error = %v", status, err)
		}
		if snapshot.Status != status {
			t.Fatalf("RefreshAccount(%s).Status = %q, want %q", status, snapshot.Status, status)
		}
	}
}

func TestFakeProviderUnsupportedCapability(t *testing.T) {
	fakeProvider := fake.New(fake.Config{
		Capabilities: provider.Capabilities{
			CanLogin:        true,
			CanRefreshUsage: false,
		},
		Accounts: []fake.AccountState{{
			Account: testAccount("fake", "acct-1"),
		}},
	})

	_, err := fakeProvider.RefreshAccount(context.Background(), testAccount("fake", "acct-1"))
	assertAppErrorCode(t, err, entity.ErrorCodeUnsupported)
}

func TestFakeProviderUnavailable(t *testing.T) {
	fakeProvider := fake.New(fake.Config{
		Unavailable: true,
		Accounts: []fake.AccountState{{
			Account: testAccount("fake", "acct-1"),
		}},
	})

	description, err := fakeProvider.Describe(context.Background())
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if description.Status != provider.StatusUnavailable {
		t.Fatalf("Status = %q, want unavailable", description.Status)
	}

	_, err = fakeProvider.DiscoverCurrentAccount(context.Background())
	assertAppErrorCode(t, err, entity.ErrorCodeUnavailable)
}

func TestFakeProviderCurrentActivateRemoveAndLogin(t *testing.T) {
	first := testAccount("fake", "acct-1")
	second := testAccount("fake", "acct-2")
	fakeProvider := fake.New(fake.Config{
		Accounts: []fake.AccountState{
			{Account: first, IsCurrent: true},
			{Account: second},
		},
	})

	current, err := fakeProvider.DiscoverCurrentAccount(context.Background())
	if err != nil {
		t.Fatalf("DiscoverCurrentAccount() error = %v", err)
	}
	if current.AccountID != "acct-1" {
		t.Fatalf("current account = %q, want acct-1", current.AccountID)
	}

	if err := fakeProvider.ActivateAccount(context.Background(), second); err != nil {
		t.Fatalf("ActivateAccount() error = %v", err)
	}
	current, err = fakeProvider.DiscoverCurrentAccount(context.Background())
	if err != nil {
		t.Fatalf("DiscoverCurrentAccount() after activate error = %v", err)
	}
	if current.AccountID != "acct-2" {
		t.Fatalf("current account = %q, want acct-2", current.AccountID)
	}

	task, err := fakeProvider.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	status, err := fakeProvider.PollLogin(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("PollLogin() error = %v", err)
	}
	if status.State != provider.LoginStatePending {
		t.Fatalf("login state = %q, want pending", status.State)
	}
	if err := fakeProvider.CancelLogin(context.Background(), task.ID); err != nil {
		t.Fatalf("CancelLogin() error = %v", err)
	}
	status, err = fakeProvider.PollLogin(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("PollLogin() after cancel error = %v", err)
	}
	if status.State != provider.LoginStateCanceled {
		t.Fatalf("login state = %q, want canceled", status.State)
	}

	if err := fakeProvider.RemoveAccountData(context.Background(), second); err != nil {
		t.Fatalf("RemoveAccountData() error = %v", err)
	}
	_, err = fakeProvider.DiscoverCurrentAccount(context.Background())
	assertAppErrorCode(t, err, entity.ErrorCodeNotFound)
}

func testAccount(providerID string, accountID string) entity.Account {
	return entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      accountID,
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
}

func assertAppErrorCode(t *testing.T, err error, want entity.ErrorCode) {
	t.Helper()

	var appErr *entity.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError %s", err, want)
	}
	if appErr.ErrorCode() != want {
		t.Fatalf("ErrorCode() = %q, want %q", appErr.ErrorCode(), want)
	}
}
