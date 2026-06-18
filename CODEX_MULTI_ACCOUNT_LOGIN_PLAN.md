# Codex 多账号登录执行计划

## 目标

在不影响当前正在使用的 Codex 账号 A 的前提下，通过前端操作引导用户登录并持久化账号 B/C 的 Codex 鉴权数据，后续可使用隔离凭据目录刷新各账号额度。

核心原则：

- 不覆盖默认 `CODEX_HOME`，不影响当前 A 账号。
- 不把 token、refresh token 或完整 `auth.json` 写入数据库、日志、前端存储或 API response。
- 每个被管理账号使用独立 `CODEX_HOME` 凭据目录。
- 登录任务使用临时 `CODEX_HOME`，成功后只复制 `auth.json` 到账号隔离目录。
- v1 只支持 VS Code + Codex 扩展环境；暂不支持 Cursor、Windsurf 或其他 VS Code-compatible IDE。

## 非目标

- 不实现 Cursor/Windsurf 扩展 runtime 探测。
- 不接管或复刻 OpenAI/Codex OAuth 协议。
- 不让前端读取、上传或保存 `auth.json`。
- 不支持远程浏览器自动化登录。
- 不自动替换当前活动 `~/.codex/auth.json`，除非用户显式执行账号激活。
- 不改造 Codex 官方登录页或浏览器登录流程。

## 背景约束

Codex 登录缓存可能存储在 `CODEX_HOME/auth.json`，也可能存储在 OS credential store。账号管理器需要拿到可复制的 `auth.json`，因此登录任务必须在临时 `CODEX_HOME/config.toml` 中强制：

```toml
cli_auth_credentials_store = "file"
```

临时 `config.toml` 只影响本次登录任务的 `CODEX_HOME`，不会影响用户默认 `~/.codex/config.toml` 或 VS Code 当前 A 账号。

## 目录设计

```text
<project-root>/
  config/
    app.example.json
    app.json
  .data/
    state.db
    login-tasks/
      <task_id>/
        codex-home/
          config.toml
          auth.json
        status.json
        codex-login.log
  .credentials/
    providers/
      codex/
        accounts/
          <account_id>/
            auth.json
  .run/
    ai-coding-account-manager
    server.pid
```

说明：

- `<project-root>/.credentials/providers/codex/accounts/<account_id>/auth.json` 是账号长期隔离凭据。
- `<project-root>/.data/login-tasks/<task_id>/codex-home` 是一次性登录沙箱，任务完成或取消后删除。
- `status.json` 只保存任务状态、错误码、候选 runtime 路径、期望邮箱和识别到的账号元数据。
- `codex-login.log` 如需保留，只允许保存非敏感摘要；默认不暴露完整日志到 API。

`config/app.example.json` 可提交到仓库，`config/app.json` 是本地配置文件并被 `.gitignore` 忽略。`.data`、`.credentials` 等路径通过配置文件字段控制；`.run` 是脚本固定运行目录。默认分开存放，避免运行状态和账号凭据混在一起。

## Runtime 发现策略

新增 `CodexRuntimeResolver`，只支持以下顺序：

1. `config/app.json` 的 `codexBin` 显式配置。
2. 当前 `PATH` 中的 `codex`。
3. VS Code Codex 扩展内置 runtime 的 best-effort 探测。

VS Code 探测范围：

```text
~/.vscode/extensions/
~/.vscode-server/extensions/
```

Windows/macOS 路径可后续补齐。v1 只要求 Linux 本机或 VS Code Server 场景。

候选 runtime 必须通过校验：

```bash
<candidate> --version
<candidate> login --help
<candidate> app-server --help
```

失败行为：

- 找不到 runtime：返回 `UNAVAILABLE`，提示安装 Codex CLI 或配置 `codexBin`。
- 找到 VS Code 扩展但无可执行 runtime：返回 `UNAVAILABLE`，不尝试调用 VS Code UI 登录。
- 检测到 Cursor/Windsurf 扩展：忽略并提示 v1 不支持。

## API 设计

遵循现有 envelope 和本地同源写请求约束。

### 创建登录任务

```text
POST /api/providers/{providerId}/login-tasks/create
```

Request:

```json
{
  "expectedEmail": "b@example.com",
  "mode": "browser"
}
```

字段：

- `expectedEmail` 可选。提供后，任务完成时必须和 `account/read` 识别的 email 匹配。
- `mode` v1 支持 `browser` 和 `device_code`。

Response:

```json
{
  "taskId": "login_xxx",
  "providerId": "codex",
  "status": "pending",
  "mode": "browser",
  "expectedEmail": "b@example.com",
  "createdAt": 1700000000000,
  "expiresAt": 1700000300000
}
```

### 查询登录任务

```text
GET /api/providers/{providerId}/login-tasks/{taskId}
```

Response:

```json
{
  "taskId": "login_xxx",
  "providerId": "codex",
  "status": "waiting_for_user",
  "mode": "browser",
  "loginUrl": "https://...",
  "deviceCode": null,
  "userCode": null,
  "account": null,
  "errorCode": null,
  "createdAt": 1700000000000,
  "updatedAt": 1700000010000,
  "expiresAt": 1700000300000
}
```

安全要求：

- 不返回 token。
- 不返回 `auth.json` 内容。
- `loginUrl` 只在 Codex CLI 输出可安全展示的情况下返回；否则前端只展示“请在浏览器完成登录”。

### 取消登录任务

```text
POST /api/providers/{providerId}/login-tasks/{taskId}/cancel
```

行为：

- 终止登录子进程。
- 删除临时 `login-tasks/<task_id>` 目录。
- 任务状态更新为 `cancelled`。

### 导入当前账号

已有：

```text
POST /api/providers/{providerId}/accounts/import-current
```

保留为辅助能力，只用于导入当前默认 `CODEX_HOME` 中已登录账号。它不满足“登录 B/C 不影响 A”的核心场景，前端默认不作为新增账号主入口。

## 登录任务状态机

```text
pending
  -> starting
  -> waiting_for_user
  -> verifying
  -> imported

pending|starting|waiting_for_user|verifying
  -> failed
  -> cancelled
  -> expired
```

状态含义：

- `pending`：任务已创建，尚未启动子进程。
- `starting`：正在创建临时 `CODEX_HOME` 并启动 Codex runtime。
- `waiting_for_user`：等待用户完成浏览器或 device code 登录。
- `verifying`：登录子进程结束，正在用 app-server 读取账号。
- `imported`：账号元数据已落库，`auth.json` 已复制到隔离目录。
- `failed`：任务失败，保存稳定 `errorCode` 和脱敏错误摘要。
- `cancelled`：用户取消。
- `expired`：超过任务有效期。

## 后端实现拆分

### `internal/infra/codexruntime`

职责：

- 解析 runtime 候选路径。
- 校验 `--version`、`login --help`、`app-server --help`。
- 返回可执行 runtime 路径和来源：`config`、`path`、`vscode_extension`。

### `internal/infra/loginrunner`

职责：

- 创建临时 `CODEX_HOME`。
- 写入临时 `config.toml`。
- 启动 `codex login` 或 `codex login --device-auth`。
- 捕获必要的登录提示信息。
- 支持 context cancellation、超时和子进程回收。

### `internal/service/login_task.go`

职责：

- 创建、查询、取消登录任务。
- 编排 runtime resolver、login runner、Codex provider 校验和凭据导入。
- 校验 `expectedEmail`。
- 成功后 upsert `accounts`，但不自动激活为当前 A 使用账号。

### `internal/dao/login_task.go`

v1 推荐先用内存任务表加磁盘临时目录，不新建 DB 表。原因：

- 登录任务是短生命周期操作状态，不需要长期保留。
- 避免把子进程状态、登录 URL 或错误摘要变成持久化兼容负担。
- 应用重启后清理未完成 `login-tasks` 目录即可。

如果后续需要跨重启恢复，再新增 `login_tasks` 表。

### `internal/provider.Provider`

保留当前 `ImportCurrentAccount`。

新增 provider 内部能力时不建议扩展通用接口；登录任务是 Codex 特定流程，应优先放在 Codex provider 或 Codex login service 内部，避免污染 fake/通用 provider contract。

## 前端交互

账号页新增“添加 Codex 账号”按钮。

流程：

1. 用户点击按钮。
2. 弹窗输入可选 `expectedEmail`。
3. 选择登录方式：
   - 浏览器登录
   - Device code 登录
4. 前端调用 `login-tasks/create`。
5. 前端轮询 `login-tasks/{taskId}`。
6. `waiting_for_user`：
   - 浏览器登录：展示“请在打开的浏览器完成 Codex 登录”。
   - Device code：展示 URL 和 user code。
7. `imported`：刷新账号列表。
8. `failed`：展示稳定错误文案和重试入口。

交互提示：

- 如果用户浏览器当前是 A 账号，登录 B/C 前需要在官方登录页切换账号或使用无痕窗口。
- 如果提供了 `expectedEmail`，导入结果不匹配时会失败，不会覆盖 B/C 隔离凭据。
- 导入 B/C 不会影响当前 A；只有点击“激活账号”才会替换活动 `CODEX_HOME/auth.json`。

## 持久化和安全

账号元数据继续使用现有 `accounts` 表：

```text
provider_id
account_id
storage_id
label
email
plan_type
is_active
created_at
updated_at
last_used_at
```

不新增 token 字段。`storage_id` 保留为数据库字段名，但值直接等于 `account_id`。

文件权限：

- `login-tasks/<task_id>`：`0700`
- `codex-home/config.toml`：`0600`
- `codex-home/auth.json`：`0600`
- `.credentials/.../<account_id>/auth.json`：`0600`

日志规则：

- 不记录 `auth.json` 内容。
- 不记录 access token、refresh token、authorization header。
- 错误摘要只保留 command、exit code、稳定错误码和脱敏 message。

清理规则：

- 成功导入后删除临时 `login-tasks/<task_id>`。
- 取消或失败后默认删除临时 `auth.json`。
- 应用启动时清理过期任务目录。

## 刷新额度流程

刷新 B/C 账号额度时：

1. 根据 `storage_id` 找到账号隔离目录；当前 `storage_id = account_id`。
2. 创建临时 `CODEX_HOME`，只从账号隔离目录复制 `auth.json`。
3. 使用临时 `CODEX_HOME` 启动 `codex app-server`。
4. 调用 `account/read { refreshToken: true }` 校验登录态。
5. 调用 `account/rateLimits/read`。
6. 将刷新后的临时 `auth.json` 原子复制回账号隔离目录。
7. 删除临时 `CODEX_HOME`，将 usage snapshot 写入 DB。

账号隔离目录长期只保存 `auth.json`。Codex app-server 运行过程中产生的 sqlite、installation、日志、插件锁等运行态文件只允许出现在临时目录，不能污染 `.credentials`。

## 阶段计划

### P1：runtime resolver

- 新增 Codex runtime resolver。
- 支持显式配置、PATH、VS Code 扩展目录探测。
- 单元测试覆盖候选排序、不可执行文件、缺少 app-server/login 能力。

验收：

- 无 CLI 但有 VS Code 扩展 runtime 时可识别。
- Cursor/Windsurf 扩展不会被选中。
- 找不到 runtime 时返回稳定 `UNAVAILABLE`。

### P2：login runner

- 创建临时 `CODEX_HOME`。
- 写入 `cli_auth_credentials_store = "file"`。
- 支持 `browser` 和 `device_code`。
- 支持超时、取消和子进程回收。

验收：

- 登录任务不会读写默认 `~/.codex`。
- 任务取消后子进程退出，临时目录被清理。
- 登录成功后临时目录存在 `auth.json`。

### P3：login task service 和 API

- 实现 create/query/cancel API。
- 实现状态机。
- 登录完成后用 Codex app-server 读取账号。
- 校验 `expectedEmail`。
- 复制 `auth.json` 到账号隔离目录，upsert 账号元数据。

验收：

- 导入 B/C 后当前 A 的 `~/.codex/auth.json` 不变。
- API response 不包含 token 或 `auth.json`。
- expected email 不匹配时任务失败且不落库。

### P4：前端接入

- 账号页添加新增账号入口。
- 实现任务轮询和状态展示。
- 支持 device code 展示。
- 成功后刷新账号列表。

验收：

- 用户可从前端完成 B/C 登录并看到新账号。
- 失败、取消、超时都有明确状态。
- 页面不展示任何敏感 token。

### P5：验证和文档

- 补充 README/TECHNICAL_DESIGN。
- 补充 fake provider 或测试 runner。
- 增加端到端手工验收清单。

验收：

- `go test ./...` 通过。
- 手工验证 A 使用中，导入 B/C 不影响 A。
- 手工验证激活 B 后才替换活动 `CODEX_HOME/auth.json`。

## 风险和处理

| 风险 | 处理 |
| --- | --- |
| VS Code 扩展 runtime 路径变化 | resolver 只做 best-effort；推荐用户显式配置 `codexBin` |
| 浏览器当前已登录 A，导致导入 A | 支持 `expectedEmail` 校验；前端提示切换账号或使用无痕窗口 |
| Codex 使用 keyring 导致无 `auth.json` | 临时 `config.toml` 强制 `cli_auth_credentials_store = "file"` |
| 登录任务挂起 | 任务超时、cancel API、进程 owner 和 cleanup |
| token 泄漏 | API/日志/DB 禁止敏感内容；文件权限 0600/0700 |
| app-server schema 变更 | 复用现有 Codex provider 映射，错误统一为稳定 `UNAVAILABLE` 或 `UNSUPPORTED` |

## 验收清单

- [ ] 当前 A 正在使用时，创建 B 登录任务不会修改默认 `CODEX_HOME/auth.json`。
- [ ] B 登录成功后，B 的 `auth.json` 只出现在账号隔离目录。
- [ ] C 登录成功后，B/C 可分别刷新额度。
- [ ] 激活 B 前，A 仍是当前活动账号。
- [ ] 点击激活 B 后，活动 `CODEX_HOME/auth.json` 才被替换。
- [ ] 无 Codex runtime 时返回明确错误，不展示无效登录入口。
- [ ] 只有 VS Code 扩展 runtime 会被探测；Cursor/Windsurf 不支持。
- [ ] API response、日志、数据库均不包含 token 或完整 `auth.json`。

## 参考

- Codex Authentication: https://developers.openai.com/codex/auth
- Codex Environment Variables: https://developers.openai.com/codex/environment-variables
- Codex IDE Extension Settings: https://developers.openai.com/codex/ide/settings
- Codex App Server: https://developers.openai.com/codex/app-server
