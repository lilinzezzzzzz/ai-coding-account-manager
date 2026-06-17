package httptransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

// DecodeStrictJSON 严格解析 JSON 请求体，拒绝未知字段、空 body 和多个 JSON 值。
func DecodeStrictJSON(r *http.Request, dst any) error {
	if dst == nil {
		return entity.NewAppError(entity.ErrorCodeInternal)
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return decodeJSONError(err)
	}

	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "请求体只能包含一个 JSON 值")
	} else if !errors.Is(err, io.EOF) {
		return decodeJSONError(err)
	}

	return nil
}

func decodeJSONError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return entity.NewAppError(entity.ErrorCodePayloadTooLarge)
	}
	if errors.Is(err, io.EOF) {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "请求体不能为空")
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "JSON 格式无效")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "JSON 字段类型无效")
	}

	return entity.WrapAppErrorWithMessage(entity.ErrorCodeValidationFailed, "JSON 请求体无效", fmt.Errorf("decode json: %w", err))
}
