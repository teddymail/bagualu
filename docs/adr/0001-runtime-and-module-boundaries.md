# ADR 0001：运行时与模块边界

- 状态：已接受
- 日期：2026-08-29

## 决策

1. 八卦炉主程序负责业务、HTTP API、调度、SQLite 和 Mihomo 子进程监督；Mihomo 不注册独立 OpenWrt 服务。
2. LuCI/UCI/ubus/procd 只管理八卦炉主程序。LuCI 不拼接 init shell 命令，通过 UCI 持久化启用状态，通过 ubus 提交生命周期操作。
3. SQLite 保存节点、来源、任务、原始测量和评分快照；基础设施失败作为可审计测量保存，但不进入评分。
4. Mihomo 私有 API、配置和进程控制保留在 `internal/infrastructure/mihomo`；OpenWrt 页面与打包文件保留在 `packaging/openwrt`。
5. 管理后台使用 React + Ant Design 明亮模式；系统参数只读展示，并跳转 LuCI 修改。

## 文件规模记录

- `cmd/bagualu/main.go` 保留为单一组合根，集中处理启动顺序、依赖装配和关闭顺序；不得继续加入业务规则。超过 900 行或出现第二种运行形态时拆为配置、运行时装配和调度三个文件。
- `web/admin/src/main.tsx` 保留为单入口管理台，页面共享同一 API 客户端、状态标签和单位格式化；超过 800 行或引入前端路由测试时按页面模块拆分。
- `internal/modules/subscription_output/render.go` 保留同一格式渲染边界，便于跨格式契约测试；新增输出格式必须新建渲染文件，不再扩大该文件。
- HTTP 集成测试已拆为 `server_test.go` 与 `server_more_test.go`，仓库不存在超过 1000 行的手写源码文件。

## 后果

- 手动和定时测试复用同一编排器与单线程队列。
- OpenWrt 生命周期可以在管理 HTTP 端口不可用时通过 procd 与状态文件诊断。
- 新协议和新订阅格式不得修改分组授权、评分或 OpenWrt 生命周期模块。
