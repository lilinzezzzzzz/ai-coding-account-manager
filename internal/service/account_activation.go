package service

import "context"

// AccountActivationCoordinator 串行化活动账号状态与活动 CODEX_HOME 凭据的更新。
type AccountActivationCoordinator struct {
	token chan struct{}
}

// NewAccountActivationCoordinator 创建活动账号协调器。
func NewAccountActivationCoordinator() *AccountActivationCoordinator {
	coordinator := &AccountActivationCoordinator{token: make(chan struct{}, 1)}
	coordinator.token <- struct{}{}
	return coordinator
}

func (coordinator *AccountActivationCoordinator) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-coordinator.token:
		return nil
	}
}

func (coordinator *AccountActivationCoordinator) tryAcquire() bool {
	select {
	case <-coordinator.token:
		return true
	default:
		return false
	}
}

func (coordinator *AccountActivationCoordinator) release() {
	coordinator.token <- struct{}{}
}
