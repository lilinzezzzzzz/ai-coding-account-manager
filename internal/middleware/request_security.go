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

// Chain 按声明顺序组合多个中间件。
func Chain(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for index := len(middlewares) - 1; index >= 0; index-- {
			next = middlewares[index](next)
		}
		return next
	}
}

// RequireHost 拒绝非当前本地服务 Host 的请求。
func RequireHost(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !manager.ValidateHost(r.Host) {
				httptransport.WriteErrorWithStatus(r.Context(), w, entity.NewAppError(entity.ErrorCodeForbidden), http.StatusForbidden)
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
				httptransport.WriteErrorWithStatus(r.Context(), w, entity.NewAppError(entity.ErrorCodeForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Mutation 保护不要求 JSON 请求体的写操作。
func Mutation(manager *security.Manager) func(http.Handler) http.Handler {
	return Chain(
		RequireOrigin(manager),
	)
}

// JSONMutation 保护使用默认请求体大小限制的 JSON 写操作。
func JSONMutation(manager *security.Manager) func(http.Handler) http.Handler {
	return Chain(
		RequireOrigin(manager),
		RequireJSONContentType,
		LimitBodySize,
	)
}

// JSONMutationWithLimit 保护使用自定义请求体大小限制的 JSON 写操作。
func JSONMutationWithLimit(manager *security.Manager, limit int64) func(http.Handler) http.Handler {
	return Chain(
		RequireOrigin(manager),
		RequireJSONContentType,
		LimitBodyBytes(limit),
	)
}

// RequireJSONContentType 要求请求体使用 application/json。
func RequireJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != jsonMediaType {
			httptransport.WriteErrorWithStatus(r.Context(), w, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "Content-Type 必须是 application/json"), http.StatusBadRequest)
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
