# Gitee 主仓库与发布闭环

本项目采用“Gitee 主仓库 → GitHub 只读镜像 → GitHub Actions Windows 构建 → Gitee 公开发布仓库”的单向链路。源码仓库可保持私有；公开发布仓库只保存 `update-manifest.json` 和版本 Release 附件。

## 仓库与本地 remote

准备三个仓库：

1. Gitee 源码主仓库：日常分支、合并和版本标签的唯一写入入口。
2. GitHub 镜像仓库：接收 Gitee Push 镜像，只运行 Actions，不直接开发或打标签。
3. Gitee 公开发布仓库：默认分支为 `main`，只保存稳定 manifest 和二进制 Release。

取得实际 Gitee 地址后，在每个开发工作副本执行：

```bash
git remote rename origin github
git remote add origin https://gitee.com/<源码空间>/<源码仓库>.git
git fetch origin --prune --tags
git branch --set-upstream-to=origin/main main
```

当前仓库没有写死空间和仓库名；不要把占位符提交成真实 remote。功能开发统一使用 `codex/<主题>` 分支，验证通过后合并到 Gitee 主分支。

## Gitee 到 GitHub 镜像

在 Gitee 源码仓库配置 Push 镜像，目标为 GitHub 镜像仓库，并同步所有分支和标签。镜像存在数分钟延迟时，GitHub Actions 发布任务会在“Gitee 源标签校验”处安全失败；确认镜像完成后重新运行任务即可。

若账号没有内置 Push 镜像能力，使用 Gitee Go 的 `mirror-to-github` 流水线完成相同的单向同步。GitHub Token 只授予目标镜像仓库的 Contents 写权限，不授予组织级管理权限。禁止配置 GitHub → Gitee 的反向自动同步，避免双向分叉。

## GitHub Actions 配置

在 GitHub 镜像仓库配置 Environment `gitee-release`，并为它增加审批人和以下值：

Secrets：

- `GITEE_TOKEN`：公开发布仓库的标签、Release、附件和 `main/update-manifest.json` 写入凭据。
- `GITEE_SOURCE_TOKEN`：可选，私有源码仓库的只读凭据，仅用于核对标签提交。未配置时暂用 `GITEE_TOKEN`；生产环境建议拆分。

Repository/Environment Variables：

- `GITEE_SOURCE_OWNER`
- `GITEE_SOURCE_REPO`
- `GITEE_RELEASE_OWNER`
- `GITEE_RELEASE_REPO`

`GITEE_TOKEN` 不应出现在仓库文件、构建包、manifest 或日志中。若 Gitee 的令牌模型不能按单仓库授权，使用专门的发布机器人账号，并只把该账号加入上述两个仓库。

## 触发与发布顺序

| 事件 | Go/Web/Tauri 前端验证 | Windows 临时 Artifact | 正式 Gitee Release |
| --- | --- | --- | --- |
| 任意分支 push | 是 | 否 | 否 |
| GitHub PR | 是 | 否 | 否 |
| `main`/`master` push | 是 | 否 | 否 |
| 手动 `workflow_dispatch` | 是 | 是，保留 14 天 | 否 |
| 合法 `vMAJOR.MINOR.PATCH[-prerelease]` 标签 | 是 | 是 | 是 |

非法的 `v*` 标签会在 Windows 构建开始前失败。正式构建把标签的前导 `v` 去掉后注入 Go 和 Tauri，例如 `v1.2.3-rc.1` 对应应用版本 `1.2.3-rc.1`。

正式发布由 `scripts/publish-gitee-release.sh` 执行：

1. 查询 Gitee 源码仓库并确认同名标签提交等于 GitHub Actions 的 `GITHUB_SHA`。
2. 在公开发布仓库创建标签和 Release。
3. 上传 server、client、all-in-one 和 updater ZIP。
4. 匿名下载全部附件，复验大小、SHA-256 和 updater 内容。
5. 仅在全部复验成功后更新公开仓库 `main/update-manifest.json`。
6. 再次匿名读取稳定 manifest 并确认内容一致。

稳定地址为：

```text
https://gitee.com/<发布空间>/<发布仓库>/raw/main/update-manifest.json
```

附件地址为：

```text
https://gitee.com/<发布空间>/<发布仓库>/releases/download/<标签>/<文件名>
```

任一上传或复验失败时，稳定 manifest 不会被更新。已存在的同版本 Release 不会自动覆盖，避免重跑时静默替换已发布二进制；需要修复时应发布新的预发布或补丁版本标签。

## 首次实际验收

外部仓库、镜像、Secrets 和 Variables 配置完成后，用预发布标签执行一次闭环，例如：

```bash
git tag v0.1.0-rc.1
git push origin v0.1.0-rc.1
```

验收以下结果：Gitee 标签同步到 GitHub；Actions 三项验证和 Windows 构建成功；Gitee Release 四个附件可匿名下载；稳定 manifest 中版本、URL、大小和 SHA-256 一致；国内内网服务电脑可访问稳定地址并由管理员立即检查、缓存客户端包。真实网络耗时和错误记录在发布验收单中。

参考：[Gitee 仓库镜像说明](https://blog.gitee.com/2021/07/15/repo-mirror/)、[Gitee Release 附件下载路由](https://blog.gitee.com/2022/08/18/update/)、[GitHub Actions Artifact](https://github.com/actions/upload-artifact)。
