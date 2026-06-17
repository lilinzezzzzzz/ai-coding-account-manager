package service

import (
	"context"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// ProviderService 封装 provider registry 的业务入口。
type ProviderService struct {
	registry *provider.Registry
}

// NewProviderService 创建 provider service。
func NewProviderService(registry *provider.Registry) ProviderService {
	return ProviderService{registry: registry}
}

// ListProviders 返回全部 provider 描述，并隔离单 provider 失败。
func (service ProviderService) ListProviders(ctx context.Context) []provider.Description {
	return service.registry.DescribeAll(ctx)
}

// GetProvider 返回可用 provider，不存在或不可用时返回稳定错误。
func (service ProviderService) GetProvider(id string) (provider.Provider, error) {
	registeredProvider, ok := service.registry.Get(id)
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return registeredProvider, nil
}

// Close 关闭全部可用 provider。
func (service ProviderService) Close(ctx context.Context) error {
	return service.registry.Close(ctx)
}
