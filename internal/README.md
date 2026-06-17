# Internal Backend

本目录保存后端私有代码。项目使用 Controller、Service、Entity、Model、DAO、
Router、Middleware 结构，`cmd/` 只负责进程启动和依赖装配。

- `controller/`：HTTP controller、request DTO 和业务用例调用。
- `entity/`：业务实体、值对象和稳定错误码。
- `model/`：与数据库表对应的 GORM 持久化模型。
- `service/`：业务用例编排、事务边界和 DAO 协调。
- `dao/`：面向表的数据库操作、model/entity 转换和稳定错误映射。
- `middleware/`：请求日志、recovery、安全头、Host、session 和 CSRF。
- `router/`：`http.Server`、Chi router、路由注册和 middleware 组装。
- `transport/httpapi/`：HTTP API response envelope 和自定义错误码映射。
- `infra/`：database、Codex、app-server 和凭证文件实现。
- `security/`：bootstrap、session、CSRF 和请求边界。
- `config/`：配置读取和启动参数校验。

## 请求流程

普通 HTTP 请求按以下路径流转：

```text
client
  -> router/http.Server
  -> middleware
  -> controller
  -> service
  -> dao
  -> model
  -> infra/database
```

- `router` 负责创建 `http.Server`、注册 Chi route，并把请求交给 middleware 链。
- `middleware` 处理跨请求的通用边界，例如安全响应头、Host、session、CSRF 和
  recovery。
- `controller` 只处理 HTTP 传输层：解析 path、query、body，做请求 DTO 校验，
  调用 service，并返回业务 error 或通过 `transport/httpapi` 写出成功 response。
- `transport/httpapi` 负责 `/api/*` 的统一 response envelope 和自定义错误码映射。
  `httpapi.Handle` 统一接收 controller 返回的 error 并写出错误 envelope。API
  业务错误不依赖 HTTP status 区分，统一通过 `error.code` 表达。
- `service` 负责业务用例编排、事务边界、DAO 调用顺序和外部 provider 协调。
  service 不直接读写 HTTP response，也不直接拼 GORM query。
- `dao` 负责数据库表访问、`model`/`entity` 转换和数据库错误映射；DAO 不保存
  业务规则。
- `entity` 用 `ErrorCode` 集中定义跨层传递的稳定业务错误码和默认文案，
  `transport/httpapi` 负责把业务错误写成错误 envelope。
- `model` 只描述持久化表结构和 GORM tag，不作为 API DTO 或业务实体向外暴露。
- `infra` 提供数据库、凭证文件、app-server 和 provider 等具体技术实现，由
  `cmd` 在启动时创建并通过构造函数注入上层。

前端独立于后端 HTTP server 部署或运行。Controller 不直接操作数据库、凭证文件或
app-server；service 不依赖 Chi、GORM model 或 `infra`；GORM 只出现在 `model`、
`dao` 和 `infra/database` 中。
