# Windows 内网 full-only 发布说明

> 适用范围：当前全新部署的 Windows 10/11 客户端与 Go 服务端。
> 不适用：旧客户端、旧 ZIP 更新协议、差分包、预发布通道、公网客户端直连。

## 1. 发布边界

- 发布标签只接受 `vMAJOR.MINOR.PATCH`。
- GitHub Actions 在 Windows runner 构建 Go 服务端、Vue/Tauri 客户端、NSIS 安装器和便携 EXE。
- Gitee 发布仓库可以作为服务端读取更新清单和资源的上游存储；Windows 客户端不访问 Gitee，也不接收上游 URL。
- Go 服务端下载并校验资源后，通过当前 ERP 的 `/api/v1/updates/client/artifacts/:sha256` 同源代理给客户端。
- 客户端更新只有 `full` 策略：NSIS 安装器或 portable 完整 EXE。不存在差分生成、差分缓存或旧客户端协议回退。

## 2. 必需配置

GitHub 环境 `gitee-release`：

- `GITEE_TOKEN`
- `GITEE_SOURCE_TOKEN`（可选；未配置时使用 `GITEE_TOKEN`）
- `TAURI_SIGNING_PRIVATE_KEY`
- `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`
- `TAURI_UPDATER_PUBLIC_KEY`
- `GITEE_SOURCE_OWNER` / `GITEE_SOURCE_REPO`
- `GITEE_RELEASE_OWNER` / `GITEE_RELEASE_REPO`

服务端部署：

- `BB_ERP_UPDATE_ENABLED=true`
- `BB_ERP_UPDATE_MANIFEST_URL=<update-manifest.json 地址>`
- `BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key`

发布私钥只存在于 GitHub 环境。部署包只携带规范化公钥。

## 3. 当前签名清单

`update-manifest.json` 的 `client_update_v2` 是当前协议名称，不表示兼容旧协议：

```json
{
  "client_update_v2": {
    "payload": "BASE64_JSON",
    "signature": "BASE64_MINISIGN_SIGNATURE"
  }
}
```

解码后的 payload 只能包含：

```json
{
  "protocol_version": 2,
  "version": "1.2.3",
  "target": "windows-x86_64",
  "layout_version": 1,
  "full": {
    "nsis": {
      "kind": "nsis",
      "url": "上游版本化资源地址",
      "size": 123,
      "sha256": "64位十六进制",
      "signature": "BASE64_MINISIGN_SIGNATURE"
    },
    "portable": {
      "kind": "portable",
      "url": "上游版本化资源地址",
      "size": 456,
      "sha256": "64位十六进制",
      "signature": "BASE64_MINISIGN_SIGNATURE"
    }
  }
}
```

服务端与客户端严格拒绝未知字段；payload 中出现 `deltas` 即拒绝。

### `v0.0.10` 至 `v0.0.12` GitHub 仅构建例外

`v0.0.10`、`v0.0.11` 与 `v0.0.12` 只用于确认 GitHub Actions 的完整 Windows 打包和签名链路。标签仍执行 Go/Web/Tauri/Rust 验证，生成服务端 ZIP、all-in-one ZIP、升级器、NSIS、portable EXE、签名清单，并将产物作为 GitHub Actions Artifact 保留 14 天。

工作流对这些标签明确跳过 `publish-gitee`：不创建或修改 Gitee Release、不上传任何成品、不更新 Gitee `update-manifest.json`。因此构建不能作为稳定更新源，也不表示 Windows 真机发布验收已完成。其他正式标签仍按下述 Gitee 发布流程执行，除非另行确认。

验收记录：GitHub Actions `#91`（run `33458004272`）已完成并成功，Windows 服务端、all-in-one、升级器、签名清单/客户端以及 portable 测试包五组 Artifact 均已生成；`Publish verified Gitee release` 结论为 `skipped`。Gitee `bb_erp_releases` 未创建 `v0.0.11` Release，稳定 manifest 保持 `0.0.9`。`v0.0.12` 的 GitHub 构建结果需在标签推送后单独记录。

## 4. 发布流程

### Windows 本机离线打包

已有 Windows 工具链可在仓库根目录运行：

```powershell
# 默认只生成 all-in-one
.\scripts\windows-package.ps1 -Version 0.0.13

# 同时生成 all-in-one 与服务器目录离线更新整包
.\scripts\windows-package.ps1 -Version 0.0.13 -Target All
```

脚本支持将 `TAURI_SIGNING_PRIVATE_KEY` 和 `TAURI_UPDATER_PUBLIC_KEY` 配置为文件路径，
自动规范化公钥并临时注入客户端构建。离线 manifest 仅包含同一目录内的相对文件名，
必须先完整解压到服务器 `updates/releases/incoming/v<版本>`，再从已安装的 `server` 目录运行 `激活离线更新.ps1 -IncomingDir updates/releases/incoming/v<版本>`。该受保护脚本只使用安装目录中的可信公钥与验证器，不执行 incoming 内程序，并在校验和、Minisign 验签完成后原子切换目录；随后 `BB_ERP_UPDATE_SOURCE=directory` 才会从 `updates/releases/active` 使用。禁止直接覆盖 `active`。
目录模式不替代签名、大小、SHA-256 和 ZIP 结构校验，也不会自动上传或修改 Gitee。

1. 确认分支测试通过并创建正式标签：

   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

2. CI 生成并签名服务端 ZIP、NSIS、portable EXE 和签名 payload；每次签名立即使用将嵌入客户端的规范化 `TAURI_UPDATER_PUBLIC_KEY` 实际验签，私钥与公钥不匹配时立即失败。
3. 发布作业确认源码标签提交与镜像提交一致。
4. 上传 manifest 声明的版本化资源。
5. 对每个匿名下载重新校验字节数和 SHA-256。
6. 再次确认新版本严格大于当前稳定版本。
7. 发布前再次用同一公钥验证本地资源与 payload 签名，最后才更新稳定 `update-manifest.json`。任一资源、签名或密钥对检查失败都不得更新稳定清单。

## 5. 客户端安装安全

- Rust 只接受 loopback/RFC1918 IPv4 的当前 ERP HTTP origin，不使用系统代理且不跟随重定向。
- 更新计划、payload 和文件分别验证 Minisign、大小及 SHA-256。
- portable 资源先写入缓存临时文件，再复制到目标 EXE 同目录暂存；旧 EXE 先备份，新版本未在 90 秒内确认启动时恢复旧 EXE 并尝试重启。
- Program Files 等不可写目录使用签名 NSIS 安装器，不直接替换 EXE。
- 更新安装前必须通过统一未保存内容守卫。

## 6. 发布验收

自动门槛：

- `go test ./...`、`go vet ./...`
- Web/Client 生产构建
- `cargo fmt --check`、`cargo check --locked`、`cargo test --locked`
- 发布脚本语法检查和稳定版本递增检查
- 签名资源匿名大小/SHA-256 复验后才更新稳定清单

Windows 10/11 真机门槛：

- 唯一内网服务发现、连接和更新检查
- portable 完整替换、重启确认及启动失败恢复
- NSIS 完整安装、取消/失败不覆盖当前可运行客户端
- 断网、损坏资源、错误签名、磁盘不足和安装目录不可写
- 1920×1080 的 100%/125%/150% 缩放下更新确认与进度可达

本地静态检查不能替代 Windows 真机验收。
