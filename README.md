# 博邦 ERP 管理系统

博邦 ERP 当前采用模块化单体架构：Go 后端使用 Echo v5 + GORM + SQLite WAL + Casbin，Web 管理端使用 Vue/Vite，桌面端使用 Tauri 壳复用 Web 代码。

## 当前状态

- 后端主线已经可运行，入口是 `cmd/server/main.go`。
- 后端已完成登录认证、JWT、Casbin 权限、组织/部门/终端、用户/角色/权限、操作审计、文件日志、业务模块骨架。
- Web 管理端位于 `web/`，桌面端位于 `client/`。
- 当前业务模块大多还是骨架接口，后续应按模块逐步补 CRUD 和业务流程。

详细交接文档见 [docs/DEVELOPMENT_STATUS.md](docs/DEVELOPMENT_STATUS.md)。

## 快速启动

### 后端

```bash
go run ./cmd/server
```

默认监听所有网卡的 `8080` 端口。本机可访问：

```text
http://127.0.0.1:8080
```

同一内网的其他电脑或 Tauri 客户端应使用运行 Go 服务电脑的局域网 IP，例如：

```text
http://192.168.1.20:8080
```

如操作系统开启了防火墙，需要允许 `8080/TCP` 入站访问。可通过 `BB_ERP_HTTP_HOST`、`BB_ERP_HTTP_PORT` 覆盖监听地址和端口。

默认管理员：

```text
账号：admin
密码：admin123456
```

生产或正式测试环境必须用环境变量覆盖默认密码和 JWT 密钥。

### Web 管理端

```bash
cd web
npm install
npm run dev
```

### 桌面端

```bash
cd client
npm install
npm run desktop:dev
```

桌面端 API 同步规则见 [client/API_SYNC.md](client/API_SYNC.md)。

桌面端登录页可测试并保存 Go 服务地址；登录后也可从顶栏“服务器”入口切换。后续部署公网服务时，将地址改为正式 HTTPS 域名即可。

## 常用检查

```bash
go test ./...
```

## 本地运行产物

以下内容是本机运行或构建产物，不应作为业务代码提交：

- `data/*.db`
- `logs/*.log`
- `web/node_modules/`
- `client/node_modules/`
- `client/src-tauri/target/`
- `web/dist/`
