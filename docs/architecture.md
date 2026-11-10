# 技术架构

## 1. 总体结构

MVP 采用模块化单体，避免把兑换、权益和下载拆成多个服务后引入分布式一致性问题。

```text
Browser
  │
  ├── Storefront React :45173（公开商品、兑换、交付、加入申请）
  ├── Console React    :46173（运营登录与控制面）
  │       └── TypeScript + Tailwind + anime.js / GSAP（表现层动效）
  │
  └── HTTPS API :48080
          │
          └── Go API
              ├── Operator Auth / Workspace
              ├── Product / Release
              ├── Redeem / Entitlement
              ├── Download Ticket
              ├── Rate Limit / Idempotency
              └── Audit Log
                    │
                    ├── PostgreSQL
                    ├── Redis（按需）
                    ├── private-assets 本地卷（MVP）
                    └── S3 兼容私有对象存储（后续）
```

### 部署与租户模型

当前 NOVER 是**单租户、自托管应用**：一次 Docker Compose 部署对应一个运营主体、一套品牌和一套商品数据。数据库账号、租户成员关系、平台/租户角色、后端权限点和可撤销会话已实现；核心业务表已完成 `tenant_id` 默认租户回填、API 查询作用域和核心关系复合外键，但 RLS、邀请设密和 MFA 尚未完成，所以现在仍不是可直接开放多商户入驻的平台。公开加入申请会创建独立的待审租户和只读账号，通过审核后激活 OWNER 商品权限。完整迁移见 [运营控制台与 SaaS 设计](admin-saas-design.md)。

如果后续提供平台托管版，需要先完成以下改造，而不是只增加一个“商户”表：

- 所有商品、图片、资产、兑换码、权益、下载和审计记录加入不可为空的 `tenant_id`；
- 所有查询、唯一键、事务和对象存储路径强制租户作用域，并增加越权测试；
- 增加租户、成员、邀请、角色和细粒度 RBAC，区分平台管理员与租户运营者；
- 增加套餐、配额、计量、账单、停服与数据导出规则；
- 增加租户域名、品牌、邮件和存储命名空间；
- 增加租户感知的限流、审计、备份恢复、删除和平台运维面板。

在这些基础完成之前，推荐保持“一次部署服务一个团队”，不在同一数据库中手工混放多个卖家数据。

## 2. 推荐技术基线

### 前端

| 层 | 默认选择 | 原因 |
| --- | --- | --- |
| 构建 | Vite | 与 Go API 解耦，开发和部署简单 |
| 路由 | React Router | 兑换、交付和管理后台有明确页面边界 |
| 数据请求 | TanStack Query | 缓存、重试和失效策略不散落在组件中 |
| 校验 | Zod | 对 API 输入和表单输入做运行时校验 |
| 样式 | Tailwind CSS | 与 NOVER 视觉 token 和响应式约束匹配 |
| 组件 | 本地组件；ReactBits 作为参考 | 只引入来源、许可证和维护成本明确的组件 |

ReactBits 是可复制组件集合，不是本 MVP 使用的 npm 运行时依赖。当前品牌图形和业务控件由本地组件实现；后续选用具体 ReactBits 组件时，再复制到仓库并记录来源及许可证。

### Go 后端

| 层 | 默认选择 | 原因 |
| --- | --- | --- |
| HTTP | `go-chi/chi` | 轻量、贴近标准库，适合模块化单体 |
| PostgreSQL | `pgx` | 原生协议能力完整，连接池和事务控制清晰 |
| 查询 | MVP 使用 `pgx` 参数化 SQL；后续可评估 `sqlc` | 当前 schema 和查询规模较小，先保持 SQL 直接可审查 |
| 迁移 | MVP 启动时执行幂等 schema；生产化前采用版本化迁移 | 当前适合新库初始化，不足以管理有数据的结构演进 |
| 日志 | `log/slog` | 标准库结构化日志，减少基础依赖 |
| 配置 | 环境变量 + typed config | 适合 Docker Compose 和生产密钥注入 |

如果团队更熟悉 Gin 或 Echo，可以替换 HTTP 层；但不要同时引入多个 Web 框架。第一版优先选择 `chi`，保持业务代码接近标准库。

## 3. 前端约定

两个前端独立组织。用户端构建不得引用控制台源文件或运营 API：

```text
web/src/
  App.tsx             公开应用壳层
  features/redeem/    兑换与商品详情
  features/delivery/ 交付空间和下载
  features/join/      运营账号申请与控制台外链
  lib/api.ts          只包含公开 API

console/src/
  App.tsx             独立控制台壳层
  features/admin/     非标入口 /ops-7f3c 下的运营工作区
  lib/api.ts          运营登录、会话和受保护资源 API
  lib/routes.ts       构建期运营 Web/API 路径
```

约定：

- API 返回业务错误码，前端按错误码显示用户可理解的提示。
- 兑换和下载页面不依赖动画完成才能提交。
- 遵守 `prefers-reduced-motion`，动画降级为淡入或无动画。
- ReactBits 只引入需要的组件，避免为了视觉效果增加大量不可维护依赖。
- anime.js 和 GSAP 不同时实现同一个动效；页面级时间轴优先 GSAP，简单状态过渡优先 anime.js。
- 所有核心流程支持键盘操作、窄屏和高对比度状态。

## 4. Go 服务分层

当前 MVP 采用小型模块化单体，接口和 SQL 暂集中在 `internal/api`，配置与数据库启动位于独立包。后续在业务增大时，再按下方职责拆分：

```text
server/
  cmd/api/              进程入口
  internal/http/        路由、请求解析、响应和中间件
  internal/modules/
    products/           商品和版本
    assets/             文件元数据和对象存储
    redeem/             兑换码和幂等事务
    entitlement/        购买权益
    download/           下载票据和事件
    admin/              管理员操作
    audit/              审计日志
  internal/platform/
    db/                 PostgreSQL 连接和迁移
    cache/              Redis 接口和内存实现
    storage/            S3 接口
    clock/              可测试时间源
```

当前实现尚未拆成上述模块，也没有 Redis cache 接口；本地私有存储使用 `STORAGE_DIR` 配置，MVP 限流保存在进程内。商品支持草稿、实时买家页预览、内容编辑、发布、下架和归档；版本草稿、版本文件绑定、版本发布和更新权益已实现。

## 5. 当前数据库模型

```text
tenants
  id, name, slug, status, created_at, updated_at

operator_users
  id, username, display_name, password_hash, platform_role, status,
  token_version, created_at, updated_at

tenant_memberships
  id, tenant_id, user_id, role, status, created_at, updated_at

operator_sessions
  id, user_id, membership_id, tenant_id, token_version, token_hash, expires_at,
  revoked_at, created_at, last_seen_at

products
  id, tenant_id, owner_user_id, name, description, detail, status(DRAFT/ACTIVE/ARCHIVED), created_at

product_releases
  id, tenant_id, product_id, version, changelog, status(DRAFT/PUBLISHED/ARCHIVED),
  created_by, published_at, created_at, updated_at

product_images
  id, tenant_id, product_id, object_key, alt_text, mime_type, size_bytes, created_at

assets
  id, tenant_id, product_id, release_id, object_key, file_name, mime_type, size_bytes, sha256, created_at

redeem_codes
  id, tenant_id, product_id, release_id, code_hmac, display_suffix, status, external_order_ref,
  expires_at, access_days, max_downloads, redeemed_at, entitlement_id

entitlements
  id, tenant_id, redeem_code_id, product_id, release_id, access_token_hash, status, valid_until,
  max_downloads, downloads_issued, created_at

download_tickets
  id, tenant_id, entitlement_id, asset_id, token_hash, expires_at, created_at

download_events
  id, tenant_id, entitlement_id, asset_id, result, created_at

admin_events
  id, tenant_id, actor, action, resource_type, resource_id, metadata_json, created_at

join_applications
  id, tenant_id, operator_user_id, organization_name, contact_name, email, desired_username, use_case,
  status, reviewer, reviewed_at, created_at, updated_at

platform_settings
  deployment_mode, storefront_url, console_url, api_base_url,
  operator_web_path, operator_api_path, updated_at
```

`redeem_codes.product_id` 标识买家购买的商品，`redeem_codes.release_id` 标识签发时选择的已发布起始版本。兑换流程将两者复制到权益；租户/商品/版本复合外键阻止跨租户、跨商品或跨版本错配。每批兑换码只对应工作台中选中的一个商品和一个起始版本。

商品版本以 SemVer 记录在 `product_releases` 中。交付文件通过 `assets.release_id` 绑定版本草稿；只有商品已发布且该版本至少有一个文件时才能发布。交付查询返回通用文件，以及权益基础版本发布时点之后的已发布版本文件。已被兑换码引用的发布版本不能撤回，避免既有权益失去基线。

实际建表时需要加唯一约束：

- `products.id`
- `redeem_codes.code_hmac`
- `entitlements.redeem_code_id`
- `download_tickets.token_hash`

## 6. Redis 使用决策

### MVP 可不启用

单实例、低并发部署可以使用 PostgreSQL 事务和进程内限流。重启会清空进程内计数，因此安全审计以数据库事件为准，进程内状态仅承担临时限流。

### 满足以下条件再启用

- 多个 API 实例需要共享限流计数；
- 兑换接口遭遇明显的撞码流量；
- 需要跨实例幂等键、短期会话或分布式锁；
- 需要后台队列处理文件扫描、邮件或更新通知。

当前尚未定义 Cache 接口。引入 Redis 时，先定义窄接口并保留进程内实现，避免 Redis 依赖扩散到业务规则。

## 7. 文件上传和下载

上传流程：

```text
管理员上传商品效果图 → 校验 JPEG/PNG、8 MB 上限和像素尺寸
→ 写入 product-images 私有卷路径 → 草稿阶段由鉴权预览路由读取
→ 商品发布后公开图片路由按商品与图片 ID 读取

管理员选择版本草稿并上传单个交付文件 → 服务端校验租户、商品与版本归属
→ 版本必须处于 DRAFT → 限制请求体和文件大小
→ 写临时文件 → 计算 SHA-256 → 写入 private-assets 卷
→ 保存文件元数据与 release_id
```

MVP 上传不执行病毒扫描、归档解包检查或内容审查。

下载流程：

```text
买家请求下载 → 事务锁定权益并检查有效期/额度
→ 校验文件为通用文件或权益起始版本之后的已发布版本
→ 创建 5 分钟票据并计入额度 → 记录下载开始事件
→ 从 private-assets 卷发送文件
```

MVP 使用 `private-assets` Docker 卷保存文件，应用数据库只记录对象键和元数据。生产化阶段应实现 S3 适配、私有桶和签名 URL。

## 8. 安全边界

- MVP 运营工作台使用 bcrypt 数据库密码换取 8 小时 HMAC 会话；服务端逐请求确认账号、成员关系、租户、角色和撤销状态。公开申请的期望密码同样只保存 bcrypt 哈希，待审成员可以登录但权限集合为空。开发首次启动引导 `admin/admin`，生产版本拒绝默认凭证并需接入正式 MFA。
- 管理后台与公开兑换 API 使用不同权限中间件。
- 兑换码只保存 HMAC/哈希；密钥从环境变量或密钥管理系统注入。
- MVP 记录商品创建、文件上传、兑换码签发、凭证状态变更和下载开始事件；业务审计事件已绑定当前租户，退款联动与审计检索仍待实现。
- 兑换、票据签发和上传有进程内按 IP 限流；重启会清空计数，多实例需要共享限流方案。
- 下载事件当前不记录 IP 或 User-Agent。
- Compose 卷持久化数据；备份与恢复流程仍待实现。
- `APP_ENV=production` 会拒绝仓库提供的示例密钥；管理员令牌、兑换 HMAC 和访问令牌密钥需分别生成并保管。

## 9. Docker Compose 边界

当前 Compose 服务：

```text
web       用户端 React 静态资源，默认 :45173
console   运营端 React 静态资源，默认 :46173
api       Go API，默认 :48080
db        PostgreSQL，默认仅回环 :5433
```

`web` 与 `console` 各自包含 Nginx 静态服务和 `/api` 反向代理。两个静态镜像独立构建；用户端镜像不包含运营页面。Compose 当前仅绑定到 loopback，不包含公网 TLS 终止。

MVP 使用 `private-assets` Docker 卷，适合单实例功能验证。后续可增加 Redis 和 S3/MinIO profile；所有对象数据始终保持在 Git 之外。

## 10. 已实现与待实现

| 能力 | MVP 状态 |
| --- | --- |
| 商品创建与列表 | 已实现 |
| 草稿编辑、实时预览、发布/下架/归档 | 已实现 |
| 单文件私有上传和 SHA-256 | 已实现；扫描未实现 |
| 批量兑换码签发和兑换 | 已实现 |
| 可恢复的交付访问凭证 | 已实现 |
| 限额和短时下载票据 | 已实现 |
| 商品版本、版本文件与更新权限 | 已实现；发布版本必须有文件，基础版本被签发后不可撤回 |
| 数据库运营账号与可撤销会话 | 已实现；TOTP/Passkey、密码重置待实现 |
| S3 对象存储和文件扫描 | 待实现 |
| 凭证暂停、恢复、撤销及活动记录 | 已实现 |
| 退款联动、审计检索和备份恢复 | 待实现 |
