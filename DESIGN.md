# 前端设计规范 / Frontend design specification

[简体中文](#简体中文) · [English](#english)

## 简体中文

本文用于指导界面改动。工作人员需要快速浏览日历、客户和预约；客户（包括老年用户）需要清晰的预约步骤和大控件。保留现有导航、语义颜色和系统字体。

### 字体与间距

根字号保持浏览器默认值的 `100%`。下表按 16 px 根字号换算；实现使用 rem 和语义变量。

| 元素 | 管理端 | 客户端 |
| --- | --- | --- |
| 正文 | 16 px | 18 px |
| 辅助文字 | 14 px | 16 px |
| 页面标题 | 24 px，粗体 | 27 px，粗体 |
| 区块标题 | 20 px，半粗体 | 22.5 px，半粗体 |
| 默认输入框与按钮高度 | 至少 44 px | 至少 54 px |
| 小按钮高度 | 40 px；粗指针设备为 44 px | 至少 45 px |
| 大按钮高度 | 至少 56 px | 至少 63 px |
| 页面区块间距 | 20 px | 24 px |
| 表单字段间距 | 16 px | 22.5 px |
| 卡片内容间距 | 16 px | 22.5 px |
| 卡片水平内边距 | 16 px；40rem 起为 20 px | 18 px；40rem 起为 27 px |
| 卡片垂直内边距 | 20 px；列表卡片为 16 px | 22.5 px；40rem 起为 27 px |

[全局样式](web/src/index.css) 定义 `--control-height`、`--control-small`、`--control-large`、`--page-gap`、`--form-gap`、`--card-gap`、`--card-padding-inline`、`--card-padding-block` 和 `--dialog-width`。

[`Surface`](web/src/components/Surface.tsx) 接受 `staff` 或 `customer`。客户区域覆盖文字、间距和控件变量，不改变全局根字号。对话框、确认框和抽屉通过 React context 把所属区域传入 portal；客户预览也需包在 `Surface kind="customer"` 中。

中文和日文使用地区对应的系统字体，不下载网页字体。标题和长词允许换行，各语言按钮均保留完整文字。日期、时间和数值控件使用等宽数字；文字放大时允许页面自然增长。

### 页面布局与操作

使用共用组件保持页面宽度、标题和控件一致：

- 管理端居中，最大宽度 90rem；客户端最大宽度 47.25rem。管理端正文水平内边距为 16 px，`sm` 起为 24 px，垂直内边距为 24 px。
- 日历导航从 `lg` 起使用 13rem 侧栏，与正文间距 24 px；窄屏改用抽屉。页头从 `xl` 起显示账号名称，提示和移动菜单中保留完整名称。
- 页面使用 [`Page` 和 `PageTitle`](web/src/components/layout.tsx)，标题以 `min-h-11` 对齐首行，旁边的操作随可用空间换行。
- [`ItemCard`](web/src/components/blocks.tsx) 在容器达到 45rem 时分为信息列与操作列；操作列使用 `fit-content(24rem)`。更窄时操作放在内容之后。
- [`ActionGroup`](web/src/components/ActionGroup.tsx) 按 DOM 顺序让完整按钮换行。响应式弹窗操作在窄容器中占满行宽，达到 24rem 时恢复自然宽度。
- [日期导航](web/src/components/AvailabilityDateFilter.tsx) 的前后日期按钮保持成组；筛选容器不足 20rem 时整体放到日期输入框下面。
- 日期范围和排班周期使用 `.control-row`。表单容器达到 34rem 时，标签、输入和帮助文字共享网格行，操作与输入对齐；窄屏保持文档顺序。周期行最大宽度 40rem。
- `SaveBar` 保留未保存、处理中、成功和错误反馈。固定保存区贴表单顶部；表单容器不足 20rem 时回到正常文档流。

### 弹窗与输入

管理端弹窗默认最大宽度 35rem，宽表单为 45rem；客户端为 36rem。视口尺寸与安全区域限制宽高。内容在弹窗内部滚动，固定的表单操作保持可见。

浮层内容始终高于遮罩，反复关闭和打开时也应如此：

| 层级 | 遮罩 z-index | 内容 z-index |
| --- | --- | --- |
| 导航抽屉 | 40 | 41 |
| 对话框 | 50 | 51 |
| 确认框 | 60 | 61 |

保留 Radix 的焦点约束、Escape 处理和焦点恢复。使用短过渡并遵循 `prefers-reduced-motion`。原生日期和时间输入保留 `appearance-none` 以控制盒模型，同时保留原生选择器。

预约规则表单保存和预览共用 React Hook Form 校验，`noValidate` 防止浏览器消息替代本地化错误。排班周期检查必填、整数和范围。字段错误关联 `aria-describedby`、`aria-invalid`，并聚焦首个无效字段。

### 统计图表

[统计页](web/src/features/manage/Statistics.tsx) 的趋势图和状态图共用边距、坐标轴样式与高度。上、右边距 16 px，左边距为零，下边距 4 px；刻度文字 14 px，使用等宽数字，与坐标轴间隔 8 px。

显示左、下坐标轴，纵轴宽度适应数值和翻译后的状态标签。分类标签在水平条形左侧，垂直居中于对应行。并排时对齐绘图区顶部、基线和高度；窄屏上下排列。保留文字图例和可展开的数据视图。

### 颜色与无障碍

沿用 OKLCH 语义令牌。明暗主题跟随操作系统，没有主题切换按钮。正文、辅助文字、控件边框、装饰分隔线和焦点圈使用各自令牌。

保留以下交互规则：

- 选中、危险和成功状态同时使用文字或图标，不只依赖颜色。
- 保留管理端跳到正文链接和可见键盘焦点。
- 触控布局的目标至少 44×44 CSS px，客户端使用更大的默认尺寸。
- 状态与错误消息保留 live region。
- 支持本地 HTTP 测试；使用现有 UUID 和剪贴板回退，不仅依赖安全上下文 API。

### 验证界面改动

按[贡献指南](CONTRIBUTING.md#运行浏览器测试)安装依赖和 Playwright 浏览器后运行：

```sh
pnpm -C web lint
pnpm -C web typecheck
pnpm -C web check:i18n
pnpm -C web build
pnpm -C web test:e2e:full --workers=4
```

使用现有测试覆盖：

| 测试文件 | 覆盖范围 |
| --- | --- |
| [`density.spec.ts`](web/e2e/density.spec.ts) | 管理端 768、1024、1280、1440、1920 px 布局；英文和日文；日期导航、portal 尺寸、校验、表单行、文字放大与反复确认 |
| [`statistics.spec.ts`](web/e2e/statistics.spec.ts) | 四种非零状态；1440、320 px 下的坐标轴和标签对齐；四种语言、两种主题；Chromium 和 WebKit |
| [`layout.spec.ts`](web/e2e/layout.spec.ts) | 带测试数据的客户和管理页面、弹窗；溢出、文字裁切、控件尺寸、触控目标及 axe 无障碍检查 |

日常运行在小屏 Chromium 和 WebKit 上选择四组语言、主题组合。完整矩阵增加标准手机的全部语言与主题，以及小屏上全部语言的浅色主题。功能回归同时运行；截图和失败追踪位于被忽略的 `web/test-results/`。

设备模拟无法验证特定 Safari 或微信版本、手机浏览器工具栏、软键盘和原生日期选择器；发布前需在真机检查。

## English

Use this specification when changing the interface. Staff need to scan calendars, customers, and appointments; customers, including older adults, need readable booking steps and large controls. Keep the existing navigation, semantic colors, and system fonts.

### Typography and spacing

The root font size stays at `100%` of the browser default. These pixel equivalents assume a 16 px root; implementation uses rem values and semantic variables.

| Element | Staff | Customer |
| --- | --- | --- |
| Body text | 16 px | 18 px |
| Supporting text | 14 px | 16 px |
| Page title | 24 px, bold | 27 px, bold |
| Section title | 20 px, semibold | 22.5 px, semibold |
| Default input and button height | At least 44 px | At least 54 px |
| Small button height | 40 px; 44 px with a coarse pointer | At least 45 px |
| Large button height | At least 56 px | At least 63 px |
| Page section gap | 20 px | 24 px |
| Form field gap | 16 px | 22.5 px |
| Card content gap | 16 px | 22.5 px |
| Card horizontal padding | 16 px; 20 px from 40rem | 18 px; 27 px from 40rem |
| Card vertical padding | 20 px; 16 px for list cards | 22.5 px; 27 px from 40rem |

[The global stylesheet](web/src/index.css) defines `--control-height`, `--control-small`, `--control-large`, `--page-gap`, `--form-gap`, `--card-gap`, `--card-padding-inline`, `--card-padding-block`, and `--dialog-width`.

[`Surface`](web/src/components/Surface.tsx) accepts `staff` or `customer`. Customer surfaces override text, spacing, and control variables. Keep the global root scale unchanged. Dialogs, alert dialogs, and sheets read the surface from React context and carry it into their portals. Wrap customer previews in `Surface kind="customer"` too.

Use regional system-font fallbacks for Chinese and Japanese; do not download web fonts. Allow headings and long words to wrap, and preserve full button labels across translations. Use tabular numbers for dates, times, and numeric controls. Let pages grow when readers increase their text size.

### Page layout and actions

Use shared components so page widths, title alignment, and controls remain consistent:

- Staff content uses a centered 90rem maximum width; customer content uses 47.25rem. Staff main content has 16 px horizontal padding, increasing to 24 px from the `sm` breakpoint, and 24 px vertical padding.
- Calendar navigation uses a 13rem sidebar from `lg`, separated from the main content by 24 px. Narrow layouts use a sheet. The header shows the account name from `xl`; its full value remains in the tooltip and mobile menu.
- Build pages with [`Page` and `PageTitle`](web/src/components/layout.tsx). Titles use `min-h-11` to align their first row, and adjacent actions wrap as space narrows.
- [`ItemCard`](web/src/components/blocks.tsx) switches to information and action columns when its container reaches 45rem. The action column uses `fit-content(24rem)`; below that width, actions follow the content.
- [`ActionGroup`](web/src/components/ActionGroup.tsx) wraps whole buttons in DOM order. Responsive dialog actions fill narrow rows and return to their natural width when the dialog container reaches 24rem.
- [Date navigation](web/src/components/AvailabilityDateFilter.tsx) keeps previous and next buttons together. Below a 20rem filter-container width, move both beneath the date input.
- Date ranges and schedule cycles use `.control-row`. From a 34rem form-container width, labels, inputs, and help text share grid rows, with action buttons aligned to inputs. Narrower forms retain document order. Limit cycle rows to 40rem.
- `SaveBar` preserves unsaved, pending, success, and error feedback. Sticky bars attach to the top of the form; below a 20rem form-container width they return to normal flow.

### Dialogs and input

Staff dialogs default to a 35rem maximum width, wide forms to 45rem, and customer dialogs to 36rem. Viewport dimensions and safe-area insets constrain both width and height. Content scrolls inside the dialog, with sticky form actions remaining visible.

Keep overlay content above its backdrop, including during repeated close/open transitions:

| Layer | Backdrop z-index | Content z-index |
| --- | --- | --- |
| Navigation sheet | 40 | 41 |
| Dialog | 50 | 51 |
| Confirmation dialog | 60 | 61 |

Preserve Radix focus trapping, Escape handling, and focus restoration. Use short transitions and respect `prefers-reduced-motion`. Retain `appearance-none` on native date and time inputs to control their box model without removing the native picker.

The booking-policy form uses the same React Hook Form validation for saving and previewing. Its `noValidate` attribute prevents browser messages from replacing localized errors. Schedule-cycle validation covers required values, integer values, and bounds. Connect field errors with `aria-describedby` and `aria-invalid`, and focus the first invalid field.

### Statistics charts

[The statistics page](web/src/features/manage/Statistics.tsx) shares chart margins, axis styling, and heights between the trend and status charts. Margins are 16 px at the top and right, zero at the left, and 4 px at the bottom. Tick labels use 14 px text, tabular numbers, and an 8 px gap from the axis.

Show the left and bottom axes. Let vertical-axis width adapt to numbers and translated status labels. Keep category labels left of the horizontal bars, centered against their rows. Align plot tops, baselines, and heights when charts sit side by side; stack them on narrow screens. Retain the text legend and expandable data view.

### Color and accessibility

Use the existing OKLCH semantic tokens. Light and dark themes follow the operating-system preference; the application has no theme toggle. Use separate tokens for text, supporting text, control borders, decorative separators, and focus rings.

Preserve these interaction rules:

- Pair selected, destructive, and success colors with text or icons.
- Keep the staff skip-to-content link and visible keyboard focus.
- Use targets at least 44×44 CSS px on touch layouts, with the larger customer defaults.
- Preserve live regions for status and error messages.
- Support plain HTTP during local testing. Use the existing UUID and clipboard fallbacks instead of relying on secure-context browser APIs.

### Verify interface changes

After installing dependencies and Playwright browsers as described in [the contribution guide](CONTRIBUTING.md#run-browser-tests), run:

```sh
pnpm -C web lint
pnpm -C web typecheck
pnpm -C web check:i18n
pnpm -C web build
pnpm -C web test:e2e:full --workers=4
```

Use the existing suites for these checks:

| Suite | Coverage |
| --- | --- |
| [`density.spec.ts`](web/e2e/density.spec.ts) | Staff layouts at 768, 1024, 1280, 1440, and 1920 px; English/Japanese text; date navigation; portal sizing; validation; form rows; enlarged text; repeated confirmations |
| [`statistics.spec.ts`](web/e2e/statistics.spec.ts) | Four nonzero statuses, axis and label alignment at 1440 and 320 px, all four languages and both themes, Chromium and WebKit |
| [`layout.spec.ts`](web/e2e/layout.spec.ts) | Customer and staff pages and dialogs with seeded data, overflow, clipped text, control dimensions, touch targets, and axe accessibility checks |

Daily runs select four language/theme combinations on small Chromium and WebKit viewports. The full matrix adds standard phones in all languages and themes, plus all languages in light mode on small viewports. Functional regressions run alongside these checks; screenshots and failure traces stay in the ignored `web/test-results/` directory.

Device emulation cannot verify a particular Safari or WeChat release, mobile browser chrome, software keyboards, or native date pickers. Check these behaviors on real devices before release.
