# Infrastructure

`infra` 保存数据库、文件系统、子进程和外部工具等技术实现，不保存业务规则。

- `database/`：SQLite、GORM 初始化、migration 和 transaction manager。
- `provider/codex/`：Codex provider 的具体实现。
- `appserver/`：Codex app-server 子进程和 JSON-RPC client。
- `credentials/`：`auth.json` 的目录权限、校验和原子替换。

`service`、`controller`、`entity`、`model` 和 `dao` 不得依赖具体 provider、
app-server 或 credentials 实现。`cmd` 创建基础设施依赖并通过
构造函数注入。

持久化调用链固定为：`service -> dao -> model -> GORM/SQLite`。DAO 不包含业务
规则，service 不直接使用持久化 model 或拼装 GORM query。
