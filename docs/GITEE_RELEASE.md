# Windows 内网离线发布

> 文件名为历史保留链接。当前版本不再向 Gitee、GitHub Release 或其他公网更新源发布，也不支持增量包、安装器更新和服务端自动升级。

> 正式签名前，必须在 GitHub 中为 `release-signing` Environment 配置 required reviewers 和受保护的部署标签/分支。签名私钥及密码只能保存为该 Environment 的 secrets，不得保存为普通仓库 secrets。未完成该保护配置前，不得执行正式签名任务。

## 产物

正式版本由 GitHub Actions 的 Windows 构建生成三个仅供人工下载的 Artifact：

- `bb-erp-all-in-one-windows-v<版本>.zip`：首次部署。
- `bb-erp-client-update-windows-v<版本>.zip`：投放到服务器根目录的 `client` 目录。
- `bb-erp-server-windows-v<版本>.zip`：管理员人工替换服务端程序。

Artifact 文件名与服务端版本使用发布标签；客户端 EXE 和 `client-update.json` 内的更新版本独立取自 `client/src-tauri/tauri.conf.json`。这使服务端发布版本与客户端升级版本可以独立递增；CI 必须检查签名更新清单与客户端源码版本一致。

客户端更新包固定包含 `bb_erp_client.exe`、`client-update.json` 和操作说明。清单是严格 JSON 信封 `{payload, signature}`；其 Base64 payload 只包含 `version`、`target: windows-x86_64` 和单个 `artifact`，artifact 只包含 `kind: portable`、`size`、`sha256`、`signature`。CI 使用受控私钥分别签署 EXE 与清单载荷，并立即用客户端内置公钥验签。私钥不得复制到服务器。

## 客户端投放

部署结构固定为：

```text
部署根目录/
├── server/
│   ├── bb-erp-server.exe
│   ├── update-public.key
│   └── updates/client-cache/
└── client/
    ├── bb_erp_client.exe
    └── client-update.json
```

1. 从同一次 CI 构建下载客户端更新包并完整解压到临时目录。
2. 先把 `bb_erp_client.exe` 覆盖到服务器同级 `client` 目录。
3. 最后覆盖 `client-update.json`。服务端只有在载荷签名、EXE签名、大小和 SHA-256 全部通过后，才把文件复制进内容寻址缓存并对客户端发布。
4. 用旧版 Windows 客户端在登录页手动检查；确认目标版本和大小正确后再通知用户。已连接的客户端启动后只静默检查一次，不自动下载。

客户端只接受更高的 SemVer、固定 `windows-x86_64` 和完整 portable EXE。客户端从当前已验证的内网 ERP 服务器下载，下载后再次验签并校验大小与 SHA-256；随后由临时助手原子替换并启动新版，启动失败自动恢复旧版。

## 服务端人工升级

服务端不检查、不下载也不自动替换自身程序：

1. 通知用户，在 Windows 通知区右键博邦托盘图标，选择“退出博邦服务”并确认停止。
2. 备份整个 `server` 目录，至少确认 `data`、`static/uploads`、配置和日志可恢复。
3. 从服务端 Artifact 复制程序与 `web/dist`；不得覆盖或删除现有业务数据目录。
4. 启动服务，依次验证 `/health`、`/ready`、登录和 `/api/v1/version`。
5. 失败时停止新程序并恢复备份。

Windows 正式服务端使用无控制台的托盘入口，启动成功后显示当前全部有效局域网 IPv4 地址。托盘重启会先优雅释放 UDP、HTTP、SQLite 和日志资源，再重新初始化；不得用任务管理器强制结束代替正常退出。

## 发布验收边界

CI 成功只证明源码测试、Windows 构建、签名和压缩包结构通过。正式发布前仍需在目标 Windows Server 与 Windows 10/11 客户端验证：首次部署、登录前检查、Range 下载、目录不可写、断网、坏签名、替换重启和故障回滚。
