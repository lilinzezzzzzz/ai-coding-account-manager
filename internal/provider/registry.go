package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

// Registry 保存 provider 实例并隔离单 provider 失败。
type Registry struct {
	mu           sync.RWMutex
	providers    map[string]Provider
	descriptions map[string]Description
}

// NewRegistry 创建空 provider registry。
func NewRegistry() *Registry {
	return &Registry{
		providers:    map[string]Provider{},
		descriptions: map[string]Description{},
	}
}

// Register 注册可用 provider。
func (registry *Registry) Register(ctx context.Context, provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is required")
	}

	description, err := provider.Describe(ctx)
	if err != nil {
		return fmt.Errorf("describe provider: %w", err)
	}
	if description.ID == "" {
		return fmt.Errorf("provider id is required")
	}
	if description.Status == "" {
		description.Status = StatusAvailable
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.providers[description.ID] = provider
	registry.descriptions[description.ID] = description
	return nil
}

// RegisterUnavailable 注册初始化失败但仍应展示的 provider。
func (registry *Registry) RegisterUnavailable(description Description, _ error) error {
	if description.ID == "" {
		return fmt.Errorf("provider id is required")
	}
	description.Status = StatusUnavailable
	if description.ErrorCode == nil {
		code := entity.ErrorCodeUnavailable
		description.ErrorCode = &code
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.descriptions[description.ID] = description
	delete(registry.providers, description.ID)
	return nil
}

// Get 返回可用 provider。
func (registry *Registry) Get(id string) (Provider, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	provider, ok := registry.providers[id]
	return provider, ok
}

// DescribeAll 返回全部 provider 描述。单个 provider Describe 失败会被隔离为 unavailable。
func (registry *Registry) DescribeAll(ctx context.Context) []Description {
	registry.mu.RLock()
	providers := make(map[string]Provider, len(registry.providers))
	descriptions := make(map[string]Description, len(registry.descriptions))
	for id, provider := range registry.providers {
		providers[id] = provider
	}
	for id, description := range registry.descriptions {
		descriptions[id] = description
	}
	registry.mu.RUnlock()

	for id, registeredProvider := range providers {
		description, err := registeredProvider.Describe(ctx)
		if err != nil {
			slog.Warn("provider describe failed", "provider_id", id, "error", err)
			description = descriptions[id]
			description.Status = StatusUnavailable
			code := entity.ErrorCodeUnavailable
			description.ErrorCode = &code
		} else if description.Status == "" {
			description.Status = StatusAvailable
		}
		descriptions[id] = description
	}

	result := make([]Description, 0, len(descriptions))
	for _, description := range descriptions {
		result = append(result, description)
	}
	sort.Slice(result, func(i int, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Close 关闭所有已注册 provider。
func (registry *Registry) Close(ctx context.Context) error {
	registry.mu.RLock()
	providers := make([]Provider, 0, len(registry.providers))
	for _, provider := range registry.providers {
		providers = append(providers, provider)
	}
	registry.mu.RUnlock()

	var closeErr error
	for _, provider := range providers {
		if err := provider.Close(ctx); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}
