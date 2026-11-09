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

## 当前状态

MVP 已包含 Go API、React 页面、PostgreSQL schema 和 Docker Compose。首页现在包含品牌介绍、公开测试商品目录、购买到领取的三步说明和兑换入口；商品卡可直接跳转到兑换区。运营工作台可创建商品、上传私有文件、签发兑换码、兑换并下载；版本管理、文件扫描、S3 适配、退款运营和多管理员账号系统仍在后续范围。

## 本地运行

运行整套容器只需要 Docker Compose。复制开发配置后启动：

```powershell
Copy-Item .env.example .env
docker compose up --build
```

打开 [http://localhost:5173](http://localhost:5173)。首页会展示品牌介绍和测试商品目录；进入“运营工作台”登录，开发环境账号密码均为 `admin`，然后可以创建商品、上传文件并签发兑换码。售卖文件保存在 Docker 私有卷 `private-assets`，数据库保存在 `postgres-data`。

本地开发前端和 API 时，先启动数据库：

```powershell
docker compose up -d db
Get-Content .env | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process') } }
$env:DATABASE_URL = "postgres://{0}:{1}@localhost:{2}/{3}?sslmode=disable" -f $env:POSTGRES_USER, $env:POSTGRES_PASSWORD, $env:DB_HOST_PORT, $env:POSTGRES_DB
$env:STORAGE_DIR = '.\runtime-data\files'
cd server
go run ./cmd/api
```

另开终端启动前端：

```powershell
cd web
npm ci
npm run dev
```

本地运行需要 Go 1.24+、Node.js 24+ 和 npm。PowerShell 从仓库根目录启动；Go API 使用 `.env` 中的管理员令牌和密钥。

Vite 将 `/api` 请求代理至 `http://localhost:8080`。`.env.example` 只用于本地启动；公开部署前替换所有示例密钥、启用 HTTPS 并配置持久化备份。

## API 入口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 进程存活检查 |
| `GET` | `/readyz` | 数据库就绪检查 |
| `GET` | `/api/v1/products` | 首页公开商品目录 |
| `POST` | `/api/v1/ops/login` | 运营账号密码登录，签发短期会话 |
| `GET` | `/api/v1/ops/products` | 运营工作台商品列表 |
| `GET` | `/api/v1/ops/activity` | 最近兑换、权益和下载活动 |
| `POST` | `/api/v1/ops/products` | 创建商品 |
| `POST` | `/api/v1/ops/products/{id}/assets` | 上传单个交付文件 |
| `POST` | `/api/v1/ops/products/{id}/codes` | 签发兑换码，明文仅返回本次 |
| `POST` | `/api/v1/ops/codes/{id}/action` | 暂停、恢复或撤销兑换凭证 |
| `POST` | `/api/v1/redeem` | 兑换并取得交付凭证 |
| `GET` | `/api/v1/delivery` | 获取商品和文件清单 |
| `POST` | `/api/v1/assets/{id}/ticket` | 签发 5 分钟下载票据 |
| `GET` | `/api/v1/downloads/{ticket}` | 下载私有文件 |

运营 API 使用登录返回的 `Authorization: Bearer <operator-session>`；交付 API 使用兑换返回的 Bearer 凭证。开发环境使用 `admin/admin`，生产环境必须替换账号、密码和 `OPERATOR_TOKEN_KEY`，并启用 HTTPS。

## 视觉方向

NOVER 采用“极简岛屿 / Minimal Island”：

- 暖白 `#F6F6F3` 作为主背景
- 墨黑 `#17191C` 作为主要文字和品牌色
- 雾蓝 `#A7B9CB` 作为装饰和浅色模块
- 银灰 `#D9DDE2` 作为边界和分隔
- 深雾蓝 `#5B7085` 作为需要更高对比度的链接和主要操作色

视觉应服务于交付流程：兑换、版本、文件、有效期和下载状态必须清晰可见，不为了留白隐藏关键业务信息。
