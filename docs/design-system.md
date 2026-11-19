# NOVER 视觉设计系统与 UX/可访问性规范

适用范围：`web/`（用户端）与 `console/`（运营控制台）。本文是**可执行规范**：新增页面、组件与视觉改版必须符合；存量差异按 §10 回修清单逐步收敛，不要求一次性重写。

实现基线：两端均为 Tailwind 4 + 自定义 CSS，token 定义在 `web/src/styles.css` 与 `console/src/styles.css` 的 `:root`，深色主题通过 `:root[data-theme=dark]` 覆盖同名变量。**组件样式只允许引用 var(--token)，不得写死十六进制颜色**（`#0000` 透明除外）。

## 1. 品牌定位与原则

“极简岛屿 / Minimal Island”：暖白画布、墨黑结构、雾蓝点缀，像一座安静、可信的岛屿。视觉服务于交付流程，不为了留白隐藏关键业务信息。

1. **内容优先**：兑换、版本、文件、有效期、下载额度、状态必须一屏可见；装饰不得抢占层级。
2. **低噪音**：一个视图只有一个主行动；同一屏最多两个强调色（墨黑主操作 + 雾蓝信息）。
3. **诚实状态**：加载、空、错误、无权限、已过期必须是不同状态，不能都显示为空白。
4. **可恢复**：危险操作需说明后果；不可撤销的操作（退款、停用、恢复、删除）必须二次确认。
5. **无障碍即完成标准**：不满足 §8 的页面不算完成。

## 2. 颜色 token

| Token | 浅色 | 深色 | 用途与对比度（实测，WCAG） |
| --- | --- | --- | --- |
| `--ink` | `#17191C` | `#E7EBEC` | 主文字、主按钮背景、结构线；暖白上 16.27:1 |
| `--warm` | `#F6F6F3` | `#101415` | 页面画布 |
| `--white` | `#FFFFFF` | `#191E20` | 卡片/输入面；深色下为抬高面 |
| `--muted` | `#69717A` | `#A1ABAD` | 辅助文字；暖白 4.57:1，仅用于 ≥12px 信息 |
| `--blue` | `#A7B9CB` | `#8EA7B9` | 装饰、浅色模块；暖白仅 1.86:1，**禁止用于文字** |
| `--blue-action` | `#5B7085` | `#9BB5C6` | 链接、下载/信息操作；暖白 4.73:1 |
| `--silver` | `#D9DDE2` | `#394246` | 分隔、弱边框 |
| `--line` | `#DFE2E3` | `#2D3538` | 常规边框/分隔线（装饰性） |
| `--border-strong` | `#C9D0D2` | `#4B575B` | 悬停边框、分组边界 |
| `--border-control` | `#7C8B91` | `#5E6E74` | **新增**：输入/控件边界（3:1 以上） |
| `--green` / `--green-soft` | `#2C7255` / `#E7F1EB` | `#7EB293` / `#1B2C23` | 成功、生效、可用 |
| `--coral` / `--coral-soft` | `#B85E4D` / `#F5EAE6` | `#D48675` / `#34221F` | 错误、危险、撤销；文字用 `#8F493B`(5.59:1) |
| `--amber` / `--amber-soft` | `#89641E` / `#F4EFDF` | `#D0AE66` / `#30291A` | 警告、草稿、即将过期 |
| `--surface-muted` | `#F1F3F2` | `#151A1C` | 次级面、列表悬停 |
| `--surface-raised` | `#FBFCFB` | `#1D2325` | 输入底、浮层底 |
| `--shadow-color` | `#1F282C17` | `#00000047` | 阴影基色 |
| `--topbar-bg` | `#F6F6F3E0` | `#101415E0` | 毛玻璃顶栏 |
| `--skeleton-base` / `--skeleton-sheen` | `#E8EDEE` / `#F5F8F8` | `#212A2D` / `#2B3538` | 骨架屏底色与微光 |
| `--toast-surface` / `--toast-shadow`（用户端） | `#FFFFFF` / `0 18px 44px #1F282C24` | `#1D2426` / `0 18px 44px #00000059` | Toast 面与阴影；控制台复用 `--white` / `--shadow-color` |

规则：

- 正文文字 ≥ 4.5:1；≥18.66px 粗体或 ≥24px 文字 ≥ 3:1；控件边界与状态图形 ≥ 3:1。
- 对比不达标的历史灰色（如 `#8A9296` 约 2.92:1、`#9AA4A9` 约 2.35:1）只能做装饰/分隔，不得承载信息。
- 状态不得只靠颜色表达：必须同时有文字（如 `已退款`）或图标；`status-pill` 需要可读文案。
- 阴影在深色主题下弱化，改用 `1px` 边框 + `--surface-raised` 抬升；不用彩色阴影。
- 新增颜色先加 token 到两端 `:root` 与深色覆盖，再使用；禁止页面内硬编码。

## 3. 字体与排版

字体栈（本机可用、不打包商业字体）：

```css
--font-sans: "Noto Sans SC", "Noto Sans", "Source Han Sans SC", "Microsoft YaHei", "PingFang SC", "Segoe UI", sans-serif;
--font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
```

| 层级 | 字号 / 行高 / 字重 | 典型类名 |
| --- | --- | --- |
| 品牌标题 | `clamp(42px,5vw,61px)` / 1.12 / 580 | `.home-hero h1`、`.redeem-copy h1` |
| 页面主标题 | `clamp(38px,5vw,58px)` / 1.1 / 600 | `.product-detail-title` |
| 页面标题（控制台） | 29px / 1.25 / 620 | `.page-title` |
| 区块标题 | 25px / 1.3 / 620；面板 17px / 630 | `.section-heading-row h2`、`.panel-heading h2` |
| 卡片标题 | 15px / 1.4 / 640 | `.product-card h3`、`.detail-point h3` |
| 正文 | 14–15px / 1.8–1.9 / 400 | `.lead-copy`、`.product-detail-description` |
| 紧凑正文（控制台） | 12–13px / 1.6–1.75 | `.stack-form`、列表行 |
| 元信息 | 12px / 1.5 / 400，`--muted` | `.field-note`、`.asset-meta` |
| Kicker/眉题 | 11px / 700 / `letter-spacing:.12em` / 大写 | `.eyebrow`、`.section-kicker` |
| 等宽/编号 | 10–13px，`--font-mono`，哈希/编号启用 tabular | `.product-index`、`.hash-copy` |

规则：正文最小 12px（控制台）/13px（用户端优先）；<12px 只允许眉题、索引等非关键信息；中文行高 ≥1.6，长段落 ≤ 约 70 个汉字宽；金额、下载次数、文件大小数字右对齐并等宽；标题层级不得跳级（h1→h2→h3）。

## 4. 间距、圆角、阴影、边框

- 间距基准 4px：`4 / 8 / 12 / 16 / 24 / 32 / 48 / 64 / 96`。组件内边距 12–18px，卡片间距 13px，区块上下 54–72px，控制台页面区块 24–34px。
- 圆角：`4`（输入/标签）→ `6`（卡片/面板/按钮）→ `8`（搜索框/列表行）→ `10–12`（pill/徽标）→ `14`（Hero/浮层）→ `999`（胶囊）。
- 阴影三级：e1 `0 1px 2px var(--shadow-color)`（控件）；e2 `0 8px 30px #20262C0A`（面板）/ `0 7px 18px`（悬停）；e3 `0 20px 55px var(--shadow-color)`（浮层/对话框）。页面区块默认无阴影。
- 边框：常规 `1px solid var(--line)`；控件 `--border-control`；强调/聚焦 `2px` 实线；品牌岛形 glyph 可用 1.5px。禁止用双线、渐变描边表达状态。
- 布局宽度：用户端内容 `min(1100px, 100% - 64px)`；控制台 `100%` + 24px 内边距。栅格间距 13px；≥1051px 桌面、761–1050px 收为两列/折叠侧栏、≤760px 单列、≤420px 紧凑间距（16px）。

## 5. 组件规范

### 按钮
- `.button` 基类：`min-height 42px`（移动端 44px）、左右 `14px`、圆角 6、字号 12/620、`transition .25s`；悬停 `translateY(-1px)` + e2 阴影，按下 `scale(.985)`，禁用 `opacity .65` 且 `cursor:wait`，加载中保持宽度并用 `aria-busy`，防重复提交。
- `.button-primary`：`--ink` 底 + `--white` 字，页面的唯一下一步。`.button-secondary`：白底 + `--line` 边框 + `--ink` 字，用于次要/取消。`.button-download`：`--blue-action` 底，用于下载。`.text-action`/`.hash-copy`：纯文字操作，悬停变 `--ink`，不得仅靠下划线辨识。
- **新增组件规范** `.button-danger`（待实现）：白底 `--coral` 文字（`#8F493B` 满足 5.59:1），悬停 `--coral-soft`；破坏性确认使用实心 `#9E4636` + 白字（6.21:1）。控制台行内危险操作沿用现有 `.icon-button--danger`（悬停 `#A74E3C`）。退款/停用/删除必须二次确认并说明影响。
- `.icon-button`/`.theme-toggle`：视觉 38px，移动端命中区扩到 44px；图标 `aria-hidden="true"`，按钮必须有 `aria-label` 或可见文字。

### 输入与表单
- `.stack-form` / `.redeem-form`，`label 12/620` + `margin-bottom 8px`；输入 `min-height 42–48px`、`padding 12px 13px`、圆角 4、`--surface-raised` 底、`--border-control` 边框；聚焦 `border-color:#7891A7` + `box-shadow:0 0 0 3px #A7B9CB35`（3.03:1 控件对比）。
- 校验：错误用 `.status-message--error`（`role="alert"`、`aria-live="assertive"`）+ `aria-invalid="true"` + `aria-describedby` 指向错误文本；成功/说明用 `--success/--info` 与 `aria-live="polite"`。错误文案必须说明原因与下一步，不能只写“操作失败”。
- 必填用文字“必填/可选”标记，不只靠红色；`autocomplete` 正确填写；placeholder 不能替代 label；兑换码输入使用 mono 字体并按 `-` 分组展示。

### 卡片与面板
- `.surface-panel`：`--white` 底、`--line` 边框、圆角 6、e2 阴影、内边距 24–30px；`.product-card`：`min-height 205px`、内边距 18px、悬停 `translateY(-2px)` + e2、`focus-visible` 3px 雾蓝环；整卡只允许一个链接，禁止卡内嵌套按钮。
- `.admin-section` / `.product-studio`：控制台分区带 kicker 标题与右侧操作；用于 2–4 组同质内容，超过 6 项改用列表/表格。
- `.tool-icon` 38×38、圆角 5，用于区块标识；同一区块只用一种语义色。

### 表格与列表
- 行高：`.asset-row` 78px、`.activity-row`/`.member-row` 64px；`1px` 底分隔、悬停 `--surface-muted`、列宽固定以便扫读。
- 表头 11px kicker、`--muted`；数字右对齐等宽；排序表头加 `aria-sort`；行内操作最多 3 个，其余收进菜单。
- ≤760px 时表格转卡片堆叠或横向滚动容器（`overflow-x:auto`），不得压缩到不可读；行点击目标 ≥44px。

### 状态标签
- `.status-pill`：圆角 10、高 20–22px、水平 7px、字号 ≥11px；映射：`active/redeemed` 绿、`assigned` 蓝、`draft/expired/suspended` 琥珀、`revoked/refunded` 珊瑚、`archived` 中性。
- 文案使用业务词（`已退款`、`草稿`、`已撤销`），不得只显示色块；关键状态（过期、额度用尽）同时使用 `.access-state.is-expired` / `.delivery-quota.is-exhausted` 等强调样式。

### 空状态、加载与骨架屏
- 空状态：用户端新版用 `.empty-state-card`（`.empty-state-illustration` 内联 SVG + `.empty-state-copy` + `.empty-state-action`），旧 `.empty-state` / `.catalog-empty` / `.product-gallery-empty` 保持兼容；控制台对应 `.empty-state` / `.application-empty`。文案 12–13px `--muted`，必须含一个可执行动作（刷新/返回/联系运营），并区分“无数据”“无权限”“加载失败”。
- 载入文案：保留单行 `.catalog-loading` / `.loading-line` 作为兜底；列表/首屏优先用骨架屏。
- 骨架屏（v0.4.0 已实现）：`.skeleton-block`（块）+ `.skeleton-line`（行），修饰类 `--kicker/--index/--lg/--short/--meta/--page-title/--section-title/--button/--quota`；组合 `.skeleton-card`、`.skeleton-panel`、`.skeleton-gallery-block`、`.delivery-skeleton-*`。用户端使用 `--skeleton-base`/`--skeleton-sheen` 的 `1.6s` 微光，控制台用 `.skeleton-row`/`.skeleton-bar` 的 `1.5s` 明暗；容器必须 `aria-busy="true"`，可见文案用 `.sr-only role="status"`，装饰骨架 `aria-hidden="true"`；尺寸对齐真实布局避免 CLS；`prefers-reduced-motion` 下改为静态灰块；首屏 3s 未完成需给出可感知进度，10s 失败提供重试。

### Toast / 全局提示
- v0.4.0 已实现全局 Toast：用户端 `.toast-viewport`（`role="region" aria-label="操作通知"`），控制台 `.toast-region`；条目 `.toast` + `.toast--success/--error/--info`，图标 + 标题/正文 `.toast-copy`（控制台 `.toast-message`）+ `.toast-close`（`aria-label="关闭通知"`）。
- 行为：错误用 `role="alert" aria-live="assertive"`，成功/信息用 `role="status" aria-live="polite"`，均 `aria-atomic="true"`；用户端默认 3s / 3.8s / 6.5s，控制台 4.2s / 5.2s / 8s，最多同屏 4 条；用户端悬停/聚焦暂停计时，相同消息去重；控制台 `Esc` 清空。两端时长差异属实现选择，同一操作在两个前台的语气与关闭方式必须一致。
- 规则：Toast 是**补充反馈**，表单错误仍须行内展示；不承载需要用户决策的内容；关闭按钮 ≥44px；`prefers-reduced-motion` 下仅保留淡入。

### 光箱 / 图片查看
- v0.4.0 已实现 `.lightbox`（页面遮罩）→ `.lightbox-dialog`（`role="dialog" aria-modal="true"`）；内容 `.lightbox-figure` + `.lightbox-caption`、`.lightbox-counter`，控制 `.lightbox-close`、`.lightbox-nav--prev/--next`。
- 行为：点击遮罩或 `Esc` 关闭；方向键 `←/→` 切换（多图时）；焦点陷阱与打开时 body 滚动锁定；关闭后焦点归还触发缩略图（`returnFocus`）；图注 `aria-live="polite"` 播报切换；所有按钮有 `aria-label`；`prefers-reduced-motion` 下取消缩放/滑入动画。

### 导航与移动端
- `.topbar`（用户端 76px）/`.console-topbar`（控制台 64px）sticky + 毛玻璃 `--topbar-bg`；导航项 13px，当前项用 2px 墨黑下划线 + `aria-current="page"`。
- 首个可聚焦元素是 `.skip-link`（跳到主内容）；`main` 有 id 与 landmark；≤760px 用汉堡菜单，展开后焦点限制在菜单内并支持 `Esc`；控制台 ≤1050px 侧栏折叠为顶部导航。
- 移动端底部固定操作栏高度含安全区（`env(safe-area-inset-bottom)`）；关键操作不被键盘或弹层遮挡。

## 6. 动效原则

- 时长：微交互 150ms（边框/颜色）、控件 250ms、卡片与面板 320ms、路由内容 200–300ms；超过 400ms 视为异常。
- 缓动：位移动画用 `cubic-bezier(.2,.7,.2,1)`，颜色用 `ease`；只动画 `transform`/`opacity`，禁止动画 `width/height/top/left`；不使用弹跳、抖动、自动轮播、视差。
- 页面切换 `.page-motion` 保持 `min-height` 防止跳动；骨架/加载动画是唯一允许的无限动画。
- 必须实现 `@media (prefers-reduced-motion: reduce)` 全局降级（两端已有），动画时长 `.01ms`、取消位移与缩放；降级后信息与状态仍完整。

## 7. 可访问性清单（逐项验收）

- [ ] 键盘：全部交互可 Tab 到达、顺序符合视觉；`Esc` 关闭浮层；对话框焦点陷阱；无键盘陷阱。
- [ ] 焦点可见：`2px solid #35587A`、`outline-offset:2px`；深色主题 `#9CC4E4`（6.86:1 / 10.09:1）。禁止 `outline:none` 无替代。
- [ ] 语义结构：`header/nav/main/footer` landmark；每页一个 `h1`；标题不跳级；`html lang="zh-CN"`。
- [ ] 动态播报：状态/结果 `role="status" aria-live="polite"`，错误 `role="alert" aria-live="assertive"`；异步区域 `aria-busy`；复制哈希等操作有可感知反馈。
- [ ] 对比度：两种主题都满足 §2 要求；截图/工具抽查正文、按钮、标签、输入边界。
- [ ] 触控目标：≥44×44px（视觉可 32–38px，用 padding/伪元素扩大命中区），相邻目标间距 ≥8px。
- [ ] 表单：`label for` 关联、必填与格式说明、错误聚焦到首个无效字段、`aria-describedby`、正确的 `autocomplete`。
- [ ] 图片与图标：内容图片有描述性 `alt`，装饰图片 `alt=""` 且 `aria-hidden="true"`；画廊图注可见。
- [ ] 缩放与重排：200% 缩放、320px 宽仍可操作；无横向滚动（表格自身滚动除外）；文本不被固定高度裁切。
- [ ] 不依赖颜色传达信息（状态、错误、必填均有文字/图标）；动画可关闭；5 分钟下载票据等时限有明确提示。

## 8. 页面级视觉验收 checklist

- [ ] **首页/公开目录**：Hero 对比度与标题层级正确；商品卡键盘可达且悬停/聚焦一致；搜索框有 aria-label、结果数 `aria-live`；空目录显示 `.catalog-empty` 而非空白。
- [ ] **商品详情**：标题/描述/发布徽标层级清晰；图片 16:10 不拉伸、有无图空态；Markdown 链接可聚焦且颜色达 4.5:1；返回链接 ≥44px。
- [ ] **兑换页**：输入框 mono、错误集中在表单顶部/字段旁；提交按钮 loading 与禁用；429 显示 `Retry-After` 可读提示；成功后焦点移到结果区。
- [ ] **交付页**：版本权益面板、额度、更新策略一屏可读；复制 SHA-256 有 `aria-live` 反馈；410/过期态给出下一步；退出按钮有确认或清晰副作用。
- [ ] **控制台总览**：指标卡数字等宽右对齐；运行状态文字 + 颜色双编码；空数据有引导动作。
- [ ] **商品/版本/文件**：发布门槛（至少一个文件）在 UI 中解释；草稿状态显著；危险操作（撤回/删除）二次确认；编辑器与实时预览在 ≤1050px 不重叠。
- [ ] **兑换码/退款**：退款必须确认并显示“将撤销权益”；状态标签映射 §5；批量操作提供结果汇总（成功/失败数）。
- [ ] **成员与权限**：角色与权限点可读；暂停成员提示会撤销会话；不能操作自己/OWNER 的按钮应禁用并解释原因；邀请/MFA 流程有逐步状态。
- [ ] **部署设置**：路径、卷名、环境变量以 mono 展示；复制按钮有反馈；危险操作说明影响范围。
- [ ] **通用**：浅色/深色、1440/768/375 三档、键盘全程、200% 缩放各抽查一次；截图存证到验收记录。

## 9. 变更与验收流程

1. 先在本文件补/改规范，再改代码；token 变更必须同时更新 `web/src/styles.css` 与 `console/src/styles.css` 的 `:root` 与 `[data-theme=dark]` 覆盖。
2. 新增组件需提供：默认/悬停/聚焦/禁用/加载/错误/深色 七种状态的类名与可访问性说明。
3. 改视觉后跑两端 `npm.cmd run build`（含 `tsc -b`），并按 §8 做 6 个视口截图抽查。
4. 提交前 `git diff --check`；新增颜色用 `grep` 确认只出现在 token 定义处。

## 10. 存量回修清单（按优先级）

- [ ] 9–10px 的辅助字号提升到 ≥11–12px，确保关键信息可读。
- [ ] 移动端 <44px 的按钮/图标扩大命中区（`.icon-button`、`.hash-copy`、`.theme-toggle`、`.join-link`）。
- [ ] 硬编码颜色收口到 token，重点是 `#7A858A`、`#8A9296` 等低对比灰。
- [ ] 校验 v0.4.0 新增的 `.toast` / `.skeleton-*` / `.lightbox` / `.empty-state-card` 在两端行为一致（时长、关闭方式、焦点归还、reduced-motion）。
- [ ] 深色主题逐页抽查对比度与边框可见性（`--border-control` 替换输入边框）。
- [ ] 表格在 ≤760px 的堆叠/滚动策略统一，避免文字挤压。
