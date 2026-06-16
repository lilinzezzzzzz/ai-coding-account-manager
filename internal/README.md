# Internal Backend

本目录保存后端私有代码。项目使用 Controller、Service、Entity、Model、DAO、
Router、Middleware 结构，`cmd/` 只负责进程启动和依赖装配。

- `controller/`：HTTP controller、request DTO、response envelope 和错误映射。
- `entity/`：业务实体、值对象和稳定错误码。
- `model/`：与数据库表对应的 GORM 持久化模型。
- `service/`：业务用例编排、事务边界和 DAO 协调。
- `dao/`：面向表的数据库操作、model/entity 转换和稳定错误映射。
- `middleware/`：请求日志、recovery、安全头、Host、session 和 CSRF。
- `router/`：`http.Server`、Chi router、路由注册和 middleware 组装。
- `infra/`：database、Codex、app-server 和凭证文件实现。
- `security/`：bootstrap、session、CSRF 和请求边界。
- `config/`：配置读取和启动参数校验。

前端 `frontend/` 是独立的 View。Controller 不直接操作数据库、凭证文件或
app-server；service 不依赖 Chi、GORM model 或 `infra`；GORM 只出现在 `model`、
`dao` 和 `infra/database` 中。
