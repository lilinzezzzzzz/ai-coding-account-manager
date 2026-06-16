package entity

import "errors"

// ErrorCode 是对外稳定的业务错误码。
type ErrorCode string

const (
	// ErrorCodeValidationFailed 表示请求参数或输入数据不符合约束。
	ErrorCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	// ErrorCodeNotFound 表示目标资源不存在。
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrorCodeMethodNotAllowed 表示当前资源不支持该 HTTP method。
	ErrorCodeMethodNotAllowed ErrorCode = "METHOD_NOT_ALLOWED"
	// ErrorCodeUnsupported 表示 provider 或当前阶段不支持该操作。
	ErrorCodeUnsupported ErrorCode = "UNSUPPORTED"
	// ErrorCodeConflict 表示请求与当前资源状态冲突。
	ErrorCodeConflict ErrorCode = "CONFLICT"
	// ErrorCodeInternal 表示服务端未归类错误。
	ErrorCodeInternal ErrorCode = "INTERNAL"
)

// AppError 保存跨 service、dao 和 controller 传递的稳定业务错误。
type AppError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// NewAppError 创建不包装底层 cause 的业务错误。
func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// WrapAppError 创建包装底层 cause 的业务错误。
func WrapAppError(code ErrorCode, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

// Error 返回可用于日志的错误描述。
func (err *AppError) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Message != "" {
		return string(err.Code) + ": " + err.Message
	}
	return string(err.Code)
}

// Unwrap 返回底层 cause，支持 errors.Is 和 errors.As。
func (err *AppError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// AsAppError 从普通 error 中提取 AppError。
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
