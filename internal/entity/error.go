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

// DefaultMessage 返回错误码对应的默认安全文案。
func (code ErrorCode) DefaultMessage() string {
	switch code {
	case ErrorCodeValidationFailed:
		return "请求参数无效"
	case ErrorCodeNotFound:
		return "资源不存在"
	case ErrorCodeMethodNotAllowed:
		return "请求方法不支持"
	case ErrorCodeUnsupported:
		return "当前操作不支持"
	case ErrorCodeConflict:
		return "资源状态冲突"
	default:
		return "服务内部错误"
	}
}

// AppError 保存跨 service、dao 和 controller 传递的稳定业务错误。
type AppError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// NewAppError 使用 ErrorCode 的默认文案创建业务错误。
func NewAppError(code ErrorCode) *AppError {
	return &AppError{Code: code}
}

// NewAppErrorWithMessage 使用指定文案创建业务错误。
func NewAppErrorWithMessage(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// WrapAppError 使用 ErrorCode 的默认文案包装底层 cause。
func WrapAppError(code ErrorCode, cause error) *AppError {
	return &AppError{Code: code, Cause: cause}
}

// WrapAppErrorWithMessage 使用指定文案包装底层 cause。
func WrapAppErrorWithMessage(code ErrorCode, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

// ErrorCode 返回对外稳定错误码。
func (err *AppError) ErrorCode() ErrorCode {
	if err == nil {
		return ErrorCodeInternal
	}
	if err.Code == "" {
		return ErrorCodeInternal
	}
	return err.Code
}

// DisplayMessage 返回对外展示文案，优先使用场景覆盖文案。
func (err *AppError) DisplayMessage() string {
	if err == nil {
		return ErrorCodeInternal.DefaultMessage()
	}
	if err.Message != "" {
		return err.Message
	}
	return err.ErrorCode().DefaultMessage()
}

// Error 返回可用于日志的错误描述。
func (err *AppError) Error() string {
	if err == nil {
		return "<nil>"
	}
	return string(err.ErrorCode()) + ": " + err.DisplayMessage()
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
	if appErr, ok := errors.AsType[*AppError](err); ok {
		return appErr, true
	}
	return nil, false
}
