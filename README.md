# 知屿 NOVER

> A Space for Digital Discovery.
> 探索数字，发现更多可能。

NOVER 是一个面向数字产品的开源交付平台。首页负责介绍品牌和展示公开商品；平台代码负责商品、版本、兑换凭证、权益和安全下载。实际售卖的程序源码、站点源码或其他数字文件，在项目完成后由运营者独立上传，不进入本仓库，也不随开源代码发布。

## 项目定位

NOVER 不是传统的多商户商城，而是“外部平台成交 + NOVER 数字交付”的轻量系统：

```text
闲鱼/其他平台成交
        ↓
卖家创建或分配兑换凭证
        ↓
买家在 NOVER 兑换
        ↓
创建购买权益并签发临时下载地址
        ↓
下载指定版本、查看更新与校验信息
```

第一阶段只解决可靠交付，不内置站内支付、购物车、多商户分账和复杂营销系统。

当前以**单实例、自托管**部署为主，并已形成基础多租户交付闭环。数据库身份、租户成员关系、平台/租户角色、权限中间件和可撤销会话已经建立；商品、效果图、交付资产、兑换码、权益、下载票据、下载事件和审计记录具有租户作用域。公开申请会创建独立的待审租户和只读账号：申请人可登录查看账号与申请状态，管理员通过后才获得商品、文件和兑换码权限。兑换码和访问令牌由服务端定位租户；商户商品目录可通过 `/spaces/{tenant_slug}` 展示。RLS、租户域名、邀请设密、MFA、配额与账单尚未完成，当前不应视为生产级 SaaS。

## 开源边界

| 内容 | 是否进入本仓库 | 说明 |
| --- | --- | --- |
| NOVER 前端、后端、部署配置 | 是 | 按仓库许可证发布 |
| 示例数据、测试文件、占位资源 | 是 | 必须是合成或可公开分发内容 |
| 实际出售的源码、素材、安装包 | 否 | 通过管理端或对象存储独立上传 |
| 兑换码、订单号、用户数据、密钥 | 否 | 仅存在于部署实例的运行环境 |
| 生产环境配置与对象存储凭证 | 否 | 使用环境变量或密钥管理系统注入 |

仓库当前使用 Apache License 2.0。若未来需要更换许可证，应在加入业务代码和发布版本前单独决策。

## 技术方向

### 前端

- React
- TypeScript
- Tailwind CSS
- ReactBits：作为核对来源与许可证后选用的可复制组件参考
- anime.js：用于轻量过渡和状态动效
- GSAP：用于复杂时间轴、滚动和场景动画（统一使用 `GSAP` 拼写）

ReactBits 是可复制组件参考集合，不是本项目依赖的运行时包；MVP 的品牌图形和业务控件先由仓库内组件实现，后续仅引入来源和许可证清晰的具体组件。anime.js 和 GSAP 专注于表现层，动画必须可关闭、可降级，兑换和下载始终可用。

### 后端与部署

- Go：API、鉴权、兑换事务、下载签发和后台服务
- PostgreSQL：持久化业务数据
- Redis：当前 MVP 不启用，后续按多实例限流或队列需求评估
- 本地私有文件卷：当前 MVP 交付存储；后续可替换为 S3 兼容对象存储
- Docker Compose：本地开发和单机部署
- Caddy 或 Nginx：TLS、反向代理和静态资源服务

单实例 MVP 使用 PostgreSQL 和进程内限流；下载票据由 PostgreSQL 持久化。多实例部署、后台队列或高频兑换防刷时再评估 Redis。

## 文档

- [产品定义](docs/product-definition.md)：角色、业务流程、状态和 MVP 边界
- [技术架构](docs/architecture.md)：前后端分层、数据模型、文件交付和安全边界
- [迭代路线](docs/roadmap.md)：从空仓库到可运营版本的阶段计划
- [运营控制台与 SaaS 设计](docs/admin-saas-design.md)：前后台拆分、账号准入、RBAC、租户隔离和部署配置
- [开发指南](docs/development.md)：本地开发、迁移、冒烟与排障（DX）
- [运维手册](docs/operations.md)：部署检查、维护清理、备份恢复与排障
- [升级与回滚](docs/upgrade.md)：本地升级、迁移状态、回滚与升级后验证
- [设计系统](docs/design-system.md)：颜色/字体/组件/动效/可访问性规范
- [API 接口说明](docs/api.md)：接口契约、错误码、分页与退避约定
- [智能体指南](AGENTS.md)：仓库地图、不变量、常用命令与改动流程（AX）
- [前后台拆分验收](docs/audit-split.md)：本地容器、浏览器流程、响应式和构建边界证据
- [2026-11-11 阶段审计](docs/audit-2026-11-11.md)：租户交付、文件管理、页面测试与剩余风险
- [2026-11-19 版本审计](docs/audit-2026-11-19.md)：v0.2.0 工程收口、UX/DX/AX 改造与验证证据
- [2026-11-19 v0.3.0 审计](docs/audit-2026-11-19-v0.3.0.md)：商品资料、退款、成员、账户安全与审计
- [2026-11-19 v0.4.0 审计](docs/audit-2026-11-19-v0.4.0.md)：MFA、邀请、重置、指标与体验打磨
- [2026-11-19 v0.5.0 审计](docs/audit-2026-11-19-v0.5.0.md)：本地升级、迁移状态、集成测试与验证体验
- [CHANGELOG](CHANGELOG.md)：版本变更记录

## 当前状态

MVP 已包含 Go API、两个独立 React 前端、PostgreSQL schema 和 Docker Compose。`web/` 只包含公开商品、兑换、交付与加入申请；`console/` 是独立的运营控制台构建，包含数据总览、商品创建、兑换码管理、成员申请、成员与权限和部署设置。两端使用不同端口、静态镜像、应用壳层、主题状态和 API 客户端。商品创建采用“内容与实时预览 → 效果图片 → 版本草稿 → 版本文件 → 商品/版本发布”的连续流程，详细介绍支持 Markdown，并可在保存草稿后直接粘贴剪贴板图片。交付文件绑定具体 SemVer 版本；兑换码绑定一个已发布起始版本，权益在有效期内可访问该版本及后续发布版本。买家交付页按版本分组文件并提示可用更新；已被兑换码引用的基础版本不能撤回。运营登录使用数据库密码哈希、服务端可撤销会话和后端权限点。公开申请会创建只读待审账号和独立租户，通过审核后激活 OWNER 商品权限；MFA、密码重置和更完整的 SaaS 隔离按 [SaaS 设计](docs/admin-saas-design.md) 分阶段实现。

2026-11-19 发布 v0.2.0：数据库结构改为版本化迁移（自动识别历史安装、校验和防篡改），新增维护清理命令与 Compose `ops` profile、结构化请求日志、`/api/v1/meta` 构建信息、429 `Retry-After`、活动分页、交付下载额度字段；双端补齐可访问性基线与深链、目录搜索、额度与有效期展示；新增 CI、跨平台 QA 脚本、一键验证脚本、开发/运维/API 文档与智能体指南。生产化缺口（RLS、MFA、对象存储、备份恢复、指标告警）仍按路线图推进。

2026-11-19 发布 v0.3.0：商品资料升级为结构化资料（亮点、授权、技术要求、支持期、更新策略）并新增独立公开详情接口；兑换码签发时快照更新策略，交付与下载按 `LIFETIME / TILL_DATE / CURRENT_ONLY` 过滤版本；新增手工退款（权益立即失效并留痕）、成员创建与角色/状态管理、账户改密与会话管理、租户审计日志页与 API。控制台新增“账户与安全”“审计日志”视图，用户端详情页展示结构化资料与更新范围。

2026-11-19 发布 v0.4.0：运营账号新增可选 TOTP 两步验证与 10 个一次性恢复码；成员入职改为一次性邀请链接设密，管理员可为成员生成一次性密码重置链接（均线下传递，token 只存哈希）；新增 `/metrics` Prometheus 指标、备份/恢复脚本与设计系统文档。前端完成视觉与体验打磨：商品图光箱、骨架屏、Toast、空状态、控制台两步登录/MFA 设置/邀请接受页，以及移动端与可访问性改进。

2026-11-19 发布 v0.5.0：新增 `nover-migrate` 迁移 CLI（status/apply/json）与 Compose `ops` migrate 服务；控制台“部署设置 → 系统状态”展示版本、迁移 applied/pending/latest、数据库延迟与存储可写性；新增本地升级/回滚脚本 `scripts/ops/upgrade.ps1`（自动备份、镜像记录、就绪等待与摘要）；Compose 为 web/console 增加健康检查与 API 就绪依赖；验证脚本支持 JSON 报告与真实数据库集成测试，CI 新增 PostgreSQL 集成测试 job；用户端/控制台显示版本号并补齐升级体验。

## 本地运行

运行整套容器只需要 Docker Compose。复制开发配置后启动：

```powershell
Copy-Item .env.example .env
docker compose up --build
```

构建元数据可通过 `.env` 的 `APP_VERSION`、`GIT_COMMIT`、`BUILD_TIME` 注入，并出现在启动日志与 `/api/v1/meta` 中。

打开用户端 [http://localhost:45173](http://localhost:45173)，或打开独立运营控制台 [http://localhost:46173/ops-7f3c](http://localhost:46173/ops-7f3c)。已通过审核的商户可在商品创建页打开自己的 `/spaces/{tenant_slug}` 公开入口。开发环境账号密码均为 `admin`。Go API 独立暴露在 `http://localhost:48080`。用户端访问 `/ops-7f3c` 只会回到公开商品页面，不包含控制台页面或运营 API 客户端。售卖文件保存在 Docker 私有卷 `private-assets`，数据库保存在 `postgres-data`。

本地开发前端和 API 时，先启动数据库：

```powershell
docker compose up -d db
Get-Content .env | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process') } }
$env:DATABASE_URL = "postgres://{0}:{1}@localhost:{2}/{3}?sslmode=disable" -f $env:POSTGRES_USER, $env:POSTGRES_PASSWORD, $env:DB_HOST_PORT, $env:POSTGRES_DB
$env:STORAGE_DIR = '.\runtime-data\files'
cd server
go run ./cmd/api
```

另开两个终端分别启动用户端与控制台：

```powershell
# 终端 A
cd web
npm ci
npm run dev
```

```powershell
# 终端 B
cd console
npm ci
npm run dev
```

本地运行需要 Go 1.24+、Node.js 24+ 和 npm。PowerShell 从仓库根目录启动；Go API 使用 `.env` 中的管理员令牌和密钥。

用户端 Vite 使用 `45173`，控制台 Vite 使用 `46173`，Go API 使用 `48080`；两端前端默认经各自 Vite/Nginx 的同源 `/api` 与 `/readyz` 代理访问独立 API，避免 `localhost`、`127.0.0.1` 和自定义域名的跨域差异。显式设置 `VITE_API_BASE_URL` 时才直连独立 API 地址，并需同步配置后端允许来源及前端 CSP。`.env.example` 只用于本地启动；公开部署前替换所有示例密钥、启用 HTTPS 并配置持久化备份。

## 验证与回归

在仓库根目录一键验证（单元测试、静态检查、双端构建，默认再跑两套业务冒烟）：

```powershell
# 完整验证（单元测试、静态检查、双端构建、四套业务冒烟）
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1

# 输出 JSON 报告
.\scripts\verify.ps1 -ReportPath .\tmp\verify-report.json

# 额外运行真实数据库集成测试（需要可丢弃的 PostgreSQL）
$env:TEST_DATABASE_URL = "postgres://nover:<password>@127.0.0.1:5433/nover_integration_test?sslmode=disable"
$env:TEST_STORAGE_DIR = "D:\NOVER\tmp\integration-storage"
.\scripts\verify.ps1 -SkipE2E -Integration
```

冒烟脚本要求 API 已在 `127.0.0.1:48080` 运行；也可以单独执行：

```powershell
.\scripts\qa\run-all.ps1
.\scripts\qa\version-entitlement-smoke.ps1
.\scripts\qa\tenant-delivery-smoke.ps1
.\scripts\qa\operator-capabilities-smoke.ps1
.\scripts\qa\identity-mfa-invitation-smoke.ps1
```

前三套分别验证版本发布幂等、文件保护、兑换与下载额度，待审只读、审核激活、商户目录、跨租户不可见，以及商品结构化资料与更新策略、退款、成员管理、改密/会话与审计日志；第四套验证一次性邀请设密、TOTP MFA 启用/两步登录/恢复码与管理员重置密码。四套结束后均清理测试记录与文件，`-KeepFixture` 可保留夹具。CI 配置见 [.github/workflows/ci.yml](.github/workflows/ci.yml)。

## API 入口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 进程存活检查 |
| `GET` | `/readyz` | 数据库就绪检查 |
| `GET` | `/api/v1/meta` | 公开版本、提交、环境与 schema_version |
| `GET` | `/metrics` | Prometheus 文本指标（请求/运行时长/构建信息） |
| `GET` | `/api/v1/products` | 首页和商品详情页使用的公开商品目录 |
| `GET` | `/api/v1/products/{productID}` | 公开商品详情（结构化资料、图片、更新策略） |
| `GET` | `/api/v1/spaces/{slug}/products` | 指定商户的公开商品目录；图片路由同样带空间前缀 |
| `GET` | `/api/v1/spaces/{slug}/products/{productID}` | 指定商户空间的商品详情 |
| `POST` | `/api/v1/join-applications` | 提交组织与运营账号申请 |
| `POST` | `/api/v1/invitations/accept` | 一次性邀请/密码重置 token 设置密码 |
| `POST` | `/api/v1/cp-7f3c/login` | 运营账号密码登录，签发短期会话（启用 MFA 时返回两步挑战） |
| `POST` | `/api/v1/cp-7f3c/login/mfa` | 完成 TOTP/恢复码两步验证 |
| `GET/POST` | `/api/v1/cp-7f3c/mfa[...]` | 查看、设置、启用或关闭 TOTP 两步验证 |
| `GET/POST` | `/api/v1/cp-7f3c/session[...]` | 读取当前身份或撤销当前会话 |
| `POST` | `/api/v1/cp-7f3c/password` | 修改当前账号密码（保留当前会话） |
| `GET` | `/api/v1/cp-7f3c/sessions` | 查看当前账号有效会话 |
| `POST` | `/api/v1/cp-7f3c/sessions/{id}/revoke` | 撤销指定会话 |
| `POST` | `/api/v1/cp-7f3c/sessions/revoke-others` | 撤销除当前设备外的会话 |
| `GET` | `/api/v1/cp-7f3c/audit-events` | 租户审计日志（筛选 + 分页） |
| `GET` | `/api/v1/cp-7f3c/members` | 查看当前租户成员与角色 |
| `GET/POST/PATCH` | `/api/v1/cp-7f3c/members[...]` | 查看成员、邀请/创建成员、调整角色/状态 |
| `POST` | `/api/v1/cp-7f3c/members/invitations[...]` | 创建/查看/撤销一次性成员邀请 |
| `POST` | `/api/v1/cp-7f3c/members/{id}/password-reset` | 为成员生成一次性密码重置链接 |
| `GET` | `/api/v1/cp-7f3c/products` | 运营控制台商品列表 |
| `GET` | `/api/v1/cp-7f3c/activity` | 最近兑换、权益和下载活动，支持 `limit`/`offset` 分页 |
| `GET/POST` | `/api/v1/cp-7f3c/applications[...]` | 查看与审核加入申请 |
| `GET/PUT` | `/api/v1/cp-7f3c/settings` | 保存部署拓扑和路由规划 |
| `GET` | `/api/v1/cp-7f3c/system/status` | 版本、迁移状态、数据库延迟与存储健康 |
| `POST` | `/api/v1/cp-7f3c/products` | 创建商品 |
| `PUT` | `/api/v1/cp-7f3c/products/{id}` | 保存商品名称、简介与 Markdown 详细介绍 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/status` | 发布、下架为草稿或归档商品 |
| `GET/POST/DELETE` | `/api/v1/cp-7f3c/products/{id}/images[...]` | 鉴权预览、粘贴/上传和删除商品效果图 |
| `GET/POST` | `/api/v1/cp-7f3c/products/{id}/releases` | 查看和创建商品版本草稿 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/releases/{releaseID}/status` | 发布、撤回或归档商品版本 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/assets` | 上传并绑定版本草稿的单个交付文件，表单包含 `release_id` |
| `GET/DELETE` | `/api/v1/cp-7f3c/products/{id}/assets[...]` | 查看文件清单；仅草稿版本文件可删除 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/codes` | 签发绑定当前商品与已发布起始版本的兑换码 |
| `POST` | `/api/v1/cp-7f3c/codes/{id}/action` | 暂停、恢复、撤销或退款兑换凭证 |
| `POST` | `/api/v1/redeem` | 兑换并取得交付凭证 |
| `GET` | `/api/v1/delivery` | 获取商品、文件清单与下载额度信息 |
| `POST` | `/api/v1/assets/{id}/ticket` | 签发 5 分钟下载票据 |
| `GET` | `/api/v1/downloads/{ticket}` | 下载私有文件 |

运营 API 使用登录返回的 `Authorization: Bearer <operator-session>`；会话同时绑定数据库用户、成员关系与租户，退出会在服务端撤销。开发环境首次启动会把 `admin/admin` 初始化为默认空间的 `OWNER + PLATFORM_OWNER`；生产环境必须替换引导凭证和 `OPERATOR_TOKEN_KEY`，并启用 HTTPS、MFA 与密钥轮换。

公开商品详情页使用前端路由 `/products/{productID}`，当前从公开目录读取商品短简介、详细介绍和效果图；真实文件清单、版本和校验信息仍需兑换后查看。效果图为公开素材，限制 JPEG/PNG、单张 8 MB 和图像尺寸；交付文件独立保存在私有目录，新上传文件按 `tenants/{tenant_id}/...` 组织，旧版本对象键仍可读取。后台商品会显示所属租户和负责人，业务查询不会接受前端传入的租户 ID。

兑换码在签发时通过当前商品的工作台选择生成，数据库和兑换权益都绑定同一 `product_id` 与基础 `release_id`；复合外键阻止凭证与其他商品或版本的权益错配。不同商品需分别签发兑换码。旧数据中没有 `release_id` 的通用文件、兑换码和权益继续兼容，但版本草稿永远不会出现在公开交付或下载票据中。

版本发布需要商品为 `ACTIVE`，且该版本至少绑定一个交付文件。权益交付范围为“通用文件 + 起始版本及其后续已发布版本”；基础版本一旦被已签发兑换码引用，服务端返回 `release_in_use` 并阻止撤回。

`OPERATOR_WEB_PATH` 和 `OPERATOR_API_PATH` 可更改默认控制台与运营 API 路径；更改后需要重新构建 `console` 并重启 API。`CONSOLE_URL` 决定用户端“前往独立控制台”的外部链接，`STOREFRONT_URL` 决定控制台内商户公开入口的域名。控制台内的部署设置用于保存目标配置和生成环境变量，不会动态改写运行中的路由。运营业务查询的租户来自已验证服务端会话，不接受前端提交的 `tenant_id`；公开默认目录仍为默认空间，商户目录使用空间 slug，兑换与交付由服务端凭证定位租户。

## 数据库迁移与维护

数据库结构由 `server/internal/db/migrations/*.sql` 版本化管理，API 启动时自动应用，并用 `schema_migrations` 记录校验和；历史幂等 schema 安装会自动识别为基线。**已发布的迁移文件不可修改**，结构变更新增 `0003_...sql` 即可。

数据保留清理通过独立命令执行，默认干跑，`-apply` 才会删除/更新：

```powershell
# 干跑：只报告过期会话、票据、兑换码、权益与孤儿文件
docker compose --profile ops run --rm maintenance

# 执行清理并使用 JSON 输出
docker compose --profile ops run --rm maintenance -apply -json
```

运维细节（备份恢复、cron、故障排查）见[运维手册](docs/operations.md)；本地升级、迁移状态与回滚见[升级指南](docs/upgrade.md)：

```powershell
# 迁移状态 / 手工应用
docker compose --profile ops run --rm migrate -status
docker compose --profile ops run --rm migrate -apply

# 一键升级（预检 + 备份 + 构建 + 就绪检查）与回滚
.\scripts\ops\upgrade.ps1 -DryRun
.\scripts\ops\upgrade.ps1
.\scripts\ops\upgrade.ps1 -Rollback
```

接口契约见[API 说明](docs/api.md)。

## 视觉方向

NOVER 采用“极简岛屿 / Minimal Island”：

- 暖白 `#F6F6F3` 作为主背景
- 墨黑 `#17191C` 作为主要文字和品牌色
- 雾蓝 `#A7B9CB` 作为装饰和浅色模块
- 银灰 `#D9DDE2` 作为边界和分隔
- 深雾蓝 `#5B7085` 作为需要更高对比度的链接和主要操作色

字体使用本机可用的开源/系统字体栈：`Noto Sans SC`、`Noto Sans`、`Source Han Sans SC`、`Microsoft YaHei`、`PingFang SC` 和 `Segoe UI` 回退，不下载或打包商业字体；控制台正文和辅助文字已提高到更适合阅读的字号。

视觉应服务于交付流程：兑换、版本、文件、有效期和下载状态必须清晰可见，不为了留白隐藏关键业务信息。
