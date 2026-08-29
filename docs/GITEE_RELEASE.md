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
3. 按运行号 Artifact 名称逐个下载 server、client、all-in-one、updater ZIP、manifest 资源组和便携客户端目录；manifest 资源组包含 `update-manifest.json` 及本次实际生成的 `.zstpatch`，不单独增加差分 Artifact。构建侧使用 GitHub 官方默认标准归档，避免跨 Job 直接文件下载的兼容边界。下载后仍由发布脚本严格校验清单资源集合。不再额外生成重复的 `gitee-release-assets-*` 中转包；标准归档只存在于内部 Artifact，公开 Gitee 附件仍是原始发布文件。
4. 正式附件按体积从小到大串行上传，禁用 `Expect: 100-continue` 并固定 HTTP/1.1；单次请求允许慢速大文件上传，但会在持续无上传进度时中止，整个发布作业设有总时限和有限次数重试。每次请求后调用 Gitee Release 附件列表接口确认结果，即使上传请求超时但服务端已保存附件，也不会重复上传。
5. 重跑失败作业时，发布脚本只恢复名称和发布说明均匹配的自动 Release，并复用名称、大小一致的已有附件；同名尺寸冲突或人工创建的同标签 Release 会停止发布。最终仍以匿名下载、大小和 SHA-256 为准。
6. 动态读取 manifest 资源集合，匿名下载全部附件，复验大小、SHA-256 和签名字段。
7. 仅在全部复验成功后更新公开仓库 `main/update-manifest.json`。
8. 再次匿名读取稳定 manifest 并确认内容一致。

稳定地址为：

```text
https://gitee.com/SilenceJR/bb_erp_releases/raw/main/update-manifest.json
```

附件地址为：

```text
https://gitee.com/SilenceJR/bb_erp_releases/releases/download/<标签>/<文件名>
```

任一上传或复验失败时，正式版 manifest 不会被更新。发布脚本会拒绝预发布标签，即使被误调用也不能覆盖正式版 manifest。已存在的同版本人工 Release 不会自动覆盖；由当前发布器创建的未完成 Release 可安全恢复，但不会替换已有同名附件。

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

正式发布作业不再单独上传 NSIS 中转 Artifact。签名安装器已包含在 `bb-erp-client-windows.zip` 的 `installer/` 目录中，发布作业下载该客户端包后提取 `*setup.exe`，再由发布脚本按 manifest 校验并上传。这样可以减少内部 Artifact 数量，同时保留客户端更新所需的独立签名安装器。

正式版验收使用不带预发布标识的标签。只有正式版标签才生成签名 v2 更新清单、上传 Gitee Release，并在全部附件通过匿名下载、大小、SHA-256、签名和版本递增复验后更新正式版 manifest。差分升级仍只以正式版上一版本为基线。

验收以下结果：正式版客户端能访问正式 manifest 并正常完成检查、差分/完整更新、重启和失败恢复；RC/手动 Artifact 能在独立目录启动；预发布标签不会触发 Gitee 发布或修改正式版 manifest；Windows 10/11 的安装、断网、损坏资源、不可写目录和回滚结果记录在发布验收单中。

## 0.0.2 → 0.0.6 发布实施记录

本次正式发布保留 `v0.0.1` 至 `v0.0.4` 标签及其提交历史，不覆盖远程对象。`v0.0.2` 和 `v0.0.3` 的 Gitee 大附件上传均超时并已清理未完成 Release；`v0.0.4` 完成全部 Windows 构建、Artifact 下载和 NSIS 提取，但 `bb-erp-all-in-one-windows.zip` 与独立便携 EXE 在 900 秒内仍以 0 字节响应超时，公开 Release 仅含部分附件，稳定 manifest 未更新。`0.0.5` 引入可恢复的串行上传与附件状态确认，作为新的正式基线；`0.0.6` 只用于验证相邻正式版本升级链路。

由于公开发布仓库仍存在历史 `0.1.0-rc.3` 稳定清单，发布脚本增加了严格的兜底迁移条件：只有目标版本为 `0.0.2`、`0.0.3`、`0.0.4` 或最终兜底基线 `0.0.5`，当前稳定版本精确为 `0.1.0-rc.3`，且 CI 显式传入迁移标志时才允许覆盖稳定清单；`0.0.6` 及后续版本继续执行正常的 SemVer 递增保护。历史 RC Release 和源码提交保留不变。

发布顺序固定为：

1. 保留 `v0.0.2`、`v0.0.3` 和 `v0.0.4` 失败发布的源码标签，不改写历史。
2. 清理公开发布仓库中附件不完整的 `v0.0.4` Release，保留其分发标签和源码历史。
3. 推送 `v0.0.5`；确认 Go、Web、Tauri、Windows 构建、签名、全部附件匿名下载、大小、SHA-256 和稳定 manifest 校验通过。
4. 记录 `0.0.5` 的发布验收，使用中文提交信息 `发布：记录 0.0.5 验收并准备 0.0.6`，再创建并推送 `v0.0.6`。
5. 确认 `0.0.6` 的差分基线来自 `0.0.5`；若差分包超过 NSIS 的 80%，验证签名完整包回退。

`0.0.5` 和 `0.0.6` 不新增业务 API；版本号由标签注入。用户先部署 `0.0.5` 全量包，再验证 `0.0.5 → 0.0.6` 的检查、差分或完整包回退、重启和失败恢复，并确认数据库、上传图片、配置和日志未被升级覆盖。

### 0.0.5 正式发布验收

- 源码提交和标签：`8aad538adefba4cef3e378a010af0a0a95a728cc` / `v0.0.5`。
- [GitHub Actions #63](https://github.com/SilenceJR/bb_erp_echo/actions/runs/33195183416) 的 Go、Web、Tauri、Windows 构建、签名、Artifact 下载和 Gitee 发布全部成功。
- 六个正式附件均在首次请求返回 HTTP 201；最大 all-in-one 实际上传约 27.4 MB、耗时约 743 秒。随后全部附件通过匿名下载、大小和 SHA-256 复验。
- [Gitee 0.0.5 Release](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.0.5) 已包含 updater、NSIS、client、server、独立便携 EXE 和 all-in-one；公开稳定 manifest 已验证为 `0.0.5`，并包含签名 v2 更新信息。
- 历史 `0.1.0-rc.3` manifest 不含 v2 payload，因此 `0.0.5` 是完整更新基线，不生成历史 RC 差分；Windows 真机安装和 `0.0.5 → 0.0.6` 用户升级仍待验收。

### 0.0.6 首次发布问题记录

- GitHub Actions #66 的 Go、Web、Tauri、Windows 构建和签名通过，并成功生成 `0.0.5 → 0.0.6` 差分包；差分约 3.88 MB，低于约 18.5 MB 完整便携 EXE，满足发布阈值。
- 首次发布在上传 Gitee 前停止：`update-manifest` Artifact 只包含 JSON，发布作业缺少 manifest 引用的 `.zstpatch`。
- 修复为将 JSON 与可选 `.zstpatch` 放入同一个 manifest 资源组 Artifact，不增加 Artifact 数量；正式 Release、稳定 manifest 和 Windows 用户升级仍未标记完成。

### 0.0.6 正式发布验收

- [GitHub Actions #69](https://github.com/SilenceJR/bb_erp_echo/actions/runs/33200710415) 使用 `main` 的修复 Workflow 和已有 `release_tag=v0.0.6` 完成重新构建；源码标签仍指向 `357148ce0dd56072f6eab595511f6b9f5fa39cd0`，未改写。
- update-manifest Artifact 同时包含 JSON 和 `.zstpatch`；[Gitee 0.0.6 Release](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.0.6) 已包含七个正式附件，均在首次请求返回 HTTP 201，并通过匿名大小与 SHA-256 复验。
- 稳定 manifest 已独立读取确认版本为 `0.0.6`；签名 v2 payload 的差分 `from_version` 为 `0.0.5`，差分大小 3,875,261 字节，完整便携 EXE 大小 18,523,136 字节，满足体积阈值。
- 技术发布链路已闭环；Windows 10/11 真机的 `0.0.5 → 0.0.6` 检查、应用、重启、完整包回退、断网恢复和业务数据保留仍待用户验收。

### 0.0.7 / 0.0.8 公钥恢复发布

- Windows 上的 `0.0.5` 验收在获取稳定 manifest 后、提交成功状态和缓存更新资源前失败，错误为 `load client update signing public key: ... illegal base64 data at input byte 3`。从 Gitee 重新下载的 `0.0.5` all-in-one 包内 `server/update-public.key` 已通过 Base64、Minisign 结构和当前 Go 解析器验证，故障不是发布公钥材料损坏。
- 根因是部署边界未封闭：Windows 启动脚本没有清除父进程遗留的 `BB_ERP_UPDATE_SIGNING_PUBLIC_KEY`，无效直接值会优先于包内文件；all-in-one 系统启动入口还使用了与服务端子进程工作目录不一致的公钥相对路径；服务端升级器也只备份但不替换 `update-public.key` 和新启动脚本。
- `0.0.7` 修复为：三个 Windows 启动入口显式清空直接公钥并锁定包内文件；直接值无效时可回退到显式配置且有效的公钥文件；服务端升级在停服前强制校验包和公钥，只替换服务端、公钥和 Web，不覆盖用户启动脚本，重启时优先复用该脚本的部署配置，并使用同目录分阶段切换与失败自动回滚；CI 在压缩前和压缩后都会验证公钥与启动入口。
- 因 `0.0.5/0.0.6` 无法依赖有效的更新检查发现修复版，`0.0.7` 必须使用全量包手动恢复一次；恢复时不覆盖数据库、上传图片、日志和更新缓存。
- `0.0.7` 的 CI、Gitee 附件、SHA-256、签名和稳定 manifest 已完成技术验收；下一步以纯文档提交发布 `0.0.8`，验证 `0.0.7 → 0.0.8` 差分基线、完整包回退、重启版本和业务数据保留。未完成的 Windows 真机结果不标记为已验收。

#### 0.0.7 正式发布验收

- [GitHub Actions #73](https://github.com/SilenceJR/bb_erp_echo/actions/runs/33227679610) 在提交 `dce3457136f0b29fcc7c9173bef4d1bf7dc06b8d` 上完成 Go、Web、Tauri、Windows 打包、Minisign 签名和 Gitee 发布，结论为成功。
- [Gitee 0.0.7 Release](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.0.7) 包含七个正式附件和两个源码归档；发布作业已逐项完成匿名下载、大小及 SHA-256 复验，随后才将稳定 manifest 更新为 `0.0.7`。
- 签名 v2 payload 包含从 `0.0.6` 出发的 3,875,276 字节差分，以及 5,425,854 字节 NSIS 和 18,523,136 字节便携 EXE 完整回退资源；差分满足体积阈值。
- 独立匿名下载的 27,402,203 字节 all-in-one 包 SHA-256 为 `b30f043a6066d90f59df26e7bd3b3d2294e34c5143b8177e21b9d90a44f7ac2c`，与稳定 manifest 一致；包内 `server/update-public.key`、便携标记、服务端/客户端 EXE 均存在，三个启动入口均清空直接公钥并使用同工作目录的 `update-public.key`。
- 以上只证明发布技术链路和公开产物一致性。Windows 10/11 的手动恢复、更新页公钥错误消失、业务数据读取以及 `0.0.7 → 0.0.8` 真机升级仍待用户验收。

#### 0.0.7 手动恢复步骤

1. 不得运行 `0.0.5/0.0.6` 目录里的旧 `bb-erp-updater.exe` 或“一键升级服务端.bat”；它们不会替换公钥和新启动脚本。
2. 停止旧服务端，将旧部署目录整体复制为带时间的备份；记录当前数据库关键记录、一张已上传图片、端口和当前版本。
3. 下载 `0.0.7` all-in-one 全量包，使用 Windows `Get-FileHash -Algorithm SHA256` 并与稳定 manifest 的 `all_in_one.sha256` 比对，然后解压到全新目录，不覆盖旧目录。
4. 从旧目录只复制 `server/data`、`server/static/uploads`、`server/logs` 和 `server/updates` 到新目录。不复制旧 `update-public.key`、启动脚本、EXE 或 `web/dist`。
5. 如旧启动脚本修改过端口、数据库/日志/上传目录或允许来源，只将这些业务值手动填入新脚本；保留新脚本的 manifest URL、`BB_ERP_UPDATE_SIGNING_PUBLIC_KEY=` 和 `BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key`。
6. 双击新目录的“启动系统.bat”。成功标准：服务端无公钥解析错误，页面显示当前版本 `0.0.7` 并能读取最新版本，数据库关键记录与上传图片可读，自定义端口/目录生效，新日志可写入。
7. 若任一成功标准失败，停止新服务端，不再修改新目录，直接从第 2 步的备份重启旧系统，并保留新目录日志用于排查。

#### 0.0.7 → 0.0.8 Windows 验收用例

Windows 10 和 Windows 11 分别记录以下结果；每项均需保存开始/结束版本、更新页状态、服务端与客户端日志、以及数据保留结果。

1. **前置基线**：使用已通过上述恢复的 `0.0.7`，创建一条可识别的业务记录、上传一张图片，并记录端口、数据库/上传/日志/更新缓存目录及 `0.0.7` 日志基线。
2. **更新发现**：点击“立即检查”，预期不再出现公钥错误，最新版本显示 `0.0.8`，稳定 manifest 和签名 payload 版本一致。
3. **正常差分**：保留原始 `0.0.7` 便携 EXE 与布局标记，执行更新；预期计划选择 `0.0.7` 差分，签名、SHA-256 和大小验证通过，重启后显示 `0.0.8`。
4. **服务端缓存降级**：在隔离的测试副本中，等服务端缓存 `0.0.8` 差分后，备份并用无效内容替换该 `.zstpatch` 缓存文件，保留文件名；预期服务端在生成新计划时拒绝损坏缓存，直接下发签名完整包计划。本项只验收服务端降级，不冒充客户端 `delta → full` 分支。完成后删除损坏缓存并刷新更新状态。
5. **客户端差分到完整包回退**：在隔离的便携客户端副本中，保持原始 `0.0.7` EXE/布局标记和有效差分缓存，在点击更新前将客户端目录设为不可写。预期服务端仍下发 `delta` 计划，客户端差分应用因目标不可写而失败后进入签名 NSIS 完整安装流程，状态中记录 `strategy=full` 和回退原因。完成 NSIS 后新客户端为 `0.0.8`；如取消或安装失败，原 `0.0.7` 便携 EXE 仍可启动。
6. **断网恢复**：在下载前断网并执行更新；预期给出可恢复的下载错误，原 EXE 和 `0.0.7` 状态保留，恢复网络后可重试。
7. **新版启动失败**：在隔离副本中受控阻止新客户端完成启动就绪标记；预期 90 秒内恢复旧 EXE，原 `0.0.7` 可再次启动，备份和失败原因留在日志。
8. **数据保留**：每个场景结束后确认数据库关键记录、上传图片、端口与目录配置、旧/新日志和更新缓存均未被静默覆盖；任一项不符合即标记验收失败，不使用“已完成”结论。

#### 0.0.8 正式发布验收

- [GitHub Actions #75](https://github.com/SilenceJR/bb_erp_echo/actions/runs/33231182275) 在纯文档提交 `beb562e809b813266ff3958d8b323202e4eb3023` 上完成全部验证、Windows 重新构建、Minisign 签名、差分生成和 Gitee 发布，结论为成功。
- [Gitee 0.0.8 Release](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.0.8) 包含七个正式附件和两个源码归档；发布作业在所有附件完成匿名下载、大小及 SHA-256 复验后，才将稳定 manifest 更新为 `0.0.8`。
- 签名 v2 payload 的差分明确为 `0.0.7 → 0.0.8`，大小 3,875,708 字节；完整回退资源为 5,425,048 字节 NSIS 和 18,523,136 字节便携 EXE，差分满足体积阈值。
- 独立匿名下载的 27,401,392 字节 all-in-one 包 SHA-256 为 `526463a3673db984ae47169c2cb1f987715fe968e0f1a51a27501b86283f3b33`，与稳定 manifest 一致；包内 `version.json` 的服务端和客户端版本均为 `0.0.8`，公钥与三个启动入口继续满足修复约束。
- 0.0.7/0.0.8 的技术发布链路已经闭环；Windows 10/11 的 0.0.7 手动恢复、更新页检查、0.0.7→0.0.8 实际应用、重启、回滚、断网和业务数据保留仍按上述用例待用户验收。

#### 0.0.8 Windows 用户验收补充：服务端升级失败

- 用户已确认客户端升级验收通过；服务端升级未通过，不将整体升级闭环标记为完成。
- “下载升级包”此前直接使用 manifest 的 Gitee URL，Tauri WebView 可能静默拦截新窗口；修复为管理页调用受 `system:updates:read` 保护的同源下载接口，由 Go 服务端按成功 manifest 下载并校验大小、SHA-256 与 ZIP，页面显示进行中、成功或具体失败。
- 0.0.8 服务端包内“一键升级服务端.bat”调用了并不存在于解压目录内的二次 `bb-erp-server-windows.zip`；无参数 updater 没有默认 manifest、持久日志或暂停行为，所以 EXE/BAT 双击后直接消失。该旧脚本不得继续用于验收。
- 修复后的正式包必须确认：`version.json` 含标签版本和稳定 `manifest_url`；manifest 的服务端资源含由现有 Tauri/Minisign 私钥生成且可由已部署公钥流式验证的签名，包大小不超过 512 MiB；稳定加载器可激活暂存 updater/runner，runner 只按安装目录和可选明确 Service 名执行且保留退出码和 `pause`；显式 Service 必须实际指向目标目录的服务端 EXE，手工 `-package` 必须同时传入 manifest 的 `server.signature`；压缩前后 CI 都拒绝缺失元数据、引用内嵌 ZIP、不能传播升级器或隐藏日志的 server/all-in-one 包。
- 0.0.8 不具备可信服务端包签名和可传播的加载器，首次恢复必须把下一正式版 all-in-one 全量包部署到新目录并只迁移业务数据；不要复用 0.0.8 的 updater/批处理，也不要先复制新版 `version.json` 到旧目录。确认修复基线运行正常后，再用其“一键升级服务端.bat”验收下一相邻版本，才能同时覆盖可信签名、runner 激活和回滚链路。
- 下一正式版需在 Windows 10/11 复验：更新中心下载反馈、缓存复用、断网/超时/签名不符/损坏包错误、重复双击互斥、批处理窗口保留、日志内容、同路径进程停止与重启、Windows Service 显式服务名、updater/runner 下一轮激活、版本显示、失败回滚，以及数据库、上传图片、配置和历史日志保留。
- 修复提交首次镜像产生的 GitHub Actions #77 在作业创建前报 `(Line: 308, Col: 14): Exceeded max expression length 21000`。原因是新增打包逻辑使含 `${{ ... }}` 的单个 PowerShell `run` 标量超过平台上限；现已把版本、标签、更新地址及 Gitee 仓库变量改为步骤 `env` 输入，避免 GitHub 把整段脚本作为超长表达式标量处理。后续 CI 结果须另行记录，不用本地 YAML 解析成功替代远程工作流验收。

### 历史记录

此前的 `v0.1.0-rc.3` 曾按旧流程发布并写入稳定 manifest。该记录仅用于历史追踪；本次由 `0.0.5` 通过严格的一次性迁移条件建立新的正式稳定基线，保留 RC Release 和源码历史。

首次闭环已于 2026-08-26 16:35 CST 完成：

- 源码提交：`f26d8c62081a8821c973972a28a9b9d2e1d8a091`
- 历史预发布标签：`v0.1.0-rc.3`
- [GitHub Actions #18](https://github.com/SilenceJR/bb_erp_echo/actions/runs/32942763389)：Go、Web、Tauri 前端、Windows 打包和 Gitee 发布全部成功，总耗时 1 小时 6 分 7 秒。
- [Gitee 预发布版本](https://gitee.com/SilenceJR/bb_erp_releases/releases/tag/v0.1.0-rc.3)：四个 Windows ZIP 均可匿名下载。
- [历史稳定 manifest](https://gitee.com/SilenceJR/bb_erp_releases/raw/main/update-manifest.json)：当时版本为 `0.1.0-rc.3`；四个附件的 URL、实际字节数和 SHA-256 经本地匿名下载复验一致。

首次联调中发现并修复了 Gitee API 对不存在 Release 返回 `200 null`、对不存在文件返回 `200 []` 的兼容问题。GitHub 向 Gitee 上传约 51.5 MB 发布附件耗时约 53 分钟，后续正式发布需为该阶段预留足够时间。

参考：[Gitee 仓库镜像说明](https://blog.gitee.com/2021/07/15/repo-mirror/)、[Gitee Release 附件下载路由](https://blog.gitee.com/2022/08/18/update/)、[GitHub Actions Artifact](https://github.com/actions/upload-artifact)。
