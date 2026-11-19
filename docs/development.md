# 开发指南（DX）

本文面向在本仓库中持续开发的工程师与 AI 智能体。产品边界见[产品定义](product-definition.md)，系统设计见[技术架构](architecture.md)。智能体专用速查表在仓库根目录的 [AGENTS.md](../AGENTS.md)。

## 1. 环境要求

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.24+ | API、迁移与维护命令 |
| Node.js / npm | 24+ / 11+ | 用户端 `web/` 与控制台 `console/` |
| Docker + Compose | 29+ | 本地整套运行、集成冒烟 |
| PowerShell | 5.1+ 或 7+ | QA 与验证脚本（Windows 默认即可） |

Windows 注意：

- 执行策略禁止运行 `npm.ps1` 时使用 `npm.cmd`。
- 直接运行 `.ps1` 被拒绝时，用 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File <脚本>`，或在当前会话执行 `Set-ExecutionPolicy -Scope Process Bypass`。
- 仓库脚本已兼容 Windows PowerShell 5.1，不需要 PowerShell 7。

## 2. 一键验证

```powershell
# 单元测试 + 静态检查 + 双端构建（默认再跑四套业务冒烟）
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1

# 只做快速检查，跳过需要运行中容器的冒烟
.\scripts\verify.ps1 -SkipE2E

# 输出 JSON 报告 / 额外运行真实数据库集成测试
.\scripts\verify.ps1 -ReportPath .\tmp\verify-report.json
$env:TEST_DATABASE_URL = "postgres://nover:<password>@127.0.0.1:5433/nover_integration_test?sslmode=disable"
$env:TEST_STORAGE_DIR = "D:\NOVER\tmp\integration-storage"
.\scripts\verify.ps1 -SkipE2E -Integration
```

集成测试使用 build tag，需要可丢弃的 PostgreSQL；测试会创建临时商品/账号并在结束时清理。CI 在独立 PostgreSQL service 上运行同一套测试。

POSIX 环境使用 `./scripts/verify.sh`（CI 使用同一套步骤）。CI 配置见 [.github/workflows/ci.yml](../.github/workflows/ci.yml)。

## 3. 本地运行

### 3.1 整套容器

```powershell
Copy-Item .env.example .env
docker compose up --build
```

| 服务 | 地址 |
| --- | --- |
| 用户端 | http://localhost:45173 |
| 控制台 | http://localhost:46173/ops-7f3c |
| API | http://localhost:48080 |
| PostgreSQL | 127.0.0.1:5433 |

开发账号为 `admin / admin`，仅用于本地。修改后端代码后重新构建：

```powershell
docker compose up -d --build api
docker compose logs -f api
```

### 3.2 前后端分离开发

```powershell
docker compose up -d db

# 终端 A：API
$env:DATABASE_URL = "postgres://nover:local-development-password-change-me@localhost:5433/nover?sslmode=disable"
$env:STORAGE_DIR = ".\runtime-data\files"
cd server
go run ./cmd/api

# 终端 B：用户端
cd web
npm.cmd ci
npm.cmd run dev

# 终端 C：控制台
cd console
npm.cmd ci
npm.cmd run dev
```

前端默认经各自 Vite 代理访问同源 `/api`，跨域只允许 `.env` 中显式列出的 `ALLOWED_ORIGINS`。

## 4. 数据库迁移

数据库结构由 `server/internal/db/migrations/*.sql` 管理，API 启动时自动应用：

1. 读取并校验所有嵌入的迁移（文件名必须是 `0001_snake_case.sql`，版本号唯一）。
2. 创建 `schema_migrations` 表并校验已应用迁移的校验和。
3. 旧版（历史幂等 schema）安装自动识别并标记基线，再执行新迁移。
4. 所有迁移在单个事务内按版本顺序执行。

新增结构变更：

```text
复制 0002_maintenance_indexes.sql 为 0003_<描述>.sql → 写入 SQL → 本地启动 API 验证 → 提交
```

**不要修改已发布的迁移文件**：校验和不一致会阻止启动。需要修正时新增迁移。

## 5. 维护与清理

```powershell
# 迁移状态 / 手工应用（等价于 API 启动时的自动迁移）
docker compose --profile ops run --rm migrate -status
docker compose --profile ops run --rm migrate -apply
docker compose --profile ops run --rm migrate -json

# 干跑（默认，只报告）
docker compose --profile ops run --rm maintenance

# 执行清理
docker compose --profile ops run --rm maintenance -apply

# 机器可读输出
docker compose --profile ops run --rm maintenance -json
```

清理内容：过期/已撤销会话、过期下载票据、把过期兑换码与权益标记为 `EXPIRED`、删除超过 24 小时且无数据库引用的私有文件与空目录。升级/回滚流程见[升级指南](upgrade.md)。

## 6. 业务冒烟

```powershell
# 四套一起跑（需要 API 已在 127.0.0.1:48080 运行）
.\scripts\qa\run-all.ps1

# 单独运行
.\scripts\qa\version-entitlement-smoke.ps1
.\scripts\qa\tenant-delivery-smoke.ps1
.\scripts\qa\operator-capabilities-smoke.ps1
.\scripts\qa\identity-mfa-invitation-smoke.ps1
```

第三套覆盖商品结构化资料与更新策略、退款、成员管理、改密/会话与审计日志；第四套覆盖一次性邀请、设置密码、TOTP MFA 启用/两步登录/恢复码、管理员重置密码与审计。脚本会创建临时租户/商品/成员并在结束时清理；`-KeepFixture` 可保留夹具用于手动排查。

## 7. 代码结构

| 路径 | 职责 |
| --- | --- |
| `server/cmd/api` | API 进程入口、启动日志、优雅停机 |
| `server/cmd/maintenance` | 数据保留与文件清理命令 |
| `server/cmd/migrate` | 迁移状态/应用命令（本地升级与 CI） |
| `server/internal/api` | 路由、中间件、权限、业务处理器 |
| `server/internal/config` | 环境变量加载与生产环境校验 |
| `server/internal/db` | 连接、迁移器、引导账号 |
| `server/internal/maintenance` | 清理逻辑（可单测） |
| `server/internal/buildinfo` | 构建版本元数据 |
| `web/src` | 公开用户端 |
| `console/src` | 独立运营控制台 |
| `scripts/` | 验证与 QA 脚本 |

## 8. 约定

- API 错误统一 `{ "code": "...", "message": "..." }`，`X-Request-Id` 可用于关联日志。
- 所有运营查询必须使用会话语义中的 `tenant_id`，绝不信任客户端传入的租户。
- 文件只通过私有存储 + 短时票据访问；已发布版本文件不可变，草稿文件才可删除。
- 新接口需要：权限点、错误码、单元测试（或冒烟覆盖）、文档表格更新。
- 前端不引入 console 代码到 web；两套应用保持独立构建。
- 提交前运行 `scripts/verify.ps1 -SkipE2E`，涉及交付链路时再跑完整冒烟。

## 9. 常见问题

| 现象 | 处理 |
| --- | --- |
| `/api` 返回 403 / CORS 错误 | 检查 `.env` 的 `ALLOWED_ORIGINS` 是否包含当前页面 Origin |
| 登录 401 | 开发环境确认 `admin/admin`，数据库是否已引导默认账号 |
| 迁移校验和错误 | 说明修改了已发布迁移；改为新增迁移 |
| 上传 413 | 检查 `MAX_UPLOAD_BYTES` 与 Nginx `client_max_body_size` |
| 下载 410 | 票据 5 分钟过期或权益失效，回到交付页重新生成 |
| 冒烟失败 | 先 `docker compose logs api db`，再确认 `.env` 与端口占用 |
