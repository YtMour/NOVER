# NOVER 备份与恢复脚本（scripts/ops）

面向自托管单实例 NOVER 的**成对备份与恢复**工具：PostgreSQL 逻辑备份 + `private-assets` 私有资产卷归档 + 带 SHA-256 的 manifest。

> 运维背景、部署与排障见 [运维手册](../../docs/operations.md)；视觉规范见 [设计系统](../../docs/design-system.md)。

## 1. 为什么必须成对

数据库保存商品、版本、文件元数据与权益；`private-assets` 卷保存真实交付文件。只备份其中一个会出现“元数据在、文件丢失”或“文件在、无人可访问”的不一致状态，因此 `backup.ps1` 一次生成同一时间戳的三个文件，`restore.ps1` 要求同时提供数据库备份与资产归档。

## 2. 前置条件

- 仓库根目录存在 `docker-compose.yml` 与 `.env`（参考 `.env.example`）。
- Docker Desktop（Windows/macOS）或 Docker Engine + Compose v2，`docker compose version` 可用。
- 当前用户可执行 `docker`，且 Docker Desktop 已允许备份输出目录所在磁盘共享（默认 `D:\` 等本地盘）。
- 首次运行需要能拉取 helper 镜像 `alpine:3.20`（可用 `-HelperImage` 覆盖）。
- Windows PowerShell 5.1 可直接运行；pwsh 7 / Linux pwsh 同样兼容。若被执行策略拦截：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\ops\backup.ps1
```

## 3. 备份

```powershell
# 1) 先演练：只打印命令，不调用 docker、不写文件
.\scripts\ops\backup.ps1 -DryRun

# 2) 正式备份（默认输出到当前目录 ./backups）
.\scripts\ops\backup.ps1

# 3) 指定输出目录 / 时间戳 / compose 文件（适合 cron 与异地盘）
.\scripts\ops\backup.ps1 -OutputDir D:\backup\nover -Timestamp 20261119-134500
.\scripts\ops\backup.ps1 -ComposeFile D:\NOVER\docker-compose.yml -VolumeName nover_private-assets
```

产物（`<timestamp>` 形如 `20261119-134500`）：

| 文件 | 内容 |
| --- | --- |
| `nover-<timestamp>.dump` | `pg_dump -Fc` 自定义格式数据库备份 |
| `private-assets-<timestamp>.tar.gz` | `private-assets` 卷的 gzip 归档 |
| `manifest-<timestamp>.json` | 版本、时间、项目名、卷名、数据库、文件大小与 SHA-256 |

备份脚本只读数据库与卷：不执行 `docker compose down`、不删除卷、不改动文件。任何一步失败都会返回非零并清理本次未完成的输出文件，避免留下不完整的“成对备份”。

### manifest 字段

```json
{
  "manifest_version": 1,
  "tool": "scripts/ops/backup.ps1",
  "tool_version": "0.4.0",
  "created_at": "2026-11-19T05:48:25.0000000Z",
  "repo": { "root": "D:\\NOVER", "git_commit": "3a99673..." },
  "compose": { "file": "docker-compose.yml", "project": "nover" },
  "database": { "container": "db", "user": "nover", "name": "nover", "format": "custom (pg_dump -Fc)", "file": "nover-....dump", "bytes": 123, "sha256": "...." },
  "assets": { "volume": "nover_private-assets", "file": "private-assets-....tar.gz", "bytes": 456, "sha256": "...." },
  "restore": { "script": "scripts/ops/restore.ps1", "hint": "restore.ps1 -DatabaseDump <dump> -AssetsArchive <tar.gz> -Confirm" }
}
```

`-DryRun` 不生成 manifest；正式备份写入后会回读校验 JSON 与 SHA-256。备份文件本身没有加密，请放到受控目录并限制访问权限。

### 常用参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-OutputDir` | `./backups`（相对当前工作目录） | 输出目录；不存在时自动创建 |
| `-ComposeFile` | `<仓库根>/docker-compose.yml` | compose 文件路径 |
| `-ProjectName` | compose 文件所在目录名（小写化） | 用于推导卷名 |
| `-DatabaseContainer` | `db` | 数据库服务名 |
| `-VolumeName` | `<ProjectName>_private-assets` | 卷名，可用 `docker volume ls` 确认 |
| `-DatabaseUser` / `-DatabaseName` | `.env` 的 `POSTGRES_USER` / `POSTGRES_DB`，回退 `nover` | 数据库账号/库名 |
| `-HelperImage` | `alpine:3.20` | 打包卷用的临时镜像 |
| `-Timestamp` | 当前时间 | 复用时间戳以便重跑或对接发布流水线 |
| `-DryRun` | 关 | 只打印计划命令 |

## 4. 恢复（破坏性，必须 `-Confirm`）

```powershell
# 1) 演练：校验格式与 manifest SHA-256，打印将执行的命令
.\scripts\ops\restore.ps1 `
  -DatabaseDump .\backups\nover-20261119-134500.dump `
  -AssetsArchive .\backups\private-assets-20261119-134500.tar.gz -DryRun

# 2) 确认执行：停止 api/web/console → pg_restore --clean --if-exists → 清空并解包卷 → 启动并等待 /readyz
.\scripts\ops\restore.ps1 `
  -DatabaseDump .\backups\nover-20261119-134500.dump `
  -AssetsArchive .\backups\private-assets-20261119-134500.tar.gz -Confirm
```

恢复行为与安全设计：

1. **先校验再动手**：文件存在、数据库备份为 `PGDMP` 格式、资产归档为 gzip、文件大小与 SHA-256 与 manifest 完全一致；任何一项不通过都直接中止，不碰 docker。
2. 未传 `-Confirm`（且未用 `-DryRun`）时只打印拒绝说明并返回退出码 `2`。
3. 执行顺序：`docker compose up -d db`（等待 `pg_isready`）→ `stop api web console` → `pg_restore --clean --if-exists`（从标准输入读 `.dump`）→ `docker run` 清空卷并解包 → `docker compose up -d` → 轮询 `/readyz`。
4. 中途失败会尝试 `docker compose up -d` 拉起服务，并返回非零；此时数据库可能已部分恢复，请按失败输出评估后重跑或回滚到恢复前备份。
5. 脚本不会执行 `docker compose down`、不会删除卷、不会重建镜像。

### 恢复参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-DatabaseDump`（必填） | — | `nover-<timestamp>.dump` |
| `-AssetsArchive`（必填） | — | `private-assets-<timestamp>.tar.gz` |
| `-Manifest` | 自动在同目录查找匹配的 `manifest-*.json` | manifest 路径 |
| `-Confirm` | 关 | 显式确认破坏性操作 |
| `-DryRun` | 关 | 只校验并打印计划 |
| `-SkipManifestCheck` | 关 | 找不到 manifest 时跳过 SHA-256 校验（不推荐） |
| `-SkipReadyCheck` | 关 | 跳过 `/readyz` 轮询，改为人工验证 |
| `-ReadyUrl` | `http://127.0.0.1:48080/readyz` | 就绪探测地址 |
| `-ReadyTimeoutSeconds` | `90` | 就绪等待上限 |
| `-ComposeFile` / `-ProjectName` / `-DatabaseContainer` / `-VolumeName` / `-DatabaseUser` / `-DatabaseName` / `-HelperImage` | 同备份脚本 | 目标环境覆盖项 |

### 退出码

| 退出码 | 含义 |
| --- | --- |
| `0` | 成功（备份完成 / 恢复完成且就绪；DryRun 完成） |
| `1` | 校验失败或执行失败（文件缺失、SHA-256 不匹配、docker 命令失败、等待就绪超时等） |
| `2` | 恢复被拒绝：未显式传入 `-Confirm` |

## 5. 恢复后验证清单

逐项打勾并记录到运维日志：

- [ ] 容器状态：`docker compose ps`，`api`/`web`/`console`/`db` 均为 running。
- [ ] 数据库就绪：`curl.exe -fsS http://127.0.0.1:48080/readyz` 返回 200。
- [ ] 版本元数据：`curl.exe -fsS http://127.0.0.1:48080/api/v1/meta`，版本/提交与环境符合预期。
- [ ] 控制台登录：`http://localhost:46173/ops-7f3c`，账号、商品、兑换码、成员与审计记录可见。
- [ ] 公开目录：`http://localhost:45173` 商品列表、图片与详情正常。
- [ ] 兑换 → 交付 → 下载票据全链路抽查一次（使用合成兑换码或测试订单）。
- [ ] 下载文件 SHA-256 与交付页显示一致；`DRAFT` 版本不出现。
- [ ] 日志：`docker compose logs api --tail 100` 无迁移校验、数据库连接或权限错误。
- [ ] 记录恢复时间、manifest 路径、执行人与验证结果。

## 6. 风险与限制

- **成对恢复**：数据库与卷必须来自同一次备份；跨时间点混用会产生文件/元数据不一致。
- **角色与扩展**：`pg_dump` 只备份数据库内容，不包含 PostgreSQL 角色/密码；恢复到全新实例前先创建 `POSTGRES_USER` 等账号。
- **停机窗口**：恢复期间 `api/web/console` 停止服务；请选择低峰期并让外部反向代理返回维护页。
- **覆盖语义**：`pg_restore --clean --if-exists` 会删除并重建现有对象；卷内容会被清空。执行前必须确认当前环境与目标环境，且已有可回滚备份。
- **备份安全**：`.dump` 与 `.tar.gz` 含业务数据与交付文件，可能包含个人信息；请加密存储、限制权限并避免提交到 Git（`.tar.gz` 已被 `.gitignore` 忽略，但 `.dump` / `manifest.json` 未被忽略，建议 `-OutputDir` 指向仓库外）。
- **Windows 路径**：脚本会把绑定挂载路径转换为正斜杠（如 `D:/NOVER/backups`）；如遇文件共享错误，在 Docker Desktop → Settings → Resources → File sharing 中允许对应磁盘。
- **未验证项**：本仓库当前没有持续集成环境实际执行备份/恢复（会停止服务、覆盖数据）；首次上线前请在隔离环境完整演练一次，并记录耗时与恢复点目标（RPO/RTO）。

## 7. 演练建议

- 每季度至少一次：在独立目录/独立机器上恢复最近一次备份，走完第 5 节清单。
- 每次发布数据库迁移前先备份；迁移失败时用同一时间戳的数据库备份回滚。
- 备份保留策略建议：每日 7 份、每周 4 份、每月 3 份，异地保存；定期校验 manifest 中 SHA-256 是否仍匹配。
- cron 示例（Linux 主机，按实际路径调整）：

```cron
30 3 * * * cd /srv/nover && pwsh -NoProfile -File ./scripts/ops/backup.ps1 -OutputDir /srv/backup/nover >> /var/log/nover-backup.log 2>&1
```

## 8. 本地升级与回滚

`upgrade.ps1` 面向单机 Docker Compose 部署，把「记录当前镜像 → 备份 → 构建启动 → 就绪/迁移摘要」串成一次可回滚的操作，并提供基于 `:previous` 镜像标签的快速回滚。完整的升级模型、迁移检查与升级后验证清单见 [升级与回滚指南](../../docs/upgrade.md)；本节聚焦脚本用法。

### 8.1 前置条件

- 在仓库根目录执行，存在 `docker-compose.yml`、`.env`（参考 `.env.example`）；`docker compose version` 可用。
- `api` / `web` / `console` 已由 `docker compose up -d` 运行（升级只记录**运行中容器**的镜像；服务未运行会警告并跳过该服务的 `:previous` 标签）。
- 输出目录所在磁盘空间足够（默认 `./backups`，数据库 dump + 资产卷归档 + manifest）。
- 升级前先执行 `-DryRun` 演练；建议同时确认 `docker compose --profile ops run --rm migrate -status` 无异常 pending 之外的差异。
- Windows PowerShell 5.1 可直接运行（UTF-8 BOM + LF），pwsh 7 / Linux pwsh 同样兼容。被执行策略拦截时：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\ops\upgrade.ps1 -DryRun
```

### 8.2 升级步骤

1. **预检**：仓库根、compose 文件、`.env`、docker、git HEAD 与工作区状态（dirty 只警告），读取 `.env` 的 `APP_VERSION`（缺省显示 `dev`）。
2. **记录并打标**：读取 `api`/`web`/`console` 容器的镜像 ID 与镜像引用，写入 `<OutputDir>/upgrade-<timestamp>.json`，并给镜像打 `nover-api:previous`、`nover-web:previous`、`nover-console:previous` 标签（只新增标签，不删除任何镜像）。
3. **备份**：默认调用 `scripts/ops/backup.ps1 -OutputDir <OutputDir> -ComposeFile <compose>`；备份失败立即中止升级（不会继续构建）。
4. **构建启动**：默认 `docker compose build` + `docker compose up -d`；`-BackendOnly` 时只执行 `docker compose up -d --build api`。
5. **等待就绪**：轮询 `/readyz`（默认最多 180 秒，`-TimeoutSeconds` 可调）；超时即失败。
6. **摘要**：读取 `/api/v1/meta` 与运营 `/system/status`（用 `.env` 的 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 登录；登录失败、管理员启用 MFA 等情况只警告），输出「旧版本 → 新版本、迁移 applied/pending/latest、数据库与存储健康」摘要。

升级记录 `upgrade-<timestamp>.json`（JSON，UTF-8 无 BOM）包含：模式（`upgrade-full` / `upgrade-backend`）、结果（`in_progress` / `success` / `failed`）、仓库提交与工作区 dirty、compose 项目、`APP_VERSION`、各服务 `container_id` / `image_id` / `image_ref` / `previous_tag`、升级前后 `meta` 与 `system_status`、`/readyz` 健康结果与错误信息。失败时同一文件会标记 `failed` 并写入 `error`，便于事后排查；回滚记录为独立的 `rollback-<timestamp>.json`（`mode: rollback`）。

### 8.3 升级命令

```powershell
# 1) 演练：只打印预检结果与计划命令，不调用 docker、不写文件
.\scripts\ops\upgrade.ps1 -DryRun

# 2) 标准升级：备份 → 构建 api/web/console → up -d → 等待就绪 → 摘要
.\scripts\ops\upgrade.ps1

# 3) 只重建后端（前端未改动时更快，只处理 api 镜像与容器）
.\scripts\ops\upgrade.ps1 -BackendOnly

# 4) 已手工完成备份时跳过备份（不推荐；失败时仅剩 :previous 镜像可回滚）
.\scripts\ops\upgrade.ps1 -SkipBackup

# 5) 自定义超时、输出目录与 compose 文件
.\scripts\ops\upgrade.ps1 -TimeoutSeconds 300 -OutputDir D:\backups\nover
.\scripts\ops\upgrade.ps1 -ComposeFile D:\NOVER\docker-compose.yml
```

### 8.4 回滚

```powershell
# 1) 演练：打印将检查/重打的标签与启动命令
.\scripts\ops\upgrade.ps1 -DryRun -Rollback

# 2) 回滚：把 :previous 指回升级记录中的镜像引用 → docker compose up -d（不 build）→ 等待就绪 → 摘要
.\scripts\ops\upgrade.ps1 -Rollback

# 3) 只回滚后端 api 镜像
.\scripts\ops\upgrade.ps1 -Rollback -BackendOnly
```

回滚要点：

- 目标镜像引用优先取 `<OutputDir>` 中**最新**的 `upgrade-*.json` 记录（例如 `nover-api:latest`），没有记录时回退到 `<project>-<service>:latest` 约定。
- 不执行 `docker compose build`，不删除镜像；找不到任何 `:previous` 镜像时返回退出码 `2` 并提示。
- 回滚记录写入 `<OutputDir>/rollback-<timestamp>.json`（`mode: rollback`，含 `source_record`、重打标签列表、回滚前后版本与健康结果）。
- **回滚只恢复镜像，不回退数据库迁移与已写入数据**；需要回退数据时使用 `restore.ps1` 恢复同一时间点的成对备份。

### 8.5 备份策略

- 升级默认先备份：数据库 dump、`private-assets` 卷归档与 manifest 写入 `<OutputDir>`；备份失败会中止升级，不会进入构建阶段。
- `-SkipBackup` 只用于已确认存在可恢复备份或纯排障场景；脚本会在摘要与记录中标注 `backup.skipped = true`。
- 建议保留最近 3～7 次升级前备份并异地保存；发布数据库迁移前必须备份，迁移失败时用同一时间点的数据库备份恢复。
- 备份与升级记录都包含业务数据/路径信息，请限制目录权限、避免提交到 Git（`backups/` 已加入 `.gitignore`）。

### 8.6 失败处理

- **升级失败**：脚本不会执行 `docker compose down`，不会删除卷或镜像。按顺序排查：`docker compose ps` → `docker compose logs api --tail 200` → `/readyz` 超时原因（数据库未就绪、迁移失败、端口占用）→ 必要时 `.\scripts\ops\upgrade.ps1 -Rollback`。
- **就绪超时**：确认 `.env` 中 `POSTGRES_PASSWORD` / `OPERATOR_TOKEN_KEY` / `REDEEM_HMAC_KEY` / `ACCESS_TOKEN_KEY` 未缺失；查看 `docker compose logs db api --tail 200`。
- **回滚后未生效**：若容器未被重建，执行 `docker compose up -d --force-recreate`（仅重打镜像标签后需要时使用，不会重新构建）。
- **数据库迁移失败**：先 `restore.ps1` 恢复升级前备份，再修复迁移；不要修改已应用迁移文件（checksum 不匹配会拒绝启动）。
- **记录排查**：`upgrade-<timestamp>.json` / `rollback-<timestamp>.json` 中的 `result`、`error`、`before`、`after` 与 `health` 字段可直接用于复盘。

### 8.7 迁移状态查看

```bash
# 只读：查看已应用 / 待执行迁移与最新版本（-status 为默认行为）
docker compose --profile ops run --rm migrate -status
docker compose --profile ops run --rm migrate -status -json

# 应用待执行迁移（执行前先备份；API 启动时也会自动执行迁移）
docker compose --profile ops run --rm migrate -apply
```

本机没有 Docker 时可在 `server/` 目录直接运行（需要 `DATABASE_URL` 指向目标数据库）：

```bash
cd server
go run ./cmd/migrate -status
```

### 8.8 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-DryRun` | 关 | 只打印预检结果与计划命令，不调用 docker、不写文件 |
| `-Rollback` | 关 | 用 `:previous` 镜像回滚（retag + `up -d`，不 build） |
| `-SkipBackup` | 关 | 跳过升级前备份（仅排障/已手工备份时使用） |
| `-BackendOnly` | 关 | 只处理 `api` 服务（升级时 `up -d --build api`，回滚时只回滚 api） |
| `-TimeoutSeconds` | `180` | 等待 `/readyz` 的上限（1–3600 秒） |
| `-OutputDir` | `./backups`（相对当前工作目录） | 升级/回滚记录与备份的输出目录；不存在时自动创建（`-DryRun` 只打印不创建） |
| `-ComposeFile` | `<仓库根>/docker-compose.yml` | compose 文件路径；同时决定项目名与 `.env` 位置 |

### 8.9 退出码

| 退出码 | 含义 |
| --- | --- |
| `0` | 升级/回滚成功，或 `-DryRun` 完成 |
| `1` | 预检、备份、构建、启动或就绪检查失败 |
| `2` | `-Rollback` 找不到任何 `:previous` 镜像 |

### 8.10 风险与未验证项

- **未在 CI 中执行真实升级/回滚**：仓库验证只覆盖 PowerShell Parser 与 `-DryRun`；首次上线前请在隔离环境完整演练一次（含 `-Rollback`），并记录耗时与回滚点。
- **`-DryRun` 不调用 docker**：因此不会校验镜像/容器是否真实存在；它与实际执行共用同一份命令定义，但仍建议先人工 `docker compose ps` 确认栈在运行。
- **system/status 查询限制**：需要 `.env` 管理员凭据；管理员启用 MFA 或未配置凭据时脚本跳过该步并只输出 `/api/v1/meta` 与警告。
- **镜像命名约定**：`:previous` 标签固定为 `nover-api:previous` / `nover-web:previous` / `nover-console:previous`；自定义 compose 项目名或手工改镜像名时，回滚目标从升级记录读取，必要时用 `-ComposeFile` 与记录保持一致。
- **Windows 与 Linux 差异**：脚本用法一致；Linux 上 cron 调用示例可参考第 7 节备份 cron，把脚本名换为 `upgrade.ps1`（建议在低峰期执行）。
