# qq-codex-go

一个专门给“官方 QQ 机器人 → Codex CLI”使用的 Go 版桥接项目。

## 目标

- 用你现在的 **官方 QQ 机器人** 接法，不走 OneBot
- 支持 **多个 QQ 机器人账号**
- 支持 **多个项目目录切换**
- 支持 **本地持久会话**，把 QQ 会话绑定到 Codex `thread_id`
- 比旧版“每条消息都重新 `codex exec` 一次”的桥接更省上下文 token

## 当前实现

- QQ 官方机器人网关接入
- 多账号并行监听
- 普通消息默认直接进入 Codex 对话
- `/ask` 显式只读分析
- `/run` 显式工作区可写执行
- `/project list|current|use <alias>` 项目切换
- `/session list|current|new|switch` 本地会话管理
- `/mode write|read` 切换当前会话默认执行模式
- `/confirm` 高风险任务确认
- 默认只把最终回复和原生审批提示同步到 QQ，不转发 thinking/工具进度
- `/approve` `/deny` 回复 Codex 原生审批
- `state.json` 持久化本地会话与项目映射
- `bridge-audit.jsonl` 审计日志

## 和旧版 Node 桥接的差异

旧版 `qqbot-mcp-server`：

- 重点是“把 QQ 消息转成一次性 `codex exec` 任务”
- 每条消息都是新子进程 + 新上下文
- 权限升级靠失败后重跑

这个 Go 版：

- 本地维护“QQ 会话 → 项目 → 本地会话 → Codex thread_id”映射
- 首轮新开 `codex exec`
- 后续同会话继续走 `codex exec resume <thread_id>`
- 因而更接近 `cc-connect` 的会话型做法

## 审批机制

当前版本已经切到 **Codex 原生审批 + PTY 终端桥接**：

1. QQ 发来的需求会进入 Codex 原生执行会话
2. 如果 Codex 请求审批，桥接会把原生审批提示转发到 QQ
3. 你回复“同意/拒绝”或 `/approve` `/deny`
4. 桥接把这个决定回写给同一条 Codex 原生终端会话

也就是说：

- 不再使用 `-a never` 关闭 Codex 审批
- “同意/拒绝”或 `/approve` `/deny` 不再是“失败后补跑”的替代审批
- 而是直接回复 **Codex 原生审批**

## 命令

- 普通一句话/一个路径/一段需求，都可以直接发
- `/ping`
- `/help`
- `/project list`
- `/project current`
- `/project use <alias>`
- `/clear`
- `/session list`
- `/session current`
- `/session new [name]`
- `/session switch <id>`
- `/mode`
- `/mode write|read`
- `/ask 你的问题`
- `/run 你的需求`
- `/confirm`
- `/approve`
- `/deny`

## 配置

- 示例配置：`config/qqbot.example.json`
- 本地实际配置：`config/qqbot.json`
- 默认状态文件：`data/state.json`
- 默认审计日志：`data/bridge-audit.jsonl`

## 运行

```bash
cd /Users/ming/ai/feishu-connect/qq-codex-go
go mod tidy
go run ./cmd/qq-codex-go -config ./config/qqbot.json
```

## 典型使用流

1. QQ 发 `/project list`
2. QQ 发 `/project use feishu-connect`
3. 如果想完全开新会话，QQ 发 `/clear`
4. 直接发：`帮我看看这个仓库是做什么的`
5. 直接发：`修复 README 里明显的错别字`
5. 如果桥接识别到高风险任务，回复 `/confirm`
6. 如果 Codex 发来原生审批提示，回复 `/approve` 或 `/deny`

## 后续最值得继续做的增强

- 可选的调试模式进度流（默认保持静默）
- 读取 `~/.codex/sessions`，支持直接挂接历史 Codex 会话
- 更细的权限策略与命令白名单
- 群聊里按发送者分更多会话策略
