package service

import (
	"context"
	"errors"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
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
