package middleware

import (
	"mime"
	"net/http"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

const (
	csrfHeaderName = "X-CSRF-Token"
	jsonMediaType  = "application/json"
	maxBodyBytes   = 16 * 1024
)

// RequireHost 拒绝非当前本地服务 Host 的请求。
func RequireHost(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !manager.ValidateHost(r.Host) {
				httptransport.WriteErrorWithStatus(w, entity.NewAppError(entity.ErrorCodeForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSession 要求请求携带有效 session Cookie。
func RequireSession(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, ok := manager.SessionFromRequest(r, time.Now())
			if !ok {
				httptransport.WriteErrorWithStatus(w, entity.NewAppError(entity.ErrorCodeUnauthorized), http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(security.WithSession(r.Context(), session)))
		})
	}
}

// RequireOrigin 拒绝非同源写请求。
func RequireOrigin(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !manager.ValidateOriginForHost(r.Header.Get("Origin"), r.Host) {
				httptransport.WriteErrorWithStatus(w, entity.NewAppError(entity.ErrorCodeForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireCSRF 要求写请求携带与 session 绑定的 CSRF token。
func RequireCSRF(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, ok := security.SessionFromContext(r.Context())
			if !ok || !manager.ValidateCSRF(session, r.Header.Get(csrfHeaderName)) {
				httptransport.WriteErrorWithStatus(w, entity.NewAppError(entity.ErrorCodeForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireJSONContentType 要求请求体使用 application/json。
func RequireJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != jsonMediaType {
			httptransport.WriteErrorWithStatus(w, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "Content-Type 必须是 application/json"), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// LimitBodySize 将 JSON 请求体限制为 16 KiB。
func LimitBodySize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}
