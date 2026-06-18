const state = {
  providers: [],
  accounts: [],
  loginTasks: new Map(),
  refreshingAccountKeys: new Set(),
  loading: false,
};

const successCode = "SUCCESS";

const elements = {
  message: document.querySelector("#message"),
  providers: document.querySelector("#providers"),
};

boot();

async function boot() {
  try {
    await loadData();
  } catch (error) {
    showMessage(error.message || "初始化失败", true);
  }
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

async function createAccount(providerId) {
  const email = await promptTextDialog({
    title: "手动录入",
    fieldName: "email",
    inputType: "email",
    autocomplete: "email",
    placeholder: "name@example.com",
    submitText: "新增",
    validate: (value) => (isValidEmail(value) ? "" : "OpenAI 账号邮箱无效"),
  });
  if (email === null) {
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(providerId)}/accounts/create`, {
      method: "POST",
      body: { email },
    });
    showMessage("账号已新增");
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

async function importAccountAuthJSON(account) {
  const authJson = await promptAuthJSON(account);
  if (authJson === null) {
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/auth-json/import`, {
      method: "POST",
      body: { authJson },
    });
    showMessage("auth.json 已导入");
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

async function api(path, options) {
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
    throw new Error(errorMessage);
  }
  return envelope.data;
}

function getErrorMessage(errorCode) {
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
      section.append(emptyState("还没有账号", "点击上方“登录添加 Codex 账号”导入隔离凭据。"));
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
  const addButton = document.createElement("button");
  addButton.type = "button";
  addButton.textContent = "登录添加";
  addButton.disabled = state.loading || providerInfo.status !== "available";
  addButton.addEventListener("click", () => createLoginTask(providerInfo.id));
  actions.append(addButton);
  const manualButton = document.createElement("button");
  manualButton.type = "button";
  manualButton.textContent = "手动录入";
  manualButton.disabled = state.loading || providerInfo.status !== "available";
  manualButton.addEventListener("click", () => createAccount(providerInfo.id));
  actions.append(manualButton);
  return actions;
}

function accountCard(account, providerInfo) {
  const card = document.createElement("article");
  card.className = `account-card ${account.isActive ? "active" : ""}`;
  const isRefreshing = isAccountRefreshing(account);
  if (isRefreshing) {
    card.setAttribute("aria-busy", "true");
  }

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
  card.append(main);

  card.append(usageBlock(account.usage));

  const actions = document.createElement("div");
  actions.className = "account-actions";
  actions.append(accountActionButton(isRefreshing ? "刷新中" : "刷新", () => refreshAccount(account), isRefreshing));
  actions.append(accountActionButton("导入", () => importAccountAuthJSON(account), isRefreshing));
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(accountActionButton("激活", () => activateAccount(account), isRefreshing));
  }
  const deleteButton = accountActionButton("删除", () => deleteAccount(account), isRefreshing);
  deleteButton.className = "danger";
  deleteButton.dataset.isActive = account.isActive;
  deleteButton.disabled = state.loading || isRefreshing || account.isActive;
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

function promptAuthJSON(account) {
  const textarea = document.createElement("textarea");
  textarea.name = "authJson";
  textarea.rows = 14;
  textarea.autocomplete = "off";
  textarea.spellcheck = false;
  textarea.placeholder = '{\n  "tokens": {}\n}';
  return new Promise((resolve) => {
    openFormDialog({
      title: "导入 auth.json",
      detail: account.label || account.email || account.accountId,
      body: textarea,
      submitText: "导入",
      initialFocus: textarea,
      validate: () => (textarea.value.trim() ? "" : "auth.json 内容不能为空"),
      onSubmit: () => resolve(textarea.value.trim()),
      onCancel: () => resolve(null),
    });
  });
}

function promptTextDialog(options) {
  const input = document.createElement("input");
  input.name = options.fieldName;
  input.type = options.inputType || "text";
  input.autocomplete = options.autocomplete || "off";
  input.placeholder = options.placeholder || "";
  input.value = options.initialValue || "";
  return new Promise((resolve) => {
    openFormDialog({
      title: options.title,
      detail: options.detail || "",
      body: input,
      submitText: options.submitText || "确定",
      initialFocus: input,
      validate: () => {
        const value = input.value.trim();
        if (options.validate) {
          return options.validate(value);
        }
        return "";
      },
      onSubmit: () => resolve(input.value.trim()),
      onCancel: () => resolve(null),
    });
  });
}

function confirmDialog(options) {
  const detail = document.createElement("p");
  detail.className = "dialog-detail";
  detail.textContent = options.detail || "";
  return new Promise((resolve) => {
    openFormDialog({
      title: options.title,
      body: detail,
      submitText: options.confirmText || "确定",
      submitDanger: options.danger || false,
      onSubmit: () => resolve(true),
      onCancel: () => resolve(false),
    });
  });
}

function openFormDialog(options) {
  const dialog = document.createElement("dialog");
  dialog.className = "app-dialog";
  const form = document.createElement("form");
  form.method = "dialog";

  const title = document.createElement("h2");
  title.textContent = options.title;
  form.append(title);

  if (options.detail) {
    const detail = document.createElement("p");
    detail.className = "dialog-account";
    detail.textContent = options.detail;
    form.append(detail);
  }

  form.append(options.body);

  const error = document.createElement("p");
  error.className = "dialog-error";
  error.hidden = true;
  form.append(error);

  const actions = document.createElement("div");
  actions.className = "dialog-actions";
  const cancelButton = document.createElement("button");
  cancelButton.type = "button";
  cancelButton.textContent = "取消";
  const submitButton = document.createElement("button");
  submitButton.type = "submit";
  submitButton.textContent = options.submitText || "确定";
  if (options.submitDanger) {
    submitButton.classList.add("danger");
  }
  actions.append(cancelButton, submitButton);
  form.append(actions);

  dialog.append(form);
  document.body.append(dialog);

  let settled = false;
  const close = (cancelled) => {
    if (settled) {
      return;
    }
    settled = true;
    dialog.close();
    dialog.remove();
    if (cancelled && options.onCancel) {
      options.onCancel();
    }
  };
  cancelButton.addEventListener("click", () => close(true));
  dialog.addEventListener("cancel", (event) => {
    event.preventDefault();
    close(true);
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const message = options.validate ? options.validate() : "";
    if (message) {
      error.textContent = message;
      error.hidden = false;
      if (options.initialFocus) {
        options.initialFocus.focus();
      }
      return;
    }
    if (settled) {
      return;
    }
    settled = true;
    dialog.close();
    dialog.remove();
    if (options.onSubmit) {
      options.onSubmit();
    }
  });
  dialog.showModal();
  const initialFocus = options.initialFocus || dialog.querySelector("button[type='submit']");
  if (initialFocus) {
    initialFocus.focus();
  }
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

function showMessage(text, isError = false) {
  elements.message.hidden = false;
  elements.message.textContent = text;
  elements.message.classList.toggle("error", isError);
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

function isValidEmail(value) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
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
