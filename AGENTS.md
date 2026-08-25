# 项目代理协作规则

## 子代理与模型路由

主代理负责需求拆解、API 契约、任务协调和最终验收。

根据任务类型选择代理：

- 常规 Go 后端实现：`go_backend`
- Go 架构、并发、事务、安全或疑难性能问题：`go_architect`
- 新页面、复杂交互、视觉设计或响应式布局：`web_frontend`
- 小型前端修改、接口接入或明确 Bug：`web_maintainer`
- 大范围代码搜索和调用链定位：`code_explorer`

不要为简单搜索或机械修改调用高成本代理。

跨前后端任务：

1. 主代理先确定 API 契约。
2. `code_explorer` 可以先定位相关代码。
3. 后端由 `go_backend` 实现。
4. 新页面由 `web_frontend` 实现；普通接入由 `web_maintainer` 实现。
5. 架构风险交给 `go_architect` 只读审查。
6. 写代理不得同时修改相同文件。
7. 主代理等待所有结果并完成最终验证。
