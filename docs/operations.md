# 运维手册（Operations）

本文覆盖自托管单实例 NOVER 的部署、迁移、清理、备份与排障。API 契约见[接口说明](api.md)，开发流程见[开发指南](development.md)。

## 1. 运行拓扑

```text
storefront（web/:45173） ─┐
console（console/:46173） ─┼─ Go API :48080 ─ PostgreSQL :5433（仅回环）
                           └─ private-assets 卷（交付文件）
```

Compose 默认只绑定回环地址，公网部署必须在前面加 TLS 反向代理（Caddy/Nginx），并替换全部示例密钥。

## 2. 关键环境变量

| 变量 | 作用 | 生产要求 |
| --- | --- | --- |
| `APP_ENV` | 运行环境 | `production` |
| `POSTGRES_*` / `DATABASE_URL` | 数据库 | 强密码、独立实例、备份 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | 首次引导账号 | 不使用 `admin/admin` |
| `OPERATOR_TOKEN_KEY` | 运营会话签名 | ≥32 字符随机值，独立保管 |
| `REDEEM_HMAC_KEY` | 兑换码 HMAC | ≥32 字符随机值 |
| `ACCESS_TOKEN_KEY` | 交付凭证签名 | ≥32 字符随机值 |
| `ALLOWED_ORIGINS` | CORS 白名单 | 精确域名，逗号分隔 |
| `OPERATOR_WEB_PATH` / `OPERATOR_API_PATH` | 非标路径 | 构建/启动时确定 |
| `STORAGE_DIR` | 私有文件根目录 | 挂载持久卷 |
| `MAX_UPLOAD_BYTES` | 上传上限 | 与反向代理限制一致 |
| `APP_VERSION` / `GIT_COMMIT` / `BUILD_TIME` | 构建元数据 | 发布流水线注入 |

`APP_ENV=production` 会拒绝仓库示例密钥；这只是一道基础保护，仍需自行确认密码强度与密钥轮换。

## 3. 首次部署检查

- [ ] 复制 `.env.example` 为 `.env`，替换数据库、管理员、三组密钥与 Origin。
- [ ] 通过 TLS 反向代理暴露 `web`、`console`、`api`，API 不直接暴露公网。
- [ ] 控制台域名加访问限制（IP allowlist / SSO）。
- [ ] 创建 `private-assets` 与 `postgres-data` 卷的备份策略。
- [ ] 验证 `/healthz`、`/readyz`、`/metrics`、`/api/v1/meta` 返回正常。
- [ ] 登录控制台后在“部署设置 → 系统状态”确认版本、迁移 applied/pending/latest、数据库延迟与存储可写。
- [ ] 运行一次业务冒烟（见开发指南），随后清理夹具。
- [ ] 记录当前版本、迁移状态与备份恢复步骤。

## 4. 数据库迁移

API 启动时自动执行 `server/internal/db/migrations/`：

- `schema_migrations` 记录已应用版本与校验和；
- 历史安装会自动识别并标记基线；
- 校验和不匹配会拒绝启动（防止就地修改旧迁移）；
- 迁移在单事务内执行，失败整体回滚。

也可以手工查看/应用迁移：

```bash
docker compose --profile ops run --rm migrate -status
docker compose --profile ops run --rm migrate -apply
```

发布顺序建议：先备份数据库 → 部署新镜像 → 观察启动日志 `database migrations applied` → 验证 `/readyz` 与 `/api/v1/cp-7f3c/system/status`。完整升级/回滚流程见[升级指南](upgrade.md)。

## 5. 维护清理

```bash
# 干跑：只统计，不改数据
docker compose --profile ops run --rm maintenance

# 执行
docker compose --profile ops run --rm maintenance -apply

# JSON（适合 cron / 采集）
docker compose --profile ops run --rm maintenance -json
```

| 项目 | 行为 |
| --- | --- |
| 过期会话 | 删除超过 1 天过期的会话；删除撤销超过 30 天的会话 |
| 过期下载票据 | 删除过期超过 1 天的票据 |
| 过期兑换码 | `ASSIGNED` 且已过期的标记为 `EXPIRED`（保留历史） |
| 过期权益 | `ACTIVE` 且已过期的标记为 `EXPIRED` |
| 孤儿文件 | 删除无数据库引用且超过 24 小时的文件 |
| 空目录 | 清理已空的租户/资源目录 |

建议每日 cron：

```cron
30 3 * * * cd /srv/nover && docker compose --profile ops run --rm maintenance -apply >> /var/log/nover-maintenance.log 2>&1
```

清理只处理过期与无引用数据；不会删除有效权益、已发布文件或审计事件。

## 6. 备份与恢复

当前尚未提供自动化备份工具，最小可靠流程如下；仓库已提供 `scripts/ops/backup.ps1` 与 `restore.ps1`（见 [scripts/ops/README.md](../scripts/ops/README.md)）作为可重复执行的版本，仍建议定期做恢复演练。

### 备份

```bash
docker compose exec -T db pg_dump -U nover -d nover -Fc > nover-$(date +%F).dump

docker run --rm -v nover_private-assets:/data -v "$PWD":/backup alpine \
  tar czf /backup/private-assets-$(date +%F).tar.gz -C /data .
```

数据库与文件卷必须成对备份，否则会出现元数据存在但文件缺失（或反之）。

### 恢复

```bash
docker compose stop api web console
docker compose exec -T db pg_restore -U nover -d nover --clean --if-exists < nover-YYYY-MM-DD.dump
docker run --rm -v nover_private-assets:/data -v "$PWD":/backup alpine \
  sh -c "rm -rf /data/* && tar xzf /backup/private-assets-YYYY-MM-DD.tar.gz -C /data"
docker compose up -d
```

恢复后验证：登录控制台、公开目录、兑换交付、下载票据、`/api/v1/meta` 版本。建议每季度做一次全新机器演练。

> 注意：卷名以 `docker volume ls` 实际输出为准（通常是 `<项目名>_private-assets`）。

## 7. 日志与可观测性- API 输出结构化 JSON 日志（`log/slog`），每请求一行：`request_id`、`method`、`path`、`route`、`status`、`bytes`、`duration_ms`、`ip`。
- 每个响应带 `X-Request-Id`；错误响应同时带统一错误码。
- `/healthz` 为进程存活，`/readyz` 为数据库就绪，`/api/v1/meta` 返回版本/提交/环境。
- `GET /metrics` 提供 Prometheus 文本指标（请求计数、in-flight、运行时长、构建信息）；公网部署应在反向代理限制访问。
- 健康检查请求不写访问日志，`/metrics` 抓取不计入指标自身。
- 当前没有告警与追踪接入；生产化前应补 Prometheus/OpenTelemetry 或托管日志告警。

## 8. 限流与容量

- 登录、兑换、上传、票据、加入申请均按客户端 IP 做进程内限流（登录 30 次/分钟、MFA 验证 20 次/分钟、邀请/重置创建 30 次/小时、邀请接受 10 次/小时，兑换 12 次/分钟，上传 30 次/分钟，票据 60 次/分钟，加入申请 5 次/小时），超限返回 `429` 与 `Retry-After`。
- 进程内计数在重启后清零，多实例之间不共享；高流量或多副本前需接入 Redis 或网关限流。
- 上传默认上限 500MB，反向代理需同步放宽请求体与超时。
- 单实例目标规模：小型团队与数百订单量级；更大规模需评估对象存储、队列与只读副本。

## 9. 故障排查

| 症状 | 排查 |
| --- | --- |
| 容器启动即退出 | `docker compose logs api`，多为 `DATABASE_URL` 或密钥长度错误 |
| 迁移校验和错误 | 有人修改了已发布迁移；恢复该文件并新增迁移 |
| 控制台 401/403 | 会话过期、成员非 ACTIVE 或权限点不足 |
| 兑换 410 | 码已过期/撤销/已兑换到无效权益 |
| 下载 410/404 | 票据过期、权益暂停、文件被移除或路径非法 |
| 上传失败 | 体积限制、磁盘空间、版本不是草稿 |
| 目录与状态异常 | 检查 `ALLOWED_ORIGINS` 与前端同源代理 |

## 10. 已知运维限制

- 无自动化备份/恢复与租户级导出；
- 无对象存储/S3 适配与上传病毒扫描；
- 无共享限流、告警、追踪与租户级指标；
- 无生产级 RLS 与多租户配额；
- 下载票据在 5 分钟内可重复使用，额度按签发票据计数。
