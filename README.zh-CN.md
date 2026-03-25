# codecli-channels

把聊天 channel 变成远程 Codex / Claude Code 编码工作台。

[English](README.md) · [简体中文](README.zh-CN.md)

`codecli-channels` 是一个自托管的本地 bridge，用来把聊天渠道接到本机上的编码 CLI。它会把每个会话稳定映射到本地项目、bridge session 和后端线程，让你可以直接从 QQ、飞书或个人微信里继续做真实开发任务，而不是每次都重新建立上下文。

它不是普通问答机器人封装，更像一个“聊天端入口 + 本地执行上下文层”。项目切换、会话连续性、审批转发、可控写权限、任务回看和中断，都是一等能力。

## 为什么做这个项目

- 把 `channel 会话 -> 本地项目 -> bridge session -> Codex / Claude 线程` 这条映射链固定下来。
- 让普通聊天消息成为默认工作入口，而不是把体验建立在一堆命令前缀上。
- 把后端审批请求回挂到聊天端，让高风险动作仍然可见、可控。
- 用统一 bridge core 承接多个 channel driver，而不是把项目绑定死在单一平台上。
- 把个人微信扫码拿 token 这类初始化流程和运行期消息桥接解耦。

## 当前状态

| 领域 | 状态 | 说明 |
| --- | --- | --- |
| QQ | 可用 | 基于官方 bot API 和 gateway，不依赖 OneBot |
| 飞书 | 文本 MVP | 已有文本消息链路，支持群聊/线程隔离选项 |
| 个人微信 | 文本 MVP | 内建二维码与 token 绑定流程，文本收发链路已打通 |
| Codex 后端 | 主路径 | 基于 `codex app-server` 的 stdio JSON-RPC |
| Claude 后端 | 可选支持 | 基于 `claude -p` headless 模式 |

需要对外明确的边界：

- 飞书和微信目前都还是以文本能力为主。
- 微信媒体消息还没有进入媒体工作流，只会做保守降级。
- 后端发起的 `request_user_input` 和 MCP elicitation 这类交互式追问，bridge 还没有完整承接。

## 工作原理

```text
Channel Driver
  -> 统一的 channel.Message
  -> bridge service
  -> 本地项目 + 持久化 session
  -> Codex / Claude runner
  -> 回复和审批再回到聊天端
```

核心思路是让 bridge 负责路由、状态和会话连续性，channel driver 只处理各平台自己的协议细节。

## 快速开始

### 环境要求

- Go `1.25+`
- 如果使用 `bridge.backend=codex`，需要本机已安装可用的 `codex`
- 如果使用 Codex 后端，需要本机已完成 Codex 登录
- 如果使用 `bridge.backend=claude`，需要本机已安装可用的 `claude`
- 如果使用 Claude 后端，需要本机已完成 Claude Code 登录
- 至少准备一个要启用的 channel 凭据
- 本机能访问你想暴露给 bridge 的项目目录

### 1. 复制示例配置

```bash
cp config/codecli-channels.example.json config/codecli-channels.json
```

### 2. 填写凭据、作用域和项目路径

一个面向 QQ 的最小示例：

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
    "projects": {
      "codecli-channels": {
        "path": "/path/to/codecli-channels",
        "description": "channels bridge"
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

补充说明：

- 新配置优先使用 `channels`、`channelIds`、`allowedScopes`，旧字段 `accounts`、`accountIds`、`allowedTargets` 仍兼容。
- 建议显式设置 `dataDir`、`stateFile`、`auditFile`，避免运行期状态落到你意料之外的位置。
- `bridge.backend` 只决定默认后端，实际聊天中仍可用 `/backend use codex` 或 `/backend use claude` 按会话切换。
- 如果 `config/codecli-channels.json` 不存在，程序仍会回退读取 `config/qqbot.json`。

### 3. 启动 bridge

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

或者先编译：

```bash
go build -o ./bin/codecli-channels ./cmd/codecli-channels
./bin/codecli-channels -config ./config/codecli-channels.json
```

### 4. 验证第一条会话

建议按这个顺序：

1. 如果你先接的是 QQ，确认日志出现 `QQ 网关 READY`。
2. 发送 `/help`。
3. 直接发送普通消息，例如 `解释一下这个仓库是做什么的`。
4. 第一条任务完成后发送 `/history`。

### 可选：接入个人微信

仓库内已经内建个人微信初始化命令：

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

这个流程会在终端打印二维码，等待扫码确认，并把得到的 token 以及对应的 `bridge.channelIds`、`bridge.allowedScopes` 自动回写到配置里。

如果你已经有 token：

```bash
go run ./cmd/codecli-channels weixin bind -config ./config/codecli-channels.json -token '<你的token>'
```

## 日常使用

普通消息就是默认工作入口。bridge 会沿用当前项目、当前会话和当前后端继续执行，除非你主动切换。

### 通用命令

| 命令 | 说明 |
| --- | --- |
| `/help` | 查看环境控制命令 |
| `/stop` | 中断当前任务 |
| `/history` | 查看当前项目最近任务记录 |
| 普通消息 | 直接把任务交给当前后端 |

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
| `/session new [name]` | 开一个全新的会话 |
| `/session switch <id>` | 切换到已有会话 |

### 后端命令

| 命令 | 说明 |
| --- | --- |
| `/backend current` | 查看当前后端 |
| `/backend list` | 查看可用后端 |
| `/backend use <codex|claude>` | 切换后端 |

### 审批命令

| 命令 | 说明 |
| --- | --- |
| `/approve` | 同意当前待处理审批 |
| `/approve session` | 同意并记忆到本会话 |
| `/deny` | 拒绝当前待处理审批 |

## 审批模型

项目里有两层审批：

1. bridge 自己的高风险确认，例如 `rm -rf`、`git reset --hard`、`drop database`
2. Codex 执行过程中的原生审批请求

这样做的目的，是在不替代后端原生审批的前提下，对明显危险的任务保持更强的保护。

## 仓库结构

- `cmd/codecli-channels`：CLI 入口
- `internal/app`：顶层命令路由，以及 `weixin setup` / `weixin bind`
- `internal/bridge`：调度、命令处理、审批、历史记录、回包行为
- `internal/channel`：通道抽象，以及 QQ、飞书、微信 driver
- `internal/codex`：Codex runner，包括 `app-server` 传输层
- `internal/claude`：Claude Code headless runner
- `internal/config`：配置加载、归一化和兼容逻辑
- `internal/store`：项目、后端、会话、线程状态的持久化
- `docs/`：对外文档，以及带日期的设计/计划记录

## 当前限制

- 飞书和微信仍然是各自独立演进的 MVP，能力并未完全对齐 QQ。
- 个人微信接入当前面向本仓库使用的 iLink 风格接口，并不是一个通用公开微信机器人方案。
- 微信回包依赖此前入站消息携带并持久化下来的 `context_token`。
- 后端如果要求结构化追问，当前 bridge 还不能完整承接。
- 这是一个自托管 bridge，默认假设运行机器同时能访问本地仓库和所需 CLI。

## 文档

建议从这里开始：

- [`docs/project-overview.zh-CN.md`](docs/project-overview.zh-CN.md)
- [`docs/architecture.zh-CN.md`](docs/architecture.zh-CN.md)
- [`docs/configuration.zh-CN.md`](docs/configuration.zh-CN.md)
- [`docs/troubleshooting.zh-CN.md`](docs/troubleshooting.zh-CN.md)
- [`docs/README.zh-CN.md`](docs/README.zh-CN.md)

英文文档：

- [`docs/project-overview.md`](docs/project-overview.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/configuration.md`](docs/configuration.md)
- [`docs/troubleshooting.md`](docs/troubleshooting.md)
- [`docs/README.md`](docs/README.md)

## 开发

运行测试：

```bash
go test ./...
```

贡献说明见 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

## Roadmap

- 更好地处理后端交互式追问
- 补齐飞书和微信的富消息能力
- 提供更清晰的启动诊断和配置校验
- 补充 Docker / launchd / systemd 部署示例
- 继续完善 GitHub 开源仓库配套

## License

本仓库采用 [MIT License](LICENSE)。
