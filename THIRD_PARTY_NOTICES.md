# 第三方许可 / Third-party notices

[简体中文](#简体中文) · [English](#english)

## 简体中文

本项目源码使用 [MIT 许可证](LICENSE)。第三方组件保留各自的版权和许可条款；本项目的 MIT 声明不替代这些条款。

发布镜像在 `/usr/share/licenses/register_manager/` 随附以下文件：

| 路径 | 内容与来源 |
| --- | --- |
| `LICENSE` | 本项目许可证 |
| `THIRD_PARTY_NOTICES.md` | 本说明 |
| `go/` | go-licenses 根据服务构建依赖收集的许可、声明及必要源文件 |
| `go/LICENSE-go` | 构建所用 Go 工具链的许可证 |
| `frontend.md` | Vite 根据前端构建收集的依赖版本、许可证和许可正文 |

前端许可清单也生成在 `web/dist/THIRD_PARTY_LICENSES.md`，并嵌入 Go 服务，可从网站的 `/THIRD_PARTY_LICENSES.md` 获取。构建产物不纳入 Git，每次镜像构建会重新生成。依赖版本以 `go.mod`、`go.sum`、`web/package.json` 和 `web/pnpm-lock.yaml` 为准。

重新分发镜像或拆分发布二进制、前端文件时，保留适用的版权和许可声明。修改依赖后检查生成文件；收集工具不替代对新增依赖许可条件的审阅。基础镜像 Alpine Linux 中的系统软件仍适用各自的许可。

## English

This project's source uses the [MIT license](LICENSE). Third-party components retain their own copyrights and license terms, which the project's MIT notice does not replace.

Published images include these files under `/usr/share/licenses/register_manager/`:

| Path | Contents and source |
| --- | --- |
| `LICENSE` | Project license |
| `THIRD_PARTY_NOTICES.md` | This notice |
| `go/` | Licenses, notices, and required source files collected by go-licenses from the server's build dependencies |
| `go/LICENSE-go` | License from the Go toolchain used for the build |
| `frontend.md` | Dependency versions, licenses, and license text collected by Vite during the frontend build |

The frontend notice is also generated as `web/dist/THIRD_PARTY_LICENSES.md`, embedded in the Go service, and served at `/THIRD_PARTY_LICENSES.md`. Build output stays outside Git and is regenerated for each image. Dependency versions come from `go.mod`, `go.sum`, `web/package.json`, and `web/pnpm-lock.yaml`.

Retain applicable copyright and license notices when redistributing images or distributing the binary and frontend separately. Inspect generated notices after dependency changes; collection tools do not replace reviewing new dependencies' license terms. System software in the Alpine Linux base image remains subject to its respective licenses.
