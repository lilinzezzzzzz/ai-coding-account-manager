package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/codexruntime"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/loginrunner"
)

func TestLoginTaskCleansUpWhenAccountIsMissing(t *testing.T) {
	root := t.TempDir()
	type startedRun struct {
		finished <-chan struct{}
		input    loginrunner.Input
	}
	started := make(chan startedRun, 1)
	service, err := NewLoginTaskService(LoginTaskConfig{
		Resolver: codexruntime.NewResolver(codexruntime.Config{
			ConfiguredBin:   filepath.Join(root, "test-codex"),
			ApplicationDirs: []string{},
			Validator:       testLoginRuntimeValidator{},
		}),
		Runner: testLoginRunner(func(ctx context.Context, input loginrunner.Input) (loginrunner.Result, error) {
			err := os.MkdirAll(input.CodexHome, 0o700)
			started <- startedRun{finished: ctx.Done(), input: input}
			return loginrunner.Result{}, err
		}),
		Importer:   &trackingLoginAccountImporter{},
		RootDir:    root,
		Activation: NewAccountActivationCoordinator(),
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Create(t.Context(), CreateLoginTaskInput{ProviderID: "codex", Mode: loginrunner.ModeDeviceCode})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	var run startedRun
	select {
	case run = <-started:
	case <-deadline.C:
		t.Fatal("login runner did not start")
	}
	select {
	case <-run.finished:
	case <-deadline.C:
		t.Fatal("completed login task did not release its context")
	}
	if run.input.Mode != loginrunner.ModeDeviceCode {
		t.Fatalf("login mode = %s, want device_code", run.input.Mode)
	}
	if _, err := os.Stat(filepath.Dir(run.input.CodexHome)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("task directory still present or inaccessible: %v", err)
	}
	result, err := service.Get(t.Context(), "codex", task.TaskID)
	if err != nil || result.Status != LoginTaskStatusFailed || result.ErrorCode == nil || *result.ErrorCode != entity.ErrorCodeUnavailable {
		t.Fatalf("login task returned %+v, %v, want failed with UNAVAILABLE", result, err)
	}
}

type testLoginRunner func(context.Context, loginrunner.Input) (loginrunner.Result, error)

func (run testLoginRunner) Run(ctx context.Context, input loginrunner.Input) (loginrunner.Result, error) {
	return run(ctx, input)
}

type testLoginRuntimeValidator struct{}

func (testLoginRuntimeValidator) Validate(context.Context, string) error { return nil }

func TestPersistAndSyncImportedAccount(t *testing.T) {
	tests := []struct {
		name          string
		isActive      bool
		activateErr   error
		wantCalls     int
		wantErrorCode entity.ErrorCode
	}{
		{name: "active account", isActive: true, wantCalls: 1},
		{name: "inactive account", isActive: false, wantCalls: 0},
		{name: "activation failure", isActive: true, activateErr: errors.New("write auth.json"), wantCalls: 1, wantErrorCode: entity.ErrorCodeUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			appDB, err := database.Open(ctx, database.Config{Path: filepath.Join(t.TempDir(), "state.db")})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			t.Cleanup(func() {
				if err := appDB.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			})
			daos := dao.NewDAOs(appDB.GORM())
			account := entity.Account{
				ProviderID: "codex",
				AccountID:  "acct-1",
				StorageID:  entity.StorageIDForAccount("codex", "acct-1"),
				Label:      "acct-1",
				IsActive:   test.isActive,
				CreatedAt:  1000,
				UpdatedAt:  1000,
			}
			if err := daos.Accounts.Create(ctx, account); err != nil {
				t.Fatalf("seed account error = %v", err)
			}

			importer := &trackingLoginAccountImporter{activateErr: test.activateErr}
			service := LoginTaskService{
				uow:        dao.NewUnitOfWork(appDB.GORM()),
				daos:       daos,
				importer:   importer,
				activation: NewAccountActivationCoordinator(),
				now:        time.Now,
			}
			staleAccount := account
			staleAccount.IsActive = !account.IsActive

			_, err = service.persistAndSyncImportedAccount(ctx, staleAccount)
			if importer.activateCalls != test.wantCalls {
				t.Fatalf("activate calls = %d, want %d", importer.activateCalls, test.wantCalls)
			}
			if test.wantErrorCode == "" {
				if err != nil {
					t.Fatalf("persistAndSyncImportedAccount() error = %v", err)
				}
				return
			}
			appErr, ok := entity.AsAppError(err)
			if !ok || appErr.ErrorCode() != test.wantErrorCode {
				t.Fatalf("persistAndSyncImportedAccount() error = %v, want %s", err, test.wantErrorCode)
			}
		})
	}
}

type trackingLoginAccountImporter struct {
	activateCalls int
	activateErr   error
}

func (*trackingLoginAccountImporter) ReadAccountFromCodexDir(context.Context, string) (*entity.Account, error) {
	return nil, nil
}

func (*trackingLoginAccountImporter) ImportAccountAuthFromCodexDir(context.Context, entity.Account, string) error {
	return nil
}

func (importer *trackingLoginAccountImporter) ActivateAccount(context.Context, entity.Account) error {
	importer.activateCalls++
	return importer.activateErr
}
