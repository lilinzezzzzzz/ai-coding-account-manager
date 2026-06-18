# AI Coding Account Manager

本项目是本地运行的 AI coding 工具多账号管理器。当前面向 OpenAI Codex
账号，提供浏览器管理页面，用于导入账号凭据、查看账号和额度状态、刷新额度、
切换活动账号和删除非活动账号。

项目只在本机 loopback 地址提供服务，不设计为公网、局域网或多用户系统。SQLite
只保存账号元数据和 usage snapshot；Codex `auth.json` 保存在账号隔离凭据目录中，
不会写入数据库、浏览器持久化存储或 API 响应。

## 已实现能力

- Go HTTP server 和 Chi router 装配。
- `/api/health` 健康检查接口。
- 统一 API response envelope。
- Host 校验、同源 Origin 校验、strict JSON 请求解析和 body size 限制。
- SQLite/GORM 持久化底座、SQL migration、DAO、unit-of-work 和数据库启动校验。
- Provider contract、registry、fake provider 和 provider service facade。
- Codex app-server provider、runtime 发现、隔离凭据目录和活动账号原子切换。
- Codex 登录任务：通过临时 `CODEX_HOME` 登录并导入账号 `auth.json`。
- 账号核心 API：列表、登录添加、手动录入、导入 `auth.json`、刷新额度、设置套餐到期日、激活和删除。
- 原生 HTML/CSS/JavaScript 管理页面。
- Dockerfile、compose.yaml 和本地启动/停止脚本。

仍需人工按真实 Codex 账号环境补充发布前验收记录；自动化测试不能替代真实账号和
IDE reload 验收。

## 账号管理流程

### 登录添加

管理页面的“登录添加”按钮会直接创建 Codex browser 登录任务，不需要先输入邮箱。

流程：

1. 前端调用 `POST /api/providers/codex/login-tasks/create`，请求体为
   `{"mode":"browser"}`。
2. 后端创建临时 `CODEX_HOME`，并写入只影响本次登录任务的 Codex 配置。
3. 用户在浏览器中完成官方 Codex 登录。
4. 登录进程结束后，后端用 app-server 从临时 `CODEX_HOME` 读取真实账号 email 和套餐信息。
5. 后端根据真实 email 生成稳定 `accountId`，把临时目录里的 `auth.json` 复制到
   `.credentials/providers/codex/accounts/<account_id>/auth.json`。
6. 临时登录目录被清理，账号列表刷新。

注意：

- 如果浏览器当前已登录其他 OpenAI/Codex 账号，官方登录页可能直接使用该账号。
  需要在官方页面切换到目标账号，或使用无痕窗口。
- API 仍支持可选 `expectedEmail`。提供后，登录完成读取到的账号 email 必须匹配，
  否则任务失败且不会导入凭据。当前前端默认不传该字段。
- 登录添加不会替换当前活动 `CODEX_HOME/auth.json`；只有点击“激活”才会切换活动账号。

### 手动录入

“手动录入”只根据输入的 OpenAI 邮箱创建本地账号元数据，不会创建真实 Codex
凭据目录，也不能直接刷新真实额度。它适合先占位记录账号；真实使用前仍需要通过
登录添加或导入 `auth.json` 补齐凭据。

### 导入 auth.json

账号卡片里的“导入 auth.json”用于把已有 Codex 凭据内容导入指定账号的隔离目录。
后端会解析 JSON 并校验账号归属，避免把不匹配的凭据写到当前账号下。不要把
`auth.json` 内容提交到 Git、日志、issue 或聊天记录。

### 刷新额度

刷新单个账号时，后端会把该账号隔离目录中的 `auth.json` 复制到临时 Codex
运行目录，启动 app-server 读取账号和 rate limit 信息，再把刷新后的
`auth.json` 写回账号隔离目录。刷新不会修改当前活动账号。

### 激活账号

点击“激活”会把对应账号隔离目录里的 `auth.json` 原子替换到配置的活动
`codexHome` 目录。已经运行中的 VS Code/Codex 进程可能仍持有旧 token，通常需要在
VS Code 中执行 `Developer: Reload Window`。

### 删除账号

只能删除非活动账号。删除会移除数据库中的账号记录，并清理对应账号隔离凭据目录。

## Codex 登录任务

登录任务是短生命周期操作状态，默认存放在 `dataDir/login-tasks/` 下。任务完成、
取消、失败或超时后会清理临时目录。

登录任务使用隔离的临时 `CODEX_HOME`，并在该目录写入：

```toml
cli_auth_credentials_store = "file"
```

该配置只影响本次登录任务，目的是让 Codex 登录凭据落到临时目录里的
`auth.json`，便于导入账号隔离目录。它不会修改用户默认 `~/.codex/config.toml`。

登录任务状态：

```text
pending -> starting -> waiting_for_user -> verifying -> imported

pending|starting|waiting_for_user|verifying -> failed|cancelled|expired
```

## 项目结构

```text
cmd/ai-coding-account-manager/  进程入口、配置加载、启动和关闭编排
frontend/static/                前端静态资源源码
internal/config/                启动配置读取和校验
internal/httpserver/            http.Server 构造、timeout 和 header limit
internal/router/                Chi router、路由注册和 middleware 组装
internal/middleware/            跨请求 middleware
internal/httptransport/         HTTP API response envelope 和错误响应适配
internal/httpcontract/          HTTP API request/response contract 和 mapper
internal/controller/            HTTP controller
internal/provider/              provider-neutral contract 和 registry
internal/entity/                业务实体、值对象和稳定错误码
internal/service/               业务用例编排
internal/dao/                   持久化访问边界
internal/model/                 GORM 持久化模型
internal/infra/                 database、provider、credentials、app-server 实现
scripts/                        本地启动、停止脚本
config/                         配置示例和 fake provider 配置
```

## 后端分层约定

普通 HTTP 请求按以下路径流转：

```text
client
  -> httpserver/http.Server
  -> router/Chi
  -> middleware
  -> controller
  -> httpcontract
  -> service
  -> dao
  -> model
  -> infra/database
```

- `cmd/` 只负责进程启动、配置加载和依赖装配。
- `httpserver` 负责创建带 timeout 和 header limit 的 `http.Server`。
- `router` 负责注册 Chi route，并把请求交给 middleware 链。
- `middleware` 处理跨请求通用边界，例如 recovery、安全响应头和请求约束。
- `controller` 只处理 HTTP handler 编排：调用 `httpcontract` 解析 path 和 request
  DTO，调用 service，并通过 `httptransport` 写出 response。
- `httpcontract` 定义 HTTP API request/response DTO、path 参数解析和
  entity/service view 到 response 的 mapper。
- `httptransport` 负责 `/api/*` 的统一 response envelope 和错误码映射。
  API 业务错误不依赖 HTTP status 区分，统一通过 `error.code` 表达。
- `service` 负责业务用例编排、事务边界、DAO 调用顺序和外部 provider 协调。
  service 不直接读写 HTTP response，也不直接拼 GORM query。
- `dao` 负责数据库表访问、`model`/`entity` 转换和数据库错误映射；DAO 不保存
  业务规则。
- `entity` 集中定义跨层传递的稳定业务错误码和默认文案。
- `model` 只描述持久化表结构和 GORM tag，不作为 API DTO 或业务实体向外暴露。
- `infra` 提供 database、provider、credentials 和 app-server 等具体技术实现，
  由 `cmd` 在启动时创建并通过构造函数注入上层。

前端资源由 Go server 提供静态文件。Controller 不直接操作数据库、凭证文件或
app-server；service 不依赖 Chi、GORM model 或 `infra`；GORM 只出现在 `model`、
`dao` 和 `infra/database` 中。

## 本地运行

默认监听地址是 `127.0.0.1:43127`。

推荐使用脚本启动本地完整应用。当前 Go server 同时提供后端 API 和
`frontend/static` 前端页面，因此无需单独启动前端 dev server：

```bash
./scripts/start-local.sh
```

脚本默认使用项目根目录下的 `config/`、`.data/`、`.credentials/` 和 `.run/`。
其中 `.run/` 保存临时二进制和 PID 文件。需要结束本地服务时执行：

```bash
./scripts/stop-local.sh
```

启动日志会打印本地 URL。用浏览器打开该 URL 即可使用。

如果只需要验收前端交互，可用 fake provider 脚本启动：

```bash
./scripts/start-local-fake.sh
```

如果不需要 PID 文件和停止脚本，也可以直接使用底层命令：

```bash
go run ./cmd/ai-coding-account-manager
```

也可以显式指定配置文件：

```bash
go run ./cmd/ai-coding-account-manager --config config/app.fake.json
```

## Docker 本地启动

Docker 只把服务发布到宿主机 loopback 地址：

```bash
docker compose up --build
```

默认使用 named volume 保存 `/data` 和 `/codex`。如果要复用宿主机 Codex 登录态，
可以把 `compose.yaml` 中的 `codex-home` volume 改为只限本机使用的 bind mount，
例如 `~/.codex:/codex`。不要把真实 `auth.json` 放入 build context 或镜像 layer。

基础检查：

```bash
docker compose config
docker compose build
```

## 测试

常规测试：

```bash
go test ./...
```

如果本机 Go build cache 目录不可写，可以临时指定 `GOCACHE`：

```bash
GOCACHE=/tmp/ai-coding-account-manager-go-build go test ./...
```

发布前建议补充：

```bash
gofmt -l .
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build ./cmd/ai-coding-account-manager
docker compose config
docker compose build
```

真实 Codex 验收至少覆盖：

- 登录添加账号后不修改原活动 `CODEX_HOME/auth.json`。
- 新账号可以刷新额度并显示 email、plan 和 reset time。
- 激活账号后，VS Code reload 后 Codex 使用目标账号。
- 删除非活动账号后，对应 `.credentials` 子目录被清理。
- `git status` 不出现凭据文件。

## API 响应约定

API 统一返回 HTTP `200 OK`。业务成功或失败通过响应体判断：

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
    "code": "NOT_FOUND",
    "message": "接口不存在"
  }
}
```

客户端判断规则：

```text
error == null 表示业务成功
error != null 表示业务失败，按 error.code 分支处理
```

## 主要 API

```text
GET    /api/health
GET    /api/providers
GET    /api/accounts
POST   /api/providers/{providerId}/accounts/create
POST   /api/providers/{providerId}/accounts/import-current
POST   /api/providers/{providerId}/accounts/{accountId}/auth-json/import
POST   /api/providers/{providerId}/accounts/{accountId}/activate
POST   /api/providers/{providerId}/accounts/{accountId}/plan-expiration/update
POST   /api/providers/{providerId}/accounts/{accountId}/relogin
POST   /api/providers/{providerId}/accounts/{accountId}/refresh
DELETE /api/providers/{providerId}/accounts/{accountId}
POST   /api/providers/{providerId}/login-tasks/create
GET    /api/providers/{providerId}/login-tasks/{taskId}
POST   /api/providers/{providerId}/login-tasks/{taskId}/cancel
```

所有写请求都需要同源 Origin。POST 请求需要 `Content-Type: application/json`；
`auth-json/import` 还会使用单独的请求体大小限制。`relogin` 当前是保留路由，
返回稳定 `UNSUPPORTED`。

## 配置

当前配置读取顺序是：内置默认值、`config/app.json`。可参考
`config/app.example.json` 创建本地 `config/app.json`。除 `CODEX_HOME` 作为外部工具
约定的 fallback 外，不再使用 `AI_CODING_ACCOUNT_MANAGER_*` 环境变量作为正式配置入口。

| 配置字段 | 默认值 | 说明 |
| --- | --- | --- |
| `bindAddr` | `127.0.0.1:43127` | HTTP 监听地址，只允许 `127.0.0.1` 或 `localhost` |
| `dataDir` | `.data` | SQLite 和登录任务运行数据目录 |
| `credentialsDir` | `.credentials` | 账号隔离凭据目录 |
| `codexBin` | 空 | Codex CLI 可执行文件路径；空值时自动发现 |
| `codexHome` | `CODEX_HOME` 或 `~/.codex` | 活动 Codex 凭据目录；配置文件优先于 `CODEX_HOME` |
| `providerMode` | 空 | 设为 `fake` 时使用 fake provider |

`scripts/start-local.sh` 固定使用 `.run/server.pid` 和 `.run/ai-coding-account-manager`。

## 数据目录

默认本地目录：

```text
config/app.json                                      本地配置，默认被 Git 忽略
.data/state.db                                      SQLite 数据库
.data/login-tasks/<task_id>/                        登录任务临时目录
.credentials/providers/codex/accounts/<account_id>/ 账号隔离凭据目录
.run/                                               本地脚本运行目录
```

备份时建议同时保存 `.data/state.db` 和 `.credentials/`。只备份数据库无法恢复真实
Codex 凭据；只备份凭据目录会丢失账号元数据、活动状态和 usage snapshot。

可用以下方式做本地只读健康检查：

```bash
go test ./internal/infra/database
```

需要清理本地运行数据时，先停止服务，再按需删除 `.run/`、`.data/` 或
`.credentials/`。删除 `.credentials/` 会移除所有已导入账号凭据。

## 安全边界

- 不向局域网或公网暴露管理服务。
- 只接受配置端口上的 `127.0.0.1` 或 `localhost` Host。
- 写请求需要同源 Origin、`Content-Type: application/json`，并限制请求体大小。
- JSON 请求体使用 strict decode，拒绝未知字段、空 body、多个 JSON 值和超大 body。
- 不把 token 或完整 `auth.json` 写入数据库、项目文件、浏览器持久化存储或 API
  响应。
- `.credentials` 账号目录长期只保存 `auth.json`，刷新额度时的 Codex 运行态文件写入
  临时目录并在结束后清理。
- Codex 凭证文件读取、校验和原子替换逻辑封装在 `internal/infra/credentials`
  边界内。
- 日志和错误响应只能包含脱敏上下文，不能输出 token、refresh token 或完整
  `auth.json`。

## 维护约定

- 新增或修改 API 时，同步 `internal/router`、`internal/httpcontract`、测试和 README。
- 新增持久化字段必须通过 SQL migration，不能用 `AutoMigrate` 作为正式 schema
  变更路径。
- 所有 I/O、app-server 和文件操作必须在数据库事务外执行。
- 新增 provider 不得破坏通用账号主键、API envelope 和现有 Codex 账号隔离目录。
- 修改 Docker 暴露方式必须重新评估 localhost 安全边界。
- 修改凭证写入、导入、刷新或激活流程时，补充失败恢复测试和敏感字段泄漏检查。
- 不用代码阅读代替测试结论；报告验收时写明实际执行的命令和结果。
