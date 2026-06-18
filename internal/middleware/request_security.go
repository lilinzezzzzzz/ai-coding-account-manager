package middleware

import (
	"mime"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

const (
	jsonMediaType = "application/json"
	maxBodyBytes  = 16 * 1024
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
	return LimitBodyBytes(maxBodyBytes)(next)
}

// LimitBodyBytes 将请求体限制为指定字节数。
func LimitBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
