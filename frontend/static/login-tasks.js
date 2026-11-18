import { api, getErrorMessage, providerApiPath } from "./api.js?v=frontend-cleanup-1";
import { delay } from "./formatters.js?v=single-plan-date";

const terminalStatuses = new Set(["imported", "failed", "cancelled", "expired"]);

export async function requestLoginTask(providerId, expectedEmail) {
  const body = { mode: "browser" };
  if (expectedEmail) {
    body.expectedEmail = expectedEmail;
  }
  return api(`${providerApiPath(providerId)}/login-tasks/create`, { method: "POST", body });
}

export async function pollLoginTask(providerId, initialTask, onUpdate) {
  let task = initialTask;
  const path = `${providerApiPath(providerId)}/login-tasks/${encodeURIComponent(task.taskId)}`;
  while (!terminalStatuses.has(task.status)) {
    await delay(1500);
    task = await api(path, { method: "GET", allowDataErrorCode: true });
    onUpdate(task);
  }
  return task;
}

export function loginTaskMessage(task) {
  switch (task.status) {
    case "waiting_for_user":
      return {
        text: task.userCode
          ? `请打开登录页并输入 device code：${task.userCode}`
          : "请在浏览器完成 Codex 登录。",
      };
    case "verifying":
      return { text: "正在校验 Codex 登录账号。" };
    case "imported": {
      const account = task.account;
      const label = account?.email || account?.label || account?.accountId || task.taskId;
      return { text: `账号已导入：${label}` };
    }
    case "failed":
    case "expired":
      return { text: task.errorMessage || getErrorMessage(task.errorCode), isError: true };
    case "cancelled":
      return { text: "登录任务已取消", isError: true };
    default:
      return { text: "Codex 登录任务已创建。" };
  }
}
