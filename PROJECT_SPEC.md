# 古生物修复制备台项目说明

## 项目目标

本项目为古生物标本制备组提供一个可运行的后端基线，用于管理标本登记、修复方案审批、制备进度和研究成果放行。系统关注的是标本状态的真实流转、同一方案下的连续记录以及放行前的完整性约束。

## 使用方

- 登记员：录入标本目录号、野外编号、基质类型和临时保护信息。
- 制备负责人（`prep_lead`）：为标本拟定修复目标和方法、作出审批决定，并把待处理方案领用或转派给具备资格的制备人员。
- 制备人员（`preparator`）：被领用为方案当前责任人后，按阶段提交基质去除比例、操作说明和记录人。
- 标本管理员：核对已完成制备的标本并办理研究借阅放行。

## 核心实体

- 标本：保存目录号、野外标签、分类提示、基质类型、夹克备注、状态和版本号。
- 修复方案：绑定一份标本，保存目标、方法、黏结材料、目标暴露比例、审批信息和状态。
- 人员：保存人员标识、姓名、角色（`prep_lead` 或 `preparator`）和在册状态；人员标识唯一。
- 方案责任：以不可覆盖的台账行记录方案每次领用（`claim`）和转派（`transfer`），每行状态为 `active` 或 `released`；任一时刻每个方案最多一行 `active`。
- 进度记录：属于一个修复方案，按序号保存阶段名称、完成比例、说明和记录人。
- 放行记录：绑定已稳定的标本，保存借阅目的、接收方和批准人。

## 状态与规则

标本状态依次经过 `registered`、`planned`、`preparing`、`stabilized` 和 `available`。同一目录号不能重复登记。一个标本同时最多存在一个未结束的修复方案。

修复方案状态依次经过 `draft`、`approved`、`active` 和 `completed`，草稿也可以进入 `cancelled`。只有草稿能够审批；只有已审批且未结束、尚未领用的方案能够领用，已领用的方案只能通过转派更换责任人。

一名制备人员同时最多承担 `3` 个未结束方案（领用上限）。只有在册且角色为 `preparator` 的人员可以成为责任人；只有 `prep_lead` 可以执行领用和转派。转派在同一仓储事务中把旧责任行置为 `released` 并追加新的 `active` 行，旧行计数释放、新行计数占用同时生效。方案进入 `completed` 或 `cancelled` 后不能转派，完成时释放容量但保留最后责任人。

进度比例必须位于零到一百之间，且新的比例不得低于上一条记录。只有方案当前责任人（`active` 责任行对应的制备人员）可以追加进度，记录人必须与当前责任人一致。比例达到一百时形成最后一条记录，方案进入 `completed`，标本进入 `stabilized`。完成后的方案不能再写入进度。

放行只允许发生在标本进入 `stabilized` 且不存在未结束方案时。放行成功后标本进入 `available`，并永久保留一条放行记录。

## 业务链路

### 登记标本

登记员通过 `POST /specimens` 提交目录号、野外标签、分类提示、基质类型和夹克备注。服务校验必填字段和目录号唯一性，然后创建 `registered` 标本。`GET /specimens/{id}` 可读取完整快照。

### 审批修复方案

负责人通过 `POST /specimens/{id}/plans` 创建草稿方案，再通过 `POST /plans/{id}/approval` 写入批准人和审批时间。每一步都检查标本状态和未结束方案约束，审批成功后标本进入 `preparing`。

### 领用与转派方案

制备负责人通过 `POST /plans/{id}/assignments` 提交责任制备人员和分配人，把已审批待处理方案领用给在册的 `preparator`。负责人通过 `POST /plans/{id}/assignment/transfer` 把方案转派给另一名合格制备人员：旧责任行与新责任行在同一仓储事务内一放一立，中间不存在无主或双主状态。`GET /plans/{id}/assignments` 返回责任台账，`GET /workbench` 在工作台列出每个未结束方案及其当前责任人，`GET /staff/{id}` 返回该人员当前责任数与领用上限。

重复领用返回 `409 plan_already_claimed`；并发领用由仓储互斥保证恰好一方成功、其余 `409`；转派给无资格人员（未知人员或非制备角色）返回 `409 preparator_not_qualified`，非负责人操作返回 `409 actor_not_prep_lead`；目标满员返回 `409 assignment_limit_reached`；未知方案返回 `404 plan_not_found`。方案完成或取消后转派返回 `409 plan_not_transferable`。

### 记录制备进度

制备人员通过 `POST /plans/{id}/progress` 追加阶段记录，且记录人必须是方案当前责任人。第一条有效进度会把方案从 `approved` 推进到 `active`；比例达到一百时方案和标本在同一仓储事务中进入终态，防止只更新其中一方。

### 办理研究放行

标本管理员通过 `POST /specimens/{id}/release` 提交借阅目的、接收方和批准人。服务再次检查标本状态和方案闭合状态，成功后写入不可覆盖的放行记录并把标本置为 `available`。

## 模块边界

- `cmd/paleo-prep-station` 负责进程参数、监听和关闭。
- `cmd/workflowcheck` 负责通过公共 HTTP 路由执行独立的本地链路检查。
- `internal/httpapi` 负责传输层解码、输出和错误状态映射。
- `internal/station` 负责加载对象、执行业务条件并提交原子变更。
- `internal/domain` 负责实体构造、状态转换和不变量。
- `internal/repository` 负责互斥保护和事务副本提交。
- `internal/contracts` 负责公共请求和稳定错误类型。

## 接口约定

- `POST /specimens` 成功返回 `201` 和标本对象。
- `POST /specimens/{id}/plans` 成功返回 `201` 和方案对象。
- `POST /plans/{id}/approval` 成功返回 `200` 和更新后的方案。
- `POST /plans/{id}/assignments` 成功返回 `201` 和当前责任行；`GET /plans/{id}/assignments` 成功返回 `200`。
- `POST /plans/{id}/assignment/transfer` 成功返回 `201` 和新的当前责任行。
- `GET /workbench`、`GET /staff`、`GET /staff/{id}` 成功返回 `200`；`POST /staff` 成功返回 `201`。
- `POST /plans/{id}/progress` 成功返回 `201` 和进度记录。
- `POST /specimens/{id}/release` 成功返回 `201` 和放行记录。
- 输入不合法返回 `400`，对象不存在返回 `404`，状态冲突、重复领用、超上限或无资格操作返回 `409`。

## 验证计划

构建命令为 `go build ./...`。当前基线不生成自动化测试文件，使用五条生产级 HTTP 链路检查覆盖主要成功路径和关键冲突边界，其中 `assign-and-transfer` 覆盖重复领用、并发抢单、无资格转派、上限、完成后禁转派和责任人进度权限，并在竞态检测下重复执行。后续任务阶段再补充红绿验证测试，并保留这些检查作为端到端烟雾路径。
