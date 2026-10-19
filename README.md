# register_manager

[简体中文](#简体中文) · [English](#english)

[GitHub](https://github.com/Next-Page-Vi/register_manager) · [Issues](https://github.com/Next-Page-Vi/register_manager/issues)

## 简体中文

自托管预约系统：管理多个独立预约表，配置排班，审核客户，并通过个人链接预约。支持简体中文、繁体中文、英语和日语；单节点运行，数据保存在 SQLite 中。

当前为 `0.x` 早期版本。旧开发数据库和 API 不保证兼容，升级前阅读[兼容边界](deploy/OPERATIONS.md#更新或固定镜像版本)。

每个预约表只有一条服务时间线，不同预约表不共享操作员的时间占用。后台代约可绕过客户预约规则；产品范围和暂停行为见[使用边界](deploy/OPERATIONS.md#理解使用边界)。

### 部署

使用 Docker Compose 部署。准备 Linux x86-64 服务器、Docker Engine 和 Compose 插件，并按[反向代理说明](deploy/OPERATIONS.md#配置-https-反向代理)配置 HTTPS 和凭证日志保护，转发到宿主机的 `127.0.0.1:8080`。

镜像由 GitHub Actions 构建并发布到 GitHub Container Registry（GHCR）。无需克隆源码或在服务器构建，创建部署目录：

```sh
mkdir -p deploy
cd deploy
```

将以下完整配置保存为 `compose.yaml`，内容与仓库的[部署配置](deploy/compose.yaml)一致：

```yaml
services:
  app:
    image: ${RM_IMAGE:-ghcr.io/next-page-vi/register_manager:v0.1.0}
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      RM_TRUSTED_PROXY_HEADER: X-Forwarded-For
    volumes:
      - data:/data

volumes:
  data:
```

默认固定到公开的 `v0.1.0` 镜像，无需登录 GHCR 或创建 `.env`。拉取并启动：

```sh
docker compose pull
docker compose up -d
docker compose logs app
```

其他版本见 [GitHub Releases](https://github.com/Next-Page-Vi/register_manager/releases)。`latest` 跟随不含 `rc`、`beta` 等后缀的版本发布；`edge` 跟随 `main` 的成功构建。正式部署建议固定 `vX.Y.Z` 标签。

打开你的 HTTPS 域名下的 `/account/login`，使用账号 `admin` 和日志中的初始密码登录，立即修改密码。随后创建操作员和预约表，设置排班与预约规则，再开放预约。

### 更新与备份

更新前阅读[发布说明](https://github.com/Next-Page-Vi/register_manager/releases)和[兼容边界](deploy/OPERATIONS.md#更新或固定镜像版本)，并[备份数据库](deploy/OPERATIONS.md#备份数据库)。在 `deploy/.env` 中设置 `RM_IMAGE=ghcr.io/next-page-vi/register_manager:vX.Y.Z`，将 `vX.Y.Z` 替换为已发布的目标版本，再在 `deploy/` 中执行：

```sh
docker compose pull
docker compose up -d
```

数据保存在 Docker 命名卷 `data` 中，容器内路径为 `/data/register_manager.db`；`docker compose down -v` 会删除该数据卷。

[配置、备份、恢复与账号管理](deploy/OPERATIONS.md) · [开发与贡献](CONTRIBUTING.md) · [安全报告](SECURITY.md) · [MIT 许可证](LICENSE) · [第三方许可](THIRD_PARTY_NOTICES.md)

## English

A self-hosted appointment system with independent calendars, working hours, customer approval, and personal booking links. It supports English, Japanese, Simplified Chinese, and Traditional Chinese. Run one server with a SQLite database.

This project is in early `0.x` development. Older development databases and APIs are not guaranteed compatible. Read the [compatibility limits](deploy/OPERATIONS.md#update-or-pin-an-image) before upgrading.

Each calendar has one service timeline. Calendars do not share an operator's occupied time. Staff bookings can bypass customer rules; see [usage limits](deploy/OPERATIONS.md#understand-usage-limits) for product scope and paused-calendar behavior.

### Deploy

Deploy with Docker Compose on a Linux x86-64 server with Docker Engine and the Compose plugin. Follow the [reverse proxy instructions](deploy/OPERATIONS.md#configure-an-https-reverse-proxy) for HTTPS and credential-safe logging, forwarding to `127.0.0.1:8080` on the host.

GitHub Actions builds the image and publishes it to GitHub Container Registry (GHCR). Create a deployment directory; you don't need to clone the source or build on your server:

```sh
mkdir -p deploy
cd deploy
```

Save this complete configuration as `compose.yaml`. It matches the repository's [deployment configuration](deploy/compose.yaml):

```yaml
services:
  app:
    image: ${RM_IMAGE:-ghcr.io/next-page-vi/register_manager:v0.1.0}
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      RM_TRUSTED_PROXY_HEADER: X-Forwarded-For
    volumes:
      - data:/data

volumes:
  data:
```

The configuration pins the public `v0.1.0` image. You don't need a GHCR login or an `.env` file. Pull and start the image:

```sh
docker compose pull
docker compose up -d
docker compose logs app
```

See [GitHub Releases](https://github.com/Next-Page-Vi/register_manager/releases) for other versions. The `latest` tag follows releases without suffixes such as `rc` or `beta`; `edge` follows successful `main` builds. Pin a `vX.Y.Z` tag for production deployments.

Open `/account/login` on your HTTPS domain. Sign in as `admin` with the initial password from the logs and change it immediately. Create an operator and calendar, configure working hours and booking rules, then open booking.

### Update and back up

Read the [release notes](https://github.com/Next-Page-Vi/register_manager/releases) and [compatibility limits](deploy/OPERATIONS.md#update-or-pin-an-image), then [back up the database](deploy/OPERATIONS.md#back-up-the-database). Set `RM_IMAGE=ghcr.io/next-page-vi/register_manager:vX.Y.Z` in `deploy/.env`, replacing `vX.Y.Z` with the published target version. Run these commands from `deploy/`:

```sh
docker compose pull
docker compose up -d
```

Docker stores the database in the named `data` volume at `/data/register_manager.db` inside the container. Running `docker compose down -v` deletes that volume.

[Configuration, backups, recovery, and accounts](deploy/OPERATIONS.md) · [Contributing](CONTRIBUTING.md) · [Security reporting](SECURITY.md) · [MIT license](LICENSE) · [Third-party notices](THIRD_PARTY_NOTICES.md)
