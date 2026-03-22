# codecli-channels

把聊天 channel 变成远程 Codex / Claude Code 工作台。

[English](README.md) · [简体中文](README.zh-CN.md)

`codecli-channels` 是一个面向 Code CLI 的本地 channels bridge。当前它已经完成多平台通道内核重构，并把 **QQ channel** 接到 **本机 Codex**；后续的 **Feishu** 与 **个人微信** 会以独立 driver 的形式接入，而不是继续修改 bridge 核心。

它不是普通问答机器人，更适合真实项目协作：项目切换、会话连续性、原生审批转发、受控可写执行、状态可见性和任务中断能力，都是设计的一部分。QQ 是当前第一个 channel，而不是产品的长期边界。

## 特性

- 通过官方 API 接入 QQ channel，不依赖 OneBot
- channel 会话到本地 session 和 Codex thread 的持久映射
- 多个本地项目工作区
- 在聊天端处理 Codex 原生审批
- 可选 Claude Code 后端（`claude -p` headless 模式）
- `/status` 查看当前项目/会话/模式/审批状态
- `/ask` 只读分析
- `/run` 可写执行
- `/stop` 中断当前任务
- `/history` 回看最近任务
- `/session` 会话管理
- `/project` 项目切换
- 较慢任务会先回“收到，正在处理…”
- 主传输层采用 `codex app-server` over stdio JSON-RPC

## 架构

```text
Channel Driver
  -> 本地项目
  -> bridge session
  -> Codex thread
  -> codex app-server
```

这样可以在多轮消息中保留上下文、隔离不同项目，并把审批准确地挂回到正确的 channel 会话。

## 快速开始

### 环境要求

- Go `1.22+`
- 已安装并可用的 `codex` CLI（当 `bridge.backend=codex`）
- `codex` 已完成登录（当 `bridge.backend=codex`）
- 已安装并可用的 `claude`（Claude Code CLI，当 `bridge.backend=claude`）
- Claude Code 已完成登录（当 `bridge.backend=claude`）
- QQ 机器人 `appId` / `clientSecret`
- 能访问你要暴露的本地项目目录

### 1. 创建配置文件

```bash
cp config/codecli-channels.example.json config/codecli-channels.json
```

### 2. 填写凭据和路径

最小示例：

```json
{
  "defaultAccountId": "default",
  "defaultTimezone": "Asia/Shanghai",
  "channels": {
    "default": {
      "type": "qq",
      "enabled": true,
      "options": {
        "appId": "YOUR_APP_ID",
        "clientSecret": "YOUR_APP_SECRET"
      }
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "channelIds": ["default"],
    "allowAllTargets": false,
    "allowedScopes": [
      "default:user:YOUR_OPENID"
    ],
    "requireCommandPrefix": true,
    "readOnlyPrefixes": ["/ask", "/read", "问问"],
    "writePrefixes": ["/run", "/exec", "执行"],
    "confirmPrefixes": ["/confirm", "/确认"],
    "projects": {
      "codecli-channels": {
        "path": "/path/to/codecli-channels",
        "description": "channels bridge（当前启用 QQ channel）"
      }
    },
    "defaultProject": "codecli-channels",
    "maxReplyChars": 1500,
    "readOnlyCodexSandbox": "read-only",
    "writeCodexSandbox": "workspace-write",
    "defaultRunMode": "write",
    "implicitMessageMode": "write",
    "claudeBinary": "claude",
    "claudeModel": null,
    "dataDir": "/path/to/codecli-channels/data",
    "stateFile": "/path/to/codecli-channels/data/state.json",
    "auditFile": "/path/to/codecli-channels/data/bridge-audit.jsonl"
  }
}
```

> 强烈建议显式设置 `dataDir`、`stateFile`、`auditFile`。
> 旧字段 `qqMaxReplyChars` 仍然兼容，但推荐改用 `maxReplyChars`。
> 旧字段 `accounts`、`accountIds`、`allowedTargets` 仍然兼容，但推荐迁移到 `channels`、`channelIds`、`allowedScopes`。
> `bridge.backend` 是默认后端；也可以在 QQ 里用 `/backend use codex|claude` 按会话切换。

### 3. 启动 bridge

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

或者先编译：

```bash
go build -o ./bin/codecli-channels ./cmd/codecli-channels
./bin/codecli-channels -config ./config/codecli-channels.json
```

### 4. 首次验证

建议按这个顺序验收：

1. 观察日志出现 `QQ 网关 READY`
2. 在 QQ 中发送 `/ping`
3. 发送 `/status`
4. 发送 `/ask 帮我解释一下这个项目是做什么的`

如果你本地已经有旧的 `config/qqbot.json`，新二进制在找不到 `config/codecli-channels.json` 时会自动回退到旧路径。

## 命令

### 通用命令

| 命令 | 说明 |
| --- | --- |
| `/ping` | 健康检查 |
| `/help` | 查看帮助 |
| `/status` | 查看当前项目、会话、模式、审批和运行状态 |
| `/stop` | 停止当前正在执行的任务 |
| `/history` | 查看当前项目最近任务 |
| `/backend current` | 查看当前后端 |
| `/backend use <codex\|claude>` | 切换后端 |
| `/clear` | 开一个新会话 |
| `/mode` | 查看当前默认模式 |
| `/mode write` | 把当前会话默认模式切到可写执行 |
| `/mode read` | 把当前会话默认模式切到只读分析 |

### 项目命令

| 命令 | 说明 |
| --- | --- |
| `/project list` | 查看项目列表 |
| `/project current` | 查看当前项目 |
| `/project use <alias>` | 切换项目 |

### 会话命令

| 命令 | 说明 |
| --- | --- |
| `/session current` | 查看当前会话 |
| `/session list` | 查看当前项目下的会话，并带最近任务摘要 |
| `/session new [名称]` | 新建会话 |
| `/session switch <id>` | 切换会话 |

### 执行命令

| 命令 | 说明 |
| --- | --- |
| `/ask <问题>` | 只读分析 |
| `/run <任务>` | 可写执行 |
| 普通消息 | 按当前会话模式执行；首次自动建会话时初始值来自 `implicitMessageMode` |

## 审批模型

这里有两层审批。

### bridge 层确认

对于明显危险的任务，bridge 可能要求先发：

- `/confirm`

常见高风险关键词包括：

- `rm -rf`
- `git reset --hard`
- `drop database`
- `truncate table`
- `sudo`

这不是 Codex 原生审批，而是 bridge 自己的保护。

### Codex 原生审批

当 Codex 在执行中触发审批时，bridge 会把请求转发到 QQ。

支持的回复：

- `/approve`
- `/approve session`
- `/deny`
- `同意`
- `拒绝`
- `本会话允许`
- `yes`
- `no`

## 响应行为

### 慢任务提示

如果大约 2 秒内还没有最终答案，bridge 可能先回：

```text
收到，正在处理…
```

这主要改善的是体感速度。

### 首个最终答案优先回包

一旦拿到第一个 `final_answer`，bridge 会优先把它发回 QQ，而不是等待所有尾部生命周期事件结束。

### 长回复自动分段

长回复会根据 `maxReplyChars` 自动分段发送；旧字段 `qqMaxReplyChars` 仍然兼容。

### 状态与回看

中途想重新对齐上下文时，优先用这 3 个命令：

- `/status`：看当前项目、会话、模式、审批和是否还在执行
- `/stop`：想改方向时主动停止当前任务
- `/history`：回看当前项目最近做过什么

## 文档

更多文档在 [`docs/`](docs/README.md)：

- [`docs/README.md`](docs/README.md)
- [`docs/README.zh-CN.md`](docs/README.zh-CN.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/architecture.zh-CN.md`](docs/architecture.zh-CN.md)
- [`docs/configuration.md`](docs/configuration.md)
- [`docs/configuration.zh-CN.md`](docs/configuration.zh-CN.md)
- [`docs/troubleshooting.md`](docs/troubleshooting.md)
- [`docs/troubleshooting.zh-CN.md`](docs/troubleshooting.zh-CN.md)

## 开发

运行测试：

```bash
go test ./...
```

贡献方式见：[`CONTRIBUTING.md`](CONTRIBUTING.md)

## Roadmap

- 为短 `/ask` 请求提供更快的模型路径
- 改进流式回包策略
- 提供更清晰的配置校验和启动诊断
- 补充 Docker / launchd / systemd 部署示例
- 继续完善 GitHub 开源仓库配套

## License

本仓库采用 [MIT License](LICENSE)。
