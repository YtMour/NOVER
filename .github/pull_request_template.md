# 变更摘要

<!-- 用 1–3 句话说明改了什么、为什么改。 -->

## 变更类型

- [ ] 新功能（feat）
- [ ] 修复（fix）
- [ ] 重构 / 清理（refactor / chore）
- [ ] 文档（docs）
- [ ] 测试 / CI（test / ci）
- [ ] 数据库迁移（migration）

## DX 检查清单

- [ ] 已阅读 `AGENTS.md`，未修改 `server/`、`web/`、`console/` 之外不应触碰的边界（如 `.env*`、售卖文件）。
- [ ] 未提交 `.env`、密钥、真实订单/用户数据或实际售卖的数字文件。
- [ ] 后端：`cd server && go vet ./... && go test ./...` 通过。
- [ ] 前端：`web/` 与 `console/` 均通过 `npm run build`（Windows 使用 `npm.cmd`）。
- [ ] 已运行 `.\scripts\verify.ps1 -SkipE2E`（或说明为何不适用）。
- [ ] 涉及兑换、权益、交付、下载额度、租户隔离的改动，已运行 `scripts/qa/run-all.ps1`（需要 `docker compose up -d --build`）。
- [ ] API 变更已同步：`web/` 与 `console/` 的类型/校验、`README.md` API 表、`docs/architecture.md`、冒烟脚本断言。
- [ ] 数据库变更只新增 `server/internal/db/migrations/NNNN_*.sql`，未改写已应用的迁移。
- [ ] 关键不变量未被破坏：租户来自服务端会话/凭证、已发布文件不可变、草稿不可交付、兑换码绑定已发布版本。
- [ ] 新增/更新了对应测试（Go `*_test.go` 或冒烟断言），没有只靠手工验证。
- [ ] 文档已更新（`docs/`、`AGENTS.md`、`README.md` 视情况）。

## 测试证据

<!-- 粘贴关键命令输出：go test 摘要、npm build 摘要、冒烟脚本 pass JSON。 -->

```text

```

## 风险与回滚

<!-- 影响范围、部署注意事项（是否需重建容器/执行迁移）、回滚方式。 -->
