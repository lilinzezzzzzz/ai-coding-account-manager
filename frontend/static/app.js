import { api, getErrorMessage } from "./api.js?v=split-modules";
import { confirmDialog, promptAuthJSON, promptTextDialog } from "./dialogs.js?v=split-modules";

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
let messageHideTimer = 0;

const elements = {
  message: document.querySelector("#message"),
  providers: document.querySelector("#providers"),
};

boot();

async function boot() {
  setupGlobalEvents();
  try {
    await loadData();
  } catch (error) {
    showMessage(error.message || "初始化失败", true);
  }
}

function setupGlobalEvents() {
  document.addEventListener("click", closeAddMenus);
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeAddMenus();
    }
  });
}

function closeAddMenus() {
  document.querySelectorAll(".add-menu-trigger[aria-expanded='true']").forEach((trigger) => {
    trigger.setAttribute("aria-expanded", "false");
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
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(providerId)}/accounts/auth-json/import`, {
      method: "POST",
      body: { authJson },
    });
    showMessage("账号已导入并刷新");
    await loadData();
  });
}

async function createLoginTask(providerId) {
  setLoading(true);
  try {
    const task = await api(`/api/providers/${encodeURIComponent(providerId)}/login-tasks/create`, {
      method: "POST",
      body: { mode: "browser" },
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
      return;
    }
    if (["failed", "cancelled", "expired"].includes(task.status)) {
      return;
    }
  }
}

async function refreshAccount(account) {
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
    showMessage(error.message || "账号状态刷新失败", true);
  } finally {
    state.refreshingAccountKeys.delete(key);
    render();
  }
}

async function resetAccountRateLimit(account) {
  const key = accountKey(account);
  if (state.loading || state.refreshingAccountKeys.has(key) || state.resettingAccountKeys.has(key)) {
    return;
  }
  const resetCredits = usageResetCredits(account.usage);
  const confirmed = await confirmDialog({
    title: account.label || account.email || account.accountId,
    detailContent: resetConfirmationDetail(resetCredits ? resetCredits.availableCount : 0),
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
    showMessage(error.message || "额度重置失败", true);
  } finally {
    state.resettingAccountKeys.delete(key);
    render();
  }
}

function resetConfirmationDetail(availableCount) {
  const content = document.createDocumentFragment();
  content.append("当前可重置次数：");
  const count = document.createElement("strong");
  count.className = "dialog-reset-count";
  count.textContent = `${availableCount}`;
  content.append(count, "\n点击确认重置，将消耗 1 次重置机会，并重置当前符合条件的额度窗口。");
  return content;
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

async function updatePlanExpiration(account) {
  if (state.loading) {
    return;
  }
  const currentValue = account.planExpiresAt ? formatDateInput(account.planExpiresAt) : "";
  const input = await promptTextDialog({
    title: "套餐到期日",
    detail: account.label || account.email || account.accountId,
    fieldName: "planExpiresAt",
    inputType: "text",
    initialValue: currentValue,
    placeholder: "YYYY-MM-DD",
    submitText: "保存",
    validate: (value) => (!value || isValidDateInput(value) ? "" : "套餐到期日格式无效"),
  });
  if (input === null) {
    return;
  }
  const value = input;
  let planExpiresAt = null;
  if (value) {
    planExpiresAt = new Date(`${value}T00:00:00`).getTime();
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/plan-expiration/update`, {
      method: "POST",
      body: { planExpiresAt },
    });
    await loadData();
    showMessage(planExpiresAt === null ? "套餐到期日已清除" : "套餐到期日已更新");
  });
}

async function activateAccount(account) {
  const confirmed = await confirmDialog({
    title: "激活账号",
    detail: `${account.label || account.email || account.accountId}\n切换后需要 reload VS Code 窗口。`,
    confirmText: "激活",
  });
  if (!confirmed) {
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/activate`, {
      method: "POST",
      body: {},
    });
    showMessage("账号已切换。请在 VS Code 执行 Developer: Reload Window。");
    await loadData();
  });
}

async function deleteAccount(account) {
  const confirmed = await confirmDialog({
    title: "删除账号",
    detail: `${account.label || account.email || account.accountId}\n该操作会删除隔离凭据目录。`,
    confirmText: "删除",
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
  elements.providers.replaceChildren();
  if (state.providers.length === 0) {
    elements.providers.append(emptyState("没有 provider", "后端没有注册可用账号 provider。"));
    return;
  }

  for (const providerInfo of state.providers) {
    const section = document.createElement("section");
    section.className = "provider-section";

    const header = document.createElement("div");
    header.className = "provider-header";
    header.append(providerTitle(providerInfo));
    header.append(providerActions(providerInfo));
    section.append(header);

    const accounts = state.accounts.filter((account) => account.providerId === providerInfo.id);
    if (accounts.length === 0) {
      section.append(emptyState("还没有账号", "点击上方“添加”导入 auth.json，后端会自动识别并刷新账号。"));
    } else {
      const grid = document.createElement("div");
      grid.className = "account-grid";
      for (const account of accounts) {
        grid.append(accountCard(account, providerInfo));
      }
      section.append(grid);
    }
    elements.providers.append(section);
  }
}

function providerTitle(providerInfo) {
  const wrapper = document.createElement("div");
  wrapper.className = "provider-title";
  const title = document.createElement("h2");
  title.textContent = providerInfo.displayName || providerInfo.id;
  wrapper.append(title);
  return wrapper;
}

function providerActions(providerInfo) {
  const actions = document.createElement("div");
  actions.className = "toolbar";
  const menu = document.createElement("div");
  menu.className = "add-menu";
  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "primary add-menu-trigger";
  trigger.textContent = "添加";
  trigger.setAttribute("aria-haspopup", "menu");
  trigger.setAttribute("aria-expanded", "false");
  trigger.disabled = state.loading || providerInfo.status !== "available";
  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    const expanded = trigger.getAttribute("aria-expanded") === "true";
    closeAddMenus();
    trigger.setAttribute("aria-expanded", expanded ? "false" : "true");
  });
  menu.append(trigger);

  const options = document.createElement("div");
  options.className = "add-menu-options";
  options.setAttribute("role", "menu");
  options.append(addMenuItem("登录添加", () => createLoginTask(providerInfo.id), providerInfo));
  options.append(addMenuItem("导入添加", () => importProviderAuthJSON(providerInfo.id), providerInfo));
  menu.append(options);
  actions.append(menu);
  return actions;
}

function addMenuItem(label, handler, providerInfo) {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  button.setAttribute("role", "menuitem");
  button.disabled = state.loading || providerInfo.status !== "available";
  button.addEventListener("click", () => {
    closeAddMenus();
    handler();
  });
  return button;
}

function accountCard(account, providerInfo) {
  const card = document.createElement("article");
  card.className = `account-card ${account.isActive ? "active" : ""}`;
  const isRefreshing = isAccountRefreshing(account);
  const isResetting = state.resettingAccountKeys.has(accountKey(account));
  const isBusy = isRefreshing || isResetting;
  if (isRefreshing || isResetting) {
    card.setAttribute("aria-busy", "true");
  }

  const header = document.createElement("div");
  header.className = "account-card-header";

  const main = document.createElement("div");
  main.className = "account-main";
  const name = document.createElement("div");
  name.className = "account-name";
  if (account.isActive) {
    const activeDot = document.createElement("span");
    activeDot.className = "active-dot";
    name.append(activeDot);
  }
  const title = document.createElement("h3");
  title.textContent = account.label || account.email || account.accountId;
  name.append(title);
  main.append(name);

  const meta = document.createElement("div");
  meta.className = "meta";
  if (account.planType) {
    meta.append(pill(account.planType));
  }
  meta.append(planExpirationPill(account));
  meta.append(pill(shortId(account.accountId), account.accountId));
  main.append(meta);
  header.append(main);
  const resetCredits = usageResetCredits(account.usage);
  if (resetCredits) {
    header.append(usageResetButton(account, resetCredits, isRefreshing, isResetting));
  }
  card.append(header);

  card.append(usageBlock(account.usage));

  const actions = document.createElement("div");
  actions.className = "account-actions";
  actions.append(accountActionButton(isRefreshing ? "刷新中" : "刷新", () => refreshAccount(account), isBusy));
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(accountActionButton("激活", () => activateAccount(account), isBusy));
  }
  const deleteButton = accountActionButton("删除", () => deleteAccount(account), isBusy);
  deleteButton.className = "danger";
  deleteButton.dataset.isActive = account.isActive;
  deleteButton.disabled = state.loading || isBusy || account.isActive;
  actions.append(deleteButton);
  card.append(actions);

  return card;
}

function usageBlock(usage) {
  const wrapper = document.createElement("div");
  wrapper.className = "usage";
  const limits = usageLimitItems(usage);
  if (limits.length === 0) {
    const empty = document.createElement("div");
    empty.className = "usage-empty";
    empty.textContent = usage ? "额度数据不可用" : "额度未刷新";
    wrapper.append(empty);
    return wrapper;
  }
  for (const item of limits) {
    wrapper.append(usageLimitBlock(item));
  }
  return wrapper;
}

function usageLimitItems(usage) {
  if (!usage) {
    return [];
  }
  const snapshot = parseSnapshot(usage.snapshotJson);
  const rateLimits = snapshot && snapshot.rateLimits ? snapshot.rateLimits : null;
  if (rateLimits) {
    return [
      limitItem("5 小时限额", rateLimits.primary),
      limitItem("7 天限额", rateLimits.secondary),
    ].filter(Boolean);
  }
  if (typeof usage.usedPercent === "number") {
    return [limitItem("5 小时限额", { usedPercent: usage.usedPercent, resetsAt: usage.resetsAt })].filter(Boolean);
  }
  return [];
}

function limitItem(label, limit) {
  if (!limit || typeof limit.usedPercent !== "number") {
    return null;
  }
  const usedPercent = clampPercent(limit.usedPercent);
  return {
    label,
    usedPercent,
    remainingPercent: clampPercent(100 - usedPercent),
    resetsAt: limit.resetsAt || null,
  };
}

function usageResetCredits(usage) {
  if (!usage) {
    return null;
  }
  const credits = usage.rateLimitResetCredits;
  if (!credits || typeof credits.availableCount !== "number") {
    return null;
  }
  return {
    availableCount: Math.max(0, Math.trunc(credits.availableCount)),
  };
}

function usageResetButton(account, resetCredits, isRefreshing, isResetting) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "usage-reset-button";
  const icon = document.createElement("span");
  icon.className = "usage-reset-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = "↻";
  button.append(icon);
  button.setAttribute("aria-label", isResetting ? "正在重置额度" : `可重置次数 ${resetCredits.availableCount}，点击重置`);
  button.title = isResetting ? "正在重置额度" : `使用 1 次重置，剩余 ${resetCredits.availableCount} 次`;
  button.classList.toggle("is-resetting", isResetting);
  button.setAttribute("aria-busy", `${isResetting}`);
  button.disabled = state.loading || isRefreshing || isResetting || resetCredits.availableCount <= 0;
  button.addEventListener("click", () => resetAccountRateLimit(account));
  return button;
}

function usageLimitBlock(item) {
  const section = document.createElement("section");
  section.className = "usage-limit";
  const title = document.createElement("div");
  title.className = "usage-limit-title";
  title.textContent = `${item.label}:`;
  section.append(title);

  const progress = document.createElement("div");
  progress.className = "usage-progress";
  progress.setAttribute("role", "meter");
  progress.setAttribute("aria-label", `${item.label}剩余额度`);
  progress.setAttribute("aria-valuemin", "0");
  progress.setAttribute("aria-valuemax", "100");
  progress.setAttribute("aria-valuenow", `${Math.round(item.remainingPercent)}`);
  const bar = document.createElement("div");
  bar.className = "usage-progress-bar";
  bar.style.width = `${item.remainingPercent}%`;
  progress.append(bar);
  section.append(progress);

  const detail = document.createElement("div");
  detail.className = "usage-limit-detail";
  const remaining = document.createElement("span");
  remaining.textContent = `剩余 ${formatPercent(item.remainingPercent)}`;
  const reset = document.createElement("span");
  reset.className = "usage-reset";
  reset.textContent = `(重置时间：${item.resetsAt ? formatDateTime(item.resetsAt) : "未知"})`;
  detail.append(remaining, reset);
  section.append(detail);
  return section;
}

function parseSnapshot(snapshotJson) {
  if (!snapshotJson) {
    return null;
  }
  try {
    return JSON.parse(snapshotJson);
  } catch (_) {
    return null;
  }
}

function clampPercent(value) {
  return Math.max(0, Math.min(100, value));
}

function formatPercent(value) {
  const rounded = Math.round(value * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}%` : `${rounded.toFixed(1)}%`;
}

function emptyState(title, detail) {
  const section = document.createElement("section");
  section.className = "empty-state";
  const heading = document.createElement("h2");
  heading.textContent = title;
  const text = document.createElement("p");
  text.textContent = detail;
  section.append(heading, text);
  return section;
}

function actionButton(label, handler) {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  button.disabled = state.loading;
  button.addEventListener("click", handler);
  return button;
}

function accountActionButton(label, handler, accountRefreshing) {
  const button = actionButton(label, handler);
  button.dataset.accountRefreshing = accountRefreshing ? "true" : "false";
  button.disabled = state.loading || accountRefreshing;
  return button;
}

function pill(text, title) {
  const item = document.createElement("span");
  item.className = "pill";
  item.textContent = text;
  if (title) {
    item.title = title;
  }
  return item;
}

function planExpirationPill(account) {
  const text = account.planExpiresAt ? formatPlanDate(account.planExpiresAt) : "YYYY-MM-DD";
  const item = pill(text, "点击录入套餐到期日，留空可清除");
  item.classList.add("interactive", "plan-expiration");
  item.tabIndex = 0;
  item.setAttribute("role", "button");
  item.addEventListener("click", () => updatePlanExpiration(account));
  item.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      updatePlanExpiration(account);
    }
  });
  return item;
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
    if (btn.classList.contains("danger") && btn.dataset.isActive === "true") {
      btn.disabled = true;
    } else {
      btn.disabled = loading || btn.dataset.accountRefreshing === "true";
    }
  });
}

function replaceAccount(updatedAccount) {
  const index = state.accounts.findIndex((account) => accountKey(account) === accountKey(updatedAccount));
  if (index === -1) {
    return;
  }
  state.accounts = state.accounts.map((account, accountIndex) => (accountIndex === index ? updatedAccount : account));
}

function isAccountRefreshing(account) {
  return state.refreshingAccountKeys.has(accountKey(account));
}

function accountKey(account) {
  return `${account.providerId}:${account.accountId}`;
}

function isValidDateInput(value) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  const date = new Date(`${value}T00:00:00`);
  return !Number.isNaN(date.getTime()) && formatDateInput(date.getTime()) === value;
}

function shortId(value) {
  if (!value || value.length <= 18) {
    return value || "id 未知";
  }
  return `${value.slice(0, 10)}...${value.slice(-6)}`;
}

function normalizeEpochMillis(value) {
  return value > 0 && value < 100000000000 ? value * 1000 : value;
}

function formatDateTime(value) {
  if (!value) {
    return "未知";
  }
  const millis = normalizeEpochMillis(value);
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZoneName: "short",
  }).format(new Date(millis));
}

function formatPlanDate(value) {
  const millis = normalizeEpochMillis(value);
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "numeric",
    day: "numeric",
  }).format(new Date(millis));
}

function formatDateInput(value) {
  const millis = normalizeEpochMillis(value);
  const date = new Date(millis);
  const year = date.getFullYear();
  const month = `${date.getMonth() + 1}`.padStart(2, "0");
  const day = `${date.getDate()}`.padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function delay(ms) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
