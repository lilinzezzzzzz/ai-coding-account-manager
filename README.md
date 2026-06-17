# AI Coding Account Manager

本项目是本地运行的 AI coding 工具多账号管理器。MVP 面向 OpenAI Codex
账号，目标是在浏览器中查看账号和额度状态，并为账号新增、删除、切换和额度刷新
保留清晰的后端边界。

详细设计见 [TECHNICAL_DESIGN.md](TECHNICAL_DESIGN.md)，阶段拆解见
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)。

## 当前状态

项目处于 MVP 分阶段实现中，当前已包含：

- Go HTTP server 和 Chi router 装配。
- `/api/health` 健康检查接口。
- 统一 API response envelope。
- Host 校验、同源 Origin 校验、strict JSON 请求解析和 body size 限制。
- SQLite/GORM 持久化底座、SQL migration、DAO、unit-of-work 和数据库启动校验。
- Provider contract、registry、fake provider 和 provider service facade。
- 账号核心 API、Codex app-server provider、隔离凭据目录和原子切换。
- 原生 HTML/CSS/JavaScript 管理页面。
- Dockerfile、compose.yaml 和本地容器启动配置。

后续仍需补充更完整的真实 Codex 人工验收记录和发布打包流程。

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

前端独立于后端 HTTP server 部署或运行。Controller 不直接操作数据库、凭证文件或
app-server；service 不依赖 Chi、GORM model 或 `infra`；GORM 只出现在 `model`、
`dao` 和 `infra/database` 中。

## 本地运行

默认监听地址是 `127.0.0.1:43127`。

推荐使用脚本启动本地完整应用。当前 Go server 同时提供后端 API 和
`frontend/static` 前端页面，因此无需单独启动前端 dev server：

```bash
./scripts/start-local.sh
```

脚本会把临时二进制和 PID 文件写入 `.run/`。需要结束本地服务时执行：

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

可通过环境变量覆盖监听地址，但只允许 loopback 地址：

```bash
AI_CODING_ACCOUNT_MANAGER_BIND_ADDR=127.0.0.1:43127 go run ./cmd/ai-coding-account-manager
```

或通过脚本传入同样的环境变量：

```bash
AI_CODING_ACCOUNT_MANAGER_BIND_ADDR=127.0.0.1:43127 ./scripts/start-local.sh
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

```bash
go test ./...
```

如果本机 Go build cache 目录不可写，可以临时指定 `GOCACHE`：

```bash
GOCACHE=/tmp/ai-coding-account-manager-go-build go test ./...
```

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

## 配置

当前配置来自环境变量，由 `internal/config` 在启动时读取和校验。

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AI_CODING_ACCOUNT_MANAGER_BIND_ADDR` | `127.0.0.1:43127` | HTTP 监听地址，只允许 `127.0.0.1` 或 `localhost` |
| `AI_CODING_ACCOUNT_MANAGER_CONTAINER` | 空 | 设为 `1` 时允许容器内监听 `0.0.0.0`，compose 仍只发布到宿主机 `127.0.0.1` |
| `AI_CODING_ACCOUNT_MANAGER_DATA_DIR` | `${XDG_DATA_HOME:-~/.local/share}/ai-coding-account-manager` | SQLite、隔离凭据和运行数据目录 |
| `AI_CODING_ACCOUNT_MANAGER_CODEX_BIN` | `codex` | Codex CLI 可执行文件路径 |
| `AI_CODING_ACCOUNT_MANAGER_PROVIDER_MODE` | 空 | 设为 `fake` 时使用 fake provider |
| `CODEX_HOME` | `~/.codex` | 活动 Codex 凭据目录 |

当前不使用根目录 `configs/` 配置文件目录。

## 安全边界

- 不向局域网或公网暴露管理服务。
- 只接受配置端口上的 `127.0.0.1` 或 `localhost` Host。
- 写请求需要同源 Origin、`Content-Type: application/json`，并限制请求体不超过
  16 KiB。
- JSON 请求体使用 strict decode，拒绝未知字段、空 body、多个 JSON 值和超大 body。
- 不把 token 或完整 `auth.json` 写入数据库、项目文件、浏览器持久化存储或 API
  响应。
- Codex 凭证文件读取、校验和原子替换逻辑应封装在 infra/provider/credentials
  边界内。
