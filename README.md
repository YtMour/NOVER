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

当前部署模型仍是**单租户、自托管**：一套部署服务一个卖家或团队。数据库身份、租户成员关系、平台/租户角色、权限中间件和可撤销会话已经建立；商品、效果图、交付资产、兑换码、权益、下载票据、下载事件和审计记录已完成默认租户回填、API 查询作用域以及核心关系复合外键。公开申请会创建独立的待审租户和只读账号：申请人可登录查看账号与申请状态，但只有管理员通过后才获得商品、文件和兑换码权限。RLS、租户域名、邀请设密和 MFA 尚未完成，因此当前实例仍不是可直接开放多商户入驻的平台。

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
- [前后台拆分验收](docs/audit-split.md)：本地容器、浏览器流程、响应式和构建边界证据

## 当前状态

MVP 已包含 Go API、两个独立 React 前端、PostgreSQL schema 和 Docker Compose。`web/` 只包含公开商品、兑换、交付与加入申请；`console/` 是独立的运营控制台构建，包含数据总览、商品创建、兑换码管理、成员申请、成员与权限和部署设置。两端使用不同端口、静态镜像、应用壳层、主题状态和 API 客户端。商品创建采用“内容与实时预览 → 效果图片 → 私有交付文件 → 版本说明 → 发布检查”的连续流程，详细介绍支持 Markdown，并可在保存草稿后直接粘贴剪贴板图片。版本管理支持 SemVer 草稿、更新说明和发布/撤回，公开商品展示最近一次已发布版本。运营登录使用数据库密码哈希、服务端可撤销会话和后端权限点。公开申请会创建只读待审账号和独立租户，通过审核后激活 OWNER 商品权限；MFA、密码重置和更完整的 SaaS 隔离按 [SaaS 设计](docs/admin-saas-design.md) 分阶段实现。

## 本地运行

运行整套容器只需要 Docker Compose。复制开发配置后启动：

```powershell
Copy-Item .env.example .env
docker compose up --build
```

打开用户端 [http://localhost:45173](http://localhost:45173)，或打开独立运营控制台 [http://localhost:46173/ops-7f3c](http://localhost:46173/ops-7f3c)。开发环境账号密码均为 `admin`。Go API 独立暴露在 `http://localhost:48080`。用户端访问 `/ops-7f3c` 只会回到公开商品页面，不包含控制台页面或运营 API 客户端。售卖文件保存在 Docker 私有卷 `private-assets`，数据库保存在 `postgres-data`。

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

用户端 Vite 使用 `45173`，控制台 Vite 使用 `46173`，Go API 使用 `48080`；前端默认直接连接 API 高端口，Vite/Nginx 同时保留代理通道以兼容同源部署。`.env.example` 只用于本地启动；公开部署前替换所有示例密钥、启用 HTTPS 并配置持久化备份。

## API 入口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 进程存活检查 |
| `GET` | `/readyz` | 数据库就绪检查 |
| `GET` | `/api/v1/products` | 首页和商品详情页使用的公开商品目录 |
| `POST` | `/api/v1/join-applications` | 提交组织与运营账号申请 |
| `POST` | `/api/v1/cp-7f3c/login` | 运营账号密码登录，签发短期会话 |
| `GET/POST` | `/api/v1/cp-7f3c/session[...]` | 读取当前身份或撤销当前会话 |
| `GET` | `/api/v1/cp-7f3c/members` | 查看当前租户成员与角色 |
| `GET` | `/api/v1/cp-7f3c/products` | 运营控制台商品列表 |
| `GET` | `/api/v1/cp-7f3c/activity` | 最近兑换、权益和下载活动 |
| `GET/POST` | `/api/v1/cp-7f3c/applications[...]` | 查看与审核加入申请 |
| `GET/PUT` | `/api/v1/cp-7f3c/settings` | 保存部署拓扑和路由规划 |
| `POST` | `/api/v1/cp-7f3c/products` | 创建商品 |
| `PUT` | `/api/v1/cp-7f3c/products/{id}` | 保存商品名称、简介与 Markdown 详细介绍 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/status` | 发布、下架为草稿或归档商品 |
| `GET/POST/DELETE` | `/api/v1/cp-7f3c/products/{id}/images[...]` | 鉴权预览、粘贴/上传和删除商品效果图 |
| `GET/POST` | `/api/v1/cp-7f3c/products/{id}/releases` | 查看和创建商品版本草稿 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/releases/{releaseID}/status` | 发布、撤回或归档商品版本 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/assets` | 上传单个交付文件 |
| `POST` | `/api/v1/cp-7f3c/products/{id}/codes` | 签发绑定当前商品的兑换码 |
| `POST` | `/api/v1/cp-7f3c/codes/{id}/action` | 暂停、恢复或撤销兑换凭证 |
| `POST` | `/api/v1/redeem` | 兑换并取得交付凭证 |
| `GET` | `/api/v1/delivery` | 获取商品和文件清单 |
| `POST` | `/api/v1/assets/{id}/ticket` | 签发 5 分钟下载票据 |
| `GET` | `/api/v1/downloads/{ticket}` | 下载私有文件 |

运营 API 使用登录返回的 `Authorization: Bearer <operator-session>`；会话同时绑定数据库用户、成员关系与租户，退出会在服务端撤销。开发环境首次启动会把 `admin/admin` 初始化为默认空间的 `OWNER + PLATFORM_OWNER`；生产环境必须替换引导凭证和 `OPERATOR_TOKEN_KEY`，并启用 HTTPS、MFA 与密钥轮换。

公开商品详情页使用前端路由 `/products/{productID}`，当前从公开目录读取商品短简介、详细介绍和效果图；真实文件清单、版本和校验信息仍需兑换后查看。效果图为公开素材，限制 JPEG/PNG、单张 8 MB 和图像尺寸；交付文件独立保存在私有目录，新上传文件按 `tenants/{tenant_id}/...` 组织，旧版本对象键仍可读取。后台商品会显示所属租户和负责人，业务查询不会接受前端传入的租户 ID。

兑换码在签发时通过当前商品的工作台选择生成，数据库和兑换权益都绑定同一 `product_id`；复合外键阻止凭证与其他商品的权益错配。不同商品需分别签发兑换码。

`OPERATOR_WEB_PATH` 和 `OPERATOR_API_PATH` 可更改默认控制台与运营 API 路径；更改后需要重新构建 `console` 并重启 API。`CONSOLE_URL` 决定用户端“前往独立控制台”的外部链接。控制台内的部署设置用于保存目标配置和生成环境变量，不会动态改写运行中的路由。运营业务查询的租户来自已验证服务端会话，不接受前端提交的 `tenant_id`；公开目录与兑换目前固定到默认租户。

## 视觉方向

NOVER 采用“极简岛屿 / Minimal Island”：

- 暖白 `#F6F6F3` 作为主背景
- 墨黑 `#17191C` 作为主要文字和品牌色
- 雾蓝 `#A7B9CB` 作为装饰和浅色模块
- 银灰 `#D9DDE2` 作为边界和分隔
- 深雾蓝 `#5B7085` 作为需要更高对比度的链接和主要操作色

字体使用本机可用的开源/系统字体栈：`Noto Sans SC`、`Noto Sans`、`Source Han Sans SC`、`Microsoft YaHei`、`PingFang SC` 和 `Segoe UI` 回退，不下载或打包商业字体；控制台正文和辅助文字已提高到更适合阅读的字号。

视觉应服务于交付流程：兑换、版本、文件、有效期和下载状态必须清晰可见，不为了留白隐藏关键业务信息。
