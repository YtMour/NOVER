# API 接口说明

面向客户端、集成方与 AI 智能体的 HTTP 契约。所有接口使用 JSON，错误统一返回：

```json
{ "code": "invalid_json", "message": "Enter a valid username and password." }
```

响应头包含 `X-Request-Id`，可用于对照 API 访问日志。限流响应返回 `429` 与 `Retry-After`（秒）。

## 1. 基础信息

| 项 | 值 |
| --- | --- |
| Base URL（本地） | `http://127.0.0.1:48080` |
| 公开 API 前缀 | `/api/v1` |
| 运营 API 前缀 | `/api/v1/cp-7f3c`（由 `OPERATOR_API_PATH` 配置） |
| 运营认证 | `Authorization: Bearer <operator-session>`（8 小时，可服务端撤销） |
| 买家交付认证 | `Authorization: Bearer <access_token>`（兑换返回，有效期由兑换码决定） |
| 健康检查 | `GET /healthz`、`GET /readyz`、`GET /api/v1/meta` |
| 内容类型 | `application/json; charset=utf-8`（下载为文件流） |

## 2. 构建与健康

### `GET /api/v1/meta`

公开，无敏感信息，用于确认服务版本。

```json
{
  "name": "nover",
  "version": "0.2.0",
  "commit": "8d75a0f",
  "built_at": "2026-11-19T05:00:00Z",
  "environment": "development"
}
```

### `GET /readyz`

数据库可用时返回 `{"status":"ready"}`，否则 `503 database_unavailable`。

## 3. 公开商品与申请

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/products` | 默认空间公开商品目录 |
| `GET` | `/api/v1/products/{productID}` | 默认空间公开商品详情（仅 `ACTIVE`） |
| `GET` | `/api/v1/spaces/{slug}/products` | 指定商户空间目录；返回 `space_name` |
| `GET` | `/api/v1/spaces/{slug}/products/{productID}` | 指定商户空间商品详情 |
| `GET` | `/api/v1/products/{productID}/images/{imageID}` | 默认空间商品效果图 |
| `GET` | `/api/v1/spaces/{slug}/products/{productID}/images/{imageID}` | 商户空间效果图 |
| `POST` | `/api/v1/join-applications` | 提交加入申请，返回 `{id,status,created_at}` |

目录响应包含商品名称、简介、Markdown 详情、状态、当前发布版本、更新说明、文件数量与效果图列表；不包含交付文件和下载地址。

商品资料字段（列表与详情一致）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `highlights` | `string[]` | 亮点，最多 8 条，每条 ≤120 字符 |
| `license_terms` | `string` | 授权/商用说明，≤2000 字符 |
| `requirements` | `string` | 技术要求，≤2000 字符 |
| `support_period` | `string` | 支持期说明，≤120 字符 |
| `update_policy` | `LIFETIME` / `TILL_DATE` / `CURRENT_ONLY` | 更新策略 |
| `update_until` | `string \| null` | 仅 `TILL_DATE` 时存在，RFC3339 时间 |

详情不存在、已下架或租户空间不可用时返回 `404 product_not_found` / `space_not_found`。

## 4. 兑换与交付

### `POST /api/v1/redeem`

```json
{ "code": "NVR-XXXX-XXXX-XXXX-XXXX-XXXX-XX" }
```

成功 `201`（重复兑换返回 `200` 并恢复同一凭证）：

```json
{ "access_token": "...", "product_id": "..." }
```

常见错误：`invalid_code_format`、`code_not_found`、`code_unavailable`、`code_expired`、`rate_limited`。

### `GET /api/v1/delivery`

请求头：`Authorization: Bearer <access_token>`。

```json
{
  "entitlement_id": "...",
  "valid_until": "2026-12-19T04:00:00Z",
  "max_downloads": 10,
  "downloads_used": 2,
  "update_policy": "TILL_DATE",
  "update_until": "2027-01-31T00:00:00Z",
  "base_version": "1.0.0",
  "product": { "id": "...", "name": "...", "description": "...", "detail": "...", "status": "ACTIVE" },
  "assets": [
    {
      "id": "...",
      "product_id": "...",
      "release_id": "...",
      "release_version": "1.0.0",
      "file_name": "source.zip",
      "mime_type": "application/zip",
      "size_bytes": 123456,
      "sha256": "..."
    }
  ]
}
```

只返回通用文件、权益起始版本及其之后满足更新策略的已发布版本；草稿版本永不出现在交付中。

更新策略在兑换码签发时从商品快照，兑换后复制到权益：

- `LIFETIME`：起始版本 + 之后所有已发布版本；
- `TILL_DATE`：额外包含 `published_at <= update_until` 的已发布版本；
- `CURRENT_ONLY`：仅起始版本（以及历史通用文件）。

起始版本与实际发布文件不受策略影响，始终可见。

### `POST /api/v1/assets/{assetID}/ticket`

请求头同上。成功 `201`：

```json
{ "url": "/api/v1/downloads/<short-lived-token>", "expires_at": "..." }
```

票据默认 5 分钟；每次签发计入权益的下载额度。错误：`delivery_auth_invalid`、`entitlement_expired`、`download_limit_reached`、`asset_not_found`。

### `GET /api/v1/downloads/{token}`

返回文件流（`Content-Disposition: attachment`）。票据过期或权益失效返回 `410 download_link_expired`。

## 5. 运营控制台 API

所有接口需要运营会话，并受权限点控制。租户来自会话，不接受客户端传入 `tenant_id`。

### 5.1 会话与身份

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/v1/cp-7f3c/login` | 公开（限流） | 用户名密码换会话 |
| `GET` | `/api/v1/cp-7f3c/session` | 登录 | 当前账号、租户、角色、权限点 |
| `POST` | `/api/v1/cp-7f3c/session/revoke` | 登录 | 服务端撤销当前会话 |

```bash
curl -s -X POST http://127.0.0.1:48080/api/v1/cp-7f3c/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'
```

### 5.2 商品、版本与文件

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| `GET` | `/products` | `product.read` |
| `POST` | `/products` | `product.write` |
| `PUT` | `/products/{id}` | `product.write` |
| `POST` | `/products/{id}/status` | `product.publish` |
| `GET/POST` | `/products/{id}/images[...]` | `product.read` / `product.write` |
| `GET` | `/products/{id}/releases` | `product.read` |
| `POST` | `/products/{id}/releases` | `product.write` |
| `POST` | `/products/{id}/releases/{releaseID}/status` | `product.publish` |
| `GET` | `/products/{id}/assets` | `product.read` |
| `POST` | `/products/{id}/assets`（multipart：`file`、`release_id`） | `fulfillment.write` |
| `DELETE` | `/products/{id}/assets/{assetID}` | `fulfillment.write`（仅草稿版本） |

`POST /products` 与 `PUT /products/{id}` 除名称、简介、Markdown 详情外，还接收 `highlights`（string[]）、`license_terms`、`requirements`、`support_period`、`update_policy`、`update_until`。`TILL_DATE` 必须同时提供 `update_until`；其他策略的 `update_until` 必须为 `null`。

规则：

- 商品切换为 `ACTIVE` 前必须至少有一个交付文件；
- 版本发布要求商品为 `ACTIVE` 且该版本至少有一个文件；
- 已发布文件不可删除；被兑换码引用的版本不可撤回（`release_in_use`）。

### 5.3 兑换码与活动

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/products/{id}/codes` | `code.issue` | 批量签发，明文仅本次响应返回；同时快照商品更新策略 |
| `POST` | `/codes/{codeID}/action` | `code.manage` | `SUSPEND` / `RESUME` / `REVOKE` / `REFUND`（可带 `reason`） |
| `GET` | `/activity` | `activity.read` | 最近兑换与下载活动 |

退款：`{"action":"REFUND","reason":"可选，≤300 字符"}`。成功返回 `{"status":"REFUNDED"}`，关联权益转为 `REVOKED`；已退款/已撤销/已过期返回 `409 code_not_refundable`。退款动作与原因写入审计事件。

`GET /activity` 支持分页：

```text
?limit=50&offset=0    # limit 1-200，默认 100；offset >= 0
```

```json
{
  "items": [],
  "summary": { "total_codes": 0, "redeemed_codes": 0, "download_events": 0, "products": [] },
  "total": 0,
  "limit": 100,
  "offset": 0,
  "has_more": false
}
```

非法分页参数返回 `400 invalid_pagination`。

### 5.4 组织与系统

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| `GET` | `/applications` | `application.read` |
| `POST` | `/applications/{id}/review` | `application.review` |
| `GET` | `/members` | `member.read` |
| `POST` | `/members` | `member.manage` |
| `PATCH` | `/members/{membershipID}` | `member.manage` |
| `GET/PUT` | `/settings` | `settings.read` / `settings.manage` |
| `GET` | `/sessions` | 登录即可 |
| `POST` | `/sessions/{id}/revoke` | 登录即可（仅自己的会话） |
| `POST` | `/sessions/revoke-others` | 登录即可 |
| `POST` | `/password` | 登录即可 |
| `GET` | `/audit-events` | `audit.read` |

审核动作：`REVIEW`、`APPROVE`、`REJECT`。重复审核已完成状态返回 `409 application_status_conflict`；审核通过激活独立租户 OWNER。

成员管理：

- `POST /members` 请求 `{username, display_name, password, role}`；`role` 为 `ADMIN` / `EDITOR` / `FULFILLMENT` / `ANALYST`，不能创建 `OWNER`；用户名全局唯一。
- `PATCH /members/{membershipID}` 请求 `{role?, status?}`；不能修改自己（`member_self_forbidden`）或 OWNER（`member_owner_protected`）；暂停成员会撤销其会话。
- 账号只能来自运营者创建或加入申请审核；初始密码需线下告知，系统不发送邮件。

账户安全：

- `POST /password` 请求 `{current_password, new_password}`；成功后当前会话保留，其他设备会话失效。
- `GET /sessions` 返回当前账号的有效会话；可单个撤销或一次性撤销其他设备。

审计日志：

```text
GET /audit-events?limit=50&offset=0&action=REFUND&resource_type=redeem_code
```

```json
{
  "items": [
    {
      "id": "...",
      "actor": "admin",
      "action": "REFUND",
      "resource_type": "redeem_code",
      "resource_id": "...",
      "metadata": { "status": "REFUNDED", "reason": "..." },
      "created_at": "..."
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0,
  "has_more": false
}
```

## 6. 状态与错误码

| 类别 | 值 |
| --- | --- |
| 商品状态 | `DRAFT` / `ACTIVE` / `ARCHIVED` |
| 版本状态 | `DRAFT` / `PUBLISHED` / `ARCHIVED` |
| 兑换码状态 | `ASSIGNED` / `REDEEMED` / `SUSPENDED` / `REFUNDED` / `REVOKED` / `EXPIRED` |
| 权益状态 | `ACTIVE` / `SUSPENDED` / `REVOKED` / `EXPIRED` |
| 注册申请 | `PENDING` / `REVIEWING` / `APPROVED` / `REJECTED` |

高频错误码：`operator_auth_required`、`operator_permission_required`、`rate_limited`、`invalid_code_format`、`code_expired`、`code_not_refundable`、`release_in_use`、`asset_immutable`、`delivery_asset_required`、`download_limit_reached`、`invalid_pagination`、`invalid_member`、`member_exists`、`member_not_found`、`member_self_forbidden`、`member_owner_protected`、`invalid_password`、`invalid_current_password`、`session_not_found`、`invalid_audit_filter`。

权限点：`product.read`、`product.write`、`product.publish`、`fulfillment.write`、`code.issue`、`code.manage`、`activity.read`、`member.read`、`member.manage`、`audit.read`、`application.read`、`application.review`、`settings.read`、`settings.manage`。OWNER 与 ADMIN 拥有成员管理与审计日志权限；其他租户角色不具备。

## 7. 集成建议

- 先 `GET /api/v1/meta` 记录版本，再执行业务调用，便于日志对齐。
- 对 `429` 按 `Retry-After` 退避；不要立即重试。
- 兑换接口幂等：同一兑换码重复请求返回同一权益凭证。
- 轮询版本更新时使用 `GET /api/v1/delivery` 的 `product.current_release_version`。
- 文件完整性用交付响应的 `sha256` 校验；下载地址短时有效，不要持久化。
