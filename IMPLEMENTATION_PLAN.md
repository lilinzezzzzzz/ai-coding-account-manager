# AI Coding 多账号管理器实现计划

## 1. 执行原则

本计划落地 `TECHNICAL_DESIGN.md` 中的当前决策：Go、Chi、GORM、SQLite、
前后端同仓、单二进制或 Docker 本地启动。实现顺序采用后端优先，前端在 API
和 fake provider 稳定后补齐。

阶段推进规则：

- 每个阶段必须有可运行或可验证的交付物。
- 不把 token、完整 `auth.json`、OAuth URL、session 或 CSRF token 写入数据库、
  日志、前端存储、Git 工作区或 Docker image layer。
- Handler 只处理 HTTP 边界，业务编排在 service，数据库访问在 repository。
- 数据库 schema 以 SQL migration 为唯一事实来源，禁止用 `AutoMigrate` 作为
  正式启动流程。
- 所有 I/O、app-server、OAuth 和文件操作必须在数据库事务外执行。
- 每阶段先做最小验证，再进入下一阶段；不能用代码阅读代替测试结果。

## 2. 阶段总览

| 阶段 | 主题 | 主要产出 | 进入下一阶段条件 |
| --- | --- | --- | --- |
| 0 | 项目骨架 | Go module、目录、基础命令 | `go test ./...` 可运行 |
| 1 | HTTP 与静态资源 | `http.Server`、Chi、嵌入页面、health | 原生启动可访问 `/api/health` |
| 2 | 配置与安全基础 | config、bootstrap、session、CSRF、strict JSON | 安全边界测试通过 |
| 3 | SQLite/GORM | migration、repository、unit-of-work | 数据库集成测试通过 |
| 4 | Provider 基础 | contract、registry、fake provider | fake provider API 可用 |
| 5 | 账号核心 API | account service、import、refresh、activate、delete | API 集成测试通过 |
| 6 | Codex 集成 | app-server client、Codex provider、凭证目录 | 只读协议验证通过 |
| 7 | 前端页面 | 账号列表、操作流、错误展示 | fake provider UI 验收通过 |
| 8 | Docker 与发布 | Dockerfile、compose、README、smoke test | Docker 本地启动通过 |
| 9 | 完整验收 | race、备份恢复、三账号人工验收 | 完成标准全部满足 |

## 3. Phase 0：项目骨架

目标：

- 建立 Go module 和后续代码目录。
- 固定基础依赖引入方式，避免后续大范围重排。

交付物：

- `go.mod`、`go.sum`
- `cmd/ai-coding-account-manager/main.go`
- `internal/config`
- `internal/server`
- `internal/web/static`
- `tests` 或按 Go 习惯使用包内 `_test.go`

实施要点：

- 先创建最小可编译 main。
- 只添加当前阶段真实使用的依赖。
- 依赖新增使用 `go get` 或 `go mod tidy`，不手写 `go.sum`。

验证：

```bash
go test ./...
go vet ./...
```

阻断条件：

- `go test ./...` 无法在空业务骨架上运行。
- module path、目录结构或包名需要反复调整。

## 4. Phase 1：HTTP Server 与静态资源

目标：

- 跑通本地 HTTP 服务、Chi router、优雅关闭和静态资源嵌入。
- 先放一个极薄的前端占位页，避免后期才发现 embed 或路由问题。

交付物：

- `internal/server` 中的 `http.Server` 构造和 shutdown。
- Chi router 注册。
- `/api/health`。
- `/` 返回嵌入的 `index.html`，页面只显示服务已启动。
- 基础安全响应头 middleware。

实施要点：

- 显式构造 `http.Server`，不使用 `http.ListenAndServe` 快捷启动。
- 原生运行默认绑定 `127.0.0.1:43127`。
- Docker 相关监听差异暂不实现，只保留 config 扩展点。
- `log/slog` 不记录 query string、Cookie 或 token。

验证：

```bash
go test ./...
go vet ./...
go run ./cmd/ai-coding-account-manager --no-open
```

阻断条件：

- 不能稳定关闭 HTTP server。
- 静态资源无法从二进制内提供。
- health endpoint 无法被 `httptest` 覆盖。

## 5. Phase 2：配置与本地安全边界

目标：

- 实现本地 Web 安全底座，为后续 API 提供统一入口。

交付物：

- typed `Config`。
- bootstrap token 生成和一次性兑换。
- session Cookie。
- CSRF token。
- Host、Origin、Content-Type 和 body size middleware。
- strict JSON decode helper。
- 统一 success/error envelope。

实施要点：

- `AI_CODING_ACCOUNT_MANAGER_BIND_ADDR` 原生运行只接受 `127.0.0.1`。
- Docker 镜像入口使用 `0.0.0.0` 的能力必须单独测试，宿主机只发布 loopback。
- JSON helper 使用 `DisallowUnknownFields`，拒绝多个 JSON 值和尾随内容。
- validation error 不返回内部 struct、环境变量或堆栈。

验证：

```bash
go test ./internal/server ./internal/middleware ./internal/security
go test ./...
```

测试重点：

- bootstrap token 只能兑换一次。
- 未登录访问受保护 API 返回 `401`。
- 非法 Host、Origin、CSRF 返回 `403`。
- body 过大返回 `413`。
- unknown JSON field 返回稳定 `400`。

阻断条件：

- handler 需要重复实现安全校验。
- session、CSRF 或 bootstrap token 出现在日志或响应之外的位置。

## 6. Phase 3：SQLite、GORM 与 Repository

目标：

- 建立持久化层，先不接真实 Codex。

交付物：

- `internal/database`。
- `internal/database/migrations/0001_initial.sql`。
- GORM 初始化和 PRAGMA 校验。
- repository persistence model。
- domain model。
- unit-of-work。
- 数据库 quick check 和 schema version 检查。

实施要点：

- migration SQL 创建 `accounts`、`usage_snapshots`、`schema_migrations`。
- 使用 `github.com/libtnb/sqlite` 和 GORM。
- SQLite 连接池限制为单连接。
- repository 每次调用从 `db.WithContext(ctx)` 派生。
- 禁止 `Save`、global update、隐式 association save/preload。
- repository 只返回 domain model 或 domain error，不泄露 GORM model。

验证：

```bash
go test ./internal/database ./internal/repository ./internal/service
go test ./...
```

测试重点：

- 空库初始化。
- migration 重复执行。
- schema 过新拒绝启动。
- foreign key 和 cascade delete。
- 单 provider 只能有一个 active account。
- duplicate key、foreign key violation、record not found 映射为稳定错误。
- context 取消和 busy timeout。

阻断条件：

- `AutoMigrate` 进入正式启动路径。
- 事务中执行 app-server、OAuth 或文件 I/O。
- token 字段进入 schema 或 repository DTO。

## 7. Phase 4：Provider Contract 与 Fake Provider

目标：

- 先用 fake provider 打通 provider-neutral 设计和 API 行为。

交付物：

- `internal/provider` contract。
- provider registry。
- fake provider。
- provider capability。
- service 对 provider failure isolation 的处理。

实施要点：

- 通用层只依赖 `(provider_id, account_id)` 和 capability。
- capability 不支持的操作返回稳定 `UNSUPPORTED`。
- fake provider 支持构造多种账号状态：`ready`、`refreshing`、
  `auth_expired`、`rate_limit_reached`、`unavailable`、`unsupported`。

验证：

```bash
go test ./internal/provider ./internal/service ./internal/handler
go test ./...
```

阻断条件：

- Codex 私有字段泄露到通用 service。
- fake provider 无法覆盖前端需要展示的状态。

## 8. Phase 5：账号核心 API

目标：

- 在 fake provider 和 repository 基础上实现完整 HTTP API。

交付物：

- `/api/session`
- `/api/providers`
- `/api/accounts`
- `/api/providers/{providerId}/accounts/import-current`
- `/api/providers/{providerId}/accounts/{accountId}/activate`
- `/api/providers/{providerId}/accounts/{accountId}/rename`
- `/api/providers/{providerId}/accounts/{accountId}/relogin`
- `/api/providers/{providerId}/accounts/{accountId}`
- `/api/providers/{providerId}/login-tasks`
- `/api/login-tasks/{id}`
- `/api/usage/refresh`

实施要点：

- API 先对 fake provider 完整可用。
- handler 只解析 path/query/body 并调用 service。
- service 负责事务边界、凭证锁和错误映射。
- 登录任务只存在内存中，有过期、取消和关闭清理。

验证：

```bash
go test ./internal/handler ./internal/service ./internal/server
go test ./...
```

测试重点：

- 未认证、非法 Origin、非法 CSRF。
- import、rename、refresh、activate、delete 成功路径。
- 活动账号不能删除。
- 单账号 refresh 失败不影响其他账号。
- 并发 activate 返回 `409 OPERATION_IN_PROGRESS`。
- API 响应不包含 token、完整凭证或 app-server 原始响应。

阻断条件：

- API envelope 不稳定。
- handler 直接操作 DB、文件系统或 app-server。

## 9. Phase 6：Codex Provider 与 App Server

目标：

- 接入真实 Codex，只先完成可控、可验证的只读能力，再实现登录和切换。

交付物：

- `internal/appserver` JSON-RPC client。
- `internal/provider/codex`。
- Codex CLI 探测。
- `CODEX_HOME` 隔离启动。
- `account/read`。
- `account/rateLimits/read`。
- 凭证解析和目录权限校验。
- app-server 子进程生命周期管理。

实施顺序：

1. app-server process wrapper。
2. JSON-RPC request/response correlation。
3. `account/read` 只读验证。
4. `account/rateLimits/read` 只读验证。
5. import current account。
6. login task。
7. refresh usage。
8. activate account。
9. delete account data。

实施要点：

- 子进程使用 `exec.CommandContext`，同时消费 stdout 和 stderr。
- JSON-RPC pending map 必须加锁，关闭时拒绝全部 pending。
- app-server 原始报文默认不落日志。
- 切换账号使用全局凭证写锁和补偿恢复。
- 只替换活动 `$CODEX_HOME/auth.json`，不覆盖其他 Codex 状态。

验证：

```bash
go test ./internal/appserver ./internal/provider/codex ./internal/service
go test ./...
```

本机只读验证：

```bash
go run ./cmd/ai-coding-account-manager --no-open
```

人工验证：

- 当前账号可 import。
- rate limit 可刷新。
- 切换后页面提示 reload。
- VS Code reload 后账号符合预期。

阻断条件：

- token、OAuth URL 或 app-server 原始响应进入日志。
- 文件替换失败或 DB 提交失败无法恢复。
- app-server 子进程或 goroutine 泄漏。

## 10. Phase 7：前端页面

目标：

- 基于稳定 API 和 fake provider 完成前端交互。

交付物：

- `internal/web/static/index.html`
- `internal/web/static/app.css`
- `internal/web/static/app.js`
- 账号列表和 provider 分组。
- usage snapshot 展示。
- import current、login、refresh、activate、rename、delete 操作。
- login task 轮询。
- reload VS Code 指引。
- 稳定错误展示。

实施要点：

- 不引入 React/Vue 或前端构建链。
- 不使用 `localStorage`、`sessionStorage` 或 IndexedDB 保存账号数据。
- 所有写请求带 CSRF token。
- 不使用 inline script 或第三方 CDN。
- 前端状态优先基于 fake provider 开发，再接 Codex provider。

验证：

```bash
go test ./internal/server ./internal/handler
go test ./...
```

人工验证：

- fake provider 的各状态展示正确。
- 按钮按 capability 显示或隐藏。
- 删除和切换有二次确认。
- 刷新页面后 OAuth 任务状态行为符合预期。

阻断条件：

- 前端持久化敏感数据。
- 错误展示包含异常栈、环境变量或内部结构。

## 11. Phase 8：Docker 与本地发布

目标：

- 支持无本机 Go 工具链的 Docker 本地启动，同时保持 loopback 安全边界。

交付物：

- `Dockerfile`
- `compose.yaml`
- `.dockerignore`
- README 启动说明。
- Docker smoke test 脚本或手动步骤。

实施要点：

- 镜像内运行同一个 Go 二进制。
- 不拆独立前端服务或反向代理。
- `compose.yaml` 只发布 `127.0.0.1:43127:43127`。
- 容器内可使用 `AI_CODING_ACCOUNT_MANAGER_BIND_ADDR=0.0.0.0`。
- 数据目录和 `CODEX_HOME` 使用 bind mount 或 named volume。
- build context 排除真实凭证、数据库、WAL、备份和临时目录。

验证：

```bash
docker compose config
docker compose build
docker compose up --build
```

检查项：

- 宿主机只监听 `127.0.0.1:43127`。
- `/api/health` 可访问。
- 容器日志不包含 token、OAuth URL、Cookie 或 CSRF token。
- 没有真实凭证进入 image layer。

阻断条件：

- `compose.yaml` 暴露到 `0.0.0.0` 或使用 host network。
- Docker 只能通过传 token 环境变量工作。

## 12. Phase 9：完整验收

目标：

- 收敛测试、并发、安全、备份恢复和真实账号验收。

自动化验证：

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
CGO_ENABLED=0 go build ./cmd/ai-coding-account-manager
docker compose config
docker compose build
```

补充验证：

- 二进制启动 smoke test。
- Docker 启动 smoke test。
- SQLite backup/restore 验证。
- `PRAGMA quick_check` 失败路径。
- 日志和 API 响应敏感字段扫描。
- shutdown 后无遗留 app-server 子进程。

人工验收：

1. 启动本地服务并打开管理页面。
2. 保存当前账号 A。
3. 新增账号 B、C。
4. 页面同时显示 A、B、C 的额度与重置时间。
5. 切换到 B，页面提示 reload。
6. 在 VS Code 执行 `Developer: Reload Window`。
7. Codex `/status` 显示 B 的账号和额度。
8. A -> B -> C -> A 切换均可恢复。
9. refresh token 更新后仍可切回对应账号。
10. 删除非活动账号后对应私有凭证目录被清理。
11. `git status` 不出现任何凭证文件。
12. 从其他网页构造 localhost 写请求时被拒绝。

完成条件：

- `TECHNICAL_DESIGN.md` 第 15.2 节完成标准全部满足。
- README 覆盖原生启动、Docker 启动、数据目录、备份恢复和安全注意事项。
- 关键风险均有自动化或人工验收记录。

## 13. 暂不做事项

- 不实现 Codex 以外真实 provider。
- 不实现自动账号轮换。
- 不实现 VS Code 自动 reload。
- 不实现公网、局域网或多用户访问。
- 不引入前端框架或构建链。
- 不把 SQLite 放在网络文件系统。

## 14. 后续维护规则

- 新增 API 时先更新 `TECHNICAL_DESIGN.md` 和本计划，再实现。
- 新增持久化字段必须通过 migration，且说明兼容性、回滚和备份影响。
- 新增 provider 不得修改通用账号主键和 API envelope。
- 修改 Docker 暴露方式必须重新评估 localhost 安全边界。
- 修改凭证写入流程必须补充失败恢复测试和敏感字段扫描。
