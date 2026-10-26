import { setupThemeMenu } from "./components/theme-menu.js?v=theme-menu-1";
import { api, getErrorMessage } from "./api.js?v=reauthentication-required";
import { closeMenus } from "./components/add-menu.js?v=account-action-menu";
import { emptyState } from "./components/common.js?v=components";
import { providerSection } from "./components/provider-section.js?v=account-removal";
import { copyTextFrom } from "./clipboard.js?v=copy-auth-json";
import { usageResetConfirmationDetail, usageResetCredits } from "./components/usage-limit.js?v=account-action-menu";
import {
  confirmDialog,
  promptAuthJSON,
  promptAuthJSONFile,
  promptPlanDialog,
  promptTextDialog,
} from "./dialogs.js?v=single-plan-date";
import {
  delay,
  formatDateInput,
} from "./formatters.js?v=single-plan-date";
import { hideTooltip, setupTooltips } from "./tooltip.js?v=tooltip-position";

const state = {
  providers: [],
  accounts: [],
  loginTasks: new Map(),
  refreshingAccountKeys: new Set(),
  resettingAccountKeys: new Set(),
  loading: false,
};

const messageAutoHideMs = 4200;
const errorMessageAutoHideMs = 7000;
const codexReloginErrorCodes = new Set([
  "REAUTHENTICATION_REQUIRED",
  "token_expired",
  "token_invalidated",
  "token_revoked",
  "refresh_token_expired",
  "refresh_token_reused",
  "refresh_token_invalidated",
]);
let messageHideTimer = 0;

const elements = {
  message: document.querySelector("#message"),
  providers: document.querySelector("#providers"),
};

boot();

async function boot() {
  setupThemeMenu();
  setupGlobalEvents();
  try {
    await loadData();
  } catch (error) {
    showMessage(error.message || "初始化失败", true);
  }
}

function setupGlobalEvents() {
  document.addEventListener("click", closeMenus);
  setupTooltips();
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeMenus();
      hideTooltip();
    }
  });
}

async function loadData() {
  setLoading(true);
  try {
    const [providers, accounts] = await Promise.all([
      api("/api/providers", { method: "GET" }),
      api("/api/accounts", { method: "GET" }),
    ]);
    state.providers = providers;
    state.accounts = accounts;
    render();
  } finally {
    setLoading(false);
  }
}

async function importProviderAuthJSON(providerId) {
  const authJson = await promptAuthJSON();
  if (authJson === null) {
    return;
  }
  await submitProviderAuthJSON(providerId, authJson);
}

async function importProviderAuthJSONFile(providerId) {
  const file = await promptAuthJSONFile();
  if (file === null) {
    return;
  }
  let authJson;
  try {
    authJson = await file.text();
  } catch {
    showMessage("JSON 文件读取失败", true);
    return;
  }
  await submitProviderAuthJSON(providerId, authJson);
}

async function submitProviderAuthJSON(providerId, authJson) {
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(providerId)}/accounts/auth-json/import`, {
      method: "POST",
      body: { authJson },
    });
    showMessage("账号已导入并刷新");
    await loadData();
  });
}

async function createLoginTask(providerId, expectedEmail = "") {
  setLoading(true);
  try {
    const body = { mode: "browser" };
    if (expectedEmail) {
      body.expectedEmail = expectedEmail;
    }
    const task = await api(`/api/providers/${encodeURIComponent(providerId)}/login-tasks/create`, {
      method: "POST",
      body,
    });
    state.loginTasks.set(task.taskId, task);
    showLoginTaskMessage(task);
    pollLoginTask(providerId, task.taskId);
  } catch (error) {
    showMessage(error.message || "创建登录任务失败", true);
  } finally {
    setLoading(false);
  }
}

async function pollLoginTask(providerId, taskId) {
  for (;;) {
    await delay(1500);
    let task;
    try {
      task = await api(`/api/providers/${encodeURIComponent(providerId)}/login-tasks/${encodeURIComponent(taskId)}`, {
        method: "GET",
        allowDataErrorCode: true,
      });
    } catch (error) {
      showMessage(error.message || "查询登录任务失败", true);
      return;
    }
    state.loginTasks.set(task.taskId, task);
    showLoginTaskMessage(task);
    if (task.status === "imported") {
      await loadData();
      const account = task.account
        ? state.accounts.find((item) => item.providerId === providerId && item.accountId === task.account.accountId)
        : null;
      if (account) {
        await refreshAccount(account, { promptRelogin: false });
      }
      return;
    }
    if (["failed", "cancelled", "expired"].includes(task.status)) {
      return;
    }
  }
}

async function refreshAccount(account, options = {}) {
  const promptRelogin = options.promptRelogin !== false;
  const key = accountKey(account);
  if (state.loading || state.refreshingAccountKeys.has(key)) {
    return;
  }

  state.refreshingAccountKeys.add(key);
  render();
  try {
    const result = await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/refresh`, {
      method: "POST",
      body: {},
    });
    if (!result.account) {
      throw new Error(result.errorMessage || "账号状态刷新失败");
    }
    replaceAccount(result.account);
    showMessage("账号状态已刷新");
  } catch (error) {
    if (promptRelogin && await promptAccountRelogin(account, error)) {
      return;
    }
    showMessage(error.message || "账号状态刷新失败", true);
  } finally {
    state.refreshingAccountKeys.delete(key);
    render();
  }
}

async function copyAccountAuthJSON(account) {
  const key = accountKey(account);
  if (state.loading || state.refreshingAccountKeys.has(key) || state.resettingAccountKeys.has(key)) {
    return;
  }
  await runAction(async () => {
    await copyTextFrom(async () => {
      const result = await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/auth-json/export`, {
        method: "POST",
        body: {},
      });
      if (typeof result?.authJson !== "string" || !result.authJson.trim()) {
        throw new Error("此账号没有可导出的登录凭据");
      }
      return result.authJson;
    });
    showMessage("已复制，请妥善保管登录凭据");
  });
}

async function resetAccountRateLimit(account) {
  const key = accountKey(account);
  if (state.loading || state.refreshingAccountKeys.has(key) || state.resettingAccountKeys.has(key)) {
    return;
  }
  const resetCredits = usageResetCredits(account.usage);
  const confirmed = await confirmDialog({
    title: account.label || account.email || account.accountId,
    detailContent: usageResetConfirmationDetail(resetCredits || { availableCount: 0, credits: [] }),
    confirmText: "确认重置",
  });
  if (!confirmed) {
    return;
  }

  state.resettingAccountKeys.add(key);
  render();
  try {
    const result = await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/rate-limit/reset`, {
      method: "POST",
      body: { idempotencyKey: crypto.randomUUID() },
    });
    replaceAccount(result.account);
    showMessage(resetOutcomeMessage(result.outcome));
  } catch (error) {
    if (await promptAccountRelogin(account, error)) {
      return;
    }
    showMessage(error.message || "额度重置失败", true);
  } finally {
    state.resettingAccountKeys.delete(key);
    render();
  }
}

async function promptAccountRelogin(account, error) {
  if (account.providerId !== "codex" || !codexReloginErrorCodes.has(error.code)) {
    return false;
  }
  hideMessage();
  const confirmed = await confirmDialog({
    title: "登录态已失效",
    detail: `${account.label || account.email || account.accountId}\n此账号的 OpenAI/Codex 登录凭据已失效。重新登录后将更新该账号的本地凭据。`,
    confirmText: "重新登录",
  });
  if (confirmed) {
    await createLoginTask(account.providerId, account.email || "");
  }
  return true;
}

function resetOutcomeMessage(outcome) {
  const messages = {
    reset: "额度已重置",
    nothingToReset: "当前没有可重置的额度窗口",
    noCredit: "没有可用的重置次数",
    alreadyRedeemed: "本次重置已完成",
  };
  return messages[outcome] || "额度重置状态已更新";
}

async function updatePlan(account) {
  if (state.loading) {
    return;
  }
  const currentDate = account.planDate;
  const input = await promptPlanDialog({
    detail: account.label || account.email || account.accountId,
    autoRenew: account.planAutoRenew,
    date: currentDate ? formatDateInput(currentDate) : "",
  });
  if (input === null) {
    return;
  }
  const planDate = input.date ? new Date(`${input.date}T00:00:00`).getTime() : null;
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/plan/update`, {
      method: "POST",
      body: { planAutoRenew: input.planAutoRenew, planDate },
    });
    await loadData();
    showMessage("套餐信息已更新");
  });
}

async function activateAccount(account) {
  const confirmed = await confirmDialog({
    title: "激活账号",
    detail: `账号：${account.label || account.email || account.accountId}\n确认激活后，请 reload VS Code 窗口使配置生效。`,
    confirmText: "确认激活",
  });
  if (!confirmed) {
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/activate`, {
      method: "POST",
      body: {},
    });
    showMessage("账号已切换");
    await loadData();
  });
}

async function deleteAccount(account) {
  const confirmed = await confirmDialog({
    title: account.isActive ? "删除当前活动账号" : "删除账号",
    detail: `账号：${account.label || account.email || account.accountId}\n确认删除后，将移除管理器中的账号记录及隔离凭据。${account.isActive ? "\n此账号正在使用中。删除不会退出 Codex 客户端，也不会删除当前 CODEX_HOME 的登录凭据。" : ""}`,
    confirmText: "确认删除",
    danger: true,
  });
  if (!confirmed) {
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}`, {
      method: "DELETE",
    });
    showMessage("账号已删除");
    await loadData();
  });
}

async function runAction(action) {
  setLoading(true);
  try {
    await action();
  } catch (error) {
    showMessage(error.message || "操作失败", true);
  } finally {
    setLoading(false);
  }
}

function render() {
  hideTooltip();
  elements.providers.replaceChildren();
  if (state.providers.length === 0) {
    elements.providers.append(emptyState("没有 provider", "后端没有注册可用账号 provider。"));
    return;
  }

  for (const providerInfo of state.providers) {
    const accounts = state.accounts.filter((account) => account.providerId === providerInfo.id);
    elements.providers.append(
      providerSection({
        providerInfo,
        accounts,
        loading: state.loading,
        accountState,
        actions: {
          createLoginTask,
          importProviderAuthJSON,
          importProviderAuthJSONFile,
          refreshAccount,
          activateAccount,
          deleteAccount,
          resetAccountRateLimit,
          copyAccountAuthJSON,
          updatePlan,
        },
      }),
    );
  }
}

function showMessage(text, isError = false, options = {}) {
  clearMessageTimer();
  elements.message.hidden = false;
  elements.message.textContent = text;
  elements.message.classList.toggle("error", isError);
  const timeoutMs = options.timeoutMs ?? (isError ? errorMessageAutoHideMs : messageAutoHideMs);
  if (timeoutMs > 0) {
    messageHideTimer = window.setTimeout(hideMessage, timeoutMs);
  }
}

function hideMessage() {
  clearMessageTimer();
  elements.message.hidden = true;
  elements.message.textContent = "";
  elements.message.classList.remove("error");
}

function clearMessageTimer() {
  if (messageHideTimer) {
    window.clearTimeout(messageHideTimer);
    messageHideTimer = 0;
  }
}

function showLoginTaskMessage(task) {
  if (task.status === "waiting_for_user") {
    if (task.userCode) {
      showMessage(`请打开登录页并输入 device code：${task.userCode}`);
      return;
    }
    showMessage("请在浏览器完成 Codex 登录。");
    return;
  }
  if (task.status === "verifying") {
    showMessage("正在校验 Codex 登录账号。");
    return;
  }
  if (task.status === "imported") {
    const label = task.account && (task.account.email || task.account.label || task.account.accountId);
    showMessage(`账号已导入：${label || task.taskId}`);
    return;
  }
  if (task.status === "failed" || task.status === "expired") {
    showMessage(task.errorMessage || getErrorMessage(task.errorCode), true);
    return;
  }
  if (task.status === "cancelled") {
    showMessage("登录任务已取消", true);
    return;
  }
  showMessage("Codex 登录任务已创建。");
}

function setLoading(loading) {
  state.loading = loading;
  document.querySelectorAll("button").forEach((btn) => {
    btn.disabled = loading || btn.dataset.disabledWhenIdle === "true";
  });
}

function replaceAccount(updatedAccount) {
  const index = state.accounts.findIndex((account) => accountKey(account) === accountKey(updatedAccount));
  if (index === -1) {
    return;
  }
  state.accounts = state.accounts.map((account, accountIndex) => (accountIndex === index ? updatedAccount : account));
}

function accountState(account) {
  return {
    isRefreshing: isAccountRefreshing(account),
    isResetting: state.resettingAccountKeys.has(accountKey(account)),
  };
}

function isAccountRefreshing(account) {
  return state.refreshingAccountKeys.has(accountKey(account));
}

function accountKey(account) {
  return `${account.providerId}:${account.accountId}`;
}
