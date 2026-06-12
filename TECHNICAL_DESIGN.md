# AI Coding 多账号管理器技术方案

## 1. 文档状态

- 状态：设计评审
- 目标平台：Linux，本地运行
- 使用场景：统一管理本机 AI coding 工具账号；v1 支持 OpenAI Codex
- 目标运行时：Python `3.12+`
- 当前 Codex 验证版本：Codex CLI `0.140.0-alpha.2`
- 实现位置：项目根目录 `ai-coding-account-manager/`
- 方案形态：本地 Web 管理工具，不实现为 VS Code 插件

> 产品层使用 provider-aware 架构。v1 只实现 `codex` provider，并通过 Codex app-server 读取账号与 rate limit；未来其他 AI coding 工具必须通过独立 provider adapter 接入，不能把 Codex 私有协议扩散到通用服务层。

## 2. 背景与目标

当前存在三个 ChatGPT 账号，日常通过 VS Code Codex 扩展进行开发。单个账号达到 Codex 使用额度后，需要退出登录并重新认证，切换成本较高，也无法同时查看各账号的额度状态。后续还可能同时使用其他 AI coding 工具，因此项目名称、账号模型、API 和存储结构从第一版开始保持 provider-neutral。

本方案实现以下能力：

1. 通过浏览器中的简单管理页面查看不同 provider 的账号。
2. 提供统一的账号新增、重命名、重新认证、删除和激活操作。
3. 通过 provider adapter 读取各工具可公开获取的额度或使用状态。
4. v1 支持 Codex ChatGPT 账号的额度监控和活动账号切换。
5. 确保任何 provider 的 token 不进入项目仓库、浏览器存储、URL、日志或前端响应。
6. 使用单条本地命令启动服务并自动打开管理页面。
7. 新增 provider 时不迁移通用账号表、不修改现有 API envelope、不重写前端基础组件。

## 3. 非目标

v1 不实现以下能力：

- 不实现 Codex 以外 provider 的真实认证和账号切换，仅预留扩展边界。
- 不实现 VS Code 插件、状态栏或 Command Palette 命令。
- 不在正在执行的 Codex turn 中热切换账号。
- 不根据额度自动轮换账号或自动重试用户请求。
- 不绕过 OpenAI 的账号、套餐、workspace 或使用政策限制。
- 不抓取 ChatGPT 网页，不依赖浏览器 Cookie。
- 不把 ChatGPT 订阅额度转换为 API billing 数据。
- 不管理 Codex Cloud 中的远程任务归属。
- 不在多台开发机之间同步账号凭证。
- 不对局域网或公网提供访问。

自动轮换不纳入 v1。Codex IDE Extension 或 app-server 进程可能仍持有旧 token，运行中替换凭证无法保证当前请求、后续请求和日志归属同一账号。

### 3.1 Provider 扩展原则

- 通用层只使用 `provider_id`、`account_id`、能力声明和标准化 quota snapshot。
- provider 自己负责凭证格式、认证流程、CLI 探测、活动账号切换和 reload 提示。
- 不假设所有 provider 都支持额度查询、自动刷新、浏览器 OAuth 或账号切换。
- 前端根据 provider capability 决定是否展示“额度”“重新登录”“切换”等操作。
- 不通过继承层级预先抽象所有差异；v1 先定义 Codex 实际需要的最小 `ProviderAdapter` contract。

## 4. 已确认的 Codex 行为

根据当前 Codex 官方文档与本机协议 schema：

1. Codex CLI 与 Codex IDE Extension 共用登录缓存。
2. 文件型凭证默认位于 `$CODEX_HOME/auth.json`，未设置 `CODEX_HOME` 时为 `~/.codex/auth.json`。
3. `auth.json` 含 access token、refresh token 等敏感信息，必须按密码处理。
4. Codex app-server 是本地 JSON-RPC 接口，可被独立本地工具调用。
5. 当前协议提供：
   - `account/read`
   - `account/login/start`
   - `account/login/completed`
   - `account/rateLimits/read`
   - `account/rateLimits/updated`
   - `account/usage/read`
6. `account/rateLimits/read` 返回的数据包括：
   - `usedPercent`
   - `resetsAt`
   - `windowDurationMins`
   - `planType`
   - `credits`
   - 可选的多个 `limit_id` bucket
7. IDE 内 `/status` 只能查看当前登录账号，不提供个人多账号聚合视图。
8. 替换磁盘上的 `auth.json` 不保证已经运行的 IDE 扩展立即重新读取凭证。

官方参考：

- [Codex Authentication](https://developers.openai.com/codex/auth)
- [Codex App Server](https://developers.openai.com/codex/app-server)
- [Codex IDE Extension](https://developers.openai.com/codex/ide)
- [Codex Environment Variables](https://developers.openai.com/codex/environment-variables)
- [FastAPI Lifespan Events](https://fastapi.tiangolo.com/advanced/events/)
- [FastAPI Advanced Middleware](https://fastapi.tiangolo.com/advanced/middleware/)
- [Uvicorn Settings](https://www.uvicorn.org/settings/)
- [Python asyncio subprocess](https://docs.python.org/3/library/asyncio-subprocess.html)
- [Python os.replace](https://docs.python.org/3/library/os.html#os.replace)

## 5. 技术选型

### 5.1 后端

使用 Python `3.12+` 实现本地服务：

- FastAPI：HTTP API、依赖注入、异常处理和静态文件托管。
- Uvicorn：ASGI Server，仅监听 `127.0.0.1`，固定单进程运行。
- Pydantic v2：API request/response model 与边界校验。
- `pydantic-settings`：环境变量读取、类型转换和范围校验。
- AnyIO：后台任务组、取消、并发限制和服务关闭清理。
- `asyncio.create_subprocess_exec()`：无 shell 地启动 Codex app-server。
- `pathlib`、`os`、`tempfile`：账号目录、权限和原子文件替换。
- `hashlib`、`secrets`：账号目录哈希、bootstrap token、session 和 CSRF token。

不引入数据库。账号规模固定且很小，JSON 元数据加独立凭证目录即可满足一致性和恢复要求。FastAPI 应关闭生产运行时的 `/docs`、`/redoc` 和 `/openapi.json`，避免为本地敏感操作暴露额外调试面。

### 5.2 前端

使用原生 HTML、CSS 和 JavaScript：

- 单页面账号列表。
- 使用 `fetch` 调用同源本地 API。
- 使用轮询读取登录状态和刷新进度。
- 不使用 React/Vue，不需要构建步骤。
- 不使用 `localStorage`、`sessionStorage` 或 IndexedDB 保存账号数据。

### 5.3 本地启动方式

```bash
cd ai-coding-account-manager
uv sync
uv run ai-coding-account-manager
```

默认行为：

1. 监听 `127.0.0.1:43127`。
2. 如果端口被占用则直接失败，不自动监听公网地址。
3. 生成本次进程随机会话。
4. 自动打开管理页面。
5. 收到 `SIGINT` 或 `SIGTERM` 时停止子进程并退出。

支持 `--no-open`，用于不希望自动打开浏览器的场景。

开发调试可使用：

```bash
uv run uvicorn ai_coding_account_manager.main:create_app --factory --host 127.0.0.1 --port 43127
```

不启用 `--reload` 作为日常运行方式，因为 reload supervisor 会产生额外进程，增加 OAuth 任务、内存会话和 Codex 子进程的生命周期复杂度。

### 5.4 依赖与打包

使用 `uv` 管理解释器、依赖、lockfile、测试和构建。`pyproject.toml` 的依赖边界为：

运行依赖：

- `fastapi`
- `uvicorn`
- `pydantic-settings`
- `anyio`

开发依赖：

- `pytest`
- `pytest-anyio`
- `httpx`
- `asgi-lifespan`
- `ruff`
- `pyright`
- `pyinstaller`，仅用于发布构建

`uv.lock` 固定实际解析版本，文档不硬编码当前最新版本。console script 入口：

```toml
[project.scripts]
ai-coding-account-manager = "ai_coding_account_manager.cli:main"
```

静态文件作为 Python package data 随 wheel 分发，代码通过 `importlib.resources` 定位，不能依赖当前工作目录。v1 以 `uv run ai-coding-account-manager` 作为标准交付方式；功能稳定后再增加 PyInstaller 构建，生成单目录或单文件本地可执行程序。

## 6. 总体架构

```text
┌──────────────────────── Browser ────────────────────────┐
│                                                        │
│  Account Dashboard                                     │
│  - 账号列表                                             │
│  - 额度进度                                             │
│  - 新增/重命名/删除                                     │
│  - 切换账号                                             │
│                 │ same-origin HTTP                     │
└─────────────────┼──────────────────────────────────────┘
                  ▼
┌──────────── Local Server: 127.0.0.1 only ──────────────┐
│                                                        │
│  FastAPI / Uvicorn / StaticFiles                       │
│       │                    │                           │
│       ▼                    ▼                           │
│  AccountService      UsageRefreshScheduler             │
│       │                    │                           │
│       └──────────┬─────────┘                           │
│                  ▼                                     │
│       ProviderRegistry / ProviderAdapter               │
│                  │                                     │
│          ┌───────┴────────┐                            │
│          ▼                ▼                            │
│   CodexProvider     Future Providers                   │
│          │                                             │
│          ▼                                             │
│   CodexAppServerClient                                 │
│                                                        │
│  AccountStore（provider-aware metadata）                │
└───────┼────────────────────┼───────────────────────────┘
        │                    │
        ▼                    ▼
 ~/.local/share/...      codex app-server
 provider account data  isolated provider home
        │
        ▼
 active ~/.codex/auth.json
        │
        ▼
 VS Code / Codex CLI 在 reload 或重启后读取新账号
```

## 7. 模块设计

### 7.1 `ai_coding_account_manager/cli.py`

负责：

- 解析启动参数和环境变量。
- 验证数据目录和已启用 provider 的运行时依赖。
- 创建本次进程的安全会话。
- 启动固定监听 loopback 的 Uvicorn Server。
- 使用 `webbrowser.open()` 打开带一次性 bootstrap token 的页面。
- 将 `SIGINT` 和 `SIGTERM` 交给 Uvicorn/FastAPI lifespan 完成优雅退出。

### 7.2 `ai_coding_account_manager/main.py`

负责：

- 通过 application factory 创建 FastAPI app。
- 关闭 OpenAPI 和交互式文档端点。
- 注册 security middleware、exception handler、API routers 和静态文件。
- 使用 FastAPI `lifespan` 创建共享服务、注册 provider、启动刷新调度器并在退出时清理 provider 子进程。
- 共享对象放入 `app.state`，不使用可变模块级单例。

### 7.3 `AccountService`

负责：

- 组织新增、保存当前账号、重新登录、切换和删除流程。
- 串行化所有凭证写操作。
- 管理正在进行的 OAuth 登录任务。
- 管理额度刷新任务。
- 将后端错误转换为稳定的 API error code。
- 只通过 `ProviderRegistry` 调用 provider，不直接依赖 Codex 类。

### 7.4 `AccountStore`

负责：

- 保存账号元数据。
- 以 `(provider_id, account_id)` 作为账号唯一键。
- 保存通用账号元数据和 provider 私有状态目录。
- 向 provider 提供受约束的私有目录，不理解具体凭证格式。
- 校验路径，避免删除或覆盖非账号目录。

### 7.5 `ProviderRegistry` 与 `ProviderAdapter`

`ProviderRegistry` 负责：

- 按 `provider_id` 注册和解析 adapter。
- 启动时检查 provider ID 冲突。
- 向 API 暴露 provider 名称、状态和 capability。
- 隔离 provider 初始化失败，Codex provider 不可用时管理页面仍可启动。

最小 `ProviderAdapter` contract 包括：

- `describe()`：返回 provider 元数据与 capability。
- `discover_current_account()`：发现当前活动账号，可不支持。
- `start_login()`、`poll_login()`、`cancel_login()`：认证生命周期，可不支持。
- `refresh_account()`：刷新账号信息和标准化 usage snapshot。
- `activate_account()`：切换活动账号，可不支持。
- `remove_account_data()`：删除 provider 私有凭证。
- `close()`：清理子进程和后台任务。

capability 至少包括：

- `can_import_current_account`
- `can_login`
- `can_refresh_usage`
- `can_activate_account`
- `requires_client_reload_after_activate`

### 7.6 `CodexProvider`

负责：

- 将 Codex `account/read` 结果映射为通用账号信息。
- 将 Codex rate limit window 映射为标准化 usage snapshot。
- 管理每个 Codex 账号的隔离 `CODEX_HOME`。
- 在激活账号时同步并原子替换活动 `$CODEX_HOME/auth.json`。
- 返回 Codex IDE Extension 需要 reload 的 provider-specific 提示。

### 7.7 `CodexAppServerClient`

负责：

- 使用 `asyncio.create_subprocess_exec()` 启动指定 `CODEX_HOME` 的 `codex app-server --stdio` 子进程，不经过 shell。
- 执行 initialize/initialized 握手。
- 发送 JSON-RPC 请求并关联响应。
- 监听登录完成和额度更新通知。
- 提供超时、进程退出和取消处理。
- 并发读取 stdout/stderr，stderr 只输出脱敏后的诊断摘要，避免 pipe 堵塞。

### 7.8 `UsageRefreshScheduler`

负责：

- 服务启动后执行一次刷新。
- 按配置间隔刷新所有账号。
- 限制 app-server 并发数。
- 保存内存中的标准化 usage snapshot 和错误状态。
- 防止重复刷新任务重入。
- 使用 AnyIO task group 管理定时任务，应用退出时通过 cancel scope 统一取消。

### 7.9 `ai_coding_account_manager/security/local_session.py`

负责：

- bootstrap token 一次性兑换。
- session Cookie 和 CSRF token 签发、校验与过期。
- Host、Origin、Content-Type 和请求体大小检查。
- 提供 FastAPI dependency，默认拒绝未认证或校验失败的请求。

### 7.10 前端 `app.js`

负责：

- 渲染 provider 分组、账号卡片与 usage/额度组件。
- 发起新增、刷新、切换、重命名和删除请求。
- 轮询 OAuth 登录任务状态。
- 显示切换后的 reload 提示。
- 不接触或展示任何 token 字段。

## 8. 数据存储设计

### 8.1 存储位置

账号数据不写入项目目录，遵循 XDG 数据目录：

```text
${XDG_DATA_HOME:-~/.local/share}/ai-coding-account-manager/
├── state.json
└── providers/
    └── codex/
        └── accounts/
            ├── <account-hash-1>/
            │   ├── auth.json
            │   └── config.toml
            └── <account-hash-2>/
                ├── auth.json
                └── config.toml
```

通用层只规定 `providers/<provider_id>/accounts/<storage_id>/` 边界，不规定 provider 内部文件名。Codex provider 使用 `auth.json` 和 `config.toml`。`storage_id` 由 `provider_id + provider account_id` 做 SHA-256 后截取生成，不使用邮箱作为路径，避免暴露个人信息和跨 provider ID 冲突。

### 8.2 元数据结构

`state.json` 只保存非密钥信息：

```json
{
  "schemaVersion": 1,
  "accounts": [
    {
      "providerId": "codex",
      "accountId": "0123456789abcdef",
      "storageId": "fedcba9876543210",
      "label": "personal-1",
      "email": "masked@example.com",
      "planType": "plus",
      "isActive": true,
      "createdAt": "2026-06-12T08:00:00.000Z",
      "lastUsedAt": "2026-06-12T09:00:00.000Z"
    }
  ]
}
```

账号唯一键为 `(providerId, accountId)`。通用层不保存 provider 原始 token、原始认证响应或原始 quota payload。usage snapshot 默认只保存在服务内存中，服务重启后由对应 provider 重新读取，避免展示已经过期的历史数据。

### 8.3 文件权限

Linux/macOS：

- 数据根目录：`0700`
- 账号目录：`0700`
- `auth.json`：`0600`
- 写入流程：同目录创建 `0600` 临时文件，写入后执行 `flush()` 和 `os.fsync()`，校验内容，再通过 `os.replace()` 原子替换目标文件。
- 关键元数据替换后对父目录执行 `fsync`，保证异常断电后的目录项持久性。
- 阻塞文件操作集中在 repository 层，并通过 `anyio.to_thread.run_sync()` 从 async service 调用。

Windows 不是 v1 目标平台；后续支持时需要使用当前用户 ACL，并验证替换已有文件的恢复流程。

### 8.4 为什么不把凭证放在浏览器

浏览器存储会扩大 XSS、扩展程序和调试工具泄露凭证的风险。前端只接收账号 ID、名称、脱敏展示信息和额度状态，所有 token 均由本地后端处理。

## 9. 本地 Web 安全设计

本地 HTTP 服务仍然是安全边界。恶意网页可以尝试请求 localhost，因此不能因为服务仅监听本机就省略鉴权和 CSRF 防护。

### 9.1 网络边界

- 只监听 `127.0.0.1`，不监听 `0.0.0.0`、局域网地址或 IPv6 wildcard。
- 启动参数不提供远程监听能力。
- 使用 `TrustedHostMiddleware` 校验 Host，只允许 `127.0.0.1` 和 `localhost`；自定义中间件进一步校验实际配置端口。
- 不设置任何 CORS 响应头。
- 不安装 `CORSMiddleware`。
- 拒绝带有非本服务 Origin 的写请求。

### 9.2 启动会话

服务启动时生成至少 256 bit 随机 bootstrap token：

```text
http://127.0.0.1:43127/?bootstrap=<one-time-token>
```

首次访问后：

1. 后端校验一次性 token。
2. 建立随机 session。
3. 设置 `HttpOnly; SameSite=Strict; Path=/` Cookie。
4. 立即重定向到不含 token 的 `/`。
5. bootstrap token 失效，不能重复使用。

token 只存在于本机进程和首次打开的 URL，不写入日志、前端源码或持久化文件。

### 9.3 CSRF 与请求校验

- 所有 `/api/*` 接口必须有有效 session Cookie。
- 状态变更请求只允许 `POST` 或 `DELETE`。
- 状态变更请求必须携带 session 绑定的 `X-CSRF-Token`。
- 校验 `Origin` 为当前本地服务 origin。
- 只接受 `application/json`。
- 请求体设置较小上限，例如 16 KiB。
- 账号 ID、label 和 login task ID 都执行格式与长度校验。
- FastAPI/Pydantic validation error 转换为统一错误 envelope，不向前端返回内部 model 或堆栈。

### 9.4 浏览器响应安全头

至少设置：

```text
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cache-Control: no-store
```

前端不使用 inline script 和第三方 CDN。

## 10. 核心流程

### 10.1 首次保存当前账号

适用于用户已经在 Codex CLI 或 IDE Extension 中登录一个账号的场景。

```text
用户点击“保存当前账号”
        │
        ▼
POST /api/accounts/import-current
        │
        ▼
读取活动 $CODEX_HOME/auth.json
        │
        ▼
通过活动 CODEX_HOME 启动 app-server
        │
        ▼
account/read 获取 email / planType
        │
        ▼
复制 auth.json 到账号私有目录
        │
        ▼
记录为当前活动账号
```

如果 `account/read` 失败但 `auth.json` 结构有效，可以只保存凭证，邮箱和套餐显示为未知。

### 10.2 新增账号

新增账号不会先登出当前账号：

```text
POST /api/login-tasks
        │
        ▼
创建 .pending-<uuid> 临时 CODEX_HOME
        │
        ▼
启动 codex app-server
        │
        ▼
account/login/start { type: "chatgpt" }
        │
        ▼
后端返回 loginTaskId 和 authUrl
        │
        ▼
前端新窗口打开 authUrl
        │
        ▼
用户在浏览器完成登录
        │
        ▼
后端收到 account/login/completed
        │
        ▼
前端轮询 GET /api/login-tasks/:id
        │
        ▼
account/read + account/rateLimits/read
        │
        ▼
校验 auth.json 并迁移到正式账号目录
```

登录取消、超时或失败时终止 app-server，并删除 `.pending-*` 临时目录。登录任务只保存在内存中，设置 10 分钟过期时间。

### 10.3 额度刷新

每个保存账号都有独立 `CODEX_HOME`：

```text
for each account, concurrency = 2
    启动 app-server(CODEX_HOME=<account-home>)
    account/read { refreshToken: true }
    account/rateLimits/read
    更新账号元数据和内存额度快照
    停止 app-server
```

设计约束：

- 默认每 5 分钟刷新一次，可配置为 1 到 60 分钟。
- 同时最多启动两个 app-server。
- 单账号失败不影响其他账号刷新。
- 请求设置超时，子进程退出时拒绝所有 pending 请求。
- refresh token 可能被 Codex 自动轮换，刷新后的 `auth.json` 必须保留在该账号目录。
- 手动刷新与定时刷新共用一个 single-flight 任务，避免重入。
- 不使用网页抓取，不推断额度，只展示 app-server 返回的数据。

### 10.4 账号切换

```text
用户点击“切换到此账号”
        │
        ▼
前端展示中断当前 Codex 任务的确认框
        │
        ▼
POST /api/accounts/:id/activate
        │
        ▼
将当前活动 auth.json 同步回当前账号目录
        │
        ▼
校验目标账号 auth.json
        │
        ▼
原子替换活动 $CODEX_HOME/auth.json
        │
        ▼
更新 activeAccountId / lastUsedAt
        │
        ▼
前端提示执行 Developer: Reload Window
```

普通网页不能可靠调用 VS Code 的 `workbench.action.reloadWindow`，也不应通过模糊匹配强制 kill Codex 或 VS Code 进程。因此 v1 的切换定义为：完成磁盘凭证切换，然后要求用户在 VS Code 中执行 `Developer: Reload Window`。

如果此时没有运行 Codex CLI 或 IDE Extension，新启动的进程会直接读取目标账号。

切换不会覆盖 `~/.codex/config.toml`、skills、MCP、sessions 或其他 Codex 状态，只替换活动 `auth.json`。

### 10.5 重新登录失效账号

对于 refresh token 失效的账号：

1. 用户点击“重新登录”。
2. 后端为该账号创建新的 pending `CODEX_HOME`。
3. 用户完成 ChatGPT OAuth。
4. 后端校验新账号身份。
5. 只有新 `account_id` 与原账号一致时才覆盖原凭证。
6. 如果登录成另一个账号，则拒绝覆盖，并允许用户作为新账号保存。

## 11. HTTP API 设计

所有响应使用统一结构：

```json
{
  "data": {},
  "error": null
}
```

失败响应：

```json
{
  "data": null,
  "error": {
    "code": "ACCOUNT_AUTH_EXPIRED",
    "message": "账号登录已失效，请重新登录"
  }
}
```

API 列表：

| Method | Path | 用途 |
| --- | --- | --- |
| `GET` | `/api/session` | 获取 CSRF token 和服务状态 |
| `GET` | `/api/providers` | 获取 provider 状态和 capability |
| `GET` | `/api/accounts` | 获取全部 provider 的账号与 usage snapshot |
| `POST` | `/api/providers/:providerId/accounts/import-current` | 保存该 provider 当前活动账号 |
| `POST` | `/api/providers/:providerId/accounts/:accountId/activate` | 切换该 provider 活动凭证 |
| `POST` | `/api/providers/:providerId/accounts/:accountId/rename` | 重命名账号 |
| `POST` | `/api/providers/:providerId/accounts/:accountId/relogin` | 发起指定账号重新认证 |
| `DELETE` | `/api/providers/:providerId/accounts/:accountId` | 删除非活动账号 |
| `POST` | `/api/providers/:providerId/login-tasks` | 发起新增账号认证任务 |
| `GET` | `/api/login-tasks/:id` | 查询 OAuth 任务状态 |
| `DELETE` | `/api/login-tasks/:id` | 取消 OAuth 任务 |
| `POST` | `/api/usage/refresh` | 手动刷新支持 usage 查询的全部账号 |
| `GET` | `/api/health` | 本地进程健康检查 |

API 永远不返回以下字段：

- access token
- refresh token
- id token
- API key
- 完整 `auth.json`
- app-server 原始响应

通用账号响应必须包含 `providerId` 和 provider capability；provider-specific 字段只能放入经过 schema 约束的 `metadata`，不得改变通用顶层字段语义。

## 12. 前端页面设计

### 12.1 页面结构

```text
┌──────────────────────────────────────────────────────┐
│ AI Coding Account Manager          [刷新全部] [新增] │
├──────────────────────────────────────────────────────┤
│ Provider: Codex                                      │
│ 当前账号：personal-1                                 │
│ 切换后需要在 VS Code 执行 Developer: Reload Window   │
├──────────────────────────────────────────────────────┤
│ personal-1                          当前账号          │
│ user1@example.com · Plus                             │
│ 5 小时额度   ███████░░░  剩余 42%                   │
│ 7 天额度     ███░░░░░░░  剩余 68%                   │
│ 重置时间：06/12 18:30 / 06/16 09:00                 │
│ [重命名] [重新登录]                                  │
├──────────────────────────────────────────────────────┤
│ work-1                                               │
│ user2@example.com · Pro                              │
│ ...                                                  │
│ [切换] [重命名] [重新登录] [删除]                    │
└──────────────────────────────────────────────────────┘
```

### 12.2 页面状态

每个账号有以下状态之一：

- `ready`：账号和额度可用。
- `refreshing`：正在刷新额度。
- `auth_expired`：登录失效，需要重新登录。
- `rate_limit_reached`：已达到额度限制。
- `unavailable`：provider CLI、服务或本地协议暂时不可用。
- `unsupported`：该 provider 不支持对应 capability，例如无法读取额度。

### 12.3 交互原则

- 删除和切换必须二次确认。
- 当前活动账号不能直接删除。
- 页面按 provider 分组，并允许按 provider 过滤。
- capability 不支持的按钮不展示，不使用点击后再报错的方式模拟支持。
- 切换成功后显示醒目但不阻塞的 reload 指引。
- 页面刷新不影响正在进行的 OAuth 登录任务。
- 前端不展示底层异常栈，只展示稳定错误信息。
- 详细错误写入本地服务日志，但必须脱敏。

## 13. 配置设计

使用 `pydantic-settings` 从环境变量加载不可变 `Settings` model，避免为少量配置引入复杂配置文件：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AI_CODING_ACCOUNT_MANAGER_PORT` | `43127` | 本地监听端口 |
| `AI_CODING_ACCOUNT_MANAGER_REFRESH_MINUTES` | `5` | usage 刷新间隔 |
| `AI_CODING_ACCOUNT_MANAGER_DATA_DIR` | XDG data dir | 通用数据目录 |
| `AI_CODING_ACCOUNT_MANAGER_CODEX_BIN` | `codex` | Codex provider CLI 路径 |
| `CODEX_HOME` | `~/.codex` | 当前活动 Codex 状态目录 |

约束：

- 不提供监听地址配置，服务固定绑定 `127.0.0.1`。
- 端口必须在 `1024-65535` 范围内。
- 刷新间隔必须在 `1-60` 分钟范围内。
- Codex CLI 路径启动前必须验证可执行。
- 启动时一次性完成配置校验；配置非法则在监听端口前退出。
- API handler 不直接读取 `os.environ`，通过 FastAPI dependency 获取同一份 settings。

## 14. Provider 发现与兼容性

### 14.1 Codex provider CLI 定位顺序

1. `AI_CODING_ACCOUNT_MANAGER_CODEX_BIN` 指定的路径。
2. 当前进程 `PATH` 中的 `codex`。
3. 检测已安装 OpenAI Codex IDE Extension 的内置 CLI 路径，仅作为兼容性回退。

不应硬编码具体 VS Code extension 版本目录。

### 14.2 Provider 能力检查

每个 provider 在注册时返回 capability 和健康状态。Codex provider 首次刷新或管理账号时：

1. 启动 app-server。
2. 完成 initialize。
3. 调用 `account/read`。
4. 调用 `account/rateLimits/read`。
5. 如果返回 method not found，页面提示升级 Codex CLI 或 Codex IDE Extension。
6. 不退化为网页抓取或解析非稳定日志。

### 14.3 Provider 私有配置

活动账号路径必须遵守启动服务时的 `CODEX_HOME`。如果未设置，才使用 `~/.codex`。

Codex 账号监控进程使用工具自己的隔离 `CODEX_HOME`，不能把项目配置、sessions、memory 或 skills 复制到账号目录。未来其他 provider 必须定义自己的 home/config 定位规则，通用层不复用 `CODEX_HOME` 概念。

## 15. 并发与一致性

所有凭证写操作使用 `anyio.Lock` 串行执行：

- 保存当前账号。
- 新增账号最终落盘。
- 重新登录覆盖凭证。
- 切换活动账号。
- 删除账号。
- refresh token 回写。

切换流程的关键顺序固定为：

1. 获取写锁。
2. 同步当前账号最新凭证。
3. 校验目标凭证。
4. 原子替换活动凭证。
5. 更新元数据。
6. 释放写锁。

服务同一时间只允许一个账号切换请求。重复请求返回 `409 OPERATION_IN_PROGRESS`，不能并行覆盖 `auth.json`。

额度刷新使用 `anyio.CapacityLimiter(2)` 限制并发，并由一个 AnyIO task group 管理。手动刷新如果发现已有刷新任务运行，则复用现有任务结果，不创建第二批 app-server 子进程。

Uvicorn 固定使用单 worker。该方案的锁、session、OAuth task 和额度快照均为进程内状态，不支持多 worker；如果未来需要多进程，必须先引入跨进程锁和共享状态存储。

## 16. 异常处理

| 场景 | 处理方式 |
| --- | --- |
| Codex CLI 不存在 | 页面提示配置 CLI 路径或安装 Codex |
| app-server 协议不支持额度接口 | 提示升级，不抓取网页 |
| 某账号 refresh token 失效 | 标记“需要重新登录”，其他账号继续刷新 |
| 新增账号登录取消 | 取消 login，删除 pending 目录 |
| 新增账号登录超时 | 终止 app-server，删除 pending 目录 |
| 切换时目标 auth.json 无效 | 停止切换，不修改活动凭证 |
| 原子替换失败 | 保留或恢复原活动凭证，报告错误 |
| 当前账号未纳入管理 | 页面提示“保存当前账号” |
| 账号额度接口暂时失败 | 保留账号，显示最近错误，不自动切换 |
| 端口被占用 | 启动失败并输出明确端口信息 |
| 非法 Host、Origin 或 CSRF token | 返回 `403`，不执行任何操作 |
| 请求体过大或字段非法 | 返回 `400` 或 `413` |
| VS Code 仍使用旧账号 | 提示执行 `Developer: Reload Window` |

## 17. 日志与可观测性

日志默认写入 stderr，不持久化；可由用户自行重定向。

允许记录：

- 服务启动和停止。
- 账号哈希 ID 的前 6 位。
- 操作类型、耗时和结果。
- app-server 退出码。
- 稳定错误 code。

禁止记录：

- token、API key 和完整 `auth.json`。
- 完整 OAuth URL。
- bootstrap token、session Cookie 和 CSRF token。
- app-server 原始请求与响应。
- 未脱敏的错误对象或环境变量。

## 18. 测试方案

测试栈：

- `pytest`
- `pytest-anyio`
- `httpx.AsyncClient` + `ASGITransport`
- `asgi-lifespan` 的 `LifespanManager`
- Ruff：lint 和格式检查
- Pyright：静态类型检查

### 18.1 单元测试

覆盖：

- `auth.json` 解析与无效结构拒绝。
- `(provider_id, account_id)` 复合键与 storage ID 稳定生成。
- 元数据读写与非法账号 ID 过滤。
- provider registry 重复 ID 拒绝和 capability 校验。
- Codex provider 到通用 account/usage schema 的映射。
- 账号新增、重命名、删除。
- 当前活动 token 回写到账号目录。
- 目标账号原子切换与失败恢复。
- rate limit primary/secondary 格式化。
- 多 `limit_id` bucket 优先显示 `codex`。
- JSON-RPC 请求响应关联。
- 请求超时、app-server 退出和通知等待。
- Host、Origin、session 和 CSRF 校验。
- API 请求体大小与字段校验。
- 写操作互斥和重复请求冲突。

### 18.2 API 集成测试

使用 `tmp_path` 临时数据目录和 Python fake Codex app-server 子进程：

1. 未认证请求返回 `401`。
2. 非法 Origin 写请求返回 `403`。
3. bootstrap token 只能使用一次。
4. 前端 API 响应不包含敏感字段。
5. 新增账号任务可完成、取消和超时。
6. 单账号或单 provider 刷新失败不影响列表中的其他账号。
7. 切换失败不会破坏原活动凭证。
8. FastAPI lifespan 退出后没有遗留 Codex 子进程和后台刷新任务。

### 18.3 验证命令

```bash
uv sync
uv run ruff check .
uv run ruff format --check .
uv run pyright
uv run pytest
```

### 18.4 本机集成验证

使用当前已登录账号执行只读验证：

1. 隔离 `CODEX_HOME` 启动 app-server。
2. 完成 initialize。
3. `account/read` 能返回账号类型。
4. `account/rateLimits/read` 能返回 rate limit snapshot。
5. 测试输出不打印响应原文。

真实新增三个账号和实际切换需要用户在浏览器完成 OAuth，因此属于人工验收。

### 18.5 人工验收

1. 启动本地服务并打开管理页面。
2. 页面显示 `codex` provider 已启用及其 capability。
3. 保存当前账号 A。
4. 新增账号 B、C，不影响 A 的当前登录状态。
5. 页面同时显示 A、B、C 的额度与重置时间。
6. 切换到 B，页面提示 reload。
7. 在 VS Code 执行 `Developer: Reload Window`。
8. Codex `/status` 显示 B 的账号和额度。
9. 切回 A 后无需重新浏览器登录。
10. 等待 token refresh 后再次切换，A 的刷新后凭证仍然有效。
11. 删除非活动账号 C 后，本地私有账号目录被删除。
12. `git status` 不出现任何凭证文件。
13. 从其他网页构造 localhost 写请求时被拒绝。
14. 注册一个不支持 usage/activate 的 fake provider 时，页面隐藏对应操作且无需修改通用 API。

## 19. 交付物

```text
ai-coding-account-manager/
├── TECHNICAL_DESIGN.md
├── README.md
├── pyproject.toml
├── uv.lock
├── .python-version
├── src/
│   └── ai_coding_account_manager/
│       ├── __init__.py
│       ├── main.py
│       ├── cli.py
│       ├── config.py
│       ├── errors.py
│       ├── schemas.py
│       ├── api/
│       │   ├── accounts.py
│       │   ├── providers.py
│       │   ├── login_tasks.py
│       │   ├── usage.py
│       │   └── session.py
│       ├── services/
│       │   ├── account_service.py
│       │   └── usage_scheduler.py
│       ├── repositories/
│       │   └── account_store.py
│       ├── providers/
│       │   ├── base.py
│       │   ├── registry.py
│       │   └── codex/
│       │       ├── provider.py
│       │       ├── app_server.py
│       │       └── schemas.py
│       ├── security/
│       │   └── local_session.py
│       └── static/
│           ├── index.html
│           ├── app.css
│           └── app.js
├── tests/
│   ├── fake_codex_app_server.py
│   ├── fake_provider.py
│   ├── test_provider_registry.py
│   ├── test_account_service.py
│   ├── test_account_store.py
│   ├── test_codex_app_server.py
│   ├── test_http_security.py
│   ├── test_api.py
│   └── test_format.py
└── scripts/
    └── probe_codex_protocol.py
```

凭证和运行数据位于 XDG data dir，不属于上述项目交付物。

发布构建必须验证静态文件已包含在 wheel/PyInstaller 产物中。PyInstaller 启动后仍使用相同的 XDG 数据目录，不能把账号凭证写到解包临时目录或可执行文件所在目录。

## 20. 实施顺序

1. 创建 Python 3.12、`pyproject.toml`、`uv.lock` 和 `ai-coding-account-manager` console script 入口。
2. 实现通用 provider registry、capability model 和 provider-aware 账号存储。
3. 实现 Codex provider、app-server JSON-RPC client 和 usage 映射。
4. 实现 FastAPI application factory、lifespan 和本地 HTTP 安全边界。
5. 实现通用账号 API、认证 task 和 usage 刷新调度器。
6. 实现无框架的静态管理页面。
7. 添加单元测试、fake provider 和 fake app-server API 集成测试。
8. 运行 Ruff、Pyright 和 pytest。
9. 使用当前账号验证只读额度接口。
10. 验证新增账号 OAuth 流程。
11. 验证切换、VS Code reload 和 token 回写。
12. 使用三个真实账号完成验收。

## 21. 风险与决策

### 风险 1：Provider 私有协议发生变化

Codex app-server schema 与 CLI 版本绑定，未来 provider 也可能依赖各自不稳定的本地协议。缓解措施：

- provider 初始化后做 capability 和方法级能力检测。
- 对 method not found 给出明确升级提示。
- 不依赖协议中标记为 OpenAI internal 的 `chatgptAuthTokens` 登录方式。
- 不抓取网页或解析不稳定日志作为回退。
- provider 故障隔离，单个 provider 不可用不能阻止管理页面启动。

### 风险 2：localhost 接口被恶意网页调用

仅监听 loopback 不能单独解决该风险。必须同时实施 session、一次性 bootstrap token、Host/Origin 校验、SameSite Cookie、CSRF token、无 CORS 和严格 CSP。

### 风险 3：轮询触发 token 更新

`account/read { refreshToken: true }` 可能更新账号目录中的 `auth.json`。这是预期行为，但必须保证切换前把当前活动账号刷新后的凭证同步回账号目录，并串行化所有凭证写操作。

### 风险 4：切换时存在活动 Codex 请求

无法可靠迁移正在运行的请求。v1 明确提示切换会中断任务，并要求 reload VS Code 建立清晰账号边界。

### 风险 5：本机明文凭证

Codex 文件型登录本身使用明文 `auth.json`。v1 使用用户私有目录、最小权限和不进入仓库的方式降低风险，但不能防御已经获得当前操作系统用户权限的恶意进程。

### 风险 6：无法从网页可靠 reload VS Code

不使用进程模糊匹配、模拟键盘或非稳定 VS Code 内部接口。页面完成凭证切换后，要求用户执行 `Developer: Reload Window`。这是 v1 明确接受的一次人工操作。

### 风险 7：过早抽象所有 AI coding 工具

不同工具的认证、额度和切换模型可能完全不同。通用 contract 只覆盖账号生命周期、capability 和标准化 usage；Codex 私有字段保留在 Codex provider。新增第二个真实 provider 前，不为未知需求增加复杂继承层级或统一凭证格式。

## 22. 验收结论标准

满足以下全部条件才视为完成：

- 管理工具不依赖 VS Code 插件即可运行。
- 页面只能通过本机 loopback 安全会话访问。
- 产品命名、数据目录、API 和账号键均为 provider-neutral。
- Codex 通过 `codex` provider 接入，通用服务层不直接读取 `auth.json` 或调用 app-server。
- fake provider 能证明新增 provider 不需要修改通用 API envelope 和账号存储 schema。
- 三个账号均能保存且无需反复登录。
- 三个账号额度可独立刷新，单账号失败不影响其他账号。
- 切换后 reload VS Code，Codex IDE Extension 实际使用目标账号。
- 当前账号 refresh token 更新后仍可再次切换回来。
- 前端、API、日志和 Git 工作区中不出现凭证。
- localhost 跨站请求被 Host、Origin、session 和 CSRF 校验拒绝。
- 单元测试、API 集成测试和语法检查通过。
- 人工完成 A → B → C → A 的切换验收。
