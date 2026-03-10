# 架构说明

## 总览

`qq-codex-go` 会把一条进入的 QQ 会话映射到一个持续存在的本地执行上下文：

```text
QQ 会话
  -> 项目
  -> bridge session
  -> Codex thread
```

这样做的目的是保留上下文、隔离不同项目，并把审批准确地挂回到正确的 QQ 会话。

## 主要组件

- `internal/qq/`：QQ API 客户端与网关处理
- `internal/bridge/`：消息编排、命令解析、审批和回复逻辑
- `internal/codex/`：Codex 接入传输层
- `internal/store/`：本地状态与会话持久化

## Codex 传输层

当前主传输方式是：

```text
codex app-server --listen stdio://
```

bridge 通过 stdio JSON-RPC 与 Codex 通信，主要使用这类结构化方法：

- `thread/start`
- `thread/resume`
- `turn/start`
- `turn/interrupt`
- 各类审批请求回调

相比依赖终端输出抓取或非正式事件流，这种方式更稳定，也更适合长期维护。

## 审批流

系统里有两层审批：

1. bridge 自己的高风险确认
2. Codex 执行过程中的原生审批

QQ 中的 `/approve`、`/approve session`、`/deny` 会映射回 Codex 的审批决策。

## 回复流

对于较慢的任务，bridge 可以先回一条“收到，正在处理…”。

一旦拿到第一个最终答案，bridge 会优先把它发回 QQ，而不是死等整轮所有收尾事件结束。
