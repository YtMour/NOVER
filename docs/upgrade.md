# 升级与回滚指南

本文覆盖 NOVER 单实例自托管（Docker Compose）的本地升级、迁移状态检查、回滚与升级前后验证。日常运维见[运维手册](operations.md)，开发命令见[开发指南](development.md)。

## 1. 升级模型

- 代码与镜像：`docker compose build` 重新构建 `api`、`web`、`console`。
- 数据库：API 启动时自动执行 `server/internal/db/migrations/` 中的版本化迁移，`schema_migrations` 记录版本与校验和。
- 兼容性：迁移只追加；历史安装自动识别基线；已应用迁移被修改会拒绝启动。
- 交付文件：保存在 `private-assets` 卷，升级不触碰。
- 配置：`.env` 不随镜像变化；升级前确认新增环境变量（见 `.env.example`）。

## 2. 升级前检查

```powershell
# 当前版本、提交与工作区状态
git log --oneline -1
git status --short

# 镜像/容器状态
docker compose ps

# 迁移状态（只读）
docker compose --profile ops run --rm migrate -status

# 服务健康与版本
curl http://127.0.0.1:48080/readyz
curl http://127.0.0.1:48080/api/v1/meta
```

建议先阅读目标版本的 `CHANGELOG.md` 与对应 `docs/audit-*.md`。

## 3. 一键本地升级

```powershell
# 预览（只打印预检、备份、构建、回滚计划，不修改数据）
.\scripts\ops\upgrade.ps1 -DryRun

# 正式升级：自动备份 → 构建 → 启动 → 等待 readyz → 输出系统状态
.\scripts\ops\upgrade.ps1

# 只升级后端
.\scripts\ops\upgrade.ps1 -BackendOnly

# 跳过备份（不推荐）
.\scripts\ops\upgrade.ps1 -SkipBackup
```

脚本会：

1. 预检仓库、`.env`、Docker、Git 状态；
2. 记录当前 `api/web/console` 镜像并打 `:previous` 标签，写入 `backups/upgrade-<ts>.json`；
3. 调用 `scripts/ops/backup.ps1` 生成数据库 dump、私有卷归档与 manifest；
4. 构建并滚动启动服务，轮询 `/readyz`；
5. 登录读取 `/api/v1/cp-7f3c/system/status`，输出旧版本 → 新版本、迁移 applied/pending/latest 与健康状态。

## 4. 回滚

```powershell
.\scripts\ops\upgrade.ps1 -Rollback -DryRun   # 查看回滚计划
.\scripts\ops\upgrade.ps1 -Rollback           # 使用 :previous 镜像快速回滚
```

- 回滚只回退镜像，**不会自动回退数据库迁移**。若新版本包含破坏性迁移，需同时用 `scripts/ops/restore.ps1` 恢复备份。
- 没有 `:previous` 标签时脚本返回退出码 2。
- 迁移回退策略：优先向前修复（新增迁移），确需回退时使用升级前备份。

## 5. 手工升级（不用脚本）

```powershell
# 1. 备份
.\scripts\ops\backup.ps1 -OutputDir D:\backups\nover

# 2. 构建并启动
docker compose build
docker compose up -d

# 3. 观察启动与迁移日志
docker compose logs -f api

# 4. 验证
curl http://127.0.0.1:48080/readyz
curl http://127.0.0.1:48080/api/v1/meta
```

## 6. 迁移状态与手工执行

```powershell
docker compose --profile ops run --rm migrate -status
docker compose --profile ops run --rm migrate -apply
docker compose --profile ops run --rm migrate -json
```

输出示例：

```text
NOVER migrations: 4 applied, 0 pending
  [applied] 0001 0001_baseline 2026-11-19T05:14:26Z
  [applied] 0002 0002_maintenance_indexes ...
  [applied] 0003 0003_product_profile_and_accounts ...
  [applied] 0004 0004_mfa_and_invitations ...
latest applied: 0004
```

- `-status` 只读，不修改数据库；
- `-apply` 在单事务内应用所有待执行迁移；
- API 启动时也会自动执行同样的流程；
- 控制台“部署设置 → 系统状态”展示 applied/pending/latest 与数据库延迟。

## 7. 升级后验证

```powershell
# 完整验证：单元测试 + 双端构建 + 四套业务冒烟（需要服务已启动）
.\scripts\verify.ps1

# 生成 JSON 报告
.\scripts\verify.ps1 -ReportPath .\tmp\verify-report.json

# 额外运行真实数据库集成测试（需要临时数据库）
$env:TEST_DATABASE_URL = "postgres://nover:<password>@127.0.0.1:5433/nover_integration_test?sslmode=disable"
$env:TEST_STORAGE_DIR = "D:\NOVER\tmp\integration-storage"
.\scripts\verify.ps1 -SkipE2E -Integration
```

手工验收清单：

- [ ] `/readyz` 返回 ready，`/api/v1/meta` 版本与迁移版本符合预期；
- [ ] 控制台登录、两步验证（如启用）、邀请/重置链接可用；
- [ ] 公开目录、商品详情、兑换、交付、下载票据正常；
- [ ] 管理端商品/版本/文件/退款/成员/审计可用；
- [ ] `docker compose ps` 各服务 healthy；
- [ ] 备份文件与 manifest 校验和完整，可被 `restore.ps1 -DryRun` 识别。

## 8. 故障处理

| 现象 | 处理 |
| --- | --- |
| API 启动失败且日志提示迁移校验和不一致 | 有人修改了已发布迁移；恢复文件并新增迁移，不要改写旧迁移 |
| 启动日志 `pending>0` | 执行 `migrate -apply` 或重启 API；检查数据库权限 |
| 升级后页面 502 | `docker compose ps` 查看 api healthy；检查 CORS/`ALLOWED_ORIGINS` |
| 回滚后数据库结构不兼容 | 使用升级前备份执行 `restore.ps1 -Confirm`，再回滚镜像 |
| 备份失败 | 检查磁盘空间、Docker 权限、卷名（`docker volume ls`） |

> 生产升级前请在隔离环境演练一次备份 → 升级 → 回滚 → 恢复全流程。
