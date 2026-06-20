# Account Operations 异步任务实施计划

## 1. 目标

将账号刷新和激活从同步 HTTP 请求改为持久化异步任务，解决页面刷新后前端状态丢失、同账号重复请求并发执行、服务退出后遗留任务状态不明确等问题。

本计划确定以下最终设计：

- 新增 `account_operations` 表，账号表不增加 operation 字段。
- `account_operations` 只保存账号逻辑引用，不建立到 `accounts` 的物理外键。
- `refresh` 和 `activate` 通过统一的后台 task runner 异步执行。
- task runner 固定为单 worker，异步 provider 操作串行执行。
- POST 只创建或返回 operation，不等待 provider 操作完成。
- task context 独立于 HTTP request context，但受应用关闭和配置超时控制。
- execution context 与 finalization context 分离；任务取消后仍必须能有界地持久化终态。
- 不恢复服务重启前的任务，不自动重试失败任务。
- 所有由应用生成的 `/api/**` JSON 响应统一返回 HTTP 200，业务结果通过 envelope `code` 表达。
- 新增 `OPERATION_INTERRUPTED` 错误码；超时通过 operation 的 `timed_out` 状态表达。
- 页面刷新后通过后端 operation 状态恢复按钮禁用和轮询。

## 2. 非目标

本次不实现：

- 服务重启后恢复或重新执行未完成任务。
- 自动重试 provider 调用。
- 用户主动取消 operation 的 API。
- 多进程或多实例 worker 协调。
- operation 优先级、延迟任务、定时任务或通用任务框架。
- 将登录任务整体迁移到 `account_operations`；登录过程仍使用现有内存任务模型，但最终向共享账号凭据目录导入时必须创建短生命周期 `login_import` operation。
- 将同步的导入、删除、套餐到期日更新改造成异步任务。导入和删除仍需创建短生命周期 operation 参与账号级互斥，但继续在原 HTTP 请求内执行。

如果未来允许多实例共享同一个数据库，必须重新评审 worker ownership、heartbeat 和 lease；本计划只保证当前本地单进程部署模型。

### 2.1 部署约束

当前实现仅支持本地单实例部署：

- 一个应用实例独占一个 SQLite `state.db`。
- SQLite 数据库、`credentialsDir` 和 `codexHome` 必须属于同一个实例边界。
- 不支持多个进程或容器共享同一个 SQLite 文件。
- 不支持将 SQLite 文件部署在 NFS 等网络文件系统上。
- `account_operations` 唯一索引提供数据库层账号互斥，但不代表具备完整的多实例任务协调能力。

如需支持多实例，必须切换到 PostgreSQL 等共享数据库，并补充 `owner_id`、heartbeat、
lease、原子任务认领和实例级优雅关闭语义。不得通过共享网络 SQLite 文件实现多实例。

## 3. 核心约束

### 3.1 状态与类型

Operation 类型：

```text
refresh
activate
import_auth
import_current
login_import
delete
```

Operation 状态：

```text
pending -> running -> succeeded
                   -> failed
                   -> cancelled
                   -> timed_out
```

状态语义：

| 状态 | 含义 |
|---|---|
| `pending` | operation 已持久化，等待 worker 获取执行槽位 |
| `running` | 单 worker 或同步 operation owner 已开始执行 provider/凭据操作 |
| `succeeded` | provider 操作和对应数据库结果均已成功持久化 |
| `failed` | provider、校验或持久化失败 |
| `cancelled` | 应用正常关闭时任务被取消并确认退出 |
| `timed_out` | 到达 operation deadline，且底层执行已确认退出 |

### 3.2 并发不变量

- 同一个 `(provider_id, account_id)` 同时最多存在一个 `pending` 或 `running` operation，包括异步 refresh/activate、同步 import_auth/import_current/delete，以及登录任务最终导入使用的 login_import。
- 同一个 provider 同时最多存在一个 `pending` 或 `running` 的 `activate` operation。
- 同一个 provider 同时最多存在一个活动凭据 reservation，owner 只能是 pending/running activate 或正在执行的 import-current。
- task runner 固定为单 worker；不同账号的异步 operation 可以同时处于 `pending`，但 provider 执行严格串行，任一时刻最多一个异步 operation 为 `running`。
- 同步 import_auth/import_current/delete 不进入 worker；它们可以与不同账号的异步 operation 并行，但必须受账号级 operation 互斥和 provider credential guard 约束。
- 同一账号的 `refresh` 与 `activate` 不能并发。
- `activate` 与 `import_current` 访问同一 provider 的活动凭据目录，必须通过进程内 provider credential guard 串行化；pending activate 从创建成功起持有该 reservation，直到进入终态。
- operation 的 provider 外部调用不得运行在数据库事务中。
- 只有持有对应 `operation_id` 的 worker 或同步 operation owner 可以推进该 operation 状态。
- 只有确认旧执行已退出，operation 才能进入允许后续任务创建的终态。

### 3.3 API 与异步结果分层

统一响应 envelope：

```json
{
  "data": {},
  "code": "SUCCESS",
  "message": "成功"
}
```

语义分层：

| 层级 | 语义 |
|---|---|
| HTTP status | `/api/**` 应用层 JSON 响应固定为 200 |
| envelope `code` | 当前 API 请求是否成功处理 |
| operation `status` | 后台任务生命周期 |
| operation `errorCode` | 后台任务失败原因 |

查询 operation 成功但任务执行失败时，顶层 `code` 仍为 `SUCCESS`：

```json
{
  "data": {
    "operationId": "op_example",
    "status": "failed",
    "errorCode": "OPERATION_INTERRUPTED"
  },
  "code": "SUCCESS",
  "message": "成功"
}
```

非 API 静态资源继续使用标准 HTTP 状态，例如缺失的 JavaScript 文件保持 404。HTTP Server 在进入应用路由前产生的协议错误不在“全局 HTTP 200”保证范围内。

## 4. 配置设计

三个配置文件增加：

```json
"operationTimeoutSeconds": 120
```

涉及文件：

- `config/app.example.json`
- `config/app.json`
- `config/app.fake.json`
- `internal/config/config.go`
- `internal/config/config_test.go`

配置规则：

- 对外字段使用 `operationTimeoutSeconds`，避免单位歧义。
- `config.Config` 内部使用 `time.Duration` 类型的 `OperationTimeout`。
- `fileConfig.OperationTimeoutSeconds` 使用 `*int`，必须区分“字段缺失”和“显式配置 0”。
- 未配置时默认 `120s`。
- 必须为正整数；建议允许范围为 `1..1800` 秒。
- 超出范围时应用启动失败，不能静默修正。
- 配置在启动时读取，运行期间不热更新。

创建 operation 时计算并持久化绝对 deadline：

```text
deadline_at = created_at + operationTimeout
```

operation timeout 覆盖排队等待和外部 execution，不覆盖独立的数据库 finalization。异步 worker 和同步 operation owner 都根据持久化的 `deadline_at` 创建 execution context，而不是再次读取当前配置；外部执行在 deadline 前返回后立即冻结 outcome，随后 finalization 可以在自己的 10 秒有界 context 内完成，即使此时已经越过 `deadline_at`。

## 5. 数据库迁移

新增：

```text
internal/infra/database/migrations/0003_account_operations.sql
```

目标 schema：

```sql
CREATE TABLE account_operations (
    operation_id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    operation_type TEXT NOT NULL
        CHECK (operation_type IN (
            'refresh',
            'activate',
            'import_auth',
            'import_current',
            'login_import',
            'delete'
        )),
    status TEXT NOT NULL
        CHECK (status IN (
            'pending',
            'running',
            'succeeded',
            'failed',
            'cancelled',
            'timed_out'
        )),
    error_code TEXT,
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    deadline_at INTEGER NOT NULL,
    finished_at INTEGER,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX account_operations_one_active_per_account
ON account_operations(provider_id, account_id)
WHERE status IN ('pending', 'running');

CREATE UNIQUE INDEX account_operations_one_activate_per_provider
ON account_operations(provider_id)
WHERE operation_type = 'activate'
  AND status IN ('pending', 'running');

CREATE INDEX account_operations_account_created
ON account_operations(provider_id, account_id, created_at DESC, operation_id DESC);
```

`account_operations.provider_id/account_id` 是逻辑外键，不建立到 `accounts` 的物理外键，也不使用 `ON DELETE CASCADE`。原因是 delete 成功后仍需保留 operation 历史并允许通过 operation endpoint 查询结果。

逻辑一致性规则：

- refresh、activate、已有账号的手工 import_auth 和 delete 必须在创建 operation 的同一短事务中确认账号存在。
- login_import 和 import_current 在可信 provider 数据已经确定 account ID 后允许先创建 operation，再在成功 finalization transaction 中 upsert 账号。
- 所有账号删除必须经过 delete operation；不得从其他 service 路径直接绕过互斥调用 `AccountDAO.Delete`。
- operation 查询不依赖账号仍然存在；终态 operation 对应已删除账号是合法历史记录。
- 启动时发现 `pending/running` operation 对应账号不存在时，同样标记为 `failed / OPERATION_INTERRUPTED`。

只有 `refresh` 和 `activate` 进入后台 task runner。`import_auth`、`import_current`、`login_import` 和 `delete` 不进入该 worker，但必须创建 `running` operation 占用账号级唯一约束。登录任务不整体迁移到 runner，只在最终写共享账号凭据前创建 `login_import` operation。同步操作通过 service 提供的闭合执行方法完成，不允许 controller 或调用方手工配对 Begin/Finish。

同时：

- 将 `internal/infra/database/database.go` 的 `supportedSchemaVersion` 从 2 更新为 3。
- 更新数据库初始化测试，断言表、字段、索引、迁移版本和重复执行 migration 的行为。
- migration 只做新增表和索引，不改写现有账号或 usage 数据，也不创建账号物理外键。
- 回滚旧版本二进制前必须备份数据库；旧二进制会因 schema 版本过高而拒绝启动，不能直接二进制回退。

### 5.1 历史记录保留

第一版保留 operation 历史，但必须有界。启动时以及服务运行期间每 24 小时执行一次节流清理，删除 `finished_at` 早于 30 天的终态记录：

```text
succeeded / failed / cancelled / timed_out
```

不得删除 `pending` 或 `running`。保留期和清理间隔第一版作为内部常量，不增加额外配置项。清理失败记录日志并等待下一次调度，不影响 worker 正常状态推进；启动 stale cleanup 失败则必须启动失败，不能带着无法确认的 active operation 对外服务。

## 6. Domain、Model 与 DAO

### 6.1 新增实体

新增 `internal/entity/account_operation.go`：

- `AccountOperationType`
- `AccountOperationStatus`
- `AccountOperation`
- `IsActive()` 和 `IsTerminal()` 等无副作用状态判断

`AccountOperation` 字段与数据库列一一对应，时间统一使用 Unix milliseconds。

### 6.2 新增持久化模型

新增 `internal/model/account_operation.go`，显式声明 GORM column 和 `TableName()`。

### 6.3 新增 DAO

新增 `internal/dao/account_operation.go`，并在 `internal/dao/unit_of_work.go` 的 `DAOs` 中加入：

```go
AccountOperations AccountOperationDAO
```

DAO 至少提供：

- 创建 pending 或 running operation，并根据 operation 类型执行对应的逻辑账号存在性校验。
- 按 `operation_id` 查询。
- 查询指定账号的 active operation。
- 批量查询账号列表对应的 active operations，禁止逐账号查询造成 N+1。
- 使用条件更新执行 `pending -> running`。
- 使用条件更新执行合法的 `pending/running -> terminal`。
- 启动时将遗留 active operations 标记为 `failed / OPERATION_INTERRUPTED`。
- 清理超过保留期的终态 operation。

所有状态推进必须同时校验 `operation_id` 和预期旧状态。例如：

```sql
UPDATE account_operations
SET status = 'running', started_at = ?, updated_at = ?
WHERE operation_id = ? AND status = 'pending';
```

`RowsAffected != 1` 必须作为状态冲突处理，不能忽略。

第一版允许的完整状态转换矩阵：

| 旧状态 | 新状态 | 场景 |
|---|---|---|
| `pending` | `running` | 单 worker 开始执行 |
| `pending` | `failed` | enqueue/内部初始化失败 |
| `pending` | `timed_out` | 排队期间超过 deadline |
| `pending` | `cancelled` | 正常 shutdown 时尚未开始 |
| `running` | `succeeded` | 外部执行及结果持久化成功 |
| `running` | `failed` | provider、校验或持久化失败 |
| `running` | `timed_out` | deadline 到达且旧执行已确认退出 |
| `running` | `cancelled` | shutdown 取消且旧执行已确认退出 |
| `pending/running` | `failed` | 启动时处理上次进程遗留状态 |

DAO 不提供允许任意 old/new status 的宽泛更新接口。每个推进方法必须固定或显式校验预期旧状态、目标状态和 operation ID。启动 stale cleanup 可以批量更新，但条件必须限制为 `pending/running`。

## 7. 错误码与 HTTP 响应

### 7.1 新增错误码

在 `internal/entity/error.go` 增加：

```text
OPERATION_INTERRUPTED
```

默认安全文案：

```text
操作因服务停止而中断
```

用途：

- 启动时发现上次进程遗留的 `pending/running` operation。
- 应用关闭导致后台任务取消。

`timed_out` 已由 operation status 明确表达，第一版不额外增加 timeout error code；其 `error_code` 保持空值。provider 自身失败则保存现有稳定错误码，例如 `UNAVAILABLE`、`CONFLICT` 或 `INTERNAL`。

### 7.2 `/api/**` 全部返回 HTTP 200

修改 `internal/httptransport/response.go`：

- 新增或明确 `WriteAPIError`，固定使用 `http.StatusOK` 输出 envelope。
- API controller adapter、API NotFound/MethodNotAllowed 和 API route middleware 统一使用 `WriteAPIError`。
- 保留支持显式 HTTP status 的非 API error writer；不能把共享 `WriteError` 无条件改成 200。

修改 `internal/middleware/request_security.go`：

- Origin、Content-Type 和 body size middleware 只挂在 API route，错误统一写 HTTP 200 envelope。
- Host middleware 当前挂在根 router：`/api` 请求写 HTTP 200 envelope，非 API 请求继续返回标准 HTTP 403，不能让静态资源安全错误变成 200。
- 错误码继续分别使用 `FORBIDDEN`、`VALIDATION_FAILED`、`PAYLOAD_TOO_LARGE`。

修改 router 错误处理测试，确保：

- `/api/missing` 返回 HTTP 200 + `NOT_FOUND`。
- API method 不支持返回 HTTP 200 + `METHOD_NOT_ALLOWED`。
- 非 API 静态资源缺失仍返回 404。
- 非法 Host 访问 `/api/**` 返回 HTTP 200 + `FORBIDDEN`，访问 `/` 或静态资源返回 HTTP 403。

## 8. Account Operation Service 与 Task Runner

新增 `internal/service/account_operation.go`，建议使用独立的 `AccountOperationService`，不要把 queue、worker 和关闭生命周期全部塞入现有 `AccountService`。

类型注释必须明确部署边界：

```go
// AccountOperationService 管理当前进程创建的账号操作和后台 worker。
//
// 数据库唯一约束用于账号级持久化互斥，但当前实现没有跨实例的 owner、lease 或
// heartbeat 协调，只支持本地单实例部署。
type AccountOperationService struct {
    // ...
}
```

依赖：

- `dao.UnitOfWork`
- `dao.DAOs`
- `*AccountService`
- `OperationTimeout`
- `Now func() time.Time`
- service-owned execution root context 和 cancel-cause function
- service-owned finalization root context 和 cancel function
- 有界 FIFO 任务队列，第一版容量为 64，作为内部常量
- 固定单 worker，不提供 worker 数量配置
- 按 provider 保存 owner token 的 `ProviderCredentialGuard`
- `sync.WaitGroup`

公开能力：

- `CreateRefresh(ctx, providerID, accountID)`
- `CreateActivate(ctx, providerID, accountID)`
- `ImportAccountAuthJSON(ctx, providerID, accountID, authJSON)`
- `ImportCurrentAccount(ctx, providerID)`
- `DeleteAccount(ctx, providerID, accountID)`
- 供 `LoginTaskService` 最终导入阶段调用的闭合 import 方法
- `Get(ctx, operationID)`
- `ListActiveForAccounts(ctx, accounts)` 或由 `AccountService.ListAccounts` 通过 DAO 批量装配
- `Close(ctx)`

`AccountOperationService` 依赖 `AccountService` 提供的无 operation 生命周期执行函数；反向不允许 `AccountService` 再依赖 `AccountOperationService`，避免循环依赖。Controller 对受管账号操作只调用 `AccountOperationService` 的闭合用例，不直接获得通用 Begin/Finish API。

### 8.1 双 context 生命周期与终态提交

Service 维护两个不同用途的根 context：

```text
finalizationRootCtx
└── executionRootCtx
    └── operation deadline context
```

- execution context 只用于 provider、子进程和文件系统副作用，可以因 operation deadline 或应用关闭被取消。
- finalization context 只用于账号/usage 结果与 operation 终态的数据库提交；它不继承 HTTP request 或 task deadline 的取消。
- shutdown 先取消 execution root，等待执行退出并完成终态提交，最后才取消 finalization root。
- 每次 finalization 从 finalization root 派生最多 10 秒的短 context；禁止把已取消的 request/task context 传给 GORM 终态更新。
- 使用 `context.WithCancelCause`/`context.WithDeadlineCause` 或等价的显式 cause 区分 shutdown 和 timeout，不能仅根据 provider 返回错误猜测终态。

终态分类顺序：

```text
shutdown cause -> cancelled / OPERATION_INTERRUPTED
deadline cause -> timed_out / null error_code
provider/校验错误 -> failed / 稳定业务错误码
无错误 -> succeeded
```

只有 provider 调用和子进程已经确认退出后才能分类并提交终态。执行返回时必须立即读取 context cause、停止 execution deadline timer 并冻结 outcome；已经在 deadline 前成功返回的执行不能因为后续 finalization 越过 `deadline_at` 被改判为 timed_out。finalization transaction 对 refresh 必须原子写入账号元数据、usage 和 operation 终态；activate/import/login-import/delete 等文件副作用无法与 SQLite 原子化，但对应数据库结果和 operation 终态仍必须在同一 transaction 中提交。

允许仅针对 `STORAGE_BUSY` 等明确瞬态数据库错误有界重试 finalization，绝不重试 provider 调用。重试耗尽后 operation 保持 active，单 worker 停止消费后续任务，service 对新创建请求返回 `UNAVAILABLE` 并记录脱敏错误；不得释放互斥或虚假报告成功。重启后由 stale cleanup 收敛。

### 8.2 创建异步 operation 与队列 admission

创建流程使用短事务：

1. 在 service admission mutex 下确认仍接收任务。
2. 快速查询同账号 active operation：相同类型 refresh/activate 直接返回已有 operation，视为幂等成功；不同类型返回 `OPERATION_IN_PROGRESS`。该查询只用于快速路径，不能代替唯一索引。
3. 预留一个 queue slot；队列满时返回 `UNAVAILABLE`，不得先创建 pending operation。
4. 预生成 operation ID。activate 使用该 ID 作为 owner token 预留 provider credential guard；失败时释放 queue slot 并返回 `OPERATION_IN_PROGRESS`。
5. 在短事务内再次校验账号存在并插入 `pending` operation，依赖部分唯一索引完成原子抢占。
6. 如果 insert 因并发同步 operation 或数据库唯一约束失败，重新查询 active operation：相同异步类型返回已有 operation，其他情况返回 `OPERATION_IN_PROGRESS`；同时释放未使用的 queue slot 和本次 provider reservation。
7. 提交事务后把 operation ID 写入已预留的 queue slot；admission mutex 保证其他异步 producer 不能抢占该 slot。

不能采用“先 SELECT 是否空闲，再 INSERT”的 check-then-act 作为互斥依据。允许先查询用于快速返回已有同类型异步 operation，但最终正确性必须由唯一索引和 insert 冲突后的重新查询保证。

进程在 transaction commit 与 enqueue 之间崩溃时，pending operation 会在下次启动被标记为 interrupted；本版本不恢复或重新入队。

### 8.3 单 Worker 执行

Worker 流程：

1. 从队列取得 operation ID。
2. 使用条件更新抢占 `pending -> running`；失败则不执行 provider。
3. 根据持久化的 `deadline_at` 从 execution root 创建带明确 timeout cause 的 task context。
4. 执行对应的 refresh 或 activate 内部用例。
5. 等 provider 调用和子进程确认退出。
6. 根据 context cause 和执行结果分类终态。
7. 使用独立的短 finalization context，在 transaction 中持久化账号/usage 结果与 operation 终态。
8. activate 进入终态后按 owner token 释放 provider credential guard。
9. 记录结构化日志，但不记录 auth JSON、token 或完整上游响应。

禁止使用请求的 `r.Context()` 执行后台 operation，也禁止使用无关闭边界的裸 `context.Background()`。

### 8.4 同步 operation 的闭合执行

`import_auth`、`import_current`、`login_import` 和 `delete` 不进入 worker，但必须通过 `AccountOperationService` 的闭合方法执行：

1. 在 admission 控制下登记同步 owner 和 WaitGroup，再在短事务内创建 `running` operation；Close 停止 admission 后不得开始新的同步 operation。
2. 唯一索引冲突时返回 `OPERATION_IN_PROGRESS`。
3. 在事务外执行凭据导入或账号删除用例。
4. 无论成功、失败、panic 或 HTTP/task context 取消，都使用独立 finalization context 推进 operation 终态。
5. 只有 finalization 成功后同步 API 才能报告成功；客户端已经断开时仍要完成后端终态收敛。
6. finalization 和 provider reservation release 完成后才从 WaitGroup 注销同步 owner。

同步 operation 不复用已有同类型 active operation，因为两次 import 可能携带不同凭据内容；任何 active operation 都返回 `OPERATION_IN_PROGRESS`。不能只在执行前查询 active operation，因为查询与外部凭据操作之间存在 check-then-act 竞态。

delete 顺序：

1. 创建 running delete operation，并校验账号非 active。
2. 在事务外删除账号隔离凭据；失败则保留账号并将 operation 标记为 failed。
3. 凭据删除成功后，在 transaction 中删除账号/usage 并把 operation 标记为 succeeded；逻辑外键保证 operation 历史保留。
4. 如果凭据删除成功但数据库 transaction 失败，账号可能保留但凭据已不存在；必须将 operation 标记为 failed 并记录脱敏日志，第一版不自动补偿或重试文件副作用。

### 8.5 登录导入与 Import Current 协调

单 worker 不覆盖 HTTP 同步方法和 `LoginTaskService` goroutine，因此所有共享凭据写入口必须显式参与协调。

登录任务：

1. 登录过程继续在隔离的临时 `CODEX_HOME` 中执行，不提前占用账号 operation。
2. 读取并校验出可信 account ID 后、写共享 `credentialsDir` 前，创建 running `login_import` operation。
3. 新账号允许在账号行不存在时创建该 operation；成功 finalization transaction 中 upsert 账号并推进 succeeded。
4. 与已有 active operation 冲突时，登录任务进入 failed，保存 `OPERATION_IN_PROGRESS`，不得覆盖现有凭据。
5. 外部凭据写成功但数据库 finalization 失败时保留 active operation 并暴露失败日志，不重复执行导入。

Import Current 需要先识别当前活动账号，识别前还不知道 account ID，因此增加按 provider、owner token 管理的进程内 `ProviderCredentialGuard`：

1. import-current 预生成 operation ID，并以该 ID 为 owner token 预留 provider guard；已有 pending/running activate 或另一个 import-current reservation 时返回 `OPERATION_IN_PROGRESS`。
2. 在 guard 内只读取并识别 provider 当前活动账号，不写共享账号凭据。
3. 得到 account ID 后创建 running `import_current` operation；允许目标账号尚不存在。
4. 在事务外把当前活动凭据导入目标账号隔离目录。
5. 在一个 finalization transaction 中 upsert 账号、`SetActive` 并推进 operation succeeded。
6. operation 进入终态后按 owner token 释放 provider guard。

需要把当前 provider 的 `ImportCurrentAccount` 拆成“读取/识别当前账号”和“导入当前账号凭据”两个阶段，确保能在二者之间获取账号级 operation。登录任务使用隔离目录，不访问全局活动凭据，因此只需要账号级 operation，不需要 provider guard。

统一资源获取顺序：

```text
provider credential guard（仅 activate/import_current）
-> account operation
-> provider/文件副作用
-> database finalization
-> release provider guard
```

provider guard 采用 owner token reservation，不使用要求同一 goroutine Unlock 的裸 mutex；释放时必须校验 owner，防止旧执行释放新 reservation。

### 8.6 Timeout

- 如果 worker 取到任务时已经超过 `deadline_at`，直接标记 `timed_out`，不调用 provider。
- 如果执行中 deadline 到达，先通过 context 取消底层调用并等待其返回。
- 只有确认旧执行已退出后才能写入 `timed_out`。
- 如果底层调用无法响应取消，operation 保持 `running`，不得提前允许第二个任务；进程重启后由启动清理标记为 interrupted。
- timeout 覆盖异步 operation 的 pending + 外部 execution，也覆盖同步 operation 的外部 execution；数据库 finalization 使用独立的 10 秒上限。同步 execution context 必须合并调用方取消、service execution-root shutdown cause 和持久化 deadline：deadline 到达保存 `timed_out`，共享的应用 shutdown cause 保存 `cancelled / OPERATION_INTERRUPTED`，仅普通调用方取消保存 `failed / UNAVAILABLE`。其终态提交始终不受调用方或 execution context 取消影响。

### 8.7 应用启动和关闭

启动：

1. migration 完成。
2. 将数据库中遗留的 `pending/running` operation 标记为：

   ```text
   status = failed
   error_code = OPERATION_INTERRUPTED
   finished_at = now
   ```

3. 清理超过保留期的终态历史，并启动每 24 小时一次的节流清理调度。
4. 初始化空的 provider credential guard；stale cleanup 后不得残留 reservation。
5. 启动单 worker。
6. 开始接收 HTTP 请求。

关闭：

1. HTTP server 停止接收新请求；即使 HTTP shutdown 报错，也继续执行后续 cleanup 并聚合错误。
2. `LoginTaskService.Close(ctx)` 停止接收登录任务，使用与 AccountOperationService 相同的应用 shutdown cause 取消并等待现有登录任务，保证已进入 login_import 的执行被分类为 `cancelled / OPERATION_INTERRUPTED`，且不会在 operation service 关闭后继续导入凭据。
3. `AccountOperationService.Close(ctx)` 在 admission mutex 下停止接收新 operation。
4. 将 queue 中尚未运行的 pending operation CAS 为 `cancelled / OPERATION_INTERRUPTED`，不要求单 worker 逐个执行。
5. 使用 shutdown cause 取消 execution root，等待 running provider/子进程确认退出并通过 finalization root 写入 cancelled。
6. 单 worker 和所有同步 operation owner 已确认退出后，停止历史清理调度，取消 finalization root，并清空 owner-matched provider reservations。
7. 关闭 providers。
8. 关闭数据库。

HTTP、login task、account operation 和 provider close 使用分阶段的有界 context，不能让 HTTP shutdown 耗尽后续全部预算。如果 account operation shutdown deadline 到期且仍有执行未退出，不得抢先把 operation 写成终态，也不得在活跃 goroutine 仍可能使用资源时并发关闭 provider/database；记录错误后让进程终止，下次启动通过 stale cleanup 处理剩余 active operation。

需要调整：

- `internal/app/app.go`
- `internal/app/services.go`
- `internal/app/server.go`
- `internal/router/router.go`
- `internal/router/api.go`
- `internal/service/login_task.go`
- `internal/provider/provider.go`
- `internal/infra/provider/codex/codex.go`
- `internal/infra/provider/fake/fake.go`

`services` 结构增加 `AccountOperation`，`LoginTaskService` 增加 `Close`，shutdown 接收完整 services，而不是只接收 `ProviderService`。`app.Run` 不能再无条件 defer 关闭数据库而忽略 worker 是否已经退出；数据库关闭必须服从上述顺序。

## 9. 现有 AccountService 重构

保留现有账号领域逻辑，但把 HTTP 同步入口与可供 operation worker 调用的内部执行逻辑分开：

- 将 `refreshOne` 拆为无数据库写入的 provider 执行/校验/标准化函数，以及由 `AccountOperationService` 调用的 finalization 持久化逻辑；不得继续吞掉 `persistFailedUsage` 错误。
- 将 activate 的 provider 调用与数据库持久化拆开；provider 调用在 transaction 外，`SetActive` 与 operation succeeded 在同一 finalization transaction 中。
- 删除 `activateMu`；账号级互斥由 `account_operations` 部分唯一索引负责，活动凭据目录互斥由 `ProviderCredentialGuard` 负责。
- refresh 的账号元数据、usage 和 operation 终态必须在同一短事务中完成，不使用“尽可能”语义。
- import auth、import current 和 delete 保持同步 HTTP 语义，但全部改由 `AccountOperationService` 闭合执行。
- 登录任务最终导入不再直接写共享凭据和 upsert 账号，改为调用 `AccountOperationService` 的登录导入用例。
- provider 的 import-current 能力拆成“读取当前账号”和“导入当前凭据”两个阶段。
- provider 外部副作用已经成功但业务结果 transaction 提交失败时，如果数据库仍可写，使用新的短 finalization transaction 将 operation 标记为 `failed` 并记录脱敏日志；如果数据库 finalization 本身持续不可用，则保持 active、停止 service admission，等待重启 stale cleanup。任何情况下都不得虚假返回 succeeded。

注意：activate 会先替换活动 `auth.json`，再更新数据库 active 标记。数据库失败时存在外部文件和数据库不一致风险，这是现有行为的延续。本次不引入自动补偿，但必须通过 operation failure 和日志暴露，不能吞掉错误。

## 10. HTTP API 契约

### 10.1 创建 refresh operation

```http
POST /api/providers/{providerId}/accounts/{accountId}/refresh
Content-Type: application/json
```

请求体：

```json
{}
```

成功创建或返回同账号已有 refresh operation：

```json
{
  "data": {
    "operationId": "op_example",
    "providerId": "codex",
    "accountId": "chatgpt-example",
    "type": "refresh",
    "status": "pending",
    "errorCode": null,
    "createdAt": 1781900000000,
    "startedAt": null,
    "deadlineAt": 1781900120000,
    "finishedAt": null,
    "updatedAt": 1781900000000
  },
  "code": "SUCCESS",
  "message": "成功"
}
```

### 10.2 创建 activate operation

```http
POST /api/providers/{providerId}/accounts/{accountId}/activate
Content-Type: application/json
```

返回结构与 refresh 相同，`type` 为 `activate`。

### 10.3 查询 operation

新增：

```http
GET /api/account-operations/{operationId}
```

返回 operation 当前状态。任务失败仍返回顶层 `SUCCESS`，失败原因位于 `data.errorCode`。
operation 查询不联表要求账号存在，因此 delete 成功后仍可查询对应终态历史。

### 10.4 账号列表

`GET /api/accounts` 的每个账号新增可选字段：

```json
{
  "activeOperation": {
    "operationId": "op_example",
    "type": "refresh",
    "status": "running",
    "deadlineAt": 1781900120000
  }
}
```

只装配 `pending/running` operation，不把完整历史塞入账号列表。终态通过 operation 查询 endpoint 获取。

### 10.5 冲突响应

同账号存在不同类型 operation 或同 provider 已有其他 activate：

```json
{
  "data": null,
  "code": "OPERATION_IN_PROGRESS",
  "message": "操作正在进行中"
}
```

HTTP status 固定为 200。

### 10.6 现有同步 API

手工 import auth、import current 和 delete 的 HTTP response shape 保持现有同步契约，不额外把内部 operation 暴露为异步 API。它们在返回前必须完成 operation finalization；冲突时返回 `OPERATION_IN_PROGRESS` envelope。登录任务的对外状态仍使用现有 login task contract，最终导入冲突或失败时把稳定错误码同步到 login task。

## 11. Controller 与 Contract

新增：

- `internal/controller/account_operation.go`
- `internal/httpcontract/account_operation.go`

调整：

- `AccountController` 注入 `AccountOperationService`；refresh/activate 改为创建异步 operation。
- import auth、import current 和 delete 改为调用 `AccountOperationService` 的闭合同步用例，不再直接调用裸 `AccountService` 执行函数。
- 新 controller 提供 `GetOperation`。
- `AccountResponse` 增加可空的 `activeOperation`。
- router 注册 operation 查询路由。
- `LoginTaskService` 注入受限的登录导入协调能力，最终导入阶段必须经过 `AccountOperationService`。

Controller 只负责 path 参数、调用 service 和输出 contract，不放 task 生命周期或状态迁移逻辑。

## 12. 前端改造

修改 `frontend/static/app.js`：

- 移除把 `refreshingAccountKeys` 作为真实状态源的设计。
- `activeOperation.status` 为 `pending/running` 时禁用冲突操作。
- refresh POST 成功后保存 operation ID，并立即显示“刷新中”。
- activate POST 成功后显示“激活中”。
- 轮询 `GET /api/account-operations/{operationId}`，建议间隔 1500ms。
- 页面初始化时，从 `/api/accounts` 的 `activeOperation` 恢复轮询。
- operation 进入终态后停止轮询并重新加载账号列表。
- `failed/cancelled/timed_out` 根据状态和 `errorCode` 显示安全文案。
- 新增 `OPERATION_INTERRUPTED` 前端文案。
- 后台轮询不得调用会把全页面设置为 loading 的现有 `loadData()` 路径；拆出静默刷新账号数据的方法，避免每 1.5 秒冻结全部按钮。
- 同一 operation ID 只能存在一个轮询循环，避免重复 timer。
- 页面离开时无需取消后端任务；浏览器只停止本地轮询。
- 如果 active operation 为 `import_auth/import_current/login_import/delete`，同样禁用该账号的冲突按钮；这些同步操作无需由发起页面启动后台轮询，但其他页面仍以服务端 active state 为准。

Provider 级 activate 互斥的 UI：只要同一 provider 任一账号存在 active activate operation，该 provider 下其他账号的激活按钮都应禁用；import-current 在账号识别前的 provider reservation 不一定能通过账号列表展示，前端不是互斥保证，后端 provider guard 和数据库约束才是最终防线。

## 13. 实施阶段与文件清单

### 阶段 A：配置、schema 和基础类型

- [ ] 更新三个 JSON 配置文件。
- [ ] 更新 `internal/config/config.go` 和配置测试。
- [ ] 新增 migration 0003 并更新 schema version。
- [ ] 新增 operation entity 和 model。
- [ ] 新增 DAO 并接入 `DAOs`。
- [ ] 落实逻辑外键规则，不创建到 accounts 的物理外键。
- [ ] 增加 migration、DAO 唯一约束、逻辑引用和完整状态转换测试。

完成条件：配置和数据库测试通过，尚不改变现有 HTTP 行为。

### 阶段 B：异步服务与生命周期

- [ ] 新增 `AccountOperationService`、固定单 worker runner、容量 64 的 FIFO queue 和 admission 控制。
- [ ] 实现创建、幂等返回、冲突、查询和状态推进。
- [ ] 实现 execution/finalization 双 context、终态分类和仅限数据库 finalization 的有界重试。
- [ ] 让同步 import auth/import current/delete 通过闭合短生命周期 operation 参与账号级互斥。
- [ ] 新增 owner-token provider credential guard，协调 activate 与 import-current。
- [ ] 让登录任务最终导入通过 login_import operation，登录任务整体状态模型保持不变。
- [ ] 实现 deadline、timeout、启动 stale cleanup 和启动/每 24 小时历史清理。
- [ ] 重构 AccountService 的 refresh/activate/import-current 内部执行路径，拆开 provider 执行与数据库 finalization。
- [ ] 为 LoginTaskService 增加 Close，并接入完整 app shutdown 顺序。
- [ ] 增加并发、取消、timeout、panic/失败清理测试。

完成条件：service 测试能证明同账号互斥、不同账号可同时 pending 但异步 provider 最大并发为 1、所有凭据写入口受控、finalization 不受执行 context 取消影响、关停可收敛。

### 阶段 C：API contract 与全局 HTTP 200

- [ ] 修改 refresh/activate controller 为创建 operation。
- [ ] 新增 operation 查询 endpoint。
- [ ] 扩展 account response 的 `activeOperation`。
- [ ] 保持 import auth/import current/delete 的同步响应契约，同时接入 operation 冲突处理。
- [ ] 将 `/api/**` 应用错误统一为 HTTP 200 envelope。
- [ ] 保留非 API 的标准 HTTP status，避免共享 error writer 把静态资源错误改成 200。
- [ ] 更新 middleware、router 和 controller 测试。

完成条件：API 测试覆盖创建、重复、冲突、查询、终态和错误 envelope。

### 阶段 D：前端状态恢复

- [ ] 使用服务端 `activeOperation` 驱动按钮状态。
- [ ] 实现单 operation 单轮询循环。
- [ ] 实现页面刷新后的轮询恢复。
- [ ] 显示 refresh/activate 成功、失败、中断和超时。
- [ ] 保留后端约束为最终防线，前端只负责体验。

完成条件：页面刷新后按钮仍保持正确状态，任务完成后自动恢复并展示最终数据。

### 阶段 E：全量验证与文档同步

- [ ] 更新 `README.md` 的配置和异步操作说明。
- [ ] 运行格式化、全量测试和 vet。
- [ ] 使用 fake provider 手工验证关键流程。
- [ ] 检查日志不包含 auth JSON、access token 或 refresh token。

## 14. 测试计划

### 14.1 配置测试

- 缺少 `operationTimeoutSeconds` 使用默认 120 秒。
- 正整数配置正确转换为 `time.Duration`。
- 0、负数和超过上限的值启动失败。
- 未知字段仍被拒绝。

### 14.2 Migration 与 DAO 测试

- 空数据库升级到 schema version 3。
- 现有 version 2 数据库无损升级。
- migration 重复打开只执行一次。
- 同账号第二个 active operation 被唯一索引拒绝。
- 同 provider 第二个 active activate 被拒绝。
- 不同账号 refresh 可以同时插入。
- 终态历史不阻止新 operation。
- 所有允许的 pending/running 状态转换必须匹配旧状态和 operation ID，非法转换被拒绝。
- schema 不包含到 accounts 的物理外键。
- 删除账号后终态 operation 历史仍然存在并可查询。
- refresh/activate/已有账号手工 import/delete 在账号不存在时不能创建 operation。
- login_import/import_current 可以为可信的新 account ID 创建逻辑引用 operation，并在成功 transaction 中 upsert 账号。
- 启动 stale cleanup 能处理 active operation 对应账号不存在的异常状态。
- 运行期间历史清理只删除超过 30 天的终态记录，不删除 pending/running。

### 14.3 Service 并发测试

- 同账号并发 refresh 只调用 provider 一次，两个请求得到同一 operation。
- refresh 进行中创建 activate 返回 `OPERATION_IN_PROGRESS`。
- 两个不同账号 refresh 可以同时创建并保持 pending/running，但 provider 最大并发调用数始终为 1，第二个在第一个终态后才开始。
- 三个异步 operation 按 FIFO admission 顺序由单 worker 执行。
- queue 已满时返回 `UNAVAILABLE` 且不创建无法入队的 pending operation。
- 同 provider 两个账号 activate 只能创建一个。
- pending/running activate 持有 provider reservation，import-current 返回 `OPERATION_IN_PROGRESS`。
- import-current 持有 provider reservation 时 activate 不能创建；owner 不匹配时不能释放 reservation。
- import auth 与同账号 refresh 不能并发。
- delete 与同账号 refresh/activate 不能并发。
- 登录任务最终导入与同账号 refresh/activate/delete 不能并发。
- 两个登录任务导入同一账号只能有一个获取 login_import operation。
- import-current 读取当前账号后再获取账号级 operation，目标为新账号时成功 upsert。
- refresh 与登录导入并发时不会以旧 refresh 结果覆盖新凭据。
- provider 成功后 operation 为 `succeeded`，账号和 usage 同步更新。
- provider 失败后 operation 为 `failed` 并保存稳定错误码。
- task deadline 已取消 execution context 后，独立 finalization context 仍能写入 `timed_out`。
- provider 在 operation deadline 前成功返回、finalization 随后越过 deadline 时仍按执行结果提交 succeeded，不误判 timed_out。
- HTTP request 取消后，同步 operation 仍能完成终态提交。
- finalization 遇到一次可重试存储错误时不重复调用 provider。
- finalization 重试耗尽时 operation 保持 active、单 worker 停止消费后续任务且新创建返回 `UNAVAILABLE`。
- deadline 前未开始的 pending task 直接 `timed_out`。
- 执行中 timeout 先等待 provider 返回，再进入 `timed_out`。
- 应用关闭时一个 running 和多个 pending 都标记为 `cancelled / OPERATION_INTERRUPTED`，pending 不需要单 worker 逐个执行。
- LoginTaskService shutdown 后不会继续访问 provider、operation service 或数据库；已经进入 login_import 的执行保存 `cancelled / OPERATION_INTERRUPTED`。
- 启动时遗留 pending/running 被标记 `failed / OPERATION_INTERRUPTED`，且不会重新入队。
- worker panic 被恢复、记录并将 operation 标记为 `failed / INTERNAL`；不得导致 worker 池永久减员。
- delete 凭据删除失败时账号保留且 operation failed；delete 成功后账号删除但 operation history 保留。
- delete 凭据删除成功、数据库 transaction 失败时 operation failed，并暴露账号可能缺少凭据的部分失败日志。

### 14.4 HTTP 测试

- refresh/activate POST 立即返回 operation，不返回最终账号结果。
- operation 查询能观察 pending、running 和所有终态。
- `/api/accounts` 返回 active operation。
- API validation、forbidden、not found、method not allowed、payload too large、operation conflict 均为 HTTP 200 且 code 正确。
- 非 API 静态资源 404 行为不变。
- 非法 Host 访问 API 返回 HTTP 200 + `FORBIDDEN`，访问非 API 路径返回 HTTP 403。
- 查询失败 operation 时 envelope code 为 `SUCCESS`，错误位于 operation `errorCode`。
- delete 成功后仍能通过 operation ID 查询终态历史。

### 14.5 前端手工验证

使用 fake provider：

1. 点击刷新，按钮显示“刷新中”。
2. operation 未完成时刷新整个页面，按钮仍显示“刷新中”。
3. 重复点击同一账号刷新，不产生第二个 provider 执行。
4. 刷新中不能激活同一账号。
5. 不同账号可以同时创建刷新任务，但界面能观察到一个 running、其余 pending，provider 实际串行执行。
6. 激活中，同 provider 其他账号不能再次激活。
7. 成功后账号 usage 和激活状态自动更新。
8. 模拟 provider 失败、timeout 和服务重启，前端显示对应终态。

## 15. 验证命令

按以下顺序执行：

```bash
gofmt -w <edited-go-files>
GOCACHE=/tmp/ai-coding-account-manager-go-build go test ./internal/config ./internal/infra/database ./internal/dao ./internal/service ./internal/router
GOCACHE=/tmp/ai-coding-account-manager-go-build go test -race ./internal/service ./internal/app ./internal/router
GOCACHE=/tmp/ai-coding-account-manager-go-build go test ./...
GOCACHE=/tmp/ai-coding-account-manager-go-build go vet ./...
./scripts/local.sh fake
```

fake 模式手工验证完成后执行：

```bash
./scripts/local.sh stop
```

不得根据代码阅读推断测试通过；实施完成报告必须列出实际运行的命令和结果。

## 16. 上线与回滚

上线前：

- 备份 `.data/state.db`。
- 确认没有真实 auth JSON、token 或 `.credentials/` 被加入版本控制。
- 先在 fake 配置运行 migration 和并发流程。

上线后：

- 首次启动自动执行 migration 0003。
- 检查 `schema_migrations` 最新版本为 3。
- 检查启动日志没有 migration、stale cleanup 或 worker 初始化错误。
- 检查 `/api/accounts` 和 operation 查询 envelope。

回滚：

- 仅回退二进制不可行，因为旧二进制支持的最高 schema version 为 2。
- 回滚必须停止服务并恢复上线前数据库备份，再启动旧版本。
- 不提供在线 downgrade migration。

## 17. 完成定义

只有同时满足以下条件，任务才算完成：

- schema version 3 和 operation DAO 已落地。
- account operation 使用逻辑外键，账号删除后终态历史仍可查询。
- refresh/activate POST 已异步化，HTTP 请求 context 不影响后台任务。
- task runner 固定单 worker，异步 provider 操作串行执行。
- 同账号和 provider activate 的持久化并发约束由数据库唯一索引保证，activate/import-current 的活动凭据访问由 owner-token provider guard 保证。
- execution context 与 finalization context 已分离，timeout/shutdown/request cancellation 后仍能有界地提交终态。
- 手工 import、import-current、delete 和登录任务最终导入全部参与账号级 operation 互斥。
- timeout 从配置读取并固化为 operation deadline。
- 服务重启不恢复任务，但会把遗留任务标记为 `OPERATION_INTERRUPTED`。
- `/api/**` 应用错误统一返回 HTTP 200 envelope，非 API 标准 HTTP status 不受影响。
- 页面刷新后可以恢复 active operation 状态和轮询。
- task runner 能在应用关闭时取消并收敛。
- targeted tests、并发 race tests、全量 tests 和 `go vet` 实际通过。
- fake provider 手工流程验证通过。
- README、配置示例和 API 行为说明已同步。
