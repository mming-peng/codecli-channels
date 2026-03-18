# qq-codex-go

Remote Codex / Claude Code for QQ.

[English](README.md) · [简体中文](README.zh-CN.md)

`qq-codex-go` connects **official QQ bots** to **Codex running on your local machine**, so a QQ conversation can behave like a persistent remote coding session.

It is built for real project work instead of simple chatbot replies: project switching, session continuity, native approval forwarding, and controlled writable execution are all first-class parts of the design.

## Features

- Official QQ bot integration, no OneBot dependency
- Persistent mapping from QQ conversation to local session and Codex thread
- Multiple local project workspaces
- Native Codex approval flow in QQ
- Optional Claude Code backend (`claude -p` headless mode)
- Read-only analysis with `/ask`
- Writable execution with `/run`
- Session management with `/session`
- Project switching with `/project`
- Early “working on it” reply for slower turns
- `codex app-server` over stdio JSON-RPC as the primary transport

## Architecture

```text
QQ conversation
  -> local project
  -> bridge session
  -> Codex thread
  -> codex app-server
```

This lets the bridge keep context across messages, isolate projects cleanly, and route approvals back to the correct QQ conversation.

## Quick Start

### Requirements

- Go `1.22+`
- A working `codex` CLI installation (when `bridge.backend=codex`)
- Completed Codex login on the host machine (when `bridge.backend=codex`)
- A working `claude` (Claude Code) installation (when `bridge.backend=claude`)
- Completed Claude Code login on the host machine (when `bridge.backend=claude`)
- QQ bot `appId` and `clientSecret`
- Access to the local project directories you want to expose

### 1. Create a config file

```bash
cp config/qqbot.example.json config/qqbot.json
```

### 2. Fill in credentials and paths

Minimal example:

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
        "description": "this repository"
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

> It is strongly recommended to set `dataDir`, `stateFile`, and `auditFile` explicitly.
> `bridge.backend` is the default backend; you can switch per QQ conversation via `/backend use codex|claude`.

### 3. Run the bridge

```bash
go run ./cmd/qq-codex-go -config ./config/qqbot.json
```

Or build a binary first:

```bash
go build -o ./bin/qq-codex-go ./cmd/qq-codex-go
./bin/qq-codex-go -config ./config/qqbot.json
```

### 4. Verify the connection

Recommended validation order:

1. Confirm the log shows `QQ 网关 READY`
2. Send `/ping`
3. Send `/project current`
4. Send `/ask Explain what this project does`

## Commands

### General

| Command | Description |
| --- | --- |
| `/ping` | Health check |
| `/help` | Show help |
| `/backend current` | Show current backend |
| `/backend use <codex\|claude>` | Switch backend |
| `/clear` | Start a fresh session |
| `/mode` | Show current default mode |
| `/mode write` | Set the current session default to writable execution |
| `/mode read` | Set the current session default to read-only analysis |

### Project

| Command | Description |
| --- | --- |
| `/project list` | List configured projects |
| `/project current` | Show the active project |
| `/project use <alias>` | Switch project |

### Session

| Command | Description |
| --- | --- |
| `/session current` | Show current session |
| `/session list` | List sessions for current project |
| `/session new [name]` | Create a new session |
| `/session switch <id>` | Switch session |

### Execution

| Command | Description |
| --- | --- |
| `/ask <question>` | Read-only analysis |
| `/run <task>` | Writable execution |
| plain message | Uses the current session mode; the first auto-created session is seeded from `implicitMessageMode` |

## Approval Model

There are two approval layers.

### Bridge-level confirmation

The bridge may require `/confirm` for obviously dangerous tasks such as:

- `rm -rf`
- `git reset --hard`
- `drop database`
- `truncate table`
- `sudo`

This is the bridge's own safety layer, not Codex native approval.

### Native Codex approval

When Codex requests approval during execution, the bridge forwards it to QQ.

Supported responses:

- `/approve`
- `/approve session`
- `/deny`
- `同意`
- `拒绝`
- `本会话允许`
- `yes`
- `no`

## Behavior

### Early working reply

If no final answer is available within about 2 seconds, the bridge may send:

```text
收到，正在处理…
```

This improves perceived responsiveness in QQ.

### Final answer first

When the first `final_answer` becomes available, the bridge prefers sending it immediately instead of waiting for every trailing lifecycle event to finish.

### Reply splitting

Long replies are automatically split into multiple QQ messages according to `qqMaxReplyChars`.

## Documentation

Additional docs live under [`docs/`](docs/README.md):

- [`docs/README.md`](docs/README.md)
- [`docs/README.zh-CN.md`](docs/README.zh-CN.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/architecture.zh-CN.md`](docs/architecture.zh-CN.md)
- [`docs/configuration.md`](docs/configuration.md)
- [`docs/configuration.zh-CN.md`](docs/configuration.zh-CN.md)
- [`docs/troubleshooting.md`](docs/troubleshooting.md)
- [`docs/troubleshooting.zh-CN.md`](docs/troubleshooting.zh-CN.md)

## Development

Run tests:

```bash
go test ./...
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution guidelines.

## Roadmap

- Faster model path for short `/ask` requests
- Better streaming reply behavior
- Clearer config validation and startup diagnostics
- Deployment examples for Docker / launchd / systemd
- Additional GitHub-ready project polish

## License

This repository is licensed under the [MIT License](LICENSE).
