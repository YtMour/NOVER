# 运营控制台与多租户 SaaS 设计

## 1. 当前边界

截至 2026-11-10，NOVER 的用户端与运营端已经拆成两个 React 工程和两个容器：

| 层面 | 当前状态 | 结论 |
| --- | --- | --- |
| Web 路由 | 用户端 `:45173/`，运营端默认 `:46173/ops-7f3c` | 独立 Origin 和独立应用壳层 |
| API | 公开 API 与默认 `/api/v1/cp-7f3c` 运营 API | 已分权并使用独立认证中间件 |
| 端口 | 用户端 `45173`，控制台 `46173`，API `48080` | CORS 只允许显式 Origin 列表 |
| 构建 | `web/` 与 `console/` 独立 Vite 构建、Dockerfile 和 Nginx 配置 | 用户端产物不包含运营页面和运营 API 客户端 |
| 账号 | 数据库账号、bcrypt、角色权限、可撤销会话 | 邀请、MFA、密码重置待实现 |
| 租户 | 默认租户、成员关系和核心业务表 `tenant_id` 已落地 | 复合外键、RLS、邀请设密与 MFA 未完成，不开放多租户 |

前后端分离与两个前端的运行时隔离已经成立。当前剩余边界是生产独立域名、独立 CI/回滚，以及业务数据的强租户隔离；在这些工作完成前仍按单租户自托管部署。

## 2. 后台项目调研

以下数据通过 GitHub API 于 2026-11-10 查询，Star 会持续变化：

| 项目 | Star 快照 | 许可证 | 值得借鉴的部分 | 不直接二开的原因 |
| --- | ---: | --- | --- | --- |
| [Tabler](https://github.com/tabler/tabler) | 42,033 | MIT | 高密度导航、表格、状态与响应式壳层 | Astro/Bootstrap 视觉与现有 Tailwind 品牌体系冲突 |
| [Ant Design Pro](https://github.com/ant-design/ant-design-pro) | 38,948 | MIT | 企业后台信息架构、权限菜单、全局上下文 | Ant Design 会成为第二套完整设计系统 |
| [refine](https://github.com/refinedev/refine) | 36,020 | MIT | 资源型 CRUD、数据提供器、认证/访问控制抽象 | 当前兑换与交付事务不是标准 CRUD |
| [react-admin](https://github.com/marmelab/react-admin) | 26,987 | MIT | 数据资源、列表筛选、RBAC 与审计视图 | 对现有 API 契约和 UI 结构改造较大 |
| [shadcn-admin](https://github.com/satnaing/shadcn-admin) | 16,635 | MIT | Tailwind 侧栏、命令面板、轻量控制台布局 | 可参考组件组织，但业务能力需要自行实现 |

### 结论

不直接 Fork 完整后台。NOVER 保留现有 React、Tailwind、TanStack Query 与品牌 token，吸收以下共同模式：

- 固定侧栏，按运营、组织、系统分组；
- 顶部常驻显示当前租户、角色与账号；
- 数据总览使用指标与资源分布，而不是装饰型卡片；
- 商品编辑保持表单与买家预览同屏；
- 成员申请、角色、审计和部署设置成为独立资源；
- 移动端侧栏降级为横向可滚动功能导航。

如果后续商品、成员、工单等标准 CRUD 数量快速增加，可在独立控制台应用中评估 refine 的 headless 数据层；不迁移现有兑换和下载事务。

## 3. 目标部署拓扑

### 当前本地与自托管模式

```text
storefront React build :45173 ----+
                                  +---- Go API :48080 ---- PostgreSQL
console React build    :46173 ----+
```

两个前端共享 API 和品牌方向，但不共享路由壳层、静态产物、镜像或主题存储。`web` 只连接公开接口；`console` 才包含运营登录与运营 API 客户端。该模式适合本地开发和单团队自托管。

### 生产独立 Origin

```text
store.example.com      -> storefront static build
console.example.com    -> console static build
api.example.com        -> Go API
assets.example.com     -> private object delivery gateway
```

当前代码已满足前三项，生产部署仍需补齐独立 CI、域名和访问网关：

- CORS 使用显式 Origin 列表；
- 控制台使用独立 CSP、登录限流和会话 Cookie/Token 策略；
- 用户端构建不包含任何运营页面或运营 API 客户端；
- 控制台域名可加 IP allowlist、SSO 或额外访问网关；
- 两个前端具有独立 Dockerfile；CI 构建和回滚版本仍需落地。

### 多租户 SaaS

共享 API 与数据库，但所有租户数据必须有强制作用域。不能通过前端传入的 `tenant_id` 决定权限；租户只能来自已验证会话。

## 4. 账号准入流程

```text
公开提交申请
  -> PENDING
  -> REVIEWING
  -> APPROVED / REJECTED
  -> 创建租户
  -> 创建 OWNER 成员关系
  -> 签发一次性邀请
  -> 用户设置密码并绑定 MFA
  -> ACTIVE
```

公开申请目前只实现到 `APPROVED / REJECTED`。申请表不收集密码；资格通过也不直接获得当前默认空间权限。现有数据库账号只来自部署时的引导管理员，等业务数据租户隔离与邀请流程完成后再开放新账号激活。

后续邀请必须具备：

- 只保存邀请 Token 的哈希；
- 24 至 72 小时有效期；
- 使用后立即失效；
- 设置密码时采用 Argon2id 或 bcrypt；
- OWNER 和平台角色强制 MFA；
- 登录、失败、密码重置、MFA 和邀请操作写入审计日志。

## 5. 权限模型

平台角色与租户角色分开，禁止用一个 `admin` 布尔值同时表达两者。

### 平台角色

| 角色 | 平台设置 | 租户创建/停用 | 查看租户业务数据 | 审计 |
| --- | --- | --- | --- | --- |
| `PLATFORM_OWNER` | 读写 | 允许 | 仅紧急支持流程 | 全部 |
| `PLATFORM_SUPPORT` | 只读 | 禁止 | 临时、留痕、最小范围 | 自身操作 |
| `PLATFORM_AUDITOR` | 只读 | 禁止 | 脱敏只读 | 全部只读 |

### 租户角色

| 能力 | OWNER | ADMIN | EDITOR | FULFILLMENT | ANALYST |
| --- | :---: | :---: | :---: | :---: | :---: |
| 租户设置与成员 | 全部 | 除转移所有权 | - | - | - |
| 商品内容与效果图 | 读写 | 读写 | 读写 | 只读 | 只读 |
| 交付文件 | 读写 | 读写 | 上传 | 读写 | 只读元数据 |
| 发布商品 | 允许 | 允许 | 需审批 | - | - |
| 兑换码签发/停用 | 允许 | 允许 | - | 允许 | 只读 |
| 数据总览 | 全部 | 全部 | 内容相关 | 交付相关 | 脱敏汇总 |
| 删除租户/导出 | 允许 | - | - | - | - |

权限在后端以 `permission` 校验，例如 `product.publish`、`code.issue`、`member.invite`；前端隐藏按钮只是体验优化，不是访问控制。

## 6. 租户数据隔离

### 数据库改造

身份基础表已经新增：

```text
tenants
operator_users
tenant_memberships
operator_sessions
```

身份后续仍需新增：

```text
operator_invitations
role_permissions
```

以下业务表已增加不可为空的 `tenant_id`（历史数据使用默认租户回填）：

```text
products, product_images, assets, redeem_codes, entitlements,
download_tickets, download_events, admin_events
```

约束原则：

- 主查询和索引以 `(tenant_id, id)` 或 `(tenant_id, status, created_at)` 开头；
- 业务外键同时包含 `tenant_id`，阻止跨租户关联；
- 对象键使用 `tenants/{tenant_id}/...` 命名空间；
- 会话声明包含 `user_id`、`tenant_id`、`membership_id`、`role_version`；
- 切换租户签发新会话，不在单次请求中接受任意租户覆盖；
- PostgreSQL RLS 可作为第二层保护，但应用层仍需显式租户条件；
- 每个仓储查询都增加跨租户负向测试。

### 迁移顺序

1. 创建默认租户并把现有数据回填到该租户（已完成启动 schema 迁移）。
2. 所有核心 API 查询切换为会话租户作用域；公开目录与兑换固定默认租户（已完成）。
3. 增加复合索引（已完成），再补充复合外键并校验历史数据。
4. 将迁移从启动幂等脚本拆为版本化迁移，并加入跨租户负向测试。
5. 加入 RLS、对象存储命名空间和租户感知限流。
6. 最后开放邀请和新租户登录。

在第 6 步完成前，加入申请的 `APPROVED` 状态不创建可登录账号。

## 7. 路由与前后端设置

默认配置：

```env
WEB_HOST_PORT=45173
CONSOLE_HOST_PORT=46173
API_HOST_PORT=48080
STOREFRONT_URL=http://localhost:45173
CONSOLE_URL=http://localhost:46173
OPERATOR_WEB_PATH=/ops-7f3c
OPERATOR_API_PATH=cp-7f3c
ALLOWED_ORIGINS=http://localhost:45173,http://localhost:46173
```

运营控制台“部署设置”保存目标拓扑和路由规划，并生成环境变量片段。真实路由由 `web`/`console` 构建参数和 API 启动环境决定，需重新构建/重启后生效。

选择这个边界的原因：

- 避免数据库设置错误导致正在处理请求的路由瞬间消失；
- 便于配置进入基础设施审计、回滚和密钥管理；
- 用户端与控制台拆分后，两者的发布版本可以独立锁定；
- 非标路径只用于降低随机扫描噪声，不能替代认证、RBAC、MFA 和限流。

生产建议使用独立控制台域名，而不是依赖难猜路径。

## 8. 控制台信息架构

```text
运营
  数据总览
  商品创建
  兑换码管理
组织
  成员申请
  成员与权限
  邀请记录（下一阶段）
系统
  部署设置
  审计日志（下一阶段）
  存储与任务（后续）
```

顶部上下文固定展示当前空间和角色。多租户完成后增加租户切换器；平台角色进入租户时必须显示“支持访问”状态和审计原因。

## 9. 分阶段交付计划

### Phase 1：准入与控制面，当前阶段

- 公开“加入我们”申请页与独立控制台外链；
- 申请提交、限流、重复申请约束；
- 后台申请审核；
- 部署拓扑和自定义路由设置；
- 运营侧栏与组织/系统分区；
- Web/API 路由由环境变量配置。

### Phase 2：身份与 RBAC，进行中

- [x] 数据库账号与 bcrypt 密码哈希；
- [x] 平台角色、租户角色和成员关系；
- [x] 后端 permission middleware；
- [x] 当前会话服务端撤销；
- [ ] 一次性邀请设密、MFA、密码重置和全设备会话管理；
- [x] 成员与角色只读管理视图；

### Phase 3：强租户隔离

- 默认租户回填；
- 全业务表 `tenant_id`；
- 复合外键、对象命名空间、RLS；
- 跨租户测试矩阵；
- 申请通过后自动创建租户与 OWNER 邀请。

### Phase 4：独立前端应用，基础拆分已完成

- [x] `web/` 与 `console/` 独立工程、构建、镜像、端口和应用壳层；
- [x] 用户端产物排除运营页面、登录逻辑和运营 API 客户端；
- [x] 独立主题状态、CSP 和显式 CORS Origin；
- [ ] 提取经过验证的 `packages/ui` 与 `packages/contracts`，避免重新耦合应用；
- [ ] 生产独立域名、CI、部署和回滚演练；
- [ ] 控制台 SSO、MFA 与访问网关。

### Phase 5：SaaS 运营

- 套餐、配额、计量和账单；
- 自定义域名、品牌和邮件；
- 租户停用、导出、删除与数据保留；
- 平台运维、任务队列、Redis、对象存储和可观测性。

## 10. 验收门槛

开放新租户登录前必须同时满足：

- 所有业务查询有租户条件且有跨租户负向测试；
- 所有角色以权限点在后端校验；
- 邀请、密码、MFA 和会话撤销闭环完成；
- 对象存储路径和下载票据包含租户作用域；
- 平台支持访问全程留痕；
- 备份恢复可以按租户验证数据完整性；
- storefront 构建不包含 console 代码；
- console 使用独立 Origin、CSP 和发布流程。
