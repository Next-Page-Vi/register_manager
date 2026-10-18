# 安全报告 / Security reporting

[简体中文](#简体中文) · [English](#english)

## 简体中文

请通过仓库的 [Report a vulnerability](https://github.com/Next-Page-Vi/register_manager/security/advisories/new) 私密报告安全问题。不要在公开 Issue、PR 或讨论中发布漏洞利用步骤、客户链接、密码、Cookie、数据库或个人信息。

如果私密报告入口暂不可用，请创建只包含“请求安全问题私密联系渠道”的 [Issue](https://github.com/Next-Page-Vi/register_manager/issues/new)，等待维护者提供渠道后再发送详情。

报告请包含：

- 受影响的镜像标签或完整提交哈希、运行环境
- 问题影响、前置条件和使用虚构数据的最小复现步骤
- 已知的缓解方式或修复建议（如有）

当前处于预发布阶段，安全修复面向最新 `main` 代码及其镜像，暂不维护旧提交的安全补丁。升级前阅读[数据库兼容边界](deploy/OPERATIONS.md#更新或固定镜像版本)。维护者会通过私密报告讨论复现、修复与公开时间，不承诺固定响应期限。

## English

Report security issues privately through [Report a vulnerability](https://github.com/Next-Page-Vi/register_manager/security/advisories/new). Do not publish exploit instructions, customer links, passwords, cookies, databases, or personal information in public issues, pull requests, or discussions.

If private reporting is unavailable, open an [issue](https://github.com/Next-Page-Vi/register_manager/issues/new) requesting a private security contact channel, without disclosing the issue. Wait for the maintainer to provide a channel before sending details.

Include the following in your private report:

- Affected image tag or full commit hash and runtime environment
- Impact, prerequisites, and minimal reproduction steps using fictional data
- Any known mitigation or suggested fix

During pre-release development, security fixes target the latest `main` code and its image. Older commits do not receive backported fixes. Read the [database compatibility limits](deploy/OPERATIONS.md#update-or-pin-an-image) before upgrading. The maintainer will discuss reproduction, fixes, and disclosure through the private report, without a fixed response-time commitment.
