import { accountApiPath, api, providerApiPath } from "./api.js?v=frontend-cleanup-1";
import { closeMenus } from "./components/popup-menu.js?v=frontend-cleanup-1";
import { createStatusMessage } from "./components/status-message.js?v=frontend-cleanup-1";
import { setupThemeMenu } from "./components/theme-menu.js?v=frontend-cleanup-1";
import { emptyState } from "./components/common.js?v=tabler-1";
import { providerSection } from "./components/provider-section.js?v=frontend-cleanup-1";
import { copyTextFrom } from "./clipboard.js?v=copy-auth-json";
import { usageResetConfirmationDetail, usageResetCredits } from "./components/usage-limit.js?v=tabler-1";
import {
  confirmDialog,
  promptAuthJSON,
  promptAuthJSONFile,
  promptPlanDialog,
  promptTextDialog,
} from "./dialogs.js?v=tabler-1";
import { formatDateInput } from "./formatters.js?v=single-plan-date";
import { loginTaskMessage, pollLoginTask, requestLoginTask } from "./login-tasks.js?v=frontend-cleanup-1";
import { hideTooltip, setupTooltips } from "./tooltip.js?v=tooltip-position";

const state = {
  providers: [],
  accounts: [],
  refreshingAccountKeys: new Set(),
  resettingAccountKeys: new Set(),
  loading: false,
};

const codexReloginErrorCodes = new Set([
  "REAUTHENTICATION_REQUIRED",
  "token_expired",
  "token_invalidated",
  "token_revoked",
  "refresh_token_expired",
  "refresh_token_reused",
  "refresh_token_invalidated",
]);
const pendingActions = new Set();
let accountListRequest = 0;
let appliedAccountListRequest = 0;

const elements = {
  themeMenu: document.querySelector("#theme-menu"),
  providers: document.querySelector("#providers"),
};
const { show: showMessage, hide: hideMessage } = createStatusMessage(document.querySelector("#message"));

boot();

async function boot() {
  setupThemeMenu(elements.themeMenu);
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
  const request = ++accountListRequest;
  await withLoading(async () => {
    const [providers, accounts] = await Promise.all([
      api("/api/providers", { method: "GET" }),
      api("/api/accounts", { method: "GET" }),
    ]);
    // A background login can reload the list alongside an account action.
    if (request < appliedAccountListRequest) {
      return;
    }
    appliedAccountListRequest = request;
    state.providers = providers;
    state.accounts = accounts;
    render();
  });
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
    await api(`${providerApiPath(providerId)}/accounts/auth-json/import`, {
      method: "POST",
      body: { authJson },
    });
    showMessage("账号已导入并刷新");
    await loadData();
  });
}

async function createLoginTask(providerId, expectedEmail = "") {
  try {
    const task = await withLoading(() => requestLoginTask(providerId, expectedEmail));
    showLoginTaskMessage(task);
    void finishLoginTask(providerId, task);
  } catch (error) {
    showMessage(error.message || "创建登录任务失败", true);
  }
}

async function finishLoginTask(providerId, initialTask) {
  let task;
  try {
    task = await pollLoginTask(providerId, initialTask, showLoginTaskMessage);
  } catch (error) {
    showMessage(error.message || "查询登录任务失败", true);
    return;
  }
  if (task.status !== "imported") {
    return;
  }
  try {
    await loadData();
    while (pendingActions.size > 0) {
      await Promise.all(pendingActions);
    }
    const account = state.accounts.find((item) => (
      item.providerId === providerId && item.accountId === task.account?.accountId
    ));
    if (account) {
      await refreshAccount(account, { promptRelogin: false });
    }
  } catch (error) {
    showMessage(error.message || "登录完成后刷新账号列表失败", true);
  }
}

function showLoginTaskMessage(task) {
  const { text, isError } = loginTaskMessage(task);
  showMessage(text, isError);
}

async function refreshAccount(account, options = {}) {
  const promptRelogin = options.promptRelogin !== false;
  const key = accountKey(account);
  if (isAccountBusy(key)) {
    return;
  }

  state.refreshingAccountKeys.add(key);
  render();
  try {
    const result = await api(`${accountApiPath(account)}/refresh`, {
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
  if (isAccountBusy(key)) {
    return;
  }
  await runAction(async () => {
    await copyTextFrom(async () => {
      const result = await api(`${accountApiPath(account)}/auth-json/export`, {
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
  if (isAccountBusy(key)) {
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
    const result = await api(`${accountApiPath(account)}/rate-limit/reset`, {
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
    await api(`${accountApiPath(account)}/plan/update`, {
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
    await api(`${accountApiPath(account)}/activate`, {
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
    await api(accountApiPath(account), {
      method: "DELETE",
    });
    showMessage("账号已删除");
    await loadData();
  });
}

async function runAction(action) {
  try {
    await withLoading(action);
  } catch (error) {
    showMessage(error.message || "操作失败", true);
  }
}

async function withLoading(action) {
  let markFinished;
  const finished = new Promise((resolve) => {
    markFinished = resolve;
  });
  pendingActions.add(finished);
  setLoading(true);
  try {
    return await action();
  } finally {
    pendingActions.delete(finished);
    setLoading(pendingActions.size > 0);
    markFinished();
  }
}

function setLoading(loading) {
  if (loading === state.loading) {
    return;
  }
  state.loading = loading;
  for (const container of [elements.providers, elements.themeMenu]) {
    container.querySelectorAll("button").forEach((button) => {
      button.disabled = loading || button.dataset.disabledWhenIdle === "true";
    });
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

function replaceAccount(updatedAccount) {
  const index = state.accounts.findIndex((account) => accountKey(account) === accountKey(updatedAccount));
  if (index === -1) {
    return;
  }
  state.accounts[index] = updatedAccount;
}

function accountState(account) {
  const key = accountKey(account);
  return {
    isRefreshing: state.refreshingAccountKeys.has(key),
    isResetting: state.resettingAccountKeys.has(key),
  };
}

function isAccountBusy(key) {
  return state.loading || state.refreshingAccountKeys.has(key) || state.resettingAccountKeys.has(key);
}

function accountKey(account) {
  return `${account.providerId}:${account.accountId}`;
}
