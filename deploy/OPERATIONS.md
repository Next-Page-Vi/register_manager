# 运维指南 / Operations

[简体中文](#简体中文) · [English](#english)

## 简体中文

本文说明 Docker Compose 部署后的配置、更新和维护。先完成 [README 中的部署步骤](../README.md)，以下命令在 `deploy/` 目录执行。

### 配置容器

Compose 默认拉取 `ghcr.io/next-page-vi/register_manager:latest`，无需 `.env`。覆盖镜像时，在 `compose.yaml` 所在目录创建 `.env` 并设置 `RM_IMAGE`；仓库的 `.env.example` 可作为模板。在同一服务器移动部署文件时保留目录名 `deploy`：Compose 根据项目名识别已有数据卷。

应用配置添加到 [`compose.yaml`](compose.yaml) 的 `environment` 中；仅写入 `.env` 不会传入容器。

| 变量 | 部署默认值 | 用途 |
| --- | --- | --- |
| `RM_IMAGE` | `ghcr.io/next-page-vi/register_manager:latest` | 可选覆盖 Compose 使用的镜像地址 |
| `RM_DB_PATH` | `/data/register_manager.db` | 持久化卷中的 SQLite 数据库 |
| `RM_LISTEN` | `:8080` | 容器内 HTTP 监听地址 |
| `RM_TRUSTED_PROXY_HEADER` | `X-Forwarded-For` | 限速使用的客户端地址头 |
| `RM_COOKIE_SECURE` | `true` | 安全 Cookie；仅本地 HTTP 测试时关闭 |
| `RM_LOG_LEVEL` | `info` | `debug`、`info`、`warn` 或 `error` |
| `GOMEMLIMIT` | `96MiB` | Go 运行时软内存限制，不是 RSS 硬上限 |

应用映射到宿主机的回环地址 8080 端口。在宿主机配置 HTTPS 反向代理，保留公开域名，并根据客户端连接设置 `X-Forwarded-For`，不要原样转发客户端自行提供的地址头。

只运行一个应用实例，数据库卷使用本地磁盘。替换容器会保留数据卷；`docker compose down -v` 会删除数据卷。

查看容器状态和最近日志：

```sh
docker compose ps
docker compose logs --tail=100 app
```

镜像每 30 秒检查一次 `/healthz`，该端点检查数据库连接。

### 配置 HTTPS 反向代理

仓库提供[宿主机 Nginx 配置示例](nginx.conf.example)。先准备域名、DNS 和 TLS 证书，将示例中的 `booking.example.com` 和证书路径替换为实际值，再把文件放入 Nginx 在 `http` 块内加载的配置目录，例如 `/etc/nginx/conf.d/register_manager.conf`。该示例直接面向客户端，不在 Nginx 前再放一层代理。

检查配置成功后重载宿主机 Nginx：

```sh
sudo nginx -t && sudo nginx -s reload
```

示例保留公开 Host，覆盖客户端提供的 `X-Forwarded-For`，并在访问日志中仅记录不含查询参数的路径。凭证入口关闭错误日志，因为 Nginx 错误信息也可能包含完整请求 URL；这些请求的状态码仍保留在访问日志中。应用不可达时，检查容器状态、应用日志及 `/healthz`。

个人访问链接包含可反复使用的登录凭证，持有者可以以该客户身份预约、查看和取消预约。按密码保管链接和二维码，只交给对应客户；泄露或遗失时在“客户”重新生成，旧链接及已有会话随即失效。注册邀请码也不应出现在公开日志中。

使用其他代理、CDN 或监控服务时，同样过滤查询参数、Cookie 和认证信息，并检查错误日志与请求追踪。代理必须把原始查询参数传给应用；只从日志中移除。Nginx 日志设置见[官方文档](https://nginx.org/en/docs/http/ngx_http_log_module.html)。

### 配置账号与预约表

首次启动会创建唯一的超级管理员 `admin`，并在日志中输出初始密码。通过 HTTPS 域名下的 `/account/login` 登录并修改密码。

按以下顺序开放预约：

1. 在“账号”创建操作员。
2. 在“全部预约表”创建预约表，设置时区、默认语言，并分配操作员。新预约表默认暂停。
3. 设置“工作时间”和“预约规则”，再在“设置”开放预约。
4. 分享注册二维码，在“客户”审批申请，再把个人访问链接或二维码交给对应客户。

个人访问链接仅在签发时显示一次，可撤销。超级管理员不能删除或停用。每个账号都可在“我的账号”修改自己的登录名和密码。

忘记超级管理员登录名或密码时执行：

```sh
docker compose exec app server admin reset-password
```

命令会提示输入密码，并显示登录名。首次启动应用前也可预先设置初始密码：

```sh
docker compose run --rm --no-deps app admin reset-password
```

密码通过终端或标准输入传入，不放在命令参数中。

### 理解使用边界

开放预约前，确认这些行为符合运营方式：

- 每个预约表只有一条服务时间线，同一表内的非取消预约不能重叠。绑定多个操作员不会增加同时服务人数。
- 不同预约表独立计算时间和配额，即使绑定同一操作员也不防止跨表撞期；客户身份也不跨表合并。
- 排班、封锁、缓冲、配额和提前量约束客户自助预约。后台代建与改期只检查权限、未来起止时间和同表实际服务重叠，可使用任意时长及跨日时间；后台取消不受取消提前量限制。
- 暂停预约表会停止新客户登记和客户自助预约；已有客户仍可登录、查看及按规则取消，后台仍可代建和改期。暂停不取消已有预约，也不撤销链接。
- 修改排班和规则不会调整已有预约，也不检查其冲突；删除预约表则永久删除业务数据，没有回收站，恢复依赖整库备份。
- 系统不发送短信或邮件通知，不提供支付、候补、日历同步或客户自助找回链接。审批后的个人链接由管理人员交付。

### 备份数据库

应用运行期间可通过内置备份命令生成一致的 SQLite 快照，目标文件必须不存在。

生成快照并复制到宿主机：

```sh
backup_file="app-$(date -u +%Y%m%dT%H%M%SZ).db"
docker compose exec -T app server backup --to "/data/$backup_file" &&
  docker compose cp "app:/data/$backup_file" "./$backup_file"
```

定期备份并复制到服务器之外。确认副本可用后，删除容器 `/data` 中的临时快照，避免占满数据卷。不要直接复制运行中的数据库文件。

已有数据库执行待应用迁移前，服务还会在数据库旁生成 `.pre-migrate-v*.bak` 快照。这些本地快照不能替代异机备份。删除预约表不会清除旧备份中的记录，备份保留期需单独管理。

### 恢复备份

替换数据库前停止应用。选择要恢复的快照；如需保留现有数据，先备份当前数据库。

停止容器，将选定快照复制到容器中。把 `your_backup.db` 换成宿主机上的实际文件名：

```sh
docker compose stop app
docker compose cp ./your_backup.db app:/data/register_manager.db
```

仅在复制成功后修正文件归属，并删除被替换数据库的日志文件：

```sh
docker compose run --rm --no-deps --user root --entrypoint sh app -ec '
  chown 10001:10001 /data/register_manager.db
  rm -f /data/register_manager.db-wal /data/register_manager.db-shm
'
docker compose up -d
```

辅助容器挂载同一数据卷。重启后检查 `docker compose ps` 和应用日志。

### 更新或固定镜像版本

当前为预发布版本，旧开发数据库和旧 API 不保证兼容；不能把更换镜像视为受支持的旧开发库升级。旧 64 MiB Argon2id 密码参数也不被接受，没有自动密码迁移。

全新部署使用新数据库卷。已有开发数据需要保留时，先保留原镜像和完整备份，在独立部署中验证；不要直接覆盖唯一副本或删除原卷。正式发布后，已发布的迁移只追加，具体升级兼容性以对应发布说明为准。

先备份，再拉取并启动所选镜像：

```sh
docker compose pull
docker compose up -d
```

固定已发布版本时，在 `compose.yaml` 所在目录创建或更新 `.env`，将 `RM_IMAGE` 设置为 `ghcr.io/next-page-vi/register_manager:sha-完整提交哈希`，使用该版本的实际哈希，再执行相同命令。数据库结构发生变化时，旧镜像可能需要对应版本的数据库备份。

`:latest` 随 `main` 的成功构建更新，不表示稳定版本。需要可重复部署时固定 `:sha-` 标签，并保留该镜像对应的备份。

### 理解统计数据

预约表统计按当前预约表时区中的预约开始日期或创建日期计算。日期范围含首尾，最多 366 天；不超过 31 天按日分组，不超过 180 天按周分组，更长范围按月分组。周从星期一开始。

总数包含已取消预约及其客户。状态取当前记录，因此改期和状态修正会改变历史报表。系统不会自动把过往预约标记为已完成或未到场。

## English

Use this guide to configure, update, and maintain your Docker Compose deployment. Complete the [deployment steps](../README.md) first, then run the commands below from `deploy/`.

### Configure the container

Compose pulls `ghcr.io/next-page-vi/register_manager:latest` by default, without an `.env` file. To override the image, create `.env` beside `compose.yaml` and set `RM_IMAGE`; the repository's `.env.example` is an optional template. Keep the directory name `deploy` when moving deployment files on the same server: Compose uses its project name to identify the existing data volume.

Add application settings to the `environment` section of [`compose.yaml`](compose.yaml). Adding them to `.env` alone does not pass them into the container.

| Variable | Deployment default | Purpose |
| --- | --- | --- |
| `RM_IMAGE` | `ghcr.io/next-page-vi/register_manager:latest` | Optional override for the image address used by Compose |
| `RM_DB_PATH` | `/data/register_manager.db` | SQLite database inside the persistent volume |
| `RM_LISTEN` | `:8080` | HTTP listen address inside the container |
| `RM_TRUSTED_PROXY_HEADER` | `X-Forwarded-For` | Client-address header used for rate limiting |
| `RM_COOKIE_SECURE` | `true` | Secure cookies; disable only for local HTTP testing |
| `RM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `GOMEMLIMIT` | `96MiB` | Go's soft runtime memory limit, not a hard RSS limit |

The application listens on the host's loopback port 8080. Configure a reverse proxy on the host to serve HTTPS, preserve the public host, and set `X-Forwarded-For` from the client connection. Do not forward a client-supplied address header unchanged.

Run a single application instance. Store the database volume on local disk, not a network filesystem. Container replacement preserves the volume; `docker compose down -v` deletes it.

To inspect container state and recent logs:

```sh
docker compose ps
docker compose logs --tail=100 app
```

The image checks `/healthz` every 30 seconds. The endpoint verifies database connectivity.

### Configure an HTTPS reverse proxy

Use the [host Nginx configuration example](nginx.conf.example). Prepare a domain, DNS, and TLS certificate. Replace `booking.example.com` and the certificate paths, then install the file in a directory included inside Nginx's `http` block, such as `/etc/nginx/conf.d/register_manager.conf`. This example faces clients directly, without another proxy in front of Nginx.

Validate the configuration before reloading the host's Nginx:

```sh
sudo nginx -t && sudo nginx -s reload
```

The example preserves the public Host, replaces client-supplied `X-Forwarded-For`, and logs paths without query parameters. Credential entry points disable error logging because Nginx errors may contain complete request URLs. Access logs still record their status codes. If the application is unreachable, inspect container state, application logs, and `/healthz`.

Personal links contain reusable login credentials. Anyone holding a link can book, view, and cancel as that customer. Protect links and QR codes like passwords and deliver them only to the intended customer. Regenerate a leaked or lost link under **Customers** to invalidate the old link and existing sessions. Keep registration invite codes out of public logs too.

Apply the same query-parameter, cookie, and authentication filtering to other proxies, CDNs, and monitoring services, including error logs and request traces. Forward the original query parameters to the application; remove them only from logs. See the [Nginx logging documentation](https://nginx.org/en/docs/http/ngx_http_log_module.html).

### Set up accounts and calendars

The first start creates exactly one super admin, named `admin`, and prints its initial password in the logs. Sign in at `/account/login` on your HTTPS domain and change the password.

Configure booking in this order:

1. Create operators under **Accounts**.
2. Create a calendar under **All calendars**, set its time zone and default language, and assign its operators. New calendars start paused.
3. Set **Working hours** and **Booking rules**, then open booking under **Settings**.
4. Share the registration QR code. Approve registrations under **Customers**, then give each customer their personal access link or QR code.

Personal access links appear once when issued and can be revoked. The super admin cannot be deleted or disabled. Each account can change its login name and password under **My account**.

Recover a lost super-admin login or password with:

```sh
docker compose exec app server admin reset-password
```

The command prompts for a password and prints the login name. Before the first application start, you can choose the initial password with:

```sh
docker compose run --rm --no-deps app admin reset-password
```

Passwords come from the terminal or standard input, never command arguments.

### Understand usage limits

Confirm these behaviors fit your workflow before opening booking:

- Each calendar has one service timeline. Non-cancelled appointments cannot overlap within that calendar. Assigning more operators does not add simultaneous service capacity.
- Calendars calculate occupied time and quotas independently, even when they share an operator. They do not prevent bookings across calendars from overlapping or merge customer identities.
- Schedules, blocks, buffers, quotas, and notice periods govern customer self-booking. Staff creation and rescheduling check permission, future intervals, and actual service overlap within the calendar. Staff can choose any duration or overnight interval; staff cancellation bypasses cancellation notice.
- Pausing stops new customer registration and self-booking. Existing customers can still sign in, view, and cancel under the rules; staff can still create and reschedule. Pausing neither cancels appointments nor revokes links.
- Changing schedules or policies neither adjusts existing appointments nor checks their conflicts. Deleting a calendar permanently removes its business data without a recycle bin; recovery requires a full database backup.
- The system sends no SMS or email notifications and provides no payments, waitlists, calendar sync, or self-service link recovery. Staff deliver personal links after approval.

### Back up the database

Use the built-in backup command to create a consistent SQLite snapshot while the application runs. The target file must not already exist.

Create a snapshot and copy it to the host:

```sh
backup_file="app-$(date -u +%Y%m%dT%H%M%SZ).db"
docker compose exec -T app server backup --to "/data/$backup_file" &&
  docker compose cp "app:/data/$backup_file" "./$backup_file"
```

Schedule backups and copy them off the server. After verifying the copied snapshot, remove the temporary copy from the container's `/data` directory to avoid filling the volume. Do not copy the live database file.

Before applying pending migrations to an existing database, the server also writes a `.pre-migrate-v*.bak` snapshot next to it. These local snapshots do not replace off-server backups. Deleting a calendar does not remove its records from older backups; manage backup retention separately.

### Restore a backup

Stop the application before replacing its database. Choose the snapshot you intend to restore, and retain a backup of the current database if you need it.

Stop the container, then copy your selected snapshot into it. Replace `your_backup.db` with the actual host filename:

```sh
docker compose stop app
docker compose cp ./your_backup.db app:/data/register_manager.db
```

Only after the copy succeeds, fix file ownership and remove journal files belonging to the replaced database:

```sh
docker compose run --rm --no-deps --user root --entrypoint sh app -ec '
  chown 10001:10001 /data/register_manager.db
  rm -f /data/register_manager.db-wal /data/register_manager.db-shm
'
docker compose up -d
```

The helper container mounts the same data volume. Check `docker compose ps` and the application logs after restarting.

### Update or pin an image

This is a pre-release. Older development databases and APIs are not guaranteed compatible; changing the image is not a supported upgrade for those databases. The former 64 MiB Argon2id password profile is also unsupported, without automatic password migration.

Use a new database volume for a fresh deployment. To retain development data, keep the original image and a full backup, then validate a separate deployment. Do not overwrite the only copy or delete its volume. After formal release, published migrations are append-only; consult each release's upgrade notes for compatibility.

Back up first, then pull and start the selected image:

```sh
docker compose pull
docker compose up -d
```

To pin a published version, create or update `.env` beside `compose.yaml` and set `RM_IMAGE=ghcr.io/next-page-vi/register_manager:sha-full_commit_hash`, replacing `full_commit_hash` with that version's full commit hash. Run the same commands. An older image may require its matching database backup if the schema changed.

`:latest` follows successful `main` builds and does not identify a stable release. Pin a `:sha-` tag for reproducible deployments and retain its matching backup.

### Interpret statistics

Calendar statistics use appointment start dates or creation dates in the calendar's current time zone. The inclusive range supports up to 366 days, grouped daily through 31 days, weekly through 180 days, and monthly beyond that. Weeks start on Monday.

Totals include cancelled appointments and their customers. Statuses reflect current records, so rescheduling and corrections change past reports. The application does not automatically mark past appointments completed or no-show.
