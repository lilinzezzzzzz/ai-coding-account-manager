package entity

import "errors"

// ErrorCode 是对外稳定的业务错误码。
type ErrorCode string

const (
	// ErrorCodeUnauthorized 表示请求缺少有效会话。
	ErrorCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrorCodeForbidden 表示请求被本地安全边界拒绝。
	ErrorCodeForbidden ErrorCode = "FORBIDDEN"
	// ErrorCodeValidationFailed 表示请求参数或输入数据不符合约束。
	ErrorCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	// ErrorCodePayloadTooLarge 表示请求体超过允许大小。
	ErrorCodePayloadTooLarge ErrorCode = "PAYLOAD_TOO_LARGE"
	// ErrorCodeNotFound 表示目标资源不存在。
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrorCodeMethodNotAllowed 表示当前资源不支持该 HTTP method。
	ErrorCodeMethodNotAllowed ErrorCode = "METHOD_NOT_ALLOWED"
	// ErrorCodeUnsupported 表示 provider 或当前阶段不支持该操作。
	ErrorCodeUnsupported ErrorCode = "UNSUPPORTED"
	// ErrorCodeUnavailable 表示 provider 或依赖当前不可用。
	ErrorCodeUnavailable ErrorCode = "UNAVAILABLE"
	// ErrorCodeConflict 表示请求与当前资源状态冲突。
	ErrorCodeConflict ErrorCode = "CONFLICT"
	// ErrorCodeOperationInProgress 表示同类互斥操作正在执行。
	ErrorCodeOperationInProgress ErrorCode = "OPERATION_IN_PROGRESS"
	// ErrorCodeStorageBusy 表示 SQLite 当前无法取得写入锁。
	ErrorCodeStorageBusy ErrorCode = "STORAGE_BUSY"
	// ErrorCodeStorageCorrupted 表示 SQLite quick check 发现数据损坏。
	ErrorCodeStorageCorrupted ErrorCode = "STORAGE_CORRUPTED"
	// ErrorCodeSchemaTooNew 表示数据库 schema 版本高于当前程序支持版本。
	ErrorCodeSchemaTooNew ErrorCode = "SCHEMA_TOO_NEW"
	// ErrorCodeInternal 表示服务端未归类错误。
	ErrorCodeInternal ErrorCode = "INTERNAL"
)

// DefaultMessage 返回错误码对应的默认安全文案。
func (code ErrorCode) DefaultMessage() string {
	switch code {
	case ErrorCodeUnauthorized:
		return "未登录或会话已失效"
	case ErrorCodeForbidden:
		return "请求被拒绝"
	case ErrorCodeValidationFailed:
		return "请求参数无效"
	case ErrorCodePayloadTooLarge:
		return "请求体过大"
	case ErrorCodeNotFound:
		return "资源不存在"
	case ErrorCodeMethodNotAllowed:
		return "请求方法不支持"
	case ErrorCodeUnsupported:
		return "当前操作不支持"
	case ErrorCodeUnavailable:
		return "服务暂时不可用"
	case ErrorCodeConflict:
		return "资源状态冲突"
	case ErrorCodeOperationInProgress:
		return "操作正在进行中"
	case ErrorCodeStorageBusy:
		return "数据库暂时繁忙，请稍后重试"
	case ErrorCodeStorageCorrupted:
		return "数据库校验失败，请从备份恢复"
	case ErrorCodeSchemaTooNew:
		return "数据库版本高于当前程序支持版本"
	default:
		return "服务内部错误"
	}
}

// AppError 保存跨 service、dao 和 controller 传递的稳定业务错误。
type AppError struct {
	Code         ErrorCode
	Message      string
	UpstreamCode string
	Cause        error
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

// WrapAppErrorWithUpstreamError 使用上游错误详情包装稳定业务错误码。
//
// UpstreamCode 仅用于向调用方展示上游的稳定错误标识；Code 仍用于本地
// 状态持久化、日志分类和 HTTP status 映射。
func WrapAppErrorWithUpstreamError(code ErrorCode, upstreamCode string, message string, cause error) *AppError {
	return &AppError{
		Code:         code,
		Message:      message,
		UpstreamCode: upstreamCode,
		Cause:        cause,
	}
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
