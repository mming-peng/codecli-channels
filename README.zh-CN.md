# qq-codex-go

把 QQ 变成远程 Codex / Claude Code 工作台。

[English](README.md) · [简体中文](README.zh-CN.md)

`qq-codex-go` 是一个把 **官方 QQ 机器人** 接到 **本机 Codex** 的 Go 桥接器，让一条 QQ 会话可以像一个持续存在的远程编码会话一样工作。

它不是普通问答机器人，更适合真实项目协作：项目切换、会话连续性、原生审批转发、受控可写执行，都是设计的一部分。

## 特性

- 官方 QQ 机器人接入，不依赖 OneBot
- QQ 会话到本地 session 和 Codex thread 的持久映射
- 多个本地项目工作区
- QQ 中处理 Codex 原生审批
- 可选 Claude Code 后端（`claude -p` headless 模式）
- `/ask` 只读分析
- `/run` 可写执行
- `/session` 会话管理
- `/project` 项目切换
- 较慢任务会先回“收到，正在处理…”
- 主传输层采用 `codex app-server` over stdio JSON-RPC

## 架构

```text
QQ 会话
  -> 本地项目
  -> bridge session
  -> Codex thread
  -> codex app-server
```

这样可以在多轮消息中保留上下文、隔离不同项目，并把审批准确地挂回到正确的 QQ 会话。

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
cp config/qqbot.example.json config/qqbot.json
```

### 2. 填写凭据和路径

最小示例：

```json
{
  "defaultAccountId": "default",
  "defaultTimezone": "Asia/Shanghai",
  "accounts": {
    "default": {
      "enabled": true,
      "appId": "YOUR_APP_ID",
      "clientSecret": "YOUR_APP_SECRET"
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "accountIds": ["default"],
    "allowAllTargets": false,
    "allowedTargets": [
      "default:user:YOUR_OPENID"
    ],
    "requireCommandPrefix": true,
    "readOnlyPrefixes": ["/ask", "/read", "问问"],
    "writePrefixes": ["/run", "/exec", "执行"],
    "confirmPrefixes": ["/confirm", "/确认"],
    "projects": {
      "qq-codex-go": {
        "path": "/path/to/qq-codex-go",
        "description": "当前仓库"
      }
    },
    "defaultProject": "qq-codex-go",
    "readOnlyCodexSandbox": "read-only",
    "writeCodexSandbox": "workspace-write",
    "defaultRunMode": "write",
    "implicitMessageMode": "write",
    "claudeBinary": "claude",
    "claudeModel": null,
    "dataDir": "/path/to/qq-codex-go/data",
    "stateFile": "/path/to/qq-codex-go/data/state.json",
    "auditFile": "/path/to/qq-codex-go/data/bridge-audit.jsonl"
  }
}
```

> 强烈建议显式设置 `dataDir`、`stateFile`、`auditFile`。
> `bridge.backend` 是默认后端；也可以在 QQ 里用 `/backend use codex|claude` 按会话切换。

### 3. 启动 bridge

```bash
go run ./cmd/qq-codex-go -config ./config/qqbot.json
```

或者先编译：

```bash
go build -o ./bin/qq-codex-go ./cmd/qq-codex-go
./bin/qq-codex-go -config ./config/qqbot.json
```

### 4. 首次验证

建议按这个顺序验收：

1. 观察日志出现 `QQ 网关 READY`
2. 在 QQ 中发送 `/ping`
3. 发送 `/project current`
4. 发送 `/ask 帮我解释一下这个项目是做什么的`

## 命令

### 通用命令

| 命令 | 说明 |
| --- | --- |
| `/ping` | 健康检查 |
| `/help` | 查看帮助 |
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
| `/session list` | 查看当前项目下的会话 |
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

长回复会根据 `qqMaxReplyChars` 自动切成多条 QQ 消息发送。

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
