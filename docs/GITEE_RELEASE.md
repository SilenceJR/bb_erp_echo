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
- `TAURI_SIGNING_PRIVATE_KEY`：密码保护的 Tauri updater 私钥内容，只用于正式标签签名。
- `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`：上述私钥的密码。

Repository/Environment Variables：

- `GITEE_SOURCE_OWNER`
- `GITEE_SOURCE_REPO`
- `GITEE_RELEASE_OWNER`
- `GITEE_RELEASE_REPO`
- `TAURI_UPDATER_PUBLIC_KEY`：`tauri signer generate` 生成的整个 `.pub` 文件内容（官方 Base64 envelope），同时注入客户端并写入服务端发布包；不要手工解包或只复制内部第二行。

Token、签名私钥和密码不应出现在仓库文件、构建包、manifest 或日志中。若 Gitee 的令牌模型不能按单仓库授权，使用专门的发布机器人账号，并只把该账号加入上述两个仓库。签名私钥必须另存一份加密离线备份；确认私钥和密码都可恢复前不得发布首个 v2 基线标签。

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
3. 上传 server、client、all-in-one、updater ZIP，以及签名 NSIS、便携 EXE和可选 zstd 差分。
4. 动态读取 manifest 资源集合，匿名下载全部附件，复验大小、SHA-256 和签名字段。
5. 仅在全部复验成功后更新公开仓库 `main/update-manifest.json`。
6. 再次匿名读取稳定 manifest 并确认内容一致。

稳定地址为：

```text
https://gitee.com/SilenceJR/bb_erp_releases/raw/main/update-manifest.json
```

附件地址为：

```text
https://gitee.com/SilenceJR/bb_erp_releases/releases/download/<标签>/<文件名>
```

任一上传或复验失败时，稳定 manifest 不会被更新。已存在的同版本 Release 不会自动覆盖，避免重跑时静默替换已发布二进制；需要修复时应发布新的预发布或补丁版本标签。

全量便携包解压后的目录约定如下：

```text
启动系统.bat
server/bb-erp-server.exe
server/web/dist/...
client/bb_erp_client.exe
client/bb-erp-portable.json
installer/<Tauri 安装器>
```

`启动系统.bat` 从包根目录运行，启动服务端后查找 `client/*.exe`。构建流程会在压缩前校验便携客户端是否位于该路径，避免把客户端包的 `client` 子目录重复嵌套到全量包中。

## 首次实际验收

外部仓库、镜像、Secrets 和 Variables 配置完成后，用预发布标签执行一次闭环，例如：

```bash
git tag v0.1.0-rc.1
git push origin v0.1.0-rc.1
```

增量升级首次发布固定分两步：先发布 `v0.1.0-rc.4` 作为 v2 全量基线（没有可用的上一份 v2 manifest，因此不得生成差分）；完成 Windows 10/11 的 `rc.3 → rc.4` 全量安装、重启和配置保留验证后，再发布仅含无害版本变化的 `v0.1.0-rc.5`。`rc.5` 必须从稳定 `rc.4` 便携 EXE生成补丁、回放并逐字节等于新 EXE；补丁达到 NSIS 大小 80% 时按设计只发布全量。

验收以下结果：Gitee 标签同步到 GitHub；Actions 三项验证和 Windows 构建成功；Gitee Release 中 manifest 声明的全部附件可匿名下载；稳定 manifest 中版本、URL、大小、SHA-256 和签名一致；`rc.4 → rc.5` 精确版本/哈希选择差分，`rc.3 → rc.5` 选择全量；国内内网服务电脑可访问稳定地址并由管理员立即检查、缓存客户端包。真实网络耗时、差分节省比例、断网/损坏/不可写/回滚结果记录在发布验收单中。

首次闭环已于 2026-08-26 16:35 CST 完成：

- 源码提交：`f26d8c62081a8821c973972a28a9b9d2e1d8a091`
- 预发布标签：`v0.1.0-rc.3`
- [GitHub Actions #18](https://github.com/SilenceJR/bb_erp_echo/actions/runs/32942763389)：Go、Web、Tauri 前端、Windows 打包和 Gitee 发布全部成功，总耗时 1 小时 6 分 7 秒。
- [Gitee 预发布版本](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.1.0-rc.3)：四个 Windows ZIP 均可匿名下载。
- [稳定 manifest](https://gitee.com/SilenceJR/bb_erp_releases/raw/main/update-manifest.json)：版本为 `0.1.0-rc.3`；四个附件的 URL、实际字节数和 SHA-256 经本地匿名下载复验一致。

首次联调中发现并修复了 Gitee API 对不存在 Release 返回 `200 null`、对不存在文件返回 `200 []` 的兼容问题。`v0.1.0-rc.1` 和 `v0.1.0-rc.2` 保留用于故障追踪；稳定 manifest 只在 `v0.1.0-rc.3` 的全部附件复验成功后更新。GitHub 向 Gitee 上传约 51.5 MB 发布附件耗时约 53 分钟，后续正式发布需为该阶段预留足够时间。

参考：[Gitee 仓库镜像说明](https://blog.gitee.com/2021/07/15/repo-mirror/)、[Gitee Release 附件下载路由](https://blog.gitee.com/2022/08/18/update/)、[GitHub Actions Artifact](https://github.com/actions/upload-artifact)。
