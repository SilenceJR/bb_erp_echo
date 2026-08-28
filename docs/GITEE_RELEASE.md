# Gitee 主仓库与发布闭环

本项目采用“Gitee 主仓库 → GitHub 只读镜像 → GitHub Actions Windows 构建 → Gitee 公开发布仓库”的单向链路。源码仓库可保持私有；公开发布仓库只保存正式版 `update-manifest.json` 和正式版 Release 附件。RC 与手动构建只保留 GitHub Actions 临时 Artifact，不进入正式版更新链路。

## 仓库与本地 remote

准备三个仓库：

1. Gitee 源码主仓库：日常分支、合并和版本标签的唯一写入入口。
2. GitHub 镜像仓库：接收 Gitee Push 镜像，只运行 Actions，不直接开发或打标签。
3. Gitee 公开发布仓库：默认分支为 `main`，只保存正式版 manifest 和二进制 Release。

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
- `TAURI_UPDATER_PUBLIC_KEY`：`tauri signer generate` 生成的完整 `.pub` 文件或其 Base64 封装；正式构建会校验 Minisign 公钥长度、Base64 内容和 key identifier。若发布环境只保存了唯一公钥 Base64 行，CI 会根据公钥内容补齐匹配的注释并重新封装，不会改变密钥材料。

Token、签名私钥和密码不应出现在仓库文件、构建包、manifest 或日志中。若 Gitee 的令牌模型不能按单仓库授权，使用专门的发布机器人账号，并只把该账号加入上述两个仓库。签名私钥必须另存一份加密离线备份；确认私钥和密码都可恢复前不得发布首个 v2 基线标签。

## 触发与发布顺序

| 事件 | Go/Web/Tauri 前端验证 | Windows 临时 Artifact | 正式 Gitee Release |
| --- | --- | --- | --- |
| 任意分支 push | 是 | 否 | 否 |
| GitHub PR | 是 | 否 | 否 |
| `main`/`master` push | 是 | 否 | 否 |
| 手动 `workflow_dispatch` | 是 | 是，保留 14 天 | 默认否；填写已有正式 `release_tag` 时可安全重试该标签的发布，输入类型为字符串 |
| 合法正式版 `vMAJOR.MINOR.PATCH` 标签 | 是 | 是 | 是，更新正式版 manifest |
| 合法预发布 `vMAJOR.MINOR.PATCH-prerelease` 标签 | 是 | 是 | 否，仅用于独立测试 |

非法的 `v*` 标签会在 Windows 构建开始前失败。构建会把标签的前导 `v` 去掉后注入 Go 和 Tauri，例如 `v1.2.3-rc.1` 对应应用版本 `1.2.3-rc.1`。预发布版本不会生成正式版签名更新清单，也不会运行 Gitee 发布任务。

正式发布由 `scripts/publish-gitee-release.sh` 执行：

1. 查询 Gitee 源码仓库并确认同名标签提交等于本次构建所对应的 GitHub 标签提交；已有标签手动重试时不会把修复提交误当成标签提交。
2. 在公开发布仓库创建标签和 Release。
3. 按运行号 Artifact 名称逐个下载 server、client、all-in-one、updater ZIP、`update-manifest.json` 和便携客户端目录；构建侧单文件 Artifact 使用 GitHub 官方默认标准归档，避免跨 Job 直接文件下载的兼容边界。下载后仍由发布脚本严格校验清单资源集合。不再额外生成重复的 `gitee-release-assets-*` 中转包；标准归档只存在于内部 Artifact，公开 Gitee 附件仍是原始发布文件。单个上传设置连接和总时限，避免 Gitee 附件接口无响应时无限等待。
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

任一上传或复验失败时，正式版 manifest 不会被更新。发布脚本会拒绝预发布标签，即使被误调用也不能覆盖正式版 manifest。已存在的同版本 Release 不会自动覆盖，避免重跑时静默替换已发布二进制。

正式发布的 `all-in-one` 已包含 `client/bb_erp_client.exe` 和 `client/bb-erp-portable.json`，可用于全量部署；但客户端更新清单仍需要根目录的独立签名便携 EXE，不能用全量包替代。`bb-erp-client-windows-portable-*` Artifact 只保留为构建验收和发布时提取该独立 EXE/清单，`gitee-release-assets-*` 不再重复保存同一组文件。若正式标签的发布作业仅在下载 Artifact 阶段失败，可从 Actions 手动填写已有 `release_tag` 重试；作业会重新校验该标签的源码提交，不会覆盖已有 Release 或改写标签。

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

RC 或手动构建只用于生成独立测试包：

```bash
git tag v0.1.0-rc.1
git push origin v0.1.0-rc.1
```

在对应 GitHub Actions 运行中下载并解压 `bb-erp-client-windows-portable-*` Artifact，保持 `bb-erp-client-windows-x86_64.exe` 与 `bb-erp-portable.json` 在同一目录后运行。该客户端只作为测试副本使用，不修改正式版 manifest，也不作为正式版自动更新来源；该 Artifact 是多文件归档，不能按单文件 Artifact 处理。

正式版验收使用不带预发布标识的标签。只有正式版标签才生成签名 v2 更新清单、上传 Gitee Release，并在全部附件通过匿名下载、大小、SHA-256、签名和版本递增复验后更新正式版 manifest。差分升级仍只以正式版上一版本为基线。

验收以下结果：正式版客户端能访问正式 manifest 并正常完成检查、差分/完整更新、重启和失败恢复；RC/手动 Artifact 能在独立目录启动；预发布标签不会触发 Gitee 发布或修改正式版 manifest；Windows 10/11 的安装、断网、损坏资源、不可写目录和回滚结果记录在发布验收单中。

## 0.0.2 → 0.0.3 发布实施记录

本次正式发布原计划采用当前 `main` 代码作为 `0.0.2` 基线，保留远程既有 `v0.0.1` 标签及其提交历史，不覆盖已有远程对象。`v0.0.2` 的两次 Gitee 发布尝试均在串行附件上传阶段长时间无响应，已取消并清理未完成的公开 Release；源码 `v0.0.2` 标签保持原提交不变。第一轮兜底的 `v0.0.3` 改为并行上传后，四个大附件仍在 900 秒时限内以 0 字节响应超时，已取消并清理未完成的公开 Release。按第二轮兜底方案，当前含 Gitee multipart 表单认证修复的 `main` 将作为 `0.0.4` 正式候选。

由于公开发布仓库仍存在历史 `0.1.0-rc.3` 稳定清单，发布脚本增加了严格的兜底迁移条件：只有目标版本为 `0.0.2`、`0.0.3` 或 `0.0.4`、当前稳定版本精确为 `0.1.0-rc.3`，且 CI 显式传入迁移标志时才允许覆盖稳定清单；其他版本继续执行正常的 SemVer 递增保护。历史 RC Release 和源码提交保留不变。

发布顺序固定为：

1. 既有 `v0.0.2` 和 `v0.0.3` 发布均因 Gitee 大附件 API 超时失败，均不改写源码标签。
2. 推送当前 `main` 的 `v0.0.4`，等待 GitHub 镜像同步；确认 Go、Web、Tauri、Windows 构建、签名、Release 附件匿名下载、大小、SHA-256 和稳定 manifest 校验全部通过。
3. 记录 `0.0.4` 的 CI 与 Release 验收结果，使用中文提交信息 `发布：记录 0.0.4 验收`。
4. 确认其客户端更新基线来自可用的前一正式版本；差分包不满足体积阈值时，必须验证完整签名包回退。

`0.0.4` 不新增业务 API 或功能代码，仅通过标签注入生成正式客户端，用于验证从可用的前一正式版本升级、重启和失败恢复。用户应从可用的前一正式全量包开始测试，并确认数据库、上传图片、配置和日志未被升级覆盖。

### 历史记录

此前的 `v0.1.0-rc.3` 曾按旧流程发布并写入稳定 manifest。该记录仅用于历史追踪；本次 `0.0.2` 通过严格的一次性迁移条件建立新的正式稳定基线，保留 RC Release 和源码历史。

首次闭环已于 2026-08-26 16:35 CST 完成：

- 源码提交：`f26d8c62081a8821c973972a28a9b9d2e1d8a091`
- 历史预发布标签：`v0.1.0-rc.3`
- [GitHub Actions #18](https://github.com/SilenceJR/bb_erp_echo/actions/runs/32942763389)：Go、Web、Tauri 前端、Windows 打包和 Gitee 发布全部成功，总耗时 1 小时 6 分 7 秒。
- [Gitee 预发布版本](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.1.0-rc.3)：四个 Windows ZIP 均可匿名下载。
- [历史稳定 manifest](https://gitee.com/SilenceJR/bb_erp_releases/raw/main/update-manifest.json)：当时版本为 `0.1.0-rc.3`；四个附件的 URL、实际字节数和 SHA-256 经本地匿名下载复验一致。

首次联调中发现并修复了 Gitee API 对不存在 Release 返回 `200 null`、对不存在文件返回 `200 []` 的兼容问题。GitHub 向 Gitee 上传约 51.5 MB 发布附件耗时约 53 分钟，后续正式发布需为该阶段预留足够时间。

参考：[Gitee 仓库镜像说明](https://blog.gitee.com/2021/07/15/repo-mirror/)、[Gitee Release 附件下载路由](https://blog.gitee.com/2022/08/18/update/)、[GitHub Actions Artifact](https://github.com/actions/upload-artifact)。
