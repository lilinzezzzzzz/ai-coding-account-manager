package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/codexruntime"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func TestAccountsRemainManageableWithoutCodexRuntime(t *testing.T) {
	t.Setenv("PATH", "")
	for _, active := range []bool{false, true} {
		name := "inactive"
		if active {
			name = "active"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			cfg := config.Config{CredentialsDir: filepath.Join(root, "credentials"), CodexHome: filepath.Join(root, "active")}
			store, err := credentials.NewStore(credentials.Config{RootDir: cfg.CredentialsDir, ActiveCodexDir: cfg.CodexHome})
			if err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"tokens":{"access_token":"test-only"}}`)
			if err := store.ImportAuthJSONToCodexDir(ctx, cfg.CodexHome, original); err != nil {
				t.Fatal(err)
			}
			registry := provider.NewRegistry()
			resolver := codexruntime.NewResolver(codexruntime.Config{ApplicationDirs: []string{}})
			codexProvider, err := registerProviders(ctx, registry, cfg, resolver)
			if err != nil || codexProvider == nil {
				t.Fatalf("provider must remain registered: %v", err)
			}
			descriptions := registry.DescribeAll(ctx)
			if len(descriptions) != 1 || descriptions[0].Status != provider.StatusUnavailable {
				t.Fatalf("provider status = %+v", descriptions)
			}
			db, err := database.Open(ctx, database.Config{Path: filepath.Join(root, "state.db")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			daos := dao.NewDAOs(db.GORM())
			account := entity.Account{ProviderID: "codex", AccountID: "test-account", StorageID: "test-storage", Label: "test", IsActive: active}
			if err := daos.Accounts.Create(ctx, account); err != nil {
				t.Fatal(err)
			}
			if err := store.ImportFromCodexDir(ctx, account.ProviderID, account.StorageID, cfg.CodexHome); err != nil {
				t.Fatal(err)
			}
			accounts := service.NewAccountService(dao.NewUnitOfWork(db.GORM()), daos, registry, service.NewAccountActivationCoordinator())
			result, err := accounts.RefreshAccount(ctx, account.ProviderID, account.AccountID)
			if err != nil || result.ErrorCode == nil || *result.ErrorCode != "UNAVAILABLE" || result.ErrorMessage == nil || !strings.Contains(*result.ErrorMessage, "Codex runtime") {
				t.Fatalf("refresh must explain unavailable runtime: %+v, %v", result, err)
			}
			if !active {
				// 已丢失隔离凭据时仍可删除管理器记录。
				if err := store.RemoveAccount(ctx, account.ProviderID, account.StorageID); err != nil {
					t.Fatal(err)
				}
			}
			if err := accounts.DeleteAccount(ctx, account.ProviderID, account.AccountID); err != nil {
				t.Fatal(err)
			}
			if _, err := daos.Accounts.Get(ctx, account.ProviderID, account.AccountID); err == nil {
				t.Fatal("account still exists")
			}
			if _, err := daos.UsageSnapshots.Get(ctx, account.ProviderID, account.AccountID); err == nil {
				t.Fatal("usage still exists")
			}
			dir, err := store.AccountCodexDir(account.ProviderID, account.StorageID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("isolated credentials still exist: %v", err)
			}
			current, err := os.ReadFile(filepath.Join(cfg.CodexHome, "auth.json"))
			if err != nil || string(current) != string(original) {
				t.Fatal("deleting a managed account changed the client's current credentials")
			}
		})
	}
}
