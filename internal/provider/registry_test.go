package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

func TestRegistryDescribeAllIsolatesProviderFailure(t *testing.T) {
	registry := NewRegistry()
	stable := &stubProvider{description: Description{ID: "stable", DisplayName: "Stable"}}
	flaky := &stubProvider{description: Description{ID: "flaky", DisplayName: "Flaky"}}

	if err := registry.Register(context.Background(), stable); err != nil {
		t.Fatalf("Register(stable) error = %v", err)
	}
	if err := registry.Register(context.Background(), flaky); err != nil {
		t.Fatalf("Register(flaky) error = %v", err)
	}
	flaky.describeErr = errors.New("provider unavailable")

	descriptions := registry.DescribeAll(context.Background())
	if len(descriptions) != 2 {
		t.Fatalf("description count = %d, want 2", len(descriptions))
	}
	if descriptions[0].ID != "flaky" || descriptions[0].Status != StatusUnavailable {
		t.Fatalf("first description = %+v, want flaky unavailable", descriptions[0])
	}
	if descriptions[0].ErrorCode == nil || *descriptions[0].ErrorCode != entity.ErrorCodeUnavailable {
		t.Fatalf("flaky error code = %v, want UNAVAILABLE", descriptions[0].ErrorCode)
	}
	if descriptions[1].ID != "stable" || descriptions[1].Status != StatusAvailable {
		t.Fatalf("second description = %+v, want stable available", descriptions[1])
	}
}

func TestRegistryRegisterUnavailable(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterUnavailable(Description{ID: "codex", DisplayName: "Codex"}, errors.New("missing cli")); err != nil {
		t.Fatalf("RegisterUnavailable() error = %v", err)
	}

	if _, ok := registry.Get("codex"); ok {
		t.Fatal("Get(codex) ok = true, want false for unavailable provider")
	}
	descriptions := registry.DescribeAll(context.Background())
	if len(descriptions) != 1 {
		t.Fatalf("description count = %d, want 1", len(descriptions))
	}
	if descriptions[0].Status != StatusUnavailable {
		t.Fatalf("status = %q, want unavailable", descriptions[0].Status)
	}
}

func TestRegistryCloseClosesProviders(t *testing.T) {
	registry := NewRegistry()
	first := &stubProvider{description: Description{ID: "first"}}
	second := &stubProvider{description: Description{ID: "second"}}

	if err := registry.Register(context.Background(), first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := registry.Register(context.Background(), second); err != nil {
		t.Fatalf("Register(second) error = %v", err)
	}
	if err := registry.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if first.closed != 1 || second.closed != 1 {
		t.Fatalf("closed counts = %d/%d, want 1/1", first.closed, second.closed)
	}
}

type stubProvider struct {
	description Description
	describeErr error
	closed      int
}

func (stub *stubProvider) Describe(context.Context) (Description, error) {
	if stub.describeErr != nil {
		return Description{}, stub.describeErr
	}
	return stub.description, nil
}

func (stub *stubProvider) RefreshAccount(context.Context, entity.Account) (*entity.UsageSnapshot, error) {
	return nil, Unsupported()
}

func (stub *stubProvider) ActivateAccount(context.Context, entity.Account) error {
	return Unsupported()
}

func (stub *stubProvider) RemoveAccountData(context.Context, entity.Account) error {
	return Unsupported()
}

func (stub *stubProvider) Close(context.Context) error {
	stub.closed++
	return nil
}
