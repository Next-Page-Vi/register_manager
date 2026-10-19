# 贡献指南 / Contributing

[简体中文](#简体中文) · [English](#english)

## 简体中文

本文面向开发者和维护者，说明本地开发、验证改动，以及更新数据库、API 和翻译的方法。服务器部署请阅读 [README](README.md) 和[运维指南](deploy/OPERATIONS.md)。

### 反馈问题与提交改动

在 [GitHub Issues](https://github.com/Next-Page-Vi/register_manager/issues) 报告一般问题或提出功能建议。先搜索已有讨论，报告时附上镜像标签或提交哈希、浏览器与运行环境、复现步骤、预期和实际结果。日志与截图须删除客户信息和访问凭证；安全漏洞使用[私密报告渠道](SECURITY.md)。

提交拉取请求时：

1. Fork 仓库，在独立分支上完成一项明确改动；涉及产品范围或较大架构调整时先开 Issue 讨论。
2. 同步相关中英文文档、翻译和 API 生成文件，运行本指南中受影响的检查。
3. 向本仓库 `main` 提交 PR，说明解决的问题、最终行为、验证结果及兼容性影响。界面改动附使用虚构数据的截图。

### 本地运行

安装 Go 1.27.2 或更新版本、Node.js 26 或更新版本，以及 pnpm 12.11.2。克隆仓库后，在仓库根目录执行开发命令。

首次获取源码：

```sh
git clone https://github.com/Next-Page-Vi/register_manager.git
cd register_manager
```

安装依赖并构建嵌入 Go 服务的前端：

```sh
pnpm -C web install --frozen-lockfile
pnpm -C web build
```

在一个终端启动 API：

```sh
GOMEMLIMIT=96MiB RM_COOKIE_SECURE=false go run ./cmd/server
```

在另一个终端启动 Vite：

```sh
pnpm -C web dev
```

打开[本地登录页](http://localhost:5173/account/login)。Vite 将 API 请求和客户访问链接转发到 8080 端口的 Go 服务。后端地址不同时，在启动 Vite 时设置 `RM_BACKEND`。

首次启动会创建 `admin` 并输出初始密码。默认数据库为 `data/register_manager.db`，可通过 `RM_DB_PATH` 指定其他本地文件。`RM_COOKIE_SECURE=false` 仅用于本地 HTTP 测试。

### 了解代码结构

后端使用 Go、Chi、Huma、Bun、Goose 迁移和纯 Go SQLite 驱动；前端使用 React、TypeScript、Vite、Tailwind CSS 和 Radix 组件。具体版本以依赖清单和锁文件为准。

| 位置 | 职责 |
| --- | --- |
| `cmd/server/` | HTTP 服务与管理命令 |
| `internal/api/` | API 操作、请求结构与 OpenAPI |
| `internal/booking/`、`internal/schedule/` | 预约规则、可用时间、日期与时区 |
| `internal/storage/`、`db/migrations/` | SQLite 访问、数据模型与结构迁移 |
| `web/src/api/` | 生成的 API 定义与类型化查询辅助函数 |
| `web/src/features/`、`web/src/components/` | 页面与共用界面组件 |
| `web/e2e/` | Playwright 流程、布局检查与测试数据 |
| `deploy/` | Compose 配置与运维说明 |

修改行为时遵守以下边界：

- 直接表达领域逻辑，避免引入通用仓储、事件总线或可配置权限框架。
- 预约决策在服务端完成；前端展示可用时间并提交请求。
- 业务数据写入使用 `storage.DB.WriteTx`，在事务内复核权限与规则；日历数据必须限定 `instance_id`。
- 查询辅助函数接受 `bun.IDB`；会话持久化和数据库维护也会直接使用 `database/sql`。
- 服务端状态通过 `openapi-react-query` 接入 TanStack Query，编辑草稿使用 React Hook Form；请求状态由所属功能维护。
- 显式表达可空数据，用类型约束合法状态，不用空字符串或零代替 null。
- 界面文案使用 `t('…')`，每个 API 错误码都要补齐各语言翻译。
- 字体、间距、浮层和无障碍遵循[前端设计规范](DESIGN.md)。
- 本地依赖、数据库、构建产物和测试报告不纳入 Git；保留锁文件、生成的 API 文件和 `web/dist/.gitkeep`，后者保证首次前端构建前 Go 的嵌入路径存在。

### 验证改动

提交拉取请求前运行相关检查。后端行为改动需要 Go 测试；界面改动需要前端检查及受影响的浏览器流程。

Go 改动检查格式、静态分析与测试：

```sh
gofmt -l cmd internal db web/embed.go
go vet ./...
go test ./...
```

格式检查不得输出文件路径。前端或 API 改动运行：

```sh
pnpm -C web lint
pnpm -C web typecheck
pnpm -C web check:i18n
pnpm -C web check:api
pnpm -C web build
```

[CI 工作流](.github/workflows/ci.yml) 还检查纯 Go 构建、Staticcheck、竞态、30 秒模糊测试、依赖漏洞和日常浏览器测试。`main` 分支或版本标签推送且检查成功后才发布 Docker 镜像；拉取请求仅运行检查。

其他检查与生成命令：

| 任务 | 命令 |
| --- | --- |
| 重新生成 API 定义与规则限制 | `pnpm -C web gen:api` |
| 对可用时间进行模糊测试 | `go test ./internal/booking -run '^$' -fuzz FuzzSlotsAreValid` |
| 密码哈希与验证基准 | `GOMEMLIMIT=96MiB go test ./internal/auth -run '^$' -bench Password -benchmem` |
| 列表查询基准 | `GOMEMLIMIT=96MiB go test ./internal/server -run '^$' -bench BenchmarkLists -benchmem` |

### 运行浏览器测试

安装浏览器及系统依赖，构建前端，再运行 Playwright：

```sh
pnpm -C web exec playwright install --with-deps chromium webkit
pnpm -C web build
pnpm -C web test:e2e
```

测试在 `127.0.0.1:18080` 启动真实 Go 服务，使用 `web/e2e/.tmp/` 下的临时 SQLite 数据库。每次重新构建服务，不复用开发服务。

[Playwright 配置](web/playwright.config.ts) 中各项目的覆盖范围：

| 项目 | 覆盖范围 |
| --- | --- |
| `desktop` | Chromium；全部功能回归与桌面布局 |
| `mobile` | Pixel 7 模拟；`@smoke` 预约流程与 `@touch` 交互 |
| `iphone` | WebKit、iPhone 16 模拟；`@smoke`、`@touch` 与 `@browser` |
| `layout-320x480-ldpi` | 小屏 Chromium；英文浅色、简体中文深色 |
| `layout-iphone-small` | 375×667 WebKit；日文浅色、繁体中文深色 |

发布前或大范围界面改动后运行完整布局矩阵：

```sh
pnpm -C web test:e2e:full
```

完整矩阵增加标准 Pixel 7、iPhone 16 的全部语言与明暗主题，并在小屏上覆盖全部语言的浅色主题，保留日常深色用例。

仅运行 iPhone 流程与小屏 WebKit 布局：

```sh
pnpm -C web test:e2e --project=iphone --project=layout-iphone-small
```

行为测试放在相关功能附近。将页面和弹窗加入 `web/e2e/layout.spec.ts`，使用真实形态的测试数据。核心预约流程标记 `@smoke`，触控交互标记 `@touch`，浏览器相关检查标记 `@browser`。无需在每种设备上重复与浏览器无关的回归。

失败时截图和追踪保留在 `web/test-results/`，CI 上传后保留七天。设备模拟不能验证特定 iOS 版本、软键盘或微信内置浏览器，这些行为需要真机检查。

### 修改数据库或 API

同步更新数据库结构和生成定义：

1. 在 `db/migrations/` 新增迁移，不修改已应用的迁移。需要重建表时保留数据、索引和触发器。
2. 更新 `internal/storage/models.go` 和相关 Bun 查询。可空字段使用指针，明确更新列，参数化传值；结构变更通过迁移完成，不使用 ORM 自动建表。
3. 修改 API 类型后运行 `pnpm -C web gen:api`，包含全部三个生成文件：`web/src/api/openapi.json`、`web/src/api/schema.d.ts` 和 `web/src/api/constraints.ts`。
4. 数值规则限制集中在 `internal/booking/policy.go`；重新生成前端限制，不维护第二份常量，再运行 `pnpm -C web check:api`。

### 添加语言

服务端接受语言标签，无需修改服务端代码或数据库结构。更新前端和测试：

1. 将 `web/src/locales/en/translation.json` 复制到 `web/src/locales/fr/translation.json`，用新语言的 BCP 47 标签替换 `fr`。翻译值，保留插值占位符，使用该语言的复数后缀。
2. 在 `web/src/i18n/index.ts` 的 `SUPPORTED_LOCALES` 添加标签，在 `LOCALE_NAMES` 添加本语言名称。需要映射浏览器语言别名时扩展 `matchLocale`。
3. 更新 `web/e2e/layout.spec.ts`、`web/e2e/statistics.spec.ts` 的语言列表，检查 `web/playwright.config.ts` 中日常项目的筛选条件，使文档与覆盖范围一致。
4. 运行 `pnpm -C web check:i18n`、前端构建和相关布局测试，并更新 README 的语言列表。

所有项目文档均使用简体中文和英语，保持两种语言内容一致。README 面向部署用户，保持简洁；开发和镜像发布流程写在本指南中。

### 发布部署镜像

维护者通过 [GitHub Actions](.github/workflows/ci.yml) 发布镜像。推送到 `main` 或推送 `v*.*.*` 版本标签后，检查和测试成功才会运行 `docker` 任务。工作流通过 [Dockerfile](Dockerfile) 构建 Linux AMD64 镜像并推送到 GitHub Container Registry（GHCR）；拉取请求不发布镜像。

项目仓库为 `Next-Page-Vi/register_manager`，镜像为 `ghcr.io/next-page-vi/register_manager`。工作流根据小写仓库路径生成镜像名，并通过 Docker Metadata Action 生成版本标签和 OCI 标签。发布使用自动提供的 `GITHUB_TOKEN` 和 `packages: write` 权限，无需手动配置镜像仓库密钥。

各类推送生成的镜像标签如下：

| 推送内容 | 镜像标签 |
| --- | --- |
| `main` 分支 | `edge`、`sha-完整提交哈希` |
| `v0.1.1` 等无预发布后缀的版本标签 | 对应版本号、`latest`、`sha-完整提交哈希` |
| `v0.2.0-rc.1` 等预发布版本标签 | 对应版本号、`sha-完整提交哈希`，不更新 `latest` |

首次发布使用 `v0.1.0`。后续修复递增补丁号，新增功能递增次版本号；在 `0.x` 阶段，破坏性变更也递增次版本号，并在发布说明中写明迁移要求。版本标签发布后不要移动或覆盖，修复使用新版本。

发布前，在与公开仓库 `main` 一致的干净工作区中，将 `web/package.json` 的版本与目标版本对齐，并同步 README 中英文 Compose、`deploy/compose.yaml`、`deploy/.env.example` 和运维指南的默认版本。推送这些改动后，从该提交创建并推送版本标签，例如：

```sh
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin refs/tags/v0.1.1
```

等待该标签的 CI 和镜像发布成功，再创建对应的 GitHub Release。Git 标签中的预发布后缀决定是否更新 `latest`，GitHub Release 的 **Pre-release** 选项不控制镜像标签。版本默认值随发布改动提交，在标签构建成功前，新版本镜像还不可拉取。

Docker 构建自动收集 Go 依赖许可、Go 工具链许可和 Vite 生成的前端依赖许可，与本项目的 LICENSE 一起放入 `/usr/share/licenses/register_manager/`。依赖更新时检查构建结果和[第三方声明](THIRD_PARTY_NOTICES.md)，不要只更新根目录许可证。

公开发布前：

1. 在仓库设置中启用 **Issues** 和 **Private vulnerability reporting**，验证 [SECURITY.md](SECURITY.md) 中的私密报告入口。
2. 首次发布镜像后，在 **Package settings** 中将镜像包可见性设为 **Public**。
3. 确认 README 的 Compose 示例与部署配置一致；不设置 `RM_IMAGE`，使用未登录的 Docker 客户端按 README 拉取镜像。
4. 用空数据卷按 README 和 Nginx 示例完成 HTTPS 部署，验证登录、客户链接和日志脱敏；再使用虚构数据演练备份、恢复和固定版本更新。
5. 检查镜像内许可文件，并在 GitHub Release 中记录版本号、对应 `sha-` 标签、镜像摘要、数据库兼容性、破坏性变更和验证结果。预发布版本标记为 **Pre-release**；尚未完成的外部验证明确列为待办。

镜像包权限见 [GitHub Container registry 文档](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)。发布凭证和镜像仓库管理说明保留在维护者文档中。

### 许可证

贡献内容使用本项目的 [MIT 许可证](LICENSE)。

## English

Use this guide to run the project locally, check changes, and update its database, API, or translations. For server deployment, follow the [README](README.md) and [operations guide](deploy/OPERATIONS.md).

### Report issues and contribute changes

Use [GitHub Issues](https://github.com/Next-Page-Vi/register_manager/issues) for ordinary bugs and feature requests. Search existing discussions first. Include the image tag or commit hash, browser/runtime environment, reproduction steps, and expected versus actual results. Remove customer information and credentials from logs and screenshots. Report security vulnerabilities through the [private channel](SECURITY.md).

To submit a pull request:

1. Fork the repository and make one focused change on a separate branch. Discuss product-scope or substantial architecture changes in an issue first.
2. Update the affected Chinese/English documentation, translations, and generated API files. Run the relevant checks below.
3. Open a PR against this repository's `main` branch. Describe the problem, final behavior, validation results, and compatibility impact. Include screenshots with fictional data for interface changes.

### Run locally

Install Go 1.27.2 or later, Node.js 26 or later, and pnpm 12.11.2. After cloning, run development commands from the repository root.

To obtain the source for the first time:

```sh
git clone https://github.com/Next-Page-Vi/register_manager.git
cd register_manager
```

Install dependencies and build the frontend embedded in the Go server:

```sh
pnpm -C web install --frozen-lockfile
pnpm -C web build
```

Start the API in one terminal:

```sh
GOMEMLIMIT=96MiB RM_COOKIE_SECURE=false go run ./cmd/server
```

Start Vite in another terminal:

```sh
pnpm -C web dev
```

Open [the local login page](http://localhost:5173/account/login). Vite forwards API requests and customer access links to the Go server on port 8080. Set `RM_BACKEND` when starting Vite if the backend uses a different address.

The first server start creates `admin` and prints its password. Local data defaults to `data/register_manager.db`; override `RM_DB_PATH` to use another local file. Keep `RM_COOKIE_SECURE=false` limited to local HTTP testing.

### Understand the codebase

The backend uses Go, Chi, Huma, Bun, Goose migrations, and the pure-Go SQLite driver. The frontend uses React, TypeScript, Vite, Tailwind CSS, and Radix components. Dependency manifests and the lockfile define the versions.

| Location | Responsibility |
| --- | --- |
| `cmd/server/` | HTTP server and administrative commands |
| `internal/api/` | API operations, request schemas, and OpenAPI |
| `internal/booking/`, `internal/schedule/` | Booking rules, availability, dates, and time zones |
| `internal/storage/`, `db/migrations/` | SQLite access, database models, and schema migrations |
| `web/src/api/` | Generated API definitions and typed query helpers |
| `web/src/features/`, `web/src/components/` | Pages and shared interface components |
| `web/e2e/` | Playwright flows, layout checks, and fixtures |
| `deploy/` | Compose configuration and operations instructions |

Keep these boundaries when changing behavior:

- Prefer direct domain code over generic repositories, event buses, or configurable permission frameworks.
- Keep booking decisions on the server. The frontend displays availability and submits requests.
- Use `storage.DB.WriteTx` for business-data writes. Recheck permissions and rules inside the transaction, and scope calendar data with `instance_id`.
- Share query helpers through `bun.IDB`. Session persistence and database maintenance also use `database/sql` directly.
- Use TanStack Query through `openapi-react-query` for server state and React Hook Form for editable drafts. Keep request state in the feature that owns it.
- Represent nullable data explicitly and model valid states with types. Do not use empty strings or zero as stand-ins for null.
- Translate interface text with `t('…')`. Add every API error code to each locale.
- Follow [the frontend design specification](DESIGN.md) for typography, spacing, overlays, and accessibility.
- Keep local dependencies, databases, build output, and test reports out of Git. Retain lockfiles, generated API files, and `web/dist/.gitkeep`; the placeholder keeps Go embedding valid before the first frontend build.

### Check a change

Run the relevant checks before submitting a pull request. Backend behavior changes need Go tests; interface changes need frontend checks and the affected browser flows.

For Go changes, verify formatting, static analysis, and tests:

```sh
gofmt -l cmd internal db web/embed.go
go vet ./...
go test ./...
```

The formatting command must produce no paths. For frontend or API changes, run:

```sh
pnpm -C web lint
pnpm -C web typecheck
pnpm -C web check:i18n
pnpm -C web check:api
pnpm -C web build
```

The [CI workflow](.github/workflows/ci.yml) also checks a pure-Go build, Staticcheck, the race detector, a 30-second fuzz run, dependency vulnerabilities, and the daily browser suite. A `main` or version-tag push publishes a Docker image after the checks pass; pull requests only run checks.

Additional checks and generation commands:

| Task | Command |
| --- | --- |
| Regenerate API definitions and policy limits | `pnpm -C web gen:api` |
| Fuzz available slots | `go test ./internal/booking -run '^$' -fuzz FuzzSlotsAreValid` |
| Benchmark password hashing and verification | `GOMEMLIMIT=96MiB go test ./internal/auth -run '^$' -bench Password -benchmem` |
| Benchmark list queries | `GOMEMLIMIT=96MiB go test ./internal/server -run '^$' -bench BenchmarkLists -benchmem` |

### Run browser tests

Install the browsers and their system dependencies, then build the frontend before starting Playwright:

```sh
pnpm -C web exec playwright install --with-deps chromium webkit
pnpm -C web build
pnpm -C web test:e2e
```

The suite starts a real Go server on `127.0.0.1:18080` with a temporary SQLite database in `web/e2e/.tmp/`. It rebuilds the server for each run. It does not reuse your development server.

Projects in [the Playwright configuration](web/playwright.config.ts) select the checks:

| Project | Coverage |
| --- | --- |
| `desktop` | Chromium; all functional regressions and desktop layout checks |
| `mobile` | Pixel 7 emulation; `@smoke` booking flows and `@touch` interactions |
| `iphone` | WebKit with iPhone 16 emulation; `@smoke`, `@touch`, and `@browser` checks |
| `layout-320x480-ldpi` | Small Chromium viewport; English/light and Simplified Chinese/dark |
| `layout-iphone-small` | 375×667 WebKit viewport; Japanese/light and Traditional Chinese/dark |

Run the full layout matrix before releases or broad interface changes:

```sh
pnpm -C web test:e2e:full
```

This adds standard Pixel 7 and iPhone 16 layouts in every supported language and both themes. It also checks every language in light mode on the small viewports, retaining their daily dark cases.

To focus on iPhone flows and the small WebKit layout:

```sh
pnpm -C web test:e2e --project=iphone --project=layout-iphone-small
```

Add behavior tests close to the affected feature. Add pages and dialogs to `web/e2e/layout.spec.ts`, with realistic seed data. Tag core booking flows with `@smoke`, touch interactions with `@touch`, and browser-specific checks with `@browser`. Avoid repeating browser-independent regressions on every device.

Failures retain screenshots and traces in `web/test-results/`; CI uploads them for seven days. Device emulation does not verify a specific iOS release, the software keyboard, or WeChat's embedded browser. Check those behaviors on a real device.

### Change the database or API

Update schema and generated definitions together:

1. Add a migration in `db/migrations/`. Never edit an applied migration. Rebuild tables when needed to change constraints, preserving data, indexes, and triggers.
2. Update the models in `internal/storage/models.go` and the affected Bun queries. Use pointers for nullable fields, explicit update columns, and parameterized values. Schema changes belong in migrations, not ORM table creation.
3. After changing API types, run `pnpm -C web gen:api`. Include all three generated files: `web/src/api/openapi.json`, `web/src/api/schema.d.ts`, and `web/src/api/constraints.ts`.
4. Keep numeric booking-policy limits in `internal/booking/policy.go`. Regenerate the frontend limits instead of maintaining duplicate constants, then run `pnpm -C web check:api`.

### Add a language

The server accepts locale tags without a code or schema change. Update the frontend and its tests:

1. Copy `web/src/locales/en/translation.json` to `web/src/locales/fr/translation.json`, replacing `fr` with the new BCP 47 tag. Translate values, retain interpolation placeholders, and use the language's plural suffixes.
2. Add the tag to `SUPPORTED_LOCALES` and its native name to `LOCALE_NAMES` in `web/src/i18n/index.ts`. Extend `matchLocale` if browser-language aliases need mapping.
3. Update the locale lists in `web/e2e/layout.spec.ts` and `web/e2e/statistics.spec.ts`. Review the daily project filters in `web/playwright.config.ts` so the documented coverage stays accurate.
4. Run `pnpm -C web check:i18n`, build the frontend, and run the relevant layout tests. Update the language list in the README.

Write all project documentation in Simplified Chinese and English, keeping both versions consistent. Keep the README concise and focused on deployment users; document development and image publication in this guide.

### Publish the deployment image

Maintainers publish the image through [GitHub Actions](.github/workflows/ci.yml). Push to `main` or push a `v*.*.*` version tag, then wait for the `docker` job after checks and tests pass. The workflow builds the [Dockerfile](Dockerfile) for Linux AMD64 and pushes to GitHub Container Registry (GHCR); pull requests do not publish images.

The repository is `Next-Page-Vi/register_manager`, and its image is `ghcr.io/next-page-vi/register_manager`. The workflow derives the image name from the lowercase repository path. Docker Metadata Action generates version tags and OCI labels. Publishing uses the automatic `GITHUB_TOKEN` with `packages: write`, without a manually configured registry secret.

Each push produces these image tags:

| Push | Image tags |
| --- | --- |
| `main` branch | `edge`, `sha-full_commit_hash` |
| Version without a pre-release suffix, such as `v0.1.1` | Full version, `latest`, `sha-full_commit_hash` |
| Pre-release tag, such as `v0.2.0-rc.1` | Full version, `sha-full_commit_hash`; does not update `latest` |

Use `v0.1.0` for the first release. Increment the patch number for fixes and the minor number for new features. During `0.x` development, increment the minor number for breaking changes too, and document migration requirements. Never move or overwrite a published version tag; publish fixes as new versions.

Prepare releases in a clean checkout matching the public repository's `main` branch. Align `web/package.json` with the target version. Update the default version in both README Compose examples, `deploy/compose.yaml`, `deploy/.env.example`, and the operations guide. Push those changes, then create and push a version tag on that commit, for example:

```sh
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin refs/tags/v0.1.1
```

Wait for the tag's CI and image publication to succeed, then create its GitHub Release. The Git tag's pre-release suffix controls whether `latest` updates; the GitHub Release **Pre-release** checkbox does not. Default versions change in the release commit, but the new image cannot be pulled until its tag build succeeds.

Docker builds collect Go dependency licenses, the Go toolchain license, and Vite-generated frontend dependency licenses. These accompany the project LICENSE under `/usr/share/licenses/register_manager/`. When updating dependencies, review the build output and [third-party notices](THIRD_PARTY_NOTICES.md), not only the root license.

Before sharing a public release:

1. Enable **Issues** and **Private vulnerability reporting** in repository settings. Verify the private reporting link in [SECURITY.md](SECURITY.md).
2. After the first image publication, set its visibility to **Public** in **Package settings**.
3. Confirm the README's Compose examples match the deployment configuration. Leave `RM_IMAGE` unset and follow the README to pull the image with an unauthenticated Docker client.
4. Follow the README and Nginx example with an empty volume. Verify HTTPS, login, customer links, and credential-safe logs, then exercise backup, restore, and pinned-image updates with fictional data.
5. Inspect license files inside the image. Record its version, `sha-` tag, image digest, database compatibility, breaking changes, and validation results in the GitHub Release. Mark pre-release versions as **Pre-release** and list unfinished external checks as pending.

See [GitHub's Container registry documentation](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry) for package permissions. Keep publishing credentials and registry administration out of user deployment instructions.

### License

By contributing, you agree that your contributions use the [MIT license](LICENSE).
