# 技术架构

## 1. 总体结构

MVP 采用模块化单体，避免把兑换、权益和下载拆成多个服务后引入分布式一致性问题。

```text
Browser
  │
  ├── React + TypeScript + Tailwind + local/selected ReactBits components
  │       └── anime.js / GSAP（表现层动效）
  │
  └── HTTPS API
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

建议目录按业务能力组织，而不是按组件类型堆叠：

```text
web/src/
  App.tsx            路由与应用壳层
  features/redeem/  兑换流程
  features/delivery/交付空间和下载
  features/admin/   运营工作台实现（公开路由为 /workspace）
  components/       跨业务复用组件
  lib/              API 客户端、格式化和校验
  styles/           Tailwind 主题和全局样式
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

当前实现尚未拆成上述模块，也没有 Redis cache 接口；本地私有存储使用 `STORAGE_DIR` 配置，MVP 限流保存在进程内。

## 5. 当前数据库模型

```text
products
  id, name, description, status, created_at

assets
  id, product_id, object_key, file_name, mime_type, size_bytes, sha256, created_at

redeem_codes
  id, product_id, code_hmac, display_suffix, status, external_order_ref,
  expires_at, access_days, max_downloads, redeemed_at, entitlement_id

entitlements
  id, redeem_code_id, product_id, access_token_hash, status, valid_until,
  max_downloads, downloads_issued, created_at

download_tickets
  id, entitlement_id, asset_id, token_hash, expires_at, created_at

download_events
  id, entitlement_id, asset_id, result, created_at

admin_events
  id, actor, action, resource_type, resource_id, metadata_json, created_at
```

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
管理员上传单个文件 → 服务端限制请求体和文件大小
→ 写临时文件 → 计算 SHA-256 → 写入 private-assets 卷
→ 保存文件元数据
```

MVP 上传不执行病毒扫描、归档解包检查或内容审查。

下载流程：

```text
买家请求下载 → 事务锁定权益并检查有效期/额度
→ 创建 5 分钟票据并计入额度 → 记录下载开始事件
→ 从 private-assets 卷发送文件
```

MVP 使用 `private-assets` Docker 卷保存文件，应用数据库只记录对象键和元数据。生产化阶段应实现 S3 适配、私有桶和签名 URL。

## 8. 安全边界

- MVP 运营工作台使用账号密码换取 8 小时 HMAC 会话；开发默认 `admin/admin`，生产版本启动时拒绝默认凭证并需接入正式 MFA。
- 管理后台与公开兑换 API 使用不同权限中间件。
- 兑换码只保存 HMAC/哈希；密钥从环境变量或密钥管理系统注入。
- MVP 记录商品创建、文件上传、兑换码签发、凭证状态变更和下载开始事件；退款联动与审计检索仍待实现。
- 兑换、票据签发和上传有进程内按 IP 限流；重启会清空计数，多实例需要共享限流方案。
- 下载事件当前不记录 IP 或 User-Agent。
- Compose 卷持久化数据；备份与恢复流程仍待实现。
- `APP_ENV=production` 会拒绝仓库提供的示例密钥；管理员令牌、兑换 HMAC 和访问令牌密钥需分别生成并保管。

## 9. Docker Compose 边界

当前 Compose 服务：

```text
web       React 静态资源或前端容器
api       Go API
db        PostgreSQL
```

`web` 容器内含 Nginx 静态服务和 `/api` 反向代理；Compose 当前仅绑定到 loopback，不包含公网 TLS 终止。

MVP 使用 `private-assets` Docker 卷，适合单实例功能验证。后续可增加 Redis 和 S3/MinIO profile；所有对象数据始终保持在 Git 之外。

## 10. 已实现与待实现

| 能力 | MVP 状态 |
| --- | --- |
| 商品创建与列表 | 已实现 |
| 单文件私有上传和 SHA-256 | 已实现；扫描未实现 |
| 批量兑换码签发和兑换 | 已实现 |
| 可恢复的交付访问凭证 | 已实现 |
| 限额和短时下载票据 | 已实现 |
| 商品版本和更新权限 | 待实现 |
| 管理员账号、TOTP/Passkey | 待实现 |
| S3 对象存储和文件扫描 | 待实现 |
| 凭证暂停、恢复、撤销及活动记录 | 已实现 |
| 退款联动、审计检索和备份恢复 | 待实现 |
