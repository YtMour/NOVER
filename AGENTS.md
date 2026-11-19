# AGENTS.md — NOVER 智能体指南

> 面向在本仓库工作的 AI 智能体（AX）与贡献者。开始前先读本文件；完整人类文档见 [README.md](README.md) 与 [docs/](docs/)。
> 目标：安全、可验证地改动，不破坏交付、权益与租户隔离的不变量。

## 1. 项目定位

NOVER（知屿）是开源数字产品交付平台：**外部平台成交 + NOVER 负责兑换、权益与私有文件交付**。
它不是商城——没有站内支付、购物车或商户分账。默认单实例自托管，已具备基础多租户（租户来自服务端会话）。
开源边界：只提交平台代码与合成数据；实际售卖的程序/素材/安装包、密钥、`.env`、真实订单与用户数据**不得入库**。

## 2. 目录与技术栈

| 路径 | 内容 | 技术 |
| --- | --- | --- |
| `web/` | 用户端：公开目录、商品详情、兑换、交付、加入申请 | React 19 + TS + Vite + Tailwind 4 + TanStack Query + Zod |
| `console/` | 独立运营控制台（单独构建与端口） | 同上，独立 API 客户端 |
| `server/` | Go API、迁移与维护命令 | Go 1.24 + chi + pgx + PostgreSQL 17 |
| `server/cmd/api` | API 入口，启动时自动执行迁移并引导默认账号 | |
| `server/cmd/maintenance` | 数据保留 / 孤儿文件清理（默认 dry-run） | |
| `scripts/qa/` | 业务冒烟 + `run-all.ps1` 汇总 | 跨平台 PowerShell（5.1 / 7 / Linux） |
| `scripts/verify.ps1`、`scripts/verify.sh` | 本地一键验证（与 CI 同源步骤） | |
| `docs/` | 产品、架构、API、开发、运维文档 | |
| `.github/workflows/ci.yml` | 后端、前端 matrix、端到端冒烟 | |
| `docker-compose.yml` | 本地与单机部署 | |

## 3. 地址与端口

| 服务 | 地址 |
| --- | --- |
| 用户端 | http://localhost:45173 |
| 控制台 | http://localhost:46173/ops-7f3c |
| API | http://localhost:48080 |
| PostgreSQL | 127.0.0.1:5433（仅回环） |

运营路径由 `OPERATOR_WEB_PATH` / `OPERATOR_API_PATH` 配置（默认 `/ops-7f3c`、`cp-7f3c`）。健康检查：`GET /healthz`（存活）、`GET /readyz`（数据库就绪）、`GET /api/v1/meta`（版本元数据）。

## 4. 常用命令

Windows PowerShell 5.1 可直接运行本仓库脚本，不需要 pwsh 7：

- `.ps1` 被执行策略拦截时：`powershell.exe -NoProfile -ExecutionPolicy Bypass -File <脚本>`，或在当前会话执行 `Set-ExecutionPolicy -Scope Process Bypass`。
- 使用 `npm.cmd` 而不是 `npm.ps1`（后者可能被策略拦截）。

### 一键验证

```powershell
.\scripts\verify.ps1              # 工具检查 + go vet/test + web/console 构建 + 冒烟（需服务已启动）
.\scripts\verify.ps1 -SkipE2E     # 快速检查，无需 Docker
```

```bash
./scripts/verify.sh [--skip-e2e]  # POSIX 等价版本
```

### 业务冒烟（需 `.env` 与 `docker compose up -d --build`）

```powershell
.\scripts\qa\run-all.ps1                     # 三套顺序执行，任一失败返回非零
.\scripts\qa\version-entitlement-smoke.ps1   # 版本权益、草稿隐藏、下载额度
.\scripts\qa\tenant-delivery-smoke.ps1       # 租户隔离、商户目录与交付
.\scripts\qa\operator-capabilities-smoke.ps1 # 商品资料/更新策略/退款/成员/改密/会话/审计
```

脚本使用合成租户/商品并在结束时清理；`-KeepFixture` 保留夹具便于排查。

### 本地开发

```powershell
Copy-Item .env.example .env
docker compose up --build                  # 整套：db + api + web + console
docker compose up -d --build api           # 只重建后端
docker compose up -d db                    # 前后端分离开发时只起数据库
cd server; go run ./cmd/api
cd web; npm.cmd ci; npm.cmd run dev
cd console; npm.cmd ci; npm.cmd run dev
```

### 维护与迁移

```bash
docker compose --profile ops run --rm maintenance          # dry-run：只统计
docker compose --profile ops run --rm maintenance -apply   # 执行清理
docker compose --profile ops run --rm maintenance -json    # 机器可读输出
```

迁移位于 `server/internal/db/migrations/NNNN_snake_case.sql`，API 启动时按版本顺序自动应用并校验 checksum。

## 5. 关键不变量（改动前必读）

1. **租户来自服务端**：运营查询使用会话中的 `tenant_id`；绝不接受前端传入的 `tenant_id` / 租户 slug 作为授权依据。
2. **已发布文件不可变**：`PUBLISHED` 版本的资产不可删除、不可覆盖；只有 `DRAFT` 版本文件可删除。
3. **草稿不可交付**：`DRAFT` 版本及其文件不得出现在 `/api/v1/delivery` 或下载票据中。
4. **兑换码绑定已发布版本**：签发时绑定 `product_id` 与已发布 `release_id`；被兑换码引用的基础版本不可撤回（`release_in_use`）。
5. **发布门槛**：商品转 `ACTIVE` 前至少一个交付文件；版本发布要求商品 `ACTIVE` 且该版本至少一个文件。
6. **权益范围**：通用文件 + 权益起始版本及其之后所有已发布版本。
7. **下载额度**：票据 5 分钟有效；每次签发使 `downloads_used` +1，交付接口返回 `max_downloads` 与 `downloads_used`。
8. **更新策略快照**：商品更新策略在兑换码签发时快照到 `redeem_codes`，兑换时复制到权益；交付/票据按 `LIFETIME / TILL_DATE / CURRENT_ONLY` 过滤后续版本，起始版本始终可见。
9. **退款即收权**：`REFUND` 将兑换码置 `REFUNDED`、关联权益置 `REVOKED`，原因写入审计；退款后交付/下载立即失效。
10. **成员与审计权限**：仅 OWNER/ADMIN 拥有 `member.manage`/`audit.read`；不能修改自己或 OWNER；暂停成员会撤销其会话。
11. **密码变更语义**：改密保留当前会话、撤销其他设备；`token_version` 递增，旧会话失效。
12. **schema 变更只新增迁移**：不要修改已应用的迁移，checksum 不一致会阻止启动。
13. **不提交**：`.env`、密钥、真实用户/订单数据、售卖文件（源码包、压缩包、安装包）。
14. **前后台分离**：`web/` 不得引用 `console/` 源码或运营 API。
15. **错误码是契约**：前端与冒烟脚本断言 `code`，不要无声修改语义。
16. **PowerShell 脚本**：UTF-8 BOM + LF（Windows PowerShell 5.1 中文兼容）；JSON 请求体用 UTF-8 字节；multipart 上传用 curl，不要用 `Invoke-RestMethod -Form`。

## 6. 新增 API

1. 在 `server/internal/api/` 写 handler：参数化 SQL，租户来自请求上下文，失败用 `writeError`。
2. 在 `app.go` 注册路由：公开 API 或运营路由 + 具体权限点（见 `permissions.go`）。
3. 补测试：`*_test.go` 覆盖成功、参数校验、越权/跨租户、状态冲突。
4. 更新 [docs/api.md](docs/api.md) 与 [README.md](README.md) 的 API 表；响应变化同步 `web/`、`console/` 的 `lib/api.ts`、`types.ts`、zod 校验与冒烟断言。
5. 分页、限流、请求 ID 等横切关注点复用现有中间件与约定。

## 7. 新增页面

1. 在 `web/src/features/<domain>/` 或 `console/src/features/<domain>/` 创建组件。
2. 请求放 `lib/api.ts`，类型放 `types.ts`，输入用 zod 校验；错误展示使用 API 的 `code` 与 `message`。
3. 在 `App.tsx` 注册路由；遵循视觉 token（暖白/墨黑/雾蓝/银灰）与 `prefers-reduced-motion` 降级。
4. 确认 `web/` 不引用 console，并让 `web/`、`console/` 分别 `npm.cmd run build` 通过。

## 8. 新增迁移

1. 复制现有迁移并命名为 `000N_<snake_case>.sql`，只追加、不修改旧文件。
2. 变更需向后兼容：新列可空或带默认值；不删除旧列/旧表。
3. 本地启动 API 验证自动应用，必要时补 `server/internal/db` 测试。
4. 更新 [docs/architecture.md](docs/architecture.md) 数据模型与 [docs/api.md](docs/api.md)（若影响契约）；部署前先备份数据库。

## 9. 错误码与响应约定

- 成功：`2xx` + JSON（下载为文件流）。
- 失败：`{ "code": "snake_case", "message": "<English user-facing>" }`；前端展示 `message`，逻辑判断 `code`。
- 状态：`400` 参数/格式；`401` 未登录或凭证失效；`403` 权限不足；`404` 不存在或按租户隐藏；`409` 状态冲突/不可变；`410` 已过期；`413` 过大；`429` 限流（带 `Retry-After`）；`503` 依赖不可用。
- 运营 API 需 `Authorization: Bearer <operator-session>`；买家交付需 `Bearer <access_token>`；响应带 `X-Request-Id`。
- 列表分页：`limit` / `offset` 与 `total` / `has_more`，非法参数返回 `invalid_pagination`。
- 常见错误码：`operator_auth_required`、`operator_permission_required`、`release_in_use`、`asset_immutable`、`delivery_asset_required`、`download_limit_reached`、`invalid_pagination`。

## 10. 测试要求

- 提交前至少运行 `.\scripts\verify.ps1 -SkipE2E`（或 `./scripts/verify.sh --skip-e2e`）。
- 后端改动必须 `go vet ./...` + `go test ./...` 通过；新逻辑补 table-driven 测试。
- 前端改动必须让 `web/`、`console/` 都通过 `npm.cmd run build`（含 `tsc -b` 类型检查）。
- 改动兑换、权益、交付、下载额度或租户隔离时，必须运行 `scripts/qa/run-all.ps1` 并确认 `pass: true`；`-SkipE2E` 不能替代。
- CI 结构（`.github/workflows/ci.yml`）：hygiene（`git diff --check`）、backend、frontend matrix、smoke。
- 只使用合成数据；脚本结束清理夹具；不依赖生产数据或真实密钥。

## 11. 文档索引

- [README.md](README.md)：项目定位、本地运行、API 入口、视觉方向
- [docs/product-definition.md](docs/product-definition.md)：角色、业务流程、状态与 MVP 边界
- [docs/architecture.md](docs/architecture.md)：分层、数据模型、文件交付、安全边界
- [docs/api.md](docs/api.md)：HTTP 契约、错误码、分页与集成建议
- [docs/development.md](docs/development.md)：环境、常用命令、迁移、排障
- [docs/operations.md](docs/operations.md)：部署、备份恢复、维护清理、日志
- [docs/admin-saas-design.md](docs/admin-saas-design.md)：控制台与 SaaS 设计
- [docs/roadmap.md](docs/roadmap.md)：迭代路线
- [docs/audit-split.md](docs/audit-split.md)、[docs/audit-2026-11-11.md](docs/audit-2026-11-11.md)、[docs/audit-2026-11-19.md](docs/audit-2026-11-19.md)、[docs/audit-2026-11-19-v0.3.0.md](docs/audit-2026-11-19-v0.3.0.md)：验收与审计证据
- [CHANGELOG.md](CHANGELOG.md)：版本变更记录

## 12. 工作方式

- 修改范围最小化；不要顺手重排无关代码或改换行风格。
- 提交前运行 `git diff --check`；不同主题不要混成一个提交。
- 未经用户明确要求，不执行 `docker compose down -v`、删除卷、`git push --force` 等破坏性操作。
- 仓库统一 LF（见 `.gitattributes` / `.editorconfig`）；PowerShell 脚本使用 UTF-8 BOM。
