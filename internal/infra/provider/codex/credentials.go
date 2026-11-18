package codex

import (
	"context"
	"errors"
	"os"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

// ImportCurrentAccount 读取活动 CODEX_HOME 登录态，并把 auth.json 导入账号隔离目录。
func (providerImpl *Provider) ImportCurrentAccount(ctx context.Context) (*entity.Account, error) {
	activeDir := providerImpl.credentials.ActiveCodexDir()
	account, err := providerImpl.ReadAccountFromCodexDir(ctx, activeDir)
	if err != nil {
		return nil, err
	}
	if err := providerImpl.ImportAccountAuthFromCodexDir(ctx, *account, activeDir); err != nil {
		return nil, err
	}
	return account, nil
}

// ReadAccountFromCodexDir 使用指定 CODEX_HOME 读取 Codex 账号元数据。
func (providerImpl *Provider) ReadAccountFromCodexDir(ctx context.Context, codexHome string) (*entity.Account, error) {
	client, err := providerImpl.startClient(ctx, codexHome)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close(context.Background())
	}()
	return providerImpl.readAccount(ctx, client, false)
}

// ImportAccountAuthFromCodexDir 把指定 CODEX_HOME 的 auth.json 导入账号隔离目录。
func (providerImpl *Provider) ImportAccountAuthFromCodexDir(ctx context.Context, account entity.Account, codexHome string) error {
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, account.StorageID, codexHome); err != nil {
		return entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "导入 Codex auth.json 失败", err)
	}
	return nil
}

// ImportAccountAuthJSONAndRefresh 导入 auth.json，识别账号并刷新 usage。
func (providerImpl *Provider) ImportAccountAuthJSONAndRefresh(ctx context.Context, authJSON []byte) (*entity.Account, *entity.UsageSnapshot, error) {
	runtimeDir, err := createTemporaryCodexHome()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = os.RemoveAll(runtimeDir)
	}()
	if err := providerImpl.credentials.ImportAuthJSONToCodexDir(ctx, runtimeDir, authJSON); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "准备 Codex 临时 auth.json 失败", err)
	}

	refreshedAccount, snapshot, err := providerImpl.refreshCodexHome(ctx, runtimeDir, "")
	if err != nil {
		return nil, nil, err
	}
	if err := providerImpl.saveRefreshedCredentials(ctx, refreshedAccount.StorageID, runtimeDir); err != nil {
		return nil, nil, err
	}
	return refreshedAccount, snapshot, nil
}

// ExportAccountAuthJSON 仅导出所选账号的隔离凭据，不读取活动 CODEX_HOME。
func (providerImpl *Provider) ExportAccountAuthJSON(ctx context.Context, account entity.Account) ([]byte, error) {
	payload, err := providerImpl.credentials.ReadAccountAuthJSON(ctx, providerID, account.StorageID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "此账号没有可导出的登录凭据，请先登录或导入")
	}
	if err != nil {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "无法读取此账号的登录凭据，请检查凭据文件后重试")
	}
	return payload, nil
}

// ActivateAccount 替换活动 CODEX_HOME 的 auth.json。
func (providerImpl *Provider) ActivateAccount(ctx context.Context, account entity.Account) error {
	return providerImpl.credentials.ActivateAccount(ctx, providerID, account.StorageID)
}

// RemoveAccountData 删除账号隔离凭据。
func (providerImpl *Provider) RemoveAccountData(ctx context.Context, account entity.Account) error {
	return providerImpl.credentials.RemoveAccount(ctx, providerID, account.StorageID)
}

func createTemporaryCodexHome() (string, error) {
	dir, err := os.MkdirTemp("", "ai-coding-account-manager-codex-home-*")
	if err != nil {
		return "", entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "创建 Codex 临时运行目录失败", err)
	}
	return dir, nil
}

// prepareAccountCodexHome 成功时由调用方负责清理临时目录。
func (providerImpl *Provider) prepareAccountCodexHome(ctx context.Context, account entity.Account) (string, error) {
	if err := providerImpl.credentials.ValidateAccount(ctx, providerID, account.StorageID); err != nil {
		return "", entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "账号未导入 Codex auth.json，请先登录添加账号", err)
	}
	dir, err := createTemporaryCodexHome()
	if err != nil {
		return "", err
	}
	if err := providerImpl.credentials.ExportToCodexDir(ctx, providerID, account.StorageID, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "准备 Codex 临时 auth.json 失败", err)
	}
	return dir, nil
}

func (providerImpl *Provider) saveRefreshedCredentials(ctx context.Context, storageID string, codexHome string) error {
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, storageID, codexHome); err != nil {
		return entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "保存刷新后的 Codex auth.json 失败", err)
	}
	return nil
}
