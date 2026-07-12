const successCode = "SUCCESS";

export async function api(path, options) {
  const init = {
    method: options.method,
    credentials: "same-origin",
    headers: {},
  };
  if (options.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }

  const response = await fetch(path, init);
  const envelope = await response.json();
  if (!response.ok || envelope.code !== successCode) {
    throw new Error(envelope.message || `HTTP ${response.status}`);
  }
  // 检查业务级别的错误码
  if (envelope.data && envelope.data.errorCode && !options.allowDataErrorCode) {
    const errorCode = envelope.data.errorCode;
    const errorMessage = envelope.data.errorMessage || getErrorMessage(errorCode);
    const error = new Error(errorMessage);
    error.code = errorCode;
    throw error;
  }
  return envelope.data;
}

export function getErrorMessage(errorCode) {
  const messages = {
    UNAUTHORIZED: "未登录或会话已失效",
    FORBIDDEN: "请求被拒绝",
    VALIDATION_FAILED: "请求参数无效",
    PAYLOAD_TOO_LARGE: "请求体过大",
    NOT_FOUND: "资源不存在",
    METHOD_NOT_ALLOWED: "请求方法不支持",
    UNSUPPORTED: "当前操作不支持",
    UNAVAILABLE: "服务暂时不可用",
    CONFLICT: "资源状态冲突",
    OPERATION_IN_PROGRESS: "操作正在进行中",
    STORAGE_BUSY: "数据库暂时繁忙，请稍后重试",
    STORAGE_CORRUPTED: "数据库校验失败，请从备份恢复",
    SCHEMA_TOO_NEW: "数据库版本高于当前程序支持版本",
    INTERNAL: "服务内部错误",
  };
  return messages[errorCode] || "未知错误";
}
