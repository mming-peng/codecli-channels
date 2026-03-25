# Codex app-server 清理与旧 Runner 删除方案

## 背景

当前 `codecli-channels` 已默认通过 `codex app-server` 执行 Codex 任务，但仓库里仍保留旧的 PTY/CLI runner 实现。现状存在两个问题：

1. 服务退出时，没有统一关闭已启动的 `codex app-server` 子进程。
2. 长时间运行过程中，按会话缓存的 app-server 进程缺少回收机制，旧会话可能持续占用进程。

## 本次目标

1. 保留并强化 `codex app-server` 作为唯一 Codex 执行路径。
2. 删除旧 PTY/CLI runner 及其测试、依赖和遗留解析逻辑。
3. 为 `AppServerRunner` 增加生命周期管理能力：
   - 支持统一关闭全部已缓存会话。
   - 支持按空闲超时回收不用的会话，避免进程持续累积。
4. 在服务退出时主动停止 channel driver、取消运行中的任务，并关闭底层 runner。

## 实施步骤

### 1. 提炼 Codex 公共定义

把以下与具体 runner 无关的公共内容从旧 `runner.go` 中拆出，供 `app_server.go` 和其他调用方继续复用：

- `TurnRunner`
- `TurnOptions`
- `TurnResult`
- 审批相关类型
- `buildPrompt`
- Codex 运行时环境准备逻辑

### 2. 增强 AppServerRunner 生命周期

为 `AppServerRunner` 增加：

- `Close()`：关闭并清空所有缓存的 `appServerSession`
- 空闲会话回收：在新任务进入时顺带回收超过阈值且当前未运行的 session

为 `appServerSession` 增加：

- 运行中状态标记
- 最近使用时间
- `Close()` 能力，确保关闭时中断当前等待并终止底层子进程

### 3. 服务退出链路接入清理

新增 `Service.Close(ctx)`，负责：

- 标记并取消当前运行任务
- 停止所有 channel driver
- 若 runner 支持关闭，则调用其 `Close()`

并在 `internal/app/run.go` 中，在收到退出信号后调用该关闭链路。

### 4. 删除旧 PTY/CLI runner

删除：

- `internal/codex/runner.go`
- 仅服务于旧 runner 的测试与辅助逻辑
- 不再需要的 `github.com/creack/pty` 依赖

### 5. 验证

补充并更新测试，重点覆盖：

- `AppServerRunner.Close()` 会清理缓存 session
- 空闲 session 回收行为
- `Service.Close()` 会取消任务、停止 driver、关闭 runner

最后执行 `go test ./...` 做完整验证。
