# AI Coding 多账号管理器技术方案

## 1. 方案摘要

- 状态：MVP 设计评审
- 目标平台：Linux，本地运行
- 目标运行时：Go `1.26+`
- HTTP router：Chi
- ORM：GORM
- 数据库：SQLite
- 产品形态：前后端同仓的本地 Web 管理工具，MVP 使用单二进制启动；
  Docker 启动作为 post-MVP 交付项，不实现为 VS Code 插件
- v1 provider：OpenAI Codex
- Codex 验证基线：Codex CLI `0.140.0-alpha.2`

项目用于统一管理本机 AI coding 工具账号。MVP 支持保存多个 Codex ChatGPT
账号、查看最近额度、手动刷新额度、删除非活动账号和切换活动账号。

MVP 只落地 OpenAI Codex，不验证第二个真实 provider。通用边界保持最小化：
账号表和 API envelope 不绑定 Codex 私有凭证格式；Codex 的凭证格式、
app-server 协议和 `CODEX_HOME` 规则必须封装在 `CodexProvider` 内。

SQLite 只持久化非密钥业务状态。Codex `auth.json` 继续存放在权限受限的独立
账号目录，禁止写入数据库、WAL、日志或备份。

## 2. 目标与边界

### 2.1 目标

1. 在浏览器中查看 Codex 账号和额度状态。
2. 支持账号新增、保存当前账号、删除非活动账号和激活。
3. 通过 Codex adapter 获取可公开读取的账号和 usage 信息。
4. 使用 SQLite 可靠持久化账号元数据、活动状态、usage snapshot 和 schema 版本。
5. 确保 token 不进入数据库、项目仓库、浏览器存储、URL、日志或 API 响应。
6. MVP 使用单个 Go 二进制启动服务。
7. 为后续 provider 保留账号 schema 和 API envelope 边界，但 MVP 不做第二
   provider 的接入验证。

### 2.2 非目标

- v1 不实现 Codex 以外 provider 的真实认证和切换。
- MVP 不实现 Docker 启动、自动定时刷新、账号重命名或一键重新认证；这些能力
  作为 post-MVP 迭代项。
- 不实现自动轮换账号、请求重试或运行中热切换。
- 不绕过账号、套餐、workspace 或使用政策限制。
- 不抓取 ChatGPT 网页，不依赖浏览器 Cookie。
- 不把 ChatGPT 订阅额度映射为 API billing 数据。
- 不同步多台机器的数据库或账号凭证。
- 不把 SQLite 放在 NFS 等网络文件系统。
- 不向局域网或公网暴露管理页面。
- 不自动 reload、kill 或控制 VS Code/Codex 进程。
- 后端采用 Controller、Service、Entity、Model、DAO、Router、Middleware
  项目结构；前端 `frontend/` 作为独立 View。

运行中的 IDE Extension 或 app-server 可能持有旧 token，因此 v1 的切换定义为：
替换磁盘凭证后，由用户执行 `Developer: Reload Window`。

## 3. 已确认的 Codex 约束

1. Codex CLI 与 IDE Extension 共用登录缓存。
2. 文件型凭证位于 `$CODEX_HOME/auth.json`；默认 `CODEX_HOME=~/.codex`。
3. `auth.json` 含 access token、refresh token 等敏感信息。
4. Codex app-server 提供本地 JSON-RPC 接口。
5. v1 使用以下方法或通知：
   - `account/read`
   - `account/login/start`
   - `account/login/completed`
   - `account/rateLimits/read`
   - `account/rateLimits/updated`
6. rate limit 数据包括 `usedPercent`、`resetsAt`、
   `windowDurationMins`、`planType`、`credits` 和可选 bucket。
7. 替换磁盘 `auth.json` 不保证已运行的 IDE Extension 立即重新读取。

## 4. 技术选型与运行方式

### 4.1 后端

HTTP router 使用 Chi。Chi router 作为 `http.Handler` 运行在 Go 标准库
`http.Server` 上：

- `github.com/go-chi/chi/v5`：路由分组、path 参数和标准 `net/http`
  middleware 组合。
- `net/http.Server`：Chi 的底层运行容器，负责 timeout、header limit、
  连接生命周期和 `Shutdown()`；它不是第二个 Web framework。
- `gorm.io/gorm`：ORM、DAO 查询和事务封装。
- `github.com/libtnb/sqlite`：基于 `modernc.org/sqlite` 的 Pure-Go GORM
  SQLite driver。
- `os/exec.CommandContext`：启动并取消 Codex app-server。
- `context`、`sync`、channel：任务取消、锁、single-flight 和并发限制。
- `encoding/json` 和 `github.com/go-playground/validator/v10`：严格 HTTP
  body、请求 DTO 校验与 JSON-RPC 边界解析。
- `embed.FS`：将 HTML、CSS、JavaScript 和 migration 编译进二进制。
- `log/slog`：结构化、可脱敏日志。

Chi 只允许存在于 router、controller 和 middleware 层；GORM 只允许存在于
`internal/model`、`internal/dao` 和 `internal/infra/database`。service 和业务
entity 不依赖 `chi.Context`、`*gorm.DB` 或 GORM model。项目采用
轻量 MVC 目录结构；
前端静态页面只作为本地管理 UI，不引入独立前端 View 框架。

Router 使用 `chi.NewRouter()` 并显式注册 middleware。request logger、recovery、
安全头、Host、session 和 CSRF 均使用项目自定义 middleware，以便统一接入
`slog` 并避免记录 query string、Cookie 或其他敏感数据。Chi 版本由
`go.mod` 和 `go.sum` 固定，不在设计文档硬编码最新 patch 版本。

不直接使用 `http.ListenAndServe` 启动服务：应用必须显式构造 `http.Server`，
集中配置本项目要求的全部 timeout 和 header limit，并持有 server 实例用于
shutdown 编排。优雅关闭由应用捕获信号后调用 `http.Server.Shutdown()` 完成。

`github.com/libtnb/sqlite` 将 SQLite 以 Pure-Go 方式编译进程序，不依赖
CGO。最终用户无需安装 SQLite Server、`sqlite3` CLI、C 编译器或额外运行时。
`sqlite3` CLI 只作为可选排障工具。

### 4.2 前端

使用原生 HTML、CSS 和 JavaScript，不引入前端构建链。前后端代码位于同一个
代码仓库，但目录上保持前后端分离：前端资源放在顶层 `frontend/static`，
后端代码放在 `internal` 的 MVC 目录中。后端只提供 API，不负责嵌入或返回前端静态资源。前端通过 `fetch` 调用 API，不使用 `localStorage`、
`sessionStorage` 或 IndexedDB 保存账号数据。

### 4.3 原生启动与构建

```bash
go run ./cmd/ai-coding-account-manager
```

发布构建：

```bash
CGO_ENABLED=0 go build -trimpath -o dist/ai-coding-account-manager \
  ./cmd/ai-coding-account-manager
```

默认监听 `127.0.0.1:43127`，生成进程级安全会话，并在启动日志中输出管理页面
URL。端口被占用时直接失败，后端不主动调用桌面环境打开浏览器。

migration 通过 `//go:embed` 进入后端二进制。运行数据始终写入
XDG data dir，不能写入可执行文件目录或临时解包目录。

### 4.4 Docker 本地启动

Docker 本地启动不属于 MVP 首版范围，作为 post-MVP 交付项保留以下设计约束。
Docker 作为本地启动方式支持，目标是降低本机 Go 工具链要求，不改变本地安全
边界：

```bash
docker compose up --build
```

Docker 运行约束：

- 镜像内仍运行同一个 Go 二进制，不拆分独立前端服务或反向代理。
- `compose.yaml` 只允许发布到宿主机 `127.0.0.1:43127`，不支持
  `0.0.0.0:43127`、host network 或公网端口映射。
- 容器内可监听 `0.0.0.0:43127` 以配合 Docker port publishing；该能力只用于
  container network namespace，宿主机暴露地址仍必须是 loopback。
- 运行数据目录和 `CODEX_HOME` 通过 bind mount 或 named volume 持久化，凭证不
  写入镜像 layer、build context、环境变量或日志。
- Docker 镜像必须包含或可挂载可执行的 Codex CLI；缺失时启动前校验失败并给出
  明确错误。
- 原生运行和 Docker 运行均不主动打开浏览器；启动日志打印带 bootstrap token 的
  本地 URL。

## 5. 总体架构

```text
Browser
  └─ same-origin HTTP
      └─ net/http.Server
          └─ Chi Router
              ├─ Middleware / Controller
              ├─ AccountService
              ├─ ManualUsageRefresh
              ├─ DAO
              │   └─ Model
              │       └─ GORM -> Pure-Go SQLite driver -> SQLite
              ├─ CredentialStore
              └─ CodexProvider
                  └─ CodexAppServerClient
                      └─ codex app-server

CredentialStore
  └─ isolated provider directories and active auth.json replacement
```

### 5.1 模块职责

| 模块 | 职责 |
| --- | --- |
| `cmd/.../main.go` | 加载配置、初始化依赖、启动和关闭服务 |
| `frontend/static` | 前端 HTML、CSS 和 JavaScript，不接触 token |
| `internal/config` | 配置读取和启动参数校验 |
| `internal/httpserver` | `http.Server` 构造、timeout 和 header limit 配置 |
| `internal/router` | Chi Router、路由注册和 middleware 组装 |
| `internal/controller` | HTTP controller、request DTO、response envelope 和错误映射 |
| `internal/entity` | 业务实体、值对象和稳定错误码 |
| `internal/model` | 与数据库表对应的 GORM 持久化模型 |
| `internal/service` | 编排账号生命周期、事务边界和稳定错误码 |
| `internal/dao` | 面向表的数据库操作、model/entity 转换和稳定错误映射 |
| `internal/infra/database` | GORM 初始化、SQLite 配置、migration 和健康检查 |
| `internal/infra/credentials` | 凭证目录、权限、校验和原子替换 |
| `internal/infra/provider/codex` | Codex 认证、账号映射、usage 和切换实现 |
| `internal/infra/appserver` | 子进程、JSON-RPC、通知、超时和退出处理 |
| `internal/scheduler` | post-MVP 定时刷新、并发限制和 single-flight |
| `internal/security` | bootstrap、session、CSRF 和请求边界 |

依赖通过构造函数显式注入，不使用可变 package-level singleton。controller、
service、entity、model 和 dao 不依赖具体 provider、app-server 或 credentials
实现；`cmd` 负责创建基础设施依赖并注入 service。

### 5.2 MVC 边界约束

```text
Controller
  ├─ 解析 path/query/body
  ├─ transport validation
  ├─ 调用 Service
  └─ entity/service error -> HTTP error envelope

Service
  ├─ 业务编排
  ├─ 事务边界
  └─ Provider / DAO / Store

Frontend View
  └─ 通过同源 HTTP API 与 Controller 交互
```

controller 不直接执行 SQL、操作凭证文件或启动 app-server。service 方法接收
`r.Context()`，而不是 Chi 私有上下文，保证超时、取消和业务层框架无关。

### 5.3 MVC 持久化约束

```text
Service
  └─ DAO
      └─ Model
          └─ GORM -> Pure-Go SQLite driver
```

- controller 和 service 不持有 `*gorm.DB`。
- persistence model 位于 `internal/model`，与 API DTO、业务 entity 分离。
- `internal/dao` 每次调用使用 `db.WithContext(ctx)`，负责 GORM query、model/entity
  转换和稳定错误映射，不向 service 暴露 persistence model 或底层数据库错误。
- service 通过 transaction manager 或 DAO unit-of-work 明确开启事务。
- 禁止在循环中执行 GORM 查询、更新或删除；批量读取后再处理。
- 禁止隐式 association save/preload，关系加载必须显式声明。
- 更新使用 `Select`、`Updates` 或明确列名，禁止使用 `Save`。
- 查询明确指定条件和必要排序，不依赖数据库自然顺序。

## 6. Provider contract

`Provider` 使用 `context.Context` 传播超时和取消：

```go
type Provider interface {
    Describe(context.Context) (Description, error)
    DiscoverCurrentAccount(context.Context) (*Account, error)
    StartLogin(context.Context) (*LoginTask, error)
    PollLogin(context.Context, string) (*LoginStatus, error)
    CancelLogin(context.Context, string) error
    RefreshAccount(context.Context, Account) (*UsageSnapshot, error)
    ActivateAccount(context.Context, Account) error
    RemoveAccountData(context.Context, Account) error
    Close(context.Context) error
}
```

能力声明至少包括：

- `can_import_current_account`
- `can_login`
- `can_refresh_usage`
- `can_activate_account`
- `requires_client_reload_after_activate`

规则：

- 通用账号唯一键为 `(provider_id, account_id)`。
- capability 不支持的操作不在前端展示，并返回稳定 `UNSUPPORTED` 错误。
- provider 私有字段只能进入经过类型约束的 metadata。
- provider 初始化失败应被隔离，不能阻止管理页面启动。
- 新增第二个真实 provider 前，不抽象未知的认证或凭证格式。

Codex CLI 定位顺序：

1. `AI_CODING_ACCOUNT_MANAGER_CODEX_BIN`。
2. 当前进程 `PATH` 中的 `codex`。
3. 已安装 Codex IDE Extension 的内置 CLI，仅作兼容回退。

不得硬编码 Extension 版本目录。app-server 方法不可用时提示升级，不回退到网页
抓取或日志解析。

## 7. SQLite 与凭证存储

### 7.1 存储布局

```text
${XDG_DATA_HOME:-~/.local/share}/ai-coding-account-manager/
├── state.db
├── state.db-wal
├── state.db-shm
└── providers/
    └── codex/
        └── accounts/
            └── <storage-id>/
                └── auth.json
```

`state.db-wal` 和 `state.db-shm` 由 WAL mode 管理，不能单独复制或删除。备份
必须使用 SQLite backup API，或在服务停止且完成 checkpoint 后复制完整数据库。

SQLite 持久化：

- provider/account 元数据。
- 当前活动账号。
- 最新 usage snapshot、刷新时间和稳定错误码。
- schema migration 版本。

SQLite 禁止持久化：

- access token、refresh token、id token、API key。
- 完整 `auth.json` 或 app-server 原始响应。
- OAuth URL、bootstrap token、session Cookie、CSRF token。
- 内存中的 OAuth task 和 JSON-RPC pending request。

### 7.2 Schema

```sql
CREATE TABLE accounts (
    provider_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    storage_id TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    email TEXT,
    plan_type TEXT,
    is_active INTEGER NOT NULL DEFAULT 0
        CHECK (is_active IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_used_at INTEGER,
    PRIMARY KEY (provider_id, account_id)
);

CREATE UNIQUE INDEX accounts_one_active_per_provider
ON accounts(provider_id)
WHERE is_active = 1;

CREATE TABLE usage_snapshots (
    provider_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    status TEXT NOT NULL,
    used_percent REAL,
    resets_at INTEGER,
    snapshot_json TEXT,
    error_code TEXT,
    refreshed_at INTEGER NOT NULL,
    PRIMARY KEY (provider_id, account_id),
    FOREIGN KEY (provider_id, account_id)
        REFERENCES accounts(provider_id, account_id)
        ON DELETE CASCADE
);

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
);
```

时间统一保存为 UTC Unix millisecond。通用高频字段直接建列；provider-specific
usage bucket 保存为经过 schema 校验的 JSON。数据库约束负责唯一活动账号、
复合账号键和级联删除，不能只依赖应用层检查。

`storage_id` 由 `provider_id + account_id` 做 SHA-256 后截取，不使用邮箱。

`internal/model` 中的 GORM persistence model 显式映射表名和列名，不嵌入
`gorm.Model`，避免引入
自增 ID、软删除和与既定 schema 不一致的时间字段。复合主键通过 GORM tag 映射，
partial unique index 和外键仍由 SQL migration 创建。

### 7.3 Driver 与连接配置

使用：

```go
import (
    "github.com/libtnb/sqlite"
    "gorm.io/gorm"
)

db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
    TranslateError: true,
})
```

数据库 DSN 为本地绝对路径 URI，并通过 driver 的 `_pragma` 参数确保每个新连接
应用关键设置：

```text
file:/absolute/path/state.db
  ?_pragma=foreign_keys(1)
  &_pragma=journal_mode(WAL)
  &_pragma=synchronous(FULL)
  &_pragma=busy_timeout(5000)
```

实际构造 DSN 时必须使用 URL API 编码，不能手工拼接路径。

连接池保持保守：

```go
sqlDB, err := db.DB()
sqlDB.SetMaxOpenConns(1)
sqlDB.SetMaxIdleConns(1)
sqlDB.SetConnMaxLifetime(0)
```

本项目写入量低，单连接可简化 SQLite 单 writer、PRAGMA 和事务行为。所有 SQL
操作必须从 `db.WithContext(ctx)` 派生；应用关闭时通过底层 `sql.DB.Close()`
释放连接。

GORM 配置要求：

- 不启用全局 `PrepareStmt`，当前数据量不值得增加 statement 生命周期复杂度。
- 保留默认写事务；多条相关写操作使用显式 `db.Transaction(...)`。
- logger 使用参数化 SQL，不输出 SQL 参数或敏感字段。
- 不使用 `AllowGlobalUpdate`。
- 将 `ErrRecordNotFound`、duplicated key、foreign key violation 和
  `SQLITE_BUSY` 映射为稳定业务错误。

启动后验证：

- `PRAGMA foreign_keys` 为 `1`。
- `PRAGMA journal_mode` 为 `wal`。
- migration 全部成功。
- `PRAGMA quick_check` 返回 `ok`；失败则拒绝启动写服务。

### 7.4 Migration

- SQL 文件放在 `internal/infra/database/migrations/` 并通过 `embed.FS` 打包。
- migration 文件按单调递增版本命名，例如 `0001_initial.sql`。
- migration runner 通过 GORM `Exec` 在短事务内执行，成功后写入
  `schema_migrations`。
- 已应用 migration 不修改；修复通过新版本追加。
- 数据库 schema 高于程序支持版本时拒绝启动，避免旧程序破坏新数据。
- v1 尚无已发布持久化格式，不设计 legacy import；未来引入数据迁移必须显式、
  幂等并提供备份与回滚说明。
- 正式启动流程禁止调用 `AutoMigrate`。GORM model 只负责运行时映射，不能作为
  schema 的唯一事实来源。

### 7.5 文件权限与原子写

- 数据根目录和账号目录：`0700`
- `state.db`、WAL、SHM 和凭证文件：仅当前用户可访问
- 凭证临时文件与目标文件在同一目录创建，权限 `0600`
- 写入后执行 `File.Sync()`、重新解析校验，再通过 `os.Rename()` 原子替换
- 替换后同步父目录
- 删除前校验目标必须位于受管账号目录

数据库文件的权限在创建后立即校验并收紧。应用不依赖进程 `umask` 提供唯一
保护。

## 8. 本地 Web 安全

仅发布到 loopback 不能防止恶意网页请求 localhost，因此本地 HTTP 仍是安全边界。

### 8.1 网络与 Server

- 原生运行仅监听 `127.0.0.1`，不提供公网监听配置。
- post-MVP Docker 运行只允许容器内监听 `0.0.0.0`，宿主机端口发布必须绑定
  `127.0.0.1`。
- Host 仅允许 `127.0.0.1`、`localhost` 和实际配置端口。
- 不启用 CORS，不返回 CORS 响应头。
- Chi router 通过 `chi.NewRouter()` 显式注册 middleware，不依赖框架默认安全配置。
- 禁止信任反向代理和 `X-Forwarded-*`，客户端地址只取实际 loopback 连接。
- `http.Server` 设置 `ReadHeaderTimeout`、`ReadTimeout`、
  `WriteTimeout` 和 `IdleTimeout`。
- 关闭时先停止接收请求，再取消后台任务、停止 app-server、checkpoint 并关闭 DB。

### 8.2 启动会话

- 启动时使用 `crypto/rand` 生成至少 256 bit 一次性 bootstrap token。
- 首次访问
  `http://127.0.0.1:43127/?bootstrap=<token>` 后兑换 session。
- 设置 `HttpOnly; SameSite=Strict; Path=/` Cookie，并重定向到无 token 的
  `/`。
- bootstrap token 不写入数据库、日志、前端源码或持久化文件。

### 8.3 写请求校验

所有 API 请求要求有效 session。状态变更只允许 `POST` 或 `DELETE`，并要求：

- `X-CSRF-Token` 与 session 绑定。
- `Origin` 等于当前服务 origin。
- `Content-Type: application/json`。
- 使用 `http.MaxBytesReader` 限制请求体不超过 16 KiB。
- 使用统一 strict JSON helper，基于 `json.Decoder` 拒绝未知字段、多个 JSON
  值和尾随内容；不依赖路由框架的默认请求绑定行为。
- ID、label 和 task ID 通过格式与长度校验。

validation error 使用统一错误 envelope，不返回内部 struct、环境变量或堆栈。

路由按职责分组：

- public：bootstrap 页面和必要静态资源。
- session：要求有效 session 的只读 API。
- mutation：额外要求 Origin、CSRF、JSON Content-Type 和 body limit。

middleware 默认拒绝，handler 不重复实现安全校验。

### 8.4 响应安全头

```text
Content-Security-Policy:
  default-src 'self';
  script-src 'self';
  style-src 'self';
  img-src 'self';
  connect-src 'self';
  frame-ancestors 'none';
  base-uri 'none';
  form-action 'self'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cache-Control: no-store
```

前端不使用 inline script 或第三方 CDN。

## 9. 核心流程与一致性

### 9.1 保存当前账号

1. 读取并校验活动 `$CODEX_HOME/auth.json`。
2. 使用活动 `CODEX_HOME` 启动 app-server。
3. 调用 `account/read` 获取账号信息。
4. 将凭证原子写入账号私有目录。
5. 在短事务中 upsert 账号并设置为 provider 的活动账号。
6. 数据库失败时删除本次新建的凭证目录；已有账号凭证不得被误删。

`account/read` 失败但凭证结构有效时可保存，展示信息标记为未知。

### 9.2 新增账号登录

1. 创建 `.pending-<uuid>` 隔离 `CODEX_HOME`。
2. 通过 `exec.CommandContext` 启动 app-server。
3. 调用 `account/login/start { type: "chatgpt" }`。
4. 前端打开 `authUrl` 并轮询登录任务。
5. 登录完成后读取账号、额度并校验凭证。
6. 原子迁移凭证到正式账号目录。
7. 在短事务中 upsert 账号和 usage snapshot。

任务只保存在内存，默认 10 分钟过期。取消、失败或超时必须取消 context、等待
或终止子进程并删除 pending 目录。MVP 不提供一键重新认证；后续实现重新认证时
仅允许相同 `account_id` 覆盖原凭证，不同账号应作为新账号保存。

### 9.3 额度刷新

每个账号使用独立 `CODEX_HOME`：

1. 在事务外启动 app-server。
2. 调用 `account/read { refreshToken: true }`。
3. 调用 `account/rateLimits/read`。
4. 停止 app-server。
5. 在短事务中更新账号元数据和 usage snapshot。

MVP 仅由用户手动触发刷新，不启动定时 scheduler。手动刷新全部账号时并发上限为
2，使用 buffered channel 作为 semaphore；重复刷新请求返回
`409 OPERATION_IN_PROGRESS`。单账号失败不影响其他账号；失败状态也持久化，
但保留最近一次有效 snapshot。

任何 app-server、OAuth 或文件 I/O 都不能在数据库事务中执行。

数据库事务由 unit-of-work 实现：

```go
type UnitOfWork interface {
    WithinTransaction(
        context.Context,
        func(DAOs) error,
    ) error
}
```

GORM unit-of-work 内部使用 `db.WithContext(ctx).Transaction(...)`，构造绑定同一个
`*gorm.DB` transaction 的 DAO 集合并传入回调。禁止在 service 中直接调用 GORM。

### 9.4 切换账号

SQLite 事务无法与文件系统替换组成同一个原子事务，因此切换使用应用级写锁和
补偿恢复：

1. 获取全局凭证写锁。
2. 将当前活动 `auth.json` 同步回当前账号目录。
3. 校验目标凭证并保存活动凭证的恢复副本。
4. 原子替换活动 `$CODEX_HOME/auth.json`。
5. 在短事务中清除原活动标记并设置目标账号为活动账号。
6. 数据库提交失败时原子恢复旧活动凭证。
7. 删除恢复副本并释放锁。
8. 提示用户执行 `Developer: Reload Window`。

只替换 `auth.json`，不覆盖 `config.toml`、skills、MCP、sessions 或其他
Codex 状态。切换前必须提示可能中断当前 Codex 请求。

启动时通过活动 `auth.json` 重新识别真实账号并校准 `is_active`，用于恢复
进程崩溃造成的文件与数据库状态偏差。无法识别时不猜测活动账号，并返回可恢复
错误。

### 9.5 删除账号

1. 在事务中确认目标不是活动账号并删除数据库记录。
2. 提交成功后删除凭证目录。
3. 文件删除失败时记录稳定错误并在后续启动清理孤立目录。

不允许对活动账号执行删除。启动清理只处理能证明位于受管目录且不再被数据库
引用的目录。

## 10. HTTP API

统一成功响应：

```json
{"data": {}, "error": null}
```

统一失败响应：

```json
{
  "data": null,
  "error": {
    "code": "ACCOUNT_AUTH_EXPIRED",
    "message": "账号登录已失效，请重新登录"
  }
}
```

| Method | Path | 用途 |
| --- | --- | --- |
| `GET` | `/api/session` | CSRF token 和服务状态 |
| `GET` | `/api/accounts` | 账号和持久化 usage snapshot |
| `POST` | `/api/accounts/import-current` | 保存当前 Codex 账号 |
| `POST` | `/api/accounts/{accountId}/activate` | 激活账号 |
| `DELETE` | `/api/accounts/{accountId}` | 删除非活动账号 |
| `POST` | `/api/login-tasks/create` | 新增 Codex 账号认证 |
| `GET` | `/api/login-tasks/{id}` | 查询认证任务 |
| `DELETE` | `/api/login-tasks/{id}` | 取消认证任务 |
| `POST` | `/api/usage/refresh` | 刷新全部可用账号 |
| `GET` | `/api/health` | 进程和数据库健康检查 |

API 永远不返回 token、API key、完整 `auth.json` 或 app-server 原始响应。

## 11. 前端交互

MVP 页面只显示 Codex 账号、套餐、额度、重置时间和状态。账号状态包括：

- `ready`
- `refreshing`
- `auth_expired`
- `rate_limit_reached`
- `unavailable`

交互规则：

- 删除和切换需要二次确认。
- 活动账号不能直接删除。
- 切换成功后显示 reload 指引。
- 页面刷新不影响内存中的 OAuth 任务。
- 数据库中的最近 snapshot 可在启动后立即展示，并标记刷新时间。
- 前端只展示稳定错误信息，不展示异常栈。

## 12. 配置

使用 typed `Config` 在启动时一次性读取和校验环境变量：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AI_CODING_ACCOUNT_MANAGER_PORT` | `43127` | 本地端口 |
| `AI_CODING_ACCOUNT_MANAGER_BIND_ADDR` | `127.0.0.1` | 监听地址 |
| `AI_CODING_ACCOUNT_MANAGER_DATA_DIR` | XDG data dir | 数据目录 |
| `AI_CODING_ACCOUNT_MANAGER_CODEX_BIN` | `codex` | Codex CLI |
| `CODEX_HOME` | `~/.codex` | 活动 Codex 状态目录 |

约束：

- 端口范围为 `1024-65535`。
- MVP 阶段 `AI_CODING_ACCOUNT_MANAGER_BIND_ADDR` 只接受 `127.0.0.1`。
- post-MVP Docker 入口可显式允许容器内 `0.0.0.0`，但必须配合宿主机 loopback
  端口发布。
- 启动监听前校验 CLI、数据目录、数据库和全部配置。
- controller、model 和 DAO 不直接读取环境变量。
- 不提供数据库路径指向网络文件系统的受支持配置。
- post-MVP Docker 运行时不允许通过环境变量传入 token、完整 `auth.json` 或
  OAuth URL。

## 13. 并发、错误与日志

### 13.1 并发与资源管理

- 凭证和活动账号变更使用同一个 `sync.Mutex`。
- 同时只允许一个账号切换；重复请求返回
  `409 OPERATION_IN_PROGRESS`。
- 手动 usage 刷新使用容量为 2 的 semaphore。
- usage refresh 和 login task 使用父 `context.Context`，关闭时统一取消并等待。
- app-server 子进程必须绑定 context，并同时消费 stdout/stderr，避免 pipe 阻塞。
- JSON-RPC request ID 到 response channel 的映射必须加锁，关闭时拒绝全部 pending。
- SQLite 连接池限制为单连接，事务必须短小且禁止外部 I/O。
- GORM query chain 不跨 goroutine 共享；每次 DAO 调用从基础
  `*gorm.DB` 创建独立 session。
- HTTP handler 不启动脱离请求生命周期的 goroutine；后台任务必须交给受控
  refresh/login task manager。
- HTTP Server 和后台 goroutine 必须有明确 shutdown 顺序。

### 13.2 关键错误处理

| 场景 | 处理 |
| --- | --- |
| Codex CLI 不存在 | 提示安装或配置路径 |
| app-server 方法不可用 | 提示升级 Codex |
| refresh token 失效 | 标记需要重新登录，继续刷新其他账号 |
| OAuth 取消或超时 | 停止子进程并删除 pending 目录 |
| 目标凭证无效 | 停止切换，不修改活动凭证 |
| 文件替换后 DB 提交失败 | 恢复原活动凭证 |
| SQLite busy | 最多等待 `busy_timeout`，超时返回稳定错误 |
| migration 失败或 schema 过新 | 拒绝启动写服务 |
| quick check 失败 | 拒绝启动并提示从备份恢复 |
| usage 暂时失败 | 保留最近 snapshot 并记录错误状态 |
| 非法 Host、Origin 或 CSRF | 返回 `403`，不执行操作 |
| 请求体过大或字段非法 | 返回 `400` 或 `413` |

### 13.3 日志

使用 `log/slog` 写 stderr，默认不持久化。可记录操作类型、耗时、结果、脱敏
账号 ID、app-server 退出码、SQLite 错误类别和稳定错误码。

禁止记录 token、API key、完整凭证、完整 OAuth URL、bootstrap/session/CSRF
token、app-server 原始报文、SQL 参数中的 PII、环境变量或未脱敏异常对象。

## 14. 测试与交付

### 14.1 自动化测试

使用 Go 标准 `testing`、`httptest`、真实 Chi router 和临时目录。路由测试
通过 `http.Handler` 执行，fake app-server 作为测试子进程运行。

MVP 重点覆盖：

- migration 从空库初始化、重复执行、失败回滚和 schema 过新拒绝。
- SQLite 唯一约束、外键、级联删除和单活动账号约束。
- GORM model 的表名、列名和 tag 映射。
- DAO 查询、model/entity 转换和稳定错误处理。
- unit-of-work 提交、回滚、panic rollback、context 取消和 busy timeout。
- 禁止 `Save`、global update、隐式 association 和循环内查询的静态/评审检查。
- 凭证解析、目录权限、路径校验和原子写恢复。
- 文件替换成功但数据库提交失败时的补偿恢复。
- 启动校准、孤立 pending/凭证目录清理。
- Codex schema 映射和 app-server 错误处理。
- JSON-RPC 请求关联、通知、超时和子进程退出。
- Host、Origin、session、CSRF、请求体和字段校验。
- Chi route group 的 middleware 覆盖、404/405 和 panic recovery。
- controller 到 service 的参数、context 取消和业务错误映射。
- 写操作互斥、手动刷新去重和并发限制。
- API、数据库和日志不包含敏感字段。
- shutdown 后无遗留 goroutine、DB connection 或 app-server 子进程。

MVP 基线验证命令：

```bash
gofmt -l .
go vet ./...
go test ./...
CGO_ENABLED=0 go build ./cmd/ai-coding-account-manager
```

完整发布或实现相关能力后，额外运行 `go test -race ./...`、目标平台交叉编译、
二进制启动 smoke test、`docker compose config`、`docker compose build` 和
Docker 启动 smoke test。

### 14.2 MVP 交付结构

```text
ai-coding-account-manager/
├── TECHNICAL_DESIGN.md
├── README.md
├── go.mod
├── go.sum
├── frontend/
│   └── static/
│       ├── index.html
│       ├── app.css
│       └── app.js
├── cmd/
│   └── ai-coding-account-manager/
│       └── main.go
├── internal/
│   ├── config/
│   ├── httpserver/
│   ├── router/
│   ├── controller/
│   ├── entity/
│   ├── model/
│   ├── service/
│   ├── dao/
│   ├── infra/
│   │   ├── database/
│   │   │   └── migrations/
│   │   ├── credentials/
│   │   ├── provider/
│   │   │   ├── codex/
│   │   │   └── fake/
│   │   └── appserver/
│   ├── scheduler/
│   ├── middleware/
│   └── security/
└── scripts/
```

凭证和运行数据位于 XDG data dir，不属于项目交付物。发布构建必须验证
migration 已嵌入二进制。Dockerfile、compose、`.dockerignore` 和
`internal/scheduler` 在 post-MVP 阶段补齐。

### 14.3 MVP 实施顺序

1. 基础骨架：初始化 Go module、命令入口、配置读取、`slog`、loopback
   `http.Server` 和 shutdown。
2. 本地状态与凭证存储：实现 SQLite 初始化、SQL migration、model、DAO、
   unit-of-work、账号/usage 表和 `auth.json` 原子读写，确保 token 不进入数据库
   和日志。
3. Codex 账号能力：实现 Codex app-server client、保存当前账号、新增账号登录、
   读取账号信息和手动刷新额度。
4. 账号操作 API：实现 session、Host、Origin、CSRF、账号列表、保存当前账号、
   登录任务、手动刷新、激活和删除非活动账号。
5. 最小前端与验收：实现账号列表、额度展示、操作按钮、切换确认、reload 提示、
   README 启动说明，并完成本机三账号人工验收。

### 14.4 MVP 延后项

- Dockerfile、compose、Docker smoke test 和容器内 `0.0.0.0` 监听入口。
- fake provider、provider-neutral capability 页面和第二 provider 接入验证。
- 自动定时刷新 scheduler、刷新间隔配置和后台任务首页状态展示。
- 账号重命名和一键重新认证。
- 完整 backup/restore 流程、目标平台交叉编译和 race test 门禁。
- 更完整的 app-server 协议能力检测矩阵；MVP 只要求方法不可用时返回明确错误。

## 15. 主要风险与验收

### 15.1 主要风险

| 风险 | 决策 |
| --- | --- |
| app-server 协议变化 | 启动时检测能力；不使用网页抓取回退 |
| localhost 跨站请求 | session、一次性 bootstrap、Host/Origin、CSRF、CSP |
| Docker 误暴露到局域网 | post-MVP 实现时 compose 固定 `127.0.0.1` 端口发布，启动时校验 Host/Origin |
| SQLite 单 writer | 单连接、短事务、busy timeout、事务外执行 I/O |
| GORM 隐式行为 | DAO 隔离、禁用 AutoMigrate/Save、显式字段和事务 |
| Pure-Go GORM driver 兼容性 | 固定版本，运行 GORM/SQLite 集成测试和构建 smoke test |
| WAL 备份不完整 | 使用 backup API 或停止服务后 checkpoint |
| 数据库与凭证文件不一致 | 应用写锁、补偿恢复、启动校准和孤立目录清理 |
| token 进入 SQLite/WAL | schema、persistence model 和 DAO DTO 禁止凭证字段，测试扫描敏感字段 |
| refresh token 自动更新 | 切换前同步当前凭证，凭证写操作全局串行 |
| 切换时存在活动请求 | 明确提示中断风险并要求 reload |
| 本机明文凭证 | 私有目录、最小权限、脱敏和禁止入库 |
| 无法可靠 reload VS Code | 保留一次明确的人工操作 |
| 过早抽象 provider | MVP 只落地 Codex，contract 只覆盖当前已验证能力 |

### 15.2 MVP 完成标准

- 通过一个 Chi + GORM + SQLite 的无 CGO Go 二进制启动，无需安装 SQLite 或
  额外运行时。
- 服务仅通过本机 loopback 安全会话访问。
- SQLite migration、约束、WAL 和 quick check 验证通过。
- SQLite 中不出现 token、完整凭证或临时会话数据。
- 通用服务层不直接解析 Codex `auth.json` 或调用 app-server。
- 三个账号可保存、手动刷新额度，并在单账号刷新失败时继续展示其他账号。
- A → B → C → A 切换后，reload VS Code 可确认实际账号。
- refresh token 更新后仍可切回对应账号。
- 文件替换或数据库提交任一失败时，活动账号状态可以恢复。
- 前端、API、数据库、日志和 Git 工作区不出现凭证。
- localhost 跨站写请求被拒绝。
- `go vet`、普通测试、无 CGO 构建和本机三账号人工验收通过。

## 16. 参考资料

- [Codex Authentication](https://developers.openai.com/codex/auth)
- [Codex App Server](https://developers.openai.com/codex/app-server)
- [Codex IDE Extension](https://developers.openai.com/codex/ide)
- [Codex Environment Variables](https://developers.openai.com/codex/environment-variables)
- [Go Release History](https://go.dev/doc/devel/release)
- [Chi Documentation](https://github.com/go-chi/chi)
- [Validator Documentation](https://pkg.go.dev/github.com/go-playground/validator/v10)
- [GORM Documentation](https://gorm.io/docs/)
- [GORM SQLite](https://gorm.io/docs/connecting_to_the_database.html#SQLite)
- [Pure-Go GORM SQLite Driver](https://github.com/libtnb/sqlite)
- [SQLite Appropriate Uses](https://www.sqlite.org/whentouse.html)
- [SQLite WAL](https://www.sqlite.org/wal.html)
