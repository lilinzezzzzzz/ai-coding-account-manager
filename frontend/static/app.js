const state = {
  csrfToken: "",
  providers: [],
  accounts: [],
  loading: false,
  loginTimers: new Map(),
};

const elements = {
  message: document.querySelector("#message"),
  locked: document.querySelector("#locked"),
  providers: document.querySelector("#providers"),
  refreshButton: document.querySelector("#refresh-button"),
  importButton: document.querySelector("#import-button"),
};

elements.refreshButton.addEventListener("click", () => refreshUsage());
elements.importButton.addEventListener("click", () => importCurrentAccount());

boot();

async function boot() {
  try {
    const bootstrapToken = new URLSearchParams(window.location.search).get("bootstrap");
    if (bootstrapToken) {
      await exchangeBootstrap(bootstrapToken);
      window.history.replaceState({}, document.title, window.location.pathname);
    } else {
      await loadSession();
    }
    elements.locked.hidden = true;
    await loadData();
  } catch (error) {
    elements.locked.hidden = false;
    showMessage(error.message || "初始化失败", true);
  }
}

async function exchangeBootstrap(token) {
  const response = await api("/api/session/bootstrap", {
    method: "POST",
    body: { bootstrapToken: token },
    csrf: false,
  });
  state.csrfToken = response.csrfToken;
}

async function loadSession() {
  const response = await api("/api/session", { method: "GET", csrf: false });
  state.csrfToken = response.csrfToken;
}

async function loadData() {
  setLoading(true);
  try {
    const [providers, accounts] = await Promise.all([
      api("/api/providers", { method: "GET", csrf: false }),
      api("/api/accounts", { method: "GET", csrf: false }),
    ]);
    state.providers = providers;
    state.accounts = accounts;
    render();
  } finally {
    setLoading(false);
  }
}

async function importCurrentAccount() {
  const providerId = primaryProviderId();
  if (!providerId) {
    showMessage("没有可用 provider", true);
    return;
  }
  await runAction(async () => {
    await api(`/api/providers/${encodeURIComponent(providerId)}/accounts/import-current`, {
      method: "POST",
      body: {},
    });
    showMessage("当前账号已保存");
    await loadData();
  });
}

async function startLogin(providerId) {
  await runAction(async () => {
    const task = await api(`/api/providers/${encodeURIComponent(providerId)}/login-tasks/create`, {
      method: "POST",
      body: {},
    });
    if (task.authUrl) {
      window.open(task.authUrl, "_blank", "noopener,noreferrer");
    }
    showMessage("登录页面已打开，完成后会自动刷新账号列表");
    scheduleLoginPoll(task.id);
  });
}

function scheduleLoginPoll(taskId) {
  clearLoginPoll(taskId);
  const timer = window.setInterval(async () => {
    try {
      const status = await api(`/api/login-tasks/${encodeURIComponent(taskId)}`, {
        method: "GET",
        csrf: false,
      });
      if (status.state === "completed") {
        clearLoginPoll(taskId);
        showMessage("新账号已保存。切换账号后请 reload VS Code 窗口。");
        await loadData();
      }
      if (status.state === "failed" || status.state === "canceled") {
        clearLoginPoll(taskId);
        showMessage(`登录任务已结束：${status.state}`, status.state === "failed");
      }
    } catch (error) {
      clearLoginPoll(taskId);
      showMessage(error.message || "轮询登录任务失败", true);
    }
  }, 2500);
  state.loginTimers.set(taskId, timer);
}

function clearLoginPoll(taskId) {
  const timer = state.loginTimers.get(taskId);
  if (timer) {
    window.clearInterval(timer);
    state.loginTimers.delete(taskId);
  }
}

async function refreshUsage() {
  await runAction(async () => {
    await api("/api/usage/refresh", { method: "POST", body: {} });
    showMessage("额度已刷新");
    await loadData();
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
  if (options.csrf !== false && state.csrfToken) {
    init.headers["X-CSRF-Token"] = state.csrfToken;
  }

  const response = await fetch(path, init);
  const envelope = await response.json();
  if (!response.ok || envelope.error) {
    const message = envelope.error ? envelope.error.message : `HTTP ${response.status}`;
    throw new Error(message);
  }
  return envelope.data;
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
      section.append(emptyState("还没有账号", "保存当前账号，或新增一个 Codex 登录。"));
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
  const badge = document.createElement("span");
  badge.className = `status-badge ${providerInfo.status === "available" ? "" : "unavailable"}`;
  badge.textContent = providerInfo.status;
  wrapper.append(title, badge);
  return wrapper;
}

function providerActions(providerInfo) {
  const actions = document.createElement("div");
  actions.className = "toolbar";
  if (providerInfo.capabilities && providerInfo.capabilities.canLogin) {
    const loginButton = document.createElement("button");
    loginButton.type = "button";
    loginButton.textContent = "新增登录";
    loginButton.disabled = state.loading || providerInfo.status !== "available";
    loginButton.addEventListener("click", () => startLogin(providerInfo.id));
    actions.append(loginButton);
  }
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
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(actionButton("激活", () => activateAccount(account)));
  }
  actions.append(actionButton("重命名", () => renameAccount(account)));
  const deleteButton = actionButton("删除", () => deleteAccount(account));
  deleteButton.className = "danger";
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

function setLoading(loading) {
  state.loading = loading;
  elements.refreshButton.disabled = loading || !state.csrfToken;
  elements.importButton.disabled = loading || !state.csrfToken;
}

function primaryProviderId() {
  const available = state.providers.find((providerInfo) => providerInfo.status === "available");
  return available ? available.id : "";
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
