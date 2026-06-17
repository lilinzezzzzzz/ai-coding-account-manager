package controller

import (
	"net/http"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

type exchangeBootstrapRequest struct {
	BootstrapToken string `json:"bootstrapToken"`
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrfToken"`
}

// SessionController 处理本地管理页面会话请求。
type SessionController struct {
	securityManager *security.Manager
}

// NewSessionController 创建会话 controller。
func NewSessionController(securityManager *security.Manager) SessionController {
	return SessionController{securityManager: securityManager}
}

// GetSession 返回当前 session 状态和 CSRF token。
func (controller SessionController) GetSession(w http.ResponseWriter, r *http.Request) error {
	session, ok := security.SessionFromContext(r.Context())
	if !ok {
		return entity.NewAppError(entity.ErrorCodeUnauthorized)
	}

	httptransport.WriteOK(w, sessionResponse{
		Authenticated: true,
		CSRFToken:     session.CSRFToken,
	})
	return nil
}

// ExchangeBootstrap 使用一次性 bootstrap token 兑换浏览器 session。
func (controller SessionController) ExchangeBootstrap(w http.ResponseWriter, r *http.Request) error {
	var request exchangeBootstrapRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	if request.BootstrapToken == "" {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "bootstrapToken 不能为空")
	}

	session, err := controller.securityManager.ExchangeBootstrap(request.BootstrapToken, time.Now())
	if err != nil {
		return err
	}
	http.SetCookie(w, security.CookieForSession(session))
	httptransport.WriteOK(w, sessionResponse{
		Authenticated: true,
		CSRFToken:     session.CSRFToken,
	})
	return nil
}
