const state = {
  providers: [],
  accounts: [],
  loginTasks: new Map(),
  loading: false,
};

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
  const email = window.prompt("OpenAI 账号邮箱");
  if (email === null) {
    return;
  }
  const trimmed = email.trim();
  if (!isValidEmail(trimmed)) {
    showMessage("OpenAI 账号邮箱无效", true);
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(providerId)}/accounts/create`, {
      method: "POST",
      body: { email: trimmed },
    });
    showMessage("账号已新增");
    await loadData();
  });
}

async function createLoginTask(providerId) {
  const expectedEmail = window.prompt("要登录的 OpenAI 账号邮箱（可留空）", "");
  if (expectedEmail === null) {
    return;
  }
  const email = expectedEmail.trim();
  if (email && !isValidEmail(email)) {
    showMessage("OpenAI 账号邮箱无效", true);
    return;
  }
  const modeInput = window.prompt("登录方式：browser 或 device_code", "browser");
  if (modeInput === null) {
    return;
  }
  const mode = modeInput.trim() || "browser";
  if (mode !== "browser" && mode !== "device_code") {
    showMessage("登录方式只支持 browser 或 device_code", true);
    return;
  }

  setLoading(true);
  try {
    const task = await api(`/api/providers/${encodeURIComponent(providerId)}/login-tasks/create`, {
      method: "POST",
      body: { expectedEmail: email, mode },
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

async function refreshAccountUsage(account) {
  await runAction(async () => {
    const result = await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/usage/refresh`, {
      method: "POST",
      body: {},
    });
    // 直接更新对应账号的 usage 信息，避免重新加载所有数据
    const index = state.accounts.findIndex(
      (a) => a.providerId === account.providerId && a.accountId === account.accountId
    );
    if (index !== -1 && result.usage) {
      state.accounts[index].usage = result.usage;
      render();
    }
    showMessage("额度已刷新");
  });
}

async function activateAccount(account) {
  if (!window.confirm(`切换到 ${account.label}？切换后需要 reload VS Code 窗口。`)) {
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

async function renameAccount(account) {
  const label = window.prompt("账号名称", account.label);
  if (label === null) {
    return;
  }
  const trimmed = label.trim();
  if (!trimmed) {
    showMessage("账号名称不能为空", true);
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(account.providerId)}/accounts/${encodeURIComponent(account.accountId)}/rename`, {
      method: "POST",
      body: { label: trimmed },
    });
    showMessage("账号名称已更新");
    await loadData();
  });
}

async function deleteAccount(account) {
  if (!window.confirm(`删除 ${account.label}？该操作会删除隔离凭据目录。`)) {
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
  if (!response.ok || envelope.error) {
    const message = envelope.error ? envelope.error.message : `HTTP ${response.status}`;
    throw new Error(message);
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
  addButton.textContent = "登录添加 Codex 账号";
  addButton.disabled = state.loading || providerInfo.status !== "available";
  addButton.addEventListener("click", () => createLoginTask(providerInfo.id));
  actions.append(addButton);
  const manualButton = document.createElement("button");
  manualButton.type = "button";
  manualButton.textContent = "手动录入邮箱";
  manualButton.disabled = state.loading || providerInfo.status !== "available";
  manualButton.addEventListener("click", () => createAccount(providerInfo.id));
  actions.append(manualButton);
  return actions;
}

function accountCard(account, providerInfo) {
  const card = document.createElement("article");
  card.className = `account-card ${account.isActive ? "active" : ""}`;

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
  meta.append(pill(account.email || "email 未知"));
  if (account.planType) {
    meta.append(pill(account.planType));
  }
  meta.append(pill(shortId(account.accountId)));
  main.append(meta);
  card.append(main);

  card.append(usageBlock(account.usage));

  const actions = document.createElement("div");
  actions.className = "account-actions";
  actions.append(actionButton("刷新额度", () => refreshAccountUsage(account)));
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(actionButton("激活", () => activateAccount(account)));
  }
  actions.append(actionButton("重命名", () => renameAccount(account)));
  const deleteButton = actionButton("删除", () => deleteAccount(account));
  deleteButton.className = "danger";
  deleteButton.dataset.isActive = account.isActive;
  deleteButton.disabled = state.loading || account.isActive;
  actions.append(deleteButton);
  card.append(actions);

  return card;
}

function usageBlock(usage) {
  const wrapper = document.createElement("div");
  wrapper.className = "usage";
  const percent = usage && typeof usage.usedPercent === "number" ? usage.usedPercent : null;
  const progress = document.createElement("div");
  progress.className = "progress";
  const bar = document.createElement("div");
  bar.className = "progress-bar";
  bar.style.width = `${Math.max(0, Math.min(100, percent || 0))}%`;
  progress.append(bar);
  wrapper.append(progress);
  wrapper.append(usageRow("状态", usage ? usage.status : "未刷新"));
  wrapper.append(usageRow("使用量", percent === null ? "未知" : `${percent.toFixed(1)}%`));
  wrapper.append(usageRow("重置时间", usage && usage.resetsAt ? formatTime(usage.resetsAt) : "未知"));
  return wrapper;
}

function usageRow(label, value) {
  const row = document.createElement("div");
  row.className = "usage-row";
  const left = document.createElement("span");
  left.textContent = label;
  const right = document.createElement("span");
  right.className = "usage-value";
  right.textContent = value;
  row.append(left, right);
  return row;
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

function pill(text) {
  const item = document.createElement("span");
  item.className = "pill";
  item.textContent = text;
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
      btn.disabled = loading;
    }
  });
}

function isValidEmail(value) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

function shortId(value) {
  if (!value || value.length <= 18) {
    return value || "id 未知";
  }
  return `${value.slice(0, 10)}...${value.slice(-6)}`;
}

function formatTime(value) {
  const millis = value < 100000000000 ? value * 1000 : value;
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(millis));
}

function delay(ms) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
