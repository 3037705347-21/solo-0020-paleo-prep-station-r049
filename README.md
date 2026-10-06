# 古生物修复制备台

古生物修复制备台是一个使用 Go 编写的后端基线，用于记录化石标本、审批修复方案、领用与转派制备任务、回填制备进度，并在全部工序完成后办理研究借阅放行。

该服务只使用进程内仓储，便于在本机重复启动和检查。它不依赖数据库、远端接口或其他在线资源。

## 目录

- `cmd/paleo-prep-station`：HTTP 服务入口和监听参数。
- `cmd/workflowcheck`：通过真实 HTTP 处理器执行五条业务链路的本地验证命令。
- `internal/httpapi`：路由、请求解码、状态码映射和 JSON 输出。
- `internal/station`：跨领域对象的编排、事务边界和业务流程。
- `internal/domain`：标本、方案、人员、方案责任、进度和放行记录的状态与不变量。
- `internal/repository`：互斥保护的内存仓储和原子提交。
- `internal/contracts`：稳定错误模型和公共请求结构。

## 构建与运行

```bash
go build ./...
go run ./cmd/paleo-prep-station --addr 127.0.0.1:18133
```

服务启动后，`GET /healthz` 返回就绪状态。

## 主要接口

- `GET /staff`（人员角色、当前领用数量和领用上限）
- `GET /specimens` 与 `POST /specimens`
- `GET /specimens/{id}`
- `POST /specimens/{id}/plans`
- `GET /plans/{id}`（含当前责任与完整责任轨迹）
- `POST /plans/{id}/approval`
- `POST /plans/{id}/claim`（制备人员自取，或制备负责人指派）
- `POST /plans/{id}/reassign`（当前责任人转派）
- `POST /plans/{id}/progress`（仅当前责任人）
- `POST /specimens/{id}/release`

## 领用与转派规则

- 只有已审批且尚未领用的方案可以领用；草稿、已完成方案不能领用。
- 责任人必须具备制备人员角色；每位制备人员同时承担的未结束方案最多三个。
- 制备人员用 `{"actor_id": "..."}` 自取；制备负责人可用 `{"actor_id": "<负责人>", "assignee_id": "<制备人员>", "assigned_by": "<负责人>"}` 指派。
- 重复领用、并发抢单在同一互斥事务中决出唯一结果，其余请求返回 `409`。
- 只有当前责任人能追加进度；转派请求 `{"from_id": "...", "to_id": "..."}` 在同一提交中释放旧责任并建立新责任。
- 完成后的方案保留最后责任人，不能再转派。

## 本地验证

每条业务链路都可以独立执行：

```bash
go run ./cmd/workflowcheck --workflow register-specimen
go run ./cmd/workflowcheck --workflow approve-prep-plan
go run ./cmd/workflowcheck --workflow log-preparation
go run ./cmd/workflowcheck --workflow release-for-study
go run ./cmd/workflowcheck --workflow assign-prep-plan
```

这些检查会启动临时 HTTP 处理器并调用真实公共路由；`assign-prep-plan` 额外用并发请求验证抢单只有一个赢家。

## 环境变量

项目不需要环境变量，也没有外部服务依赖。

## 测试边界

本基线按要求暂不生成单元测试、测试夹具或端到端测试文件。后续任务阶段会补充红绿验证测试，届时再替换或扩展当前的生产级链路检查。
