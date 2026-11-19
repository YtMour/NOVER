# Changelog

本文件记录 NOVER 对外可见的版本变化。版本号与 git tag（建议 `v0.2.0`）对齐。

## [0.3.0] - 2026-11-19

主题：可运营闭环（商品资料 v2、退款售后、成员与账户安全、租户审计）。

### 商品与交付

- 商品新增结构化资料：`highlights`（≤8 条）、`license_terms`、`requirements`、`support_period`。
- 新增商品级更新策略 `LIFETIME / TILL_DATE / CURRENT_ONLY` 与截止时间；签发兑换码时快照、兑换时复制到权益，交付列表与下载票据按策略过滤后续版本（起始版本始终可见）。
- 新增独立公开详情接口 `GET /api/v1/products/{id}` 与空间版 `GET /api/v1/spaces/{slug}/products/{id}`。
- 用户端详情页展示亮点、授权、技术要求、支持期与更新策略；交付页展示权益更新范围与策略文案。

### 运营与售后

- 新增手工退款：兑换码标记 `REFUNDED`、权益立即 `REVOKED`，退款原因写入审计；重复退款返回 409。
- 新增成员管理：OWNER/ADMIN 可创建成员（ADMIN/EDITOR/FULFILLMENT/ANALYST）、调整角色与状态；暂停成员会撤销其会话；不能修改自己或 OWNER。
- 新增账户安全：修改密码（保留当前会话、撤销其他设备）、会话列表、撤销单个会话、撤销其他设备。
- 新增租户审计日志 API 与控制台视图：支持 action/resource_type 筛选、分页加载更多、元数据展开。
- 新增 `member.manage` 与 `audit.read` 权限点。

### 控制台与用户端

- 控制台新增“账户与安全”“审计日志”视图与 hash 深链；商品编辑新增资料与更新策略区块；兑换记录支持退款；成员视图支持创建、改角色、暂停/启用。
- 用户端商品详情页接入独立详情接口，新增结构化资料区块；交付页展示更新策略与权益范围。

### 工程与文档

- 新增迁移 `0003_product_profile_and_accounts.sql`（商品资料列、码/权益策略快照列、审计查询索引、演示资料）。
- 登录限流调整为 30 次/分钟/IP；新增冒烟脚本 `operator-capabilities-smoke.ps1` 并纳入 `run-all.ps1`；PowerShell JSON 请求体统一 UTF-8，修复中文在 Windows PowerShell 5.1 下的乱码。
- 更新 API、架构、产品定义、路线图、README、AGENTS、开发/运维文档与 `docs/audit-2026-11-19-v0.3.0.md`。

### 验证

- `go vet ./...`、`go test ./...` 通过（45 个测试函数）。
- `web/`、`console/` 构建通过；`scripts/verify.ps1` 全量通过。
- 三套业务冒烟全部 `pass: true`：版本权益、租户交付、运营能力（资料/退款/成员/改密/会话/审计）。
- 迁移 `0003` 在历史库成功应用；回归冒烟未破坏既有兑换/交付/额度行为。

### 升级说明

- 升级无需手工迁移：启动自动应用 `0003`。历史权益 `update_policy` 为 NULL 时按 `LIFETIME` 处理。
- 新增字段均向后兼容；旧前端缺少字段时隐藏新 UI。
- 新增错误码：`code_not_refundable`、`invalid_member`、`member_exists`、`member_not_found`、`member_self_forbidden`、`member_owner_protected`、`invalid_password`、`invalid_current_password`、`session_not_found`、`invalid_audit_filter`。

### 未包含

- MFA/Passkey、邀请邮件、密码重置、RLS、跨租户负向矩阵全覆盖。
- 外部平台退款自动同步、S3/扫描、备份恢复自动化、指标告警。
- 商品目录卡片消费新资料字段、多语言、ReactBits 组件。

## [0.2.0] - 2026-11-19

主题：工程收口、运营可用性、开发者与智能体体验。

### 工程与可靠性

- 数据库改为**版本化迁移**：`server/internal/db/migrations/NNNN_*.sql`，启动时按版本顺序在事务内应用，`schema_migrations` 记录校验和；历史幂等 schema 安装自动识别为基线，已发布迁移被修改时拒绝启动。
- 新增维护命令 `cmd/maintenance`：清理过期/撤销会话、过期下载票据、标记过期兑换码与权益、删除无引用超过 24 小时的私有文件与空目录；默认 dry-run，`-apply` 才执行，支持 `-json`。
- Compose 新增 `ops` profile 的 `maintenance` 服务，API 增加健康检查与构建元数据参数。
- 新增结构化请求日志（`request_id`/`method`/`path`/`route`/`status`/`duration_ms`/`ip`）与响应头 `X-Request-Id`；健康检查不写访问日志。
- 新增公开 `GET /api/v1/meta`，返回版本、提交、构建时间与环境。
- 限流响应统一返回 `429` + `Retry-After` 秒数；兑换、登录、上传、票据、加入申请均生效。
- `GET /api/v1/cp-7f3c/activity` 支持 `limit`/`offset` 分页并返回 `total`/`has_more`，非法参数返回 `invalid_pagination`。
- `GET /api/v1/delivery` 新增 `max_downloads`、`downloads_used`，方便前端展示下载额度。
- 申请审核状态机收紧：对已通过/已拒绝的申请重复执行相同审核动作返回 `409 application_status_conflict`。

### 用户体验（UX）

- 双端可访问性基线：跳过导航链接、主内容焦点管理、全局 `:focus-visible` 样式（浅/深色）、`aria-pressed`/`aria-live`/`aria-hidden` 语义、`StatusMessage` 播报策略。
- 运营控制台视图与 URL hash 深链同步（`#overview`、`#products`、`#codes`、`#applications`、`#members`、`#settings`），支持前进/后退、直接访问与按视图更新页面标题。
- 用户端公开目录新增名称/简介搜索、结果计数、清空与空结果状态。
- 交付页展示下载额度（已用/总量/剩余）并在耗尽时禁用下载；有效期展示“还剩 N 天/已过期”；行级下载进行态；SHA-256 复制支持失败提示与读屏播报。
- 双端 API 客户端解析 `Retry-After` 并在 429 时提示“请 N 秒后重试”；网络异常转换为友好中文错误。

### 开发者体验（DX）

- 新增 `.github/workflows/ci.yml`：提交卫生检查、Go vet/test/build、双端构建 matrix、Docker Compose 端到端冒烟。
- 新增 `scripts/verify.ps1` / `scripts/verify.sh` 一键验证与 `scripts/qa/run-all.ps1` 冒烟汇总。
- 两套业务冒烟脚本改为跨平台（Windows PowerShell 5.1 / PowerShell 7 / Linux）：multipart 上传改用 curl，不再依赖 `Invoke-RestMethod -Form`；新增下载额度断言。
- 新增 `.gitattributes`、`.editorconfig`、PR 模板，统一 LF、缩进与 PowerShell UTF-8 BOM 约定。
- 新增[开发指南](docs/development.md)、[运维手册](docs/operations.md)、[API 说明](docs/api.md)，更新 README、架构与路线图。

### 智能体体验（AX）

- 新增根目录 [AGENTS.md](AGENTS.md)：仓库地图、端口、命令、关键不变量、新增 API/页面/迁移的标准步骤、错误码约定与测试要求。
- 新增构建信息端点与结构化访问日志，便于智能体在运行时确认版本、关联请求与定位问题。
- 冒烟与验证脚本提供稳定、非交互、可机器解析（JSON/退出码）的反馈。

### 验证

- `go vet ./...`、`go test ./...` 通过（41 个测试函数，新增迁移、维护、限流、分页、请求中间件与构建信息测试）。
- `web/`、`console/` 的 `npm run build` 通过（TypeScript + Vite）。
- `scripts/verify.ps1` 全量 8/8 通过；两套业务冒烟均 `pass: true`，含下载额度 `10 / 0 → 1` 断言。
- 迁移：历史库自动基线并应用 `0002`；全新库应用 `0001` + `0002` 并生成演示数据。
- 维护命令对运行库 dry-run：过期会话 52、过期权益 1、空目录 19（未执行删除）。
- 限流：连续失败登录第 9 次起返回 `429` 且 `Retry-After: 60`。

### 升级说明

- 升级到本版本无需手工迁移：首次启动自动识别历史 schema 并记录基线，然后应用新迁移。
- 新增字段与端点均为向后兼容；旧前端可继续工作（交付额度字段缺失时隐藏）。
- 构建元数据可通过 `APP_VERSION`、`GIT_COMMIT`、`BUILD_TIME` 注入，缺失时显示 `dev/unknown`。
- `server/internal/db/schema.sql` 已移除，结构入口改为 `server/internal/db/migrations/`。

### 未包含

- RLS、MFA/TOTP/Passkey、邀请设密、密码重置、全设备会话管理。
- S3 对象存储、上传病毒扫描、归档检查、对象存储生命周期。
- 自动化备份/恢复与租户级数据导出。
- 指标、追踪、告警、Redis 共享限流与多实例部署验证。
- 退款联动、买家自助查询、商品结构化资料（亮点/授权/支持期）、搜索筛选与多语言。
