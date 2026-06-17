# AI Coding 多账号管理器剩余实施计划

## 1. 当前状态

项目 MVP 的主要实现阶段已经完成：项目骨架、HTTP server、配置与本地安全边界、
SQLite/GORM 持久化、provider contract、账号核心 API、Codex 集成、前端页面、
Docker 本地启动和脚本启动能力均已落地。

本计划不再重复记录已完成阶段的实现细节。已实现能力以代码、
`README.md` 和 `TECHNICAL_DESIGN.md` 为准；本文只跟踪当前剩余验收事项和后续维护
规则。

## 2. 执行原则

- 每项剩余工作必须有可运行或可验证的交付物。
- 不把 token、完整 `auth.json` 或 OAuth URL 写入数据库、日志、前端存储、Git
  工作区或 Docker image layer。
- Controller 只处理 HTTP 边界，业务编排在 service，GORM 表模型在 model，
  数据访问、model/entity 转换和持久化错误映射在 DAO。
- 数据库 schema 以 SQL migration 为唯一事实来源，禁止用 `AutoMigrate` 作为正式
  启动流程。
- 所有 I/O、app-server、OAuth 和文件操作必须在数据库事务外执行。
- 不能用代码阅读代替测试结果；验收结论必须来自实际命令输出或人工记录。

## 3. Phase 9：完整验收

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

- 脚本启动和停止 smoke test：`./scripts/start-local-fake.sh`、
  `./scripts/stop-local.sh`。
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
- README 覆盖原生启动、脚本启动、Docker 启动、数据目录、备份恢复和安全注意事项。
- 关键风险均有自动化或人工验收记录。

## 4. 暂不做事项

- 不实现 Codex 以外真实 provider。
- 不实现自动账号轮换。
- 不实现 VS Code 自动 reload。
- 不实现公网、局域网或多用户访问。
- 不引入前端框架或构建链。
- 不把 SQLite 放在网络文件系统。

## 5. 后续维护规则

- 新增 API 时先更新 `TECHNICAL_DESIGN.md` 和本计划，再实现。
- 新增持久化字段必须通过 migration，且说明兼容性、回滚和备份影响。
- 新增 provider 不得修改通用账号主键和 API envelope。
- 修改 Docker 暴露方式必须重新评估 localhost 安全边界。
- 修改凭证写入流程必须补充失败恢复测试和敏感字段扫描。
