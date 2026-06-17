package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/appserver"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

const (
	providerID          = "codex"
	providerDisplayName = "OpenAI Codex"
	loginTTL            = 10 * time.Minute
)

type appServerClient interface {
	Call(context.Context, string, any, any) error
	Close(context.Context) error
}

// ClientFactory 创建 app-server client，测试可替换为 fake 实现。
type ClientFactory func(context.Context, appserver.Config) (appServerClient, error)

// Config 保存 Codex provider 依赖。
type Config struct {
	Bin           string
	CodexHome     string
	Credentials   *credentials.Store
	ClientFactory ClientFactory
	Now           func() time.Time
}

// Provider 实现 OpenAI Codex 账号 provider。
type Provider struct {
	bin         string
	codexHome   string
	credentials *credentials.Store
	newClient   ClientFactory
	now         func() time.Time

	mu         sync.Mutex
	loginTasks map[string]*loginTask
}

type loginTask struct {
	id         string
	authURL    string
	pendingDir string
	expiresAt  time.Time
	client     appServerClient
	cancel     context.CancelFunc
}

// New 创建 Codex provider。
func New(cfg Config) (*Provider, error) {
	if cfg.Credentials == nil {
		return nil, fmt.Errorf("credentials store is required")
	}
	codexHome := cfg.CodexHome
	if codexHome == "" {
		codexHome = cfg.Credentials.ActiveCodexDir()
	}
	newClient := cfg.ClientFactory
	if newClient == nil {
		newClient = func(ctx context.Context, cfg appserver.Config) (appServerClient, error) {
			return appserver.Start(ctx, cfg)
		}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Provider{
		bin:         cfg.Bin,
		codexHome:   codexHome,
		credentials: cfg.Credentials,
		newClient:   newClient,
		now:         now,
		loginTasks:  map[string]*loginTask{},
	}, nil
}

// Describe 返回 Codex provider 能力。
func (providerImpl *Provider) Describe(context.Context) (provider.Description, error) {
	return provider.Description{
		ID:          providerID,
		DisplayName: providerDisplayName,
		Capabilities: provider.Capabilities{
			CanImportCurrentAccount:           true,
			CanLogin:                          true,
			CanRefreshUsage:                   true,
			CanActivateAccount:                true,
			RequiresClientReloadAfterActivate: true,
		},
		Status: provider.StatusAvailable,
	}, nil
}

// DiscoverCurrentAccount 读取活动 CODEX_HOME 当前账号，并保存对应凭据。
func (providerImpl *Provider) DiscoverCurrentAccount(ctx context.Context) (*entity.Account, error) {
	client, err := providerImpl.startClient(ctx, providerImpl.codexHome)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	account, err := providerImpl.readAccount(ctx, client, false)
	if err != nil {
		return nil, err
	}
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, account.StorageID, providerImpl.codexHome); err != nil {
		return nil, entity.WrapAppError(entity.ErrorCodeUnavailable, err)
	}
	return account, nil
}

// StartLogin 创建 Codex ChatGPT 登录任务。
func (providerImpl *Provider) StartLogin(ctx context.Context) (*provider.LoginTask, error) {
	pendingDir, err := providerImpl.credentials.NewPendingCodexDir(providerID)
	if err != nil {
		return nil, entity.WrapAppError(entity.ErrorCodeUnavailable, err)
	}

	taskCtx, cancel := context.WithCancel(context.Background())
	client, err := providerImpl.startClient(taskCtx, pendingDir)
	if err != nil {
		cancel()
		_ = providerImpl.credentials.RemoveCodexDir(context.Background(), pendingDir)
		return nil, err
	}

	var response loginStartResponse
	if err := client.Call(ctx, "account/login/start", loginStartParams{Type: "chatgpt"}, &response); err != nil {
		_ = client.Close(context.Background())
		cancel()
		_ = providerImpl.credentials.RemoveCodexDir(context.Background(), pendingDir)
		return nil, mapAppServerError("start Codex login", err)
	}
	if response.Type != "chatgpt" || response.LoginID == "" || response.AuthURL == "" {
		_ = client.Close(context.Background())
		cancel()
		_ = providerImpl.credentials.RemoveCodexDir(context.Background(), pendingDir)
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 登录响应无效")
	}

	expiresAt := providerImpl.now().UTC().Add(loginTTL)
	providerImpl.mu.Lock()
	providerImpl.loginTasks[response.LoginID] = &loginTask{
		id:         response.LoginID,
		authURL:    response.AuthURL,
		pendingDir: pendingDir,
		expiresAt:  expiresAt,
		client:     client,
		cancel:     cancel,
	}
	providerImpl.mu.Unlock()

	return &provider.LoginTask{
		ID:        response.LoginID,
		AuthURL:   response.AuthURL,
		ExpiresAt: expiresAt.UnixMilli(),
	}, nil
}

// PollLogin 轮询登录任务。完成后把 pending 凭据迁移到正式账号目录。
func (providerImpl *Provider) PollLogin(ctx context.Context, taskID string) (*provider.LoginStatus, error) {
	task, err := providerImpl.getLoginTask(taskID)
	if err != nil {
		return nil, err
	}
	if providerImpl.now().After(task.expiresAt) {
		_ = providerImpl.finishLoginTask(taskID, true)
		code := entity.ErrorCodeUnavailable
		return &provider.LoginStatus{ID: taskID, State: provider.LoginStateFailed, ErrorCode: &code}, nil
	}

	account, err := providerImpl.readAccount(ctx, task.client, false)
	if err != nil {
		if appErr, ok := entity.AsAppError(err); ok && appErr.ErrorCode() == entity.ErrorCodeUnavailable {
			return &provider.LoginStatus{ID: taskID, State: provider.LoginStatePending}, nil
		}
		code := errorCode(err)
		return &provider.LoginStatus{ID: taskID, State: provider.LoginStateFailed, ErrorCode: &code}, nil
	}
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, account.StorageID, task.pendingDir); err != nil {
		code := entity.ErrorCodeUnavailable
		return &provider.LoginStatus{ID: taskID, State: provider.LoginStateFailed, ErrorCode: &code}, nil
	}
	_ = providerImpl.finishLoginTask(taskID, true)
	return &provider.LoginStatus{ID: taskID, State: provider.LoginStateCompleted, Account: account}, nil
}

// CancelLogin 取消登录任务并清理 pending 目录。
func (providerImpl *Provider) CancelLogin(ctx context.Context, taskID string) error {
	if err := providerImpl.finishLoginTask(taskID, true); err != nil {
		return err
	}
	return ctx.Err()
}

// RefreshAccount 使用账号隔离 CODEX_HOME 刷新 usage。
func (providerImpl *Provider) RefreshAccount(ctx context.Context, account entity.Account) (*entity.UsageSnapshot, error) {
	accountDir, err := providerImpl.credentials.AccountCodexDir(providerID, account.StorageID)
	if err != nil {
		return nil, entity.WrapAppError(entity.ErrorCodeUnavailable, err)
	}
	client, err := providerImpl.startClient(ctx, accountDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	if _, err := providerImpl.readAccount(ctx, client, true); err != nil {
		return nil, err
	}

	var response rateLimitsReadResponse
	if err := client.Call(ctx, "account/rateLimits/read", map[string]any{}, &response); err != nil {
		return nil, mapAppServerError("read Codex rate limits", err)
	}
	return mapUsageSnapshot(account, response, providerImpl.now().UTC().UnixMilli())
}

// ActivateAccount 替换活动 CODEX_HOME 的 auth.json。
func (providerImpl *Provider) ActivateAccount(ctx context.Context, account entity.Account) error {
	return providerImpl.credentials.ActivateAccount(ctx, providerID, account.StorageID)
}

// RemoveAccountData 删除账号隔离凭据。
func (providerImpl *Provider) RemoveAccountData(ctx context.Context, account entity.Account) error {
	return providerImpl.credentials.RemoveAccount(ctx, providerID, account.StorageID)
}

// Close 关闭所有未完成登录任务。
func (providerImpl *Provider) Close(ctx context.Context) error {
	providerImpl.mu.Lock()
	taskIDs := make([]string, 0, len(providerImpl.loginTasks))
	for taskID := range providerImpl.loginTasks {
		taskIDs = append(taskIDs, taskID)
	}
	providerImpl.mu.Unlock()

	for _, taskID := range taskIDs {
		_ = providerImpl.finishLoginTask(taskID, true)
	}
	return ctx.Err()
}

func (providerImpl *Provider) startClient(ctx context.Context, codexHome string) (appServerClient, error) {
	client, err := providerImpl.newClient(ctx, appserver.Config{
		Bin:       providerImpl.bin,
		CodexHome: codexHome,
	})
	if err != nil {
		return nil, mapAppServerError("start Codex app-server", err)
	}
	return client, nil
}

func (providerImpl *Provider) readAccount(ctx context.Context, client appServerClient, refreshToken bool) (*entity.Account, error) {
	var response accountReadResponse
	err := client.Call(ctx, "account/read", accountReadParams{RefreshToken: refreshToken}, &response)
	if err != nil {
		return nil, mapAppServerError("read Codex account", err)
	}
	return mapAccount(response)
}

func (providerImpl *Provider) getLoginTask(taskID string) (*loginTask, error) {
	providerImpl.mu.Lock()
	defer providerImpl.mu.Unlock()

	task, ok := providerImpl.loginTasks[taskID]
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return task, nil
}

func (providerImpl *Provider) finishLoginTask(taskID string, removePending bool) error {
	providerImpl.mu.Lock()
	task, ok := providerImpl.loginTasks[taskID]
	if ok {
		delete(providerImpl.loginTasks, taskID)
	}
	providerImpl.mu.Unlock()
	if !ok {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	_ = task.client.Close(context.Background())
	task.cancel()
	if removePending {
		_ = providerImpl.credentials.RemoveCodexDir(context.Background(), task.pendingDir)
	}
	return nil
}

func mapAccount(response accountReadResponse) (*entity.Account, error) {
	if response.Account == nil || response.RequiresOpenaiAuth {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 账号未登录")
	}
	if response.Account.Type != "chatgpt" {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnsupported, "当前 Codex 账号类型不支持")
	}
	email := strings.TrimSpace(response.Account.Email)
	if email == "" {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 账号缺少 email")
	}
	planType := strings.TrimSpace(response.Account.PlanType)
	accountID := accountIDFromEmail(email)
	account := entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      email,
		Email:      &email,
	}
	if planType != "" {
		account.PlanType = &planType
	}
	return &account, nil
}

func mapUsageSnapshot(account entity.Account, response rateLimitsReadResponse, refreshedAt int64) (*entity.UsageSnapshot, error) {
	status := entity.UsageStatusReady
	if response.RateLimits.RateLimitReachedType != nil {
		status = entity.UsageStatusRateLimitReached
	}

	var usedPercent *float64
	var resetsAt *int64
	if response.RateLimits.Primary != nil {
		usedPercent = response.RateLimits.Primary.UsedPercent
		resetsAt = response.RateLimits.Primary.ResetsAt
	}
	snapshotPayload, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode Codex rate limit snapshot: %w", err)
	}
	snapshotJSON := string(snapshotPayload)
	return &entity.UsageSnapshot{
		ProviderID:   account.ProviderID,
		AccountID:    account.AccountID,
		Status:       status,
		UsedPercent:  usedPercent,
		ResetsAt:     resetsAt,
		SnapshotJSON: &snapshotJSON,
		RefreshedAt:  refreshedAt,
	}, nil
}

func accountIDFromEmail(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "chatgpt-" + hex.EncodeToString(sum[:])[:20]
}

func mapAppServerError(message string, err error) error {
	if err == nil {
		return nil
	}
	return entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, message, err)
}

func errorCode(err error) entity.ErrorCode {
	if appErr, ok := entity.AsAppError(err); ok {
		return appErr.ErrorCode()
	}
	return entity.ErrorCodeInternal
}

type accountReadParams struct {
	RefreshToken bool `json:"refreshToken"`
}

type accountReadResponse struct {
	Account            *codexAccount `json:"account"`
	RequiresOpenaiAuth bool          `json:"requiresOpenaiAuth"`
}

type codexAccount struct {
	Type     string `json:"type"`
	Email    string `json:"email"`
	PlanType string `json:"planType"`
}

type loginStartParams struct {
	Type string `json:"type"`
}

type loginStartResponse struct {
	Type    string `json:"type"`
	LoginID string `json:"loginId"`
	AuthURL string `json:"authUrl"`
}

type rateLimitsReadResponse struct {
	RateLimits rateLimitSnapshot `json:"rateLimits"`
}

type rateLimitSnapshot struct {
	Primary              *rateLimitWindow `json:"primary"`
	Secondary            *rateLimitWindow `json:"secondary"`
	PlanType             *string          `json:"planType"`
	RateLimitReachedType *string          `json:"rateLimitReachedType"`
}

type rateLimitWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	ResetsAt           *int64   `json:"resetsAt"`
	WindowDurationMins *int64   `json:"windowDurationMins"`
}
