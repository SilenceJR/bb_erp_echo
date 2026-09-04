# Changelog

## 0.0.13

### Fixed

- 模具支持 ZIP、图片和 DWG 拖拽添加；客户 Excel 导入支持拖放；Tauri Windows 原生文件拖放已接入并支持 2 GiB 资料包流式上传；修正筛选操作栏重叠、空态、详情加载失败和 Drawer 编辑布局。
- 修复 Windows Tauri 客户端对 RFC1918 IPv4 ERP 服务的 HTTP 授权范围，自动发现或手动验证成功后可正常发起登录与同源业务请求。
- Windows 客户端启动优先连接上次验证的服务器，仅在该服务器不可用或身份验证失败时才进行局域网发现。
